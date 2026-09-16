package sizing

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// in is an Input with the processors pinned, so that no test depends on the
// machine it runs on.
func in(suite Suite, availMB int, recs ...Record) Input {
	return Input{Suite: suite, Mode: Measured, Engine: Engine{512, 256}, AvailMB: availMB, CPUs: 64,
		Corpus: "/c", Lane: "disk", Records: recs}
}

// A number in the configuration wins, whatever the machine: whoever wrote one
// has decided.
func TestAConfiguredCountWins(t *testing.T) {
	x := in(Isolation, 4000)
	x.Configured = "12"
	if p := Slots(x); p.Slots != 12 {
		t.Errorf("got %d slots, want 12 (%s)", p.Slots, p.Why)
	}
	x.Configured = "nonsense"
	if p := Slots(x); p.Slots != 1 || !strings.Contains(p.Why, "not a number") {
		t.Errorf("got %d slots, %q; want one and a reason", p.Slots, p.Why)
	}
}

// A first run starts where the suite was verified, and memory can hold it lower.
func TestAFirstRunStartsWhereTheSuiteWasVerified(t *testing.T) {
	if p := Slots(in(Isolation, 64000)); p.Slots != 4 || !strings.Contains(p.Why, "first run") {
		t.Errorf("isolation: got %d slots, %q; want the four it was verified at", p.Slots, p.Why)
	}
	if p := Slots(in(Medium, 64000)); p.Slots != 1 {
		t.Errorf("medium: got %d slots, %q; want one", p.Slots, p.Why)
	}
	p := Slots(in(Isolation, 2048+shipped[Isolation]*3))
	if p.Slots != 3 || !strings.Contains(p.Why, "shipped") {
		t.Errorf("got %d slots, %q; want three by the shipped figure", p.Slots, p.Why)
	}
}

// A suite with no figure and no record runs one, and is measured doing it.
func TestAnUnmeasuredSuiteRunsOne(t *testing.T) {
	if p := Slots(in(Shell, 64000)); p.Slots != 1 || !strings.Contains(p.Why, "no measured slot cost") {
		t.Errorf("got %d slots, %q", p.Slots, p.Why)
	}
}

// Each run may go twice past the most slots this machine has run on the lane,
// and only as far as memory, processors and the knee allow.
func TestTheCountGrowsFromWhatWasRun(t *testing.T) {
	shell := Record{Corpus: "/c", Lane: "disk", Slots: 1, Cases: 100, WallS: 1000, PerSlotMB: 400}
	if p := Slots(in(Shell, 64000, shell)); p.Slots != 2 || !strings.Contains(p.Why, "2 times the 1") {
		t.Errorf("after one slot: got %d, %q; want two", p.Slots, p.Why)
	}
	other := shell
	other.Lane = "memory"
	if p := Slots(in(Shell, 64000, other)); p.Slots != 1 {
		t.Errorf("a record on another lane: got %d, %q; want a first run's one", p.Slots, p.Why)
	}
	x := in(Shell, 64000, shell)
	x.Mode = Conservative
	if p := Slots(x); p.Slots != 1 {
		t.Errorf("conservative does not grow: got %d, %q", p.Slots, p.Why)
	}
	x.Mode = Aggressive
	if p := Slots(x); p.Slots != 4 {
		t.Errorf("aggressive grows four times: got %d, %q", p.Slots, p.Why)
	}
}

