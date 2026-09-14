package contain

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Dos2UnixEnv turns off the dos2unix stand-in. Any non-empty value leaves the
// machine's PATH alone, and a machine without the tool then fails the cases
// that need it.
const Dos2UnixEnv = "TESTKIT_NO_DOS2UNIX_SHIM"

// Dos2UnixShim writes a dos2unix for a machine that has none, into dir, and
// returns dir. It returns "" when the machine already has one, which is the
// usual case and the one worth keeping: the real tool is what CI runs.
//
// CTP's shell/init_path/init.sh normalises both sides before it compares them:
//
//	dos2unix $left
//	dos2unix $right
//
// A machine without the tool gets "dos2unix: command not found" on stderr, the
// carriage returns stay where they are, and the diff that follows reports every
// line as different. The case fails on the machine rather than on the engine.
// 37 cases in the shell corpus reach that path.
//
// Same argument as GccShim, one tool over: what this removes is a difference
// between this machine and the machine the verdicts are compared against.
//
// The stand-in does the one thing init.sh asks for -- CRLF to LF, in place --
// and refuses an option rather than guessing at it, because the only caller
// passes a bare filename and a silent misreading would be worse than a failure
// that says what happened. It is not dos2unix: no encoding conversion, no
// -k, no -n. Those have no caller here, and inventing them would make the
// stand-in something to maintain.
func Dos2UnixShim(dir string) (string, error) {
	if os.Getenv(Dos2UnixEnv) != "" {
		return "", nil
	}
	if _, err := exec.LookPath("dos2unix"); err == nil {
		return "", nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	body := `#!/bin/sh
# Written by testkit for a machine with no dos2unix.
# See internal/contain/dos2unix.go.
for f in "$@"; do
  case "$f" in
    -*) echo "dos2unix (testkit): $f is not supported" >&2; exit 2 ;;
  esac
  [ -f "$f" ] || { echo "dos2unix (testkit): $f: no such file" >&2; exit 1; }
  sed -i 's/\r$//' "$f" || exit 1
done
`
	path := filepath.Join(dir, "dos2unix")
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		return "", fmt.Errorf("write dos2unix shim: %w", err)
	}
	return dir, nil
}
