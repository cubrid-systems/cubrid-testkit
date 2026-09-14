package exec

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Quote wraps a string so a shell reads it as one word, whatever is in it.
//
// Here because everything this package runs is a script: a path that came from
// a configuration file, a case, or an environment variable is interpolated into
// one, and a space alone turns one `rm -rf` into two.
//
// The idiom for a single quote inside single quotes is close, escape, reopen:
//
//	foo'bar  ->  'foo'\''bar'
//
// Closing and immediately reopening instead -- three quotes in a row, with no
// backslash -- is an empty string, so a'b comes out as ab and a delete aimed at
// one path lands on another.
//
// The examples are in an indented block because gofmt rewrites a pair of
// straight quotes in running text into a typographic one, which is exactly the
// character this comment is about.
func Quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// SafeToEmpty checks a directory before it is handed to a destructive command.
//
// Written after `find ${CUBRID}/ -name "core" | xargs -i rm -rf {}` ran with
// CUBRID unset and deleted 69 directories across a machine, one of them inside a
// VS Code server. The lesson is narrow and worth stating: in a shell script an
// unset or careless path does not fail, it becomes the root of the filesystem,
// and every guard has to be in front of the command rather than in the reader's
// head.
//
// Three things are required and none of them is clever:
//
//   - absolute, so the result does not depend on where the command happens to run;
//   - at least two segments, so "/" and "/usr" and "/home" cannot be emptied by
//     a typo in a configuration file;
//   - no shell metacharacters, because these paths are interpolated into scripts
//     and a space alone turns one `rm -rf` into two.
func SafeToEmpty(what, dir string) error {
	d := strings.TrimSpace(dir)
	if d == "" {
		return fmt.Errorf("%s is empty, and an empty path in a shell command is the root of the filesystem", what)
	}
	if !filepath.IsAbs(d) {
		return fmt.Errorf("%s must be an absolute path, and is %q", what, d)
	}
	clean := filepath.Clean(d)
	if n := len(strings.Split(strings.Trim(clean, "/"), "/")); n < 2 {
		return fmt.Errorf("%s is %q; this runner will not empty a directory that shallow", what, clean)
	}
	if i := strings.IndexAny(clean, " \t\n'\"`$&;|<>()*?[]{}\\!#~"); i >= 0 {
		return fmt.Errorf("%s contains %q, which a shell would read as syntax: %q", what, clean[i:i+1], clean)
	}
	return nil
}
