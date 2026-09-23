package hareplsuite

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cubrid-systems/cubrid-testkit/internal/conf"
	"github.com/cubrid-systems/cubrid-testkit/internal/sandbox"
	"github.com/cubrid-systems/cubrid-testkit/internal/status"
)

// boardSlot is the one lane this suite has. `sql` and `shell` run cases on
// several slots at once and the page exists to say which one is stuck; this
// runs them one at a time, because a case's verdict is about a pair and two
// cases sharing a pair is what made an earlier run's numbers a patchwork.
const boardSlot = "pair"

// openBoard is the status page, when status_http asks for one.
//
// # What it adds over the other suites' page
//
// Theirs shows the run and the machine it competes for. That is the whole
// story when a case asks one node a question. It is not the story here: this
// suite's verdict is two nodes disagreeing, so the page has to show the pair
// the verdict is about -- roles, states, each node's `fail_counter` and lag,
// the faults in force, and the artifact the cluster was built from.
//
// Every one of those is on the panel because its absence cost something. A
// pair that stopped serving turned an hour of a run into `wait_timeout` per
// case, honestly reported and unnoticed. A repair this suite makes moves
// `fail_counter` on purpose, and a reader who finds it moved afterwards has no
// way to tell that from the engine doing it. And every finding in
// evidence/ha carries a *Trees* line written from memory after the fact, which
// is what the artifact rows are for.
func openBoard(ctx context.Context, cfg *conf.Config, cli *sandbox.CLI, pair *sandbox.Pair,
	cluster, scenario string, wait time.Duration, addKey bool, resetEvery string, cases int,
) (*status.Board, func()) {
	addr := status.Addr(cfg.GetOr(StatusKey, ""))
	if addr == "" {
		return nil, func() {}
	}
	board := status.New(cases)
	where, stop, err := board.Open(addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[INFO] no status page: %v\n", err)
		return nil, func() {}
	}
	fmt.Fprintf(os.Stderr, "[INFO] status page at http://%s/\n", where)
	board.Lane(boardSlot, "pair")
	board.Setup([]status.Setting{
		{Group: "suite", Key: "task", Value: "ha_repl"},
		{Group: "suite", Key: "scenario", Value: scenario},
		{Group: "suite", Key: ClusterKey, Value: cluster},
		{Group: "suite", Key: WaitKey, Value: fmt.Sprint(wait.Milliseconds()), Default: "60000",
			Note: "the bound on the marker's crossing; the wait itself is a poll, never a sleep"},
		{Group: "suite", Key: AddKeyKey, Value: yesNo(addKey), Default: "no",
			Note: "a CREATE TABLE with no key gets one of its own, so its rows can replicate at all"},
		{Group: "suite", Key: ResetKey, Value: resetEvery, Default: "case",
			Note: "per case is what makes a run reproducible"},
		{Group: "suite", Key: ResumeKey, Value: yesNo(cfg.Bool(ResumeKey, false)), Default: "no",
			Note: "skip the cases a previous run judged; never resumes a wait_timeout"},
	})
	// The artifact is read once -- it is what the cluster was built from and
	// does not change under a run -- and the health is read on a ticker.
	static := clusterRows(ctx, cli, cluster, pair)
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for {
			board.Pair(samplePair(ctx, cli, static))
			select {
			case <-done:
				return
			case <-t.C:
			}
		}
	}()
	return board, func() {
		close(done)
		stop()
	}
}

// clusterRows is the part of the panel that does not move.
func clusterRows(ctx context.Context, cli *sandbox.CLI, cluster string, pair *sandbox.Pair) status.Pair {
	p := status.Pair{Cluster: cluster, DB: pair.DB}
	info, err := cli.Artifact(ctx)
	if err != nil {
		p.Note = "the cluster would not describe itself: " + err.Error()
		return p
	}
	p.Backend, p.Image, p.Network = info.Backend, info.Image, info.Network
	if info.PingHost != "" || info.PingMode != "" {
		p.Ping = strings.TrimSpace(info.PingMode + " " + info.PingHost)
	}
	p.Engine = strings.TrimSpace(info.Engine.Build + " " + info.Engine.Kind)
	if p.DB == "" {
		p.DB = info.DB
	}
	return p
}

// samplePair reads what the group says about itself right now.
//
// It asks csb rather than the nodes: the same reading the operator would get
// from `csb ha status`, so the page and the terminal cannot disagree about
// whether the pair is serving.
func samplePair(ctx context.Context, cli *sandbox.CLI, static status.Pair) *status.Pair {
	p := static
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	nodes, err := cli.HAStatus(ctx)
	if err != nil {
		p.Note = "the group would not say how it is doing: " + err.Error()
		return &p
	}
	for _, n := range nodes {
		p.Nodes = append(p.Nodes, status.PairNode{
			Name: n.Name, Live: n.Live, Role: n.Role, State: n.ServerState, Built: n.CreatedRole,
			Applied: n.Replication.AppliedPageID, Copied: n.Replication.CopiedPageID,
			ApplyLag: n.Replication.ApplyLagPages, CopyLag: n.Replication.CopyLagPages,
			Fail: n.Replication.FailCounter,
		})
	}
	// Said rather than left to the reader's arithmetic: none and two are
	// different failures, and both are worth a sentence.
	switch active := sandbox.Actives(nodes); len(active) {
	case 0:
		p.Note = "no node is active, so nothing this run measures means anything until it settles"
	case 1:
	default:
		p.Note = "two nodes call themselves active: " + strings.Join(active, ", ")
	}
	if faults, ferr := cli.Faults(ctx); ferr == nil {
		for _, f := range faults {
			p.Faults = append(p.Faults, f.String())
		}
	}
	return &p
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
