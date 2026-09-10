package contain

import (
	"strings"
	"testing"
)

// The failure this exists to stop: a slot's network namespace has only
// loopback, and Docker maps the container's hostname to its eth0 address, so
// cub_server's connection to the master is "Network is unreachable" and the
// server goes down. Nineteen of twenty-two cases in the CI image failed this way.
func TestTheMachineNameIsPutOnLoopback(t *testing.T) {
	const docker = "127.0.0.1\tlocalhost\n" +
		"::1\tlocalhost ip6-localhost ip6-loopback\n" +
		"172.17.0.2\tb06395d3d82b\n"
	got := rewriteHosts(docker, "b06395d3d82b")

	if !strings.HasPrefix(got, "127.0.0.1\tb06395d3d82b\n") {
		t.Errorf("the machine name is not first on loopback:\n%s", got)
	}
	// The routable answer must be gone, not merely outranked: /etc/hosts is read
	// in order and the first match wins for some resolvers and the last for
	// others, so a file with both answers answers differently per resolver.
	if strings.Contains(got, "172.17.0.2") {
		t.Errorf("the routable mapping survived:\n%s", got)
	}
	// Everything else is left alone.
	for _, keep := range []string{"127.0.0.1\tlocalhost", "ip6-loopback"} {
		if !strings.Contains(got, keep) {
			t.Errorf("an unrelated line was dropped: %q\n%s", keep, got)
		}
	}
}

// An alias on a shared line goes with it: leaving the line would leave the
// routable address answering for the name.
func TestAnAliasOnASharedLineGoesToo(t *testing.T) {
	got := rewriteHosts("10.0.0.5\tbuilder builder.example.com\n192.168.1.1\tgateway\n", "builder")
	if strings.Contains(got, "10.0.0.5") {
		t.Errorf("a shared line kept the routable address:\n%s", got)
	}
	if !strings.Contains(got, "192.168.1.1\tgateway") {
		t.Errorf("an unrelated host was dropped:\n%s", got)
	}
}

// A name only in a comment is not a mapping.
func TestACommentIsNotAMapping(t *testing.T) {
	got := rewriteHosts("10.0.0.5\tother\t# was myhost once\n", "myhost")
	if !strings.Contains(got, "10.0.0.5\tother") {
		t.Errorf("a line was dropped for a name that only appears in its comment:\n%s", got)
	}
}

// The address column is not a name. A machine called "127.0.0.1" is not a thing,
// but a rewrite that matched the first field would drop every line.
func TestTheAddressColumnIsNotSearched(t *testing.T) {
	got := rewriteHosts("127.0.0.1\tlocalhost\n10.0.0.5\thost5\n", "127.0.0.1")
	if !strings.Contains(got, "10.0.0.5\thost5") {
		t.Errorf("matching the address column dropped an unrelated line:\n%s", got)
	}
}

// A machine whose name already resolves only to loopback needs no work, which is
// every ordinary QA machine -- Debian and Ubuntu write 127.0.1.1 <hostname>.
func TestLoopbackAlreadyIsLeftAlone(t *testing.T) {
	if !loopbackAlready("localhost") {
		t.Error("localhost was not recognised as loopback-only")
	}
}
