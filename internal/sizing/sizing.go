// Package sizing decides how many cases a run does at once, and says why.
//
// The numbers it needs -- what one slot of a suite costs in memory, and how many
// slots stop paying -- are not shipped knowledge. They are what runs on this
// machine measured, because a constant was wrong about isolation by a factor of
// two on the same machine the moment the corpus grew from a 60-case sample to the
// whole of it, and because where more slots stop paying depends on the disk and
// the processors a machine has (ADR-020).
package sizing

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// Suite is the thing being sized. Each has its own cost and its own record.
type Suite string

const (
	Isolation Suite = "isolation"
	SQL       Suite = "sql"
	Medium    Suite = "medium"
	Shell     Suite = "shell"
)

// Mode is what an operator says when they do not name a slot count. It
// multiplies the machine's own figure rather than replacing it.
type Mode string

const (
	Conservative Mode = "conservative" // something else is using the machine
	Measured     Mode = "measured"     // the machine is the run's
	Aggressive   Mode = "aggressive"   // find the ceiling rather than avoid it
)

// ModeOf reads the `parallel` key. Anything unrecognised is Measured, and says
// so: a typo should not quietly double what a machine is asked to hold.
func ModeOf(s string) (Mode, string) {
	switch m := Mode(strings.ToLower(strings.TrimSpace(s))); m {
	case "":
		return Measured, ""
	case Conservative, Measured, Aggressive:
		return m, ""
	default:
		return Measured, fmt.Sprintf("parallel=%q is not conservative, measured or aggressive; sizing as measured", s)
	}
}

// factor multiplies the memory a slot is budgeted.
func (m Mode) factor() float64 {
	switch m {
	case Conservative:
		return 2
	case Aggressive:
		return 0.8
	default:
		return 1
	}
}

// growth is how far past the most slots this machine has run a suite with the
// next run may go.
func (m Mode) growth() int {
	switch m {
	case Conservative:
		return 1
	case Aggressive:
		return 4
	default:
		return 2
	}
}

// reservedMB is left to everything else on the machine, as scripts/sizing.sh
// leaves it.
const reservedMB = 2048

// margin is what a budget adds to the largest peak a run has been seen to
// reach. The next run is not the last one: a corpus grows, and a case that
// allocates more has to fit before it is measured.
const margin = 1.15

// shipped is what a slot of a suite costs where this machine has not run the
// corpus yet. Each figure is measured, and generous on purpose: it only has to
// be safe until the machine has measured itself.
//
//	isolation  1,513 MB a slot: how far available memory fell in an eight-slot
//	           run over the whole corpus, with the shipped 512 MB data buffer
//	           and 256 MB log buffer (evidence/isolation-controller.md §8). With
//	           the margin a record gets, 1,740; 1,750 is that, rounded up.
//	sql        2.6 GB at the peak of a four-slot run: server 1.38 GB, PL 0.58,
//	           the executor's JVM 0.52, CAS 0.12 (evidence/sql-native.md §3).
//	medium     the same processes as sql.
//	shell      not measured as a whole: a slot is whatever case it drew. A
//	           suite with no figure runs one slot until it has a record.
var shipped = map[Suite]int{
	Isolation: 1750,
	SQL:       2600,
	Medium:    2600,
}

// start is where a first run on a lane begins: the slot count each suite was
// verified at, or one where parallel has not been shown to pay.
//
//	isolation  four: ADR-018's rules found no runner difference at one and four.
//	sql        four: the slot cost above was measured there.
//	medium     one: 12.6 s of cases against a 26 s slot start (category/sql §5).
//	shell      one: nothing is known about the slot until a run has measured it.
var start = map[Suite]int{
	Isolation: 4,
	SQL:       4,
	Medium:    1,
	Shell:     1,
}

// Suites are the names a suite can be asked for by.
var Suites = []Suite{Isolation, SQL, Medium, Shell}

// Engine is what the server is configured to hold. A slot is a floor plus these
// two, so a record taken with other values is adjusted rather than discarded.
type Engine struct {
	DataBufferMB int `json:"dataBufferMB"`
	LogBufferMB  int `json:"logBufferMB"`
}

func (e Engine) buffers() int { return e.DataBufferMB + e.LogBufferMB }

// With overrides the buffers with what a run's configuration writes into each
// slot's install, where it says anything: the file under $CUBRID is not what the
// slots run with when the configuration changes it.
func (e Engine) With(dataBuffer, logBuffer string) Engine {
	if mb := megabytes(dataBuffer); mb > 0 {
		e.DataBufferMB = mb
	}
	if mb := megabytes(logBuffer); mb > 0 {
		e.LogBufferMB = mb
	}
	return e
}

