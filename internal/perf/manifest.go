// Package perf is the weekly performance-regression runner's own vocabulary:
// the case and fixture manifests, the branch registrations and the session
// configuration -- read once, and refused by name before any cluster exists.
//
// It is a new entry point beside the frozen task names (ADR-EXT-011), routed
// before containment: validate and list read files and start nothing. The
// field tables are the Spec's (cubrid_cv plan/perf_regression, §7.2, §7.4,
// §7.5, §7.5.1); this package is the one parser for them, and the session uses
// the same one, which is what makes "validate said yes" mean "the session will
// not stop on this file".
package perf

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Case is one case.json: a directory under cases/<module>/<name>/ and the
// program it runs. Every field but Notes is required; the manifest is a closed
// schema, so a key the table does not have is refused by name rather than
// carried as a silent typo.
type Case struct {
	ID         string            `json:"id"`
	Version    int               `json:"version"`
	Owner      string            `json:"owner"`
	Grade      string            `json:"grade"`
	Module     string            `json:"module"`
	Topology   string            `json:"topology"`
	Conf       map[string]string `json:"conf"`
	Fixture    FixtureRef        `json:"fixture"`
	Driver     string            `json:"driver"`
	Client     json.RawMessage   `json:"client"`
	Op         string            `json:"op"`
	Metric     string            `json:"metric"`
	Cold       bool              `json:"cold"`
	WarmS      int               `json:"warm_s"`
	Warmup     int               `json:"warmup"`
	Repeats    int               `json:"repeats"`
	BudgetS    int               `json:"budget_s"`
	Tolerance  float64           `json:"tolerance"`
	Counters   []string          `json:"counters"`
	Background string            `json:"background"`
	Notes      string            `json:"notes,omitempty"`

	// Dir is where the manifest was read from; the id has to agree with it.
	Dir string `json:"-"`
	// Exactly one of these is set, by Driver.
	JDBC    *JDBCClient    `json:"-"`
	Utility *UtilityClient `json:"-"`
	CDC     *CDCClient     `json:"-"`
}

// FixtureRef names the database state a case assumes, by name and version, so
// a fixture that changed under a case is caught at validate rather than read
// as a regression.
type FixtureRef struct {
	Name    string `json:"name"`
	Version int    `json:"version"`
}

// JDBCClient runs in the client container: java <main> <args...> against the
// broker.
type JDBCClient struct {
	Main string   `json:"main"`
	Args []string `json:"args"`
}

// UtilityClient runs inside the database node: a wrapper times argv and
// reports ops as the number of units one run means.
type UtilityClient struct {
	Argv []string `json:"argv"`
	Ops  int      `json:"ops"`
}

// CDCClient runs in the client container: a program linked to libcubridcs,
// connected to the server directly.
type CDCClient struct {
	Bin  string   `json:"bin"`
	Args []string `json:"args"`
}

// MaxPassS is the longest a case can take on one pair when every pass runs to
// its budget: warmup and measured passes, on both builds. The session compares
// it with what is left of the weekend before starting the case (Design §5.9).
func (c *Case) MaxPassS() int { return (c.Warmup + c.Repeats) * 2 * c.BudgetS }

// Fixture is one fixture.json: a database the cases assume, built once per
// build at session start and reset before every pass.
type Fixture struct {
	Name      string `json:"name"`
	Version   int    `json:"version"`
	Rows      int    `json:"rows"`
	SchemaSQL string `json:"schema_sql"`
	Load      string `json:"load"`
	Reset     string `json:"reset"`

	Dir string `json:"-"`
}

// The closed vocabularies. A value outside one is a refusal that names it.
var (
	Grades      = []string{"A", "B", "C"}
	Modules     = []string{"sql", "lib", "storage", "txn", "cdc"}
	Topologies  = []string{"single"}
	Drivers     = []string{"jdbc", "utility", "cdc-api"}
	Ops         = []string{"request", "row", "commit", "item", "connect", "run"}
	Metrics     = []string{"ops_per_s", "latency_s", "elapsed_s"}
	Backgrounds = []string{"keep", "defer"}
	Resets      = []string{"none", "truncate_and_reload", "restore_snapshot"}
)