// More slots that were not faster are where the count stops -- shell on a disk
// that eight slots made slower than four, medium that four did not speed up.
func TestTheKnee(t *testing.T) {
	four := Record{Corpus: "/c", Lane: "disk", Slots: 4, Cases: 100, WallS: 1000, PerSlotMB: 400}
	eight := Record{Corpus: "/c", Lane: "disk", Slots: 8, Cases: 100, WallS: 1200, PerSlotMB: 400}
	p := Slots(in(Shell, 64000, eight, four))
	if p.Slots != 4 || !strings.Contains(p.Why, "fewest slots within 5%") {
		t.Errorf("got %d slots, %q; want four, where it ran fastest", p.Slots, p.Why)
	}
	// Within the noise, the fewer slots are the knee.
	eight.WallS = 980
	if p := Slots(in(Shell, 64000, eight, four)); p.Slots != 4 {
		t.Errorf("eight within 5%% of four: got %d, %q; want four", p.Slots, p.Why)
	}
	// Clearly faster, and the count grows past it.
	eight.WallS = 600
	if p := Slots(in(Shell, 64000, eight, four)); p.Slots != 16 {
		t.Errorf("eight clearly faster: got %d, %q; want sixteen", p.Slots, p.Why)
	}
	// Aggressive looks past the knee.
	eight.WallS = 1200
	x := in(Shell, 64000, eight, four)
	x.Mode = Aggressive
	if p := Slots(x); p.Slots != 32 {
		t.Errorf("aggressive: got %d, %q; want thirty-two", p.Slots, p.Why)
	}
	// Another corpus says nothing about this one -- not its knee, not how many
	// slots have been run, and for a suite with no shipped figure not even what a
	// slot costs. Shell runs one and measures it.
	x = in(Shell, 64000, eight, four)
	x.Corpus = "/other"
	if p := Slots(x); p.Slots != 1 || !strings.Contains(p.Why, "no measured slot cost") {
		t.Errorf("another corpus: got %d, %q; want one", p.Slots, p.Why)
	}
	// A corpus that grew by a few cases is the same corpus.
	grown := in(Shell, 64000, eight, four)
	grown.Cases = 104
	if p := Slots(grown); p.Slots != 4 {
		t.Errorf("a corpus grown 4%%: got %d, %q; want the knee's four", p.Slots, p.Why)
	}
	grown.Cases = 400
	if p := Slots(grown); p.Slots != 1 {
		t.Errorf("a corpus four times the size: got %d, %q; want one", p.Slots, p.Why)
	}
}

// Once the machine has measured itself, its own figure is the budget -- with a
// margin, because the next run is not the last one.
func TestTheMachinesOwnFigureWins(t *testing.T) {
	rec := Record{Corpus: "/c", Lane: "disk", Slots: 8, Cases: 100, PerSlotMB: 1000, Engine: Engine{512, 256}}
	p := Slots(in(Isolation, 2048+1150*4, rec))
	if p.Slots != 4 || !strings.Contains(p.Why, "this machine's own runs") {
		t.Errorf("got %d slots, %q; want 4 at 1,150 MB a slot", p.Slots, p.Why)
	}
}

// A sample's slots are lighter than the whole corpus's, so a record of a smaller
// corpus does not lower the budget below the shipped figure (58 cases measured
// 623 MB a slot, against 1,422 over 6,772).
func TestASmallerCorpusDoesNotLowerTheBudget(t *testing.T) {
	sample := Record{Corpus: "/sample", Lane: "disk", Slots: 12, Cases: 58, PerSlotMB: 623, Engine: Engine{512, 256}}
	x := in(Isolation, 2048+1750*6, sample)
	x.Cases = 6772
	if mb, from := budgetFor(x); mb != shipped[Isolation] || !strings.Contains(from, "shipped") {
		t.Errorf("budget %d (%s), want the shipped 1,750", mb, from)
	}
	// And the sample's twelve slots are not what this corpus may grow from.
	if p := Slots(x); p.Slots != start[Isolation] || !strings.Contains(p.Why, "first run of this corpus") {
		t.Errorf("got %d slots, %q; want a first run of this corpus", p.Slots, p.Why)
	}
	heavy := Record{Corpus: "/sample", Lane: "disk", Slots: 12, Cases: 58, PerSlotMB: 3000, Engine: Engine{512, 256}}
	x.Records = []Record{heavy}
	if mb, from := budgetFor(x); mb != withMargin(3000) || !strings.Contains(from, "smaller corpus") {
		t.Errorf("a heavier small corpus still counts: %d (%s)", mb, from)
	}
}

// A suite with no shipped figure is not budgeted from another corpus at all:
// shell's slot is whatever case it drew, and another corpus drew other cases.
func TestShellIsNotBudgetedFromAnotherCorpus(t *testing.T) {
	other := Record{Corpus: "/other", Lane: "disk", Slots: 8, Cases: 300, WallS: 900, PerSlotMB: 300}
	if p := Slots(in(Shell, 64000, other)); p.Slots != 1 || !strings.Contains(p.Why, "no measured slot cost") {
		t.Errorf("got %d slots, %q; want one", p.Slots, p.Why)
	}
}