// Machine is what a record was taken on. A record from another machine is not
// evidence about this one, and home directories are shared more often than they
// look.
type Machine struct {
	Host       string `json:"host"`
	Cores      int    `json:"cores"`
	MemTotalMB int    `json:"memTotalMB"`
}

// ThisMachine is the machine the process is on.
func ThisMachine() Machine {
	host, _ := os.Hostname()
	return Machine{Host: host, Cores: runtime.NumCPU(), MemTotalMB: memTotalMB()}
}

func (m Machine) same(o Machine) bool {
	return m.Host == o.Host && m.Cores == o.Cores && m.MemTotalMB == o.MemTotalMB
}

// Plan is a slot count and the sentence that produced it.
type Plan struct {
	Slots int
	Why   string
}

// Input is what a decision is made from.
type Input struct {
	Suite Suite
	// Configured is `parallel_slots` as the configuration wrote it, and empty
	// when it did not. A number here wins outright: whoever wrote one has
	// decided.
	Configured string
	Mode       Mode
	Engine     Engine
	// AvailMB is the machine's available memory now; zero is unknown.
	AvailMB int
	// SharedMB is memory the run sets aside apart from its slots -- shell's
	// scenario_ram_mb, which the run's writes can fill.
	SharedMB int
	// CPUs is the processors a run may use; zero is runtime.NumCPU.
	CPUs int
	// Cases is how many cases this run has, and zero when it is decided before
	// discovery.
	Cases int
	// Corpus names the corpus -- the scenario path -- so that a run is compared
	// with runs of the same one.
	Corpus string
	// Lane is where the slots write: a record on another lane says nothing about
	// where more slots stop paying on this one.
	Lane string
	// Records are this machine's runs of the suite, newest first (Load).
	Records []Record
}

// bound is one of the things a slot count is the smallest of.
type bound struct {
	n   int
	why string
}

// Slots is the decision.
func Slots(in Input) Plan {
	if s := strings.TrimSpace(in.Configured); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 {
			return Plan{1, fmt.Sprintf("parallel_slots=%s is not a number of slots; running one", s)}
		}
		return Plan{n, fmt.Sprintf("parallel_slots=%d", n)}
	}

	budget, from := budgetFor(in)
	if budget <= 0 {
		return Plan{1, fmt.Sprintf("%s has no measured slot cost on this machine: running one, and measuring it (ADR-020)", in.Suite)}
	}
	if in.AvailMB <= 0 {
		return Plan{1, "the machine's free memory is unknown: running one"}
	}
	budget = max(int(float64(budget)*in.Mode.factor()), 1)

	var bounds []bound
	byMemory := (in.AvailMB - reservedMB - in.SharedMB) / budget
	if in.SharedMB > 0 {
		bounds = append(bounds, bound{byMemory, fmt.Sprintf("%d by memory (%d MB less %d, less %d set aside for the run's writes, at %d MB a slot, %s)",
			byMemory, in.AvailMB, reservedMB, in.SharedMB, budget, from)})
	} else {
		bounds = append(bounds, bound{byMemory, fmt.Sprintf("%d by memory (%d MB less %d, at %d MB a slot, %s)",
			byMemory, in.AvailMB, reservedMB, budget, from)})
	}

	cpus := in.CPUs
	if cpus <= 0 {
		cpus = runtime.NumCPU()
	}
	bounds = append(bounds, bound{cpus, fmt.Sprintf("%d by processors", cpus)})
	if in.Cases > 0 {
		bounds = append(bounds, bound{in.Cases, fmt.Sprintf("%d, one for each case", in.Cases)})
	}
	bounds = append(bounds, tried(in))
	if in.Mode != Aggressive {
		if b, ok := knee(in); ok {
			bounds = append(bounds, b)
		}
	}
	if b, ok := corpusBound(in); ok {
		bounds = append(bounds, b)
	}

	best := bounds[0]
	for _, b := range bounds[1:] {
		if b.n < best.n {
			best = b
		}
	}
	if best.n < 1 {
		best = bound{1, best.why + "; one is the floor"}
	}
	if in.Mode != Measured {
		best.why += fmt.Sprintf(", sized %s", in.Mode)
	}
	return Plan{best.n, best.why}
}