// ResetRestoreSnapshot restarts the server, so a case that uses it has to warm
// its working set before measuring -- warm_s = 0 is refused for it.
const ResetRestoreSnapshot = "restore_snapshot"

// The closed counter list is the collect layer's (notes/collect_layer.md):
// statdump counters by their own name, the disk and network counters, and
// the per-process L0 counters qualified by the role they are read from. A name
// outside it used to produce a column of nulls in sandbox scenarios; here it
// is refused before anything runs.
var (
	statdumpRe   = regexp.MustCompile(`^Num_[A-Za-z0-9_]+$`)
	roleCounter  = regexp.MustCompile(`^(server|broker|cas|client)\.([a-z_]+)$`)
	bareCounters = map[string]bool{
		"dev_reads": true, "dev_writes": true, "dev_flushes": true, "dev_busy": true,
		"net_packets": true, "net_bytes": true,
		"syscalls_by_type": true, "instructions": true, "cycles": true, "llc_misses": true,
	}
	roleCounters = map[string]bool{
		"cpu_user": true, "cpu_sys": true, "ctxsw_vol": true, "ctxsw_invol": true, "runq_wait": true,
		"rw_syscalls": true, "io_read_bytes": true, "io_write_bytes": true, "page_faults": true, "rss_peak": true,
	}
)

// KnownCounter says whether a counters[] entry is on the closed list.
func KnownCounter(name string) bool {
	if bareCounters[name] || statdumpRe.MatchString(name) {
		return true
	}
	if m := roleCounter.FindStringSubmatch(name); m != nil {
		return roleCounters[m[2]]
	}
	return false
}

// Problems is every refusal one file produced, so a reader fixes a manifest
// in one round rather than one key per run.
type Problems struct {
	Path string
	List []string
}

func (p *Problems) Error() string {
	return p.Path + ": " + strings.Join(p.List, "; ")
}

func (p *Problems) add(format string, args ...any) {
	p.List = append(p.List, fmt.Sprintf(format, args...))
}

func (p *Problems) err() error {
	if len(p.List) == 0 {
		return nil
	}
	return p
}

var caseKeys = []string{"id", "version", "owner", "grade", "module", "topology", "conf", "fixture", "driver",
	"client", "op", "metric", "cold", "warm_s", "warmup", "repeats", "budget_s", "tolerance", "counters",
	"background"}

// ReadCase reads cases/<module>/<name>/case.json and applies every rule the
// Spec lists for validate. The fixture it names is looked up in the suite the
// directory sits in, because the one rule that spans both files (a restored
// snapshot needs warming) cannot be checked from either alone.
func ReadCase(dir string) (*Case, error) {
	path := filepath.Join(dir, "case.json")
	p := &Problems{Path: path}
	raw, err := os.ReadFile(path)
	if err != nil {
		p.add("%v", err)
		return nil, p
	}
	if _, ok := presence(p, raw, caseKeys, []string{"notes"}); !ok {
		return nil, p
	}
	var c Case
	if err := strict(raw, &c); err != nil {
		p.add("%v", err)
		return nil, p
	}
	c.Dir = dir

	module, name := filepath.Base(filepath.Dir(dir)), filepath.Base(dir)
	if want := module + "." + name; c.ID != want {
		p.add("id %q does not match the directory, which says %q", c.ID, want)
	}
	if c.Module != module {
		p.add("module %q does not match the directory, which says %q", c.Module, module)
	}
	if c.Version < 1 {
		p.add("version must be 1 or more, got %d", c.Version)
	}
	if c.Owner == "" {
		p.add("owner is empty; the flag has nobody to go to")
	}
	oneOf(p, "grade", c.Grade, Grades)
	oneOf(p, "module", c.Module, Modules)
	oneOf(p, "topology", c.Topology, Topologies)
	if c.Fixture.Name == "" || c.Fixture.Version < 1 {
		p.add("fixture needs a name and a version of 1 or more, got %+v", c.Fixture)
	}
	oneOf(p, "driver", c.Driver, Drivers)
	readClient(p, &c)
	oneOf(p, "op", c.Op, Ops)
	oneOf(p, "metric", c.Metric, Metrics)
	if c.WarmS < 0 {
		p.add("warm_s must be 0 or more, got %d", c.WarmS)
	}
	if c.Warmup < 1 {
		p.add("warmup must be 1 or more, got %d: the first pass is always discarded", c.Warmup)
	}
	if c.Repeats < 3 {
		p.add("repeats must be 3 or more, got %d: a median of fewer says nothing", c.Repeats)
	}
	if c.BudgetS < 1 {
		p.add("budget_s must be 1 or more, got %d", c.BudgetS)
	}
	if !(c.Tolerance > 0) {
		p.add("tolerance must be above 0, got %v", c.Tolerance)
	}
	for _, name := range c.Counters {
		if !KnownCounter(name) {
			p.add("counter %q is not on the collect layer's list", name)
		}
	}
	oneOf(p, "background", c.Background, Backgrounds)

	// The fixture's rules that need the case: version agreement, and warming
	// after a restored snapshot.
	if c.Fixture.Name != "" {
		root := filepath.Dir(filepath.Dir(filepath.Dir(dir)))
		fdir := filepath.Join(root, "fixtures", c.Fixture.Name)
		if _, err := os.Stat(filepath.Join(fdir, "fixture.json")); err != nil {
			p.add("fixture %q is not in %s", c.Fixture.Name, filepath.Join(root, "fixtures"))
		} else if f, err := ReadFixture(fdir); err == nil {
			checkFixtureUse(p, &c, f)
		}
	}
	return &c, p.err()
}

