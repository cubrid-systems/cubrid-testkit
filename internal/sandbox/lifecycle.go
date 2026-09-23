package sandbox

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Standing a cluster up, and taking it down again.
//
// # Why this is here at all
//
// ADR-014 says the system under test has a topology and the runner does not
// have a fleet, and ADR-022 turned that into "a topology is asked for, never
// built here". Neither forbids asking: what they forbid is this runner owning
// node lifecycle, addressing and fault injection itself. Every function below
// is one `csb` call.
//
// # The line that decides what may be destroyed
//
// **A run destroys what it created, and only that.** Not "the clusters it used"
// -- an operator who stands a pair up to run three suites against it must still
// have it after the first. So the two cases are told apart in the
// configuration, before anything runs, rather than guessed at the end:
// `sandbox_cluster` names pairs that already exist and are never touched, and
// `sandbox_pairs` asks for pairs that are this run's to remove.
//
// # And the claim is written down where a dead run cannot lose it
//
// Ownership lives in the cluster's own describe artifact, as a label, not in
// this process's memory. A run that is killed -- and this suite has been killed
// four times in two days by things that had nothing to do with it -- leaves
// clusters that still say whose they were, so they can be found afterwards.
// `csb cluster ls` prints the label.

// RunLabel is the label key a run claims its clusters with. The value is the
// run's name.
const RunLabel = "testkit_run"

// NewRunName makes a name that is unique, short, and legible in a container
// name -- because that is what it becomes: csb derives the network, the
// containers and the database name from the cluster name, and those have to be
// lowercase letters, digits and dashes starting with a letter.
//
// The date is in it so that an orphan found a week later says when it was made
// without anyone looking anything up. The four random characters are what make
// two runs on the same day distinct; a counter would need state this has none
// of, and a PID is reused.
func NewRunName(at time.Time) string {
	var b [2]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("tk%s", at.Format("0102150405"))
	}
	return fmt.Sprintf("tk%s%s", at.Format("0102"), hex.EncodeToString(b[:]))
}

// PairName is the cluster name for one pair of a run. Numbered rather than
// random so that a run's pairs sort together and read as a set.
func PairName(run string, n int) string { return fmt.Sprintf("%s-p%d", run, n) }

// Create stands a pair up and claims it for this run.
//
// The build is passed rather than discovered because the engine under test is
// the run's decision, not csb's: a run that let the provisioner pick would not
// be able to say which build its evidence is about.
func (c *CLI) Create(ctx context.Context, build, run string) error {
	_, err := c.callLong(ctx, "cluster", "create",
		"--build", build, "--preset", "ha", "--label", RunLabel+"="+run)
	return err
}

// Destroy takes a cluster down. It does not purge: the describe artifact and the
// run record are a few kilobytes and they are the reproducible account of what
// the evidence was produced on, which is worth more than the kilobytes.
func (c *CLI) Destroy(ctx context.Context) error {
	_, err := c.callLong(ctx, "cluster", "destroy")
	return err
}

// Cluster is one row of `csb cluster ls`.
type Cluster struct {
	Name       string            `json:"name"`
	HasState   bool              `json:"has_state"`
	Containers int               `json:"containers"`
	Hosts      []string          `json:"hosts"`
	Labels     map[string]string `json:"labels"`
	Bytes      int64             `json:"bytes"`
}

// Run reports which run claimed this cluster, and whether any did.
func (cl Cluster) Run() (string, bool) {
	v, ok := cl.Labels[RunLabel]
	return v, ok && v != ""
}

// Clusters lists what is on this machine.
//
// It is a machine-wide question and not a cluster-wide one, so it is answered
// with a CLI that names no cluster -- Bind("") is enough.
func (c *CLI) Clusters(ctx context.Context) ([]Cluster, error) {
	env, err := c.callNoCluster(ctx, "cluster", "ls")
	if err != nil {
		return nil, err
	}
	var payload struct {
		Clusters []Cluster `json:"clusters"`
	}
	if derr := env.decode(&payload); derr != nil {
		return nil, derr
	}
	sort.Slice(payload.Clusters, func(i, j int) bool {
		return payload.Clusters[i].Name < payload.Clusters[j].Name
	})
	return payload.Clusters, nil
}

// Orphans are clusters some testkit run claimed and did not take back.
//
// Reported, never destroyed on sight. A run that removed another run's clusters
// because that run was not in this process table would eventually remove a
// cluster belonging to a run on another terminal, and the cost of that is worse
// than the disk.
func Orphans(all []Cluster, live map[string]bool) []Cluster {
	var out []Cluster
	for _, cl := range all {
		if run, ok := cl.Run(); ok && !live[run] {
			out = append(out, cl)
		}
	}
	return out
}

// HumanBytes is what a cluster costs, said the way an operator reads it.
func HumanBytes(n int64) string {
	switch {
	case n <= 0:
		return "-"
	case n >= 1<<30:
		return fmt.Sprintf("%.1fG", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%dM", n/(1<<20))
	default:
		return fmt.Sprintf("%dK", n/(1<<10))
	}
}

// LabelText renders a cluster's labels for a line of output.
func LabelText(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+m[k])
	}
	return strings.Join(parts, " ")
}
