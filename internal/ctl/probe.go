package ctl

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
)

// probe is the one question the controller cannot ask in Go: whether a
// transaction is waiting on a lock. native/qablocked.c connects to the database
// and answers it, one line at a time.
type probe struct {
	cmd *exec.Cmd
	in  io.WriteCloser
	out *bufio.Reader
}

// startProbe runs the probe and waits for it to say it is connected. A database
// that will not answer is started once and tried again, because recovery cases
// kill the server and the next case finds it down (qactl.c:3038).
func startProbe(program, db string) (*probe, error) {
	p, err := spawnProbe(program, db)
	if err == nil {
		return p, nil
	}
	if serverStart(db) != nil {
		return nil, err
	}
	return spawnProbe(program, db)
}

func spawnProbe(program, db string) (*probe, error) {
	cmd := exec.Command(program, db)
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	p := &probe{cmd: cmd, in: in, out: bufio.NewReader(out)}
	line, err := p.out.ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "ready" {
		p.close()
		return nil, fmt.Errorf("%s %s did not connect", program, db)
	}
	return p, nil
}

// blocked asks about one transaction index. A probe that has stopped answering
// reports false, which is what local_tm_isblocked does for a client that is not
// connected (cubrid_drv.c:594).
func (p *probe) blocked(tranIndex int) bool {
	if p == nil || tranIndex < 0 {
		return false
	}
	if _, err := io.WriteString(p.in, strconv.Itoa(tranIndex)+"\n"); err != nil {
		return false
	}
	line, err := p.out.ReadString('\n')
	if err != nil {
		return false
	}
	return strings.TrimSpace(line) == "1"
}

// lockTable is what qactl prints when a `wait until` command has failed
// (qactl.c:2271): the server's lock table, into the case's result. It is the
// other thing only a connected client can ask for.
func (p *probe) lockTable() []byte {
	if p == nil {
		return nil
	}
	if _, err := io.WriteString(p.in, "dump\n"); err != nil {
		return nil
	}
	line, err := p.out.ReadString('\n')
	if err != nil {
		return nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || n <= 0 {
		return nil
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(p.out, buf); err != nil {
		return nil
	}
	return buf
}

func (p *probe) close() {
	if p == nil {
		return
	}
	if p.in != nil {
		p.in.Close()
	}
	if p.cmd.Process != nil {
		p.cmd.Wait()
	}
}
