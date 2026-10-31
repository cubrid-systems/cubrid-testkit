package perf

import (
	"context"
	"crypto/sha1"
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
// shows here without a case of its own. Scope is the paramdump section the
// values were compared in (section), or the one build's section when only
// one has it. A nil value is a build without the parameter in any section.
// Conf lists the conf groups it was seen in, and is empty when it was seen
// in every group compared. Parameters set from the node or the start time
// are left out (instanceParams).
type ParamDiff struct {
	Scope     string   `json:"scope"`
	Name      string   `json:"name"`
	Target    *string  `json:"target"`
	Reference *string  `json:"reference"`
	Conf      []string `json:"conf,omitempty"`
}

// paramsFile is where recordParams leaves a side's paramdump, under /work.
const paramsFile = "perf/params.txt"

// recordParams keeps the parameters a side's server runs with, and the ones
// a client process on its node loads from the same cubrid.conf: cubrid
// paramdump against a fixture's running server, once per side, the
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

// keepParams copies a side's paramdump beside its describe artifact, one
// file per conf group (a role's clusters share a name across groups), the
// group on its first line.
func keepParams(s *Side, dir string) {
	if s == nil || !s.paramsTried {
		return
	}
	text, err := os.ReadFile(filepath.Join(s.Work, paramsFile))
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Join(dir, "clusters"), 0o755)
	h := sha1.Sum([]byte(s.confKey))
	name := fmt.Sprintf("%s.%x.params.txt", s.Name, h[:4])
	_ = os.WriteFile(filepath.Join(dir, "clusters", name), append([]byte("# perf conf group: "+confLabel(s.confKey)+"\n"), text...), 0o644)
}

// paramValue is one parameter as paramdump printed it, in each section that
// has it.
type paramValue struct {
	server, client     string
	inServer, inClient bool
}

// section is the section two builds compare a parameter in: the server's
// when both print it there, else the client's when both do there (11.4's
// server section has the client-only parameters too, at their defaults: the
// server skips them when it loads cubrid.conf), else each build's own, the
// server's first, the scope then "<target's>/<reference's>" when the two
// differ.
func section(t, r paramValue) (scope, tv, rv string) {
	switch {
	case t.inServer && r.inServer:
		return "server", t.server, r.server
	case t.inClient && r.inClient:
		return "client", t.client, r.client
	}
	scope, tv = own(t)
	rs, rv := own(r)
	if rs != scope {
		scope += "/" + rs
	}
	return scope, tv, rv
}

// own is a parameter's section and value in one build, the server's first.
func own(v paramValue) (scope, value string) {
	if v.inServer {
		return "server", v.server
	}
	return "client", v.client
}

// instanceParams are set from the node or the start time, not the build:
// they differ between any two clusters.
var instanceParams = map[string]bool{"error_log": true, "ha_node_list": true}

// parseParamdump reads cubrid paramdump -b into one entry per parameter.
// The sections are not the same on the two lines: 11.4 prints every
// parameter in both, develop only the client's in the first and the
// server's in the second; so a parameter is keyed by name, its value kept
// per section (section). develop prints "[S*] name=value (default)",
// 11.4.6 "name=value"; a line's section comes from the header before it
// ("... used by the client program", "... loaded by the server '<db>'").
func parseParamdump(text string) map[string]paramValue {
	out := map[string]paramValue{}
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
			line = cutDefault(line[5:])
		}
		name, value, found := strings.Cut(line, "=")
		if !found || name == "" || strings.ContainsAny(name, " \t") {
			continue
		}
		v := out[name]
		if scope == "server" {
			v.server, v.inServer = value, true
		} else {
			v.client, v.inClient = value, true
		}
		out[name] = v
	}
	return out
}

// cutDefault drops develop's " (default)" from "name=value (default)": the
// parenthesis that opens the last one, the default itself may hold some.
func cutDefault(line string) string {
	if !strings.HasSuffix(line, ")") {
		return line
	}
	depth := 0
	for i := len(line) - 1; i > 0; i-- {
		switch line[i] {
		case ')':
			depth++
		case '(':
			if depth--; depth == 0 {
				if line[i-1] == ' ' {
					return line[:i-1]
				}
				return line
			}
		}
	}
	return line
}

// confLabel is a conf key as the record shows it.
func confLabel(key string) string {
	if key == "" {
		return "(no conf)"
	}
	return key
}

// paramDiffs is a pair's ParamDiff list as it grows, one conf group at a
// time.
type paramDiffs struct {
	groups  int
	entries []*ParamDiff
}

// add compares the two sides of one conf group, and says so; a side
// without parameters is not compared.
func (d *paramDiffs) add(conf string, target, reference map[string]paramValue) bool {
	if len(target) == 0 || len(reference) == 0 {
		return false
	}
	conf = confLabel(conf)
	d.groups++
	names := make([]string, 0, len(target)+len(reference))
	for n := range target {
		names = append(names, n)
	}
	for n := range reference {
		if _, both := target[n]; !both {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	for _, n := range names {
		if instanceParams[n] {
			continue
		}
		t, tok := target[n]
		r, rok := reference[n]
		var scope, tv, rv string
		switch {
		case tok && rok:
			if scope, tv, rv = section(t, r); tv == rv {
				continue
			}
		case tok:
			scope, tv = own(t)
		default:
			scope, rv = own(r)
		}
		e := d.find(n, tv, tok, rv, rok)
		if e == nil {
			e = &ParamDiff{Name: n, Scope: scope}
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
	return true
}

func (d *paramDiffs) find(name, tv string, tok bool, rv string, rok bool) *ParamDiff {
	same := func(p *string, v string, ok bool) bool { return (p == nil) == !ok && (p == nil || *p == v) }
	for _, e := range d.entries {
		if e.Name == name && same(e.Target, tv, tok) && same(e.Reference, rv, rok) {
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
