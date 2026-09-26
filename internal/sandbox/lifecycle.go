package sandbox

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
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
// **A run destroys what it created, and only when it has to make it again.**
// Not at the end of the run: a pair that survives is what makes the next run
// cheap, and an operator who stands a pair up to run three suites against it
// must still have it after the first.
//
// So destruction has exactly one trigger -- a set whose size no longer matches
// what the conf asks for, rebuilt on request -- and never happens as a side
// effect of running cases.
//
// # And the claim is written down where a dead run cannot lose it
//
// Ownership lives in the cluster's own describe artifact, as a label, not in
// this process's memory. A run that is killed -- and this suite has been killed
// four times in two days by things that had nothing to do with it -- leaves
// clusters that still say whose they were. `csb cluster ls` prints the label,
// and a rebuild says out loud when it is about to destroy something no run
// claimed.

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

// DestroyRun takes down every cluster a run claimed, in one call.
//
// Selected by the label rather than by a list this process assembled: the label
// is what the clusters themselves say, and a teardown that trusted its own list
// instead could remove a cluster whose claim had changed under it, or miss one
// whose creation it never heard the end of.
func DestroyRun(ctx context.Context, run string) error {
	_, err := Bind("").callLongNoCluster(ctx, "cluster", "destroy", "--label", RunLabel+"="+run)
	return err
}

// DestroyNamed takes down clusters this process names rather than clusters that
// name themselves.
//
// It exists because `DestroyRun` cannot serve the case it was first used for.
// Replacing a set means removing pairs an EARLIER run made, and those carry that
// run's claim, not this one's -- so selecting by this run's label matched
// nothing, reported that nothing went down, and the caller built on top of what
// was still standing. The names are what the caller has and the names are what
// it should use.
//
// The error names every cluster that would not go, because a caller about to
// reuse those names has to know which ones are still occupied.
func DestroyNamed(ctx context.Context, names []string) error {
	var stuck []string
	for _, name := range names {
		if err := Bind(name).Destroy(ctx); err != nil {
			stuck = append(stuck, name+" ("+err.Error()+")")
		}
	}
	if len(stuck) > 0 {
		return fmt.Errorf("%d of %d cluster(s) would not go down: %s",
			len(stuck), len(names), strings.Join(stuck, "; "))
	}
	return nil
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

// MembersOf returns the clusters that belong to a named set, in order.
//
// A set is `<set>-p1`, `<set>-p2`, ... and membership is decided by the name
// alone, not by a label. That is deliberate: the set is what the conf names, and
// a conf that named a set whose members csb knows about but testkit would not
// claim is a conf pointing at something real. The label answers a different
// question -- who may destroy it -- and is asked separately.
//
// A gap is not closed over. If `-p1` and `-p3` exist and `-p2` does not, the set
// has two members and the size check refuses, which is right: a set with a hole
// in it is not a set of three and quietly renumbering it would hide whatever
// removed the middle one.
func MembersOf(all []Cluster, set string) []string {
	prefix := set + "-p"
	var found []struct {
		n    int
		name string
	}
	for _, c := range all {
		if !strings.HasPrefix(c.Name, prefix) {
			continue
		}
		n, err := strconv.Atoi(c.Name[len(prefix):])
		if err != nil || n < 1 {
			continue
		}
		found = append(found, struct {
			n    int
			name string
		}{n, c.Name})
	}
	sort.Slice(found, func(i, j int) bool { return found[i].n < found[j].n })
	out := make([]string, 0, len(found))
	for _, f := range found {
		out = append(out, f.name)
	}
	return out
}

// DownAmong reports which of the named clusters are not running.
//
// It is a separate question from membership on purpose. `MembersOf` answers
// "which of these names are taken", which is what a rebuild needs before it
// reuses them; this answers "which of them could actually run a case", which is
// what reuse needs. A destroy keeps the describe artifact -- a kilobyte saying
// what evidence was produced on -- so a set whose pairs were removed by hand
// still has every one of its names, and only the container count tells them
// apart.
func DownAmong(all []Cluster, names []string) []string {
	up := map[string]int{}
	for _, c := range all {
		up[c.Name] = c.Containers
	}
	var down []string
	for _, n := range names {
		if up[n] == 0 {
			down = append(down, n)
		}
	}
	return down
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
