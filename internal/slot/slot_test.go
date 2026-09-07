package slot

import (
	"strings"
	"testing"
)

// The failure this package exists to prevent is two slots sharing a number, and
// it must not be reachable by arithmetic. Every allocation is checked for a
// repeat rather than spot-checked.
func TestNoTwoSlotsShareAnything(t *testing.T) {
	slots, err := Allocate(7, DefaultRange, "/tmp/root")
	if err != nil {
		t.Fatal(err)
	}
	seenPort := map[int]string{}
	seenSHM := map[int]string{}
	seenPath := map[string]string{}
	for _, s := range slots {
		for name, p := range map[string]int{
			"MasterPort": s.MasterPort, "Broker1Port": s.Broker1Port, "Broker2Port": s.Broker2Port,
		} {
			if prev, dup := seenPort[p]; dup {
				t.Errorf("port %d is both %s and %s", p, prev, s.EnvID()+"."+name)
			}
			seenPort[p] = s.EnvID() + "." + name
		}
		for name, id := range map[string]int{
			"MasterSHMID": s.MasterSHMID, "Broker1SHMID": s.Broker1SHMID, "Broker2SHMID": s.Broker2SHMID,
		} {
			if prev, dup := seenSHM[id]; dup {
				t.Errorf("shm id %d is both %s and %s", id, prev, s.EnvID()+"."+name)
			}
			seenSHM[id] = s.EnvID() + "." + name
		}
		for _, p := range []string{s.CUBRID, s.Databases} {
			if prev, dup := seenPath[p]; dup {
				t.Errorf("path %s is both %s and %s", p, prev, s.EnvID())
			}
			seenPath[p] = s.EnvID()
		}
	}
}

// A range too small is the one way slots could quietly overlap, so it is a
// refusal with the arithmetic in it rather than a wrap-around.
func TestARangeTooSmallIsRefused(t *testing.T) {
	_, err := Allocate(4, Range{Low: 15230, High: 15249}, "/tmp/root")
	if err == nil {
		t.Fatal("four slots were allocated out of a range that holds two")
	}
	for _, want := range []string{"parallel_slots=4", "15230-15249", "holds 2"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not say %q: %v", want, err)
		}
	}
}

// One slot is the default and has to be the behaviour there has always been:
// nothing about it should look like a special case.
func TestOneSlotIsOrdinary(t *testing.T) {
	slots, err := Allocate(1, DefaultRange, "/tmp/root")
	if err != nil {
		t.Fatal(err)
	}
	if len(slots) != 1 {
		t.Fatalf("asked for one slot, got %d", len(slots))
	}
	if got := slots[0].MasterPort; got != DefaultRange.Low {
		t.Errorf("the first slot starts at %d rather than at the bottom of the range", got)
	}
	if got := slots[0].EnvID(); got != "slot0" {
		t.Errorf("EnvID is %q", got)
	}
}

// Slot N is always the same slot, so a failure can be reproduced by running that
// one alone.
func TestAllocationIsStable(t *testing.T) {
	a, err := Allocate(5, DefaultRange, "/tmp/root")
	if err != nil {
		t.Fatal(err)
	}
	b, err := Allocate(3, DefaultRange, "/tmp/root")
	if err != nil {
		t.Fatal(err)
	}
	for i := range b {
		if a[i] != b[i] {
			t.Errorf("slot %d differs between a 5-slot and a 3-slot allocation:\n  %+v\n  %+v", i, a[i], b[i])
		}
	}
}

func TestParseRange(t *testing.T) {
	if r, err := ParseRange(""); err != nil || r != DefaultRange {
		t.Errorf("an empty range is not the default: %v %v", r, err)
	}
	if r, err := ParseRange(" 20000 - 20099 "); err != nil || r.Low != 20000 || r.High != 20099 {
		t.Errorf("spaces are not tolerated: %v %v", r, err)
	}
	for _, bad := range []string{"20000", "20099-20000", "0-10", "x-y", "1-70000"} {
		if _, err := ParseRange(bad); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}

// The parameters a slot contributes are keyed the way deploy.go already names
// the files and sections, so that the allocator decides the values and nothing
// between here and the file gets to invent one.
func TestParametersNameTheSectionsThatExist(t *testing.T) {
	slots, err := Allocate(2, DefaultRange, "/tmp/root")
	if err != nil {
		t.Fatal(err)
	}
	p := slots[1].Parameters()
	for _, want := range []string{
		"cubrid.conf/common",
		"cubrid_broker.conf/broker",
		"cubrid_broker.conf/%query_editor",
		"cubrid_broker.conf/%BROKER1",
	} {
		if _, ok := p[want]; !ok {
			t.Errorf("no parameters for %s", want)
		}
	}
	if got := p["cubrid.conf/common"]["cubrid_port_id"]; got != "15240" {
		t.Errorf("slot 1's master port is %q, not the second block of the range", got)
	}
	// MASTER_SHM_ID is the one whose collision does not fail but interferes.
	if p["cubrid_broker.conf/broker"]["MASTER_SHM_ID"] == "" {
		t.Error("MASTER_SHM_ID is not set, which is how two slots end up sharing a segment")
	}
}
