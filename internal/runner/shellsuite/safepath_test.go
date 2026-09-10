package shellsuite

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// An unset or careless path in a shell command does not fail. It becomes the
// root of the filesystem.
func TestADirectoryTooShallowToEmptyIsRefused(t *testing.T) {
	for _, bad := range []string{"", "   ", "/", "//", "/usr", "/home", "/data", "relative/path", "~/x"} {
		if err := safeToEmpty("testcase_workspace_dir", bad); err == nil {
			t.Errorf("%q was accepted as a directory to empty", bad)
		}
	}
	for _, ok := range []string{"/data/run/workspace", "/home/qa/ws", "/tmp/a/b/c"} {
		if err := safeToEmpty("testcase_workspace_dir", ok); err != nil {
			t.Errorf("%q should be fine: %v", ok, err)
		}
	}
}

// A path is interpolated into a script, so a space in it turns one rm into two.
func TestAPathTheShellWouldReadAsSyntaxIsRefused(t *testing.T) {
	for _, bad := range []string{
		"/data/my runs/ws", "/data/ws;rm -rf /", "/data/$HOME/ws", "/data/`id`/ws",
		"/data/ws*", "/data/a|b/ws",
	} {
		if err := safeToEmpty("testcase_workspace_dir", bad); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}

// The idiom is '\” -- close, escape, reopen. Writing ”' closes and reopens,
// which is an empty string, so a'b becomes ab and a delete aimed at one path
// lands on another.
func TestQuotingSurvivesASingleQuote(t *testing.T) {
	dir := t.TempDir()
	odd := filepath.Join(dir, "a'b")
	if err := os.WriteFile(odd, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	decoy := filepath.Join(dir, "ab")
	if err := os.WriteFile(decoy, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := exec.Command("bash", "-c", "rm -rf -- "+shQuote(odd)).CombinedOutput()
	if err != nil {
		t.Fatalf("the quoted path did not survive the shell: %v: %s", err, out)
	}
	if _, err := os.Stat(odd); err == nil {
		t.Error("the file the command named is still there")
	}
	if _, err := os.Stat(decoy); err != nil {
		t.Error("a different file was deleted, which is what ''' does")
	}

	// And the shell has to see exactly one word.
	got, err := exec.Command("bash", "-c", "printf '%s\\n' "+shQuote("a b'c d")).Output()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimRight(string(got), "\n") != "a b'c d" {
		t.Errorf("quoting produced %q", got)
	}
}
