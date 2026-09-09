package shellsuite

import (
	"fmt"
	"slices"
	"strings"

	"github.com/cubrid-systems/cubrid-testkit/internal/topology"
)

// Deployment is what has to be true on a machine before the first case runs:
// the engine configured the way this instance asks for, and a pristine copy of
// the install put aside so every case can start from the same place.
//
// Two things CTP does here are not done at all.
//
// CTP upgraded itself first -- deploy_ctp ran common/script/upgrade.sh against a
// branch named in the config. Deciding when the test harness updates is an
// operations question, not a test-execution one, and it is excluded on the axis
// split (docs/concept/migration-exclusions.md).
//
// CTP also installed the build, by calling run_cubrid_install -- a shell function
// that exists only inside CTP's own deployed environment. The runner tests the
// build that is already installed. Which build to install, and from where, is the
// same operations question.

// confTarget says where a role's parameters are written.
type confTarget struct {
	role    string
	section string
	file    string
}

// iniTargets is CTP's mapping from role to ini.sh section, in the order it wrote
// them. The two broker sections do not read the way the role names suggest:
// broker1 goes to %query_editor and broker2 goes to %BROKER1, because those are
// the section names in the shipped cubrid_broker.conf and the roles are numbered
// by position rather than by name.
var iniTargets = []confTarget{
	{topology.RoleCUBRID, "common", "$CUBRID/conf/cubrid.conf"},
	{topology.RoleHA, "common", "$CUBRID/conf/cubrid_ha.conf"},
	{topology.RoleCM, "cm", "$CUBRID/conf/cm.conf"},
	{topology.RoleBroker1, "%query_editor", "$CUBRID/conf/cubrid_broker.conf"},
	{topology.RoleBroker2, "%BROKER1", "$CUBRID/conf/cubrid_broker.conf"},
	{topology.RoleBrokerCommon, "broker", "$CUBRID/conf/cubrid_broker.conf"},
}

// paramList renders a role's properties as ini.sh expects them: k=v pairs joined
// by ||, with a trailing separator.
//
// CTP built this from a Hashtable and got whatever order the JVM felt like. The
// order cannot matter -- each key is set once -- so we sort, and two runs produce
// the same script.
func paramList(props map[string]string) string {
	if len(props) == 0 {
		return ""
	}
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%s=%s||", k, props[k])
	}
	return b.String()
}

// ConfigureScript writes this instance's parameters into the engine's conf files.
// It returns the empty string when the instance asks for nothing.
//
// CTP decided "nothing" by looking at five of the six roles and forgetting
// brokercommon, so a configuration that set only broker-wide parameters -- which
// is where MASTER_SHM_ID lives -- was silently skipped and the run went ahead on
// default ports. Two instances sharing a port do not fail; they interfere, and
// the results look real. Under the config-key policy that is a wrong result
// someone would trust, so brokercommon counts here.
//
// docs/concept/migration-exclusions.md 2a.
func ConfigureScript(inst *topology.Instance) string {
	var lines []string
	empty := true
	for _, t := range iniTargets {
		list := paramList(inst.Role(t.role))
		if list == "" {
			continue
		}
		empty = false
		lines = append(lines, fmt.Sprintf("ini.sh -s '%s' --separator '||' -u '%s' %s",
			t.section, list, t.file))
	}
	if empty {
		return ""
	}

	// The broker's shared-memory key defaults to the engine's port number when
	// nothing sets it. That is why two installs on one machine collide twice
	// over: same port, and then the same segment.
	shmID := inst.Role(topology.RoleBrokerCommon)["MASTER_SHM_ID"]
	if shmID == "" {
		shmID = inst.Role(topology.RoleCUBRID)["cubrid_port_id"]
	}
	if shmID != "" {
		lines = append(lines, fmt.Sprintf(
			"ini.sh -s 'broker' -u MASTER_SHM_ID=%s $CUBRID/conf/cubrid_broker.conf", shmID))
	}
	return strings.Join(lines, "\n")
}

// SnapshotScript puts a pristine copy of the install aside. Every case is then
// run against a CUBRID restored from it, so no case can inherit another's
// configuration, database or log.
func SnapshotScript() string {
	return strings.Join([]string{
		"rm -rf ~/.CUBRID_SHELL_FM > /dev/null 2>&1",
		"cp -r ${CUBRID} ~/.CUBRID_SHELL_FM",
	}, "\n")
}

