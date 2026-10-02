package perf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeConf renders testdata/suite/perf.conf against a suite directory, with
// edits applied as whole lines: a line that starts with a key replaces it, a
// key alone removes it.
func writeConf(t *testing.T, suite string, edits ...string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "suite", "perf.conf"))
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ReplaceAll(string(b), "${SUITE}", suite)
	for _, e := range edits {
		key, _, _ := strings.Cut(e, "=")
		key = strings.TrimSpace(key)
		var kept []string
		for _, line := range strings.Split(text, "\n") {
			k, _, _ := strings.Cut(line, "=")
			if strings.TrimSpace(k) == key {
				continue
			}
			kept = append(kept, line)
		}
		text = strings.Join(kept, "\n")
		if strings.Contains(e, "=") {
			text += "\n" + e + "\n"
		}
	}
	path := filepath.Join(t.TempDir(), "perf.conf")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestConfReadsTheSpecsExample(t *testing.T) {
	suite := copySuite(t)
	c, err := ReadConf(writeConf(t, suite))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Pairs) != 1 {
		t.Fatalf("pairs = %+v", c.Pairs)
	}
	p := c.Pairs[0]
	if p.Name != "develop" || p.Target != "develop-HEAD" || p.Reference != "/data/builds/cubrid/release-11.4-0e7d3c1" || p.Overlap != "/data/builds/cubrid/release-11.5-1a2b3c4" {
		t.Errorf("pair = %+v", p)
	}
	if c.Suite != suite || c.Branches != filepath.Join(suite, "branches.conf") || c.BranchesMax != 3 {
		t.Errorf("suite=%q branches=%q max=%d", c.Suite, c.Branches, c.BranchesMax)
	}
	if strings.Join(c.Canaries, ",") != "txn.commit_single,lib.backupdb" || c.CanaryTolerance != 0.05 {
		t.Errorf("canaries=%v tolerance=%v", c.Canaries, c.CanaryTolerance)
	}
	if c.Interleave != "case" || c.SessionBudgetS != 187200 || c.MemoryCap != "24G" || c.DiskMinGB != 100 {
		t.Errorf("interleave=%q budget=%d cap=%q disk=%d", c.Interleave, c.SessionBudgetS, c.MemoryCap, c.DiskMinGB)
	}
	if c.CPUSetServer != "0-7,16-23" || c.CPUSetClient != "8-15,24-31" || c.ClientImage != "localhost/perf-client:2026-10" {
		t.Errorf("cpuset=%q/%q image=%q", c.CPUSetServer, c.CPUSetClient, c.ClientImage)
	}
	if c.ReportMode != "dry" || c.ReportWebhookFormat != "teams" || c.ConbenchURL != "http://100.118.51.99:5000" {
		t.Errorf("report=%q/%q conbench=%q", c.ReportMode, c.ReportWebhookFormat, c.ConbenchURL)
	}
	// The optional keys are optional.
	if _, err := ReadConf(writeConf(t, suite, "branches", "branches.max", "memory_cap", "disk_min_gb", "cpuset.server",
		"cpuset.client", "report.mail", "report.webhook", "report.webhook_format", "conbench.url", "overlap.develop")); err != nil {
		t.Errorf("a conf with only the required keys was refused: %v", err)
	}
}

func TestConfRefusalsNameTheKey(t *testing.T) {
	cases := []struct {
		name  string
		edits []string
		want  string
	}{
		{"an unknown key", []string{"interleave_mode = case"}, "unknown key interleave_mode"},
		{"a missing required key", []string{"csb"}, "missing csb"},
		{"no pair", []string{"pair.develop", "overlap.develop"}, "no pair.<name>"},
		{"a pair with no reference", []string{"pair.develop = develop-HEAD"}, `pair.develop wants "<target> ; <reference>"`},
		{"an overlap with no pair", []string{"overlap.feature = /x"}, "overlap.feature has no pair.feature"},
		{"an interleave off the list", []string{"interleave = pair"}, `interleave "pair" is not one of case, round`},
		{"a report mode off the list", []string{"report.mode = loud"}, `report.mode "loud" is not one of dry, team`},
		{"a cpuset that is not a list", []string{"cpuset.server = 0-7, 16"}, "cpuset.server wants a CPU list"},
		{"a tolerance of zero", []string{"canary_tolerance = 0"}, "canary_tolerance must be a number above 0"},
		{"a budget that is not a number", []string{"session_budget_s = 52h"}, "session_budget_s must be an integer"},
		{"a canary that is not a case id", []string{"canaries = txn.commit_single, Commit"}, `canaries has "Commit"`},
		{"a memory cap in words", []string{"memory_cap = half"}, "memory_cap wants a size like 24G"},
		{"a webhook with no format", []string{"report.webhook_format"}, "report.webhook is set and report.webhook_format is not"},
		{"a conbench url with no scheme", []string{"conbench.url = 100.118.51.99:5000"}, "conbench.url is not a URL"},
	}
	suite := copySuite(t)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ReadConf(writeConf(t, suite, c.edits...))
			if err == nil {
				t.Fatalf("%s: accepted", c.name)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("%s: %q does not say %q", c.name, err, c.want)
			}
		})
	}
}
