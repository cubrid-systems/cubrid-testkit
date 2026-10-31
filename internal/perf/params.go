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
// value was read from (the target's when it has the parameter). A nil value
// is a build without the parameter in any section. Conf lists the conf
// groups it was seen in, and is empty when it was seen in every group
// compared.
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

// paramValue is one parameter as paramdump printed it.
type paramValue struct{ scope, value string }

// parseParamdump reads cubrid paramdump -b into one value per parameter.
// The sections are not the same on the two lines: 11.4 prints every
// parameter in both, develop only the client's in the first and the
// server's in the second. So a parameter is keyed by name, the server's
// section winning, and the section is kept for the record. develop prints
// "[S*] name=value (default)", 11.4.6 "name=value"; a line's section comes
// from the header before it ("... used by the client program", "... loaded
// by the server '<db>'").
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
		if _, have := out[name]; have && scope == "client" {
			continue
		}
		out[name] = paramValue{scope: scope, value: value}
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
		tv, tok := target[n]
		rv, rok := reference[n]
		if tok && rok && tv.value == rv.value {
			continue
		}
		e := d.find(n, tv.value, tok, rv.value, rok)
		if e == nil {
			e = &ParamDiff{Name: n, Scope: rv.scope}
			if tok {
				e.Scope = tv.scope
				e.Target = &tv.value
			}
			if rok {
				e.Reference = &rv.value
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
