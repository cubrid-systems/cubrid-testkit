package perf

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ParamDiff is a system parameter the two builds of a pair read differently
// under the same conf, or that only one of them has: layer 1 of the
// parameter matrix -- a default that changed, or a parameter that is new,
// shows here without a case of its own. Scope is the paramdump section,
// server or client. A nil value is a build without the parameter. Conf
// lists the conf groups it was seen in, and is empty when it was seen in
// every group compared.
type ParamDiff struct {
	Scope     string   `json:"scope"`
	Name      string   `json:"name"`
	Target    *string  `json:"target"`
	Reference *string  `json:"reference"`
	Conf      []string `json:"conf,omitempty"`
}

// paramsFile is where recordParams leaves a side's paramdump, under /work.
const paramsFile = "perf/params.txt"

// recordParams keeps the parameters a side's server and clients run with:
// cubrid paramdump against a fixture's running server, once per side, the
// cluster's conf being the same for every fixture. -b: 11.4.6 prints the
// client's section only with it (develop always does). Hidden parameters are
// left out on both builds (11.4.6's paramdump has no --dump-flag). A failure
// is logged and leaves the side without parameters; nothing is measured
// from them.
func (r *Runner) recordParams(ctx context.Context, s *Side, db string) {
	s.paramsTried = true
	res, err := r.exec(ctx, s.N1, fmt.Sprintf("cubrid paramdump -b %s > /work/%s 2>&1", db, paramsFile), 2*time.Minute)
	if err == nil && res.ExitCode != 0 {
		err = fmt.Errorf("exit %d", res.ExitCode)
	}
	if err != nil {
		r.logf("%s: paramdump %s: %v", s.Role, db, err)
		return
	}
	text, err := os.ReadFile(filepath.Join(s.Work, paramsFile))
	if err != nil {
		r.logf("%s: paramdump %s: %v", s.Role, db, err)
		return
	}
	s.params = parseParamdump(string(text))
	if len(s.params) == 0 {
		r.logf("%s: paramdump %s: no parameters read", s.Role, db)
	}
}

// keepParams copies a side's paramdump beside its describe artifact.
func keepParams(s *Side, dir string) {
	if s == nil {
		return
	}
	src := filepath.Join(s.Work, paramsFile)
	if !exists(src) {
		return
	}
	_ = os.MkdirAll(filepath.Join(dir, "clusters"), 0o755)
	_ = copyFile(src, filepath.Join(dir, "clusters", s.Name+".params.txt"))
}

// parseParamdump reads cubrid paramdump's two sections into scope/name keys.
// develop prints "[S*] name=value (default)", 11.4.6 "name=value"; the
// section a line is in comes from the header before it ("... used by the
// client program", "... loaded by the server '<db>'").
func parseParamdump(text string) map[string]string {
	out := map[string]string{}
	scope := ""
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, "\r")
		switch {
		case strings.Contains(line, "used by the client program"):
			scope = "client"
			continue
		case strings.Contains(line, "loaded by the server"):
			scope = "server"
			continue
		case scope == "" || line == "" || strings.HasPrefix(line, "#"):
			continue
		}
		if len(line) > 5 && line[0] == '[' && line[3] == ']' && line[4] == ' ' {
			line = line[5:]
			if i := strings.LastIndex(line, " ("); i >= 0 && strings.HasSuffix(line, ")") {
				line = line[:i]
			}
		}
		name, value, found := strings.Cut(line, "=")
		if !found || name == "" || strings.ContainsAny(name, " \t") {
			continue
		}
		out[scope+"/"+name] = value
	}
	return out
}

// paramDiffs is a pair's ParamDiff list as it grows, one conf group at a
// time.
type paramDiffs struct {
	groups  int
	entries []*ParamDiff
}

// add compares the two sides of one conf group. A side without parameters
// is not compared.
func (d *paramDiffs) add(conf string, target, reference map[string]string) {
	if len(target) == 0 || len(reference) == 0 {
		return
	}
	d.groups++
	keys := map[string]bool{}
	for k := range target {
		keys[k] = true
	}
	for k := range reference {
		keys[k] = true
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	for _, k := range sorted {
		tv, tok := target[k]
		rv, rok := reference[k]
		if tok && rok && tv == rv {
			continue
		}
		scope, name, _ := strings.Cut(k, "/")
		e := d.find(scope, name, tv, tok, rv, rok)
		if e == nil {
			e = &ParamDiff{Scope: scope, Name: name}
			if tok {
				e.Target = &tv
			}
			if rok {
				e.Reference = &rv
			}
			d.entries = append(d.entries, e)
		}
		e.Conf = append(e.Conf, conf)
	}
}

func (d *paramDiffs) find(scope, name, tv string, tok bool, rv string, rok bool) *ParamDiff {
	same := func(p *string, v string, ok bool) bool { return (p == nil) == !ok && (p == nil || *p == v) }
	for _, e := range d.entries {
		if e.Scope == scope && e.Name == name && same(e.Target, tv, tok) && same(e.Reference, rv, rok) {
			return e
		}
	}
	return nil
}

// list is the record: an entry seen in every group compared drops its conf
// list.
func (d *paramDiffs) list() []ParamDiff {
	out := make([]ParamDiff, 0, len(d.entries))
	for _, e := range d.entries {
		c := *e
		if len(c.Conf) == d.groups {
			c.Conf = nil
		}
		out = append(out, c)
	}
	return out
}
