package contain

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cubrid-systems/cubrid-testkit/internal/exec"
)

// Slot is one place to run cases: a namespace of its own, with the install and
// the registry behind overlays of their own, and the environment every command
// run there needs.
type Slot struct {
	Label string
	NS    *Namespace
	Env   []string
}

// Channel opens a channel into the slot. A runner that has to reach the slot
// while a case holds one channel -- a timeout monitor -- opens a second.
func (s *Slot) Channel() exec.Channel { return s.NS.Channel("", s.Env...) }

// OpenSlots opens n places to run a case, each in namespaces of its own.
//
// What a slot needs to differ in used to be a list -- ports, shared-memory ids,
// the install, the registry -- and each entry was somewhere the suite already
// wrote. Namespaces answer all of them at once and without writing anything: a
// network namespace gives every slot the whole port space so each runs on the
// shipped 1523, an IPC namespace keeps the segments apart, a PID namespace makes
// `ps -e` mean this slot, and an overlay makes $CUBRID writable per slot without
// copying its 323 MB.
//
// Nothing is reconfigured, which is the point. A slotted run's conf files and
// log lines are the ones a serial run produces.
//
// mount, when it is not nil, is called for each slot once its install is behind
// its overlay and before anything runs in it: a runner that needs a mount of
// its own per slot makes it there. The returned func closes every slot and
// removes what they wrote.
func OpenSlots(n int, mount func(i int, s *Slot) error) ([]*Slot, func(), error) {
	if !Active() {
		return nil, nil, fmt.Errorf("parallel_slots needs the runner contained; set %s=1", Env)
	}
	root, err := NewSlotRoot()
	if err != nil {
		return nil, nil, err
	}

	var slots []*Slot
	closeAll := func() {
		for _, s := range slots {
			s.NS.Close()
		}
		// The upper layers go with the slots, and with them whatever the last
		// case in each slot left in $CUBRID/log. Nothing reads them after this:
		// what a case wrote is kept, when a runner asks for it, by copying it to
		// the result tree as the case finishes. Left here they are only disk --
		// the next run gets a root of its own.
		if err := os.RemoveAll(root); err != nil {
			fmt.Printf("[WARN] cannot remove the slot root %s: %v\n", root, err)
		}
	}
	for i := 0; i < n; i++ {
		label := fmt.Sprintf("slot%d", i)
		ns, err := Open(label, root)
		if err != nil {
			closeAll()
			return nil, nil, err
		}
		s := &Slot{Label: label, NS: ns}
		slots = append(slots, s)

		// $CUBRID and the registry are the two trees a case writes to. The
		// registry takes an overlay of its own only when it is outside the
		// install; where CUBRID puts it by default -- $CUBRID/databases, which
		// is also where CTP's own reset cleans and restores it -- the install's
		// overlay already covers it, and a second overlay on a subdirectory of
		// the first would nest them for nothing.
		var covered []string
		for _, dir := range []string{os.Getenv("CUBRID"), os.Getenv("CUBRID_DATABASES")} {
			if dir == "" || under(covered, dir) {
				continue
			}
			if err := ns.Overlay(dir, filepath.Join(root, label, filepath.Base(dir))); err != nil {
				closeAll()
				return nil, nil, err
			}
			covered = append(covered, dir)
		}
		if mount != nil {
			if err := mount(i, s); err != nil {
				closeAll()
				return nil, nil, err
			}
		}
		// cub_master listens on a Unix domain socket named after its port --
		// $CUBRID_TMP/CUBRID<port>, and /tmp when that is unset. Every slot keeps
		// the shipped port, because the network namespace lets it, so without
		// this they would all want /tmp/CUBRID1523: four masters over one socket
		// is four masters that do not start, and every case that wanted a server
		// fails with "Could not connect to master server on localhost".
		//
		// The engine's own variable rather than a private /tmp. A mount would
		// also take away the directory the scripts are written into, and it
		// would isolate a /tmp that cases are entitled to share.
		tmp, err := slotTmp(label)
		if err != nil {
			closeAll()
			return nil, nil, err
		}
		s.Env = []string{
			"CUBRID_TMP=" + tmp,
			// A user namespace maps one uid, so a case that unpacks an archive
			// recorded with somebody else's ownership cannot restore it: tar
			// prints "Cannot change ownership to uid 1001, gid 1001: Invalid
			// argument" and exits non-zero. The files are there -- it is the exit
			// status that fails the case, and 51 case scripts in this corpus
			// unpack something.
			//
			// On a QA machine the run is a real account and the chown succeeds,
			// so this is the isolation's bill and not the case's. GNU tar reads
			// TAR_OPTIONS, and --no-same-owner is what tar does for an ordinary
			// user anyway: extract the files, own them yourself. No case here
			// asserts anything about ownership.
			"TAR_OPTIONS=--no-same-owner",
		}
		// And a linker that keeps the libraries the command line names, where
		// the machine's would drop them. Measured per run rather than assumed,
		// and absent on a toolchain that needs no correction -- see GccShim.
		if bin, gerr := GccShim(filepath.Join(tmp, "bin")); gerr != nil {
			closeAll()
			return nil, nil, gerr
		} else if bin != "" {
			s.Env = append(s.Env, "PATH="+bin+":"+os.Getenv("PATH"))
		}
	}
	return slots, closeAll, nil
}

