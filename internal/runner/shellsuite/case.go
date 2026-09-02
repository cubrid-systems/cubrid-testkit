package shellsuite

import (
	"fmt"
	"strings"
	"time"
)

// CaseOptions is everything a case needs from the run that is not the case
// itself. It is passed rather than read from a global, which is the ambient state
// CTP's Context held and M4 removes.
type CaseOptions struct {
	// Bits is "64" or "32". It selects JAVA_HOME_64 / JAVA_HOME_32 if the remote
	// machine defines one, which is how a run pins the JDK per architecture.
	Bits string

	// BigSpaceDir is where cases that need room put their volumes. It is emptied
	// before every case, so it must never be a directory anyone else uses.
	BigSpaceDir string

	// Charset sets CUBRID_CHARSET when non-empty.
	Charset string

	// IgnoreCoresByKeyword is passed to the core analyzer as
	// EXCLUDED_CORES_BY_ASSERT_LINE: cores whose stack matches are not failures.
	IgnoreCoresByKeyword string

	// SSHHost, SSHPort and SSHUser are exported so a case can reach back to the
	// machine it is running on. An empty host or user becomes a shell expansion,
	// because a locally-driven run has no configured value for either.
	SSHHost string
	SSHPort string
	SSHUser string

	// BuildID is exported as TEST_BUIILD_ID -- three i's. The typo is CTP's, it is
	// part of the environment cases see, and no case in the corpus reads it. Kept
	// because it costs nothing and removing it is a change to what cases can see.
	BuildID string
}

func (o CaseOptions) sshInfo() []string {
	host := o.SSHHost
	if host == "" {
		host = "`hostname -i`"
	}
	user := o.SSHUser
	if user == "" {
		user = "`echo $USER`"
	}
	return []string{
		"export TEST_SSH_HOST=" + host,
		"export TEST_SSH_PORT=" + o.SSHPort,
		"export TEST_SSH_USER=" + user,
		"export TEST_BUIILD_ID=" + o.BuildID,
	}
}

// RunScript is what actually runs a case.
//
// The case is run with sh, from inside its own cases/ directory, after the result
// file has been truncated -- the case appends its verdict there, and a stale one
// from a previous attempt would be read as this attempt's.
func RunScript(c Case, o CaseOptions) string {
	lines := []string{
		"cd " + c.Dir,
		"ulimit -c unlimited",
	}
	if o.Bits != "" {
		bits := strings.ToUpper(strings.TrimSpace(o.Bits))
		lines = append(lines,
			fmt.Sprintf(`if [ "$JAVA_HOME_%s" ]; then`, bits),
			fmt.Sprintf("        export JAVA_HOME=$JAVA_HOME_%s", bits),
			"fi")
	}
	lines = append(lines,
		"export TEST_BIG_SPACE=$(echo $TEST_BIG_SPACE)",
		fmt.Sprintf("export TEST_BIG_SPACE=`if [ \"$TEST_BIG_SPACE\" = '' ]; then echo %s ; else echo $TEST_BIG_SPACE; fi`", o.BigSpaceDir),
		`if [ "$TEST_BIG_SPACE" != '' ]; then mkdir -p $TEST_BIG_SPACE; rm -rf $TEST_BIG_SPACE/*; fi`)

	if strings.TrimSpace(o.Charset) != "" {
		lines = append(lines, "export CUBRID_CHARSET="+o.Charset)
	}
	if strings.TrimSpace(o.IgnoreCoresByKeyword) != "" {
		lines = append(lines, `export EXCLUDED_CORES_BY_ASSERT_LINE="`+o.IgnoreCoresByKeyword+`"`)
	}

	lines = append(lines, "echo > "+c.Result)
	lines = append(lines, o.sshInfo()...)
	lines = append(lines, "sh "+c.Script+" 2>&1")
	return strings.Join(lines, "\n")
}

// FinalCheckScript looks for cores and fatal errors the case did not notice.
//
// It sources the case's own SKIP_CHECK_FATAL_ERROR setting first -- a case can
// declare that a fatal error is what it is testing for -- and then calls
// do_check_more_errors from the deployed init_path, which appends what it finds
// to the case's result file. That is why this runs before the result is read and
// why its own stdout is not parsed on the main host.
func FinalCheckScript(c Case, o CaseOptions) string {
	lines := []string{
		"source /dev/stdin <<EOF",
		"`grep -E \"SKIP_CHECK_FATAL_ERROR\" " + c.Path + " `",
		"EOF",
	}
	if strings.TrimSpace(o.IgnoreCoresByKeyword) != "" {
		lines = append(lines, `export EXCLUDED_CORES_BY_ASSERT_LINE="`+o.IgnoreCoresByKeyword+`"`)
	}
	lines = append(lines, o.sshInfo()...)
	lines = append(lines, `source $init_path/shell_utils.sh && do_check_more_errors "`+c.Dir+`"`)
	return strings.Join(lines, "\n")
}

// CollectScript reads the verdict the case wrote.
func CollectScript(c Case) string {
	return strings.Join([]string{"cd ", "cd " + c.Dir, "cat " + c.Result}, "\n")
}

// collectAttempts and collectPause are how long the runner waits for a result
// file that is still empty. A case whose last write has not landed yet is common
// enough that CTP re-read the file up to six times, a second apart, before
// calling it blank.
const (
	collectAttempts = 6
	collectPause    = time.Second
)

// resultItem formats one line of a case's result.
//
// A line the case wrote is passed through untouched; a line the runner adds gets
// a flag and the leading " : " that makes runner-added lines findable in a result
// file that is otherwise the case's own words.
func resultItem(flag, message string) string {
	if flag == "" {
		return message
	}
	return " : " + flag + " " + message
}

// blankResultItems are what CTP recorded when the result file stayed empty. There
// are two of them, and the first has a backslash in the middle of a POSIX path,
// because the same block was written twice and only the second was simplified.
// They are reproduced because a result file is a frozen surface and these two
// lines are in it.
func blankResultItems(c Case, now time.Time) []string {
	return []string{
		resultItem("NOK", "blank result - "+javaDate(now)+" - "+c.Dir+`\`+c.Result),
		resultItem("NOK", "blank result"),
	}
}

// javaDate renders a timestamp the way java.util.Date.toString does, which is the
// form these result lines have always had.
func javaDate(t time.Time) string { return t.Format("Mon Jan 02 15:04:05 MST 2006") }

// Verdict is what a case's result adds up to.
type Verdict struct {
	Items   []string
	Success bool
	HasCore bool
}

// verdictOf reads the result lines the way CTP did: any line containing NOK fails
// the case, and two specific NOK lines mean the failure produced evidence worth
// keeping rather than a flake worth retrying.
//
// It is a substring test over the whole line, not a prefix or a field. A case
// that prints the word NOK while explaining itself fails, and always has.
func verdictOf(items []string) Verdict {
	v := Verdict{Items: items, Success: true}
	for _, item := range items {
		if strings.Contains(item, "NOK") {
			v.Success = false
		}
		if strings.Contains(item, "NOK found core file") || strings.Contains(item, "NOK found fatal error") {
			v.HasCore = true
		}
	}
	return v
}
