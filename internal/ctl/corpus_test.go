package ctl

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestAgainstParseC runs ctltool's own parser and this port over every .ctl file
// of a corpus and requires the same statements from both. It is the only test
// that can say the port is a port; the unit tests above only say it is sane.
//
//	CTP_HOME=/path/to/CTP TESTKIT_CTL_CORPUS=/path/to/cases go test ./internal/ctl/
//
// Without both, it skips: the C source is CTP's and the corpus is not in this
// repository.
func TestAgainstParseC(t *testing.T) {
	ctp, corpus := os.Getenv("CTP_HOME"), os.Getenv("TESTKIT_CTL_CORPUS")
	if ctp == "" || corpus == "" {
		t.Skip("set CTP_HOME and TESTKIT_CTL_CORPUS to compare against parse.c")
	}
	ctltool := filepath.Join(ctp, "isolation", "ctltool")
	dump := filepath.Join(t.TempDir(), "dumpstmt")
	build := exec.Command("gcc", "-w", "-I", ctltool, "-o", dump,
		"testdata/dumpstmt.c", filepath.Join(ctltool, "parse.c"), filepath.Join(ctltool, "common.c"))
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building the C parser: %v\n%s", err, out)
	}

	var files []string
	err := filepath.Walk(corpus, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !fi.IsDir() && strings.HasSuffix(path, ".ctl") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatalf("no .ctl files under %s", corpus)
	}

	differ := 0
	for _, path := range files {
		want, err := exec.Command(dump, path).Output()
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		var got strings.Builder
		r := NewReader(f)
		for n := 1; ; n++ {
			s, ok := r.Next()
			if !ok {
				break
			}
			s = strings.ReplaceAll(s, `\`, `\\`)
			s = strings.ReplaceAll(s, "\n", `\n`)
			fmt.Fprintf(&got, "%d\t%s\n", n, s)
		}
		f.Close()
		if got.String() != string(want) {
			if differ < 5 {
				t.Errorf("%s: parsed differently\n--- parse.c ---\n%s\n--- port ---\n%s",
					path, firstDifference(string(want), got.String()), firstDifference(got.String(), string(want)))
			}
			differ++
		}
	}
	if differ > 0 {
		t.Errorf("%d of %d files parsed differently", differ, len(files))
	} else {
		t.Logf("%d files, every statement identical", len(files))
	}
}

// firstDifference returns the first line of a that b does not have in the same
// place, with its number, so a failure names one line instead of a whole file.
func firstDifference(a, b string) string {
	al, bl := strings.Split(a, "\n"), strings.Split(b, "\n")
	for i := range al {
		if i >= len(bl) || al[i] != bl[i] {
			return fmt.Sprintf("line %d: %s", i+1, al[i])
		}
	}
	return "(no line differs; the other side is longer)"
}

// TestCorpusVocabulary classifies every statement of every case and requires
// that the controller knows all of them: nothing Unknown, and nothing Retired.
// It is what lets ADR-019 drop thirteen commands -- if a case ever starts using
// one, this fails and names it, rather than the run doing something quietly
// different.
//
//	TESTKIT_CTL_CORPUS=/path/to/cases go test ./internal/ctl/
func TestCorpusVocabulary(t *testing.T) {
	corpus := os.Getenv("TESTKIT_CTL_CORPUS")
	if corpus == "" {
		t.Skip("set TESTKIT_CTL_CORPUS to classify the corpus")
	}
	counts, inFiles := map[string]int{}, map[string]int{}
	files, unknown, retired := 0, map[string]string{}, map[string]string{}
	// A client number above the count the case declared is an error qactl
	// refuses (VALID_CLIENT_ID, qactl.c:2474). Whether that path is reachable
	// at all is worth knowing before reimplementing its message.
	outOfRange := map[string]string{}
	err := filepath.Walk(corpus, func(path string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() || !strings.HasSuffix(path, ".ctl") {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		files++
		seen := map[string]bool{}
		declared, first := 0, true
		r := NewReader(f)
		for {
			s, ok := r.Next()
			if !ok {
				for k := range seen {
					inFiles[k]++
				}
				return nil
			}
			c := Classify(s)
			if first {
				declared, first = NumClients(s), false
			}
			if c.Client > declared && len(outOfRange) < 10 {
				outOfRange[path] = fmt.Sprintf("C%d, and the case declares %d", c.Client, declared)
			}
			switch c.Kind {
			case Unknown:
				if len(unknown) < 10 {
					unknown[c.Name] = path
				}
			case Retired:
				if len(retired) < 10 {
					retired[c.Name] = path
				}
			default:
				counts[kindName(c.Kind)]++
				seen[kindName(c.Kind)] = true
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, path := range unknown {
		t.Errorf("unknown command %q in %s", name, path)
	}
	for name, path := range retired {
		t.Errorf("retired command %q is in use, in %s", name, path)
	}
	for path, what := range outOfRange {
		t.Errorf("%s names %s", path, what)
	}
	t.Logf("%d files", files)
	for _, k := range []string{"setup", "wait ready", "wait blocked", "wait unblocked",
		"sleep", "deadlock pause", "to client"} {
		t.Logf("  %-15s %5d files %7d occurrences", k, inFiles[k], counts[k])
	}
}

func kindName(k Kind) string {
	switch k {
	case ToClient:
		return "to client"
	case Setup:
		return "setup"
	case WaitReady:
		return "wait ready"
	case WaitBlocked:
		return "wait blocked"
	case WaitUnblocked:
		return "wait unblocked"
	case Sleep:
		return "sleep"
	case DeadlockPause:
		return "deadlock pause"
	}
	return "?"
}
