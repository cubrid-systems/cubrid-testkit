package perf

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"
)

// Exit codes are the Spec's (§7.1): 0 done, 2 a conf or manifest refused the
// start, 3 the environment could not be prepared. validate and list never
// reach 3; they start nothing.
const (
	ExitOK          = 0
	ExitRefused     = 2
	ExitEnvironment = 3
)

const usage = `usage: testkit perf validate <case-dir|fixture-dir|suite-dir|branches.conf|perf.conf>
       testkit perf list     -c <perf.conf> | --suite <dir>

validate reads a manifest, a registration file or a session configuration and
names every problem; exit 2 when there is one. list prints the suite's cases.
session and run are not in this build yet (Design §12, M2).`

// Main is the perf entry point: testkit perf <verb> ..., routed before
// containment because none of this is a run. Measurements and tables go to
// stdout; everything said about the run goes to stderr (Spec §7.1).
func Main(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, usage)
		return ExitRefused
	}
	switch args[0] {
	case "validate":
		return validate(args[1:], stdout, stderr)
	case "list":
		return list(args[1:], stdout, stderr)
	case "session", "run":
		fmt.Fprintf(stderr, "testkit perf %s: not in this build yet; validate and list are\n", args[0])
		return ExitRefused
	default:
		fmt.Fprintf(stderr, "testkit perf: %q is not a verb\n%s\n", args[0], usage)
		return ExitRefused
	}
}

// validate decides what a path is by what is in it, so the one command reads
// every kind of file the session does, with the session's own parser.
func validate(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, usage)
		return ExitRefused
	}
	target := args[0]
	st, err := os.Stat(target)
	if err != nil {
		fmt.Fprintf(stderr, "testkit perf validate: %v\n", err)
		return ExitRefused
	}
	var what string
	switch {
	case st.IsDir() && exists(filepath.Join(target, "case.json")):
		what, err = "case", first(ReadCase(target))
	case st.IsDir() && exists(filepath.Join(target, "fixture.json")):
		what, err = "fixture", first(ReadFixture(target))
	case st.IsDir():
		what, err = "suite", first(LoadSuite(target))
	case filepath.Base(target) == "case.json":
		what, err = "case", first(ReadCase(filepath.Dir(target)))
	case filepath.Base(target) == "fixture.json":
		what, err = "fixture", first(ReadFixture(filepath.Dir(target)))
	case filepath.Base(target) == "branches.conf":
		what, err = "branches", validateBranches(target, stderr)
	case strings.HasSuffix(target, ".conf"):
		what, err = "conf", validateConf(target, stderr)
	default:
		fmt.Fprintf(stderr, "testkit perf validate: %s is not a case, a fixture, a suite, branches.conf or a .conf\n", target)
		return ExitRefused
	}
	if err != nil {
		for _, line := range problemLines(err) {
			fmt.Fprintln(stderr, line)
		}
		return ExitRefused
	}
	fmt.Fprintf(stdout, "%s %s: ok\n", what, target)
	return ExitOK
}

func validateBranches(path string, stderr io.Writer) error {
	all, err := ReadBranches(path)
	if err != nil {
		return err
	}
	for _, b := range all {
		if b.Expired(time.Now()) {
			fmt.Fprintf(stderr, "note: %s expired %s and will not run\n", b.Name, b.Until.Format("2006-01-02"))
		}
	}
	return nil
}

// validateConf reads the conf and then what it points at: the suite and the
// registrations, and that every canary is a case the suite has. A conf that
// passes here is one the session will not stop on. A suite with problems is
// reported as itself, and the checks that need a whole suite wait for one.
func validateConf(path string, stderr io.Writer) error {
	c, err := ReadConf(path)
	if err != nil {
		return err
	}
	p := &Problems{Path: path}
	s, suiteErr := LoadSuite(c.Suite)
	if suiteErr == nil {
		for _, id := range c.Canaries {
			if s.Case(id) == nil {
				p.add("canary %s is not a case in %s", id, c.Suite)
			}
		}
	}
	var branchErr error
	if c.Branches != "" {
		all, err := ReadBranches(c.Branches)
		branchErr = err
		if err == nil {
			_, left := Active(all, c.BranchesMax, time.Now())
			for _, l := range left {
				fmt.Fprintf(stderr, "note: %s\n", l)
			}
			if suiteErr == nil {
				for _, b := range all {
					if countSelected(s, b) == 0 {
						p.add("%s line %d: %s selects no case with cases=%s", c.Branches, b.Line, b.Name, strings.Join(b.Cases, ","))
					}
				}
			}
		}
	}
	return errors.Join(suiteErr, branchErr, p.err())
}

func countSelected(s *Suite, b Branch) int {
	n := 0
	for _, c := range s.Cases {
		if b.Selects(c.ID) {
			n++
		}
	}
	return n
}

// list prints what the session would consider: one line per case, with the
// pass budget and the bound the session checks the weekend against -- for
// the conf's interleave mode, or case mode when there is only a suite. A
// case that could not be read is reported after the table, and the exit says
// so.
func list(args []string, stdout, stderr io.Writer) int {
	var confPath, suite string
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "-c" && i+1 < len(args):
			confPath, i = args[i+1], i+1
		case args[i] == "--suite" && i+1 < len(args):
			suite, i = args[i+1], i+1
		default:
			fmt.Fprintln(stderr, usage)
			return ExitRefused
		}
	}
	if (confPath == "") == (suite == "") {
		fmt.Fprintln(stderr, usage)
		return ExitRefused
	}
	mode := "case"
	if confPath != "" {
		c, err := ReadConf(confPath)
		if err != nil {
			for _, line := range problemLines(err) {
				fmt.Fprintln(stderr, line)
			}
			return ExitRefused
		}
		suite, mode = c.Suite, c.Interleave
	}
	s, err := LoadSuite(suite)
	if s != nil {
		tw := tabwriter.NewWriter(stdout, 0, 8, 2, ' ', 0)
		fmt.Fprintf(tw, "ID\tVER\tGRADE\tOWNER\tDRIVER\tFIXTURE\tPASS_S\tMAX_S(%s)\n", mode)
		total := 0
		for _, c := range s.Cases {
			fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t%s\t%s@%d\t%d\t%d\n",
				c.ID, c.Version, c.Grade, c.Owner, c.Driver, c.Fixture.Name, c.Fixture.Version, c.BudgetS, c.MaxPassS(mode))
			total += c.MaxPassS(mode)
		}
		tw.Flush()
		fmt.Fprintf(stdout, "\n%d case(s), %d fixture(s); one pair is at most %s in %s mode when every pass runs to its budget\n",
			len(s.Cases), len(s.Fixtures), (time.Duration(total) * time.Second).String(), mode)
	}
	if err != nil {
		for _, line := range problemLines(err) {
			fmt.Fprintln(stderr, line)
		}
		return ExitRefused
	}
	return ExitOK
}

func first[T any](v T, err error) error { return err }

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// problemLines renders an error as one line per problem, each with its file,
// so the output reads as a list of things to fix. A joined error is walked
// first: errors.As would stop at the first file's problems and lose the
// rest.
func problemLines(err error) []string {
	if err == nil {
		return nil
	}
	if j, ok := err.(interface{ Unwrap() []error }); ok {
		var out []string
		for _, e := range j.Unwrap() {
			out = append(out, problemLines(e)...)
		}
		return out
	}
	if p, ok := err.(*Problems); ok {
		out := make([]string, 0, len(p.List))
		for _, l := range p.List {
			out = append(out, p.Path+": "+l)
		}
		return out
	}
	return []string{err.Error()}
}
