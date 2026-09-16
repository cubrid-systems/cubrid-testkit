package ctl

import "testing"

// The prefix goes in front of the chunk and in front of every line after a
// newline that is not the last byte, and a newline is added at the end. The
// trailing newline is why every chunk leaves a blank line behind it in the raw
// result -- which the normalization then deletes (runone.sh:61).
func TestWrap(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Client 1 (Transaction index =    2) is ready.\n",
			"| Client 1 (Transaction index =    2) is ready.\n\n"},
		{"Ope_no =    1, Client_no = 1\nset transaction lock timeout INFINITE;\n",
			"| Ope_no =    1, Client_no = 1\n| set transaction lock timeout INFINITE;\n\n"},
		// A chunk that ends in two newlines leaves a line holding "| ", and
		// then the added newline leaves an empty one after it.
		{"1 row affected\n\n", "| 1 row affected\n| \n\n"},
		{"no newline", "| no newline\n"},
	}
	for _, c := range cases {
		if got := string(wrap([]byte(c.in))); got != c.want {
			t.Errorf("wrap(%q):\n got %q\nwant %q", c.in, got, c.want)
		}
	}
}

// The client prints its index with %4d, so what follows the marker is spaces
// and then digits. Reading it as digits alone gave every client index 0, and
// with it every `wait until Cn blocked` asked the server about transaction 0.
func TestLeadingInt(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"   2) is ready.", 2},
		{"2)", 2},
		{" 123 ", 123},
		{"not a number", 0},
	}
	for _, c := range cases {
		if got := leadingInt([]byte(c.in)); got != c.want {
			t.Errorf("leadingInt(%q): got %d, want %d", c.in, got, c.want)
		}
	}
}
