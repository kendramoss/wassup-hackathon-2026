package main

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Report is the outcome of Check. Failures make the command exit non-zero;
// Notes are informational (for example a cited line number that has moved).
type Report struct {
	Passed   []string
	Failures []string
	Notes    []string
}

func (r *Report) fail(format string, a ...any) {
	r.Failures = append(r.Failures, fmt.Sprintf(format, a...))
}

func (r *Report) pass(s string) { r.Passed = append(r.Passed, s) }

var (
	keyRow      = regexp.MustCompile("^\\|\\s*`([a-z0-9_]+(?:\\.[a-z0-9_]+)+)`\\s*\\|\\s*([^|]*?)\\s*\\|")
	manifestRow = regexp.MustCompile(`^([0-9a-f]{64})  (\S+)$`)
	traceRef    = regexp.MustCompile("`([A-Za-z0-9_./-]+\\.(?:go|yml|yaml|json|md))(?::(\\d+)(?:-(\\d+))?)?(?:#([^`\\s]+))?`")
	privateRef  = regexp.MustCompile(`(/Users/|/home/|[A-Za-z]:\\\\|~/)`)
)

// Check runs every check against the repository at root and the document at
// docPath. It reads files only.
func Check(root, docPath string) Report {
	var r Report
	rel := func(p string) string {
		if x, err := filepath.Rel(root, p); err == nil {
			return filepath.ToSlash(x)
		}
		return filepath.ToSlash(p)
	}

	kv, problems := Compute(root)
	for _, p := range problems {
		r.fail("artifacts: %s", p)
	}
	if len(problems) == 0 {
		r.pass(fmt.Sprintf("recomputed %d figures from the committed artifacts", len(kv)))
	}

	checkPinned(root, &r)
	checkShippedCopies(root, &r)
	checkResultsMD(root, kv, &r)
	checkBaselineSource(root, &r)
	checkStalePhrases(root, &r)

	docName := rel(docPath)
	b, err := os.ReadFile(docPath)
	if err != nil {
		r.fail("%s: %v", docName, unwrapPath(err))
		return r
	}
	doc := string(b)
	checkTable(doc, docName, kv, &r)
	checkManifest(root, doc, docName, &r)
	checkWording(doc, docName, &r)
	checkTrace(root, doc, docName, &r)
	return r
}

// checkPinned verifies eval/SHA256SUMS against the files it names.
func checkPinned(root string, r *Report) {
	f, err := os.Open(filepath.Join(root, filepath.FromSlash(sumsFile)))
	if err != nil {
		r.fail("%s: %v", sumsFile, unwrapPath(err))
		return
	}
	defer f.Close()
	seen := map[string]bool{}
	bad := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		m := manifestRow.FindStringSubmatch(strings.TrimSpace(sc.Text()))
		if m == nil {
			if strings.TrimSpace(sc.Text()) != "" {
				r.fail("%s: malformed line %q", sumsFile, sc.Text())
				bad++
			}
			continue
		}
		seen[m[2]] = true
		got, err := fileSum(filepath.Join(root, "eval", filepath.FromSlash(m[2])))
		switch {
		case err != nil:
			r.fail("%s: %s: %v", sumsFile, m[2], err)
			bad++
		case got != m[1]:
			r.fail("%s: %s checksum mismatch (recorded %s, actual %s)", sumsFile, m[2], m[1][:12], got[:12])
			bad++
		}
	}
	for _, want := range []string{"data/labels.jsonl", "data/labels_usmca.jsonl", "splits/splits_final.json"} {
		if !seen[want] {
			r.fail("%s: does not pin %s", sumsFile, want)
			bad++
		}
	}
	if bad == 0 {
		r.pass(fmt.Sprintf("%s: %d pinned files match", sumsFile, len(seen)))
	}
}

