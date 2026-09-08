package status

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
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

// A run of 217 with 56 failures is a run where the list of failures is the
// thing being watched, so it has to be every failure -- not the failures among
// the last few, which answers a different question.
func TestEveryFailureIsKept(t *testing.T) {
	b := New(recentMax * 3)
	for i := 0; i < recentMax*3; i++ {
		name := "c" + string(rune('a'+i%26))
		b.Begin("slot0", name)
		b.End("slot0", name, i%3 != 0) // every third one fails
	}
	v := b.snapshot()
	if len(v.Recent) != recentMax {
		t.Errorf("the tail holds %d, not the %d it is capped at", len(v.Recent), recentMax)
	}
	want := 0
	for i := 0; i < recentMax*3; i++ {
		if i%3 == 0 {
			want++
		}
	}
	if len(v.Failed) != want {
		t.Errorf("the failure list holds %d of %d failures", len(v.Failed), want)
	}
	for _, f := range v.Failed {
		if f.OK {
			t.Error("a passing case is in the failure list")
		}
	}
	if got := len(b.snapshot().Failed); got != want {
		t.Errorf("reading the board changed the failure list: %d then %d", want, got)
	}
}

// The list is bounded too: a run failing a thousand cases is read in the result
// tree, not here.
func TestTheFailureListIsBounded(t *testing.T) {
	b := New(failedMax * 2)
	for i := 0; i < failedMax+50; i++ {
		b.Begin("slot0", "c")
		b.End("slot0", "c", false)
	}
	if got := len(b.snapshot().Failed); got != failedMax {
		t.Errorf("the failure list holds %d, not the %d it is capped at", got, failedMax)
	}
}

// The table refreshes once a second, so equal rows have to keep their places:
// sort.Slice is not stable, and without a second key a tie swaps on every poll.
func TestTiesDoNotMove(t *testing.T) {
	b := New(100)
	base := "/x/scenario/"
	// Three families with identical totals, and one that differs.
	for _, f := range []string{"_30_c", "_10_a", "_20_b"} {
		b.Begin("slot0", base+f+"/case/cases/x.sh")
		b.End("slot0", base+f+"/case/cases/x.sh", true)
	}
	first := b.snapshot().Family
	for i := 0; i < 20; i++ {
		got := b.snapshot().Family
		for j := range got {
			if got[j].Name != first[j].Name {
				t.Fatalf("row %d moved between polls: %q then %q", j, first[j].Name, got[j].Name)
			}
		}
	}
	// And the tie is broken by name, not by whatever the map iteration gave.
	var names []string
	for _, g := range first {
		names = append(names, g.Name)
	}
	if !sort.StringsAreSorted(names) {
		t.Errorf("tied rows are not in name order: %v", names)
	}
}

// A lane is where a slot's corpus writes go, and the panel has to add the run up
// by it: a lane on memory holding a small share of the case-seconds is memory
// spent where it does not pay, which is the whole reason to split the lanes.
func TestLanesAddTheRunUpByWhereTheWritesGo(t *testing.T) {
	b := New(4)
	b.Lane("slot0", "tmpfs")
	b.Lane("slot1", "tmpfs")
	b.Lane("slot2", "disk")

	run := func(slot, name string, secs int, ok bool) {
		b.Begin(slot, name)
		b.mu.Lock()
		in := b.running[slot]
		in.Since = time.Now().Add(-time.Duration(secs) * time.Second)
		b.running[slot] = in
		b.mu.Unlock()
		b.End(slot, name, ok)
	}
	run("slot0", "/x/_01_a/c/cases/c.sh", 5, true)
	run("slot1", "/x/_01_a/d/cases/d.sh", 5, false)
	run("slot2", "/x/_01_a/e/cases/e.sh", 90, true)

	v := b.snapshot()
	if len(v.Lanes) != 2 {
		t.Fatalf("two lanes were reported as %d: %+v", len(v.Lanes), v.Lanes)
	}
	// Largest share of the seconds first: the disk lane ran the 90-second case.
	if v.Lanes[0].Name != "disk" {
		t.Errorf("lanes are not ordered by share of time: %+v", v.Lanes)
	}
	if v.Lanes[0].NSlots != 1 || v.Lanes[1].NSlots != 2 {
		t.Errorf("slot counts are wrong: %+v", v.Lanes)
	}
	if v.Lanes[0].Slots != "slot2" || v.Lanes[1].Slots != "slot0-slot1" {
		t.Errorf("lanes do not name their slots: %q and %q", v.Lanes[0].Slots, v.Lanes[1].Slots)
	}
	if v.Lanes[1].Done != 2 || v.Lanes[1].NOK != 1 {
		t.Errorf("the tmpfs lane's cases were not counted: %+v", v.Lanes[1])
	}
	if v.Lanes[0].Share+v.Lanes[1].Share < 99 {
		t.Errorf("the shares do not add up: %+v", v.Lanes)
	}
	if v.Lanes[0].Share < 85 {
		t.Errorf("one 90s case against two 5s cases gave the disk lane %d%%", v.Lanes[0].Share)
	}
	// And the slot that is running says which lane it is in, because the
	// question asked of the rail is whether the stuck slot holds memory.
	b.Begin("slot0", "/x/_01_a/f/cases/f.sh")
	if s := b.snapshot().Slots; len(s) != 1 || s[0].Lane != "tmpfs" {
		t.Errorf("a running slot did not report its lane: %+v", s)
	}
}

