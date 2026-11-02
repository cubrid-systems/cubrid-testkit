package haapply

import (
	"strings"
	"testing"
)

func TestEveryPatternIsComplete(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range Catalog {
		if seen[p.Name] {
			t.Errorf("%s: named twice", p.Name)
		}
		seen[p.Name] = true
		if p.Shares == "" || p.Setup == nil || p.Round == nil {
			t.Errorf("%s: incomplete", p.Name)
			continue
		}
		if len(tablesOf(p)) == 0 {
			t.Errorf("%s: compares no table", p.Name)
		}
		r := p.Round(p.Name, 3)
		if r.T2 == "" {
			t.Errorf("%s: round has no T2", p.Name)
		}
		if p.Name != "ctl" && r.T1 == "" {
			t.Errorf("%s: round has no T1", p.Name)
		}
		for _, s := range []string{p.Setup(p.Name), r.Pre, r.T1, r.T2} {
			if s != "" && !strings.HasSuffix(strings.TrimSpace(s), ";") {
				t.Errorf("%s: statement list does not end with a semicolon: %q", p.Name, s)
			}
		}
	}
	// The prototype's 24, the VARCHAR trailing-space keys of #8052 17aae152e,
	// and the seven of eb31453ff.
	if len(Catalog) != 33 {
		t.Errorf("catalog has %d patterns, want the prototype's 24, 2 trailing-space and 7 of eb31453ff", len(Catalog))
	}
}

// A pattern whose keys are one only under ignore_trailing_space is left out
// on a pair that keeps trailing spaces, and only there.
func TestTrailingSpacePatternsRunOnlyWhereTheKeysAreOne(t *testing.T) {
	all, _ := Select(nil)
	kept, left := ForPair(all, false)
	if len(kept) != len(all)-2 || strings.Join(left, ",") != "vc_trail,fk_vc_trail" {
		t.Errorf("no trailing space: %d kept, left %v", len(kept), left)
	}
	if kept[0].Name != "ctl" {
		t.Errorf("the control was not kept first: %s", kept[0].Name)
	}
	if kept, left := ForPair(all, true); len(kept) != len(all) || len(left) != 0 {
		t.Errorf("trailing space: %d kept, left %v", len(kept), left)
	}
}

func TestSelectPutsTheControlFirstAndRefusesUnknownNames(t *testing.T) {
	got, err := Select([]string{"fp_negzero", "pk_update"})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, p := range got {
		names = append(names, p.Name)
	}
	if strings.Join(names, " ") != "ctl fp_negzero pk_update" {
		t.Errorf("Select = %v", names)
	}
	if _, err := Select([]string{"pk_update", "no_such", "nor_this"}); err == nil ||
		!strings.Contains(err.Error(), "no_such") || !strings.Contains(err.Error(), "nor_this") {
		t.Errorf("unknown names should be refused by name, got %v", err)
	}
	all, err := Select(nil)
	if err != nil || len(all) != len(Catalog) || all[0].Name != "ctl" {
		t.Errorf("Select(nil) should be the whole catalog with ctl first, got %d %v", len(all), err)
	}
}

// One instant per round in two zones, as the prototype wrote it.
func TestTheTwoZonesNameOneInstant(t *testing.T) {
	if tz0(65) != "'2020-01-01 00:01:05 +00:00'" || tz9(65) != "'2020-01-01 09:01:05 +09:00'" {
		t.Errorf("tz0/tz9 = %s %s", tz0(65), tz9(65))
	}
}

// The two VARBIT values of a round differ only in zeros past the 17th bit, so
// in whole bytes they are one.
func TestTheTwoBitStringsAreOneInWholeBytes(t *testing.T) {
	for _, r := range []int{1, 999} {
		short, long := strings.Trim(vb17(r), "B'"), strings.Trim(vb24(r), "B'")
		if len(short) != 17 || len(long) != 24 || long != short+"0000000" {
			t.Errorf("round %d: %s, %s", r, vb17(r), vb24(r))
		}
	}
}
