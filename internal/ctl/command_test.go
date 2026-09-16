package ctl

import (
	"strings"
	"testing"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		in     string
		kind   Kind
		client int
		text   string
		sleep  int
	}{
		{in: "MC: setup NUM_CLIENTS = 2;", kind: Setup},
		{in: "mc: SETUP num_clients = 3;", kind: Setup},
		{in: "MC: wait until C2 ready;", kind: WaitReady, client: 2},
		{in: "MC: wait until c12 blocked;", kind: WaitBlocked, client: 12},
		{in: "MC: WAIT UNTIL C3 UNBLOCKED;", kind: WaitUnblocked, client: 3},
		{in: "MC: sleep 30;", kind: Sleep, sleep: 30},
		{in: "MC: pause for deadlock resolution;", kind: DeadlockPause},

		// The corpus never asks for these, and ADR-019 refuses them by name.
		{in: "MC: wait until C1 finished;", kind: Retired},
		{in: "MC: wait for 5;", kind: Retired},
		{in: "MC: reconnect;", kind: Retired},
		{in: "MC: rendezvous with super;", kind: Retired},
		{in: "MC: execute delay 2 mc_gen.sh 1 t 2;", kind: Retired},
		{in: "MC: allocate client;", kind: Retired},
		{in: "MC: no-op;", kind: Retired},

		// Client statements keep their text exactly, minus the prefix.
		{in: "C1: select 1;", kind: ToClient, client: 1, text: "select 1;"},
		{in: "C2: INSERT INTO t VALUES('A');", kind: ToClient, client: 2, text: "INSERT INTO t VALUES('A');"},
		{in: "c10: quit;", kind: ToClient, client: 10, text: "quit;"},

		// The second statement of a line lost its prefix in the reader, and
		// has always gone to client 1 (ADR-019, "What is kept exactly").
		{in: "drop table t2;", kind: ToClient, client: 1, text: "drop table t2;"},
		// Not a client prefix: no digits, a zero, or no colon.
		{in: "C: select 1;", kind: ToClient, client: 1, text: "C: select 1;"},
		{in: "C0: select 1;", kind: ToClient, client: 1, text: "C0: select 1;"},
		{in: "C1 select 1;", kind: ToClient, client: 1, text: "C1 select 1;"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got := Classify(c.in)
			if got.Kind != c.kind {
				t.Fatalf("kind: got %v, want %v (%+v)", got.Kind, c.kind, got)
			}
			if c.client != 0 && got.Client != c.client {
				t.Errorf("client: got %d, want %d", got.Client, c.client)
			}
			if c.text != "" && got.Text != c.text {
				t.Errorf("text: got %q, want %q", got.Text, c.text)
			}
			if c.sleep != 0 && got.Sleep != c.sleep {
				t.Errorf("sleep: got %d, want %d", got.Sleep, c.sleep)
			}
		})
	}
}

func TestNumClients(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"MC: setup NUM_CLIENTS = 2;", 2},
		{"MC: setup num_clients = 22;", 22},
		{"MC: SETUP NUM_CLIENTS = 5;", 5},
		{"C1: select 1;", 1}, // no header: one client
	}
	for _, c := range cases {
		if got := NumClients(c.in); got != c.want {
			t.Errorf("%q: got %d, want %d", c.in, got, c.want)
		}
	}
}

// qactl sends a statement that names no client to client 1, and the controller
// keeps that (see the ToClient case in loop). What the reader has to agree
// about, for the rule to be discussable at all, is what a line is: two
// statements written on one line report the same line number.
func TestStatementsShareALineNumber(t *testing.T) {
	r := NewReader(strings.NewReader("C2: insert a; insert b;\ninsert c;\n"))
	var lines []int
	for {
		if _, ok := r.Next(); !ok {
			break
		}
		lines = append(lines, r.Lines())
	}
	if len(lines) != 3 {
		t.Fatalf("got %d statements, want 3", len(lines))
	}
	if lines[0] != lines[1] {
		t.Errorf("the first two share a line: got %d and %d", lines[0], lines[1])
	}
	if lines[2] == lines[1] {
		t.Errorf("the third is on its own line: got %d for both", lines[2])
	}
}
