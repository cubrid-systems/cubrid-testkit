// Package runner defines the unit of replacement.
//
// A Runner owns one or more tasks. Replacing a task means moving it from one
// Runner to another; nothing else in the system has to know it happened. See
// docs/design/contracts.md C1.
package runner

import (
	"context"

	"github.com/cubrid-systems/cubrid-testkit/internal/cli"
	"github.com/cubrid-systems/cubrid-testkit/internal/conf"
)

// Request is everything a Runner needs to carry out one task.
type Request struct {
	Task       cli.Task
	Home       *conf.Home
	ConfigPath string // already resolved, explicit or fallen back
	// The file itself is not parsed here. The two families do not read the same
	// shape -- shell's conf is flat and the sql family's has sections, because
	// sql.conf carries what the runner writes into the engine's own files
	// (conf/ini.go) -- so a Request that carried one parse would carry a parse
	// one of them cannot use. Each runner opens the path in its own format, and
	// a missing file is the runner's to report: unittest tolerates one, because
	// CTP called GeneralLocalTest with a null configuration.
	Interactive bool
	Extra       []string // webconsole's start|stop, and nothing else so far
}

// Runner carries out tasks.
//
// Run returns an error only when the run could not be carried out. Failing test
// cases are not errors: the exit code stays 0 however many cases fail, which is
// frozen behaviour (external-surface-freeze.md §6-1).
type Runner interface {
	Tasks() []cli.Task
	Validate(req Request) error
	Run(ctx context.Context, req Request) error
}

// ExitError lets a Runner name the exit code the process should end with, so the
// frozen codes survive the trip back to main.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string { return e.Err.Error() }
func (e *ExitError) Unwrap() error { return e.Err }
