package contain

import (
	"context"
	"fmt"
	osexec "os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/cubrid-systems/cubrid-testkit/internal/exec"
)

// Namespace is a place to run commands that outlives each of them.
//
// contain.Enter puts *this* process in namespaces, once, for the whole run.
// A slot needs the same isolation but per worker, and it cannot be built the
// same way: Go has no fork, so a namespace can only be created at exec time,
// and a namespace created per command would take the case's server down with
// it when the command returned. What a slot runs is a sequence -- start a
// server, query it, stop it -- with the server expected to survive between
// them.
//
// So the namespace is held open by a process that does nothing else, and each
// command joins it. That the namespace persists across commands is the property
// the whole design rests on, and it is measured: a tmpfs mounted by one command
// is there for the next, and a process started by one is visible to the next,
// while neither is visible outside.
//
// docs/concept/beyond-axis.md B-T3.
type Namespace struct {
	keeper *osexec.Cmd
	pid    int
	inner  *exec.Local
	label  string
}

// keeperScript is what holds the namespace open.
//
// It must not exec: this process is PID 1 of its namespace, so it inherits
// every orphan in it, and a daemon that double-forks becomes one. Left unreaped
// they are zombies their parent never learns about, which is how a `cubrid
// server stop` was once seen polling for a server that had already exited, for
// twenty-three minutes. A shell that stays a shell reaps them; one that replaces
// itself with `exec "$@"` does not, and that single word cost twelve minutes a
// case before it was found.
const keeperScript = `while :; do sleep 3600 & wait $!; done`

// Open creates a namespace and returns it. label names it in errors.
//
// Only mount, PID and IPC namespaces are asked for. A user namespace is not,
// and that is a consequence of where this runs: the runner has already entered
// one through Enter and is root inside it, so it holds CAP_SYS_ADMIN there and
// can make the rest without another mapping. Asking for a second user namespace
// would nest the uid maps for nothing.
func Open(label string) (*Namespace, error) {
	if !Active() {
		return nil, fmt.Errorf("%s: slots need the runner to be contained first (%s=1)", label, Env)
	}
	cmd := osexec.Command(Shell, "-c", keeperScript)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWNS | syscall.CLONE_NEWPID | syscall.CLONE_NEWIPC,
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("%s: hold a namespace open: %w", label, err)
	}
	ns := &Namespace{keeper: cmd, pid: cmd.Process.Pid, label: label}

	// The keeper is PID 1 but nothing has mounted /proc for it, so `ps` in there
	// would still report the machine. Doing it from outside, in the keeper's own
	// mount namespace, is the same work Setup does for the runner.
	if out, err := ns.run(context.Background(), 10*time.Second,
		"mount --make-rprivate / && mount -t proc proc /proc"); err != nil {
		ns.Close()
		return nil, fmt.Errorf("%s: mount /proc: %w: %s", label, err, strings.TrimSpace(out))
	}
	return ns, nil
}

// Shell is the interpreter the keeper and every command run under.
const Shell = "bash"

// Channel returns a channel whose commands run inside this namespace.
//
// It is an exec.Channel like Local and SSH, so the suite does not learn a third
// way to run something: a slot is handed one of these and behaves as it always
// did.
func (n *Namespace) Channel(dir string, env ...string) exec.Channel {
	inner := exec.NewLocal(dir, env...)
	n.inner = inner
	return &nsChannel{ns: n, inner: inner}
}

// enter is the command that puts a child in this namespace.
//
// The user namespace is deliberately not among them. The keeper was made with
// mount, PID and IPC only, so it is already in this process's user namespace,
// and asking nsenter to join the one it is in fails rather than doing nothing.
// A prototype that gave each keeper its own user namespace needed `-U
// --preserve-credentials` -- and needed them together, because joining a user
// namespace otherwise makes nsenter call setgroups, which an unprivileged one
// denies. Not creating the second user namespace removes both.
func (n *Namespace) enter(argv ...string) []string {
	return append([]string{
		"nsenter", "-t", strconv.Itoa(n.pid), "-p", "-i", "-m", "--",
	}, argv...)
}

func (n *Namespace) run(ctx context.Context, timeout time.Duration, script string) (string, error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	argv := n.enter(Shell, "-c", script)
	cmd := osexec.CommandContext(ctx, argv[0], argv[1:]...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// Close tears the namespace down, and everything still in it with it.
//
// Killing PID 1 of a PID namespace is what the kernel takes as the signal to
// kill the rest; killing whatever started it is not. The shell prototype got
// this wrong -- `unshare --fork` makes its *child* PID 1, so killing unshare
// left the namespace and its processes running. Starting the keeper directly
// means the process here is the one that matters.
func (n *Namespace) Close() error {
	if n.keeper == nil || n.keeper.Process == nil {
		return nil
	}
	_ = n.keeper.Process.Kill()
	done := make(chan struct{})
	go func() { _, _ = n.keeper.Process.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}
	n.keeper = nil
	return nil
}

// Alive reports whether the namespace is still held open. A slot whose keeper
// died is a slot whose next command would silently run on the machine instead,
// which is worse than failing.
func (n *Namespace) Alive() bool {
	if n.keeper == nil || n.keeper.Process == nil {
		return false
	}
	return syscall.Kill(n.pid, 0) == nil
}

type nsChannel struct {
	ns    *Namespace
	inner *exec.Local
}

func (c *nsChannel) Run(ctx context.Context, script string) (exec.Result, error) {
	if !c.ns.Alive() {
		return exec.Result{}, fmt.Errorf("%s: the namespace is gone", c.ns.label)
	}
	return c.inner.RunWith(ctx, script, c.ns.enter)
}

// Put and Get are the machine's filesystem, which the mount namespace shares
// except where a slot has bound something over it. A file written here is the
// file the slot sees, and that is the point of keeping $CUBRID at the same path.
func (c *nsChannel) Put(ctx context.Context, local, remote string) error {
	return c.inner.Put(ctx, local, remote)
}
func (c *nsChannel) Get(ctx context.Context, remote, local string) error {
	return c.inner.Get(ctx, remote, local)
}
func (c *nsChannel) Describe() string { return c.ns.label }
func (c *nsChannel) Close() error     { return c.ns.Close() }

var _ exec.Channel = (*nsChannel)(nil)