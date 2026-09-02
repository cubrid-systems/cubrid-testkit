// Package legacy carries out every task that has not been rewritten yet, by
// running the original CTP.
//
// # Why it delegates to CTP rather than to the modules underneath
//
// CTP reaches its modules by reflection: it loads a jar and calls
// shell.main.Main.exec(conf). Those entry points have no main method, so they
// cannot be started from a command line. Reproducing the dispatch from outside
// would mean reimplementing the part of CTP most likely to have undocumented
// behaviour -- which is exactly what a coexistence path must not do.
//
// So this hands the whole invocation back: java -cp cubridqa-common.jar
// com.navercorp.cubridqa.ctp.CTP <the same arguments>. Fidelity comes for free,
// because the old code makes every decision it always made. Standard output,
// standard error and the exit code pass through untouched.
//
// This is the routing shim of docs/design/architecture.md §5-1. It is not the
// jar-compatibility layer NG5 forbids: nothing old calls into anything new.
package legacy

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/cubrid-systems/cubrid-testkit/internal/cli"
	"github.com/cubrid-systems/cubrid-testkit/internal/runner"
)

// CTPMainClass is the dispatcher bin/ctp.sh has always started.
const CTPMainClass = "com.navercorp.cubridqa.ctp.CTP"

// Runner runs tasks by handing them back to CTP.
type Runner struct {
	tasks []cli.Task
}

// New builds a Runner owning the given tasks.
func New(tasks ...cli.Task) *Runner { return &Runner{tasks: tasks} }

func (r *Runner) Tasks() []cli.Task { return r.tasks }

// Validate checks what bin/ctp.sh checked before it started java. The shell
// script exited 1 when JAVA_HOME was unset; there is no JAVA_HOME to check in a
// Go program, so the same code now covers any pre-flight failure. That widening
// is deliberate and graded F2 (external-surface-freeze.md §6-1).
func (r *Runner) Validate(req runner.Request) error {
	javaHome := os.Getenv("JAVA_HOME")
	if javaHome == "" {
		return &runner.ExitError{Code: 1, Err: fmt.Errorf("JAVA_HOME is not set, and %s still needs a JVM", req.Task)}
	}
	java := filepath.Join(javaHome, "bin", "java")
	if _, err := os.Stat(java); err != nil {
		return &runner.ExitError{Code: 1, Err: fmt.Errorf("no java at %s", java)}
	}
	jar := req.Home.Jar("common", "lib", "cubridqa-common.jar")
	if _, err := os.Stat(jar); err != nil {
		return &runner.ExitError{Code: 1, Err: fmt.Errorf("no CTP install at %s: %w", req.Home.Path, err)}
	}
	return nil
}

// Run starts CTP for exactly one task. Tasks are run one at a time even when
// several were named, so that each one is routed on its own and the order the
// user asked for is preserved.
func (r *Runner) Run(ctx context.Context, req runner.Request) error {
	java := filepath.Join(os.Getenv("JAVA_HOME"), "bin", "java")
	jar := req.Home.Jar("common", "lib", "cubridqa-common.jar")

	args := []string{"-cp", jar, CTPMainClass, string(req.Task)}
	if req.ConfigPath != "" {
		args = append(args, "-c", req.ConfigPath)
	}
	if req.Interactive {
		args = append(args, "--interactive")
	}
	args = append(args, req.Extra...)

	cmd := exec.CommandContext(ctx, java, args...)
	cmd.Dir = req.Home.Path
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "CTP_HOME="+req.Home.Path)

	err := cmd.Run()
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if ok := asExitError(err, &exitErr); ok {
		// CTP's own exit code is the frozen one. -1 from System.exit reaches us as
		// 255, and it must reach the caller as 255 too.
		return &runner.ExitError{Code: exitErr.ExitCode(), Err: fmt.Errorf("%s exited %d", req.Task, exitErr.ExitCode())}
	}
	return fmt.Errorf("running %s: %w", req.Task, err)
}

func asExitError(err error, target **exec.ExitError) bool {
	e, ok := err.(*exec.ExitError)
	if ok {
		*target = e
	}
	return ok
}
