package status

import (
	"bufio"
	"net/http"
	"os"
	"strings"
	"sync"
)

// What a case actually did, for the failure you are looking at.
//
// The page can say a case failed and how long it took. What it could not say is
// *why*, and that is the question a NOK provokes -- which is answered already, in
// feedback.log: every case's block there carries its checks and, for a failing
// one, the console output of the run. Nothing new has to be recorded; the page
// just has to be able to find it.
//
// A linear scan of the file per click. feedback.log for a full corpus is tens of
// megabytes and a click is rare, so the alternative -- an index held in memory
// for the whole run -- would cost every run to make a few clicks faster.
type detail struct {
	mu   sync.Mutex
	path string
	// fn answers instead of the file, for a runner whose cases leave no
	// feedback.log -- the sql suite's are a rendering and an answer.
	fn func(name string) string
}

// Detail says where the run's feedback.log is, which is what makes a finished
// case clickable. Without it the page still works and simply does not offer the
// link.
func (b *Board) Detail(feedbackPath string) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.detail = &detail{path: feedbackPath}
}

// DetailFunc is Detail for a runner whose cases leave no feedback.log: fn
// says what a finished case did, and "" for a case it has nothing on yet.
func (b *Board) DetailFunc(fn func(name string) string) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.detail = &detail{fn: fn}
}

func (d *detail) where() string {
	if d == nil {
		return ""
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.path
}

// block returns the part of feedback.log that belongs to one case.
//
// The file is a sequence of blocks, each opened by a [OK]/[NOK] line naming the
// case. So the block is from the header that names this case to the next header,
// whatever that names.
func (d *detail) block(name string) string {
	if d.fn != nil {
		return d.fn(name)
	}
	path := d.where()
	if path == "" || name == "" {
		return ""
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	var out strings.Builder
	in := false
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 0, 64<<10), 8<<20)
	for s.Scan() {
		line := s.Text()
		header := strings.HasPrefix(line, "[OK]") || strings.HasPrefix(line, "[NOK]")
		if header {
			// The last block wins: a retried case appears twice and the second
			// attempt is the one that produced the verdict on the page.
			if strings.Contains(line, name+" ") || strings.HasSuffix(line, name) {
				in = true
				out.Reset()
			} else if in {
				in = false
			}
		}
		if in {
			out.WriteString(line)
			out.WriteByte('\n')
			// A block is bounded so that one runaway case cannot hand the browser
			// a hundred megabytes.
			if out.Len() > 512<<10 {
				out.WriteString("\n… truncated; the rest is in feedback.log\n")
				break
			}
		}
	}
	return out.String()
}

// serveDetail answers /case?name=<path> with the case's own block, as text.
func (b *Board) serveDetail(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	b.mu.Lock()
	d := b.detail
	b.mu.Unlock()

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if d == nil {
		http.Error(w, "this run did not say where its feedback.log is", http.StatusNotFound)
		return
	}
	text := d.block(name)
	if text == "" {
		// Not an error: a case that has not finished has no block yet, and
		// saying so is more use than a 404.
		if d.fn != nil {
			w.Write([]byte("nothing recorded for this case yet: it has not finished.\n"))
			return
		}
		w.Write([]byte("nothing recorded for this case yet.\n\n" +
			"A case gets a block in feedback.log when it finishes. If it has finished,\n" +
			"the run may be writing to a different result tree than the page was told about:\n  " +
			d.where() + "\n"))
		return
	}
	w.Write([]byte(text))
}
