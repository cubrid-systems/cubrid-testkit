package sqlsuite

import "testing"

func TestParseEngineReadsWhatRunShReads(t *testing.T) {
	out := "\nCUBRID 11.5.0 (11.5.0.2560-a7a1db8) (64bit release build for Linux) (Sep 11 2026 13:55:59)\n\n"
	e, err := parseEngine(out)
	if err != nil {
		t.Fatal(err)
	}
	if e.ver != "11.5.0.2560-a7a1db8" || e.bits != "64" || e.major != 11 || e.minor != 5 ||
		e.patch != "0" || e.build != "2560-a7a1db8" || e.prefix != "11.5.0" || e.debug {
		t.Errorf("got %+v", e)
	}
	if e.rel != "CUBRID 11.5.0 (11.5.0.2560-a7a1db8) (64bit release build for Linux) (Sep 11 2026 13:55:59)" {
		t.Errorf("rel is %q", e.rel)
	}
	if !e.atLeast(11, 5) || e.atLeast(11, 6) || !e.atLeast(10, 9) || e.atLeast(12, 0) {
		t.Error("atLeast does not compare the way run.sh does")
	}

	d, err := parseEngine("CUBRID 10.2.1 (10.2.1.8849-a1b2c3d) (32bit debug build for Linux) (Jan 1 2020 00:00:00)")
	if err != nil {
		t.Fatal(err)
	}
	if d.bits != "32" || !d.debug {
		t.Errorf("got %+v", d)
	}

	if _, err := parseEngine("cubrid_rel: command not found"); err == nil {
		t.Error("output without a CUBRID line was read as a version")
	}
}
