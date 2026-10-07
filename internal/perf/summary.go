package perf

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"
)

// summary.md and ledger_rows.md (Spec §7.7, §7.8; Design §5.10): what a
// person reads on Monday, and the rows this session adds to the ledger. The
// runner writes the rows beside its results and never touches ledger.md;
// the owner moves a row there with a verdict, by pull request.

// flagRow is one line of the flag table and of ledger_rows.md.
type flagRow struct {
	Session   string
	Pair      string
	Case      string // id@vN
	ID        string
	Ratio     string
	Arrow     string
	Flag      string
	Tolerance string
	Owner     string
	Repro     string
	ResultID  string
	Grade     string
}

type pendingRow struct {
	Session string
	Case    string
	Ratio   string
	Owner   string
}

type summaryData struct {
	Title    string
	Headline string
	Notes    []string
	Flags    []flagRow
	Pending  []pendingRow
	Branches []string
	Conbench string
	Summary  string
	Reports  string
}

const summaryTemplate = `# {{.Title}}

{{.Headline}}
{{range .Notes}}
- {{.}}{{end}}

## flag {{len .Flags}}
{{if .Flags}}| 쌍 | 케이스 | 비율 | 허용폭 | 담당 | 재현 |
|---|---|---|---|---|---|
{{range .Flags}}| {{.Pair}} | {{.Case}} | {{.Ratio}} {{.Arrow}}{{if ne .Flag "regression"}} ({{.Flag}}){{end}} | ±{{.Tolerance}} | {{.Owner}} | ` + "`{{.Repro}}`" + ` |
{{end}}{{else}}flag 없음
{{end}}
## 미판정 (지난 세션) {{len .Pending}}
{{if .Pending}}| 케이스 | 세션 | 비율 | 담당 |
|---|---|---|---|
{{range .Pending}}| {{.Case}} | {{.Session}} | {{.Ratio}} | {{.Owner}} |
{{end}}{{else}}없음
{{end}}
## 등록 브랜치
{{if .Branches}}| 브랜치 | 상태 |
|---|---|
{{range .Branches}}{{.}}
{{end}}{{else}}없음
{{end}}
{{if .Conbench}}conbench: {{.Conbench}}
{{end}}결과: {{.Reports}}
`

// summary writes summary.md and ledger_rows.md from the session's pairs.
func (s *Session) summary() error {
	d := s.summaryData()
	tmpl, err := template.New("summary").Parse(summaryTemplate)
	if err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(s.Out, "summary.md"))
	if err != nil {
		return err
	}
	defer f.Close()
	if err := tmpl.Execute(f, d); err != nil {
		return err
	}
	return writeLedgerRows(filepath.Join(s.Out, "ledger_rows.md"), d.Flags)
}

// summaryData gathers the tables. The flag table is grade-A cases of valid
// pairs whose flag is not none (FR-22); the pending table is earlier
// sessions' rows not yet in the ledger.
func (s *Session) summaryData() summaryData {
	d := summaryData{Reports: s.Out}
	title := "perf-weekly " + s.Started.Format("2006-01-02")
	if len(s.Pairs) > 0 {
		p := s.Pairs[0]
		title += fmt.Sprintf(" — %s %s vs %s %s", labelOf(p.Target), short(p.Target.Commit), labelOf(p.Reference), short(p.Reference.Commit))
	}
	d.Title = title

	var head []string
	run, total := 0, 0
	for _, p := range s.Pairs {
		if p.Skipped != "" {
			head = append(head, fmt.Sprintf("%s: 건너뜀 (%s)", p.Name, p.Skipped))
			continue
		}
		okCanaries := 0
		for _, cn := range p.Canaries {
			if cn.OK {
				okCanaries++
			}
		}
		state := "유효"
		if !p.Valid {
			state = "무효"
		}
		head = append(head, fmt.Sprintf("%s: %s (카나리 %d/%d 허용폭 안)", p.Name, state, okCanaries, len(p.Canaries)))
		if p.FingerprintChanged {
			d.Notes = append(d.Notes, fmt.Sprintf("지문 변화 (%s, 이전 세션 %s): %s", p.Name, s.doc.Previous, p.FingerprintNote))
		}
		run += p.CasesRun
		total += p.CasesRun + p.CasesSkipped
	}
	elapsed := s.doc.Ended
	if elapsed.IsZero() {
		elapsed = time.Now()
	}
	d.Headline = fmt.Sprintf("세션: %s · 지문: %s · %d/%d 케이스 · %s", strings.Join(head, "; "), fingerprintWord(s.Pairs), run, total, elapsed.Sub(s.Started).Round(time.Minute))
	if s.doc.State != "complete" || s.doc.ExitReason != "done" {
		d.Notes = append(d.Notes, fmt.Sprintf("세션 상태: %s (%s)", s.doc.State, s.doc.ExitReason))
	}
	if s.doc.BenchMode.Boost != nil && *s.doc.BenchMode.Boost {
		d.Notes = append(d.Notes, "CPU 부스트가 켜져 있었다 — 절대값은 믿지 말 것")
	}
	if s.doc.Pinning != PinningCPUSet {
		d.Notes = append(d.Notes, "pinning: "+s.doc.Pinning)
	}
	if s.Conf != nil && s.Conf.ReportMode == "dry" {
		d.Notes = append(d.Notes, "report.mode=dry — 보정 중")
	}
	if len(s.doc.Guard.Cleaned) > 0 {
		d.Notes = append(d.Notes, fmt.Sprintf("세션 전에 클러스터 밖 서버 프로세스 %d개를 멈췄다", len(s.doc.Guard.Cleaned)))
	}

	for _, p := range s.Pairs {
		if p.Skipped != "" || !p.Valid {
			continue
		}
		for _, e := range p.entries {
			if e.Status != StatusOK || e.Flag == FlagNone || e.Ratio == nil {
				continue
			}
			c := s.Suite.Case(e.ID)
			grade, owner := "", p.Owner
			if c != nil {
				grade = c.Grade
				if owner == "" {
					owner = c.Owner
				}
			}
			if grade == "B" {
				continue
			}
			arrow := "↑"
			if *e.Ratio < 1 {
				arrow = "↓"
			}
			d.Flags = append(d.Flags, flagRow{
				Session: s.SessionID, Pair: p.Name, ID: e.ID, Case: fmt.Sprintf("%s@v%d", e.ID, e.Version),
				Ratio: strconv.FormatFloat(*e.Ratio, 'f', 2, 64), Arrow: arrow, Flag: e.Flag,
				Tolerance: strconv.FormatFloat(e.Tolerance, 'g', -1, 64), Owner: owner,
				Repro:    fmt.Sprintf("testkit perf run %s --suite %s --build %s --build %s", e.ID, s.SuiteDir, p.Target.Build, p.Reference.Build),
				ResultID: p.Name + "/" + e.ID, Grade: grade,
			})
		}
	}

	ledger := ""
	if s.Suite != nil {
		ledger = filepath.Join(s.Suite.Root, "ledger.md")
	}
	d.Pending = pendingFlags(s.runs, s.SessionID, ledger, 8)

	for _, p := range s.Pairs {
		if p.branch == nil {
			continue
		}
		switch {
		case p.Skipped != "":
			d.Branches = append(d.Branches, fmt.Sprintf("| %s | 제외됨 — %s |", p.Name, p.Skipped))
		case !p.Valid:
			d.Branches = append(d.Branches, fmt.Sprintf("| %s | 무효 (카나리) |", p.Name))
		default:
			d.Branches = append(d.Branches, fmt.Sprintf("| %s | merge-base %s · %d 케이스 · flag %d |", p.Name, short(p.Reference.Commit), p.CasesRun, p.Flags))
		}
	}
	if s.Conf != nil && s.Conf.ConbenchURL != "" {
		d.Conbench = strings.TrimRight(s.Conf.ConbenchURL, "/") + "/cubrid/e/runs/" + s.SessionID
	}
	return d
}

