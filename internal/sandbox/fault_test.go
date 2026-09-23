package sandbox

import (
	"context"
	"strings"
	"testing"
	"time"
)

// The envelopes below are what csb 4068102 actually printed for these verbs on
// a rootless podman pair, copied from the run rather than imagined. The stand-in
// checks what this package sends and how it reads an answer; that csb still
// answers this way is what the live test beside it is for.

func TestPartitionSendsTheSelectorAndReadsWhatWasCut(t *testing.T) {
	bin, log := fakeCSB(t, `echo '{"schema":"csb/v1","command":"fault partition","ok":true,`+
		`"data":{"cut":["gbha-n1"],"mechanism":"blackhole","target":"gbha-n2"},"notes":[]}'`)
	f, err := (&CLI{Bin: bin, Cluster: "gbha"}).Partition(context.Background(), "slave", "blackhole")
	if err != nil {
		t.Fatal(err)
	}
	got := argv(t, log)
	want := []string{"fault", "partition", "--cluster", "gbha", "--json", "slave", "--mechanism", "blackhole"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("sent %v, want %v", got, want)
	}
	// The target is the node that was cut off and the cut is who can no longer
	// reach it. Reporting one as the other describes the wrong fault.
	if f.Target != "gbha-n2" || len(f.Cut) != 1 || f.Cut[0] != "gbha-n1" {
		t.Errorf("target %q cut %v", f.Target, f.Cut)
	}
	if f.Kind != "partition" || f.Mechanism != "blackhole" {
		t.Errorf("kind %q mechanism %q", f.Kind, f.Mechanism)
	}
}

// An empty mechanism is csb's default and not this package's, so nothing is
// sent: P3 needs both mechanisms expressible, and a wrapper that picked one
// would decide for every case that did not say.
func TestPartitionWithoutAMechanismSendsNoFlag(t *testing.T) {
	bin, log := fakeCSB(t, `echo '{"schema":"csb/v1","ok":true,`+
		`"data":{"cut":["n1"],"mechanism":"blackhole","target":"n2"},"notes":[]}'`)
	if _, err := (&CLI{Bin: bin, Cluster: "gbha"}).Partition(context.Background(), "slave", ""); err != nil {
		t.Fatal(err)
	}
	for _, a := range argv(t, log) {
		if a == "--mechanism" {
			t.Fatalf("sent a mechanism nobody asked for: %v", argv(t, log))
		}
	}
}

func TestSplitBrainKeepsTheEnginesOwnSentence(t *testing.T) {
	bin, log := fakeCSB(t, `echo '{"schema":"csb/v1","command":"fault splitbrain","ok":true,"data":{`+
		`"cancel_reason":"[Failback] [Cancelled] Ping check succeeded for the hosts registered in ha_ping_hosts, determining that it is not a network partition.",`+
		`"flavour":"ping-survives","masters":2,"partitioned":"gbha-n1"},"notes":[]}'`)
	sb, err := (&CLI{Bin: bin, Cluster: "gbha"}).SplitBrain(context.Background(), "", 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if sb.Masters != 2 || sb.Flavour != "ping-survives" || sb.Partitioned != "gbha-n1" {
		t.Errorf("%+v", sb)
	}
	if !strings.Contains(sb.CancelReason, "ha_ping_hosts") {
		t.Errorf("the engine's reason was not kept: %q", sb.CancelReason)
	}
	sent := strings.Join(argv(t, log), " ")
	if !strings.Contains(sent, "--wait 30s") {
		t.Errorf("the wait was not passed on: %s", sent)
	}
	if strings.Contains(sent, "--flavour") {
		t.Errorf("sent a flavour nobody asked for: %s", sent)
	}
}

// P4 needs the state unambiguous in both directions: a call that asked for two
// masters and got one has not produced a split brain, and saying so is the
// difference between a test that detects the state and one that assumes it.
func TestSplitBrainThatDidNotHappenIsAnError(t *testing.T) {
	bin, _ := fakeCSB(t, `echo '{"schema":"csb/v1","ok":true,`+
		`"data":{"flavour":"ping-survives","masters":1,"partitioned":"n1"},"notes":[]}'`)
	sb, err := (&CLI{Bin: bin, Cluster: "gbha"}).SplitBrain(context.Background(), "", 0)
	if err == nil {
		t.Fatal("one master was accepted as a split brain")
	}
	// The answer still comes back: what the cluster did reach is what a caller
	// has to report.
	if sb.Masters != 1 {
		t.Errorf("the reading was thrown away: %+v", sb)
	}
}

func TestClearReturnsWhatItTookOff(t *testing.T) {
	bin, log := fakeCSB(t, `echo '{"schema":"csb/v1","command":"fault clear","ok":true,"data":{"cleared":[`+
		`{"kind":"partition","target":"gbha-n2","mechanism":"blackhole","cut":["gbha-n1"],"since":"2026-09-22T10:22:12Z"}`+
		`]},"notes":[]}'`)
	cleared, err := (&CLI{Bin: bin, Cluster: "gbha"}).ClearFaults(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(cleared) != 1 || cleared[0].Kind != "partition" || cleared[0].Target != "gbha-n2" {
		t.Fatalf("%+v", cleared)
	}
	if cleared[0].Since.IsZero() {
		t.Error("the time the condition began was dropped")
	}
	for _, a := range argv(t, log) {
		if a == "" {
			t.Fatalf("an empty selector was sent as an argument: %v", argv(t, log))
		}
	}
}

func TestFaultsListsWhatIsInForce(t *testing.T) {
	bin, _ := fakeCSB(t, `echo '{"schema":"csb/v1","command":"fault ls","ok":true,"data":{"faults":[`+
		`{"kind":"partition","target":"gbha-n2","mechanism":"blackhole","cut":["gbha-n1"],"since":"2026-09-22T10:22:12Z"}`+
		`]},"notes":[]}'`)
	fs, err := (&CLI{Bin: bin, Cluster: "gbha"}).Faults(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 1 {
		t.Fatalf("%+v", fs)
	}
	if got := fs[0].String(); !strings.Contains(got, "gbha-n2") || !strings.Contains(got, "gbha-n1") {
		t.Errorf("the line names only half the fault: %q", got)
	}
}
