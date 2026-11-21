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

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/calibrate"
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
	calMapFile = "backend/internal/calibrate/map.json"
	resultsMD  = "eval/RESULTS.md"
	sumsFile   = "eval/SHA256SUMS"
	factsDir   = "backend/data/facts"

	finalRunName = "final-gemini-4-argon-prec-engine-v1-vertex"

	// The post-cutoff flip pairs (D170): frozen pairs and the production
	// precedent check's rows, scored once with CBP's holding on the starting
	// setup given (known-a).
	postPairs = "eval/data/flips_post.jsonl"
	postRows  = "eval/results/flips-post-verifier-v2-known-a-litellm.jsonl"
	// Repeatability (D190): three development runs of the production engine
	// through the gateway, the first from the D152 sweep.
	repRun1 = "eval/results/dev-gemini-4-argon-prec-engine-v1-litellm-gw.jsonl"
	repRun2 = "eval/results/dev-gemini-4-argon-prec-engine-v1-litellm-router-rep2.jsonl"
	repRun3 = "eval/results/dev-gemini-4-argon-prec-engine-v1-litellm-router-rep3.jsonl"
	// Held-out repeatability (D240, D241): two fresh gateway repeats of the
	// frozen engine on each held-out split, beside the official run.
	finalRep1  = "eval/results/final-gemini-4-argon-prec-engine-v1-litellm-repeat1.jsonl"
	finalRep2  = "eval/results/final-gemini-4-argon-prec-engine-v1-litellm-repeat2.jsonl"
	final2Rep1 = "eval/results/final2-gemini-4-argon-prec-engine-v1-litellm-router-repeat1.jsonl"
	final2Rep2 = "eval/results/final2-gemini-4-argon-prec-engine-v1-litellm-router-repeat2.jsonl"

	// The other regimes' held-out runs, both windows (D210). Window 1 is
	// Jan-Sep 2026 ("final"); window 2 is the post-cutoff window ("final2"),
	// each split frozen before its one run.
	textileSplits  = "eval/splits/splits_textile.json"
	pharmaSplits   = "eval/splits/splits_pharma.json"
	usmcaSplits    = "eval/splits/splits_usmca.json"
	final2Splits   = "eval/splits/splits_final2.json"
	textile2Splits = "eval/splits/splits_textile_final2.json"
	pharma2Splits  = "eval/splits/splits_pharma_final2.json"
	usmca2Splits   = "eval/splits/splits_usmca_final2.json"
	textileRows    = "eval/results/textile-final-gemini-4-argon-prec-textile-v1-vertex.jsonl"
	pharmaRows     = "eval/results/pharma-final-gemini-4-argon-prec-pharma-v1-litellm.jsonl"
	usmcaRows      = "eval/results/usmca-final-gemini-4-argon-prec-usmca-v1-litellm.jsonl"
	final2Rows     = "eval/results/final2-gemini-4-argon-prec-engine-v1-litellm-router.jsonl"
	textile2Rows   = "eval/results/textile-final2-gemini-4-argon-prec-textile-v2-litellm-reasoning-v1-router.jsonl"
	pharma2Rows    = "eval/results/pharma-final2-gemini-4-argon-prec-pharma-v1-litellm-router.jsonl"
	usmca2Rows     = "eval/results/usmca-final2-gemini-4-argon-prec-usmca-v1-litellm-router.jsonl"

	// planBar is the precedent check's plan bar (config.DefaultPlanBar).
	planBar = 0.92

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
	calibratedMetrics(out, &problems, root, adj, map[string][]engineRow{"final": fRows, "dev": dRows}, map[string]string{"final": finalScope, "dev": devScope})
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

	plannerPost(out, &problems, root)
	repeatability(out, &problems, root, adj)
	heldoutRepeat(out, &problems, root, adj, "repeat_heldout.final", finalRows, finalRep1, finalRep2)
	heldoutRepeat(out, &problems, root, adj, "repeat_heldout.final2", final2Rows, final2Rep1, final2Rep2)
	allRegimes(out, &problems, root)
	routerChecks(out, &problems, root)

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

