package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// File names are fixed: the official run of record is the frozen Vertex AI
// engine-v1 run, scored once (eval/RESULTS.md).
const (
	finalRows  = "eval/results/final-gemini-4-argon-prec-engine-v1-vertex.jsonl"
	finalScope = "eval/results/scope-final-router-v2-vertex.jsonl"
	devRows    = "eval/results/dev-gemini-4-argon-prec-engine-v1-vertex.jsonl"
	devScope   = "eval/results/scope-dev-router-v2-vertex.jsonl"
	adjustFile = "eval/data/scoring_adjustments.json"
	splitsFile = "eval/splits/splits_final.json"
	labelsFile = "eval/data/labels.jsonl"
	usmcaFile  = "eval/data/labels_usmca.jsonl"
	runlogFile = "eval/results/RUNLOG.jsonl"
	sliceFile  = "eval/results/slice30-baseline.json"
	calibFile  = "eval/results/calibration-by-family.json"
	resultsMD  = "eval/RESULTS.md"
	sumsFile   = "eval/SHA256SUMS"
	factsDir   = "backend/data/facts"

	finalRunName = "final-gemini-4-argon-prec-engine-v1-vertex"

	// fastPathBar is the auto-accept confidence bar of the frozen config.
	fastPathBar = 0.92
	// cutoff is the last ruling date of the labelled corpus.
	cutoff = "2026-09-25"
)

// KV is one recomputed figure.
type KV struct{ Key, Value string }

type engineRow struct {
	Ruling     string   `json:"ruling"`
	Date       string   `json:"date"`
	Correct    bool     `json:"correct"`
	Pred       string   `json:"pred"`
	Confidence float64  `json:"confidence"`
	Error      string   `json:"error"`
	Retrieval  string   `json:"retrieval"`
	Method     string   `json:"label_method"`
	Precedents []string `json:"precedents"`
}

type scopeRow struct {
	Ruling  string `json:"ruling"`
	InScope bool   `json:"in_scope"`
}

type labelRow struct {
	Ruling        string  `json:"ruling"`
	Date          string  `json:"date"`
	PrimaryOrigin *string `json:"primary_origin"`
}

type adjustments struct {
	Exclude      map[string]string   `json:"exclude"`
	Alternatives map[string][]string `json:"alternatives"`
}

func readJSONL[T any](root, rel string) ([]T, error) {
	f, err := os.Open(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", rel, unwrapPath(err))
	}
	defer f.Close()
	var out []T
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var v T
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", rel, n, err)
		}
		out = append(out, v)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", rel, err)
	}
	return out, nil
}

func readJSON(root, rel string, v any) error {
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return fmt.Errorf("%s: %w", rel, unwrapPath(err))
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("%s: %w", rel, err)
	}
	return nil
}

// unwrapPath drops the absolute path os errors carry, so messages name the
// repository-relative path only.
func unwrapPath(err error) error {
	if pe, ok := err.(*os.PathError); ok {
		return pe.Err
	}
	return err
}

func pct(n, d int) string {
	if d == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%.1f", math.Round(1000*float64(n)/float64(d))/10)
}

