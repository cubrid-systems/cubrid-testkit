package perf

import (
	"bufio"
	"math"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/cubrid-systems/cubrid-testkit/internal/conf"
)

// Pair is one comparison the session runs: a target build against a reference
// build, and optionally the same target against a second reference while the
// reference is being replaced (overlap).
type Pair struct {
	Name      string
	Target    string // an install tree, or a symbolic name the build step resolves
	Reference string
	Overlap   string
}

// Conf is perf.conf: the same flat properties every other testkit conf is,
// read with the same loader, with the keys the Spec's §7.5 lists and no
// others.
type Conf struct {
	Path string

	Pairs []Pair // in file order; develop is meant to come first

	Suite          string
	Branches       string
	BranchesMax    int
	Builds         string
	BuildsManifest string

	Canaries        []string
	CanaryTolerance float64
	Interleave      string
	SessionBudgetS  int
	MemoryCap       string
	DiskMinGB       int

	CPUSetServer string
	CPUSetClient string
	CSB          string
	CSBHome      string
	ClientImage  string

	ReportMode          string
	ReportMail          string
	ReportWebhook       string
	ReportWebhookFormat string
	ConbenchURL         string
}

var (
	Interleaves = []string{"case", "round"}
	ReportModes = []string{"dry", "team"}

	// A perf.conf key is one of these, or pair.<name> / overlap.<name>.
	confRequired = []string{"suite", "builds", "builds.manifest", "canaries", "canary_tolerance", "interleave",
		"session_budget_s", "csb", "csb_home", "client_image", "report.mode"}
	confOptional = []string{"branches", "branches.max", "memory_cap", "disk_min_gb", "cpuset.server", "cpuset.client",
		"report.mail", "report.webhook", "report.webhook_format", "conbench.url"}

	cpusetRe    = regexp.MustCompile(`^[0-9]+(-[0-9]+)?(,[0-9]+(-[0-9]+)?)*$`)
	memoryCapRe = regexp.MustCompile(`^[0-9]+[kKmMgG]?$`)
	caseIDRe    = regexp.MustCompile(`^[a-z]+\.[a-z0-9_]+$`)
)

