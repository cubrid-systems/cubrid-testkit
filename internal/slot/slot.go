// Package slot divides one machine into places a case can run without seeing
// another case.
//
// Two cases at once collide on the master port, the broker ports, the
// shared-memory ids, cubrid.conf and databases.txt. Every one of those is
// something the suite already writes per instance; what was missing is that they
// were computed per machine. A slot is that computation moved down a level.
//
// This package allocates and refuses. It does not run anything, and it does not
// know what a case is -- which is deliberate, because the failure it exists to
// prevent is arithmetic: two slots quietly sharing a port produce a suite that
// fails at random, and the first thing to get right is that such an allocation
// cannot be produced at all.
//
// docs/concept/beyond-axis.md B-T3.
package slot

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// portsPerSlot is how much of the range one slot consumes: the master, the two
// brokers this suite configures, and headroom for what a case starts on its own.
//
// The headroom is not decoration. A case may start a second broker or a manager,
// and a slot that has only the ports it was promised will collide with its
// neighbour the first time one does.
const portsPerSlot = 10

// Slot is one place to run a case.
type Slot struct {
	// Index is 0-based and stable: slot N always gets the same numbers, so a
	// failure can be reproduced by running that slot alone.
	Index int

	// MasterPort is cubrid_port_id.
	MasterPort int
	// Broker1Port and Broker2Port are BROKER_PORT in the two shipped sections,
	// %query_editor and %BROKER1.
	Broker1Port int
	Broker2Port int

	// MasterSHMID and Broker1SHMID/Broker2SHMID keep the shared-memory segments
	// apart. Ids that collide do not fail; they interfere, and the results look
	// real -- which is why they are allocated rather than defaulted.
	MasterSHMID  int
	Broker1SHMID int
	Broker2SHMID int

	// CUBRID and Databases are this slot's own install and registry. Cases edit
	// cubrid.conf and every case adds and removes a databases.txt entry, so
	// neither can be shared.
	CUBRID    string
	Databases string
}

// EnvID names the slot the way the result files do.
func (s Slot) EnvID() string { return fmt.Sprintf("slot%d", s.Index) }

// Env is the environment a slot's commands run with.
func (s Slot) Env() []string {
	return []string{
		"CUBRID=" + s.CUBRID,
		"CUBRID_DATABASES=" + s.Databases,
	}
}

// Range is a closed interval of ports, written "low-high".
type Range struct{ Low, High int }

// DefaultRange is what a configuration that does not say gets. It starts above
// CUBRID's own default of 1523 so that a slotted run and an ordinary one on the
// same machine do not meet.
var DefaultRange = Range{Low: 15230, High: 15299}

// DefaultSHMBase is where shared-memory ids start. CUBRID's own default is
// 0x01000000-ish territory; this sits clear of it for the same reason.
const DefaultSHMBase = 0x02000000

// ParseRange reads "low-high".
func ParseRange(s string) (Range, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return DefaultRange, nil
	}
	lo, hi, ok := strings.Cut(s, "-")
	if !ok {
		return Range{}, fmt.Errorf("port range %q is not low-high", s)
	}
	l, err := strconv.Atoi(strings.TrimSpace(lo))
	if err != nil {
		return Range{}, fmt.Errorf("port range %q: %w", s, err)
	}
	h, err := strconv.Atoi(strings.TrimSpace(hi))
	if err != nil {
		return Range{}, fmt.Errorf("port range %q: %w", s, err)
	}
	if l < 1 || h > 65535 || l > h {
		return Range{}, fmt.Errorf("port range %q is not a usable interval", s)
	}
	return Range{Low: l, High: h}, nil
}

// Capacity is how many slots a range can hold.
func (r Range) Capacity() int { return (r.High - r.Low + 1) / portsPerSlot }

// Allocate returns n slots, or an error.
//
// It refuses rather than wraps. A range too small for the slots asked for is the
// one way two slots could quietly share a port, and it is a startup failure with
// the arithmetic in the message: the operator asked for something the machine
// cannot give, and a suite that fails at random three hours later is a much
// worse way to find out.
func Allocate(n int, r Range, root string) ([]Slot, error) {
	if n < 1 {
		return nil, fmt.Errorf("parallel_slots must be at least 1, not %d", n)
	}
	if cap := r.Capacity(); n > cap {
		return nil, fmt.Errorf(
			"parallel_slots=%d needs %d ports (%d each) but the range %d-%d holds %d slots; widen parallel_port_range",
			n, n*portsPerSlot, portsPerSlot, r.Low, r.High, cap)
	}
	if root == "" {
		return nil, fmt.Errorf("slots need a root directory to put their installs under")
	}

	slots := make([]Slot, n)
	for i := range slots {
		base := r.Low + i*portsPerSlot
		shm := DefaultSHMBase + i*portsPerSlot
		slots[i] = Slot{
			Index:        i,
			MasterPort:   base,
			Broker1Port:  base + 1,
			Broker2Port:  base + 2,
			MasterSHMID:  shm,
			Broker1SHMID: shm + 1,
			Broker2SHMID: shm + 2,
			CUBRID:       filepath.Join(root, fmt.Sprintf("slot%d", i), "CUBRID"),
			Databases:    filepath.Join(root, fmt.Sprintf("slot%d", i), "databases"),
		}
	}
	return slots, nil
}

// Parameters is what a slot contributes to each configuration file, keyed the
// way deploy.go's iniTargets already names them. It exists so that the values
// live in one place: the allocator decides them and the deployer writes them,
// and nothing in between gets to invent one.
func (s Slot) Parameters() map[string]map[string]string {
	return map[string]map[string]string{
		"cubrid.conf/common": {
			"cubrid_port_id": strconv.Itoa(s.MasterPort),
		},
		"cubrid_broker.conf/broker": {
			"MASTER_SHM_ID": fmt.Sprintf("%d", s.MasterSHMID),
		},
		"cubrid_broker.conf/%query_editor": {
			"BROKER_PORT":        strconv.Itoa(s.Broker1Port),
			"APPL_SERVER_SHM_ID": fmt.Sprintf("%d", s.Broker1SHMID),
		},
		"cubrid_broker.conf/%BROKER1": {
			"BROKER_PORT":        strconv.Itoa(s.Broker2Port),
			"APPL_SERVER_SHM_ID": fmt.Sprintf("%d", s.Broker2SHMID),
		},
	}
}
