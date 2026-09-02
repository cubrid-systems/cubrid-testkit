package shellsuite

import "testing"

func TestBuildID(t *testing.T) {
	cases := []struct {
		in   string
		want string
		why  string
	}{
		{
			"http://build/11.4.0.1234/CUBRID-11.4.0.1234-6a1b2c3-Linux.x86_64.sh",
			"11.4.0.1234-6a1b2c3",
			"the new numbering carries a commit suffix, and it is part of the identity",
		},
		{
			"http://build/9.3.0.0206/CUBRID-9.3.0.0206-Linux.x86_64.sh",
			"9.3.0.0206",
			"before 10.1.0.6858 the four-part number is the whole id",
		},
		{
			"Test_Build_URL=http://build/10.2.0.8797/CUBRID-10.2.0.8797-abcdef-Linux.x86_64.sh",
			"10.2.0.8797-abcdef",
			"read out of qa.conf rather than out of a command line",
		},
		{
			"CUBRID 11.4.5 (11.4.5.1875-74d17e9) (64bit release build for Linux) (Apr 29 2026 15:30:55)",
			"11.4.5.1875-74d17e9",
			"real cubrid_rel output from a locally built engine",
		},
		{
			"CUBRID 11.3 (11.3.5.1275-0e31336) (64bit release build for Linux) (Apr 29 2026 18:40:53)",
			"11.3.5.1275-0e31336",
			"and the other install on this machine",
		},
		{"no version here at all", "", "nothing to find"},
	}
	for _, c := range cases {
		if got := BuildID(c.in); got != c.want {
			t.Errorf("BuildID(%q) = %q, want %q -- %s", c.in, got, c.want, c.why)
		}
	}
}

// A URL usually carries the version twice, in the directory and in the file name.
// The file name is the build, so the last match wins.
func TestBuildIDTakesTheLastVersionInTheString(t *testing.T) {
	got := BuildID("http://build/11.0.0.0001/CUBRID-11.4.0.9999-x-Linux.x86_64.sh")
	if got != "11.4.0.9999-x" {
		t.Errorf("got %q, want the version from the file name", got)
	}
}

// A version with no suffix after it takes everything up to the next ")", which
// on cubrid_rel's line is the end of the build description. CTP produced this and
// wrote it into main_snapshot.properties and into every case's environment. It is
// deterministic, so it still identifies the build, and it is reproduced rather
// than tidied.
func TestAVersionWithNoSuffixRunsOnToTheNextParenthesis(t *testing.T) {
	const rel = "CUBRID 11.2 (11.2.0.0000) (64bit release build for linux_gnu)"
	if got := BuildID(rel); got != "11.2.0.0000) (64bit release build for linux_gnu" {
		t.Errorf("BuildID(%q) = %q", rel, got)
	}
}

func TestBuildBits(t *testing.T) {
	for in, want := range map[string]string{
		"CUBRID-11.4.0.1234-Linux.x86_64.sh":                "64bits",
		"CUBRID-11.4.0.1234-Linux.i386.sh":                  "32bits",
		"CUBRID 11.2 (11.2.0.0000) (64bit release build)":   "64bits",
		"CUBRID-8.4.4.0136-AIX-ppc64.sh":                    "64bits",
		"http://build/CUBRID-10.1.0.7663-Linux_x64.tar.gz":  "64bits",
		"http://build/CUBRID-10.1.0.7663-Linux_i386.tar.gz": "32bits",
	} {
		if got := BuildBits(in); got != want {
			t.Errorf("BuildBits(%q) = %q, want %q", in, got, want)
		}
	}
}

// The bit width is not a number: it selects JAVA_HOME_64BITS on the remote host,
// upper-cased from "64bits". Reading it as "64" would look for a variable nobody
// sets.
func TestBitsSelectTheJavaHomeVariableByItsFullName(t *testing.T) {
	c, _ := Split("a/b/cases/b.sh")
	got := RunScript(c, CaseOptions{Bits: BuildBits("CUBRID-11.4.0.1-Linux.x86_64.sh")})
	if !contains(got, "$JAVA_HOME_64BITS") {
		t.Errorf("expected JAVA_HOME_64BITS in:\n%s", got)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(h, n string) int {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return i
		}
	}
	return -1
}
