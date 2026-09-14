package isolationsuite

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/cubrid-systems/cubrid-testkit/internal/exec"
)

// dirNotFound is the word Main.calcScenario looks for.
const dirNotFound = "DIR_NOT_FOUND"

// resolveScenario is Main.calcScenario. A scenario under $HOME becomes relative
// to it, because every script starts with `cd`, and the case paths discovered
// from it are relative too -- Test.runTestCase puts $HOME/ back in front of them
// when it runs one. A scenario anywhere else is kept as it was written.
func resolveScenario(ctx context.Context, ch exec.Channel, root string) (string, error) {
	res, err := run(ctx, ch, isolationScript("echo $(cd $HOME; pwd)"))
	if err != nil {
		return "", err
	}
	home := output(res)

	res, err = run(ctx, ch, isolationScript(
		"if [ ! -d "+root+" ]; then echo "+dirNotFound+"; fi",
		"echo $(cd "+root+"; pwd)"))
	if err != nil {
		return "", err
	}
	dir := output(res)
	if strings.Contains(dir, dirNotFound) {
		return "", fmt.Errorf("The directory in 'scenario' does not exist. Please check it again at %s.", ch.Describe())
	}
	if strings.HasPrefix(dir, home) {
		if len(dir) > len(home) {
			return dir[len(home)+1:], nil
		}
		return ".", nil
	}
	return strings.TrimSpace(root), nil
}

// discover lists the cases under the scenario (Dispatch.findAllTestCase).
//
// Sorted, which CTP did not do. find's order is readdir's, which differs between
// two runs over the same tree, so CTP's dispatch_tc_ALL.txt was not comparable
// even with itself; the set is the contract, as it is for shell
// (spec-corrections.md, "Where CTP does not agree with itself").
func discover(ctx context.Context, ch exec.Channel, root string) ([]string, error) {
	res, err := run(ctx, ch, isolationScript("cd ", `find `+root+` -name "*.ctl" -type f -print`))
	if err != nil {
		return nil, fmt.Errorf("discover cases under %s: %w", root, err)
	}
	var cases []string
	for _, line := range strings.Split(output(res), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			cases = append(cases, line)
		}
	}
	slices.Sort(cases)
	return cases, nil
}

// exclusions reads the exclusion file (Dispatch.findExcludedList).
//
// Strict where CTP was not: CTP read a file that was not there as a list holding
// cat's error message, matched no case against it, and ran every case the file
// was meant to keep out. shell's runner made the same change, for the same
// reason -- a run that silently includes what it was told to exclude produces
// results someone will believe.
func exclusions(ctx context.Context, ch exec.Channel, file string) ([]string, error) {
	file = strings.TrimSpace(file)
	// Asked separately, in words: the script's exit status is its closing marker's.
	probe, err := run(ctx, ch, isolationScript("cd > /dev/null 2>&1", "if [ -r "+file+" ]; then echo readable; fi"))
	if err == nil && output(probe) != "readable" {
		err = fmt.Errorf("the file is not there or cannot be read")
	}
	if err != nil {
		return nil, fmt.Errorf("testcase_exclude_from_file %s: %w", file, err)
	}
	res, err := run(ctx, ch, isolationScript("cd > /dev/null 2>&1", "cat "+file))
	if err != nil {
		return nil, fmt.Errorf("testcase_exclude_from_file %s: %w", file, err)
	}
	return exclusionEntries(output(res)), nil
}

// exclusionEntries keeps every line that is not blank and does not start with #
// or --, as it was written: the match below is on the untrimmed line, so an entry
// with a trailing space or a carriage return matches nothing, in CTP and here.
func exclusionEntries(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") || strings.HasPrefix(t, "--") {
			continue
		}
		out = append(out, line)
	}
	return out
}

// exclude removes, for each entry, the first case whose path contains it -- one
// case per entry, not every case that matches (Dispatch.java:126-141). An entry
// that names a directory therefore excludes one case in it.
func exclude(cases, entries []string) (kept, skipped []string) {
	kept = slices.Clone(cases)
	for _, e := range entries {
		for j, c := range kept {
			if strings.Contains(c, e) {
				skipped = append(skipped, c)
				kept = slices.Delete(kept, j, j+1)
				break
			}
		}
	}
	return kept, skipped
}