// checkShippedCopies requires the labelled corpora the backend ships to be
// byte-identical to the evaluated (and checksum-pinned) copies in eval/.
func checkShippedCopies(root string, r *Report) {
	ok := true
	for _, p := range [][2]string{{"eval/data/labels.jsonl", "backend/data/labels.jsonl"}, {"eval/data/labels_usmca.jsonl", "backend/data/labels_usmca.jsonl"}} {
		a, errA := fileSum(filepath.Join(root, filepath.FromSlash(p[0])))
		b, errB := fileSum(filepath.Join(root, filepath.FromSlash(p[1])))
		switch {
		case errA != nil || errB != nil:
			continue // a missing file is reported where it is read
		case a != b:
			r.fail("%s differs from the pinned %s", p[1], p[0])
			ok = false
		}
	}
	if ok {
		r.pass("backend/data labelled corpora are byte-identical to the pinned eval copies")
	}
}

// checkResultsMD makes sure the read-only results page still states the
// headline numbers the rows give. It never changes the page.
func checkResultsMD(root string, kv []KV, r *Report) {
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(resultsMD)))
	if err != nil {
		r.fail("%s: %v", resultsMD, unwrapPath(err))
		return
	}
	text := strings.Join(strings.Fields(string(b)), " ")
	get := func(k string) string { return lookup(kv, k) }
	if get("final.scored") == "" || get("final.fast.n") == "" {
		return // already reported as an artifact problem
	}
	want := []string{
		fmt.Sprintf("%s / %s = %s%%", get("final.fast.n"), get("final.scored"), get("final.fast.coverage_pct")),
		fmt.Sprintf("%s / %s = %s%%", get("final.fast.correct"), get("final.fast.n"), get("final.fast.accuracy_pct")),
		get("final.accuracy_pct") + "%",
	}
	ok := true
	for _, w := range want {
		if !strings.Contains(text, w) {
			r.fail("%s no longer states %q", resultsMD, w)
			ok = false
		}
	}
	if ok {
		r.pass(resultsMD + " states the recomputed headline numbers")
	}
}

// checkBaselineSource fails if the research file behind the 60.5% baseline is
// ever committed: the "unverifiable from the repository" finding would then
// be stale and the document must be revisited.
func checkBaselineSource(root string, r *Report) {
	found := ""
	for _, top := range []string{"eval", "docs"} {
		_ = filepath.WalkDir(filepath.Join(root, top), func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.EqualFold(d.Name(), "RESULTS-v2.md") {
				found = rel2(root, p)
			}
			return nil
		})
	}
	if found != "" {
		r.fail("baseline: %s is now committed; the 60.5%% finding (unverifiable from the repository) must be re-derived", found)
		return
	}
	r.pass("baseline source RESULTS-v2.md is still not in the repository")
}

func rel2(root, p string) string {
	x, err := filepath.Rel(root, p)
	if err != nil {
		return filepath.ToSlash(p)
	}
	return filepath.ToSlash(x)
}

// checkTable compares the document's key/value rows with the recomputed ones.
// Thousands separators, a trailing % and surrounding emphasis are ignored.
func checkTable(doc, docName string, kv []KV, r *Report) {
	norm := func(s string) string {
		s = strings.Trim(strings.TrimSpace(s), "*")
		s = strings.ReplaceAll(s, ",", "")
		return strings.TrimSuffix(s, "%")
	}
	have := map[string]string{}
	var order []string
	for _, line := range strings.Split(doc, "\n") {
		if m := keyRow.FindStringSubmatch(line); m != nil {
			if _, dup := have[m[1]]; dup {
				r.fail("%s: figure `%s` is listed twice", docName, m[1])
			}
			have[m[1]] = norm(m[2])
			order = append(order, m[1])
		}
	}
	want := map[string]string{}
	bad := 0
	for _, e := range kv {
		want[e.Key] = e.Value
		got, ok := have[e.Key]
		switch {
		case !ok:
			r.fail("%s: figure `%s` is missing (recomputed %s)", docName, e.Key, e.Value)
			bad++
		case got != norm(e.Value):
			r.fail("%s: figure `%s` is %s but the artifacts give %s", docName, e.Key, got, e.Value)
			bad++
		}
	}
	for _, k := range order {
		if _, ok := want[k]; !ok && len(kv) > 0 {
			r.fail("%s: figure `%s` is not one the check recomputes", docName, k)
			bad++
		}
	}
	if bad == 0 {
		r.pass(fmt.Sprintf("%s: %d figures equal the recomputed values", docName, len(kv)))
	}
}

