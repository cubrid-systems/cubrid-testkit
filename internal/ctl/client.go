package ctl

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

// readBuf is CLIENT_READ_BUFFER_SIZE. The size is not an implementation detail:
// the controller prints one header and one run of "| " prefixes per read, so
// what a read returns decides where those land, and the corpus's answers were
// written against reads of this size.
const readBuf = 8192

// clientState is PRO_STATUS, minus the two values slave mode used.
type clientState int

const (
	running  clientState = iota // started, and not yet waiting for a statement
	ready                       // has answered everything sent to it
	finished                    // the process is gone
)

// client is one qacsql process: three pipes, a transaction index it reports
// once it has connected, and a count of the statements it still owes an answer
// for.
type client struct {
	id   int
	cmd  *exec.Cmd
	in   *os.File // its standard input
	out  *os.File // its standard output
	err  *os.File // its standard error
	outF int      // out and err as raw descriptors, for poll
	errF int

	tranIndex int
	pending   int // statement_count: "is ready" messages still expected
	state     clientState
	exit      int
}

// startClient runs `qacsql <db> -cl <n>` with a pipe for each of its standard
// descriptors (start_process, qactl.c:1868).
func startClient(program, db string, id int) (*client, error) {
	inR, inW, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(program, db, "-cl", strconv.Itoa(id))
	cmd.Stdin, cmd.Stdout, cmd.Stderr = inR, outW, errW
	// Every client gets an error log of its own, as the C does
	// (qactl.c:1953): one file per client per run, not one shared.
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("CUBRID_ERROR_LOG=errlog.%.5s.%d.%d", baseName(program), os.Getpid(), id))
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	inR.Close()
	outW.Close()
	errW.Close()

	c := &client{id: id, cmd: cmd, in: inW, out: outR, err: errR, tranIndex: -1, pending: 1}
	// Fd() takes the file out of the runtime's poller and makes it blocking,
	// which is what this wants: one poll, then one read, in client order.
	c.outF, c.errF = int(outR.Fd()), int(errR.Fd())
	return c, nil
}

func baseName(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[i+1:]
		}
	}
	return p
}

// send writes a statement to the client and says so, in that order, because the
// C does: "Print next cmd_ptr AFTER telling the client so the output is aligned
// better and the client can start sooner" (qactl.c:2517).
func (c *client) send(out io.Writer, stmt string, substatements int) error {
	c.pending = substatements
	c.state = running
	if _, err := c.in.WriteString(stmt); err != nil {
		return err
	}
	if _, err := c.in.WriteString("\n"); err != nil {
		return err
	}
	fmt.Fprintf(out, "MC to C%d: %s\n", c.id, stmt)
	return nil
}

// readOutput is qacsql_output_filter (qactl.c:1407): one read, the transaction
// index scraped out of it, the whole chunk printed behind a header with every
// line prefixed, and one statement accounted for by each ") is ready.".
//
// It returns the number of bytes read; 0 means the client has closed its output
// and is on its way out.
func (c *client) readOutput(out io.Writer) int {
	buf := make([]byte, readBuf)
	n, err := syscall.Read(c.outF, buf)
	for err == syscall.EINTR {
		n, err = syscall.Read(c.outF, buf)
	}
	if n <= 0 {
		return 0
	}
	// The C reads into a buffer it then treats as a string, so a NUL byte ends
	// the chunk as far as everything below is concerned.
	chunk := buf[:n]
	if i := bytes.IndexByte(chunk, 0); i >= 0 {
		chunk = chunk[:i]
	}

	if i := bytes.Index(chunk, []byte("Transaction index = ")); i >= 0 {
		c.tranIndex = leadingInt(chunk[i+len("Transaction index = "):])
	}
	if c.tranIndex == -1 {
		fmt.Fprintf(out, "WARNING: Client %d has an invalid Transaction index\n %d\n", c.id, c.tranIndex)
	}
	fmt.Fprintf(out, "C%d output (Transaction index = %d):\n", c.id, c.tranIndex)

	wrapped := wrap(chunk)
	out.Write(wrapped)

	// The C counts the marker in the wrapped copy, not in the raw chunk.
	for i := 0; ; {
		j := bytes.Index(wrapped[i:], []byte(") is ready."))
		if j < 0 {
			break
		}
		i += j + len(") is ready.")
		c.pending--
		if c.pending == 0 {
			c.state = ready
		}
	}
	return n
}

