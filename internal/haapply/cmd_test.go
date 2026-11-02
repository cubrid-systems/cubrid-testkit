package haapply

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/cubrid-systems/cubrid-testkit/internal/sandbox"
)

func TestListNamesEveryPattern(t *testing.T) {
	var out bytes.Buffer
	if code := Main([]string{"list"}, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("list exited %d", code)
	}
	for _, p := range Catalog {
		if !strings.Contains(out.String(), p.Name) {
			t.Errorf("list does not name %s", p.Name)
		}
	}
}

// A command it cannot read stops before csb is called: none of these names a
// cluster that exists, so reaching csb would fail differently.
func TestPairsRefusesWhatItCannotRunBeforeTouchingAPair(t *testing.T) {
	cases := map[string][]string{
		"neither":            {"pairs"},
		"both":               {"pairs", "--build", "/b", "--cluster", "x"},
		"unknown pattern":    {"pairs", "--cluster", "x", "pk_update", "no_such"},
		"workers on cluster": {"pairs", "--cluster", "x", "--workers", "2"},
		"zero rounds":        {"pairs", "--cluster", "x", "--rounds", "0"},
		"too many rounds":    {"pairs", "--cluster", "x", "--rounds", "1000"},
		"negative history":   {"pairs", "--build", "/b", "--history", "-5"},
	}
	for name, args := range cases {
		var errOut bytes.Buffer
		if code := Main(args, &bytes.Buffer{}, &errOut); code != 2 {
			t.Errorf("%s: exit %d, want 2 (stderr %q)", name, code, errOut.String())
		}
	}
}

func TestAnUnknownVerbIsRefused(t *testing.T) {
	if code := Main([]string{"pears"}, &bytes.Buffer{}, &bytes.Buffer{}); code != 2 {
		t.Errorf("exit %d, want 2", code)
	}
}

func TestTheLedgerCarriesTheRunAndAVerdictARow(t *testing.T) {
	var b bytes.Buffer
	ledgerHead(&b, []string{"cluster=x", "build=11.5.0.1-abc"})
	outs := []Outcome{
		{Pattern: "ctl", Rounds: 20, Ahead: 19, Final: "same"},
		{Pattern: "fp_negzero", Rounds: 20, Ahead: 17, Final: "differ:t_fp_negzero", FailDelta: 17},
		{Pattern: "pk_update", Rounds: 20, Final: "same", Writeset: Writeset{Write: 20}},
	}
	for i := range outs {
		ledgerRow(&b, outs[i], Judge(outs[:i+1])[i])
	}
	lines := strings.Split(strings.TrimSpace(b.String()), "\n")
	if len(lines) != 5 || !strings.HasPrefix(lines[0], "# cluster=x build=") {
		t.Fatalf("ledger:\n%s", b.String())
	}
	for i, want := range []string{ControlOvertook, Missed, Held} {
		if !strings.HasSuffix(lines[2+i], "\t"+want) {
			t.Errorf("row %d = %q, want verdict %s", i, lines[2+i], want)
		}
	}
	if !strings.Contains(lines[4], "\t20\t0\t0\t0\theld") {
		t.Errorf("pk_update row lost its write conflicts: %q", lines[4])
	}
}

// A run that fails before its header removes the ledger it made, and leaves a
// file that was there before it.
func TestAnEarlyFailureRemovesOnlyALedgerItMade(t *testing.T) {
	t.Setenv(sandbox.BinEnv, os.Args[0])
	t.Setenv(fakeCSBEnv, t.TempDir())
	t.Setenv("FAKE_DESCRIBE_FAIL", "1")
	dir := t.TempDir()
	made, there := filepath.Join(dir, "made.tsv"), filepath.Join(dir, "there.tsv")
	if err := os.WriteFile(there, []byte("kept\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{made, there} {
		if code := Main([]string{"pairs", "--cluster", "x", "--ledger", path}, &bytes.Buffer{}, &bytes.Buffer{}); code != 1 {
			t.Fatalf("a failing describe exited %d, want 1", code)
		}
	}
	if _, err := os.Stat(made); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the ledger this run made is still there: %v", err)
	}
	if b, err := os.ReadFile(there); err != nil || string(b) != "kept\n" {
		t.Errorf("the file that was there before the run is %q, %v", b, err)
	}
}