// RestoreScript puts the install back to the snapshot, between cases.
//
// The last two lines delete core files, which is worth knowing about together
// with the ulimit CTP appends to the remote .bash_profile: cores are enabled
// permanently, and swept between cases. A case that dumps core and the sweep that
// follows are the whole reason a run can fill a disk.
func RestoreScript() string {
	return strings.Join([]string{
		// Nothing below this line may run with $CUBRID unset.
		//
		// Every command here is rooted at ${CUBRID}, and every one of them is
		// destructive. With the variable empty the paths do not fail -- they
		// become absolute paths at the root of the filesystem, and
		//
		//	find ${CUBRID}/ -name "core" | xargs -i rm -rf {}
		//
		// becomes `find /`, which deletes every directory named exactly "core"
		// on the machine. That is not a hypothetical: it ran, and it removed 69
		// of them across this machine's /data at 01:16:38 on 2026-09-09,
		// including node_modules/undici/lib/core out of a VS Code server, which
		// then failed to start every six minutes for the rest of the day. Files
		// called core.js and core.d.ts survived, which is the signature of
		// exactly this command and of nothing else.
		//
		// The prologue does not set $CUBRID -- it resolves CTP_HOME and
		// init_path and leaves the engine to the caller's environment -- so an
		// empty value is one unsourced profile away, and there was no guard.
		//
		// Refusing rather than defaulting: a reset that quietly does nothing is
		// a case that runs against the previous case's leftovers, and that is a
		// wrong answer rather than a lost one. The run should stop.
		`if [ -z "${CUBRID:-}" ] || [ ! -d "${CUBRID}/conf" ] || [ ! -x "${CUBRID}/bin/cub_server" ]; then` + "\n" +
			`  echo "[ERROR] the reset refuses to run: CUBRID is \"${CUBRID:-}\", which is not a CUBRID installation" >&2` + "\n" +
			`  exit 1` + "\n" +
			`fi`,
		"rm -rf ${CUBRID}/conf/*",
		"cp -rf ~/.CUBRID_SHELL_FM/conf/* ${CUBRID}/conf/",
		"rm -rf ${CUBRID}/databases/*",
		"cp -rf ~/.CUBRID_SHELL_FM/databases/* ${CUBRID}/databases/",
		"rm -rf ${CUBRID}/lib/libcubrid_??_??.so",
		"rm -rf ${CUBRID}/lib/libcubrid_all_locales.so",
		"rm -rf ${CUBRID}/var/* >/dev/null 2>&1",
		// Quoted, and -mindepth 1 so that a find whose root somehow still ends up
		// wrong cannot delete the root itself. The guard above is the real
		// defence; this is the belt behind it.
		`find "${CUBRID}/log" -mindepth 1 -type f -print | xargs -i rm -rf {} `,
		`find "${CUBRID}/" -mindepth 1 -name "core.[0-9][0-9]*" | xargs -i rm -rf {} `,
		`find "${CUBRID}/" -mindepth 1 -name "core" | xargs -i rm -rf {} `,
	}, "\n")
}

// killPatterns are the process names swept before every case, in CTP's order.
//
// Read the list before running this anywhere but a dedicated test machine. It
// matches on substrings of the process name across everything the user owns: cub
// takes any name containing "cub", java takes every JVM that is not this runner,
// and sleep, expect and dos2unix are taken outright. It then releases every
// shared-memory segment the user holds.
//
// It is this broad on purpose -- a case that leaves a cub_server behind will fail
// the next case for reasons that have nothing to do with the build -- and it is
// the reason a CTP run and anything else the same user is doing cannot share a
// machine.
var killPatterns = []string{
	"cub_admin", "cub_master", "cub_auto", "cub_broker", "cub_server",
	"cub", "broker", "cm_admin", "csql", "loadjava", "make_locale",
	"migrate", "shard",
}

