package coredump

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tkexec "github.com/cubrid-systems/cubrid-testkit/internal/exec"
)

// report is the head of one the engine wrote on this project's machine, when
// the server died in the aggregate hash table (CBRD-27407).
const report = `process info : cub_server ctldb

[20] 0x000000000029d8c5: er_dump_call_stack_internal(print_output&) at /src/base/stack_dump.c:390
[19] 0x000000000029e164: print_output::~print_output() at /src/base/printer.hpp:43
[18] 0x00000000002559a2: er_print_crash_callstack at /src/base/error_manager.c:3399
[17] 0x0000000000045330: __restore_rt at libc_sigaction.c:?
[16] 0x000000000042f4ee: qdata_save_agg_hentry_to_list(cubthread::entry*, cubquery::aggregate_hash_key*) at /src/query/query_aggregate.cpp:2974
[15] 0x000000000042fcc6: qdata_save_agg_htable_to_list(cubthread::entry*, mht_table*) at /src/query/query_aggregate.cpp:3264
`

func TestReadHeadSkipsTheHandlersOwnFrames(t *testing.T) {
	program, where := readHead(report)
	if program != "cub_server ctldb" {
		t.Errorf("program = %q, want the report's first line", program)
	}
	if where != "qdata_save_agg_hentry_to_list at query_aggregate.cpp:2974" {
		t.Errorf("where = %q, want the first frame below the handler", where)
	}
}

func TestAReportWithNothingToRead(t *testing.T) {
	if program, where := readHead("not a crash report at all\n"); program != "" || where != "" {
		t.Errorf("got %q, %q; want nothing", program, where)
	}
}

// chan1 answers the two commands Crashes runs, and records what it was asked.
type chan1 struct {
	list string
	head string
	cat  string
	ran  []string
}

func (c *chan1) Run(_ context.Context, script string) (tkexec.Result, error) {
	c.ran = append(c.ran, script)
	if strings.HasPrefix(script, "find") {
		return tkexec.Result{Stdout: c.list}, nil
	}
	if strings.HasPrefix(script, "cat ") {
		return tkexec.Result{Stdout: c.cat}, nil
	}
	return tkexec.Result{Stdout: c.head}, nil
}
func (c *chan1) Put(context.Context, string, string) error { return nil }
func (c *chan1) Get(context.Context, string, string) error { return nil }
func (c *chan1) Describe() string                          { return "a test channel" }
func (c *chan1) Close() error                              { return nil }

func TestCrashesReportsEachReportOnce(t *testing.T) {
	ch := &chan1{list: "/cubrid/log/coredump/cub_server_20260916210148.963.coredump\n", head: report}
	seen := map[string]bool{}
	got := Crashes(context.Background(), ch, "/cubrid", seen)
	if len(got) != 1 {
		t.Fatalf("got %d crashes, want one", len(got))
	}
	if got[0].Where != "qdata_save_agg_hentry_to_list at query_aggregate.cpp:2974" {
		t.Errorf("where = %q", got[0].Where)
	}
	if s := got[0].String(); !strings.Contains(s, "cub_server_20260916210148.963.coredump") ||
		!strings.Contains(s, "cub_server ctldb") {
		t.Errorf("String() = %q, want the file and the program", s)
	}
	// The directory is not swept between cases, so the same report must not be
	// a second case's failure.
	if again := Crashes(context.Background(), ch, "/cubrid", seen); len(again) != 0 {
		t.Errorf("the same report came back: %+v", again)
	}
	if !strings.Contains(ch.ran[0], "/cubrid/log/coredump") {
		t.Errorf("looked in %q", ch.ran[0])
	}
}

// No install to look at is no crash, not a find from the root.
func TestNoInstallIsNoCrash(t *testing.T) {
	ch := &chan1{list: "/somewhere/x.coredump\n"}
	if got := Crashes(context.Background(), ch, "  ", map[string]bool{}); len(got) != 0 || len(ch.ran) != 0 {
		t.Errorf("got %+v after running %v", got, ch.ran)
	}
}

