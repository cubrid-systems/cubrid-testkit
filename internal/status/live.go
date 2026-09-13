package status

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
)

// A running case has nothing in feedback.log: the block is written when it
// finishes. What it does have is its own result file, which CTP's write_ok and
// write_nok append a line to as the case goes -- so "held 325 s against a plan
// of 6" becomes "and it is stuck after check 3 of 40", which is the difference
// between knowing a case is slow and knowing where.
//
// The path is registered rather than derived. Working out a case's result file
// means Split, which lives in the shell suite, and the shell suite imports this
// package; the worker knows both and simply says.
type liveFiles struct {
	mu sync.Mutex
	of map[string]string
}

// Live records the file the page shows while a case is running, and an empty
// path when it finishes, so the page stops offering a file that is about to be
// reclaimed.
//
// What that file is belongs to the suite. A shell case writes its verdicts as
// it goes, so it is the growing result; a sql case writes nothing until it ends
// -- the executor renders the whole case at once -- so it is the case's own
// statements, which is what "what is it doing" means there.
func (b *Board) Live(name, path string) {
	if b == nil || name == "" {
		return
	}
	b.live.mu.Lock()
	defer b.live.mu.Unlock()
	if b.live.of == nil {
		b.live.of = map[string]string{}
	}
	if path == "" {
		delete(b.live.of, name)
		return
	}
	b.live.of[name] = path
}

func (b *Board) livePath(name string) string {
	b.live.mu.Lock()
	defer b.live.mu.Unlock()
	return b.live.of[name]
}

// liveTail is how much of a running case's result file the page shows. A case
// that writes hundreds of checks is one whose last few are the interesting ones.
const liveTail = 16 << 10

// serveLive answers /live?name=<case> with what the case has written so far.
func (b *Board) serveLive(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")

	path := b.livePath(name)
	if path == "" {
		fmt.Fprintln(w, "This case is not running here, so it has no live output.")
		fmt.Fprintln(w, "A finished case's full log is under case detail.")
		return
	}
	f, err := os.Open(path)
	if err != nil {
		// Before the case's first write_ok there is no file, which is a state
		// worth naming rather than an error worth reporting.
		fmt.Fprintf(w, "%s\n\nNothing written yet: the case has not reached its first check.\n",
			strings.TrimPrefix(path, "/"))
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	at := int64(0)
	if info.Size() > liveTail {
		at = info.Size() - liveTail
		fmt.Fprintf(w, "... %d earlier bytes\n\n", at)
	}
	if _, err := f.Seek(at, io.SeekStart); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := io.Copy(w, f); err != nil {
		return
	}
	if info.Size() == 0 {
		fmt.Fprintln(w, "(the file is empty: the case has not reached its first check)")
	}
}
