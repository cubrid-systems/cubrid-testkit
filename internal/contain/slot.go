package contain

import (
	"context"
	"errors"
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
// docs/project/concept/beyond-axis.md B-T3.
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

// Open creates a namespace and returns it. label names it in errors; root is the
// run's slot root, from NewSlotRoot, where the namespace keeps its scripts.
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
func Open(label, root string) (*Namespace, error) {
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
	// Under the slot root and not /tmp on purpose: this is where the scripts a
	// command runs are written, and Private can give the slot a /tmp that this
	// process cannot see into.
	ns.scratch = filepath.Join(root, "scratch", label)
	if err := os.MkdirAll(ns.scratch, 0o755); err != nil {
		ns.Close()
		return nil, fmt.Errorf("%s: %w%s", label, err, whyMkdirFailed(ns.scratch, err))
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

	// And an address the machine will admit to having.
	//
	// A fresh network namespace has loopback and nothing else, and `hostname -I`
	// reports addresses on every interface *except* loopback -- deliberately, so
	// that it answers "how do I reach this machine" rather than "what does it
	// call itself". In a slot it therefore answers with an empty string, and 108
	// case scripts in this corpus begin with some form of
	//
	//	hostip=$(hostname -I | awk '{print $1}')
	//
	// after which they connect to :port, register a dblink server at [dba].[srv],
	// or hand the empty string to a utility that says "Incorrect hostname
	// format". Every one of those failures is the runner's, produced by the
	// isolation rather than by the case.
	//
	// A dummy interface fixes it and costs nothing: the address is local, so a
	// server bound to the wildcard is reachable on it, and the namespace has
	// nothing else to collide with. 192.0.2.0/24 is TEST-NET-1, reserved by
	// RFC 5737 for documentation, so it cannot be mistaken for a real host in
	// anything a case records. Every slot gets the same address for the same
	// reason every slot keeps the shipped port: a slot should look like a
	// machine, and they are machines that cannot see each other.
	//
	// Best effort. The dummy driver may not be there, and a slot without an
	// address is what every slot had until now -- worse for those 108 cases, no
	// worse than before for the rest.
	if out, err := ns.run(context.Background(), 10*time.Second,
		"ip link add "+slotLink+" type dummy && ip addr add "+SlotAddress+"/24 dev "+slotLink+
			" && ip link set "+slotLink+" up"); err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] %s: no address on %s (%v: %s); "+
			"cases that read `hostname -I` will see an empty string\n",
			label, slotLink, err, strings.TrimSpace(out))
	}
	return ns, nil
}

// SlotAddress is the address a slot answers `hostname -I` with. TEST-NET-1,
// RFC 5737: reserved for documentation, so it cannot be mistaken for a real
// host anywhere a case records it.
const SlotAddress = "192.0.2.1"

// slotLink is the interface that address sits on. Not "eth0": a case that greps
// for a real interface name should not find one that is not real.
const slotLink = "tkslot0"

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

// Command returns a command, not yet started, that runs argv inside this
// namespace. env extends this process's environment, as it does for Channel.
//
// A channel runs a script to its end and hands back what it printed. A process
// a runner talks to while it runs -- sql's executor, one JVM a slot hands a case
// at a time on its standard input -- needs its pipes instead, so the caller
// sets them and starts the command itself.
//
// The group kill exec.Command arranges matters more here than anywhere:
// nsenter forks to put its child in the PID namespace, so the process that
// matters is not the one started here, and a signal to nsenter alone would
// leave it running.
func (n *Namespace) Command(ctx context.Context, dir string, env []string, argv ...string) *osexec.Cmd {
	return exec.Command(ctx, dir, env, n.enter(argv...)...)
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
			return fmt.Errorf("%s: %w%s", n.label, err, whyMkdirFailed(d, err))
		}
	}
	// A comma in any of these would be read as an option separator, and the
	// mount would fail somewhere less obvious than here.
	for _, d := range []string{target, upper, work} {
		if strings.ContainsAny(d, ",:") {
			return fmt.Errorf("%s: %q cannot be an overlay directory: the option string is comma-separated", n.label, d)
		}
	}
	opts := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", target, upper, work)
	why := whyOverlayFailed(target)
	if Volatile() {
		opts = "volatile," + opts
		why += fmt.Sprintf("\n  %s=1 asks for a volatile overlay, which needs Linux 5.10 or later.", SlotVolatileEnv)
	}
	script := fmt.Sprintf("mount -t overlay overlay -o %s %s", opts, target)
	if out, err := n.run(context.Background(), 20*time.Second, script); err != nil {
		return fmt.Errorf("%s: overlay %s: %w: %s%s", n.label, target, err,
			strings.TrimSpace(out), why)
	}
	return nil
}

// Volatile reports whether slot overlays are mounted volatile: every fsync,
// syncfs and sync on the slot's layer returns at once, having done nothing.
//
// What a server waits on at every commit is its log's fsync, and on a SATA SSD
// that is most of a sql case: eight sql slots on one took 1,131 s, the same
// slots with their overlays volatile 338-394 s, and their cases half the time
// CTP's serial run spends on them (docs/project/evidence/sql-native.md §3).
//
// It is sound for a slot because a slot's layer is thrown away at the end, and a
// sync only matters to a machine that goes down: a server killed in the middle of
// a case loses nothing, since what it wrote is in the page cache, and recovery
// reads it back from there. It is still not the default, because the syncs are
// part of the conditions CTP ran under, and a run that skips them is a different
// run.
func Volatile() bool { return os.Getenv(SlotVolatileEnv) == "1" }