// labelOf is the build's label for a title: what build-info.json says, else
// the install directory's name without its -<sha7>.
func labelOf(b BuildRef) string {
	if b.Fingerprint.Label != "" {
		return b.Fingerprint.Label
	}
	base := filepath.Base(b.Build)
	if sha := short(b.Commit); sha != "?" && strings.HasSuffix(base, "-"+sha) {
		return strings.TrimSuffix(base, "-"+sha)
	}
	return base
}

func fingerprintWord(pairs []*SessionPair) string {
	for _, p := range pairs {
		if p.FingerprintChanged {
			return "변화 있음"
		}
	}
	return "변화 없음"
}

// ledger_rows.md: one row per flag, in ledger.md's columns, verdict open.
func writeLedgerRows(path string, rows []flagRow) error {
	var b strings.Builder
	b.WriteString("| 세션 | 케이스 | 비율 | 판정 | 근거·JIRA | 결과 ID |\n|---|---|---|---|---|---|\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "| %s | %s | %s | (미판정) | | %s |\n", r.Session, r.Case, r.Ratio, r.ResultID)
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// ledgerKeys reads (session, case) of every row of a ledger-shaped file.
func ledgerKeys(path string) map[string]bool {
	out := map[string]bool{}
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if r, ok := parseLedgerRow(sc.Text()); ok {
			out[r.Session+"\x00"+r.Case] = true
		}
	}
	return out
}

func parseLedgerRow(line string) (pendingRow, bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "|") || strings.HasPrefix(line, "|---") || strings.HasPrefix(line, "| 세션") {
		return pendingRow{}, false
	}
	cells := strings.Split(strings.Trim(line, "|"), "|")
	if len(cells) < 3 {
		return pendingRow{}, false
	}
	for i := range cells {
		cells[i] = strings.TrimSpace(cells[i])
	}
	return pendingRow{Session: cells[0], Case: cells[1], Ratio: cells[2]}, cells[0] != "" && cells[1] != ""
}

// pendingFlags is the rows of the newest n earlier sessions' ledger_rows.md
// that ledger.md does not have.
func pendingFlags(root, selfID, ledger string, n int) []pendingRow {
	settled := ledgerKeys(ledger)
	var out []pendingRow
	ids := previousSessions(root, selfID)
	if len(ids) > n {
		ids = ids[:n]
	}
	for _, id := range ids {
		for _, rel := range []string{"results/ledger_rows.md", "ledger_rows.md"} {
			f, err := os.Open(filepath.Join(root, id, rel))
			if err != nil {
				continue
			}
			sc := bufio.NewScanner(f)
			for sc.Scan() {
				r, ok := parseLedgerRow(sc.Text())
				if ok && !settled[r.Session+"\x00"+r.Case] {
					out = append(out, r)
				}
			}
			f.Close()
			break
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Session > out[j].Session })
	return out
}
