package sqlsuite

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// fakeExecutor speaks the executor's side of the protocol: two startup lines
// in READY, a rendering per X that names the case and its length, an error for
// F, and death on a case called "die".
const fakeExecutor = `
printf 'READY 3 2\n\n\n'
echo "a word on stderr" >&2
while read -r verb path; do
  case "$verb" in
    X) [ "$path" = die ] && exit 3
       out="rendered $path
with a second line"
       printf 'R %d 42\n%s' "${#out}" "$out" ;;
    F) printf 'E no report for %s\n' "$path" ;;
    S) printf 'Full thread dump OpenJDK 64-Bit Server VM\n' ;;
  esac
done
`

func TestTheExecutorProtocol(t *testing.T) {
	var said []string
	x, err := attach(exec.Command("bash", "-c", fakeExecutor), func(line string) { said = append(said, line) })
	if err != nil {
		t.Fatal(err)
	}
	if x.cases != 3 || len(x.startup) != 2 || x.startup[0] != "" || x.startup[1] != "" {
		t.Errorf("READY read as %d cases and startup %q", x.cases, x.startup)
	}

	// A rendering holds line breaks of its own; the length, not a line, frames it.
	for _, name := range []string{"/c/one.sql", "/c/a case with spaces.sql"} {
		out, ms, err := x.Run(t.Context(), name)
		if err != nil {
			t.Fatal(err)
		}
		if want := "rendered " + name + "\nwith a second line"; string(out) != want || ms != 42 {
			t.Errorf("Run(%q) = %q, %d; want %q, 42", name, out, ms, want)
		}
	}
	if _, _, err := x.Run(t.Context(), "bad\npath"); err == nil {
		t.Error("a path with a line break would split the request")
	}
	// An E is the case's, and the executor carries on.
	if got := x.Failure("/c/one.sql"); got != "" {
		t.Errorf("an E for a report should be no report, got %q", got)
	}
	if _, _, err := x.Run(t.Context(), "/c/after.sql"); err != nil {
		t.Errorf("the executor should still answer after an E: %v", err)
	}

	// A line that is neither a reply nor an error means the two sides no longer
	// agree where replies start; the next answer would be the wrong case's.
	if _, _, err := x.ask("S", "/c/stray.sql"); !errors.Is(err, errExecutorGone) {
		t.Errorf("a stray line on the protocol should end the executor, got %v", err)
	}

	// Death is the run's: it is told apart, with what the executor last said.
	_, _, err = x.Run(t.Context(), "die")
	if !errors.Is(err, errExecutorGone) {
		t.Fatalf("got %v, want errExecutorGone", err)
	}
	if !strings.Contains(err.Error(), "a word on stderr") {
		t.Errorf("the error should carry the executor's last words: %v", err)
	}
	x.Close()
	if len(said) == 0 || said[0] != "a word on stderr" {
		t.Errorf("stderr lines should reach say: %q", said)
	}
}

func TestAnExecutorThatNeverStartsSaysWhy(t *testing.T) {
	_, err := attach(exec.Command("bash", "-c", `echo "E checkDb failed"; echo "cannot connect" >&2; exit 2`), nil)
	if err == nil || !strings.Contains(err.Error(), "E checkDb failed") || !strings.Contains(err.Error(), "cannot connect") {
		t.Errorf("got %v", err)
	}
}
