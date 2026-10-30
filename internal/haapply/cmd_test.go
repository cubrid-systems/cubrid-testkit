package haapply

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
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
	if _, err := os.Stat(there); err != nil {
		t.Errorf("the file that was there before the run is gone: %v", err)
	}
}

func TestPairsHelpIsNotAnError(t *testing.T) {
	if code := Main([]string{"pairs", "-h"}, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Errorf("pairs -h exited %d, want 0", code)
	}
}