// wrap puts "| " in front of the chunk and in front of every line after a
// newline that is not the last byte, and ends with a newline of its own. A
// chunk that ends in two newlines therefore leaves a line holding exactly "| ".
func wrap(chunk []byte) []byte {
	b := make([]byte, 0, len(chunk)+len(chunk)/8+8)
	b = append(b, '|', ' ')
	for i := 0; i < len(chunk); i++ {
		b = append(b, chunk[i])
		if chunk[i] == '\n' && i+1 < len(chunk) {
			b = append(b, '|', ' ')
		}
	}
	return append(b, '\n')
}

// readError reports what the client sent on its standard error. The C passes
// the buffer to a printf as the format string (qactl.c:1662); this writes it
// as it is, which differs only for a chunk holding a percent sign.
func (c *client) readError(out io.Writer) int {
	buf := make([]byte, readBuf)
	n, err := syscall.Read(c.errF, buf)
	for err == syscall.EINTR {
		n, err = syscall.Read(c.errF, buf)
	}
	if n <= 0 {
		return 0
	}
	chunk := buf[:n]
	if i := bytes.IndexByte(chunk, 0); i >= 0 {
		chunk = chunk[:i]
	}
	fmt.Fprintf(out, "\nClient C%d (with tran_index %d) send on the STDERR:\n", c.id, c.tranIndex)
	out.Write(chunk)
	return n
}

// close reaps the client. This is kill_aclient (qactl.c:1111) without its
// sleep: the C waits 100 ms before looking, even for a client that has already
// exited, and every case ends by quitting every client. Here the wait comes
// first and the 100 ms only buys time for one that has not gone yet
// (ADR-019, "What changes, deliberately").
func (c *client) close(errOut io.Writer) {
	if c.state == finished {
		return
	}
	c.in.Close()
	c.out.Close()
	c.err.Close()
	if c.cmd.Process != nil {
		var ws syscall.WaitStatus
		pid, err := syscall.Wait4(c.cmd.Process.Pid, &ws, syscall.WNOHANG, nil)
		if pid == 0 && err == nil {
			// Still there: give it the C's grace, then insist.
			sleepms(100)
			if pid, _ = syscall.Wait4(c.cmd.Process.Pid, &ws, syscall.WNOHANG, nil); pid == 0 {
				if c.cmd.Process.Kill() == nil {
					fmt.Fprintf(errOut, "STATUS: forcefully killing pid %d\n", c.cmd.Process.Pid)
					sleepms(100)
					syscall.Wait4(c.cmd.Process.Pid, &ws, syscall.WNOHANG, nil)
				}
			}
		}
		c.exit = ws.ExitStatus()
	}
	c.state = finished
}

// leadingInt is atoi on a byte slice: white space, then digits, or 0. The white
// space matters -- the client prints its index with %4d, so what follows
// "Transaction index = " is usually spaces first.
func leadingInt(b []byte) int {
	i := 0
	for i < len(b) && (b[i] == ' ' || b[i] == '\t' || b[i] == '\n' || b[i] == '\r' || b[i] == '\f' || b[i] == '\v') {
		i++
	}
	b = b[i:]
	i = 0
	for i < len(b) && b[i] >= '0' && b[i] <= '9' {
		i++
	}
	if i == 0 {
		return 0
	}
	n, err := strconv.Atoi(string(b[:i]))
	if err != nil {
		return 0
	}
	return n
}