// SlotVolatileEnv names the switch for volatile slot overlays.
const SlotVolatileEnv = "TESTKIT_SLOT_VOLATILE"

// whyMkdirFailed names the reason a slot cannot make its own directory, when the
// reason is the one that is invisible from the error.
//
// A user namespace maps one uid. A directory owned by anyone else -- the uid a
// bind mount carries in from the host, say -- is nobody in there, and root
// inside the namespace has no privilege over an unmapped owner. So a path this
// process could write to a moment ago answers "permission denied", and says
// nothing about why.
func whyMkdirFailed(path string, err error) string {
	if !errors.Is(err, os.ErrPermission) {
		return ""
	}
	me := os.Getuid()
	for d := path; d != "/" && d != "."; d = filepath.Dir(d) {
		var st syscall.Stat_t
		if syscall.Lstat(d, &st) != nil {
			continue
		}
		if int(st.Uid) != me {
			return fmt.Sprintf("\n  %s is owned by uid %d and this run is uid %d. Slots run in a "+
				"user namespace\n  that maps only this uid, so anything owned by another one "+
				"cannot be written\n  there. Give the slot root to uid %d, or point "+
				"TESTKIT_SLOT_ROOT somewhere it owns.", d, st.Uid, me, me)
		}
		break
	}
	return ""
}

// whyOverlayFailed turns overlayfs's one message into the reason, when the
// reason is one of the two that are checkable from here.
//
// "wrong fs type, bad option, bad superblock on overlay, missing codepage or
// helper program, or other error" is what the kernel says for every refusal,
// and it says nothing. Both of these were met while getting a run to work in a
// container, and each cost an hour of reading a message that named none of it.
func whyOverlayFailed(target string) string {
	// A lower layer that is itself a mount point is refused. Measured: the same
	// overlay succeeds on a plain directory of the same filesystem, and fails
	// when the directory is a bind mount -- which is what `docker run -v` makes
	// of any path it is given.
	if isMountPoint(target) {
		return fmt.Sprintf("\n  %s is a mount point, and a lower layer cannot be one. "+
			"Mount its parent and leave %s a directory inside it.",
			target, filepath.Base(target))
	}
	// A tmpfs, an overlay: neither can carry another overlay's upper layer.
	if fs := fsTypeOf(target); fs == "overlay" {
		return fmt.Sprintf("\n  %s is on an overlayfs, and an overlay cannot be stacked on one. "+
			"Put it on a real filesystem.", target)
	}
	return ""
}

// isMountPoint reports whether path is where a filesystem is mounted, by asking
// whether it and its parent are on the same device.
func isMountPoint(path string) bool {
	var here, up syscall.Stat_t
	if err := syscall.Lstat(path, &here); err != nil {
		return false
	}
	if err := syscall.Lstat(filepath.Dir(path), &up); err != nil {
		return false
	}
	return here.Dev != up.Dev
}

// fsTypeOf is the filesystem a path is on, or "" when it cannot be read.
func fsTypeOf(path string) string {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return ""
	}
	// The few this code cares about; anything else is reported as unknown
	// rather than guessed at from a number.
	switch st.Type {
	case 0x794c7630: // OVERLAYFS_SUPER_MAGIC
		return "overlay"
	case 0x01021994: // TMPFS_MAGIC
		return "tmpfs"
	}
	return ""
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

// NewSlotRoot makes the directory this run keeps everything it makes per slot
// in: the overlay upper layers and the scripts commands are written into. The
// caller removes it once the slots are closed.
//
// Made fresh every time, because a slot root is only safe to mount on if no
// earlier run has written to it. It used to be named after the process id, and
// that was the wrong key twice over. The runner is contained, so the id is the
// namespace's -- a small number, and one that comes round again -- and nothing
// removed the directory afterwards. A run therefore mounted its $CUBRID overlay
// over the upper layer an earlier run had left, and started with that run's
// writes already in the install. A directory from MkdirTemp cannot have been
// anyone else's, and two runs at once get one each.
//
// Under /var/tmp, and not os.TempDir() which usually is /tmp: Private can give a
// slot a /tmp of its own, and a script written into this process's would then
// not be there when the command went looking. /var/tmp is not the directory
// being replaced.
//
// TESTKIT_SLOT_ROOT moves it -- somewhere this uid owns, or off an overlayfs that
// cannot carry another overlay's upper layer -- and the run's own directory is
// made inside it all the same.
func NewSlotRoot() (string, error) {
	base := os.Getenv(SlotRootEnv)
	if base == "" {
		base = filepath.Join("/var/tmp", "testkit-slots")
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		return "", fmt.Errorf("slot root: %w%s", err, whyMkdirFailed(base, err))
	}
	dir, err := os.MkdirTemp(base, "run-")
	if err != nil {
		return "", fmt.Errorf("slot root: %w%s", err, whyMkdirFailed(base, err))
	}
	return dir, nil
}

// SlotRootEnv names where slot roots are made.
const SlotRootEnv = "TESTKIT_SLOT_ROOT"