// A machine that cannot be read is not a failing case.
func TestNoDirectoryIsNoCrash(t *testing.T) {
	ch := &chan1{list: "", head: ""}
	if got := Crashes(context.Background(), ch, "/cubrid", map[string]bool{}); len(got) != 0 {
		t.Errorf("got %+v, want nothing", got)
	}
}

func TestKeepNamesTheCaseAndTheReport(t *testing.T) {
	dir := t.TempDir()
	ch := &chan1{cat: report}
	c := Crash{Path: "/cubrid/log/coredump/cub_server_20260916210148.963.coredump"}
	local, err := Keep(context.Background(), ch, c, dir, "/corpus/_01_ReadCommitted/catalog/db_index_04.ctl")
	if err != nil {
		t.Fatal(err)
	}
	if got := filepath.Base(local); got != "db_index_04.cub_server_20260916210148.963.coredump" {
		t.Errorf("kept as %q", got)
	}
	if b, err := os.ReadFile(local); err != nil || !strings.Contains(string(b), "qdata_save_agg_hentry_to_list") {
		t.Errorf("the report was not copied: %v %s", err, b)
	}
	// A report that cannot be read is an error and not an empty file.
	if _, err := Keep(context.Background(), &chan1{}, Crash{Path: "/gone"}, dir, "x.ctl"); err == nil {
		t.Error("an unreadable report should say so")
	}
}

// chan2 answers whatever the script asks for, by prefix.
type chan2 struct {
	byPrefix map[string]string
	ran      []string
}

func (c *chan2) Run(_ context.Context, script string) (tkexec.Result, error) {
	c.ran = append(c.ran, script)
	for p, out := range c.byPrefix {
		if strings.HasPrefix(script, p) {
			return tkexec.Result{Stdout: out}, nil
		}
	}
	return tkexec.Result{}, nil
}
func (c *chan2) Put(context.Context, string, string) error { return nil }
func (c *chan2) Get(context.Context, string, string) error { return nil }
func (c *chan2) Describe() string                          { return "a test channel" }
func (c *chan2) Close() error                              { return nil }

// Cores are CTP's own pattern, and each one counts against one case.
func TestCores(t *testing.T) {
	ch := &chan2{byPrefix: map[string]string{"find": "/cubrid/core.123\n/cubrid/log/core.log\n"}}
	seen := map[string]bool{}
	got := Cores(context.Background(), ch, seen, "/cubrid", "/ctltool", "")
	if len(got) != 1 || got[0] != "/cubrid/core.123" {
		t.Fatalf("got %v, want the one core", got)
	}
	if again := Cores(context.Background(), ch, seen, "/cubrid"); len(again) != 0 {
		t.Errorf("the same core came back: %v", again)
	}
	if !strings.Contains(ch.ran[0], "'/cubrid' '/ctltool'") || strings.Contains(ch.ran[0], "''") {
		t.Errorf("searched %q; an empty directory should not be searched", ch.ran[0])
	}
	if n := len(Cores(context.Background(), ch, seen)); n != 0 {
		t.Errorf("no directories should find nothing, got %d", n)
	}
}

// A log is appended to and never emptied, so what belongs to this case is what
// the count gained -- not the total, which CTP reported again for every case
// after the first.
func TestFatalsReportTheIncreaseOnly(t *testing.T) {
	ch := &chan2{byPrefix: map[string]string{"grep": "/cubrid/log/server.err:2\n/cubrid/log/other.err:0\n"}}
	counts := map[string]int{}
	got := Fatals(context.Background(), ch, "/cubrid", counts)
	if len(got) != 1 || !strings.Contains(got[0], "server.err") || !strings.Contains(got[0], "2 line(s)") {
		t.Fatalf("got %v, want server.err's two lines", got)
	}
	if again := Fatals(context.Background(), ch, "/cubrid", counts); len(again) != 0 {
		t.Errorf("the same lines came back: %v", again)
	}
	ch.byPrefix["grep"] = "/cubrid/log/server.err:5\n"
	if more := Fatals(context.Background(), ch, "/cubrid", counts); len(more) != 1 || !strings.Contains(more[0], "3 line(s)") {
		t.Errorf("got %v, want the three it gained", more)
	}
	if none := Fatals(context.Background(), ch, "", counts); len(none) != 0 {
		t.Errorf("no install is nothing to read: %v", none)
	}
}