// calibratedMetrics recomputes the calibrated figures (D160) from the
// committed map and rows: the auto-accept set at the calibrated bar, mean
// stated and calibrated confidence, and ECE (one bin per value) before and
// after. It also checks that the calibrated bar accepts exactly the cases the
// stated 0.92 bar does, which the published auto-accept figures rely on.
func calibratedMetrics(out *kvs, probs *[]string, root string, adj adjustments, rows map[string][]engineRow, scopes map[string]string) {
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(calMapFile)))
	if err != nil {
		*probs = append(*probs, fmt.Sprintf("%s: %v", calMapFile, unwrapPath(err)))
		return
	}
	m, err := calibrate.Parse(b)
	if err != nil {
		*probs = append(*probs, err.Error())
		return
	}
	out.add("cal.version", m.Version)
	out.add("cal.bar", m.Bar)
	for _, name := range []string{"final", "dev"} {
		rs := rows[name]
		if rs == nil {
			continue
		}
		sc, err := readJSONL[scopeRow](root, scopes[name])
		if err != nil {
			*probs = append(*probs, err.Error())
			continue
		}
		in := map[string]bool{}
		for _, s := range sc {
			in[s.Ruling] = s.InScope
		}
		var scored []calibrate.Scored
		autoN, autoC, differ := 0, 0, 0
		for _, r := range rs {
			if _, skip := adj.Exclude[r.Ruling]; skip {
				continue
			}
			ok := adj.rowOK(r)
			scored = append(scored, calibrate.Scored{Stated: r.Confidence, OK: ok})
			calAuto := r.Error == "" && in[r.Ruling] && m.Value(r.Confidence) >= m.Bar
			if calAuto != (r.Error == "" && in[r.Ruling] && r.Confidence >= fastPathBar) {
				differ++
			}
			if calAuto {
				autoN++
				if ok {
					autoC++
				}
			}
		}
		rep := m.Evaluate(scored)
		p := name + ".cal."
		r4 := func(x float64) string { return fmt.Sprintf("%.4f", math.Round(x*1e4)/1e4) }
		r1 := func(x float64) string { return fmt.Sprintf("%.1f", math.Round(x*1e3)/10) }
		out.add(p+"fast.n", autoN)
		out.add(p+"fast.correct", autoC)
		out.add(p+"fast.accuracy_pct", pct(autoC, autoN))
		out.add(p+"mean_stated_pct", r1(rep.MeanStated))
		out.add(p+"mean_calibrated_pct", r1(rep.MeanCalibrated))
		out.add(p+"ece_stated", r4(rep.ECEStated))
		out.add(p+"ece_calibrated", r4(rep.ECECalibrated))
		if differ != 0 {
			*probs = append(*probs, fmt.Sprintf("%s: the calibrated bar %v and the stated bar %v accept different %s cases (%d differ)", calMapFile, m.Bar, fastPathBar, name, differ))
		}
	}
}