// The knee looks at the last few runs only, so one that a hang lengthened stops
// holding the count down once the machine has run past it.
func TestAStaleKneeExpires(t *testing.T) {
	c := func(when string, slots, wall int) Record {
		return Record{When: when, Corpus: "/c", Lane: "disk", Slots: slots, Cases: 100, WallS: wall, PerSlotMB: 400}
	}
	// Newest first: four runs at four slots, and the eight-slot run that looked
	// slower is older than the window.
	recs := []Record{c("8", 4, 1000), c("7", 4, 1010), c("6", 4, 1000), c("5", 4, 1020), c("1", 8, 1200)}
	if p := Slots(in(Shell, 64000, recs...)); p.Slots != 8 {
		t.Errorf("got %d slots, %q; want eight -- the old knee has aged out", p.Slots, p.Why)
	}
	// One run per slot count, so the same eight-slot run inside the window still
	// holds it at four.
	recs = []Record{c("8", 4, 1000), c("7", 4, 1010), c("6", 8, 1200)}
	if p := Slots(in(Shell, 64000, recs...)); p.Slots != 4 {
		t.Errorf("got %d slots, %q; want four", p.Slots, p.Why)
	}
}

// A record taken with other buffer sizes is evidence about the floor, so it is
// adjusted rather than thrown away -- and compared after the adjustment.
func TestBuffersAdjustAndAreComparedAfter(t *testing.T) {
	small := Record{Corpus: "/c", Lane: "disk", Slots: 8, Cases: 100, PerSlotMB: 1400, Engine: Engine{512, 256}}
	big := Record{Corpus: "/c", Lane: "disk", Slots: 8, Cases: 100, PerSlotMB: 1500, Engine: Engine{1024, 256}}
	// At 768 MB of buffers: small is 1,400, big is 1,500 - 512 = 988.
	if mb, _ := budgetFor(in(Isolation, 64000, big, small)); mb != withMargin(1400) {
		t.Errorf("budget %d, want the adjusted larger, %d", mb, withMargin(1400))
	}
	// Unknown buffers on this side adjust nothing.
	x := in(Isolation, 64000, small)
	x.Engine = Engine{}
	if mb, _ := budgetFor(x); mb != withMargin(1400) {
		t.Errorf("unknown buffers: budget %d, want %d", mb, withMargin(1400))
	}
}

// A tiny budget in aggressive mode does not divide by zero, and a record the
// adjustment drives below zero is not used.
func TestNoDivisionByZero(t *testing.T) {
	tiny := Record{Corpus: "/c", Lane: "disk", Slots: 1, Cases: 100, PerSlotMB: 1}
	x := in(Isolation, 64000, tiny)
	x.Mode = Aggressive
	_ = Slots(x)
	neg := Record{Corpus: "/c", Lane: "disk", Slots: 1, Cases: 100, PerSlotMB: 100, Engine: Engine{512, 256}}
	x = in(Isolation, 64000, neg)
	x.Engine = Engine{16, 16}
	if mb, from := budgetFor(x); mb != shipped[Isolation] {
		t.Errorf("a record adjusted below zero: budget %d (%s), want the shipped figure", mb, from)
	}
}

// What the run sets aside for its writes in memory is not memory for slots.
func TestSharedMemoryIsSetAside(t *testing.T) {
	rec := Record{Corpus: "/c", Lane: "memory", Slots: 8, Cases: 100, WallS: 100, PerSlotMB: 870}
	x := in(Shell, 2048+10000+1000*4, rec)
	x.Lane = "memory"
	x.SharedMB = 10000
	if p := Slots(x); p.Slots != 4 || !strings.Contains(p.Why, "set aside") {
		t.Errorf("got %d slots, %q; want four after 10,000 MB set aside", p.Slots, p.Why)
	}
}

func TestModeOf(t *testing.T) {
	for s, want := range map[string]Mode{
		"": Measured, "measured": Measured, "  Conservative ": Conservative, "AGGRESSIVE": Aggressive,
	} {
		if got, warn := ModeOf(s); got != want || warn != "" {
			t.Errorf("ModeOf(%q) = %v, %q; want %v and no warning", s, got, warn, want)
		}
	}
	got, warn := ModeOf("fast")
	if got != Measured || warn == "" {
		t.Errorf("an unrecognised mode should be measured and say so: %v, %q", got, warn)
	}
}

// The corpus bounds the count as well, scaled to the run.
func TestTheCorpusBoundsTheCount(t *testing.T) {
	rec := Record{Corpus: "/c", Lane: "disk", Slots: 8, Cases: 1000, CaseS: 1000, LongestUnitS: 250, PerSlotMB: 100}
	x := in(Isolation, 100000, rec)
	if p := Slots(x); p.Slots != 4 || !strings.Contains(p.Why, "by the corpus") {
		t.Errorf("got %d slots, %q; want four bounded by the corpus", p.Slots, p.Why)
	}
	x.Corpus, x.Cases = "/part", 500
	if b, ok := corpusBound(x); !ok || b.n != 2 {
		t.Errorf("half the corpus: got %+v, want two", b)
	}
	x.Cases = 0
	if _, ok := corpusBound(x); ok {
		t.Error("another corpus of unknown size should not be bounded by this one")
	}
}