// A slot with no lane is still a slot. The panel must leave it out rather than
// invent a name, because a serial run on disk reports one and a run of the old
// shape reports none.
func TestASlotWithNoLaneIsNotGivenOne(t *testing.T) {
	b := New(1)
	b.Lane("slot0", "")
	b.Begin("slot0", "/x/_01_a/c/cases/c.sh")
	b.End("slot0", "/x/_01_a/c/cases/c.sh", true)
	v := b.snapshot()
	if len(v.Lanes) != 0 {
		t.Errorf("a lane was invented: %+v", v.Lanes)
	}
	if v.Slots != nil && len(v.Slots) > 0 && v.Slots[0].Lane != "" {
		t.Errorf("a slot reported a lane it was not given: %+v", v.Slots)
	}
}

// Counting cases says the corpus is mostly short cases; counting seconds says a
// few long ones own the run. The lane threshold is chosen from the second, so
// both have to be there.
func TestTheDistributionIsWeightedByTimeAsWellAsCount(t *testing.T) {
	b := New(10)
	at := func(secs int, name string) {
		b.Begin("slot0", name)
		b.mu.Lock()
		in := b.running["slot0"]
		in.Since = time.Now().Add(-time.Duration(secs) * time.Second)
		b.running["slot0"] = in
		b.mu.Unlock()
		b.End("slot0", name, true)
	}
	for i := 0; i < 9; i++ {
		at(1, "/x/_01_a/s/cases/s.sh") // nine cases under 2s
	}
	at(183, "/x/_01_a/l/cases/l.sh") // one at 183s

	v := b.snapshot()
	if v.Hist[0] != 9 {
		t.Errorf("nine short cases landed as %v", v.Hist)
	}
	if got := v.HistSecs[0]; got != 9 {
		t.Errorf("nine one-second cases are %d seconds, not 9", got)
	}
	// 183 s lands in the 120-300 bucket, not the one above it: the edges are
	// upper bounds, so bucketOf(183) is the last edge it is under.
	long := bucketOf(183)
	if v.HistSecs[long] < 180 {
		t.Errorf("the 183s case contributed %d seconds to bucket %d: %v", v.HistSecs[long], long, v.HistSecs)
	}
	// The point: 10% of the cases are 95% of the time.
	total := 0
	for _, s := range v.HistSecs {
		total += s
	}
	if share := v.HistSecs[long] * 100 / total; share < 90 {
		t.Errorf("one case of ten holds %d%% of the seconds, expected over 90", share)
	}
}

// The lanes table refreshes once a second like every other, so tied rows have
// to keep their places.
func TestLaneTiesDoNotMove(t *testing.T) {
	b := New(4)
	for _, l := range []string{"zebra", "alpha", "middle"} {
		b.Lane("slot-"+l, l)
		b.Begin("slot-"+l, "/x/_01_a/c/cases/c.sh")
		b.End("slot-"+l, "/x/_01_a/c/cases/c.sh", true)
	}
	first := b.snapshot().Lanes
	for i := 0; i < 20; i++ {
		got := b.snapshot().Lanes
		for j := range got {
			if got[j].Name != first[j].Name {
				t.Fatalf("lane row %d moved: %q then %q", j, first[j].Name, got[j].Name)
			}
		}
	}
	if first[0].Name != "alpha" {
		t.Errorf("tied lanes are not in name order: %+v", first)
	}
}

