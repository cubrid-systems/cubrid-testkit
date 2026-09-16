package ctl

import (
	"strconv"
	"strings"
)

// Kind is what a statement asks the controller to do.
type Kind int

const (
	// ToClient sends the statement to a client and prints it.
	ToClient Kind = iota
	// Setup is the NUM_CLIENTS header, which is read before the loop and
	// skipped inside it.
	Setup
	// WaitReady, WaitBlocked and WaitUnblocked are the three states the corpus
	// waits for.
	WaitReady
	WaitBlocked
	WaitUnblocked
	// Sleep is MC: sleep <n>, in seconds.
	Sleep
	// DeadlockPause is MC: pause for deadlock resolution -- three seconds, so
	// that the server's deadlock detector can run.
	DeadlockPause
	// Retired is an MC command qactl implements and no case in the corpus uses.
	// ADR-019 drops them, and dropped means refused: the run stops naming the
	// command rather than doing something else quietly.
	Retired
	// Unknown is an MC command qactl would not have recognized either.
	Unknown
)

// Command is one statement, classified.
type Command struct {
	Kind   Kind
	Client int    // for ToClient and the three waits
	Text   string // for ToClient: the statement with its Cn: prefix removed
	Name   string // for Retired and Unknown: what was asked for
	Sleep  int    // for Sleep: seconds
}

// retired are the MC commands qactl.c implements that no .ctl file in the
// corpus uses (analysis/isolation/ctl-grammar.md §8a). The controller knows
// their names so that it can refuse them by name.
var retired = []string{
	"wait for",
	"reconnect",
	"rendezvous with",
	"execute",
	"allocate client",
	"no-op",
}

// Classify reads one statement the way control_clients does (qactl.c:2239).
// Matching is case-insensitive and the text is never modified: what the client
// receives is what the file held.
func Classify(stmt string) Command {
	s := skipWhite(stmt)
	if !strings.HasPrefix(strings.ToLower(s), "mc:") {
		return toClient(s)
	}
	body := skipWhite(s[3:])
	low := strings.ToLower(body)

	switch {
	case strings.HasPrefix(low, "setup"):
		return Command{Kind: Setup}

	case strings.HasPrefix(low, "pause for deadlock resolution;"):
		return Command{Kind: DeadlockPause}

	// "wait for" is checked before "wait until c" only because neither is a
	// prefix of the other; the order below follows qactl.c's.
	case strings.HasPrefix(low, "wait until c"):
		return waitCommand(body[len("wait until c"):])

	case strings.HasPrefix(low, "sleep"):
		n, ok := firstInt(skipWhite(body[len("sleep"):]))
		if !ok {
			return Command{Kind: Unknown, Name: body}
		}
		return Command{Kind: Sleep, Sleep: n}
	}

	for _, r := range retired {
		if strings.HasPrefix(low, r) {
			return Command{Kind: Retired, Name: r}
		}
	}
	return Command{Kind: Unknown, Name: body}
}

// waitCommand parses what follows "wait until c": a client number and then one
// of four subcommands (qactl.c:994). "finished" is one of qactl's four and no
// case uses it, so it is retired with the rest.
func waitCommand(rest string) Command {
	n, ok := firstInt(rest)
	if !ok {
		return Command{Kind: Unknown, Name: "wait until c" + rest}
	}
	for len(rest) > 0 && rest[0] >= '0' && rest[0] <= '9' {
		rest = rest[1:]
	}
	switch low := strings.ToLower(skipWhite(rest)); {
	case strings.HasPrefix(low, "blocked;"):
		return Command{Kind: WaitBlocked, Client: n}
	case strings.HasPrefix(low, "unblocked;"):
		return Command{Kind: WaitUnblocked, Client: n}
	case strings.HasPrefix(low, "ready;"):
		return Command{Kind: WaitReady, Client: n}
	case strings.HasPrefix(low, "finished;"):
		return Command{Kind: Retired, Name: "wait until c<n> finished"}
	default:
		return Command{Kind: Unknown, Name: "wait until c" + rest}
	}
}

// toClient decides which client a statement is for. qactl reads a number after
// the first character and requires a colon right after it (qactl.c:2460);
// everything else goes to client 1 -- including the second statement of a line,
// which lost its prefix when parse.c split on the semicolon. That is kept; the
// ToClient case in loop says what was tried instead and why it was reverted.
func toClient(s string) Command {
	if len(s) > 1 && (s[0] == 'C' || s[0] == 'c') {
		i := 1
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		if i > 1 && i < len(s) && s[i] == ':' {
			if n, err := strconv.Atoi(s[1:i]); err == nil && n > 0 {
				return Command{Kind: ToClient, Client: n, Text: skipWhite(s[i+1:])}
			}
		}
	}
	return Command{Kind: ToClient, Client: 1, Text: s}
}

// skipWhite is skip_white_comments as it behaves on a statement the reader has
// already been over: the comments are gone and the white space is single
// spaces, so there is only a leading space to drop.
func skipWhite(s string) string {
	return strings.TrimLeft(s, " \t\n\r\f\v")
}

// firstInt reads the leading decimal digits of s, as atoi does.
func firstInt(s string) (int, bool) {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 {
		return 0, false
	}
	n, err := strconv.Atoi(s[:i])
	if err != nil {
		return 0, false
	}
	return n, true
}

// NumClients reads the count out of the first statement, which client_init
// finds by looking for "num_clients = " in the statement lowercased
// (qactl.c:3209). A first statement without it means one client.
func NumClients(stmt string) int {
	low := strings.ToLower(stmt)
	i := strings.Index(low, "num_clients = ")
	if i < 0 {
		return 1
	}
	if n, ok := firstInt(skipWhite(stmt[i+len("num_clients = "):])); ok {
		return n
	}
	return 1
}
