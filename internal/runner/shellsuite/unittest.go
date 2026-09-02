package shellsuite

import (
	"context"
	"fmt"
	"os"

	"github.com/cubrid-systems/cubrid-testkit/internal/cli"
	"github.com/cubrid-systems/cubrid-testkit/internal/exec"
	"github.com/cubrid-systems/cubrid-testkit/internal/feedback"
	"github.com/cubrid-systems/cubrid-testkit/internal/result"
	"github.com/cubrid-systems/cubrid-testkit/internal/runner"
)

// UnitTest runs the unittest task.
//
// It is the first task to run natively, and it was chosen because it runs on this
// machine: no SSH, no deployment, no remote assets. What it does exercise is the
// plug-in protocol and the output layer, which is most of what the shell task will
// need later.
//
// The configuration file is optional. CTP calls GeneralLocalTest.exec with a null
// argument when none was given, so a missing file is not an error here either.
type UnitTest struct{}

// NewUnitTest returns the runner for the unittest task.
func NewUnitTest() *UnitTest { return &UnitTest{} }

func (u *UnitTest) Tasks() []cli.Task { return []cli.Task{cli.UnitTest} }

// Validate checks the plug-in exists. TEST_TYPE selects it, and CTP reads that
// from a JVM system property, so here it comes from the environment.
func (u *UnitTest) Validate(req runner.Request) error {
	testType := testTypeFor(req)
	path := req.Home.Jar("shell", "local", testType+".sh")
	if _, err := os.Stat(path); err != nil {
		return &runner.ExitError{
			Code: 1,
			Err:  fmt.Errorf("no unittest plug-in at %s (TEST_TYPE=%s)", path, testType),
		}
	}
	return nil
}

// Run drives init, list, execute and finish, and prints what CTP printed.
func (u *UnitTest) Run(ctx context.Context, req runner.Request) error {
	testType := testTypeFor(req)

	// The result directory is named after TEST_TYPE, not after the task:
	// GeneralLocalTest sets the log directory from System.getProperty("TEST_TYPE"),
	// falling back to "general".
	sink, err := result.Open(req.Home, testType, false)
	if err != nil {
		return err
	}
	defer sink.Close()

	p := &plugin{
		ch:       exec.NewLocal(req.Home.Path, "CTP_HOME="+req.Home.Path),
		testType: testType,
	}

	sink.Step("Init")
	out, err := p.Init(ctx)
	if err != nil {
		return err
	}
	sink.Raw(out)
	sink.Blank()

	sink.Step("List")
	cases, err := p.List(ctx)
	if err != nil {
		return err
	}
	if len(cases) == 0 {
		// CTP returns from start() here. Not finding cases is not a failure of the
		// run, and the exit code stays 0.
		sink.NoCases()
		return nil
	}

	// FeedbackFile prints these two lines to standard output as well as to its own
	// file, so they are part of the console surface even though they come from a
	// feedback backend. CTP emits them once the case list is known.
	fb := feedback.Console(categoryFor(req, testType), os.Stdout)
	fb.TaskStart("")
	fb.TotalTestCase(len(cases), 0, 0)

	sink.Step("Execute")
	for i, name := range cases {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		sink.UnitCaseStart(i+1, name)
		_, ok, err := p.Execute(ctx, name)
		if err != nil {
			return err
		}
		sink.UnitCaseVerdict(ok)
	}

	sink.Step("Finish")
	out, err = p.Finish(ctx)
	if err != nil {
		return err
	}
	sink.Raw(out)
	sink.Blank()
	return nil
}

// categoryFor resolves test_category, which labels the run. GeneralLocalTest
// falls back to "general" when the configuration does not say.
func categoryFor(req runner.Request, fallback string) string {
	if req.Config != nil {
		if v, ok := req.Config.Get("test_category"); ok && v != "" {
			return v
		}
	}
	if v := os.Getenv("TEST_CATEGORY"); v != "" {
		return v
	}
	if fallback != "" {
		return fallback
	}
	return "general"
}

// testTypeFor resolves TEST_TYPE. CTP read it as a JVM system property and fell
// back to "general"; the environment is where that lives for a Go program.
func testTypeFor(req runner.Request) string {
	if v := os.Getenv("TEST_TYPE"); v != "" {
		return v
	}
	if req.Config != nil {
		if v, ok := req.Config.Get("TEST_TYPE"); ok && v != "" {
			return v
		}
	}
	return "general"
}