// checkManifest requires the document to carry a checksum for every file the
// check reads and every checksum to match the file now.
func checkManifest(root, doc, docName string, r *Report) {
	have := map[string]string{}
	for _, line := range strings.Split(doc, "\n") {
		if m := manifestRow.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			have[m[2]] = m[1]
		}
	}
	bad := 0
	for _, rel := range requiredManifest {
		want, ok := have[rel]
		if !ok {
			r.fail("%s: checksum manifest has no entry for %s", docName, rel)
			bad++
			continue
		}
		got, err := entryHash(root, rel)
		switch {
		case err != nil:
			r.fail("%s: checksum of %s: %v", docName, rel, err)
			bad++
		case got != want:
			r.fail("%s: checksum mismatch for %s (document %s, actual %s)", docName, rel, want[:12], got[:12])
			bad++
		}
	}
	if bad == 0 {
		r.pass(fmt.Sprintf("%s: %d manifest checksums match", docName, len(requiredManifest)))
	}
}

// checkWording refuses unqualified statements of the figures the repository
// cannot support, and private references in a public document.
func checkWording(doc, docName string, r *Report) {
	rules := []struct {
		needle string
		words  []string
		why    string
	}{
		{"60.5", []string{"unverified", "unverifiable"}, "the 60.5 baseline cannot be reproduced from the repository"},
		{"29.7", []string{"unverified", "unverifiable"}, "the all-case lift rests on the unverified baseline"},
		{"35.5", []string{"unsupported", "not like-for-like"}, "the 35.5 lift compares a 300-case subset with an all-case baseline"},
	}
	bad := 0
	for i, line := range strings.Split(doc, "\n") {
		low := strings.ToLower(line)
		for _, ru := range rules {
			if !strings.Contains(line, ru.needle) {
				continue
			}
			ok := false
			for _, w := range ru.words {
				if strings.Contains(low, w) {
					ok = true
				}
			}
			if !ok {
				r.fail("%s:%d: wording: a line stating %s must say %s (%s)", docName, i+1, ru.needle, strings.Join(ru.words, " or "), ru.why)
				bad++
			}
		}
		if privateRef.MatchString(line) {
			r.fail("%s:%d: private path reference in a public document", docName, i+1)
			bad++
		}
	}
	if bad == 0 {
		r.pass(docName + ": wording rules hold")
	}
}

