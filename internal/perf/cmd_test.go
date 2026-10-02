package perf

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func run(args ...string) (code int, stdout, stderr string) {
	var out, errb bytes.Buffer
	code = Main(args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestValidateSaysOkOrNamesTheProblemAndExits2(t *testing.T) {
	suite := copySuite(t)
	for _, target := range []string{suite, caseDir(suite, "txn.commit_single"), filepath.Join(suite, "fixtures", "wide_100k"),
		filepath.Join(suite, "branches.conf"), writeConf(t, suite)} {
		code, out, errs := run("validate", target)
		if code != ExitOK || !strings.Contains(out, ": ok") || strings.Contains(errs, suite) {
			t.Errorf("validate %s: code=%d out=%q err=%q", target, code, out, errs)
		}
	}
	// Spec §13 A7: an unknown key in case.json is exit 2 with the key named.
	dir := caseDir(suite, "txn.commit_single")
	rewrite(t, filepath.Join(dir, "case.json"), func(m map[string]any) { m["tolerence"] = 0.1 })
	code, _, errs := run("validate", dir)
	if code != ExitRefused || !strings.Contains(errs, "unknown key tolerence") || !strings.Contains(errs, filepath.Join(dir, "case.json")) {
		t.Errorf("code=%d err=%q", code, errs)
	}
	// And the suite as a whole says the same, one line per problem.
	code, _, errs = run("validate", suite)
	if code != ExitRefused || strings.Count(errs, "\n") != 1 {
		t.Errorf("code=%d err=%q", code, errs)
	}
}

// A conf is validated with what it points at: a canary the suite does not
// have, or a registration whose cases= selects nothing, is the session
// stopping on Saturday night, and validate says so on Friday.
func TestValidateConfReadsWhatItPointsAt(t *testing.T) {
	suite := copySuite(t)
	code, _, errs := run("validate", writeConf(t, suite, "canaries = txn.commit_single, storage.pgbuf_cold_scan"))
	if code != ExitRefused || !strings.Contains(errs, "canary storage.pgbuf_cold_scan is not a case") {
		t.Errorf("code=%d err=%q", code, errs)
	}
	code, out, errs := run("validate", writeConf(t, suite))
	if code != ExitOK || !strings.Contains(out, "note: old/expired: expired 2020-01-01") {
		t.Errorf("code=%d out=%q err=%q", code, out, errs)
	}
	code, _, errs = run("validate", writeConf(t, suite, "suite = "+filepath.Join(suite, "nowhere")))
	if code != ExitRefused || !strings.Contains(errs, "no cases/ directory") {
		t.Errorf("code=%d err=%q", code, errs)
	}
}

func TestListPrintsTheCasesAndTheBound(t *testing.T) {
	suite := copySuite(t)
	for _, args := range [][]string{{"list", "--suite", suite}, {"list", "-c", writeConf(t, suite)}} {
		code, out, errs := run(args...)
		if code != ExitOK || errs != "" {
			t.Fatalf("%v: code=%d err=%q", args, code, errs)
		}
		for _, want := range []string{"ID", "txn.commit_single", "narrow_1m@1", "1440", "lib.backupdb", "utility", "3 case(s), 2 fixture(s)"} {
			if !strings.Contains(out, want) {
				t.Errorf("%v: output lacks %q:\n%s", args, want, out)
			}
		}
	}
	if code, _, _ := run("list"); code != ExitRefused {
		t.Error("list with neither -c nor --suite was accepted")
	}
}

func TestVerbsNotHereYetAreRefusedByName(t *testing.T) {
	for _, verb := range []string{"session", "run", "frobnicate", ""} {
		args := []string{verb}
		if verb == "" {
			args = nil
		}
		if code, _, errs := run(args...); code != ExitRefused || errs == "" {
			t.Errorf("%q: code=%d err=%q", verb, code, errs)
		}
	}
}
