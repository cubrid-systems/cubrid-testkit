package perf

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// dropCachesInstalled is a root-owned copy of page_cache_drop.sh the hub's
// sudoers names. A sudoers line that names a file inside a writable checkout
// would hand root to whoever can edit the checkout, so the fixed path is
// preferred and the suite's own script is the fallback for a machine without
// the line, where sudo -n fails and the pass is null(cache_drop) either way.
const dropCachesInstalled = "/usr/local/sbin/perf-drop-caches"

// pageCacheDrop empties the OS page cache on this host, which needs root.
// sudo is asked without a prompt so that a machine without the line fails
// here and not on a terminal nobody is watching. The script itself is what
// sudo runs -- the sudoers line matches the command, and `sudo bash script`
// would be asking for bash.
func pageCacheDrop(suiteDir string) error {
	script := dropCachesInstalled
	if _, err := os.Stat(script); err != nil {
		script = filepath.Join(suiteDir, "scripts", "page_cache_drop.sh")
	}
	out, err := exec.Command("sudo", "-n", script).CombinedOutput()
	if err != nil {
		return fmt.Errorf("sudo -n %s: %v: %s", script, err, tail(string(out), 200))
	}
	return nil
}
