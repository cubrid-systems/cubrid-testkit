package contain

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// A slot's own network namespace has only loopback, so the machine's own name
// has to resolve to loopback inside it.
//
// The namespace is what makes a slot cost nothing to configure: every slot keeps
// the shipped port 1523 and no conf is rewritten. What it also does is take away
// every address but 127.0.0.1, and cub_server resolves the machine's hostname to
// find the master. If that name maps to an address on a real interface, the
// address does not exist in here:
//
//	Cannot make connection to master server on host "b06395d3d82b".
//	... Network is unreachable
//	Server status is DOWN.
//
// Which is why this was invisible on a host and fatal in a container. Debian and
// Ubuntu write `127.0.1.1 <hostname>` into /etc/hosts, and 127.0.1.1 is
// loopback, so a slot on such a machine reaches it. Docker writes the
// container's eth0 address instead -- `172.17.0.2 b06395d3d82b` -- and a slot
// cannot. Measured: nineteen of twenty-two cases failed at `cubrid server start`
// in the CI image and every one of them was this.
//
// The fix is the same shape as the bash-compatible /bin/sh above it: a bind
// mount inside this run's mount namespace, so the machine's own /etc/hosts is
// untouched and nothing outside the run sees a different one.
func hostsFix(dir string) (string, error) {
	host, err := os.Hostname()
	if err != nil || host == "" || host == "localhost" {
		return "", nil
	}
	if loopbackAlready(host) {
		// Every ordinary QA machine, so the common case does no work.
		return "", nil
	}
	src, err := os.ReadFile("/etc/hosts")
	if err != nil {
		// No /etc/hosts to fix is not a reason to refuse the run: the name may
		// resolve some other way, and a case will say so far more clearly than a
		// refusal here would.
		return "", nil
	}
	fixed := rewriteHosts(string(src), host)
	out := filepath.Join(dir, "hosts")
	if err := os.WriteFile(out, []byte(fixed), 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", out, err)
	}
	return out, nil
}

// loopbackAlready reports whether the name already resolves only to loopback,
// which is what a slot can reach.
//
// Only: a name with both a loopback and a routable address still hands the
// routable one out often enough to fail a case some of the time, and a case that
// fails one run in three is worse than one that fails every time.
func loopbackAlready(host string) bool {
	addrs, err := net.LookupHost(host)
	if err != nil || len(addrs) == 0 {
		return false
	}
	for _, a := range addrs {
		ip := net.ParseIP(a)
		if ip == nil || !ip.IsLoopback() {
			return false
		}
	}
	return true
}

// rewriteHosts drops every existing mapping for host and puts it on 127.0.0.1.
//
// Dropped rather than left in place: /etc/hosts is read in order and the first
// match wins for some resolvers and the last for others, so a file with both
// answers is a file whose answer depends on the resolver.
func rewriteHosts(src, host string) string {
	var out []string
	for _, line := range strings.Split(src, "\n") {
		body := line
		if i := strings.IndexByte(body, '#'); i >= 0 {
			body = body[:i]
		}
		names := strings.Fields(body)
		named := false
		for _, n := range names[min(1, len(names)):] {
			if n == host {
				named = true
				break
			}
		}
		if named {
			continue
		}
		out = append(out, line)
	}
	// First, so that a resolver taking the first match takes this one.
	return "127.0.0.1\t" + host + "\n" + strings.Join(out, "\n")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// bindHosts puts the rewritten file over /etc/hosts for this namespace only.
func bindHosts(dir string) error {
	out, err := hostsFix(dir)
	if err != nil || out == "" {
		return err
	}
	if err := syscall.Mount(out, "/etc/hosts", "", syscall.MS_BIND, ""); err != nil {
		return fmt.Errorf("bind %s over /etc/hosts: %w", out, err)
	}
	return nil
}