func checkFixtureUse(p *Problems, c *Case, f *Fixture) {
	if c.Fixture.Version != f.Version {
		p.add("fixture %s is version %d in the suite and the case assumes %d", f.Name, f.Version, c.Fixture.Version)
	}
	if f.Reset == ResetRestoreSnapshot && c.WarmS == 0 {
		p.add("fixture %s restores a snapshot, which restarts the server; warm_s must be above 0", f.Name)
	}
}

func readClient(p *Problems, c *Case) {
	if len(c.Client) == 0 {
		return
	}
	switch c.Driver {
	case "jdbc":
		if _, ok := presence(p, c.Client, []string{"main", "args"}, nil); !ok {
			return
		}
		var j JDBCClient
		if err := strict(c.Client, &j); err != nil {
			p.add("client: %v", err)
			return
		}
		if j.Main == "" {
			p.add("client.main is empty")
		}
		c.JDBC = &j
	case "utility":
		if _, ok := presence(p, c.Client, []string{"argv", "ops"}, nil); !ok {
			return
		}
		var u UtilityClient
		if err := strict(c.Client, &u); err != nil {
			p.add("client: %v", err)
			return
		}
		if len(u.Argv) == 0 {
			p.add("client.argv is empty")
		}
		if u.Ops < 1 {
			p.add("client.ops must be 1 or more, got %d", u.Ops)
		}
		c.Utility = &u
	case "cdc-api":
		if _, ok := presence(p, c.Client, []string{"bin", "args"}, nil); !ok {
			return
		}
		var d CDCClient
		if err := strict(c.Client, &d); err != nil {
			p.add("client: %v", err)
			return
		}
		if d.Bin == "" {
			p.add("client.bin is empty")
		}
		c.CDC = &d
	}
}

var fixtureKeys = []string{"name", "version", "rows", "schema_sql", "load", "reset"}

