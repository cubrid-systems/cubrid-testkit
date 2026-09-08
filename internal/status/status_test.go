package status

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// A run must not fail because nobody asked to watch it, and the runner passes
// nil when no port was named. Every method has to survive that without the call
// sites checking.
func TestANilBoardIsUsable(t *testing.T) {
	var b *Board
	b.Begin("slot0", "a")
	b.End("slot0", "a", true)
	addr, stop, err := b.Serve(":0")
	if err != nil || addr != "" {
		t.Fatalf("a nil board tried to listen: %q %v", addr, err)
	}
	stop()
}

func get(t *testing.T, base string) view {
	t.Helper()
	res, err := http.Get(base + "/api")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var v view
	if err := json.NewDecoder(res.Body).Decode(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

// The question a run of ten slots cannot answer from its log is which slot is
// stuck and on what, so that is what this has to report.
func TestItSaysWhichSlotIsOnWhat(t *testing.T) {
	b := New(3)
	addr, stop, err := b.Serve("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	base := "http://" + addr

	b.Begin("slot0", "/x/scenario/_01_utility/_01_sqlx/a/cases/a.sh")
	b.Begin("slot1", "/x/scenario/_01_utility/_02_csql/b/cases/b.sh")

	v := get(t, base)
	if len(v.Slots) != 2 {
		t.Fatalf("two slots are running, the page says %d", len(v.Slots))
	}
	if v.Slots[0].Slot != "slot0" || v.Slots[1].Slot != "slot1" {
		t.Errorf("slots are not in a stable order: %+v", v.Slots)
	}
	if !strings.HasSuffix(v.Slots[0].Case, "a.sh") {
		t.Errorf("slot0 is not reported as running its case: %q", v.Slots[0].Case)
	}

	b.End("slot0", "/x/scenario/_01_utility/_01_sqlx/a/cases/a.sh", false)
	v = get(t, base)
	if v.Done != 1 || v.NOK != 1 || v.OK != 0 {
		t.Errorf("a failed case is counted as done=%d ok=%d nok=%d", v.Done, v.OK, v.NOK)
	}
	if len(v.Slots) != 1 || v.Slots[0].Slot != "slot1" {
		t.Errorf("a finished slot is still shown as running: %+v", v.Slots)
	}
	if len(v.Recent) != 1 || v.Recent[0].OK {
		t.Errorf("the finished case is not in the history as a failure: %+v", v.Recent)
	}
	if v.Finished {
		t.Error("the run is reported finished with a case still running")
	}
}

// The history is bounded: a corpus of 3,452 cases must not be held in memory
// twice over so that a page can show the last few.
func TestTheHistoryIsBounded(t *testing.T) {
	b := New(1000)
	for i := 0; i < recentMax*3; i++ {
		b.Begin("slot0", "c")
		b.End("slot0", "c", true)
	}
	if got := len(b.snapshot().Recent); got != recentMax {
		t.Errorf("the history holds %d entries, not the %d it is capped at", got, recentMax)
	}
}

// A port already in use is an error the caller sees, not a page that silently
// never appears.
func TestAPortInUseIsAnError(t *testing.T) {
	a := New(1)
	addr, stop, err := a.Serve("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	if _, _, err := New(1).Serve(addr); err == nil {
		t.Error("a second board took a port that was already listening")
	}
}

// 8080 is what everything else on a developer's machine is already using, and
// a bare port or a plain "on" has to reach somewhere it will not collide.
func TestAddr(t *testing.T) {
	for in, want := range map[string]string{
		"":               "",
		"  ":             "",
		"on":             DefaultAddr,
		"YES":            DefaultAddr,
		"1":              DefaultAddr,
		"9999":           ":9999",
		":9999":          ":9999",
		"127.0.0.1:9999": "127.0.0.1:9999",
		"0.0.0.0:80":     "0.0.0.0:80",
	} {
		if got := Addr(in); got != want {
			t.Errorf("Addr(%q) = %q, want %q", in, got, want)
		}
	}
	// Loopback by default: turning the page on should not publish a run to the
	// network.
	if !strings.HasPrefix(DefaultAddr, "127.0.0.1:") {
		t.Errorf("the default address is not loopback: %q", DefaultAddr)
	}
}