// The page is one file with no build step, so the only thing that catches a
// panel wired to nothing is asking for it. Every id the script writes into has
// to exist in the markup, and every field it reads has to be one the API sends.
func TestThePageIsWiredToWhatTheAPISends(t *testing.T) {
	b := New(1)
	addr, stop, err := b.Serve("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	res, err := http.Get("http://" + addr + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	page := string(body)

	for _, id := range []string{"lanes", "hist", "family", "slot", "slots", "machine", "recent",
		"tplpanel", "tplkv", "tpltop", "tplwhere"} {
		if !strings.Contains(page, "id="+id) {
			t.Errorf("the script writes into %q but the markup has no such element", id)
		}
	}
	// The lane fields the renderer reads, against the JSON tags the API emits.
	raw, err := json.Marshal(b.snapshot())
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"lanes"`, `"histSecs"`} {
		if !strings.Contains(string(raw), key) {
			t.Errorf("the page reads %s and the API does not send it", key)
		}
	}
	if !strings.Contains(page, "v.lanes") || !strings.Contains(page, "v.histSecs") {
		t.Error("the API sends lanes and histSecs and the page does not read them")
	}
}

// A slot has to appear in the by-slot table from the moment the run opens it.
// Building the table only from cases that had finished hid exactly the slots
// worth looking at: the slow lane's slots were on 195, 166 and 125-second
// cases, so the panel showed five of eight for the first three minutes.
func TestEverySlotIsInTheTableBeforeItFinishesAnything(t *testing.T) {
	b := New(10)
	for _, s := range []string{"slot0", "slot1"} {
		b.Lane(s, "tmpfs")
	}
	b.Lane("slot2", "disk")
	b.Begin("slot2", "/x/_15_backupdb/c/cases/c.sh") // a long case, nothing finished

	v := b.snapshot()
	if len(v.Slot) != 3 {
		t.Fatalf("the by-slot table has %d rows for three open slots: %+v", len(v.Slot), v.Slot)
	}
	// In slot order, so the lanes read as contiguous blocks and a row does not
	// move as it works.
	for i, want := range []string{"slot0", "slot1", "slot2"} {
		if v.Slot[i].Name != want {
			t.Errorf("row %d is %q, want %q", i, v.Slot[i].Name, want)
		}
	}
	// And each row says which lane it is in.
	if v.Slot[0].Lane != "tmpfs" || v.Slot[2].Lane != "disk" {
		t.Errorf("the by-slot rows do not carry their lane: %+v", v.Slot)
	}
	if v.Slot[2].Done != 0 {
		t.Errorf("a slot that finished nothing reports %d done", v.Slot[2].Done)
	}
}

// The lanes panel has to say which slots, not how many: the question it is read
// for is which lane the stuck slot is in.
func TestALaneNamesItsSlots(t *testing.T) {
	b := New(10)
	for _, s := range []string{"slot0", "slot1", "slot2", "slot3", "slot4"} {
		b.Lane(s, "tmpfs")
	}
	for _, s := range []string{"slot5", "slot6", "slot7"} {
		b.Lane(s, "disk")
	}
	b.Begin("slot0", "/x/_01_a/c/cases/c.sh")
	b.End("slot0", "/x/_01_a/c/cases/c.sh", true)

	byName := map[string]laneView{}
	for _, l := range b.snapshot().Lanes {
		byName[l.Name] = l
	}
	if got := byName["tmpfs"].Slots; got != "slot0-slot4" {
		t.Errorf("the tmpfs lane names its slots as %q, want slot0-slot4", got)
	}
	if got := byName["disk"].Slots; got != "slot5-slot7" {
		t.Errorf("the disk lane names its slots as %q, want slot5-slot7", got)
	}
	if byName["disk"].NSlots != 3 {
		t.Errorf("the disk lane counts %d slots, want 3", byName["disk"].NSlots)
	}
}

func TestCompactSlots(t *testing.T) {
	for _, c := range []struct {
		in   []string
		want string
	}{
		{nil, ""},
		{[]string{"slot0"}, "slot0"},
		{[]string{"slot0", "slot1", "slot2"}, "slot0-slot2"},
		{[]string{"slot2", "slot0", "slot1"}, "slot0-slot2"},
		{[]string{"slot0", "slot2"}, "slot0, slot2"},
		{[]string{"slot0", "slot1", "slot5", "slot6"}, "slot0-slot1, slot5-slot6"},
		// Ten before two is how strings sort and not how anyone reads slots.
		{[]string{"slot10", "slot2", "slot9"}, "slot2, slot9-slot10"},
		{[]string{"a", "b"}, "a, b"},
	} {
		if got := compactSlots(c.in); got != c.want {
			t.Errorf("compactSlots(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// The template cache lives on the shell side and leaves everything needed to
// describe itself in its own store, so the page reads the store and the two
// cannot disagree.
func TestTheTemplateCachePanelReadsTheStore(t *testing.T) {
	store := t.TempDir()
	mk := func(key string, refs int, origin string, bytes int) {
		d := filepath.Join(store, key)
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(d, ".refs"), []byte(strconv.Itoa(refs)+"\n"), 0o644)
		os.WriteFile(filepath.Join(d, ".origin"), []byte(origin+"\n"), 0o644)
		os.WriteFile(filepath.Join(d, "db"), make([]byte, bytes), 0o644)
	}
	mk("aaaa1111", 7, "/x/scenario/_06_createdb/itrack_10001/cases", 3<<20)
	mk("bbbb2222", 2, "/x/scenario/_17_loaddb/itrack_10007/cases", 1<<20)
	// The store's own bookkeeping must not be counted as a template.
	os.WriteFile(filepath.Join(store, ".plan"), []byte("aaaa1111\n"), 0o644)
	os.WriteFile(filepath.Join(store, ".lk.aaaa1111"), nil, 0o644)
	os.WriteFile(filepath.Join(store, ".used.123"), []byte("aaaa1111\nbbbb2222\naaaa1111\n"), 0o644)

	b := New(1)
	b.WatchTemplates(store, 1024)
	// WatchTemplates samples once before its ticker, but that happens in a
	// goroutine, so wait for it rather than racing it.
	var v *templateView
	for i := 0; i < 100 && (v == nil || v.Count == 0); i++ {
		v = b.snapshot().Templates
		time.Sleep(10 * time.Millisecond)
	}
	if v == nil {
		t.Fatal("the panel is absent for a run that has a cache")
	}
	if v.Count != 2 {
		t.Errorf("counted %d templates, want 2 -- the store's own dot files are not templates", v.Count)
	}
	if v.CapMB != 1024 || v.Dir != store {
		t.Errorf("the panel does not name the store it read: %+v", v)
	}
	// Three lines in .used.123 are three restores this run.
	if v.Restored != 3 {
		t.Errorf("counted %d restores, want 3", v.Restored)
	}
	if v.MB < 3 {
		t.Errorf("the store measures %d MB, and 4 MB of files were written", v.MB)
	}
	if len(v.Top) != 2 || v.Top[0].Key != "aaaa1111" || v.Top[0].Refs != 7 {
		t.Errorf("most-used first is not what came back: %+v", v.Top)
	}
	// The origin is named the way the rest of the page names cases.
	if v.Top[0].Origin != "_06_createdb/itrack_10001" {
		t.Errorf("origin is %q, want the family and the case", v.Top[0].Origin)
	}
}

// A run without a cache gets no panel at all, rather than a panel of zeroes
// that reads as "nothing is hitting".
func TestNoCacheMeansNoPanel(t *testing.T) {
	b := New(1)
	b.WatchTemplates("", 0)
	if v := b.snapshot().Templates; v != nil {
		t.Errorf("a run with no cache got a panel: %+v", v)
	}
}

// Two runs on one machine is a normal thing to want, and the page is not worth
// failing a run over.
func TestTheDefaultPortMovesAlong(t *testing.T) {
	a := New(1)
	addr, stop, err := a.Serve(DefaultAddr)
	if err != nil {
		t.Skip("the default port is not available on this machine")
	}
	defer stop()
	if addr != DefaultAddr {
		t.Fatalf("listened on %q, want the default", addr)
	}
	// A second board finds it taken and takes the next one.
	b := New(1)
	var got string
	var stop2 func()
	for try := 1; try <= 16; try++ {
		got, stop2, err = b.Serve(NearDefault(try))
		if err == nil {
			break
		}
	}
	if err != nil {
		t.Fatalf("no port near the default was free: %v", err)
	}
	defer stop2()
	if got == DefaultAddr || got == "" {
		t.Errorf("the second run took %q", got)
	}
}

// A failing case provokes one question -- why -- and feedback.log already has the
// answer, so the page has to be able to find it.
func TestClickingAFinishedCaseFindsItsBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "feedback.log")
	os.WriteFile(path, []byte(
		"[OK]:  /x/a/cases/a.sh 100 EnvId=local[slot0]\n"+
			"a-1 : OK\n"+
			"[NOK]: TRY-> = 0 /x/b/cases/b.sh 200 EnvId=local[slot1]\n"+
			"b-1 : NOK it did not work\n"+
			"===== CONSOLE OUTPUT =====\n"+
			"+ some trace\n"+
			"[OK]:  /x/c/cases/c.sh 300 EnvId=local[slot0]\n"+
			"c-1 : OK\n"), 0o644)

	b := New(3)
	b.Detail(path)
	addr, stop, err := b.Serve("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	get := func(name string) string {
		res, err := http.Get("http://" + addr + "/case?name=" + url.QueryEscape(name))
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		body, _ := io.ReadAll(res.Body)
		return string(body)
	}

	got := get("/x/b/cases/b.sh")
	for _, want := range []string{"NOK it did not work", "CONSOLE OUTPUT", "+ some trace"} {
		if !strings.Contains(got, want) {
			t.Errorf("the failing case's block is missing %q:\n%s", want, got)
		}
	}
	// And it stops at the next case rather than running on.
	if strings.Contains(got, "c-1 : OK") {
		t.Errorf("the block ran into the next case:\n%s", got)
	}
	// A case with no block says so rather than 404ing.
	if s := get("/x/nope/cases/nope.sh"); !strings.Contains(s, "nothing recorded") {
		t.Errorf("an unknown case gave %q", s)
	}
	// And a board nobody told still serves the page.
	plain := New(1)
	addr2, stop2, _ := plain.Serve("127.0.0.1:0")
	defer stop2()
	res, err := http.Get("http://" + addr2 + "/case?name=/x/a/cases/a.sh")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("a board with no feedback.log answered %d", res.StatusCode)
	}
}

// Everything a parallel run has been wrong about turned out to be a resource
// question, and none of it was visible while a run was going. The panel is only
// worth having if the numbers are real.
func TestTheMachinePanelReportsRealNumbers(t *testing.T) {
	b := New(1)
	b.Watch(t.TempDir(), "", 0)
	// CPU and disk are counters: the first reading seeds them and the second
	// produces a rate, so wait for one tick.
	var m machineView
	for i := 0; i < 60; i++ {
		m = b.snapshot().Machine
		if m.MemAll > 0 && m.CPUIdle+m.CPUUser+m.CPUSys > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if m.Cores < 1 {
		t.Error("no core count")
	}
	if m.MemAll <= 0 || m.MemUsed <= 0 {
		t.Errorf("memory came back as %d used of %d", m.MemUsed, m.MemAll)
	}
	// The four CPU states are shares of one whole.
	total := m.CPUUser + m.CPUSys + m.CPUIdle + m.CPUIOWait + m.CPUSteal
	if total < 95 || total > 105 {
		t.Errorf("the cpu states add to %.1f%%, not 100", total)
	}
	if m.Load1 < 0 || m.Load15 < 0 {
		t.Errorf("load came back as %.2f/%.2f", m.Load1, m.Load15)
	}
	// Disk is a rate, so zero is a legitimate answer on an idle machine -- but
	// it must not be negative, which is what a counter wrapping backwards gives.
	if m.ReadMBs < 0 || m.WriteMBs < 0 || m.ReadIOPS < 0 || m.WritIOPS < 0 {
		t.Errorf("a negative disk rate: %+v", m)
	}
}