// rowOK scores one row the way the evaluation does: a ruling with an
// alternatives entry is right when every "/"-separated country predicted is
// one CBP allowed; otherwise the row's own correct flag stands.
func (a adjustments) rowOK(r engineRow) bool {
	alts, ok := a.Alternatives[r.Ruling]
	if !ok || r.Error != "" {
		return r.Correct
	}
	n := 0
	for _, p := range strings.Split(r.Pred, "/") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n++
		found := false
		for _, x := range alts {
			if x == p {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return n > 0
}

type kvs struct{ list []KV }

func (k *kvs) add(key string, v any) { k.list = append(k.list, KV{key, fmt.Sprint(v)}) }

func ids[T any](rows []T, id func(T) string) map[string]bool {
	m := map[string]bool{}
	for _, r := range rows {
		m[id(r)] = true
	}
	return m
}

func diffSets(a, b map[string]bool) (onlyA, onlyB int) {
	for k := range a {
		if !b[k] {
			onlyA++
		}
	}
	for k := range b {
		if !a[k] {
			onlyB++
		}
	}
	return
}

// splitMetrics scores one split. final gets the extra confidence-only and
// calibration-band figures.
func splitMetrics(out *kvs, probs *[]string, root, name, rowsRel, scopeRel string, adj adjustments, splitIDs map[string]bool, extra bool) (rows []engineRow, ok bool) {
	rows, err := readJSONL[engineRow](root, rowsRel)
	if err != nil {
		*probs = append(*probs, err.Error())
		return nil, false
	}
	scopes, err := readJSONL[scopeRow](root, scopeRel)
	if err != nil {
		*probs = append(*probs, err.Error())
		return nil, false
	}
	inScope := map[string]bool{}
	for _, s := range scopes {
		inScope[s.Ruling] = s.InScope
	}
	haveScope := ids(scopes, func(s scopeRow) string { return s.Ruling })

	var scored, correct, rawCorrect, errRows, excluded, missingScope, outScope int
	var fastN, fastC, confN, confC int
	var bandN, bandC [4]int
	notBM25, regex, manual := 0, 0, 0
	dmin, dmax := "", ""
	for _, r := range rows {
		if r.Correct {
			rawCorrect++
		}
		if r.Retrieval != "" && r.Retrieval != "bm25" {
			notBM25++
		}
		switch r.Method {
		case "regex":
			regex++
		case "manual":
			manual++
		}
		if r.Error != "" {
			errRows++
		}
		if dmin == "" || r.Date < dmin {
			dmin = r.Date
		}
		if r.Date > dmax {
			dmax = r.Date
		}
		if !haveScope[r.Ruling] {
			missingScope++
		}
		if !inScope[r.Ruling] {
			outScope++
		}
		if _, skip := adj.Exclude[r.Ruling]; skip {
			excluded++
			continue
		}
		ok := adj.rowOK(r)
		scored++
		if ok {
			correct++
		}
		if r.Confidence >= fastPathBar {
			confN++
			if ok {
				confC++
			}
			if inScope[r.Ruling] {
				fastN++
				if ok {
					fastC++
				}
			}
		}
		b := 3
		switch {
		case r.Confidence >= 0.95:
			b = 0
		case r.Confidence >= 0.90:
			b = 1
		case r.Confidence >= 0.80:
			b = 2
		}
		bandN[b]++
		if ok {
			bandC[b]++
		}
	}
	p := name + "."
	out.add(p+"rows", len(rows))
	out.add(p+"excluded", excluded)
	out.add(p+"scored", scored)
	out.add(p+"raw_correct", rawCorrect)
	out.add(p+"correct", correct)
	out.add(p+"accuracy_pct", pct(correct, scored))
	out.add(p+"error_rows", errRows)
	out.add(p+"rows_missing_scope", missingScope)
	out.add(p+"rows_not_bm25", notBM25)
	out.add(p+"label_method_regex", regex)
	out.add(p+"label_method_manual", manual)
	out.add(p+"out_of_scope", outScope)
	out.add(p+"date_min", dmin)
	out.add(p+"date_max", dmax)
	out.add(p+"fast.n", fastN)
	out.add(p+"fast.correct", fastC)
	out.add(p+"fast.coverage_pct", pct(fastN, scored))
	out.add(p+"fast.accuracy_pct", pct(fastC, fastN))
	if extra {
		out.add(p+"fast_conf_only.n", confN)
		out.add(p+"fast_conf_only.correct", confC)
		out.add(p+"fast_conf_only.coverage_pct", pct(confN, scored))
		out.add(p+"fast_conf_only.accuracy_pct", pct(confC, confN))
		for i, n := range []string{"band_ge95", "band_90_95", "band_80_90", "band_lt80"} {
			out.add(p+n+".n", bandN[i])
			out.add(p+n+".correct", bandC[i])
			out.add(p+n+".accuracy_pct", pct(bandC[i], bandN[i]))
		}
	}
	if splitIDs != nil {
		onlyRows, onlySplit := diffSets(ids(rows, func(r engineRow) string { return r.Ruling }), splitIDs)
		if onlyRows != 0 || onlySplit != 0 {
			*probs = append(*probs, fmt.Sprintf("%s: %s ruling ids differ from %s (%d only in results, %d only in split)", rowsRel, name, splitsFile, onlyRows, onlySplit))
		}
	}
	return rows, true
}

// Compute recomputes every figure the document states from the committed
// artifacts. problems lists files that are missing or unreadable and
// artifacts that contradict each other; figures that could not be computed
// are simply absent.
func Compute(root string) (kv []KV, problems []string) {
	out := &kvs{}
	var adj adjustments
	if err := readJSON(root, adjustFile, &adj); err != nil {
		problems = append(problems, err.Error())
	}
	var splits map[string]json.RawMessage
	splitIDs := map[string]map[string]bool{}
	if err := readJSON(root, splitsFile, &splits); err != nil {
		problems = append(problems, err.Error())
	} else {
		for _, k := range []string{"final", "dev", "pilot"} {
			var l []string
			if err := json.Unmarshal(splits[k], &l); err != nil {
				problems = append(problems, fmt.Sprintf("%s: key %q is not a list of ruling ids", splitsFile, k))
				continue
			}
			set := map[string]bool{}
			for _, id := range l {
				set[id] = true
			}
			splitIDs[k] = set
			out.add("splits."+k, len(l))
		}
	}

	fRows, fok := splitMetrics(out, &problems, root, "final", finalRows, finalScope, adj, splitIDs["final"], true)
	dRows, dok := splitMetrics(out, &problems, root, "dev", devRows, devScope, adj, splitIDs["dev"], false)
	if fok && dok {
		fmin, dmax := "", ""
		for _, r := range fRows {
			if fmin == "" || r.Date < fmin {
				fmin = r.Date
			}
		}
		for _, r := range dRows {
			if r.Date > dmax {
				dmax = r.Date
			}
		}
		out.add("temporal.final_starts_after_dev_ends", fmin > dmax)
	}

	labels, lerr := readJSONL[labelRow](root, labelsFile)
	usmca, uerr := readJSONL[labelRow](root, usmcaFile)
	if lerr != nil {
		problems = append(problems, lerr.Error())
	}
	if uerr != nil {
		problems = append(problems, uerr.Error())
	}
	labelIDs, usmcaIDs := map[string]bool{}, map[string]bool{}
	if lerr == nil {
		lmin, lmax, withOrigin, after := "", "", 0, 0
		for _, r := range labels {
			labelIDs[r.Ruling] = true
			if lmin == "" || r.Date < lmin {
				lmin = r.Date
			}
			if r.Date > lmax {
				lmax = r.Date
			}
			if r.PrimaryOrigin != nil {
				withOrigin++
			}
			if r.Date > cutoff {
				after++
			}
		}
		out.add("labels.rows", len(labels))
		out.add("labels.date_min", lmin)
		out.add("labels.date_max", lmax)
		out.add("labels.with_primary_origin", withOrigin)
		out.add("labels.rows_after_cutoff", after)
	}
	if uerr == nil {
		umin, umax, withOrigin := "", "", 0
		for _, r := range usmca {
			usmcaIDs[r.Ruling] = true
			if umin == "" || r.Date < umin {
				umin = r.Date
			}
			if r.Date > umax {
				umax = r.Date
			}
			if r.PrimaryOrigin != nil {
				withOrigin++
			}
		}
		out.add("usmca.rows", len(usmca))
		out.add("usmca.date_min", umin)
		out.add("usmca.date_max", umax)
		out.add("usmca.with_primary_origin", withOrigin)
	}
	if lerr == nil && uerr == nil {
		overlap := 0
		for id := range usmcaIDs {
			if labelIDs[id] {
				overlap++
			}
		}
		union := map[string]bool{}
		for id := range labelIDs {
			union[id] = true
		}
		for id := range usmcaIDs {
			union[id] = true
		}
		if fok && splitIDs["final"] != nil {
			dates := map[string]string{}
			for _, r := range labels {
				dates[r.Ruling] = r.Date
			}
			for _, r := range usmca {
				dates[r.Ruling] = r.Date
			}
			var slots, inFinal, casesWith, notBefore, undated int
			for _, r := range fRows {
				has := false
				for _, p := range r.Precedents {
					slots++
					if splitIDs["final"][p] {
						inFinal++
						has = true
					}
					d, ok := dates[p]
					switch {
					case !ok:
						undated++
					case d >= r.Date:
						notBefore++
					}
				}
				if has {
					casesWith++
				}
			}
			out.add("final.precedent_slots", slots)
			out.add("final.precedent_slots_not_before_case", notBefore)
			out.add("final.precedent_slots_undated", undated)
			out.add("final.precedent_slots_in_final_split", inFinal)
			out.add("final.cases_with_final_split_precedent", casesWith)
			if notBefore > 0 {
				problems = append(problems, fmt.Sprintf("%s: %d retrieved precedents are not dated strictly before their case", finalRows, notBefore))
			}
		}
		out.add("usmca.overlap_with_labels", overlap)
		out.add("usmca.not_in_labels", len(usmcaIDs)-overlap)
		out.add("pool.union", len(union))

		names, err := factNames(root)
		if err != nil {
			problems = append(problems, err.Error())
		} else {
			out.add("facts.files", len(names))
			facts := map[string]bool{}
			for _, n := range names {
				facts[strings.TrimSuffix(n, ".txt")] = true
			}
			a, b := diffSets(facts, union)
			if a != 0 || b != 0 {
				problems = append(problems, fmt.Sprintf("%s: facts file names differ from labels + usmca ruling ids (%d only in facts, %d only in labels)", factsDir, a, b))
			}
		}
	}

	runs, err := readJSONL[map[string]any](root, runlogFile)
	if err != nil {
		problems = append(problems, err.Error())
	} else {
		var last map[string]any
		for _, r := range runs {
			if r["run"] == finalRunName {
				last = r
			}
		}
		if last == nil {
			problems = append(problems, fmt.Sprintf("%s: no row for run %q", runlogFile, finalRunName))
		} else {
			out.add("runlog.final.time", last["time"])
			for _, c := range []struct {
				field, key string
			}{{"cases", "final.rows"}, {"correct", "final.raw_correct"}, {"fast_path_coverage", "final.fast_conf_only.n"}, {"fast_path_correct", "final.fast_conf_only.correct"}} {
				rv, _ := last[c.field].(float64)
				if got := lookup(out.list, c.key); got != "" && got != strconv.Itoa(int(rv)) {
					problems = append(problems, fmt.Sprintf("%s: run %s %s = %v, but the result rows give %s = %s", runlogFile, finalRunName, c.field, last[c.field], c.key, got))
				}
			}
		}
	}

	var slice struct {
		Source  string `json:"source"`
		N       int    `json:"n"`
		Correct int    `json:"correct"`
		Band95N int    `json:"band95_n"`
		Band95C int    `json:"band95_correct"`
	}
	if err := readJSON(root, sliceFile, &slice); err != nil {
		problems = append(problems, err.Error())
	} else {
		out.add("slice.baseline.source", slice.Source)
		out.add("slice.baseline.n", slice.N)
		out.add("slice.baseline.correct", slice.Correct)
		out.add("slice.baseline.band95_n", slice.Band95N)
		out.add("slice.baseline.band95_correct", slice.Band95C)
	}

	var calib struct {
		Splits []struct {
			Split  string `json:"split"`
			Global struct {
				Auto    int `json:"auto"`
				Correct int `json:"correct"`
			} `json:"global"`
		} `json:"splits"`
	}
	if err := readJSON(root, calibFile, &calib); err != nil {
		problems = append(problems, err.Error())
	} else {
		for _, s := range calib.Splits {
			if s.Split != "final" && s.Split != "dev" {
				continue
			}
			wn, wc := lookup(out.list, s.Split+".fast.n"), lookup(out.list, s.Split+".fast.correct")
			if wn != "" && (wn != strconv.Itoa(s.Global.Auto) || wc != strconv.Itoa(s.Global.Correct)) {
				problems = append(problems, fmt.Sprintf("%s: %s global policy auto/correct = %d/%d, but the result rows give %s/%s", calibFile, s.Split, s.Global.Auto, s.Global.Correct, wn, wc))
			}
		}
	}
	return out.list, problems
}

func lookup(kv []KV, key string) string {
	for _, e := range kv {
		if e.Key == key {
			return e.Value
		}
	}
	return ""
}

func factNames(root string) ([]string, error) {
	ents, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(factsDir)))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", factsDir, unwrapPath(err))
	}
	var names []string
	for _, e := range ents {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return nil, fmt.Errorf("%s: no fact files", factsDir)
	}
	return names, nil
}