// ReadFixture reads fixtures/<name>/fixture.json. The files it names have to
// be beside it: a load script that is not there fails at session start, after
// two clusters were built for it.
func ReadFixture(dir string) (*Fixture, error) {
	path := filepath.Join(dir, "fixture.json")
	p := &Problems{Path: path}
	raw, err := os.ReadFile(path)
	if err != nil {
		p.add("%v", err)
		return nil, p
	}
	if _, ok := presence(p, raw, fixtureKeys, nil); !ok {
		return nil, p
	}
	var f Fixture
	if err := strict(raw, &f); err != nil {
		p.add("%v", err)
		return nil, p
	}
	f.Dir = dir
	if want := filepath.Base(dir); f.Name != want {
		p.add("name %q does not match the directory, which says %q", f.Name, want)
	}
	if f.Version < 1 {
		p.add("version must be 1 or more, got %d", f.Version)
	}
	if f.Rows < 1 {
		p.add("rows must be 1 or more, got %d", f.Rows)
	}
	for _, kv := range []struct{ key, file string }{{"schema_sql", f.SchemaSQL}, {"load", f.Load}} {
		if kv.file == "" {
			p.add("%s is empty", kv.key)
		} else if _, err := os.Stat(filepath.Join(dir, kv.file)); err != nil {
			p.add("%s names %q, which is not beside the manifest", kv.key, kv.file)
		}
	}
	oneOf(p, "reset", f.Reset, Resets)
	return &f, p.err()
}

// Suite is benchmarks/regression/ read whole: every case and every fixture,
// each checked on its own and against the other.
type Suite struct {
	Root     string
	Cases    []*Case // by id
	Fixtures map[string]*Fixture
}

// LoadSuite reads every manifest under root and returns every problem found,
// joined, so one validate run names them all.
func LoadSuite(root string) (*Suite, error) {
	s := &Suite{Root: root, Fixtures: map[string]*Fixture{}}
	var errs []error
	if st, err := os.Stat(filepath.Join(root, "cases")); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("%s: no cases/ directory; is this benchmarks/regression?", root)
	}
	fixtures, _ := filepath.Glob(filepath.Join(root, "fixtures", "*", "fixture.json"))
	for _, m := range fixtures {
		f, err := ReadFixture(filepath.Dir(m))
		if err != nil {
			errs = append(errs, err)
			continue
		}
		s.Fixtures[f.Name] = f
	}
	cases, _ := filepath.Glob(filepath.Join(root, "cases", "*", "*", "case.json"))
	for _, m := range cases {
		c, err := ReadCase(filepath.Dir(m))
		if err != nil {
			errs = append(errs, err)
			continue
		}
		s.Cases = append(s.Cases, c)
	}
	sort.Slice(s.Cases, func(i, j int) bool { return s.Cases[i].ID < s.Cases[j].ID })
	if len(cases) == 0 {
		errs = append(errs, fmt.Errorf("%s: no cases/<module>/<name>/case.json", root))
	}
	return s, errors.Join(errs...)
}

// Case returns a case by id.
func (s *Suite) Case(id string) *Case {
	for _, c := range s.Cases {
		if c.ID == id {
			return c
		}
	}
	return nil
}

// presence decodes the key set alone and reports what is missing and what is
// unknown, every one by name, before the typed decode has a chance to say
// only "unknown field" for the first of them.
func presence(p *Problems, raw []byte, required, optional []string) (map[string]json.RawMessage, bool) {
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		p.add("not a JSON object: %v", err)
		return nil, false
	}
	known := map[string]bool{}
	var missing, unknown []string
	for _, k := range required {
		known[k] = true
		if _, ok := keys[k]; !ok {
			missing = append(missing, k)
		}
	}
	for _, k := range optional {
		known[k] = true
	}
	for k := range keys {
		if !known[k] {
			unknown = append(unknown, k)
		}
	}
	sort.Strings(unknown)
	if len(missing) > 0 {
		p.add("missing %s", strings.Join(missing, ", "))
	}
	if len(unknown) > 0 {
		p.add("unknown key %s", strings.Join(unknown, ", "))
	}
	return keys, len(missing) == 0 && len(unknown) == 0
}

// strict decodes with unknown fields refused and a type error named by field.
func strict(raw []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		var te *json.UnmarshalTypeError
		if errors.As(err, &te) {
			return fmt.Errorf("%s is %s, want %s", te.Field, te.Value, te.Type)
		}
		return err
	}
	return nil
}

func oneOf(p *Problems, key, got string, allowed []string) {
	for _, a := range allowed {
		if got == a {
			return
		}
	}
	p.add("%s %q is not one of %s", key, got, strings.Join(allowed, ", "))
}
