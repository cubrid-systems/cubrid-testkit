package hareplsuite

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
func openBoard(ctx context.Context, cfg *conf.Config, shards []*runShard,
	scenario string, wait time.Duration, addKey bool, resetEvery string, cases int,
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
	// The machine panel was empty on this suite's page, because only the disk
	// and memory a run actually competes for are worth showing and nobody had
	// said which those are here. They are not $CUBRID: this runner writes
	// nothing to the host's install. They are csb's state root, where every
	// pair's volumes and copy logs live -- the directory that grew to 53 GB
	// across eleven pairs and took the filesystem to 98%.
	board.Watch(csbStateRoot(), "", 0)
	watchClusters(ctx, board, shards)
	names := make([]string, 0, len(shards))
	for _, sh := range shards {
		board.Lane(sh.cluster, "pair "+sh.cluster)
		names = append(names, sh.cluster)
	}
	board.Setup([]status.Setting{
		{Group: "suite", Key: "task", Value: "ha_repl"},
		{Group: "suite", Key: "scenario", Value: scenario},
		{Group: "suite", Key: ClusterKey, Value: strings.Join(names, ", "),
			Note: "one pair per shard; the corpus is dealt a directory at a time"},
		{Group: "suite", Key: WaitKey, Value: fmt.Sprint(int(wait.Seconds())),
			Default: fmt.Sprint(int(WaitDefault.Seconds())),
			Note:    "seconds, and CTP's own key and default; the wait itself is a poll, never a sleep"},
		{Group: "suite", Key: AddKeyKey, Value: yesNo(addKey), Default: "no",
			Note: "a CREATE TABLE with no key gets one of its own, so its rows can replicate at all"},
		{Group: "suite", Key: ResetKey, Value: resetEvery, Default: "case",
			Note: "per case is what makes a run reproducible"},
		{Group: "suite", Key: ResumeKey, Value: yesNo(cfg.Bool(ResumeKey, false)), Default: "no",
			Note: "skip the cases a previous run judged; never resumes a wait_timeout"},
	})
	// The artifact is read once per pair -- it is what the cluster was built
	// from and does not change under a run -- and the health is read on a
	// ticker. One sampler each, so a pair that stops answering makes its own
	// panel stale rather than every panel.
	done := make(chan struct{})
	for _, sh := range shards {
		static := clusterRows(ctx, sh.cli, sh.cluster, sh.pair)
		go func(cli *sandbox.CLI, static status.Pair) {
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
		}(sh.cli, static)
	}
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

// csbStateRoot is where cluster-sandbox keeps what it stands up: one directory
// per cluster, holding its describe artifact and every node's filesystem.
//
// It is read here rather than asked of csb because the page needs it before the
// first call, and because it is a documented default with one override
// (`CSB_HOME`) rather than something a cluster reports about itself.
func csbStateRoot() string {
	if h := strings.TrimSpace(os.Getenv("CSB_HOME")); h != "" {
		return h
	}
	if h, err := os.UserHomeDir(); err == nil {
		return filepath.Join(h, ".local", "share", "csb")
	}
	return ""
}

// watchClusters keeps the page's machine rows filled with what is standing on
// each machine.
//
// # Why the page asks csb rather than the filesystem
//
// A cluster's size is knowable by walking `~/.local/share/csb`, and that was
// the first version. It is wrong for one reason that decides it: the page has
// to be able to name a machine that is not this one, and this process cannot
// walk another machine's disk. `cluster ls` is the answer to the same question
// wherever it is asked, and the artifact already records which machine a node
// is on.
//
// # And why it is a slow ticker
//
// `cluster ls` walks every cluster's tree to size it. That is cheap beside a
// case and pointless at a case's cadence: a pair grows over a run, not between
// two statements. Thirty seconds is often enough to watch a disk fill and rare
// enough to cost nothing.
func watchClusters(ctx context.Context, board *status.Board, shards []*runShard) {
	mine := map[string]bool{}
	for _, sh := range shards {
		mine[sh.cluster] = true
	}
	sample := func() {
		sctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		all, err := sandbox.Bind("").Clusters(sctx)
		if err != nil {
			board.MachineRow(status.Machine{
				Name: status.ThisMachine(),
				Note: "csb would not list the clusters on this machine: " + err.Error(),
			})
			return
		}
		byMachine := map[string][]status.MachineCluster{}
		for _, c := range all {
			// Running, or costing. A destroyed cluster keeps its describe
			// artifact on purpose -- a kilobyte saying what evidence was
			// produced on -- and a page that lists nineteen of those beside
			// eight real pairs has buried the eight. A cluster that is down but
			// still holding gigabytes is the other case and must stay: that is
			// exactly the one somebody needs to find.
			if c.Containers == 0 && c.Bytes < 1<<20 {
				continue
			}
			run, _ := c.Run()
			row := status.MachineCluster{
				Name: c.Name, Containers: c.Containers, Bytes: c.Bytes,
				Run: run, Mine: mine[c.Name],
			}
			// A cluster whose artifact predates the host field, or that this
			// tool did not create, belongs to the machine asking -- it is the
			// one whose state directory it was found in. Guessing wider would
			// be inventing a machine.
			hosts := c.Hosts
			if len(hosts) == 0 {
				hosts = []string{status.ThisMachine()}
			}
			for _, h := range hosts {
				byMachine[h] = append(byMachine[h], row)
			}
		}
		for host, rows := range byMachine {
			board.MachineRow(status.Machine{Name: host, Clusters: rows})
		}
	}
	go func() {
		sample()
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				sample()
			}
		}
	}()
}