// ReadConf reads perf.conf and applies every rule a session would stop on.
// Whole-line comments only: a comment after a value is part of the value,
// which is the loader's contract and not something this file changes.
func ReadConf(file string) (*Conf, error) {
	p := &Problems{Path: file}
	home, err := conf.FindHome()
	if err != nil {
		home = &conf.Home{}
	}
	cfg, err := home.Load(file)
	if err != nil {
		p.add("%v", err)
		return nil, p
	}
	duplicateKeys(p, file)
	c := &Conf{Path: file, BranchesMax: 3}

	known := map[string]bool{}
	for _, k := range append(append([]string{}, confRequired...), confOptional...) {
		known[k] = true
	}
	for _, k := range confRequired {
		if v, ok := cfg.Get(k); !ok || strings.TrimSpace(v) == "" {
			p.add("missing %s", k)
		}
	}
	// A pair that could not be read is remembered, so its overlap is not
	// also reported as having no pair.
	broken := map[string]bool{}
	for _, k := range cfg.Keys() {
		v := strings.TrimSpace(cfg.GetOr(k, ""))
		switch {
		case strings.HasPrefix(k, "pair."):
			name := strings.TrimPrefix(k, "pair.")
			target, ref, ok := strings.Cut(v, ";")
			target, ref = strings.TrimSpace(target), strings.TrimSpace(ref)
			if name == "" {
				p.add("pair. has no name")
				continue
			}
			if !ok || target == "" || ref == "" || strings.Contains(ref, ";") {
				p.add("%s wants \"<target> ; <reference>\", got %q", k, v)
				broken[name] = true
				continue
			}
			c.Pairs = append(c.Pairs, Pair{Name: name, Target: target, Reference: ref})
		case strings.HasPrefix(k, "overlap."):
			// handled after the pairs, whatever order the file has them in
		case !known[k]:
			p.add("unknown key %s", k)
		}
	}
	if len(c.Pairs) == 0 && len(broken) == 0 {
		p.add("no pair.<name> = <target> ; <reference>; a session has nothing to compare")
	}
	for _, k := range cfg.Keys() {
		if !strings.HasPrefix(k, "overlap.") {
			continue
		}
		name, v := strings.TrimPrefix(k, "overlap."), strings.TrimSpace(cfg.GetOr(k, ""))
		i := indexPair(c.Pairs, name)
		switch {
		case broken[name]:
		case i < 0:
			p.add("%s has no pair.%s to overlap", k, name)
		case v == "":
			p.add("%s is empty", k)
		default:
			c.Pairs[i].Overlap = v
		}
	}

	c.Suite = cfg.GetOr("suite", "")
	c.Branches = cfg.GetOr("branches", "")
	c.Builds = cfg.GetOr("builds", "")
	c.BuildsManifest = cfg.GetOr("builds.manifest", "")
	c.MemoryCap = cfg.GetOr("memory_cap", "")
	c.CPUSetServer = cfg.GetOr("cpuset.server", "")
	c.CPUSetClient = cfg.GetOr("cpuset.client", "")
	c.CSB = cfg.GetOr("csb", "")
	c.CSBHome = cfg.GetOr("csb_home", "")
	c.ClientImage = cfg.GetOr("client_image", "")
	c.Interleave = cfg.GetOr("interleave", "")
	c.ReportMode = cfg.GetOr("report.mode", "")
	c.ReportMail = cfg.GetOr("report.mail", "")
	c.ReportWebhook = cfg.GetOr("report.webhook", "")
	c.ReportWebhookFormat = cfg.GetOr("report.webhook_format", "")
	c.ConbenchURL = cfg.GetOr("conbench.url", "")

	c.BranchesMax = intAtLeast(p, cfg, "branches.max", 3, 0)
	c.SessionBudgetS = intAtLeast(p, cfg, "session_budget_s", 0, 1)
	c.DiskMinGB = intAtLeast(p, cfg, "disk_min_gb", 0, 0)
	if v, ok := cfg.Get("canary_tolerance"); ok {
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil || math.IsInf(f, 0) || math.IsNaN(f) || !(f > 0) {
			p.add("canary_tolerance must be a number above 0, got %q", v)
		}
		c.CanaryTolerance = f
	}
	if v, ok := cfg.Get("canaries"); ok {
		for _, id := range strings.Split(v, ",") {
			id = strings.TrimSpace(id)
			if !caseIDRe.MatchString(id) {
				p.add("canaries has %q, which is not a <module>.<name> case id", id)
			}
			c.Canaries = append(c.Canaries, id)
		}
	}
	if c.Interleave != "" {
		oneOf(p, "interleave", c.Interleave, Interleaves)
	}
	if c.ReportMode != "" {
		oneOf(p, "report.mode", c.ReportMode, ReportModes)
	}
	// FR-25 (2026-10-06): delivery is the hub's dashboard, which everyone can
	// reach; mail and messenger pushes are a later phase. report.mail and
	// report.webhook are accepted so a conf written for that phase validates
	// now, and are not read.
	if c.MemoryCap != "" && !memoryCapRe.MatchString(c.MemoryCap) {
		p.add("memory_cap wants a size like 24G, got %q", c.MemoryCap)
	}
	for _, kv := range []struct{ key, set string }{{"cpuset.server", c.CPUSetServer}, {"cpuset.client", c.CPUSetClient}} {
		if kv.set != "" && !cpusetRe.MatchString(kv.set) {
			p.add("%s wants a CPU list like 0-7,16-23, got %q", kv.key, kv.set)
		}
	}
	if c.ConbenchURL != "" {
		if u, err := url.Parse(c.ConbenchURL); err != nil || u.Scheme == "" || u.Host == "" {
			p.add("conbench.url is not a URL: %q", c.ConbenchURL)
		}
	}
	if c.ReportWebhook != "" && c.ReportWebhookFormat == "" {
		p.add("report.webhook is set and report.webhook_format is not")
	}
	return c, p.err()
}

// duplicateKeys reads the file once more as lines, because the loader keeps
// the last value of a repeated key and says nothing -- and a second
// pair.develop that silently replaces the first is a session comparing the
// wrong builds. branches.conf refuses a duplicate; so does this.
func duplicateKeys(p *Problems, file string) {
	f, err := os.Open(file)
	if err != nil {
		return
	}
	defer f.Close()
	first := map[string]int{}
	sc := bufio.NewScanner(f)
	continued := false
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimLeft(sc.Text(), " \t\f")
		if continued {
			continued = strings.HasSuffix(line, `\`)
			continue
		}
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		continued = strings.HasSuffix(line, `\`)
		key := strings.TrimSpace(line[:strings.IndexAny(line+"=", "=:")])
		if prev, dup := first[key]; dup {
			p.add("%s appears twice, on line %d and line %d; the second would win in silence", key, prev, n)
			continue
		}
		first[key] = n
	}
}

func indexPair(pairs []Pair, name string) int {
	for i, pr := range pairs {
		if pr.Name == name {
			return i
		}
	}
	return -1
}

func intAtLeast(p *Problems, cfg *conf.Config, key string, fallback, min int) int {
	v, ok := cfg.Get(key)
	if !ok {
		return fallback
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < min {
		p.add("%s must be an integer of %d or more, got %q", key, min, v)
		return fallback
	}
	return n
}
