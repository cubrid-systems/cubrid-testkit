package result

import (
	"fmt"
	"io"
	"strings"
	"time"
)

// The dispatcher's own output, from CTP.java's task loop. It brackets every task,
// including one whose name it does not recognise -- the opening banner is printed
// before the lookup, so an unknown task gets a banner and then help.
//
// These are package functions rather than Sink methods because they are printed
// by the dispatcher, before any runner has opened a result directory. Keeping
// them here holds the rule that every observable byte comes out of this package.

// javaDateLayout matches java.util.Date.toString(), which is what CTP
// interpolated: "Wed Sep 02 21:03:13 KST 2026".
const javaDateLayout = "Mon Jan 02 15:04:05 MST 2006"

// TaskBanner opens a task: a blank line, then the rule.
//
//	(blank)
//	====================================== SHELL ==========================================
func TaskBanner(w io.Writer, task string, _ time.Time) {
	fmt.Fprintln(w)
	fmt.Fprintf(w, "====================================== %s ==========================================\n",
		strings.ToUpper(task))
}

// TaskStarted announces the start, after the name has been recognised.
//
//	[SHELL] TEST STARTED (Wed Sep 02 21:03:13 KST 2026)
//	(blank)
func TaskStarted(w io.Writer, task string, start time.Time) {
	fmt.Fprintf(w, "[%s] TEST STARTED (%s)\n", strings.ToUpper(task), start.Format(javaDateLayout))
	fmt.Fprintln(w)
}

// TaskEnded closes a task.
//
//	[SHELL] TEST END (Wed Sep 02 21:05:44 KST 2026)
//	[SHELL] ELAPSE TIME: 151 seconds
//
// The elapsed value is whole seconds: CTP divides milliseconds by 1000.0 and casts
// the result to long, so it truncates rather than rounding.
func TaskEnded(w io.Writer, task string, end time.Time, elapsed time.Duration) {
	upper := strings.ToUpper(task)
	fmt.Fprintf(w, "[%s] TEST END (%s)\n", upper, end.Format(javaDateLayout))
	fmt.Fprintf(w, "[%s] ELAPSE TIME: %d seconds\n", upper, int64(elapsed.Seconds()))
}