// sunPathMax is the size of sockaddr_un.sun_path, and the reason a slot's
// CUBRID_TMP cannot simply live under the slot root. A run whose working
// directory is deep enough produces a path the kernel cannot hold a socket at,
// and the engine says so -- "The $CUBRID_TMP is too long" -- on every command,
// after which the case fails on a comparison rather than on the real cause.
const sunPathMax = 108

// slotTmp is the directory a slot's master keeps its socket in.
//
// $CUBRID/tmp first, because it is already this slot's own -- $CUBRID is behind
// a per-slot overlay, so two slots writing CUBRID1523 there do not meet -- and
// because staying under $CUBRID keeps the cases' own normalisation working:
// several compare output holding a socket path against an answer that says
// "${CUBRID}/...", and a path outside $CUBRID is a path their sed does not
// rewrite. Observed on _08_shard/_13_shard_command, whose answer expects
// ${CUBRID}/var/CUBRID_SOCK and got /var/tmp/tk<pid>/slot1.
//
// Not $CUBRID/var/CUBRID_SOCK, which is what the engine itself picks when
// CUBRID_TMP says nothing -- broker_filename.c's FID_SOCK_DIR and pl_comm.c
// both fall back to it -- and which would make that case's answer match
// exactly. Measured, and it does not work: the per-case reset runs
// `rm -rf ${CUBRID}/var/*`, so a socket directory there is gone after the first
// case, the master cannot create its socket, and a two-case run went from 26
// seconds to 426 with both cases failing and no shard output at all. The reset
// leaves $CUBRID/tmp alone. So _13_shard_command and _06_issues/_24_1h/cbrd_25076,
// which assert the engine's default location, cannot be satisfied here.
//
// /var/tmp is the fallback and not the default. It exists because sun_path is
// 108 bytes: an install deep enough produces a socket path the kernel cannot
// hold, the engine says "The $CUBRID_TMP is too long" on every command, and the
// case then fails on a comparison rather than on the real cause. Deliberately
// not derived from TESTKIT_SLOT_ROOT, which is where the overlays go and is
// often long.
func slotTmp(label string) (string, error) {
	// The longest name the engine puts here is the socket, CUBRID<port>.
	const leaf = "/CUBRID65535"
	if home := os.Getenv("CUBRID"); home != "" {
		dir := filepath.Join(home, "tmp")
		if len(dir)+len(leaf) < sunPathMax {
			if err := os.MkdirAll(dir, 0o1777); err != nil {
				return "", fmt.Errorf("%s: %w", label, err)
			}
			return dir, nil
		}
	}
	dir := filepath.Join("/var/tmp", fmt.Sprintf("tk%d", os.Getpid()), label)
	if n := len(dir) + len(leaf) + 1; n > sunPathMax {
		return "", fmt.Errorf("%s: CUBRID_TMP would be %s, and a socket under it needs %d of the %d bytes a Unix socket path has",
			label, dir, n, sunPathMax)
	}
	if err := os.MkdirAll(dir, 0o1777); err != nil {
		return "", fmt.Errorf("%s: %w", label, err)
	}
	return dir, nil
}

// under reports whether dir is one of parents or sits inside one of them.
//
// Comparing cleaned strings is not enough: "/a/bc" starts with "/a/b" and is
// not inside it. filepath.Rel answers the question the mount actually asks --
// is there a path from the parent down to dir that never climbs.
func under(parents []string, dir string) bool {
	d, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	for _, p := range parents {
		a, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(a, d)
		if err != nil {
			continue
		}
		if rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..") {
			return true
		}
	}
	return false
}