// requiredManifest lists every file the check reads that eval/SHA256SUMS does
// not already pin.
var requiredManifest = []string{
	sumsFile, adjustFile,
	finalRows, finalScope, devRows, devScope,
	runlogFile, calibFile, sliceFile, postRows, calMapFile,
	repRun1, repRun2, repRun3,
	finalRep1, finalRep2, final2Rep1, final2Rep2,
	textileSplits, pharmaSplits,
	textileRows, pharmaRows, usmcaRows, final2Rows, textile2Rows, pharma2Rows, usmca2Rows,
	"backend/data/labels.jsonl", "backend/data/labels_usmca.jsonl",
	factsNamesPath,
	routedGeneral, routedUSMCAv2, routedUSMCAv4,
	"eval/results/route-dev-router-v2-gemini-3.8-flash-litellm.jsonl",
	"eval/results/route-dev-router-v3-gemini-3.8-flash-litellm.jsonl",
	"eval/results/route-dev-router-v4-gemini-3.8-flash-litellm.jsonl",
	"eval/results/route-final-router-v2-gemini-3.8-flash-litellm.jsonl",
	"eval/results/route-final-router-v4-gemini-3.8-flash-litellm.jsonl",
	"eval/results/route-final2-router-v2-d211.jsonl",
	"eval/results/route-final2-router-v4-gemini-3.8-flash-litellm.jsonl",
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
	"cal.version":                          "calibration map version (backend/internal/calibrate/map.json, D160)",
	"cal.bar":                              "auto-accept bar on calibrated probability",
	"final.cal.fast.n":                     "auto-accepted at the calibrated bar and in scope",
	"final.cal.fast.correct":               "of those, correct",
	"final.cal.fast.accuracy_pct":          "accuracy of auto-accepted, percent",
	"final.cal.mean_stated_pct":            "mean stated confidence, percent",
	"final.cal.mean_calibrated_pct":        "mean calibrated probability, percent",
	"final.cal.ece_stated":                 "ECE of stated confidence, one bin per value",
	"final.cal.ece_calibrated":             "ECE of calibrated probability, one bin per value",
	"labels.rows":                          "labelled corpus rows",
	"pool.union":                           "labelled + Part 102 ruling ids",
	"facts.files":                          "fact files in backend/data/facts",
	"usmca.not_in_labels":                  "Part 102 rulings absent from the labelled corpus",
	"runlog.final.time":                    "run-of-record timestamp in RUNLOG",
	"temporal.final_starts_after_dev_ends": "held-out window starts after the dev window ends",
	"planner_post.pairs":                   "frozen post-cutoff flip pairs (D170)",
	"planner_post.real_changes":            "pairs where CBP's two holdings differ",
	"planner_post.controls":                "pairs where work moved and the origin did not",
	"planner_post.core_pairs":              "pairs tiered core",
	"planner_post.known_a.error_rows":      "precedent-check rows with an error",
	"planner_post.known_a.shown":           "moves the check supported at the 0.92 plan bar",
	"planner_post.known_a.shown_right":     "of those, real changes with CBP's later origin named",
	"planner_post.known_a.controls_shown":  "no-change pairs supported at the bar",
	"planner_post.known_a.recall_pct":      "real changes shown and right, percent",
}

// RenderTable formats figures as the document's key/value rows.
func RenderTable(kv []KV) string {
	var b strings.Builder
	for _, e := range kv {
		fmt.Fprintf(&b, "| `%s` | %s | %s |\n", e.Key, e.Value, meanings[e.Key])
	}
	return b.String()
}

