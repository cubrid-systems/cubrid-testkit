// The controller: what qactl did, minus what no case asks for.
//
// ADR-019 has the decision and what it drops. The shape here is qactl.c's, on
// purpose -- the same statement loop, the same three pipes per client, the same
// bytes out -- because the corpus's answers are that program's output. What is
// deliberately different is timing: the two fixed 100 ms sleeps are gone,
// clients start together, and waiting for a client to be ready waits on its
// pipe instead of polling every 10 ms.

package ctl

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"
)

const (
	// shortDuration and longDuration are SHORT_DURATION_DEFAULT and
	// LONG_DURATION_DEFAULT: a wait warns after the first and fails after the
	// second.
	shortDuration = 100 * time.Second
	longDuration  = 300 * time.Second
	// deadlockPause is DEADLOCK_DURATION, in seconds.
	deadlockPause = 3
	// serviceQuiet is the select timeout of service_clients: once no client has
	// said anything for this long, the controller goes back to the script.
	serviceQuiet = 500 * time.Microsecond
	// blockedPoll is how often the server is asked whether a transaction is
	// waiting -- qactl's interval, and kept because asking more often was
	// measured and bought nothing: 24 cases took 27.23 s at 1 ms and 27.73 s at
	// 10 ms, individual cases swinging 500 ms either way, and 25 of 26 results
	// were byte-identical between the two (evidence/isolation-controller.md §6).
	blockedPoll = 10 * time.Millisecond
)

// Options is one case, and the programs that run it.
type Options struct {
	DB     string    // the database, always ctldb under CTP
	Case   string    // the .ctl file
	Client string    // the client program, an unchanged ctltool qacsql
	Probe  string    // qablocked, built from native/qablocked.c
	Out    io.Writer // the result file: runone.sh points both descriptors at it
	Err    io.Writer
}

type run struct {
	o       Options
	clients []*client
	probe   *probe
}

// Run executes one case and returns the process's exit status. Everything it
// prints is the case's result; runone.sh normalizes it and compares it with the
// answers.
func Run(o Options) int {
	if o.Out == nil {
		o.Out = os.Stdout
	}
	if o.Err == nil {
		o.Err = os.Stderr
	}
	r := &run{o: o}

	fmt.Fprintf(o.Out, "MC: Attempting to restart master client.\n")
	p, err := startProbe(o.Probe, o.DB)
	if err != nil {
		fmt.Fprintf(o.Err, "ERROR! attempting to restart the database:\n %v\n", err)
		return 1
	}
	r.probe = p
	defer p.close()

	f, err := os.Open(o.Case)
	if err != nil {
		fmt.Fprintf(o.Out, "unable to open input file: %s, %v\n", o.Case, err)
		return 1
	}
	defer f.Close()

	// The first statement says how many clients the case wants.
	n := 1
	if first, ok := NewReader(f).Next(); ok {
		n = NumClients(first)
	}
	if n < 1 {
		fmt.Fprintf(o.Err, "ERROR: Could not read number of clients\n")
		return 1
	}
	fmt.Fprintf(o.Out, "INFO! Setting up %d clients.\n", n)

	// Started together. qactl waited for each one to report before starting the
	// next, which cost about 21 ms a client; what the two orders can differ in
	// is the order of the startup chunks, and every line that tells them apart
	// carries "Transaction index" and is deleted by the normalization.
	for i := 1; i <= n; i++ {
		c, err := startClient(o.Client, o.DB, i)
		if err != nil {
			fmt.Fprintf(o.Err, "ERROR: Failed to create all %d clients, client %d failed: %v\n", n, i, err)
			r.killAll()
			return 1
		}
		r.clients = append(r.clients, c)
	}
	defer r.killAll()
	for _, c := range r.clients {
		if !r.waitReady(c, "client_init: Initial Client readiness test") {
			return 1
		}
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		fmt.Fprintf(o.Err, "ERROR: Failed trying to fseek() to start of file, %v\n", err)
		return 1
	}
	return r.loop(NewReader(f))
}

