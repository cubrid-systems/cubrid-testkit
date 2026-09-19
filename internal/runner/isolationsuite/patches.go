package isolationsuite

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cubrid-systems/cubrid-testkit/internal/exec"
	"github.com/cubrid-systems/cubrid-testkit/internal/patch"
)

// applyPatches puts the run's patches into the corpus, and putPatchesBack takes
// them out again.
//
// What these carry is not what shell's and sql's do. shell's are a machine's
// layout and sql's are a case's dependence on what ran before it; isolation's
// are cases that leave two clients' output unordered and whose answer records
// the order one controller happened to produce. They fail under any controller
// that is faster, which is every reason ADR-019 exists
// (evidence/isolation-corpus-races.md), and each is written to be sent upstream
// as it stands.
//
// A patch that does not apply stops the run. It means the case has moved, and
// running it unpatched would answer a question nobody asked -- here more
// literally than elsewhere, since the question is whether the case orders what
// it prints and the patch is the ordering.
func applyPatches(ctx context.Context, ch exec.Channel, p *patch.Set, cases []string) error {
	for _, c := range cases {
		pf := p.For(c)
		if pf == "" {
			continue
		}
		res, err := exec.Check(ch.Run(ctx, patch.ApplyScript(caseDirOf(c), pf)))
		if err != nil {
			return fmt.Errorf("%s does not apply to %s: %w: %s", pf, c, err, strings.TrimSpace(res.Output()))
		}
		p.Applied(c, pf)
	}
	return nil
}

// putPatchesBack is best effort and says what it could not do: the run is over,
// and a corpus left patched is a corpus the next run reads from git.
//
// Isolation has no overlay over the corpus unless scenario_disk asks for one --
// runone.sh writes result/ and <name>.result into the cases tree as CTP does --
// so this is the only thing that puts the tree back, not a second line of
// defence as it is for shell.
func putPatchesBack(ch exec.Channel, p *patch.Set, cases []string) {
	for _, c := range cases {
		pf := p.For(c)
		if pf == "" {
			continue
		}
		if res, err := exec.Check(ch.Run(context.Background(), patch.RevertScript(caseDirOf(c), pf))); err != nil {
			fmt.Fprintf(os.Stderr, "[WARN] %s is still patched with %s: %v: %s\n",
				c, pf, err, strings.TrimSpace(res.Output()))
		}
	}
}

// caseDirOf is the directory a patch applies in: the one holding the .ctl and
// the answer/ beside it, so a single patch can change a case and its answer
// together.
//
// One level up, not two as in shell and sql. An isolation case is a file in a
// directory it shares with its siblings -- `aggregate/delete_select_02.ctl` and
// `aggregate/answer/delete_select_02.answer` -- and there is no cases/ around
// it (docs/category/isolation/02-writing-a-case.md).
func caseDirOf(caseFile string) string {
	return filepath.Dir(caseFile)
}