// sameCorpus reports whether a record ran the corpus this run is about to. A
// corpus grows and its exclusions change, so the case counts only have to be
// within a tenth of each other; a different path is a different corpus.
func sameCorpus(in Input, r Record) bool {
	if in.Corpus == "" || r.Corpus != in.Corpus {
		return false
	}
	cases := in.Cases
	if cases == 0 {
		// Decided before discovery (shell): the newest run of this path says how
		// large it is.
		for _, x := range in.Records {
			if x.Corpus == in.Corpus && x.Cases > 0 {
				cases = x.Cases
				break
			}
		}
	}
	if cases == 0 || r.Cases == 0 {
		return true
	}
	d := r.Cases - cases
	return d*10 <= cases && -d*10 <= cases
}

// peersWindow is how many of this corpus's recent runs on this lane the growth
// and the knee look at. It is short on purpose: a knee found from a run a hang
// lengthened would otherwise hold until ten runs had passed, and a machine that
// has changed -- a faster disk, fewer other jobs -- would never be asked again.
const peersWindow = 4

// peers are the recent runs of this corpus on this lane, newest first and one
// per slot count. Runs of another corpus, or on another disk, say nothing about
// where more slots stop paying here.
func peers(in Input) []Record {
	var recent []Record
	for _, r := range in.Records {
		if r.Lane != in.Lane || !sameCorpus(in, r) || r.Slots <= 0 {
			continue
		}
		recent = append(recent, r)
		if len(recent) == peersWindow {
			break
		}
	}
	// The window is over the runs, not over the slot counts: a count that has not
	// been run for four runs has aged out, whatever it measured then.
	var out []Record
	seen := map[int]bool{}
	for _, r := range recent {
		if seen[r.Slots] {
			continue
		}
		seen[r.Slots] = true
		out = append(out, r)
	}
	return out
}

// tried is how far past what this machine has already run this corpus with, on
// this lane, the next run may go. A first run starts where the suite was
// verified.
func tried(in Input) bound {
	most := 0
	for _, r := range peers(in) {
		most = max(most, r.Slots)
	}
	if most == 0 {
		n := max(start[in.Suite], 1)
		return bound{n, fmt.Sprintf("%d, where a first run of this corpus on %s starts", n, laneName(in.Lane))}
	}
	g := in.Mode.growth()
	if g == 1 {
		return bound{most, fmt.Sprintf("%d, the most this machine has run on %s", most, laneName(in.Lane))}
	}
	return bound{most * g, fmt.Sprintf("%d, %d times the %d this machine has run on %s", most*g, g, most, laneName(in.Lane))}
}

// knee is the slot count past which this machine measured no gain: a recent run
// of the same corpus on the same lane used more slots and was not faster. Five
// per cent is the noise a run's wall time has been seen to carry with nothing
// changed.
func knee(in Input) (bound, bool) {
	var runs []Record
	for _, r := range peers(in) {
		if r.WallS > 0 {
			runs = append(runs, r)
		}
	}
	if len(runs) < 2 {
		return bound{}, false
	}
	fastest := runs[0]
	for _, r := range runs[1:] {
		if r.WallS < fastest.WallS {
			fastest = r
		}
	}
	// The fewest slots that came within the noise of the fastest.
	best := fastest
	for _, r := range runs {
		if float64(r.WallS) <= float64(fastest.WallS)*1.05 && r.Slots < best.Slots {
			best = r
		}
	}
	var over *Record
	for i, r := range runs {
		if r.Slots > best.Slots && (over == nil || r.WallS < over.WallS) {
			over = &runs[i]
		}
	}
	if over == nil {
		return bound{}, false
	}
	return bound{best.Slots, fmt.Sprintf("%d, the fewest slots within 5%% of the fastest run of this corpus on %s: "+
		"%d took %d s, %d took %d s", best.Slots, laneName(in.Lane), best.Slots, best.WallS, over.Slots, over.WallS)}, true
}

// corpusBound is the slot count past which the longest unit, and not the slots,
// decides the wall time. A run of this corpus says it directly; another corpus's
// case seconds are scaled by the case count, which is the most that can be said
// about a corpus this machine has not run. The longest unit is the longest that
// passed on its first attempt: one that timed out and was retried says how long
// a hang lasts, not how long the corpus is.
func corpusBound(in Input) (bound, bool) {
	var other *Record
	for i, r := range in.Records {
		if r.CaseS <= 0 || r.LongestUnitS <= 0 {
			continue
		}
		if sameCorpus(in, r) {
			n := r.CaseS / r.LongestUnitS
			return bound{n, fmt.Sprintf("%d by the corpus (%d s of cases, the longest unit %d s: more slots stop helping)",
				n, r.CaseS, r.LongestUnitS)}, true
		}
		if other == nil && in.Cases > 0 && r.Cases > 0 {
			other = &in.Records[i]
		}
	}
	if other == nil {
		return bound{}, false
	}
	caseS := int(int64(other.CaseS) * int64(in.Cases) / int64(other.Cases))
	n := caseS / other.LongestUnitS
	return bound{n, fmt.Sprintf("%d by the corpus (about %d s of cases, scaled from another corpus's %d, "+
		"the longest unit %d s)", n, caseS, other.Cases, other.LongestUnitS)}, true
}