// ManifestEntry is one pinned file: the sha256 of its bytes, or for the facts
// directory the sha256 of its sorted file-name list (so 3,000+ small files
// are pinned by one line instead of being duplicated).
type ManifestEntry struct{ Hash, Path string }

const factsNamesPath = "names:" + factsDir

// requiredManifest lists every file the check reads that eval/SHA256SUMS does
// not already pin.
var requiredManifest = []string{
	sumsFile, adjustFile,
	finalRows, finalScope, devRows, devScope,
	runlogFile, calibFile, sliceFile,
	"backend/data/labels.jsonl", "backend/data/labels_usmca.jsonl",
	factsNamesPath,
}

func fileSum(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", unwrapPath(err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Manifest hashes every required file under root.
func Manifest(root string) ([]ManifestEntry, error) {
	var out []ManifestEntry
	for _, rel := range requiredManifest {
		h, err := entryHash(root, rel)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", rel, err)
		}
		out = append(out, ManifestEntry{h, rel})
	}
	return out, nil
}

func entryHash(root, rel string) (string, error) {
	if rel == factsNamesPath {
		names, err := factNames(root)
		if err != nil {
			return "", err
		}
		h := sha256.Sum256([]byte(strings.Join(names, "\n") + "\n"))
		return hex.EncodeToString(h[:]), nil
	}
	return fileSum(filepath.Join(root, filepath.FromSlash(rel)))
}

