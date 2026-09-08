package contain

import (
	"context"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
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
	keeper  *osexec.Cmd
	pid     int
	inner   *exec.Local
	label   string
	scratch string
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
// Mount, PID, IPC and network namespaces are asked for. A user namespace is
// not, and that is a consequence of where this runs: the runner has already
// entered one through Enter and is root inside it, so it holds CAP_SYS_ADMIN
// there and can make the rest without another mapping. Asking for a second user
// namespace would nest the uid maps for nothing -- and it is not free, because
// nsenter joining the user namespace it is already in fails rather than doing
// nothing.
//
// The network namespace is what makes a slot cost nothing to configure. Two
// slots collide on the master port, the two broker ports and whatever else a
// case starts; a network namespace gives each of them the whole port space, so
// every slot runs on 1523 and the shipped configuration is never rewritten.
// That is not only simpler than allocating ports: a slotted run's conf files
// and log lines stay byte-identical to a serial run's, which is the evidence
// B-T3 has to produce.
//
// What it costs is the outside. Six of the 3,452 cases mention wget or curl and
// one of them, _06_issues/_25_2h/cbrd_26350, fetches a URL that is genuinely
// external; the rest are aimed at localhost or a broker. 201 cases say
// localhost and 34 say 127.0.0.1, all of which work here, and the 126 that read
// the hostname are unaffected because the UTS namespace is not among these.
func Open(label string) (*Namespace, error) {
	if !Active() {
		return nil, fmt.Errorf("%s: slots need the runner to be contained first (%s=1)", label, Env)
	}
	cmd := osexec.Command(Shell, "-c", keeperScript)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWNS | syscall.CLONE_NEWPID |
			syscall.CLONE_NEWIPC | syscall.CLONE_NEWNET,
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("%s: hold a namespace open: %w", label, err)
	}
	ns := &Namespace{keeper: cmd, pid: cmd.Process.Pid, label: label}
	// Outside /tmp on purpose: this is where the scripts a command runs are
	// written, and the slot is about to get a /tmp that this process cannot see
	// into.
	ns.scratch = filepath.Join(scratchRoot(), label)
	if err := os.MkdirAll(ns.scratch, 0o755); err != nil {
		ns.Close()
		return nil, fmt.Errorf("%s: %w", label, err)
	}

	// The keeper is PID 1 but nothing has mounted /proc for it, so `ps` in there
	// would still report the machine. Doing it from outside, in the keeper's own
	// mount namespace, is the same work Setup does for the runner.
	// Three things the namespaces do not give a slot on their own.
	//
	// /proc, because the keeper is PID 1 of a new PID namespace but nothing has
	// mounted a /proc that reports it, so `ps` would still show the machine.
	//
	// /dev/shm, because an IPC namespace separates System V shared memory and
	// POSIX shared memory is a *file* on a tmpfs -- and the tmpfs a mount
	// namespace inherits is the one it was cloned from. cub_broker and cub_cas
	// call both shmget and shm_open, so without this the slots' brokers share
	// their segments and a case fails to connect to a database that is running.
	//
	// Loopback, because it comes up down in a fresh network namespace and every
	// connection a case makes goes through it.
	if out, err := ns.run(context.Background(), 10*time.Second,
		"mount --make-rprivate / && mount -t proc proc /proc && "+
			"mount -t tmpfs -o mode=1777 shm /dev/shm && ip link set lo up"); err != nil {
		ns.Close()
		return nil, fmt.Errorf("%s: prepare: %w: %s", label, err, strings.TrimSpace(out))
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
	// The script has to be written somewhere both sides can see. This slot has a
	// /tmp of its own, so the usual place is the wrong one.
	inner.ScriptDir = n.scratch
	n.inner = inner
	return &nsChannel{ns: n, inner: inner}
}

// Private gives this slot a directory of its own at path, backed by under.
//
// /tmp is the case for which this exists. cub_master listens on a Unix domain
// socket named after its port -- /tmp/CUBRID1523 -- and the network namespace
// that lets every slot keep the shipped 1523 is exactly what makes them all want
// that one path. Four masters then fight over one socket and none of them comes
// up: "Could not connect to master server on localhost", six times, and every
// case that needed a server fails.
//
// A bind of a directory rather than a tmpfs, because a case is free to write
// something large to /tmp and a tmpfs would take it out of memory.
func (n *Namespace) Private(path, under string) error {
	if !n.Alive() {
		return fmt.Errorf("%s: the namespace is gone", n.label)
	}
	// Replacing the directory the scripts are written into would leave every
	// command looking for a file that is not there, and the symptom is a command
	// that produces nothing at all rather than an error.
	if rel, err := filepath.Rel(path, n.scratch); err == nil && !strings.HasPrefix(rel, "..") {
		return fmt.Errorf("%s: cannot make %s private: it holds this slot's scripts (%s)",
			n.label, path, n.scratch)
	}
	if err := os.MkdirAll(under, 0o1777); err != nil {
		return fmt.Errorf("%s: %w", n.label, err)
	}
	if err := os.Chmod(under, 0o1777); err != nil {
		return fmt.Errorf("%s: %w", n.label, err)
	}
	if out, err := n.run(context.Background(), 20*time.Second,
		fmt.Sprintf("mount --bind %s %s", under, path)); err != nil {
		return fmt.Errorf("%s: private %s: %w: %s", n.label, path, err, strings.TrimSpace(out))
	}
	return nil
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
		"nsenter", "-t", strconv.Itoa(n.pid), "-p", "-i", "-m", "-n", "--",
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

// Overlay makes target writable for this slot alone, and costs nothing to set
// up.
//
// $CUBRID is 323 MB and a run writes to five places in it: conf/ because cases
// edit cubrid.conf, log/ and var/, locales/loclib/ where make_locale builds, and
// lib/ where it puts the result. Copying the install per slot to isolate that is
// 323 MB a slot; binding a copy of each writable directory is five mounts and a
// list that has already been wrong once -- it named lib/ and not
// locales/loclib/, which make_locale writes first.
//
// An overlay needs neither. The install is the lower layer, shared and untouched,
// and everything a slot writes lands in its own upper layer. There is no list to
// keep correct: whatever a case writes anywhere under target is this slot's.
//
// Unprivileged overlayfs is what makes it possible and it is measured here
// rather than assumed: inside the runner's user namespace a lower layer reads
// through, a write lands in the upper, and the lower is unchanged.
func (n *Namespace) Overlay(target, upperRoot string) error {
	if !n.Alive() {
		return fmt.Errorf("%s: the namespace is gone", n.label)
	}
	upper := filepath.Join(upperRoot, "upper")
	work := filepath.Join(upperRoot, "work")
	for _, d := range []string{upper, work} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("%s: %w", n.label, err)
		}
	}
	// A comma in any of these would be read as an option separator, and the
	// mount would fail somewhere less obvious than here.
	for _, d := range []string{target, upper, work} {
		if strings.ContainsAny(d, ",:") {
			return fmt.Errorf("%s: %q cannot be an overlay directory: the option string is comma-separated", n.label, d)
		}
	}
	script := fmt.Sprintf("mount -t overlay overlay -o lowerdir=%s,upperdir=%s,workdir=%s %s",
		target, upper, work, target)
	if out, err := n.run(context.Background(), 20*time.Second, script); err != nil {
		return fmt.Errorf("%s: overlay %s: %w: %s", n.label, target, err, strings.TrimSpace(out))
	}
	return nil
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

// scratchRoot is where slots keep the scripts their commands run from.
//
// Not under /tmp, and not under os.TempDir() which usually is /tmp: a slot gets
// a /tmp of its own, and a script written into this process's would not be there
// when the command went looking. /var/tmp is the same filesystem and not the
// directory being replaced.
func scratchRoot() string { return filepath.Join(SlotRoot(), "scratch") }

// SlotRoot is where this run keeps everything it makes per slot: the overlay
// upper layers and the scripts commands are written into.
//
// The default carries the process id, because two runs on one machine would
// otherwise both call a slot "slot0" and mount an overlay over the same upper
// directory -- which is not a collision that announces itself, it is one run
// quietly writing into another's $CUBRID. Everything else a run makes is already
// unique: the corpus tmpfs comes from MkdirTemp and CUBRID_TMP carries the pid.
//
// TESTKIT_SLOT_ROOT still overrides, and a caller that sets it takes
// responsibility for keeping two runs apart.
func SlotRoot() string {
	if r := os.Getenv(SlotRootEnv); r != "" {
		return r
	}
	return filepath.Join("/var/tmp", "testkit-slots", strconv.Itoa(os.Getpid()))
}

// SlotRootEnv names the override.
const SlotRootEnv = "TESTKIT_SLOT_ROOT"