// stalePhrases are statements that later work made false. They fail
// wherever they appear in public text, so a merge cannot bring one back.
// docs/DECISIONS.md is exempt: it is a dated log, and an entry stays as
// written when a later entry supersedes it.
//
// unless, when set, exempts a line that also matches it (a figure stated
// beside the label or the later figure that makes it true). skip names files
// that keep the phrase as a dated record.
var stalePhrases = []struct {
	re     *regexp.Regexp
	why    string
	unless *regexp.Regexp
	skip   []string
}{
	{
		re:  regexp.MustCompile(`(?i)\bno (scored )?part 102 plan searche?s?\b|part 102 plan searche?s?[^.]{0,40}\b(scored|to score) yet`),
		why: "three Part 102 plan searches were scored in D502",
	},
	{
		re:  regexp.MustCompile(`(?i)\bdo(es)? not apply [^.]{0,30}\bpart 102\b`),
		why: "plans on Part 102 cases use the Part 102 engine and the 102.20 order (D500, D541, D562)",
	},
	{
		re:  regexp.MustCompile(`(?i)plan move[^.]{0,20}\(built, display only\)`),
		why: "the per-move check orders shown plans on Part 102 cases (D500, D562)",
	},
	{
		re:  regexp.MustCompile(`(?i)general engine, in this run`),
		why: "/examples/part102-plan shows the Part 102 engine's run since D562",
	},
	{
		re:  regexp.MustCompile(`(?i)stricter checks passed 9 of 12`),
		why: "lead with the 8 of 8 runs the budget did not touch (D511, D562)",
	},
	{
		re:  regexp.MustCompile(`(?i)\b(14|fourteen) product families\b|\bon the (14|fourteen) families\b`),
		why: "plans cover 20 product families since D580 (#634, D620)",
	},
	{
		re:  regexp.MustCompile(`(?i)\b(20|twenty) product families\b|\b20 families with (a plan )?templates?\b|\b20 family templates\b|\b(the|has) 20 templates\b|\bpins the 20 families\b|\(20 families, listed`),
		why: "plans cover 27 product families since D826",
	},
	{
		re:  regexp.MustCompile(`(?i)\b(26|twenty-six) product families\b|\b26 families with (a plan )?templates?\b|\b26 family templates\b|\b(the|has) 26 templates\b|\bpins the 26 families\b|\(26 families, listed`),
		why: "plans cover 27 product families since D826 (optical and measuring instruments added; lifting machines folded into off-road and lifting machinery)",
	},
	{
		re:     regexp.MustCompile(`14 of 29 held[ -]out`),
		why:    "give the held-out figure without the five chapters picked with held-out data, 12 of 29, beside it (D560, #631, D620)",
		unless: regexp.MustCompile(`12 of 29`),
	},
	{
		re:     regexp.MustCompile(`(?i)agrees? with the (102\.20 )?rule on 13 of (the )?14|agreed with the (102\.20 )?rule on 3 of 6`),
		why:    "these are usmca-v1's figures; the production Part 102 engine is usmca-v5 since D602, so name usmca-v1 on the line (D620)",
		unless: regexp.MustCompile(`usmca-v1`),
		skip:   []string{"docs/EVALUATION.md"},
	},
	{
		re:     regexp.MustCompile(`100%[^%]{0,60}\b(46\.9|47)%`),
		why:    "the 100% at confidence 0.95 on 46.9% of Part 102 rulings is usmca-v1's; usmca-v5 auto-accepts 11 of 32 (D613, D620)",
		unless: regexp.MustCompile(`usmca-v1`),
	},
	{
		re:  regexp.MustCompile("(?i)version `?usmca-v1`?[^;.]{0,20}\\)"),
		why: "the production Part 102 engine is usmca-v5 (engine.USMCAProduction) since D602; name usmca-v1 only as the engine before it (#643, D642)",
	},
	{
		re:  regexp.MustCompile(`(?i)\btheir clean check\b|part 102[^.]{0,200}\bclean check is\b`),
		why: "usmca-v5 has no clean held-out check: both windows were scored first with usmca-v1 (D602), and window 2 had a 15-ruling precedent pool (D612) (#643, D642)",
	},
	{
		re:     regexp.MustCompile(`(?i)part 102 marking 11/13|11/13 = 84\.6%|usmca-v[0-9][^|]{0,120}\b11 of 13\b|\b11 of 13\b[^|]{0,60}\busmca-v[0-9]`),
		why:    "window 2's Part 102 counts were measured with only the 15 window-2 rulings as precedents; say so beside them (D612, #643, D642)",
		unless: regexp.MustCompile(`D612|15 window-2 rulings`),
	},
	{
		re:  regexp.MustCompile(`(?i)\bfirst held-out evidence\b|\bD602 change carries over\b`),
		why: "the D631 plan pass is a candidate comparison on window-1 rulings usmca-v5 was scored on in D601 before its adoption, not new held-out evidence (#649, D670)",
	},
	{
		re:     regexp.MustCompile(`(?i)usmca-v5[^.]{0,160}\bheld-out evidence\b|\bheld-out evidence\b[^.]{0,160}usmca-v5`),
		why:    "usmca-v5 has no clean held-out check: its held-out scores and the D631 plan pass are candidate comparisons (D602, #649, D670)",
		unless: regexp.MustCompile(`(?i)\bnot (clean |new )?held-out evidence\b|candidate comparison`),
	},
	{
		re:     regexp.MustCompile(`(?i)\bmisses? one more current setup\b|\bN363381\b[^.]{0,120}\bnew miss\b`),
		why:    "D631's 'Worse' reading of N363381 is superseded: D571 did not plan it and D601 already scored it wrong (#658, D670)",
		unless: regexp.MustCompile(`(?i)\bnot a new miss\b|\balready scored it wrong\b|\bnot planned\b`),
	},
}

