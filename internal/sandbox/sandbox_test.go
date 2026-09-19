package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeCSB writes a stand-in for the tool: a script that records the argv it was
// given and prints the envelope the test wants back.
//
// A stand-in rather than the real thing, because the real thing needs Docker and
// a CUBRID build and this is a test of what this package sends and reads. What
// it cannot check is that csb still answers this way, which is why the shape it
// imitates is quoted from csb's own source in each test.
func fakeCSB(t *testing.T, body string) (bin, argvLog string) {
	t.Helper()
	dir := t.TempDir()
	bin = filepath.Join(dir, "csb")
	argvLog = filepath.Join(dir, "argv")
	// NUL-separated: an argument may itself contain newlines -- that is the
	// point of one of the tests below -- so a newline cannot also separate them.
	script := "#!/bin/bash\n" +
		"printf '%s\\0' \"$@\" > " + argvLog + "\n" + body + "\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, argvLog
}

func argv(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := strings.Split(string(b), "\x00")
	return out[:len(out)-1] // printf leaves a trailing separator

}

// The flags go before the separator. `node exec` ends in `-- <command>` and
// everything after that belongs to the command, so a --cluster appended at the
// end would be handed to the node as part of the script it runs.
func TestTheClusterFlagIsNotSweptIntoTheCommand(t *testing.T) {
	bin, log := fakeCSB(t, `echo '{"schema":"csb/v1","ok":true,"data":{"master":{"exit":0,"stdout":"","stderr":""}}}'`)
	n := NewNode(&CLI{Bin: bin, Cluster: "hadb"}, "master")
	if _, err := n.Run(context.Background(), "echo hello"); err != nil {
		t.Fatal(err)
	}
	got := argv(t, log)
	sep := -1
	for i, a := range got {
		if a == "--" {
			sep = i
		}
	}
	if sep == -1 {
		t.Fatalf("no separator in %v", got)
	}
	for _, a := range got[sep+1:] {
		if strings.HasPrefix(a, "--") {
			t.Errorf("%q is after the separator, so the node would run it: %v", a, got)
		}
	}
	if len(got[sep+1:]) != 1 {
		t.Errorf("the command should be one argument, got %d: %v", len(got[sep+1:]), got[sep+1:])
	}
}

// A script goes as one argument, newlines and all. csb joins what follows the
// separator with spaces (internal/cli/cluster.go: strings.Join(c.Args[1:], " ")),
// so a script split across arguments comes back with its line breaks turned into
// spaces -- which is a different program that usually still runs.
func TestAScriptArrivesWholeWithItsNewlines(t *testing.T) {
	bin, log := fakeCSB(t, `echo '{"schema":"csb/v1","ok":true,"data":{"master":{"exit":0,"stdout":"","stderr":""}}}'`)
	n := NewNode(&CLI{Bin: bin, Cluster: "hadb"}, "master")
	script := "set -e\ncd /tmp\necho done"
	if _, err := n.Run(context.Background(), script); err != nil {
		t.Fatal(err)
	}
	got := argv(t, log)
	last := got[len(got)-1]
	if last != script {
		t.Errorf("the script did not arrive whole:\nwant %q\ngot  %q", script, last)
	}
}

// A command that ran and failed is data, not an error. csb exits 0 and carries
// the inner status in data.<node>.exit with a remote_exit_nonzero note, which is
// the same distinction exec.Channel's contract makes.
func TestANonZeroExitIsDataAndNotAnError(t *testing.T) {
	bin, _ := fakeCSB(t, `echo '{"schema":"csb/v1","ok":true,`+
		`"data":{"master":{"exit":3,"stdout":"out","stderr":"err"}},`+
		`"notes":[{"code":"remote_exit_nonzero","severity":"warn","message":"the command exited 3"}]}'`)
	n := NewNode(&CLI{Bin: bin, Cluster: "hadb"}, "master")
	res, err := n.Run(context.Background(), "false")
	if err != nil {
		t.Fatalf("a failing command was reported as a channel failure: %v", err)
	}
	if res.ExitCode != 3 || res.Stdout != "out" || res.Stderr != "err" {
		t.Errorf("got %+v", res)
	}
}

// A tool that has moved to a second envelope version still prints JSON. Reading
// it as if nothing had changed is the quiet failure this whole integration is
// arranged to avoid, so the version is checked.
func TestASecondSchemaIsRefused(t *testing.T) {
	bin, _ := fakeCSB(t, `echo '{"schema":"csb/v2","ok":true,"data":{}}'`)
	n := NewNode(&CLI{Bin: bin, Cluster: "hadb"}, "master")
	_, err := n.Run(context.Background(), "echo hi")
	if err == nil || !strings.Contains(err.Error(), "csb/v2") {
		t.Fatalf("a second schema was accepted: %v", err)
	}
}

// ok:false is the tool saying the command did not happen. The notes carry why,
// and the error repeats them with their codes.
func TestARefusalCarriesItsNotes(t *testing.T) {
	bin, _ := fakeCSB(t, `echo '{"schema":"csb/v1","ok":false,"data":{},`+
		`"notes":[{"code":"unresolved_selector","severity":"error","message":"no node matches"}]}'`)
	n := NewNode(&CLI{Bin: bin, Cluster: "hadb"}, "nope")
	_, err := n.Run(context.Background(), "echo hi")
	if err == nil || !strings.Contains(err.Error(), "unresolved_selector") {
		t.Fatalf("the refusal did not carry its note: %v", err)
	}
}

// A channel is one node. A selector that matched several would make "the result"
// a choice this package is not entitled to make.
func TestASelectorMatchingSeveralNodesIsRefused(t *testing.T) {
	bin, _ := fakeCSB(t, `echo '{"schema":"csb/v1","ok":true,"data":{`+
		`"master":{"exit":0,"stdout":"a","stderr":""},`+
		`"slave":{"exit":0,"stdout":"b","stderr":""}}}'`)
	n := NewNode(&CLI{Bin: bin, Cluster: "hadb"}, "all")
	_, err := n.Run(context.Background(), "echo hi")
	if err == nil || !strings.Contains(err.Error(), "selects 2 nodes") {
		t.Fatalf("two nodes were read as one result: %v", err)
	}
}

// Nothing printed at all, and the tool failed: the error has to name what did
// not run rather than a JSON parse error nobody can act on.
func TestAToolThatCannotRunSaysSo(t *testing.T) {
	n := NewNode(&CLI{Bin: "/nonexistent/csb", Cluster: "hadb"}, "master")
	_, err := n.Run(context.Background(), "echo hi")
	if err == nil || !strings.Contains(err.Error(), "/nonexistent/csb") {
		t.Fatalf("got %v", err)
	}
}

func TestPutAndGetSayWhatWouldHaveToExist(t *testing.T) {
	n := NewNode(&CLI{Bin: "csb", Cluster: "hadb"}, "master")
	for _, err := range []error{
		n.Put(context.Background(), "/a", "/b"),
		n.Get(context.Background(), "/b", "/a"),
	} {
		if err == nil || !strings.Contains(err.Error(), "operation 11") {
			t.Errorf("a refusal that does not name the seam: %v", err)
		}
	}
}
