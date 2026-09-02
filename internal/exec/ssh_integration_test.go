package exec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These run against a real sshd. Set TESTKIT_SSH_HOST (and, if they differ from
// the defaults, TESTKIT_SSH_PORT / TESTKIT_SSH_USER / TESTKIT_SSH_PASSWORD) to
// enable them; without a password they need an ssh-agent holding an authorised
// key, which is what SSH_AUTH_SOCK points at.
//
// Everything else in this package can be checked without a server. The framing
// cannot: what it defends against is a shell that prints the script back, and no
// unit test can produce one.
func integrationChannel(t *testing.T) *SSH {
	t.Helper()
	host := os.Getenv("TESTKIT_SSH_HOST")
	if host == "" {
		t.Skip("set TESTKIT_SSH_HOST to run against a real sshd")
	}
	cfg := SSHConfig{
		Host:     host,
		Port:     envOr("TESTKIT_SSH_PORT", "22"),
		User:     envOr("TESTKIT_SSH_USER", os.Getenv("USER")),
		Password: os.Getenv("TESTKIT_SSH_PASSWORD"),
	}
	ch := NewSSH(cfg)
	t.Cleanup(func() { ch.Close() })
	return ch
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func TestSSHRunsAScriptAndReturnsWhatItPrinted(t *testing.T) {
	ch := integrationChannel(t)

	res, err := ch.Run(t.Context(), "echo hello\necho world")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(res.Stdout); got != "hello\nworld" {
		t.Errorf("got %q, want \"hello\\nworld\"", got)
	}
	if res.ExitCode != 0 {
		t.Errorf("exit code %d, want 0", res.ExitCode)
	}
}

// The reason the markers are written as ALL_${NOTEXIST}STARTED rather than
// ALL_STARTED: a shell tracing its input prints the script back, and a literal
// marker in the script would open the frame before the script had run.
//
// With set -x, "+ echo ALL_STARTED" appears on stderr and the frame is read from
// stdout, so this passes either way on a well-behaved sshd -- but a shell that
// merges them, or a profile that echoes commands, does not. This test runs the
// hostile case for real.
func TestTheFrameSurvivesAShellThatEchoesItsInput(t *testing.T) {
	ch := integrationChannel(t)

	res, err := ch.Run(t.Context(), "set -x\necho payload\nset +x")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Stdout, "payload") {
		t.Errorf("the payload is missing from:\n%q", res.Stdout)
	}
	if strings.Contains(res.Stdout, startFlagLiteral) || strings.Contains(res.Stdout, compFlagLiteral) {
		t.Errorf("a marker leaked into the output:\n%q", res.Stdout)
	}
}

// Anything a login prints before the script starts -- a banner, a motd, a
// profile's own chatter -- lies outside the frame and must not reach the caller.
func TestOutputBeforeTheFrameIsDropped(t *testing.T) {
	ch := integrationChannel(t)

	res, err := ch.Run(t.Context(), "echo inside")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(res.Stdout) != "inside" {
		t.Errorf("got %q, want just the script's own output", res.Stdout)
	}
}

// A case that fails is data, not a malfunction. The exit code comes back in the
// Result and the error stays nil, or every failing test case would look like a
// broken connection.
func TestANonZeroExitIsNotAnError(t *testing.T) {
	ch := integrationChannel(t)

	res, err := ch.Run(t.Context(), "echo before\nexit 3")
	if err != nil {
		t.Fatalf("a failing script was reported as a channel failure: %v", err)
	}
	if res.ExitCode != 3 {
		t.Errorf("exit code %d, want 3", res.ExitCode)
	}
	if !strings.Contains(res.Stdout, "before") {
		t.Errorf("output before the failure was lost: %q", res.Stdout)
	}
}

// A case is free to print anything, including the marker text. The frame is
// opened by the first occurrence and closed by the first one after it, so a case
// that prints ALL_COMPLETED truncates its own output -- which is CTP's behaviour
// and is recorded here rather than fixed, because fixing it would change what a
// case's output means.
func TestACaseThatPrintsTheMarkerTruncatesItself(t *testing.T) {
	ch := integrationChannel(t)

	res, err := ch.Run(t.Context(), "echo kept\necho ALL_COMPLETED\necho lost")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Stdout, "kept") {
		t.Errorf("output before the marker was lost: %q", res.Stdout)
	}
	if strings.Contains(res.Stdout, "lost") {
		t.Errorf("output after the marker survived, so the frame no longer closes there: %q", res.Stdout)
	}
}

func TestPutAndGetRoundTrip(t *testing.T) {
	ch := integrationChannel(t)
	dir := t.TempDir()

	// Content chosen to break a naive shell quoting: quotes, a backslash, a
	// dollar sign, a backtick and a line that looks like a marker.
	const body = "line one\n'single' \"double\" \\backslash $dollar `backtick`\nALL_STARTED\nlast\n"
	local := filepath.Join(dir, "payload.txt")
	if err := os.WriteFile(local, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	remote := filepath.Join(dir, "remote-copy.txt")
	if err := ch.Put(t.Context(), local, remote); err != nil {
		t.Fatal(err)
	}
	back := filepath.Join(dir, "round-trip.txt")
	if err := ch.Get(t.Context(), remote, back); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(back)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != body {
		t.Errorf("round trip changed the file:\n%q\nwant\n%q", got, body)
	}
}

// A worker holds one connection for the whole run, so the second command must
// not pay to reconnect -- and after Close the next one must reconnect rather
// than fail.
func TestTheConnectionIsReusedAndReopened(t *testing.T) {
	ch := integrationChannel(t)

	for i := range 3 {
		if _, err := ch.Run(t.Context(), "echo reuse"); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	if err := ch.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := ch.Run(t.Context(), "echo reopened"); err != nil {
		t.Fatalf("the channel did not reconnect after Close: %v", err)
	}
}

// The scripts the shell task actually sends are multi-line, quote-heavy and full
// of shell substitution. This is the real RunScript shape.
func TestARealisticCaseScriptSurvivesTheTrip(t *testing.T) {
	ch := integrationChannel(t)
	dir := t.TempDir()

	script := strings.Join([]string{
		"cd " + dir,
		"ulimit -c unlimited",
		`if [ "$JAVA_HOME_64BITS" ]; then`,
		"        export JAVA_HOME=$JAVA_HOME_64BITS",
		"fi",
		"export TEST_BIG_SPACE=$(echo $TEST_BIG_SPACE)",
		"export TEST_SSH_HOST=`hostname -i`",
		"export TEST_BUIILD_ID=11.4.5.1875-74d17e9",
		"echo > probe.result",
		`echo " : OK $TEST_BUIILD_ID" >> probe.result`,
		"cat probe.result",
	}, "\n")

	res, err := ch.Run(t.Context(), script)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Stdout, " : OK 11.4.5.1875-74d17e9") {
		t.Errorf("the case script did not produce its result line:\n%q", res.Stdout)
	}
}