// staleRoots are the public texts checkStalePhrases reads: the README, the
// docs, the linked results page, the site, the MCP server and the backend's
// user-facing strings.
var staleRoots = []string{"README.md", "docs", "eval/RESULTS.md", "frontend/src", "frontend/scripts", "mcp", "backend/internal", "examples"}

// checkStalePhrases fails on any stalePhrases match in staleRoots.
func checkStalePhrases(root string, r *Report) {
	exts := map[string]bool{".md": true, ".ts": true, ".tsx": true, ".mjs": true, ".go": true, ".py": true, ".json": true, ".txt": true, ".yaml": true, ".yml": true}
	bad := 0
	for _, top := range staleRoots {
		start := filepath.Join(root, filepath.FromSlash(top))
		_ = filepath.WalkDir(start, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // a missing root is fine: not every checkout has every part
			}
			if d.IsDir() {
				if n := d.Name(); n == "node_modules" || n == ".next" || n == "testdata" {
					return filepath.SkipDir
				}
				return nil
			}
			rel, _ := filepath.Rel(root, p)
			rel = filepath.ToSlash(rel)
			if !exts[filepath.Ext(p)] || rel == "docs/DECISIONS.md" {
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return nil
			}
			for i, line := range strings.Split(string(b), "\n") {
				for _, sp := range stalePhrases {
					if slices.Contains(sp.skip, rel) || (sp.unless != nil && sp.unless.MatchString(line)) {
						continue
					}
					if m := sp.re.FindString(line); m != "" {
						r.fail("%s:%d: wording: stale phrase %q (%s)", rel, i+1, m, sp.why)
						bad++
					}
				}
			}
			return nil
		})
	}
	if bad == 0 {
		r.pass("no stale phrases in the public text")
	}
}

// checkTrace resolves every `path:line#symbol` reference. A missing file, a
// missing symbol or a line past the end of the file fails; a symbol that
// moved to another line is only a note, because unrelated edits shift lines.
func checkTrace(root, doc, docName string, r *Report) {
	n, bad := 0, 0
	for _, m := range traceRef.FindAllStringSubmatch(doc, -1) {
		path, lineS, anchor := m[1], m[2], m[4]
		if lineS == "" && anchor == "" {
			continue // a plain file mention, not a trace
		}
		n++
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			r.fail("trace: %s: %v", path, unwrapPath(err))
			bad++
			continue
		}
		lines := strings.Split(string(b), "\n")
		line := 0
		if lineS != "" {
			line, _ = strconv.Atoi(lineS)
			if line < 1 || line > len(lines) {
				r.fail("trace: %s:%d is beyond the end of the file (%d lines)", path, line, len(lines))
				bad++
				continue
			}
		}
		if anchor == "" {
			continue
		}
		at := declLines(path, anchor, lines)
		if len(at) == 0 {
			r.fail("trace: %s does not contain %s", path, anchor)
			bad++
			continue
		}
		if line > 0 {
			onLine := false
			for _, l := range at {
				if l == line {
					onLine = true
				}
			}
			if !onLine {
				r.Notes = append(r.Notes, fmt.Sprintf("trace: %s#%s is at line %d, the document says %d", path, anchor, at[0], line))
			}
		}
	}
	if bad == 0 {
		r.pass(fmt.Sprintf("%s: %d code/test references resolve", docName, n))
	}
}

// declLines returns the 1-based lines where anchor is declared in a Go file,
// or where it occurs as text in any other file.
//
// Test names must be declared as functions. Other Go anchors are declarations
// when one exists; failing that they may be literal text (a hard-coded value
// is traced by quoting its key).
func declLines(path, anchor string, lines []string) []int {
	var at []int
	if strings.HasSuffix(path, ".go") {
		re := regexp.MustCompile(`^(?:func\s+(?:\([^)]*\)\s*)?|type\s+|const\s+|var\s+|\t+)` + regexp.QuoteMeta(anchor) + `\b`)
		for i, l := range lines {
			if re.MatchString(l) {
				at = append(at, i+1)
			}
		}
		if len(at) > 0 || strings.HasPrefix(anchor, "Test") {
			return at
		}
	}
	for i, l := range lines {
		if strings.Contains(l, anchor) {
			at = append(at, i+1)
		}
	}
	return at
}