// loop is control_clients: one statement at a time, until the file ends.
func (r *run) loop(rd *Reader) int {
	for {
		stmt, sub, ok := rd.NextCompound()
		if !ok {
			return r.endOfInput()
		}
		s := skipWhite(stmt)
		if len(s) == 0 {
			fmt.Fprintf(r.o.Out, "Got an empty statement.\n")
			continue
		}
		cmd := Classify(s)
		if cmd.Kind != ToClient {
			// Every master command is echoed with the line it came from.
			fmt.Fprintf(r.o.Err, "QACTL %d line: %d statement: (%s)\n", 0, rd.Lines(), s)
		}
		switch cmd.Kind {
		case Setup:
			// Read before the loop started.

		case ToClient:
			// A statement that does not name a client goes to client 1, which is
			// qactl's rule and is kept. It reads wrong for the second statement
			// of `C2: insert a; insert b;` -- five files write 32 of those -- but
			// sending them to the client the line names was tried and reverted:
			// of the five, two are untouched by it, one loses a line of its
			// answer, and two break, because the first statement of the line is
			// the one that blocks and a blocked client cannot be given the next
			// one. Measured on `insert_delete_02`: 551 ms and OK as qactl routes
			// it, 300,328 ms and NOK with the line's client, the
			// 300 s being this controller waiting for a client that will not come
			// back (evidence/isolation-controller.md §5).
			if !r.toClient(cmd, sub) {
				return 1
			}

		case WaitReady:
			c, ok := r.client(cmd.Client, s)
			if !ok {
				return r.waitFailed()
			}
			if !r.waitReady(c, s) {
				return r.waitFailed()
			}

		case WaitBlocked:
			c, ok := r.client(cmd.Client, s)
			if !ok {
				return r.waitFailed()
			}
			if !r.waitBlocked(c, s) {
				return r.waitFailed()
			}

		case WaitUnblocked:
			c, ok := r.client(cmd.Client, s)
			if !ok {
				return r.waitFailed()
			}
			if !r.waitUnblocked(c, s) {
				return r.waitFailed()
			}

		case Sleep:
			// No client is serviced while this sleeps, as in the C: what a
			// client says meanwhile waits in its pipe and is read after.
			sleepms(cmd.Sleep * 1000)

		case DeadlockPause:
			for i := 0; i < deadlockPause; i++ {
				r.service()
				sleepms(1000)
			}

		case Retired:
			// Refused rather than ignored, and told where it does exist: this is
			// a command ctltool's qactl implements and no case in the corpus uses
			// (ADR-019). A case that starts using one should stop the run and say
			// so, not quietly do something else.
			fmt.Fprintf(r.o.Out,
				"ERROR! %q is a command of ctltool's qactl that this controller does not\n"+
					"  implement: no case in the corpus uses it (ADR-019). Unset\n"+
					"  TESTKIT_ISOLATION_CTL to run this case with qactl instead.\n"+
					"  Unable to process the statement on line %d: %s\n",
				cmd.Name, rd.Lines(), s)
			return 1

		default:
			fmt.Fprintf(r.o.Out,
				"Master controller syntax error. Try one of the following:\n"+
					"  WAIT UNTIL Cx {READY|BLOCKED|UNBLOCKED}, SLEEP x, PAUSE FOR DEADLOCK.\n"+
					"  Unable to process the statement on line %d: %s\n", rd.Lines(), s)
			return 1
		}
	}
}

// client finds a client by the number a statement named.
func (r *run) client(id int, stmt string) (*client, bool) {
	if id < 1 || id > len(r.clients) {
		fmt.Fprintf(r.o.Out, "ERROR! client identifier (%d) out of range.\n=> %s.\n", id, stmt)
		return nil, false
	}
	return r.clients[id-1], true
}

// toClient sends a statement to its client, once that client has answered
// everything it was sent before.
func (r *run) toClient(cmd Command, substatements int) bool {
	c, ok := r.client(cmd.Client, cmd.Text)
	if !ok {
		return false
	}
	if c.state == finished {
		return true // the C ignores statements for a client that has gone
	}
	if !r.serviceUntil(func() bool { return c.state == ready || c.state == finished }, longDuration, false) {
		r.notResponding(c)
		return false
	}
	if c.state != ready {
		r.notResponding(c)
		return false
	}
	if err := c.send(r.o.Out, cmd.Text, substatements); err != nil {
		fmt.Fprintf(r.o.Out, "ERROR! Detected broken pipe for client %d.\n", c.id)
		fmt.Fprintf(r.o.Out, "Code: %v\n", err)
		return false
	}
	return true
}