// Never more slots than cases.
func TestNoMoreSlotsThanCases(t *testing.T) {
	x := in(Isolation, 100000)
	x.Cases = 1
	if p := Slots(x); p.Slots != 1 || !strings.Contains(p.Why, "one for each case") {
		t.Errorf("got %d slots, %q; want one for one case", p.Slots, p.Why)
	}
}

// The meter's memory is how far available memory fell, less what was the run's
// writes in memory at that moment; its longest unit ignores a unit with a retried
// case, which measures the timeout, not the corpus.
func TestTheMeter(t *testing.T) {
	avail := []int{20000, 15000, 9000, 12000}
	i := 0
	m := startMeter(time.Hour, func() int {
		a := avail[min(i, len(avail)-1)]
		i++
		return a
	}, func() int { return 1000 * i })
	m.sample()
	m.sample()
	m.Case("a/1", "a", 40*time.Second, true)
	m.Case("a/2", "a", 26*time.Second, true)
	m.Case("b/1", "b", 1500*time.Second, false)
	m.Case("b/1", "b", 200*time.Millisecond, true)
	m.Case("c/1", "c", 200*time.Millisecond, true)
	r := m.Stop()
	if r.PeakMB != 11000 || r.SharedMB != 3000 {
		t.Errorf("peak %d MB with %d shared, want 11000 and the 3000 of that moment", r.PeakMB, r.SharedMB)
	}
	if r.LongestUnitS != 66 {
		t.Errorf("longest: got %d s, want unit a's 66 -- unit b was retried", r.LongestUnitS)
	}
	if r.Cases != 4 || r.CaseS != 1566 {
		t.Errorf("cases: got %d and %d s, want 4 and 1566 s", r.Cases, r.CaseS)
	}
}

// A record is written where git cannot see it, is read back by the machine that
// wrote it, and is ignored on any other.
func TestARecordRoundTrips(t *testing.T) {
	d := t.TempDir()
	t.Setenv("TESTKIT_SIZING_DIR", d)
	if err := Save(Isolation, Record{Slots: 8, PeakMB: 12000, SharedMB: 400, CaseS: 9561, LongestUnitS: 66,
		Engine: Engine{512, 256}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(d, "isolation.json")); err != nil {
		t.Fatalf("the record is not where Path says: %v", err)
	}
	got := Load(Isolation)
	if len(got) != 1 {
		t.Fatalf("the machine that wrote a record should read it back: %+v", got)
	}
	if got[0].PerSlotMB != (12000-400)/8 {
		t.Errorf("per slot: got %d, want %d", got[0].PerSlotMB, (12000-400)/8)
	}
	if left, _ := filepath.Glob(filepath.Join(d, ".isolation.*")); len(left) != 0 {
		t.Errorf("the temporary file was left behind: %v", left)
	}
}

func TestARecordFromAnotherMachineIsIgnored(t *testing.T) {
	d := t.TempDir()
	t.Setenv("TESTKIT_SIZING_DIR", d)
	other := ThisMachine()
	other.Host += "-somewhere-else"
	if err := Save(Isolation, Record{Machine: other, Slots: 8, PeakMB: 11380}); err != nil {
		t.Fatal(err)
	}
	if got := Load(Isolation); len(got) != 0 {
		t.Errorf("a record from %s should not size this machine: %+v", other.Host, got)
	}
}

// The file keeps the last few runs of each machine: another machine sharing the
// home directory does not push this one's out.
func TestTrimmedPerMachine(t *testing.T) {
	d := t.TempDir()
	t.Setenv("TESTKIT_SIZING_DIR", d)
	if err := Save(Isolation, Record{When: "0", Slots: 4, PeakMB: 4000}); err != nil {
		t.Fatal(err)
	}
	other := ThisMachine()
	other.Host += "-elsewhere"
	for i := 0; i < keep+3; i++ {
		if err := Save(Isolation, Record{Machine: other, When: "1", Slots: 8, PeakMB: 8000}); err != nil {
			t.Fatal(err)
		}
	}
	if got := Load(Isolation); len(got) != 1 {
		t.Errorf("this machine's run was pushed out by another's: %+v", got)
	}
	if n := len(read(Isolation)); n != keep+1 {
		t.Errorf("the file holds %d runs, want %d of the other machine's and this one", n, keep+1)
	}
}

func TestMemAvailableIsReadable(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("meminfo is Linux's")
	}
	if MemAvailableMB() <= 0 {
		t.Error("this machine should report some available memory")
	}
}
