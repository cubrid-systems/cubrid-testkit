#!/bin/bash
# What a pair has to be before the HA shell corpus can run on it.
#
#   preflight.sh <master> <slave> <user> [<password>] [<ssh-port>]
#
# The 373 cases do not set HA up themselves in any sense that matters: they call
# $init_path/make_ha.sh, which reads $init_path/HA.properties and hands the work
# to make_ha_upper.sh. Reading those three says exactly what a node has to be,
# and this checks it -- in seconds, before a run that takes hours finds out
# inside a case with a message about something else.
#
# Every check below is one of the requirements measured in ha-topology.md §3.
# Nothing here is a guess about what might be needed.
#
# Password authentication because that is what CTP's helper uses: run_on_slave is
# an alias for common/script/run_remote_script, which is a wrapper around the
# Java class com.navercorp.cubridqa.common.RunRemoteScript and takes -password.
# A key-only slave will pass every check here and fail every case.
set -u

master=${1:-}; slave=${2:-}; user=${3:-}; pass=${4:-}; port=${5:-22}
if [ -z "$master" ] || [ -z "$slave" ] || [ -z "$user" ]; then
  echo "usage: preflight.sh <master> <slave> <user> [<password>] [<ssh-port>]" >&2
  exit 2
fi

fail=0
say()  { printf '  %-42s %s\n' "$1" "$2"; }
ok()   { say "$1" "ok${2:+ — $2}"; }
bad()  { say "$1" "NO${2:+ — $2}"; fail=$((fail+1)); }

# on <host> <command> -- runs it there, quietly, and yields its stdout.
#
# The profile is sourced first, every time, because that is exactly what CTP
# does: common/ShellInput.java prepends ". ~/.bash_profile" to every script it
# sends. A check that looks without it is looking at a different machine than the
# one the cases will run on -- which this script got wrong on its first run,
# reporting a build and a JVM missing on a node that had both.
on() {
  ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new -o ConnectTimeout=10 \
      -p "$port" "$user@$1" ". ~/.bash_profile >/dev/null 2>&1; $2" 2>/dev/null
}

echo "=== reachable ==="
for h in "$master" "$slave"; do
  if on "$h" true; then ok "ssh $user@$h:$port" ; else
    bad "ssh $user@$h:$port" "BatchMode failed; a password-only account is fine for CTP but this script needs a key to look"
  fi
done
[ "$fail" -gt 0 ] && { echo; echo "cannot look further"; exit 1; }

echo
echo "=== each node is a QA machine (ha-topology.md §3) ==="
for h in "$master" "$slave"; do
  echo "-- $h"
  # cubrid_rel opens with a blank line, so the version is grepped rather than headed.
  v=$(on "$h" 'cubrid_rel 2>/dev/null | grep -m1 CUBRID')
  [ -n "$v" ] && ok "a CUBRID build" "$v" || bad "a CUBRID build" "cubrid_rel says nothing"

  # The JVM and the tree are the master's: CTP's -initfile is read where the
  # Java process runs and sent inline, so a slave needs neither.
  if [ "$h" = "$master" ]; then
    j=$(on "$h" 'echo ${JAVA_HOME:-}')
    [ -n "$j" ] && ok "JAVA_HOME" "$j" || bad "JAVA_HOME" "run_remote_script is a Java class (§3b)"

    c=$(on "$h" 'echo ${CTP_HOME:-}')
    if [ -n "$c" ] && [ -n "$(on "$h" "[ -d '$c/shell/init_path' ] && echo y")" ]; then
      ok "CTP_HOME with shell/init_path" "$c"
    else
      bad "CTP_HOME with shell/init_path" "\$init_path is where HA.properties goes (§3c)"
    fi
  fi

  for tool in expect scp ssh csql; do
    [ -n "$(on "$h" "command -v $tool")" ] && ok "$tool" || bad "$tool" "make_ha.sh's expect files need it (§3d)"
  done

  w=$(on "$h" '[ -n "$CUBRID" ] && [ -w "$CUBRID/conf/cubrid.conf" ] && echo y')
  [ -n "$w" ] && ok "\$CUBRID/conf is writable" || bad "\$CUBRID/conf is writable" "modify_cubrid_conf rewrites it"
done

echo
echo "=== the pair can reach each other, which is the part csb could not do ==="
# The master reaches the slave by the address the conf will carry, and back. A
# node that can be reached from here and not from its peer is the failure this
# catches: replication runs between the nodes, not through the controller.
if [ -n "$(on "$master" "(</dev/tcp/$slave/$port) >/dev/null 2>&1 && echo y")" ]; then
  ok "master -> slave:$port"
else
  bad "master -> slave:$port" "the cases ssh from the master to the slave"
fi
p=$(on "$master" 'ini.sh -s common $CUBRID/conf/cubrid.conf cubrid_port_id 2>/dev/null')
p=${p:-1523}
if [ -n "$(on "$slave" "(</dev/tcp/$master/$p) >/dev/null 2>&1 && echo y")" ]; then
  ok "slave -> master:$p (engine port)"
else
  say "slave -> master:$p (engine port)" "closed — expected until a server is running there"
fi

echo
echo "=== password authentication, which CTP needs and a key hides ==="
if [ -z "$pass" ]; then
  say "password given" "not checked — pass one to check it"
else
  if sshpass -V >/dev/null 2>&1; then
    if sshpass -p "$pass" ssh -o StrictHostKeyChecking=accept-new -o PreferredAuthentications=password \
         -o PubkeyAuthentication=no -p "$port" "$user@$slave" true 2>/dev/null; then
      ok "password login to the slave"
    else
      bad "password login to the slave" "run_remote_script passes -password and nothing else"
    fi
  else
    say "password login to the slave" "sshpass not here; check by hand"
  fi
fi

echo
if [ "$fail" -eq 0 ]; then
  echo "READY — $fail unmet"
else
  echo "NOT READY — $fail unmet. Each one is a case that would fail for a reason that is not the engine's."
fi
exit $((fail > 0))
