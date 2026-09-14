package sqlsuite

import (
	"bufio"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf16"

	"github.com/cubrid-systems/cubrid-testkit/internal/result"
)

// caseSet is what a run's discovery found: every case, in CQT's order, and the
// answer each one that runs is judged against.
type caseSet struct {
	all []string
	// answer holds only the cases that run. CQT's build() keeps a case that
	// has no answer in its list and in N, and never runs it.
	answer map[string]string
}

// cqtConfig is what CQT reads out of its configuration XML
// (sql/configuration/test_config/<jdbc_config_file>) that decides a verdict or
// a record: which answer variant wins, and what gets written.
type cqtConfig struct {
	runMode, runModeSecondary   string
	hasRunMode, hasSecondary    bool
	xmlSummary, answerInSummary bool
	queryPlanAll                bool // System.xml, not the test XML
}

// readCQTConfig reads the test XML and System.xml as PropertiesUtil and
// isPrintQueryPlan do: the first element of each name under the root, its own
// text untrimmed, and a flag that is the word "true" in any case.
func readCQTConfig(ctpHome, jdbcConfig string) (cqtConfig, error) {
	var c cqtConfig
	vals, err := rootChildren(filepath.Join(ctpHome, "sql", "configuration", "test_config", jdbcConfig))
	if err != nil {
		return c, err
	}
	c.runMode, c.hasRunMode = vals["run_mode"]
	c.runModeSecondary, c.hasSecondary = vals["run_mode_secondary"]
	c.xmlSummary = strings.EqualFold(vals["need_xml_summary"], "true")
	c.answerInSummary = strings.EqualFold(vals["need_answer_in_summary"], "true")
	if sys, err := rootChildren(filepath.Join(ctpHome, "sql", "configuration", "System.xml")); err == nil {
		c.queryPlanAll = strings.EqualFold(sys["queryPlan"], "true")
	}
	return c, nil
}

// rootChildren is the text of each element directly under the root, the first
// of each name.
func rootChildren(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	vals := map[string]string{}
	dec := xml.NewDecoder(f)
	depth := 0
	var name string
	var text strings.Builder
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return vals, nil
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			if depth == 2 {
				name = t.Name.Local
				text.Reset()
			}
		case xml.CharData:
			if depth == 2 {
				text.Write(t)
			}
		case xml.EndElement:
			if depth == 2 {
				if _, seen := vals[name]; !seen {
					vals[name] = text.String()
				}
			}
			depth--
		}
	}
}

// discover is CQT's discovery (ConsoleBO.getCaseFiles and build), which a run
// uses so that its order, its N and every relative path are CQT's: a sorted
// depth-first walk of the corpus, the exclusion file, then an answer for each
// case.
func discover(st *settings, cfg cqtConfig) (*caseSet, error) {
	var found []string
	walkCases(st.scenario, &found)

	if st.excludeFile != "" {
		patterns, err := lineList(st.excludeFile)
		if err != nil {
			return nil, err
		}
		// A file that is not there is no filter and no message: getLineList
		// returns null and filterExcludedCaseFile does nothing. Every shipped
		// conf names ${CTP_HOME}/conf/exclusions.txt, which CTP does not ship.
		if patterns != nil {
			kept := found[:0]
			for _, c := range found {
				rel := c[min(len(st.scenario), len(c)):]
				excluded := false
				for _, p := range patterns {
					if containPath(rel, p) {
						excluded = true
						break
					}
				}
				if !excluded {
					kept = append(kept, c)
				}
			}
			found = kept
		}
	}

	set := &caseSet{answer: map[string]string{}}
	seen := map[string]bool{}
	for _, c := range found {
		if seen[c] {
			continue
		}
		seen[c] = true
		set.all = append(set.all, c)
		answer := result.AnswerPath(c)
		if !exists(answer) {
			continue
		}
		if cfg.hasRunMode && cfg.runMode != "" {
			// runModeSecondary is null when the XML has no such element, and
			// Java's string concatenation makes that "null".
			secondary := cfg.runModeSecondary
			if !cfg.hasSecondary {
				secondary = "null"
			}
			if exists(answer + "_" + cfg.runMode) {
				answer += "_" + cfg.runMode
			} else if exists(answer + "_" + secondary) {
				answer += "_" + secondary
			}
		}
		set.answer[c] = answer
	}
	return set, nil
}

// walkCases is TestUtil.getCaseFiles: File.list sorted by String.compareTo,
// directories recursed where they are met, answers and common skipped.
func walkCases(path string, out *[]string) {
	st, err := os.Stat(path)
	if err != nil {
		return
	}
	if strings.Contains(strings.TrimSuffix(path, "/")+"/", "/answers/") {
		return
	}
	if !st.IsDir() {
		if strings.HasSuffix(path, ".sql") {
			*out = append(*out, path)
		}
		return
	}
	d, err := os.Open(path)
	if err != nil {
		return
	}
	names, err := d.Readdirnames(-1)
	d.Close()
	if err != nil {
		return
	}
	sort.Slice(names, func(i, j int) bool { return javaLess(names[i], names[j]) })
	sep := "/"
	if strings.HasSuffix(path, "/") {
		sep = ""
	}
	for _, n := range names {
		child := path + sep + n
		if cst, err := os.Stat(child); err == nil && cst.IsDir() && n != "common" && n != "answers" {
			walkCases(child, out)
		} else if strings.HasSuffix(child, ".sql") {
			*out = append(*out, child)
		}
	}
}

// lineList is CommonUtils.getLineList: the file's lines, trimmed, blank ones
// dropped -- and nil, not an error, for a file that is not there.
func lineList(path string) ([]string, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	lines := []string{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		if l := javaTrim(sc.Text()); l != "" {
			lines = append(lines, l)
		}
	}
	return lines, sc.Err()
}

// containPath is CommonUtils.containPath: a pattern that does not end in "/"
// or ".sql" gets a "/", and it matches anywhere in the path relative to the
// corpus root -- a substring, not a prefix.
func containPath(rel, pattern string) bool {
	if rel == "" || pattern == "" {
		return false
	}
	rel = strings.ReplaceAll(javaTrim(rel), `\`, "/")
	pattern = strings.ReplaceAll(javaTrim(pattern), `\`, "/")
	if !strings.HasSuffix(pattern, "/") && !strings.HasSuffix(pattern, ".sql") {
		pattern += "/"
	}
	return strings.Contains(rel, pattern)
}

// javaTrim is String.trim: every character at or below U+0020, from both ends.
func javaTrim(s string) string {
	return strings.TrimFunc(s, func(r rune) bool { return r <= ' ' })
}

// javaLess is String.compareTo: UTF-16 code units.
func javaLess(a, b string) bool {
	ua, ub := utf16.Encode([]rune(a)), utf16.Encode([]rune(b))
	for i := 0; i < len(ua) && i < len(ub); i++ {
		if ua[i] != ub[i] {
			return ua[i] < ub[i]
		}
	}
	return len(ua) < len(ub)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