// plannerPost recomputes the post-cutoff flip-pair figures (D170) from the
// frozen pairs and the known-a precedent-check rows. A move is shown when
// the check is precedent-supported at confidence >= the plan bar; it is
// right when the pair is a real origin change and the check named CBP's
// later origin.
func plannerPost(out *kvs, probs *[]string, root string) {
	type pair struct {
		ID            string `json:"id"`
		Date          string `json:"date"`
		Tier          string `json:"tier"`
		OriginChanged bool   `json:"origin_changed"`
	}
	type row struct {
		ID         string  `json:"id"`
		Supported  bool    `json:"precedent_supported"`
		Confidence float64 `json:"confidence"`
		OriginBOK  bool    `json:"origin_b_ok"`
		Error      string  `json:"error"`
	}
	pairs, err := readJSONL[pair](root, postPairs)
	if err != nil {
		*probs = append(*probs, err.Error())
		return
	}
	rows, err := readJSONL[row](root, postRows)
	if err != nil {
		*probs = append(*probs, err.Error())
		return
	}
	byID := map[string]pair{}
	var real, controls, core, early int
	for _, p := range pairs {
		byID[p.ID] = p
		if p.OriginChanged {
			real++
		} else {
			controls++
		}
		if p.Tier == "core" {
			core++
		}
		if p.Date < "2026-09-26" {
			early++
		}
	}
	if early > 0 {
		*probs = append(*probs, fmt.Sprintf("%s: %d pairs have no ruling dated after %s", postPairs, early, cutoff))
	}
	if len(rows) != len(pairs) {
		*probs = append(*probs, fmt.Sprintf("%s: %d rows for %d frozen pairs", postRows, len(rows), len(pairs)))
	}
	var shown, right, ctlShown, errs int
	for _, r := range rows {
		p, ok := byID[r.ID]
		if !ok {
			*probs = append(*probs, fmt.Sprintf("%s: row %s is not a frozen pair", postRows, r.ID))
			continue
		}
		if r.Error != "" {
			errs++
			continue
		}
		if r.Supported && r.Confidence >= planBar {
			shown++
			if p.OriginChanged && r.OriginBOK {
				right++
			}
			if !p.OriginChanged {
				ctlShown++
			}
		}
	}
	out.add("planner_post.pairs", len(pairs))
	out.add("planner_post.real_changes", real)
	out.add("planner_post.controls", controls)
	out.add("planner_post.core_pairs", core)
	out.add("planner_post.known_a.error_rows", errs)
	out.add("planner_post.known_a.shown", shown)
	out.add("planner_post.known_a.shown_right", right)
	out.add("planner_post.known_a.controls_shown", ctlShown)
	out.add("planner_post.known_a.recall_pct", pct(right, real))
}

// repeatability recomputes the D190 figures: three development runs of the
// same engine configuration, scored with the D23 adjustments, the fast path
// on the fixed router-v2 scope file. A resumed run can hold a ruling twice;
// the last row counts, as in the eval command.
func repeatability(out *kvs, probs *[]string, root string, adj adjustments) {
	scope, err := readJSONL[scopeRow](root, devScope)
	if err != nil {
		*probs = append(*probs, err.Error())
		return
	}
	in := map[string]bool{}
	for _, s := range scope {
		in[s.Ruling] = s.InScope
	}
	type state struct {
		pred     string
		ok, fast bool
	}
	var runs []map[string]state
	for _, rel := range []string{repRun1, repRun2, repRun3} {
		rows, err := readJSONL[engineRow](root, rel)
		if err != nil {
			*probs = append(*probs, err.Error())
			return
		}
		m := map[string]state{}
		for _, r := range rows {
			if _, ex := adj.Exclude[r.Ruling]; ex {
				continue
			}
			m[r.Ruling] = state{r.Pred, adj.rowOK(r), r.Error == "" && r.Confidence >= fastPathBar && in[r.Ruling]}
		}
		runs = append(runs, m)
	}
	n := len(runs[0])
	minC, maxC := n, 0
	for i, m := range runs {
		if len(m) != n {
			*probs = append(*probs, fmt.Sprintf("repeatability: run %d has %d scored rulings, run 1 has %d", i+1, len(m), n))
			return
		}
		c := 0
		for _, s := range m {
			if s.ok {
				c++
			}
		}
		minC, maxC = min(minC, c), max(maxC, c)
		out.add(fmt.Sprintf("repeat.run%d.correct", i+1), c)
	}
	var answer, correct, fast int
	for id, s := range runs[0] {
		a, c, f := false, false, false
		for _, m := range runs[1:] {
			t, ok := m[id]
			if !ok {
				*probs = append(*probs, "repeatability: "+id+" missing from a repeat run")
				return
			}
			a = a || t.pred != s.pred
			c = c || t.ok != s.ok
			f = f || t.fast != s.fast
		}
		if a {
			answer++
		}
		if c {
			correct++
		}
		if f {
			fast++
		}
	}
	out.add("repeat.n", n)
	out.add("repeat.accuracy_min_pct", pct(minC, n))
	out.add("repeat.accuracy_max_pct", pct(maxC, n))
	out.add("repeat.answer_flips", answer)
	out.add("repeat.correctness_flips", correct)
	out.add("repeat.fast_path_flips", fast)
}

