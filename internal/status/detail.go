package status

import (
	"bufio"
	"io"
	"net/http"
	"os"
	"strconv"
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
	// from is where this run's own output begins: the size of the file when
	// the run said where it was.
	//
	// feedback.log is appended to, and a result directory is reused. So a case
	// that ran yesterday and has not run yet today still has a block in there,
	// and a scan from the start hands it to the page as though it were this
	// run's. That is not hypothetical -- the same accumulation in
	// test_<env>.log sent a reader an hour after a trace from the day before,
	// and the page would have done it silently.
	//
	// Everything before this offset belongs to a run that is over. The file
	// ends every line with a newline, so the offset is a line boundary; and if
	// the file is ever shorter than this, it was replaced rather than appended
	// to and the offset means nothing, so the scan starts again from zero.
	from int64
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

// DetailSince is Detail for a run that is about to write: the file's current
// length is remembered, and only what comes after it is this run's.
//
// The two are separate because a replay means the opposite. A replay is handed
// a finished log and everything in it is the run being replayed, so a replay
// that skipped to the end would show nothing at all -- which is what happened
// when this was one method: two tests went red saying the block they had just
// written was not there, and they were right.
func (b *Board) DetailSince(feedbackPath string) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.detail = &detail{path: feedbackPath, from: sizeOf(feedbackPath)}
}

// sizeOf is the file's length now, and zero for a file that is not there yet --
// which is the common case, and the right answer for it.
func sizeOf(path string) int64 {
	st, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return st.Size()
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

func (d *detail) where() (string, int64) {
	if d == nil {
		return "", 0
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.path, d.from
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
	path, from := d.where()
	if path == "" || name == "" {
		return ""
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	if st, serr := f.Stat(); serr == nil && st.Size() < from {
		from = 0
	}
	if from > 0 {
		if _, serr := f.Seek(from, io.SeekStart); serr != nil {
			return ""
		}
	}

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
		where, from := d.where()
		w.Write([]byte("nothing recorded for this case yet.\n\n" +
			"A case gets a block in feedback.log when it finishes. If it has finished,\n" +
			"the run may be writing to a different result tree than the page was told about:\n  " +
			where + "\n\n" +
			"Only this run's own part of that file is read -- everything before byte " +
			strconv.FormatInt(from, 10) + " belongs to a run that is over.\n"))
		return
	}
	w.Write([]byte(text))
}
