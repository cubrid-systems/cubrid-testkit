package perf

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"strings"
	"time"
)

// Branch is one registration line in branches.conf: a team branch the weekly
// session compares against its merge-base with develop. The file changes only
// by pull request; the session reads it after a pull.
type Branch struct {
	Name  string
	Repo  string // owner/repo on GitHub; CUBRID/cubrid when the line does not say
	Owner string // GitHub login the flag goes to
	Cases []string
	Until time.Time // local midnight of the until= day; zero when open-ended
	Line  int
}

// DefaultRepo is where a branch lives when the registration does not say:
// team branches are usually on a personal fork, and that is what repo= is for.
const DefaultRepo = "CUBRID/cubrid"

// Expired says whether the registration's until day has passed -- the whole
// day, in this machine's zone, because the session starts at 02:00 local and
// an until= read as UTC would run a Friday registration on Saturday.
func (b Branch) Expired(now time.Time) bool {
	return !b.Until.IsZero() && !now.Before(b.Until.AddDate(0, 0, 1))
}

// Selects says whether a case id is on the registration's cases= list; a
// registration without one runs the whole primary list.
func (b Branch) Selects(id string) bool {
	if len(b.Cases) == 0 {
		return true
	}
	for _, g := range b.Cases {
		if ok, _ := path.Match(g, id); ok {
			return true
		}
	}
	return false
}

// ReadBranches parses branches.conf. One line is one registration:
//
//	<branch>  repo=<owner/repo>  owner=<login>  [cases=<glob,glob>]  [until=<YYYY-MM-DD>]
//
// A '#' at the start of a line or after a space begins a comment. Every
// problem is reported with its line, and a key the format does not have is
// one of them -- a misspelt until= would otherwise register a branch for
// ever.
func ReadBranches(file string) ([]Branch, error) {
	p := &Problems{Path: file}
	f, err := os.Open(file)
	if err != nil {
		p.add("%v", err)
		return nil, p
	}
	defer f.Close()

	var out []Branch
	seen := map[string]int{}
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := uncomment(sc.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		b := Branch{Name: fields[0], Repo: DefaultRepo, Line: n}
		if prev, dup := seen[b.Name]; dup {
			p.add("line %d: %s is already registered on line %d", n, b.Name, prev)
			continue
		}
		seen[b.Name] = n
		for _, kv := range fields[1:] {
			k, v, ok := strings.Cut(kv, "=")
			if !ok || k == "" || v == "" {
				p.add("line %d: %q is not key=value", n, kv)
				continue
			}
			switch k {
			case "repo":
				owner, repo, ok := strings.Cut(v, "/")
				if !ok || owner == "" || repo == "" || strings.Contains(repo, "/") {
					p.add("line %d: repo=%q is not owner/repo", n, v)
				}
				b.Repo = v
			case "owner":
				b.Owner = v
			case "cases":
				for _, g := range strings.Split(v, ",") {
					if _, err := path.Match(g, ""); err != nil || g == "" {
						p.add("line %d: cases=%q has a pattern that does not match anything: %q", n, v, g)
					}
					b.Cases = append(b.Cases, g)
				}
			case "until":
				t, err := time.ParseInLocation("2006-01-02", v, time.Local)
				if err != nil {
					p.add("line %d: until=%q is not YYYY-MM-DD", n, v)
				}
				b.Until = t
			default:
				p.add("line %d: unknown key %s", n, k)
			}
		}
		if b.Owner == "" {
			p.add("line %d: %s has no owner=; the flag has nobody to go to", n, b.Name)
		}
		out = append(out, b)
	}
	if err := sc.Err(); err != nil {
		p.add("%v", err)
	}
	return out, p.err()
}

func uncomment(line string) string {
	line = strings.TrimSpace(line)
	if strings.HasPrefix(line, "#") {
		return ""
	}
	for _, sep := range []string{" #", "\t#"} {
		if i := strings.Index(line, sep); i >= 0 {
			line = line[:i]
		}
	}
	return strings.TrimSpace(line)
}

// Active applies the two rules that take a registration out of a session
// without an error: an until day that passed, and the cap on how many run.
// Lines are appended as branches are registered, so the oldest are the first
// lines, and those are what the cap skips (Spec §7.5.1). The ones left out
// are returned with the reason, for the summary.
func Active(all []Branch, limit int, now time.Time) (run []Branch, left []string) {
	var live []Branch
	for _, b := range all {
		if b.Expired(now) {
			left = append(left, fmt.Sprintf("%s: expired %s", b.Name, b.Until.Format("2006-01-02")))
			continue
		}
		live = append(live, b)
	}
	for i, b := range live {
		if len(live)-i > limit {
			left = append(left, fmt.Sprintf("%s: beyond branches.max=%d, and the oldest registration", b.Name, limit))
			continue
		}
		run = append(run, b)
	}
	return run, left
}
