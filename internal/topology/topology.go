// Package topology reads the machine a run uses out of the configuration.
//
// A run uses one (ADR-014). The instance keys can describe several, because the
// frozen configuration allows it and because a reader has to be told which ones
// are being left out -- but choosing between them, deploying to all of them and
// spreading cases across them is fleet management, and that is the operations
// layer's job rather than this one's.
//
// The shape comes from two families of key:
//
//	default.<role>.<property>          applies to every instance
//	env.instance<N>.<role>.<property>  applies to instance N, and wins
//
// Both are frozen (docs/concept/external-surface-freeze.md §2-3). <property> is
// not drawn from a list: whatever follows the role is carried through, which is
// how an arbitrary cubrid.conf parameter reaches the remote machine.
package topology

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"

	"github.com/cubrid-systems/cubrid-testkit/internal/conf"
)

// Roles that appear under default.* and env.instanceN.*.
const (
	RoleSSH          = "ssh"
	RoleCUBRID       = "cubrid"
	RoleBroker1      = "broker1"
	RoleBroker2      = "broker2"
	RoleBrokerCommon = "brokercommon"
	RoleHA           = "ha"
	RoleCM           = "cm"
	RoleMaster       = "master"
	RoleSlave        = "slave"
)

// Roles is every role the frozen configuration uses.
var Roles = []string{
	RoleSSH, RoleCUBRID, RoleBroker1, RoleBroker2,
	RoleBrokerCommon, RoleHA, RoleCM, RoleMaster, RoleSlave,
}

var instancePattern = regexp.MustCompile(`^env\.instance([0-9]+)\.`)

// Instance is one machine in the run, identified the way CTP identifies it.
type Instance struct {
	ID int

	// name overrides the generated id. It is set only for the local instance,
	// which CTP calls "local" rather than "envN".
	name string

	// roles holds the merged property maps: defaults first, then the instance's
	// own keys on top.
	roles map[string]map[string]string
}

// EnvID is what appears in the frozen markers, as in "[ENV START] env1".
func (i *Instance) EnvID() string {
	if i.name != "" {
		return i.name
	}
	return fmt.Sprintf("env%d", i.ID)
}

// IsLocal reports whether this is the machine the runner is on.
func (i *Instance) IsLocal() bool { return i.name == LocalName }

// LocalName is the environment id CTP gives a run with no configured machines.
const LocalName = "local"

// Local returns the single instance a configuration with no env.instanceN keys
// describes: this machine, carrying whatever default.* roles were set.
//
// CTP built this in the Context constructor and never wrote it down. A run that
// names no machines is not a misconfiguration -- it is how the shell suite is
// driven against the engine on the machine you are sitting at.
func Local(cfg *conf.Config) *Instance {
	inst := &Instance{name: LocalName, roles: map[string]map[string]string{}}
	for _, role := range Roles {
		inst.roles[role] = cfg.Prefixed("default." + role)
	}
	return inst
}

// Role returns the merged properties for a role. The map is a copy.
func (i *Instance) Role(role string) map[string]string {
	out := map[string]string{}
	for k, v := range i.roles[role] {
		out[k] = v
	}
	return out
}

// SSH is the connection to this instance.
type SSH struct {
	Host     string
	Port     string
	User     string
	Password string
	// Related lists the extra hosts an HA instance touches, from
	// env.instanceN.ssh.relatedhosts.
	Related string
}

// SSH reads the ssh role.
func (i *Instance) SSH() SSH {
	p := i.roles[RoleSSH]
	return SSH{
		Host:     p["host"],
		Port:     firstNonEmpty(p["port"], "22"),
		User:     p["user"],
		Password: p["pwd"],
		Related:  p["relatedhosts"],
	}
}

// From builds the instances a configuration describes, ordered by id.
//
// An instance exists because at least one env.instanceN.* key mentions it. A
// configuration with no such key describes no instances, which is correct for the
// local-only tasks.
func From(cfg *conf.Config) ([]*Instance, error) {
	ids := map[int]bool{}
	for _, k := range cfg.Keys() {
		if m := instancePattern.FindStringSubmatch(k); m != nil {
			n, err := strconv.Atoi(m[1])
			if err != nil {
				return nil, fmt.Errorf("instance number in %q: %w", k, err)
			}
			ids[n] = true
		}
	}

	ordered := make([]int, 0, len(ids))
	for id := range ids {
		ordered = append(ordered, id)
	}
	sort.Ints(ordered)

	out := make([]*Instance, 0, len(ordered))
	for _, id := range ordered {
		inst := &Instance{ID: id, roles: map[string]map[string]string{}}
		for _, role := range Roles {
			merged := cfg.Prefixed("default." + role)
			for k, v := range cfg.Prefixed(fmt.Sprintf("env.instance%d.%s", id, role)) {
				merged[k] = v
			}
			inst.roles[role] = merged
		}
		out = append(out, inst)
	}
	return out, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
