package haapply

// Outcome is what a pattern's rounds measured.
type Outcome struct {
	Pattern string
	Rounds  int
	// Ahead is the rounds whose witness was on the slave while T1's ballast
	// was not: T2 applied first.
	Ahead int
	// Never is the rounds whose witness did not reach the slave in time.
	Never int
	// Refused is the rounds the master would not take one of the writes of.
	Refused int
	// Stalled is a slave that stopped applying a round's ballast; the pattern
	// ended there.
	Stalled bool
	// Final is "same", "differ:<table>" for the first table that differs, or
	// "stalled".
	Final     string
	FailDelta int
	Writeset
}

// Verdicts (ADR-EXT-012 3-3).
const (
	Held         = "held"
	Missed       = "missed"
	Inconclusive = "inconclusive"
	// The control shares nothing, so its going first is what it is for.
	ControlOvertook = "control-overtook"
	ControlStill    = "control-did-not-overtake"
)

// Judge gives each outcome its verdict, in order.
//
// A miss stands on its own: T2 seen first, an apply failure, a slave that
// stopped, or nodes that differ at the end are each direct evidence. A hold
// is evidence only if the run could have seen a miss, which is what the
// control says: on a pair where it never went first the slave did not
// reorder, and every hold there is inconclusive. A witness that did not
// arrive in time, with nothing else wrong, is a slow slave rather than a
// miss, and so is inconclusive too.
func Judge(outs []Outcome) []string {
	overtook := false
	for _, o := range outs {
		if o.Pattern == "ctl" && o.Ahead > 0 {
			overtook = true
		}
	}
	verdicts := make([]string, len(outs))
	for i, o := range outs {
		broken := o.Final != "same" || o.FailDelta != 0 || o.Stalled
		switch {
		case o.Pattern == "ctl" && broken:
			verdicts[i] = Missed
		case o.Pattern == "ctl" && o.Ahead > 0:
			verdicts[i] = ControlOvertook
		case o.Pattern == "ctl":
			verdicts[i] = ControlStill
		case broken || o.Ahead > 0:
			verdicts[i] = Missed
		case o.Refused > 0 || o.Never > 0 || !overtook:
			verdicts[i] = Inconclusive
		default:
			verdicts[i] = Held
		}
	}
	return verdicts
}