// budgetFor is what one slot is allowed, and where the figure came from.
//
// Only a record of a corpus at least as large as this run's is evidence about it:
// a slot's peak is the heaviest case it drew, and a sample has fewer heavy cases
// to draw -- 58 isolation cases measured 623 MB a slot where the whole corpus
// measured 1,422. Where no record covers the run, the suite's shipped figure is
// the floor; a suite that has none runs one slot rather than guess.
func budgetFor(in Input) (int, string) {
	covering, any := 0, 0
	for _, r := range in.Records {
		mb := adjusted(r, in.Engine)
		if mb <= 0 {
			continue
		}
		any = max(any, mb)
		if sameCorpus(in, r) || (in.Cases > 0 && r.Cases >= in.Cases) {
			covering = max(covering, mb)
		}
	}
	if covering > 0 {
		return withMargin(covering), "this machine's own runs of it"
	}
	ship := shipped[in.Suite]
	if ship == 0 {
		return 0, ""
	}
	if withMargin(any) > ship {
		return withMargin(any), "this machine's runs of a smaller corpus, which is more than the shipped figure"
	}
	return ship, "the shipped figure, until this machine has run this corpus"
}

func withMargin(mb int) int { return int(math.Round(float64(mb) * margin)) }

// adjusted is a record's cost of a slot at this run's buffers: a slot is a floor
// plus the two buffers, so a record taken with other sizes is still evidence
// about the floor. When either side's buffers are unknown it is taken as it is.
func adjusted(r Record, eng Engine) int {
	if r.PerSlotMB <= 0 {
		return 0
	}
	if r.Engine.buffers() > 0 && eng.buffers() > 0 {
		return r.PerSlotMB - r.Engine.buffers() + eng.buffers()
	}
	return r.PerSlotMB
}

func laneName(lane string) string {
	if lane == "" {
		return "this lane"
	}
	return lane
}

// dir is where records live: outside the repository, because the measurement is
// a fact about the machine and not about the checkout (ADR-020).
func dir() string {
	if d := os.Getenv("TESTKIT_SIZING_DIR"); d != "" {
		return d
	}
	if d := os.Getenv("XDG_STATE_HOME"); d != "" {
		return filepath.Join(d, "testkit", "sizing")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "state", "testkit", "sizing")
}

// memTotalMB and MemAvailableMB read /proc/meminfo. Zero means unknown, and an
// unknown bounds nothing.
func memTotalMB() int     { return meminfo("MemTotal:") }
func MemAvailableMB() int { return meminfo("MemAvailable:") }

func meminfo(key string) int {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, key) {
			f := strings.Fields(line)
			if len(f) >= 2 {
				if kb, err := strconv.Atoi(f[1]); err == nil {
					return kb / 1024
				}
			}
		}
	}
	return 0
}

// EngineFromConf reads the two buffer sizes out of an install's cubrid.conf.
// Anything it cannot read is zero, which no adjustment uses.
func EngineFromConf(cubridDir string) Engine {
	var e Engine
	if cubridDir == "" {
		return e
	}
	b, err := os.ReadFile(filepath.Join(cubridDir, "conf", "cubrid.conf"))
	if err != nil {
		return e
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch strings.TrimSpace(k) {
		case "data_buffer_size":
			e.DataBufferMB = megabytes(v)
		case "log_buffer_size":
			e.LogBufferMB = megabytes(v)
		}
	}
	return e
}

// megabytes reads what cubrid.conf writes: a number with an optional unit,
// where a bare number is bytes.
func megabytes(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	mult := 1
	switch last := s[len(s)-1]; last {
	case 'K', 'k':
		mult, s = 1024, s[:len(s)-1]
	case 'M', 'm':
		mult, s = 1024*1024, s[:len(s)-1]
	case 'G', 'g':
		mult, s = 1024*1024*1024, s[:len(s)-1]
	}
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n * mult / (1024 * 1024)
}