// RenderManifest formats entries as sha256sum lines.
func RenderManifest(m []ManifestEntry) string {
	var b strings.Builder
	for _, e := range m {
		fmt.Fprintf(&b, "%s  %s\n", e.Hash, e.Path)
	}
	return b.String()
}

var meanings = map[string]string{
	"final.rows":                           "official held-out result rows",
	"final.correct":                        "correct after scoring adjustments",
	"final.accuracy_pct":                   "accuracy, percent",
	"final.fast.n":                         "fast path: confidence >= 0.92 and router in scope",
	"final.fast.correct":                   "fast path correct",
	"final.fast.coverage_pct":              "fast path n / scored, percent",
	"final.fast.accuracy_pct":              "fast path accuracy, percent",
	"final.fast_conf_only.n":               "confidence >= 0.92 ignoring router scope",
	"final.date_min":                       "earliest ruling date",
	"final.date_max":                       "latest ruling date",
	"labels.rows":                          "labelled corpus rows",
	"pool.union":                           "labelled + Part 102 ruling ids",
	"facts.files":                          "fact files in backend/data/facts",
	"usmca.not_in_labels":                  "Part 102 rulings absent from the labelled corpus",
	"runlog.final.time":                    "run-of-record timestamp in RUNLOG",
	"temporal.final_starts_after_dev_ends": "held-out window starts after the dev window ends",
}

// RenderTable formats figures as the document's key/value rows.
func RenderTable(kv []KV) string {
	var b strings.Builder
	for _, e := range kv {
		fmt.Fprintf(&b, "| `%s` | %s | %s |\n", e.Key, e.Value, meanings[e.Key])
	}
	return b.String()
}
