package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// The engine's opinion of itself, which is not the oracle.
//
// This is here to be contradicted. `SameOnBothNodes` asks the data; this asks
// the engine how it thinks it is doing, and the whole of P5 is that the two can
// disagree -- after a healed partition there is a permanent difference over
// which every gauge reads healthy (design/module-ha.md §4, and
// cluster-sandbox `findings/active-active-window.md`). The same pattern has
// already been measured twice in this suite without a fault at all: an
// object-domain column and a trigger's owner both differ while `fail_counter`
// stays where it was and the applier stays `working`.
//
// So a verdict is never taken from here. What this is for is saying, in a
// finding, exactly what the pair was claiming about itself while it was wrong.

// NodeHealth is one node's line in `ha status`.
type NodeHealth struct {
	Name        string `json:"name"`
	Live        bool   `json:"live"`
	Role        string `json:"role"`
	ServerState string `json:"server_state"`
	// CreatedRole is what the node was built as, which is how an inverted
	// topology is told from an original one: after a failover `role` is a
	// question about now and this is a question about then.
	CreatedRole string      `json:"created_role"`
	Replication Replication `json:"replication"`
}

// Replication is the applier's own account of itself, sampled from
// `db_ha_apply_info`.
//
// FailCounter is the number this project's inspection leans on
// (design/05-inspect.md §2) and the number that does not move when the two
// databases disagree.
type Replication struct {
	CopiedPageID  int64  `json:"copied_pageid"`
	AppliedPageID int64  `json:"applied_pageid"`
	ApplyLagPages int64  `json:"apply_lag_pages"`
	CopyLagPages  int64  `json:"copy_lag_pages"`
	FailCounter   int64  `json:"fail_counter"`
	Source        string `json:"source"`
	Rows          int    `json:"rows"`
}

// HAStatus is what the group says about itself now.
func (c *CLI) HAStatus(ctx context.Context) ([]NodeHealth, error) {
	env, err := c.call(ctx, "ha", "status")
	if err != nil {
		return nil, err
	}
	var data struct {
		Nodes []NodeHealth `json:"nodes"`
	}
	if jerr := json.Unmarshal(env.Data, &data); jerr != nil {
		return nil, fmt.Errorf("sandbox: reading the group's own status: %w", jerr)
	}
	return data.Nodes, nil
}

// Actives names the nodes calling themselves active.
//
// Two is a split brain and zero is a group that has not settled; both are
// states a caller waits out or fails on, and neither is visible from a reading
// that only asks "is the master up".
func Actives(nodes []NodeHealth) []string {
	var out []string
	for _, n := range nodes {
		if strings.EqualFold(n.Role, "active") {
			out = append(out, n.Name)
		}
	}
	return out
}

// Healthy reports whether every gauge reads well: one active node, every node
// live, no applier failure counted anywhere.
//
// The name is the engine's claim and not this package's finding. A pair can be
// Healthy and hold two different databases, which is the sentence this whole
// file exists to make sayable.
func Healthy(nodes []NodeHealth) bool {
	if len(Actives(nodes)) != 1 {
		return false
	}
	for _, n := range nodes {
		if !n.Live || n.Replication.FailCounter > 0 {
			return false
		}
	}
	return true
}
