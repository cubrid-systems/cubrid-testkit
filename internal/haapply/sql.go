package haapply

import (
	"regexp"
	"strconv"
	"strings"
)

// shellQuote makes s one word for sh: the SQL carries quotes of its own.
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

var numberLine = regexp.MustCompile(`^\s*(-?[0-9]+)\s*$`)

// firstNumber is the first line of csql's output that is a number alone, or ""
// for none: the shape of a one-value query, header and row count aside.
func firstNumber(out string) string {
	for _, l := range strings.Split(out, "\n") {
		if m := numberLine.FindStringSubmatch(l); m != nil {
			return m[1]
		}
	}
	return ""
}

// Writeset is the master's writeset counters the runner reads around a round.
type Writeset struct {
	Write, Ref, Fallback, Clear int
}

func (w Writeset) minus(o Writeset) Writeset {
	return Writeset{w.Write - o.Write, w.Ref - o.Ref, w.Fallback - o.Fallback, w.Clear - o.Clear}
}

func (w *Writeset) add(o Writeset) {
	w.Write += o.Write
	w.Ref += o.Ref
	w.Fallback += o.Fallback
	w.Clear += o.Clear
}

// parseWriteset reads `SHOW WRITESET STATUS` in csql's line mode (-l), a
// "name : value" per line.
func parseWriteset(out string) Writeset {
	var w Writeset
	for _, l := range strings.Split(out, "\n") {
		name, val, ok := strings.Cut(l, ":")
		if !ok {
			continue
		}
		fields := strings.Fields(name)
		if len(fields) == 0 {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(val))
		if err != nil {
			continue
		}
		switch fields[len(fields)-1] {
		case "Conflict_write_count":
			w.Write = n
		case "Conflict_ref_count":
			w.Ref = n
		case "Commit_fallback_count":
			w.Fallback = n
		case "History_clear_count":
			w.Clear = n
		}
	}
	return w
}

// normalise drops what differs between two runs of one query on two nodes:
// timings, csql's connection notices and blank lines.
func normalise(out string) string {
	var keep []string
	for _, l := range strings.Split(out, "\n") {
		t := strings.TrimSpace(l)
		switch {
		case t == "",
			strings.Contains(t, "sec)"),
			strings.Contains(t, "Committed"),
			strings.Contains(t, "NOTIFICATION"),
			strings.HasPrefix(t, "Program 'csql'"),
			strings.HasPrefix(t, "Time:"):
			continue
		}
		keep = append(keep, t)
	}
	return strings.Join(keep, "\n")
}
