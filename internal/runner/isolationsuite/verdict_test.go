package isolationsuite

import (
	"slices"
	"strings"
	"testing"
)

// The shapes below are runone.sh's own, cut down to the lines the rule reads: the
// retry loop's `set -x` trace, and what `cat .test.log` prints after an attempt
// that did not pass (runone.sh:395-407).
const (
	attemptPassed = "Testing /s/a/b.ctl (retry count: 0)\n" +
		"+ grep 'flag: OK' .test.log\n" +
		"flag: OK\n" +
		"+ break\n"
	attemptFailed = "Testing /s/a/b.ctl (retry count: 0)\n" +
		"+ grep 'flag: OK' .test.log\n" +
		"+ cat .test.log\n" +
		"Testing /s/a/b.ctl (retry count: 0)\n" +
		"elapse: 408\n" +
		"flag: NOK \n"
)

func TestAPassIsAnOKThatNothingFollows(t *testing.T) {
	v := judge(attemptPassed)
	if !v.ok || v.hasCore || len(v.items) != 0 {
		t.Errorf("verdict = %+v, want a pass with no items", v)
	}
}

// The trace of `grep 'flag: OK'` puts "flag: OK" into the output of every attempt,
// passed or not. A failed attempt fails because cat prints its NOK after it.
func TestAFailureIsTheNOKAfterTheTracedGrep(t *testing.T) {
	v := judge(attemptFailed + attemptFailed)
	if v.ok {
		t.Fatal("two failed attempts passed")
	}
	if len(v.items) != 0 {
		t.Errorf("items = %q; a NOK from runone.sh adds none, the diff is what explains it", v.items)
	}
}

func TestARetryThatPassesIsAPass(t *testing.T) {
	if v := judge(attemptFailed + attemptPassed); !v.ok {
		t.Errorf("a case that passed on its second attempt failed: %+v", v)
	}
}

// A core fails the case whatever came after it, and the trace repeats the message
// in quotes -- which is why the quotes are removed before items are compared.
func TestACoreFailsTheCaseEvenAfterAnOK(t *testing.T) {
	out := "flag: NOK found core file on host 127.0.0.1(/root/error_backup/e)\n" +
		"+ out='found core file on host 127.0.0.1(/root/error_backup/e)'\n" +
		attemptPassed
	v := judge(out)
	if v.ok || !v.hasCore {
		t.Fatalf("verdict = %+v, want a failure with a core", v)
	}
	want := []string{" : NOK found core file on host 127.0.0.1(/root/error_backup/e)"}
	if !slices.Equal(v.items, want) {
		t.Errorf("items = %q, want %q", v.items, want)
	}
}

func TestNoOKAtAllIsAFailureThatSaysSo(t *testing.T) {
	v := judge("runone.sh: line 265: qactl: No such file or directory\n")
	if v.ok {
		t.Fatal("output with no verdict in it passed")
	}
	if want := []string{" : NOK Not found OK word."}; !slices.Equal(v.items, want) {
		t.Errorf("items = %q, want %q", v.items, want)
	}
}

func TestTheResultTextIsItemsThenTheDiff(t *testing.T) {
	if got := resultText(verdict{ok: true}, ""); got != "" {
		t.Errorf("a pass with nothing to say produced %q", got)
	}
	v := verdict{items: []string{" : NOK Not found OK word."}}
	got := resultText(v, "| a\t| a\r\n")
	want := " : NOK Not found OK word.\n" + diffBanner + "\n| a\t| a\r\n\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if n := len(strings.Trim(diffBanner, "= ")); diffBanner[:67] != strings.Repeat("=", 67) || n != len("D I F F") {
		t.Errorf("the banner is not CTP's: %q", diffBanner)
	}
}

func TestTheDiffGetsWindowsLineEndingsOnce(t *testing.T) {
	if got := crlf("a\nb\n"); got != "a\r\nb\r\n" {
		t.Errorf("got %q", got)
	}
	if got := crlf("a\r\nb\n"); got != "a\r\nb\n" {
		t.Errorf("a diff that already has one was changed: %q", got)
	}
}