// allRegimes recomputes the held-out accuracy of every regime in both
// windows and the pooled figures (D210). Each result file must score exactly
// its split, the last row per ruling counting. "sum" adds the splits as they
// are; "dedup" counts a ruling in both the textile and the Part 102 split
// once, under Part 102.
func allRegimes(out *kvs, probs *[]string, root string) {
	type run struct{ key, rows, splits, list string }
	windows := [][]run{
		{{"w1.st", finalRows, splitsFile, "final"}, {"w1.textile", textileRows, textileSplits, "final"},
			{"w1.pharma", pharmaRows, pharmaSplits, "final"}, {"w1.part102", usmcaRows, usmcaSplits, "final"}},
		{{"w2.st", final2Rows, final2Splits, "final2"}, {"w2.textile", textile2Rows, textile2Splits, "final2"},
			{"w2.pharma", pharma2Rows, pharma2Splits, "final2"}, {"w2.part102", usmca2Rows, usmca2Splits, "final2"}},
	}
	var sumN, sumK, dedN, dedK int
	for wi, runs := range windows {
		right := map[string]map[string]bool{}
		var wn, wk int
		for _, r := range runs {
			var sp map[string]json.RawMessage
			var want []string
			if err := readJSON(root, r.splits, &sp); err != nil {
				*probs = append(*probs, err.Error())
				return
			}
			if err := json.Unmarshal(sp[r.list], &want); err != nil || len(want) == 0 {
				*probs = append(*probs, fmt.Sprintf("%s: no %q list", r.splits, r.list))
				return
			}
			rows, err := readJSONL[engineRow](root, r.rows)
			if err != nil {
				*probs = append(*probs, err.Error())
				return
			}
			got := map[string]bool{}
			for _, row := range rows {
				got[row.Ruling] = row.Correct && row.Error == ""
			}
			if len(got) != len(want) {
				*probs = append(*probs, fmt.Sprintf("%s: %d rulings, but %s %s lists %d", r.rows, len(got), r.splits, r.list, len(want)))
				return
			}
			k := 0
			for _, id := range want {
				ok, seen := got[id]
				if !seen {
					*probs = append(*probs, fmt.Sprintf("%s: %s of %s is not scored", r.rows, id, r.splits))
					return
				}
				if ok {
					k++
				}
			}
			right[r.key] = got
			out.add("regimes."+r.key+".n", len(want))
			out.add("regimes."+r.key+".correct", k)
			wn += len(want)
			wk += k
		}
		w := fmt.Sprintf("w%d.", wi+1)
		p := "regimes." + w
		dn, dk := wn, wk
		for id, ok := range right[w+"textile"] {
			if _, both := right[w+"part102"][id]; both {
				dn--
				if ok {
					dk--
				}
			}
		}
		out.add(p+"overlap", wn-dn)
		out.add(p+"sum.correct", wk)
		out.add(p+"sum.n", wn)
		out.add(p+"dedup.correct", dk)
		out.add(p+"dedup.n", dn)
		sumN, sumK, dedN, dedK = sumN+wn, sumK+wk, dedN+dn, dedK+dk
	}
	out.add("regimes.all.sum.correct", sumK)
	out.add("regimes.all.sum.n", sumN)
	out.add("regimes.all.sum.pct", pct(sumK, sumN))
	out.add("regimes.all.dedup.correct", dedK)
	out.add("regimes.all.dedup.n", dedN)
	out.add("regimes.all.dedup.pct", pct(dedK, dedN))
}

