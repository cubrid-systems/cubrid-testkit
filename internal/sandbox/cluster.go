package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
)

// ClusterInfo is what a cluster says it is: the artifact csb would rebuild it
// from.
//
// # Why a verdict needs it
//
// This suite's answer is "the two nodes agreed" or "they did not", and neither
// means anything without the pair it was asked of. A difference found on a
// rootless podman pair built from a develop tree of 2026-09-21 is a different
// claim from the same difference on two machines, and the finding documents in
// evidence/ha all carry a *Trees* line for exactly that reason -- written by
// hand, from memory, after the run.
//
// So the run reads it instead. Everything here comes from `cluster describe`,
// which csb calls the reproducible artifact.
type ClusterInfo struct {
	Cluster string `json:"cluster"`
	DB      string `json:"db"`
	Backend string `json:"backend"`
	Image   string `json:"image"`
	Network string `json:"network"`
	Preset  string `json:"preset"`
	// PingHost is the witness a node pings to tell "the peer is gone" from "I
	// am gone", and it is why a split brain is reachable on one cluster and not
	// another (findings/split-brain.md in cluster-sandbox).
	PingHost string `json:"ping_host"`
	PingMode string `json:"ping_mode"`
	Engine   struct {
		Version string `json:"version"`
		Build   string `json:"build"`
		Commit  string `json:"commit"`
		BuiltAt string `json:"built_at"`
		Kind    string `json:"kind"`
		Path    string `json:"path"`
	} `json:"engine"`
	Nodes []struct {
		Name string `json:"name"`
		Kind string `json:"kind"`
		Role string `json:"role"`
	} `json:"nodes"`
}

// Artifact reads it. Named for csb's own phrase -- `cluster describe` prints
// what it calls the reproducible artifact -- and not `Cluster`, which is
// already the field naming which cluster this CLI talks to.
func (c *CLI) Artifact(ctx context.Context) (ClusterInfo, error) {
	env, err := c.call(ctx, "cluster", "describe")
	if err != nil {
		return ClusterInfo{}, err
	}
	var info ClusterInfo
	if jerr := json.Unmarshal(env.Data, &info); jerr != nil {
		return ClusterInfo{}, fmt.Errorf("sandbox: reading what the cluster says it is: %w", jerr)
	}
	return info, nil
}