// A ledger that is a pipe or a device takes the header as a file does: there
// is nothing to truncate, and the run goes on. A FIFO here, so that drop is
// seen to leave it.
func TestALedgerMayBeAPipe(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "ledger")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	// A FIFO opened for writing waits for a reader.
	r, err := os.OpenFile(fifo, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	f, start, drop, err := ledgerFile(fifo)
	if err != nil {
		t.Fatal(err)
	}
	if err := start(); err != nil {
		t.Errorf("start on a FIFO: %v", err)
	}
	if _, err := f.WriteString("# header\n"); err != nil {
		t.Error(err)
	}
	drop()
	if _, err := os.Stat(fifo); err != nil {
		t.Errorf("drop removed a FIFO that was there before: %v", err)
	}
}

// A pair the run stands up may have ignore_trailing_space from its build's
// cubrid.conf, so naming a trailing-space pattern with --build is asked of
// the pair, not refused here.
func TestPairsTakesAnyPatternWithABuild(t *testing.T) {
	for _, args := range [][]string{{"--build", "/b"}, {"--build", "/b", "pk_update"}, {"--build", "/b", "vc_trail"}} {
		if _, err := parsePairs(args, &bytes.Buffer{}); err != nil {
			t.Errorf("%v: %v", args, err)
		}
	}
}

// On a pair that keeps trailing spaces the whole catalog runs without the
// trailing-space patterns and says so; the ledger that was at the path is
// replaced once the run measures. The fake's csql dies in the first pattern
// after the control, so the run stops there: the whole catalog through the
// fake takes minutes under -race, and what is asked here comes before it.
func TestTheCatalogLeavesTheTrailingSpacePatternsOutWhereTheKeysAreTwo(t *testing.T) {
	state := fakeEnv(t, "1")
	setState(t, state, "look", "1")
	t.Setenv("CRASH", "_fp_negzero")
	ledger := filepath.Join(t.TempDir(), "l.tsv")
	if err := os.WriteFile(ledger, []byte(strings.Repeat("an earlier run\n", 1000)), 0o644); err != nil {
		t.Fatal(err)
	}
	var errOut bytes.Buffer
	code := Main([]string{"pairs", "--cluster", "fake", "--rounds", "1", "--rows", "1", "--ledger", ledger},
		&bytes.Buffer{}, &errOut)
	if code != 1 || !strings.Contains(errOut.String(), "so vc_trail, fk_vc_trail are not run") ||
		!strings.Contains(errOut.String(), "ha-apply: fp_negzero: ") {
		t.Fatalf("exit %d, stderr %q", code, errOut.String())
	}
	b, err := os.ReadFile(ledger)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 3 || !strings.Contains(lines[0], " ignore_trailing_space=no ") ||
		!strings.HasPrefix(lines[2], "ctl\t") || strings.Contains(string(b), "an earlier run") {
		t.Errorf("ledger:\n%s", b)
	}
}

// The patterns the pair leaves out are not run: a trailing-space pattern run
// where its keys are two would write a missed that is not one.
func TestTheLeftOutPatternsAreNotRun(t *testing.T) {
	if raceEnabled {
		t.Skip("the whole catalog through the fake takes minutes under -race")
	}
	state := fakeEnv(t, "1")
	setState(t, state, "look", "1")
	ledger := filepath.Join(t.TempDir(), "l.tsv")
	var errOut bytes.Buffer
	if code := Main([]string{"pairs", "--cluster", "fake", "--rounds", "1", "--rows", "1", "--ledger", ledger},
		&bytes.Buffer{}, &errOut); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut.String())
	}
	b, err := os.ReadFile(ledger)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 2+len(Catalog)-2 || strings.Contains(string(b), "vc_trail") {
		t.Errorf("ledger:\n%s", b)
	}
}

// A trailing-space pattern named for a pair that keeps trailing spaces is
// refused before anything is measured: no header, and no ledger left.
func TestATrailingSpacePatternIsRefusedOnAPairThatKeepsTheSpaces(t *testing.T) {
	fakeEnv(t, "0")
	ledger := filepath.Join(t.TempDir(), "l.tsv")
	var out, errOut bytes.Buffer
	code := Main([]string{"pairs", "--cluster", "fake", "--ledger", ledger, "vc_trail"}, &out, &errOut)
	if code != 1 || out.Len() != 0 || !strings.Contains(errOut.String(), "vc_trail cannot run there") {
		t.Errorf("exit %d, stdout %q, stderr %q", code, out.String(), errOut.String())
	}
	if _, err := os.Stat(ledger); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the ledger was left: %v", err)
	}
}

func TestPairsHelpIsNotAnError(t *testing.T) {
	if code := Main([]string{"pairs", "-h"}, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Errorf("pairs -h exited %d, want 0", code)
	}
}