// Router files (D230-D232).
const (
	routerSplitsFile = "eval/splits/splits_router.json"
	routedGeneral    = "eval/results/final2-gemini-4-argon-prec-engine-v1-litellm-routed-v2.jsonl"
	routedUSMCAv2    = "eval/results/usmca-final2-gemini-4-argon-prec-usmca-v1-litellm-routed-v2.jsonl"
	routedUSMCAv4    = "eval/results/usmca-final2-gemini-4-argon-prec-usmca-v1-litellm-routed-v4.jsonl"
)

// routerRuns are the route files by split and router version (D230-D232).
// The router-v2 dev and v3 dev runs predate the four "none" dev rulings.
var routerRuns = []struct{ split, key, file string }{
	{"dev", "v2", "eval/results/route-dev-router-v2-gemini-3.8-flash-litellm.jsonl"},
	{"dev", "v3", "eval/results/route-dev-router-v3-gemini-3.8-flash-litellm.jsonl"},
	{"dev", "v4", "eval/results/route-dev-router-v4-gemini-3.8-flash-litellm.jsonl"},
	{"final", "v2", "eval/results/route-final-router-v2-gemini-3.8-flash-litellm.jsonl"},
	{"final", "v4", "eval/results/route-final-router-v4-gemini-3.8-flash-litellm.jsonl"},
	{"final2", "v2", "eval/results/route-final2-router-v2-d211.jsonl"},
	{"final2", "v4", "eval/results/route-final2-router-v4-gemini-3.8-flash-litellm.jsonl"},
}

type routeCheckRow struct {
	Ruling     string `json:"ruling"`
	GoldEngine string `json:"gold_engine"`
	Engine     string `json:"engine"`
	Error      string `json:"error"`
}

// routerChecks recomputes the router's routing per gold engine on each
// router split, with the gold engine taken from the frozen router splits
// (not the row), and the routed end-to-end path of router-v2 and router-v4
// on the post-cutoff rulings: each ruling scored with the engine its label
// picks. An errored route counts as wrong.
func routerChecks(out *kvs, probs *[]string, root string) {
	var sp struct {
		Dev    []string          `json:"dev"`
		Final  []string          `json:"final"`
		Final2 []string          `json:"final2"`
		Gold   map[string]string `json:"gold"`
	}
	if err := readJSON(root, routerSplitsFile, &sp); err != nil {
		*probs = append(*probs, err.Error())
		return
	}
	lastRoute := func(rel string) (map[string]routeCheckRow, bool) {
		rows, err := readJSONL[routeCheckRow](root, rel)
		if err != nil {
			*probs = append(*probs, err.Error())
			return nil, false
		}
		m := map[string]routeCheckRow{}
		for _, r := range rows {
			m[r.Ruling] = r
		}
		return m, true
	}
	ids := map[string][]string{"dev": sp.Dev, "final": sp.Final, "final2": sp.Final2}
	routes := map[string]map[string]routeCheckRow{}
	for _, f := range routerRuns {
		m, ok := lastRoute(f.file)
		if !ok {
			return
		}
		routes[f.split+"."+f.key] = m
		k, n := map[string]int{}, map[string]int{}
		for _, id := range ids[f.split] {
			g := sp.Gold[id]
			r, seen := m[id]
			if !seen {
				if g == "none" && f.split == "dev" {
					continue // the first dev runs predate the "none" rulings (D230)
				}
				*probs = append(*probs, fmt.Sprintf("%s: %s of the %s router split is not routed", f.file, id, f.split))
				return
			}
			if g == "none" {
				continue
			}
			right := r.Error == "" && r.Engine == g
			keys := []string{"all", g}
			if g != "general" {
				keys = append(keys, "specialist")
			}
			for _, key := range keys {
				n[key]++
				if right {
					k[key]++
				}
			}
		}
		for _, key := range []string{"all", "specialist", "general", "usmca_part102"} {
			p := "router." + f.split + "." + f.key + "." + key
			out.add(p+".correct", k[key])
			out.add(p+".n", n[key])
		}
		out.add("router."+f.split+"."+f.key+".all.pct", pct(k["all"], n["all"]))
	}
	// Routed path on the post-cutoff rulings.
	pred := map[string]map[string]bool{}
	for eng, files := range map[string][]string{
		"general":        {final2Rows, routedGeneral},
		"textile_102_21": {textile2Rows},
		"pharma_api":     {pharma2Rows},
		"usmca_part102":  {usmca2Rows, routedUSMCAv2, routedUSMCAv4},
	} {
		pred[eng] = map[string]bool{}
		for _, rel := range files {
			rows, err := readJSONL[engineRow](root, rel)
			if err != nil {
				*probs = append(*probs, err.Error())
				return
			}
			for _, r := range rows {
				pred[eng][r.Ruling] = r.Correct && r.Error == ""
			}
		}
	}
	for _, v := range []string{"v2", "v4"} {
		m := routes["final2."+v]
		var k, n, sent int
		for _, id := range sp.Final2 {
			r := m[id]
			right, scored := pred[r.Engine][id]
			if !scored {
				*probs = append(*probs, fmt.Sprintf("routed path %s: no %s prediction for %s", v, r.Engine, id))
				return
			}
			n++
			if right {
				k++
			}
			if sp.Gold[id] != r.Engine {
				sent++
			}
		}
		p := "router.w2.routed." + v
		out.add(p+".correct", k)
		out.add(p+".n", n)
		out.add(p+".pct", pct(k, n))
		out.add(p+".sent_elsewhere", sent)
	}
}

