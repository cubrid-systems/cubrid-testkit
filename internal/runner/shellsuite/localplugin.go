package shellsuite

import (
	"context"
	"fmt"
	"strings"

	"github.com/cubrid-systems/cubrid-testkit/internal/exec"
)

// The three literals the unittest plug-in protocol is made of.
//
// GeneralLocalTest appends `echo GPROPSTART` and then one
// `echo G_PROPERTY_<K>=${<K>}EEOOKK` line per variable it wants back. Everything
// before GPROPSTART is the script's own output; each value is what sits between
// its G_PROPERTY_<K>= and the next EEOOKK.
//
// The Phase 0 notes recorded only EEOOKK, which is a third of the protocol. All
// three are frozen: a plug-in author cannot see them, but the shape of the output
// depends on them.
const (
	propStart  = "GPROPSTART"
	propPrefix = "G_PROPERTY_"
	propEnd    = "EEOOKK"
)

// plugin drives one shell/local/<TEST_TYPE>.sh through the four functions it must
// define: init, list, execute and finish.
type plugin struct {
	ch       exec.Channel
	testType string
}

// invoke sources the plug-in and runs a command in the same shell, so that
// anything init exported is still in scope. That sourcing is why the four
// functions can be functions rather than separate scripts.
//
// The command line CTP builds is:
//
//	cd ${CTP_HOME}; source shell/local/<TEST_TYPE>.sh; <command>
//
// ${CTP_HOME} is left for the shell to expand -- it is not substituted here,
// because it was not substituted there.
func (p *plugin) invoke(ctx context.Context, command string, keys ...string) (string, map[string]string, error) {
	script := fmt.Sprintf("cd ${CTP_HOME}; source shell/local/%s.sh; %s", p.testType, command)
	if len(keys) > 0 {
		script += "; echo " + propStart + "\n"
		for _, k := range keys {
			script += fmt.Sprintf("echo %s%s=${%s}%s\n", propPrefix, k, k, propEnd)
		}
	}

	res, err := p.ch.Run(ctx, script)
	if err != nil {
		return "", nil, err
	}
	combined := res.Combined()

	output := combined
	if at := strings.Index(combined, propStart); at != -1 {
		output = combined[:at]
	}

	if len(keys) == 0 {
		return output, nil, nil
	}
	props := map[string]string{}
	for _, k := range keys {
		marker := propPrefix + k + "="
		start := strings.Index(combined, marker)
		if start == -1 {
			continue
		}
		rest := combined[start+len(marker):]
		end := strings.Index(rest, propEnd)
		if end == -1 {
			continue
		}
		props[k] = strings.TrimSpace(rest[:end])
	}
	return output, props, nil
}

// Init prepares the environment.
func (p *plugin) Init(ctx context.Context) (string, error) {
	out, _, err := p.invoke(ctx, "init")
	return out, err
}

// List asks the plug-in which cases exist: one per line on stdout.
func (p *plugin) List(ctx context.Context) ([]string, error) {
	out, _, err := p.invoke(ctx, "list")
	if err != nil {
		return nil, err
	}
	var cases []string
	for _, line := range strings.Split(out, "\n") {
		if s := strings.TrimSpace(line); s != "" {
			cases = append(cases, s)
		}
	}
	return cases, nil
}

// Execute runs one case and reads back IS_SUCC.
//
// The 2>&1 is part of the command CTP sends, not a detail of how it is run: the
// plug-in's stderr has to reach the same stream the markers are read from.
func (p *plugin) Execute(ctx context.Context, testCase string) (string, bool, error) {
	out, props, err := p.invoke(ctx, "execute "+testCase+" 2>&1 ", "IS_SUCC")
	if err != nil {
		return "", false, err
	}
	return out, isTrue(props["IS_SUCC"]), nil
}

// Finish tears the environment down.
func (p *plugin) Finish(ctx context.Context) (string, error) {
	out, _, err := p.invoke(ctx, "finish")
	return out, err
}

// isTrue mirrors CommonUtils.convertBoolean: a plug-in may say true or yes, and
// anything it does not say is a failure.
func isTrue(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "yes", "y", "1", "on":
		return true
	default:
		return false
	}
}
