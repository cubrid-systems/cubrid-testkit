// Package registry is the single place a task name turns into something that
// runs it.
//
// CTP had three dispatch mechanisms sitting side by side -- shelling out,
// in-process reflection, and one branch that returned before the task loop began.
// Collapsing them into one lookup is M1 (docs/project/concept/north-star.md §2).
package registry

import (
	"sort"

	"github.com/cubrid-systems/cubrid-testkit/internal/cli"
	"github.com/cubrid-systems/cubrid-testkit/internal/runner"
)

// Registry maps tasks to runners.
type Registry struct {
	byTask map[cli.Task]runner.Runner
}

// New returns an empty registry.
func New() *Registry {
	return &Registry{byTask: make(map[cli.Task]runner.Runner)}
}

// Register claims every task the runner reports. A second claim on the same task
// replaces the first, which is how a rewritten task takes over from legacy:
// register legacy first, then the native runner.
func (r *Registry) Register(rn runner.Runner) {
	for _, t := range rn.Tasks() {
		r.byTask[t] = rn
	}
}

// Lookup finds the runner for a task.
func (r *Registry) Lookup(t cli.Task) (runner.Runner, bool) {
	rn, ok := r.byTask[t]
	return rn, ok
}

// Tasks lists everything registered, sorted, for help and diagnostics.
func (r *Registry) Tasks() []cli.Task {
	out := make([]cli.Task, 0, len(r.byTask))
	for t := range r.byTask {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