// heldoutRepeat recomputes the D241 figures for one held-out split: the
// official run and two fresh repeats of the same frozen engine, scored with
// the D23 adjustments. A resumed run can hold a ruling twice; the last row
// counts. Flips are counted across all three runs.
func heldoutRepeat(out *kvs, probs *[]string, root string, adj adjustments, prefix string, files ...string) {
	type state struct {
		pred string
		ok   bool
	}
	var runs []map[string]state
	for _, rel := range files {
		rows, err := readJSONL[engineRow](root, rel)
		if err != nil {
			*probs = append(*probs, err.Error())
			return
		}
		m := map[string]state{}
		for _, r := range rows {
			if _, ex := adj.Exclude[r.Ruling]; ex {
				continue
			}
			m[r.Ruling] = state{r.Pred, adj.rowOK(r)}
		}
		runs = append(runs, m)
	}
	n := len(runs[0])
	names := []string{"official", "repeat1", "repeat2"}
	sum := 0
	for i, m := range runs {
		if len(m) != n {
			*probs = append(*probs, fmt.Sprintf("%s: %s has %d scored rulings, the official run %d", prefix, names[i], len(m), n))
			return
		}
		c := 0
		for _, s := range m {
			if s.ok {
				c++
			}
		}
		if i > 0 {
			sum += c
		}
		out.add(prefix+"."+names[i]+".correct", c)
		out.add(prefix+"."+names[i]+".accuracy_pct", pct(c, n))
	}
	var answer, correct int
	for id, s := range runs[0] {
		a, c := false, false
		for _, m := range runs[1:] {
			t, ok := m[id]
			if !ok {
				*probs = append(*probs, prefix+": "+id+" missing from a repeat run")
				return
			}
			a = a || t.pred != s.pred
			c = c || t.ok != s.ok
		}
		if a {
			answer++
		}
		if c {
			correct++
		}
	}
	out.add(prefix+".n", n)
	// Mean of the two repeats, percent: (c1 + c2) / 2n.
	out.add(prefix+".repeat_mean_pct", fmt.Sprintf("%.2f", math.Round(10000*float64(sum)/float64(2*n))/100))
	out.add(prefix+".answer_flips", answer)
	out.add(prefix+".correctness_flips", correct)
}
