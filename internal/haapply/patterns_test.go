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
	if len(Catalog) != 24 {
		t.Errorf("catalog has %d patterns, the prototype had 24", len(Catalog))
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