// KillScript stops everything the previous case may have left running.
//
// local drops the final sweep of shell scripts: when the runner is driving the
// machine it is running on, killing every *.sh the user owns would kill the case
// that is asking for the sweep.
func KillScript(local, contained bool) string {
	var lines []string
	lines = append(lines, "cubrid service stop")
	for _, p := range killPatterns {
		lines = append(lines, bothKill(psPIDs(p, contained)))
	}
	// Same argument for shared memory: an IPC namespace holds this run's segments
	// and no others, so there is no owner to match on.
	if contained {
		lines = append(lines, "ipcs -m | awk 'NR>3 {print $2}' | xargs -i ipcrm -m {}")
	} else {
		lines = append(lines, "ipcs | grep $USER | awk '{print $2}'  | xargs -i ipcrm -m {}")
	}

	// Every JVM except this run's own -- except that it never has. The test is
	// written "[ $isExistPid -eq 0]" with no space before the bracket, which is a
	// shell syntax error, so the branch is never taken and final_list is always
	// empty. Kept verbatim: reproducing it costs nothing and keeps the worker log
	// identical, while "fixing" it would start killing JVMs on machines where
	// nothing has killed one in years.
	lines = append(lines,
		"ctp_java_pid_list=`"+psAllCmd(contained)+"| grep -v grep | grep -E 'com.navercorp.cubridqa|service.Server' | awk '{print $1}'`",
		"all_java_pid_list=`"+psAll(contained)+"| grep -v grep | grep -i java | awk '{print $1}'`",
		`final_list=""`,
		"for x in ${all_java_pid_list};do isExistPid=`echo ${ctp_java_pid_list}|grep -w $x|wc -l`;if [ $isExistPid -eq 0];then final_list=\"${final_list} $x\" ;fi;done",
		bothKill("echo ${final_list}"),
	)
	for _, p := range []string{"sleep", "expect", "dos2unix"} {
		lines = append(lines, bothKill(psPIDsInsensitive(p, contained)))
	}
	if !local {
		sh := "ps -u $USER -o pid,cmd"
		if contained {
			sh = "ps -e -o pid,cmd"
		}
		lines = append(lines, bothKill(sh+`| grep -v grep | grep -i '\.sh' | awk '{print $1}'`))
	}
	// The four dumps that close the script are diagnostics, not selectors, and
	// they have to follow the same move: contained, `ps -u $USER -f` prints an
	// empty table and the worker log loses the only picture it has of what the
	// machine was doing.
	dump := "ps -u $USER -f"
	if contained {
		dump = "ps -e -f"
	}
	lines = append(lines,
		dump,
		"ps -ef | grep cub ",
		"netstat -n -e -p -a",
		"ipcs",
	)
	return strings.Join(lines, "\n")
}

// psPIDs selects by process name across the user's processes, or -- when the run
// has namespaces of its own -- across the namespace, which is the same set with
// none of the machine in it. See contain.
func psPIDs(name string, contained bool) string {
	return fmt.Sprintf("%s | grep -v grep | grep %s | awk '{print $1}'", psAll(contained), name)
}

// psAll is what the sweep looks at. Contained, "everything here" is already
// "everything this run started", so there is nothing to filter on and nothing
// that can be filtered wrongly. Uncontained it is CTP's own selector, kept
// verbatim -- including that it matches nothing when the run's uid does not
// answer to $USER.
func psAll(contained bool) string {
	if contained {
		return "ps -e -o pid,comm"
	}
	return "ps -u $USER -o pid,comm"
}

func psPIDsInsensitive(name string, contained bool) string {
	return fmt.Sprintf("%s | grep -v grep | grep -i %s | awk '{print $1}'", psAll(contained), name)
}

// bothKill runs the same pid query twice, piped into kill and then substituted
// into it. The second form runs kill with no arguments when nothing matched,
// which is where the stray "kill: not enough arguments" in a worker log comes
// from. Kept because a worker log that stops matching CTP's is harder to compare
// than one with a known harmless line in it.
// psAllCmd is psAll with the full command line, which the JVM filter needs.
func psAllCmd(contained bool) string {
	if contained {
		return "ps -e -o pid,command"
	}
	return "ps -u $USER -o pid,command"
}

func bothKill(query string) string {
	return query + " | xargs -i kill -9 {} \n" + "kill -9 `" + query + "`"
}