// waitReady is `MC: wait until Cn ready;` and the readiness test each client is
// given when it starts. Where qactl polled every 10 ms, this waits on the
// client's own pipe and returns as soon as the client reports.
func (r *run) waitReady(c *client, stmt string) bool {
	done := func() bool { return c.state == ready || c.state == finished }
	if r.serviceUntil(done, shortDuration, false) {
		return true
	}
	if r.allBlocked() {
		fmt.Fprintf(r.o.Out, "ERROR! All clients are blocked.\n"+
			"Check the script for unintentional deadlock situation.\n"+
			"If the script is correct then check for possible deadlock bug.\n"+
			"=> %s.\n", stmt)
		return false
	}
	fmt.Fprintf(r.o.Out, "WARNING! Client %d was not ready after waiting %d seconds.\n=> %s.\n",
		c.id, int(shortDuration.Seconds()), stmt)
	if r.serviceUntil(done, longDuration, false) {
		return true
	}
	fmt.Fprintf(r.o.Out, "ERROR! Client was not ready after waiting %d more seconds.\n"+
		"Verify client needs more time to finish processing.\n"+
		"If client finished processing then check qacsql for problems.\n"+
		"=> %s.\n", int(longDuration.Seconds()), stmt)
	return false
}

// waitBlocked is `MC: wait until Cn blocked;` -- the statement the language
// exists for. The answer is the server's, so it is still a poll, but a finer
// one than the C's 10 ms.
func (r *run) waitBlocked(c *client, stmt string) bool {
	blocked := func() bool { return c.state == finished || r.probe.blocked(c.tranIndex) }
	if r.serviceUntil(blocked, shortDuration, true) {
		return true
	}
	if c.state == ready {
		fmt.Fprintf(r.o.Out, "ERROR! Client %d is ready.\n Check the script for errors.\n"+
			"If the script is correct then check qacsql by testing manually.\n=> %s.\n", c.id, stmt)
		return false
	}
	fmt.Fprintf(r.o.Out, "WARNING! Client %d was not blocked after waiting %d seconds.\n => %s.\n",
		c.id, int(shortDuration.Seconds()), stmt)
	if r.serviceUntil(blocked, longDuration, true) {
		return true
	}
	fmt.Fprintf(r.o.Out, "ERROR! Client %d was not blocked after waiting %d more seconds.\n"+
		" Verify client needs more time to finish processing.\n"+
		"If client finished processing then check qacsql for problems.\n=> %s.\n",
		c.id, int(longDuration.Seconds()), stmt)
	return false
}

// waitUnblocked is the other side of it: 150 cases use it.
func (r *run) waitUnblocked(c *client, stmt string) bool {
	free := func() bool { return c.state == finished || !r.probe.blocked(c.tranIndex) }
	if r.serviceUntil(free, shortDuration, true) {
		return true
	}
	fmt.Fprintf(r.o.Out, "WARNING! Client %d blocked after waiting %d seconds.\n=> %s.\n",
		c.id, int(shortDuration.Seconds()), stmt)
	if r.serviceUntil(free, longDuration, true) {
		return true
	}
	fmt.Fprintf(r.o.Out, "ERROR! Client %d remains blocked after waiting %d more seconds.\n=> %s.\n",
		c.id, int(longDuration.Seconds()), stmt)
	return false
}

// allBlocked is all_blocked(): every live client waiting on a lock, which is
// the script's own deadlock rather than the database's.
func (r *run) allBlocked() bool {
	live := 0
	for _, c := range r.clients {
		if c.state == finished {
			continue
		}
		live++
		if !r.probe.blocked(c.tranIndex) {
			return false
		}
	}
	return live > 0
}

// endOfInput tells every client still waiting for work to quit, and waits for
// all of them to go (qactl.c:2131).
func (r *run) endOfInput() int {
	for _, c := range r.clients {
		if c.state == finished {
			continue
		}
		if c.state == ready {
			if _, err := c.in.WriteString("quit;\n"); err != nil {
				fmt.Fprintf(r.o.Out, "ERROR! Detected broken pipe for client %d.\n", c.id)
				c.close(r.o.Err)
				continue
			}
		}
		if !r.serviceUntil(func() bool { return c.state == finished }, longDuration, false) {
			r.notResponding(c)
			return 1
		}
	}
	return 0
}

