package exec

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// The framing CTP wraps every remote script in.
//
// The script writes `echo ALL_${NOTEXIST}STARTED`, and because $NOTEXIST is unset
// the shell prints ALL_STARTED. The indirection is the point: the script's own
// text never matches the marker, so a shell echoing its input -- `set -x`, a login
// banner, a sudo prompt replaying the line -- cannot be mistaken for the frame.
// Output is what lies between the two markers.
const (
	startFlagLiteral = "ALL_STARTED"
	compFlagLiteral  = "ALL_COMPLETED"
	startFlagEcho    = "echo ALL_${NOTEXIST}STARTED"
	compFlagEcho     = "echo ALL_${NOTEXIST}COMPLETED"
)

// SSHConfig describes one connection.
type SSHConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Timeout  time.Duration
}

// SSH runs commands on another machine.
//
// Host keys are not checked, which is what CTP does: SSHConnect sets
// StrictHostKeyChecking=no. The alternative would be to fail against every QA
// machine that has been reimaged, and the credentials are already sitting in a
// plaintext configuration file, so verifying the host adds nothing this deployment
// does not already concede. It is a deliberate match, not an oversight.
type SSH struct {
	cfg SSHConfig

	mu     sync.Mutex
	client *ssh.Client
}

// NewSSH prepares a channel. The connection is made on first use.
func NewSSH(cfg SSHConfig) *SSH {
	if cfg.Port == "" {
		cfg.Port = "22"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	return &SSH{cfg: cfg}
}

func (s *SSH) Describe() string {
	return fmt.Sprintf("ssh %s@%s:%s", s.cfg.User, s.cfg.Host, s.cfg.Port)
}

// auth offers the methods CTP asked jsch for, in the order it asked for them:
// password, then public key, then keyboard-interactive.
func (s *SSH) auth() []ssh.AuthMethod {
	var methods []ssh.AuthMethod
	if s.cfg.Password != "" {
		methods = append(methods, ssh.Password(s.cfg.Password))
	}
	if signers := agentSigners(); len(signers) > 0 {
		methods = append(methods, ssh.PublicKeys(signers...))
	}
	if s.cfg.Password != "" {
		// Many sshd builds answer a password only through keyboard-interactive.
		methods = append(methods, ssh.KeyboardInteractive(
			func(_, _ string, questions []string, _ []bool) ([]string, error) {
				answers := make([]string, len(questions))
				for i := range questions {
					answers[i] = s.cfg.Password
				}
				return answers, nil
			}))
	}
	return methods
}

func (s *SSH) connect(ctx context.Context) (*ssh.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client != nil {
		return s.client, nil
	}

	config := &ssh.ClientConfig{
		User:            s.cfg.User,
		Auth:            s.auth(),
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // see the type comment
		Timeout:         s.cfg.Timeout,
	}

	dialer := net.Dialer{Timeout: s.cfg.Timeout}
	addr := net.JoinHostPort(s.cfg.Host, s.cfg.Port)
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}
	c, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("ssh handshake with %s: %w", addr, err)
	}
	s.client = ssh.NewClient(c, chans, reqs)
	return s.client, nil
}

// Run sends a script and returns what it printed between the frame markers.
func (s *SSH) Run(ctx context.Context, script string) (Result, error) {
	client, err := s.connect(ctx)
	if err != nil {
		return Result{}, err
	}
	sess, err := client.NewSession()
	if err != nil {
		return Result{}, fmt.Errorf("ssh session: %w", err)
	}
	defer sess.Close()

	var stdout, stderr bytes.Buffer
	sess.Stdout = &stdout
	sess.Stderr = &stderr

	framed := strings.Join([]string{Profile, startFlagEcho, script, compFlagEcho}, "\n")

	done := make(chan error, 1)
	go func() { done <- sess.Run(framed) }()

	select {
	case <-ctx.Done():
		_ = sess.Signal(ssh.SIGKILL)
		return Result{}, ctx.Err()
	case err = <-done:
	}

	res := Result{Stdout: between(stdout.String()), Stderr: stderr.String()}
	if err == nil {
		return res, nil
	}
	var exitErr *ssh.ExitError
	if e, ok := err.(*ssh.ExitError); ok {
		exitErr = e
		res.ExitCode = exitErr.ExitStatus()
		return res, nil
	}
	return res, fmt.Errorf("remote run on %s: %w", s.cfg.Host, err)
}

// between keeps what lies inside the frame, and returns the whole thing when a
// marker is missing -- a script killed halfway still has output worth reporting.
func between(raw string) string {
	out := raw
	if at := strings.Index(out, startFlagLiteral); at != -1 {
		out = out[at+len(startFlagLiteral):]
		out = strings.TrimPrefix(out, "\n")
	}
	if at := strings.Index(out, compFlagLiteral); at != -1 {
		out = out[:at]
	}
	return out
}

// Put copies a local file over, through the exec channel rather than SFTP: it
// costs no extra dependency and the assets moved this way are shell scripts.
func (s *SSH) Put(ctx context.Context, local, remote string) error {
	f, err := os.Open(local)
	if err != nil {
		return err
	}
	defer f.Close()

	client, err := s.connect(ctx)
	if err != nil {
		return err
	}
	sess, err := client.NewSession()
	if err != nil {
		return err
	}
	defer sess.Close()
	sess.Stdin = f
	if err := sess.Run(fmt.Sprintf("mkdir -p $(dirname %q) && cat > %q", remote, remote)); err != nil {
		return fmt.Errorf("put %s to %s: %w", local, remote, err)
	}
	return nil
}

// Get copies a remote file back.
func (s *SSH) Get(ctx context.Context, remote, local string) error {
	client, err := s.connect(ctx)
	if err != nil {
		return err
	}
	sess, err := client.NewSession()
	if err != nil {
		return err
	}
	defer sess.Close()

	f, err := os.Create(local)
	if err != nil {
		return err
	}
	defer f.Close()
	sess.Stdout = f
	if err := sess.Run(fmt.Sprintf("cat %q", remote)); err != nil {
		return fmt.Errorf("get %s from %s: %w", remote, s.cfg.Host, err)
	}
	return nil
}

func (s *SSH) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client == nil {
		return nil
	}
	err := s.client.Close()
	s.client = nil
	return err
}

// agentSigners returns keys from a running ssh-agent, if there is one.
func agentSigners() []ssh.Signer {
	sock := os.Getenv("SSH_AUTH_SOCK")
	if sock == "" {
		return nil
	}
	conn, err := net.Dial("unix", sock)
	if err != nil {
		return nil
	}
	signers, err := agentClientSigners(conn)
	if err != nil {
		conn.Close()
		return nil
	}
	return signers
}