// waitFailed is what follows a `wait until` that did not come true: the lock
// table, printed into the result, and then the run is over (qactl.c:2268-2273).
func (r *run) waitFailed() int {
	if dump := r.probe.lockTable(); len(dump) > 0 {
		r.o.Out.Write(dump)
	}
	return 1
}

func (r *run) notResponding(c *client) {
	fmt.Fprintf(r.o.Out, "ERROR! Client %d is not responding.\n", c.id)
}

func (r *run) killAll() {
	for _, c := range r.clients {
		c.close(r.o.Err)
	}
}

// service reads whatever the clients have said and returns when they have been
// quiet for serviceQuiet. It is service_clients: select, then read the ready
// descriptors in client order, one read each.
func (r *run) service() { r.serviceUntil(func() bool { return false }, serviceQuiet, false) }

// serviceUntil services the clients until done() holds or limit runs out.
//
// asking says the answer comes from somewhere other than a client's pipe -- the
// server, for `blocked` -- so the wait has to end on a timer as well as on a
// descriptor. Without it the wait is the pipe's: select returns when the client
// says something or closes, and nothing spins.
func (r *run) serviceUntil(done func() bool, limit time.Duration, asking bool) bool {
	deadline := time.Now().Add(limit)
	for {
		if done() {
			return true
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return done()
		}
		wait := remaining
		switch {
		case limit == serviceQuiet:
			// A plain service(): quiet is the end of it.
			if !r.selectOnce(serviceQuiet) {
				return false
			}
			continue
		case asking:
			wait = blockedPoll
		case wait > time.Second:
			// Nothing but the client can end this wait; a bound only keeps the
			// deadline honest.
			wait = time.Second
		}
		if wait > remaining {
			wait = remaining
		}
		r.selectOnce(wait)
	}
}

// selectOnce waits up to d for any client to say something and reads from every
// one that has. It reports whether anything was read.
func (r *run) selectOnce(d time.Duration) bool {
	var set syscall.FdSet
	max := 0
	live := 0
	for _, c := range r.clients {
		if c.state == finished {
			continue
		}
		live++
		for _, fd := range []int{c.outF, c.errF} {
			fdSet(&set, fd)
			if fd > max {
				max = fd
			}
		}
	}
	if live == 0 {
		// Nothing to wait on. Only a caller waiting for something a dead client
		// can never do would get here, and it should not spin while it does.
		time.Sleep(d)
		return false
	}
	tv := syscall.NsecToTimeval(d.Nanoseconds())
	n, err := syscall.Select(max+1, &set, nil, nil, &tv)
	if err == syscall.EINTR {
		return true
	}
	if n <= 0 {
		r.reap()
		return false
	}
	for _, c := range r.clients {
		if c.state == finished {
			continue
		}
		if fdIsSet(&set, c.outF) {
			if c.readOutput(r.o.Out) == 0 {
				// End of its output: the client is on its way out.
				c.close(r.o.Err)
				continue
			}
		}
		if fdIsSet(&set, c.errF) {
			c.readError(r.o.Err)
		}
	}
	return true
}

// reap is master_timeout's first half: notice the clients that have exited.
func (r *run) reap() {
	for _, c := range r.clients {
		if c.state == finished || c.cmd.Process == nil {
			continue
		}
		var ws syscall.WaitStatus
		if pid, err := syscall.Wait4(c.cmd.Process.Pid, &ws, syscall.WNOHANG, nil); pid > 0 && err == nil {
			c.exit = ws.ExitStatus()
			c.state = finished
		}
	}
}

func fdSet(s *syscall.FdSet, fd int)        { s.Bits[fd/64] |= 1 << (uint(fd) % 64) }
func fdIsSet(s *syscall.FdSet, fd int) bool { return s.Bits[fd/64]&(1<<(uint(fd)%64)) != 0 }
func sleepms(ms int)                        { time.Sleep(time.Duration(ms) * time.Millisecond) }
func serverStart(db string) error           { return exec.Command("cubrid", "server", "start", db).Run() }
