package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// alertsAsOf is the fixed "today" of the committed count, so the file does
// not change with the wall clock. It only decides which final changes are
// in effect and which are scheduled.
const alertsAsOf = "2026-11-22"

// heatgunBound is the heat-gun demo's evidence bound, as in
// eval/supersession/impact.py: its plan, ask and seed files carry no per-row
// date.
const heatgunBound = "2026-01-14"

var alertFlipData = []string{"eval/data/flips.jsonl", "eval/data/flips_post.jsonl", "eval/data/flips_post_addendum.jsonl"}

// revocationAlerts counts, offline, how many committed demo cases and
// committed result rows would show a revocation alert today (D480), with the
// same rule the API uses (domain.RulingAlerts). No model call. With -check
// it fails when the committed file differs.
func revocationAlerts(args []string) error {
	fl := flag.NewFlagSet("alerts", flag.ExitOnError)
	root := fl.String("root", "..", "repository root")
	out := fl.String("out", "eval/supersession/alerts.json", "output, relative to -root")
	check := fl.Bool("check", false, "fail when the committed file is stale")
	_ = fl.Parse(args)
	asOf, _ := time.Parse(time.DateOnly, alertsAsOf)

	bounds, err := alertFlipBounds(*root)
	if err != nil {
		return err
	}
	files, err := filepath.Glob(filepath.Join(*root, "eval", "results", "*.json"))
	if err != nil {
		return err
	}
	more, _ := filepath.Glob(filepath.Join(*root, "eval", "results", "*.jsonl"))
	files = append(files, more...)
	sort.Strings(files)

	rowTotals := map[string]int{}
	caseTotals := map[string]int{}
	rowHits, caseHits := []map[string]any{}, []map[string]any{}
	caseDocs := []string{}
	rulingsFinal, rulingsProposed := map[string]int{}, map[string]int{}
	relevanceFinal := map[string]int{} // relied-on final changes by D531 relevance, rows and cases
	for _, path := range files {
		rel, _ := filepath.Rel(*root, path)
		rel = filepath.ToSlash(rel)
		if rel == "eval/results/supersession-impact.json" {
			continue // a derived count, not results
		}
		docs, err := alertDocs(path)
		if err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
		jsonl := strings.HasSuffix(path, ".jsonl")
		for _, d := range docs {
			target := "" // the plan search's target, for its plans' supporting rulings (D561)
			alertWalk(d, func(m map[string]any) bool {
				if _, ok := m["plans"].([]any); ok && str(m["target"]) != "" {
					target = str(m["target"])
				}
				if isCaseDoc(m) {
					b, _ := json.Marshal(m)
					var c store.Case
					if err := json.Unmarshal(b, &c); err != nil {
						return false
					}
					cd, _ := time.Parse(time.DateOnly, c.Date)
					as := domain.WithRelevanceBy(domain.RulingAlerts(cd, asOf, c.RulingRefs()), c.RelevanceBasis())
					tallyRelevance(as, relevanceFinal)
					cat := alertCategory(as, true)
					caseTotals["cases"]++
					caseDocs = append(caseDocs, rel+" "+c.ID)
					caseTotals[cat]++
					if cat != "none" {
						caseHits = append(caseHits, map[string]any{"file": rel, "case": c.ID, "case_date": c.Date, "category": cat, "alerts": alertWords(as)})
					}
					return false // its prediction and plan rows are counted here, not again below
				}
				shown := strList(m["precedents"])
				cited := append(strList(m["cited"]), strList(m["cited_rulings"])...)
				support := strList(m["supporting_ruling"])
				counter := strList(m["counter_ruling"])
				if len(shown)+len(cited)+len(support)+len(counter) == 0 {
					return !jsonl
				}
				date := str(m["date"])
				if date == "" {
					date = str(m["precedents_before"])
				}
				if date == "" {
					date = bounds[str(m["id"])]
				}
				if date == "" && strings.Contains(filepath.Base(path), "heatgun") {
					date = heatgunBound
				}
				var refs []domain.RulingRef
				if self := str(m["ruling"]); self != "" {
					// The row scores the engine against this CBP ruling: a
					// change it made is its own act, not an alert (D490).
					refs = append(refs, domain.RulingRef{ID: self, Role: domain.RoleSelf})
				}
				for _, id := range cited {
					refs = append(refs, domain.RulingRef{ID: id, Role: domain.RoleCited})
				}
				for _, id := range support {
					refs = append(refs, domain.RulingRef{ID: id, Role: domain.RolePlanSupport})
				}
				for _, id := range append(counter, shown...) {
					refs = append(refs, domain.RulingRef{ID: id, Role: domain.RoleRead})
				}
				cd, _ := time.Parse(time.DateOnly, date)
				answer := str(m["pred"])
				if answer == "" {
					answer = str(m["predicted_origin"])
				}
				basis := domain.RelevanceBasis{Predicted: answer}
				if len(support) > 0 && target != "" {
					basis.PlanTargets = map[string][]string{}
					for _, id := range support {
						basis.PlanTargets[id] = append(basis.PlanTargets[id], target)
					}
				}
				as := domain.WithRelevanceBy(domain.RulingAlerts(cd, asOf, refs), basis)
				tallyRelevance(as, relevanceFinal)
				cat := alertCategory(as, date != "")
				rowTotals["rows_with_rulings"]++
				if date == "" {
					rowTotals["rows_undated"]++
				}
				rowTotals[cat]++
				for _, a := range as {
					if !a.Relied() {
						continue
					}
					if a.Status == domain.AlertFinal {
						rulingsFinal[a.Ruling]++
					} else {
						rulingsProposed[a.Ruling]++
					}
				}
				if strings.HasPrefix(cat, "final") || cat == "proposed" {
					row := str(m["ruling"])
					if row == "" {
						row = str(m["id"])
					}
					rowHits = append(rowHits, map[string]any{"file": rel, "row": row, "case_date": date, "category": cat, "alerts": alertWords(as)})
				}
				return !jsonl // a .jsonl file is one row per line, as impact.py reads it
			})
		}
	}
	delete(rowTotals, "none")
	delete(caseTotals, "none")

	demos, err := demoAlerts(*root, asOf)
	if err != nil {
		return err
	}
	sort.SliceStable(rowHits, func(i, j int) bool {
		return rowHits[i]["file"].(string)+"\x00"+rowHits[i]["row"].(string) < rowHits[j]["file"].(string)+"\x00"+rowHits[j]["row"].(string)
	})
	doc := map[string]any{
		"note": "Offline count (D480): committed demo cases and committed result rows that would show a revocation alert, by the API's rule (domain.RulingAlerts): a ruling the case or row relies on (cited, a plan's supporting ruling, CBP's holding on the current setup) has a final revocation or modification (final_after_case: dated on or after the case date; final_before_case: dated before it, which the guard of D320/D370 would refuse today; final_undated: the row has no date) or a pending proposal only (proposed). read_only_final: only a ruling read and not cited changed (a calm line, no badge). relevance_final_relied counts the relied-on final changes by whether the change bears on what the row or case relies on the ruling for (domain.WithRelevanceBy, D531, D561: a cited ruling against the predicted origin, a plan's supporting ruling against the plan search's target, the current setup's ruling against CBP's holding there; touches_answer, not_answer, holding_unchanged, unknown). " +
			"Rows are every JSON object in eval/results that names rulings, as eval/supersession/impact.py reads them, except stored case documents (an id case_..., a date and a prediction), which are counted once as cases. No model call; in effect is judged as of " + alertsAsOf + ".",
		"as_of":                   alertsAsOf,
		"map":                     "backend/internal/domain/supersession.json",
		"rows":                    rowTotals,
		"row_examples":            rowHits,
		"cases":                   caseTotals,
		"case_examples":           caseHits,
		"case_documents":          caseDocs,
		"demo_fixtures":           demos,
		"rulings_final_relied":    rulingsFinal,
		"rulings_proposed_relied": rulingsProposed,
		"relevance_final_relied":  relevanceFinal,
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", " ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(doc); err != nil {
		return err
	}
	dest := filepath.Join(*root, *out)
	if *check {
		old, err := os.ReadFile(dest)
		if err != nil || !bytes.Equal(old, buf.Bytes()) {
			return fmt.Errorf("%s is stale: run go run ./cmd/corpus alerts", *out)
		}
		fmt.Printf("%s matches (rows: %v; cases: %v)\n", *out, rowTotals, caseTotals)
		return nil
	}
	if err := os.WriteFile(dest, buf.Bytes(), 0o644); err != nil {
		return err
	}
	fmt.Printf("rows: %v\ncases: %v\n", rowTotals, caseTotals)
	return nil
}

// alertCategory is the strongest alert on a case or row.
func alertCategory(as []domain.RulingAlert, dated bool) string {
	cat := "none"
	rank := map[string]int{"none": 0, "read_only_final": 1, "proposed": 2, "final_undated": 3, "final_before_case": 3, "final_after_case": 4}
	for _, a := range as {
		c := "read_only_final"
		switch {
		case !a.Relied():
			if a.Status != domain.AlertFinal {
				continue
			}
		case a.Status == domain.AlertProposed:
			c = "proposed"
		case !dated:
			c = "final_undated"
		case a.BeforeCase:
			c = "final_before_case"
		default:
			c = "final_after_case"
		}
		if rank[c] > rank[cat] {
			cat = c
		}
	}
	return cat
}

// tallyRelevance counts the relied-on final changes by relevance (D531).
func tallyRelevance(as []domain.RulingAlert, n map[string]int) {
	for _, a := range as {
		if a.Relied() && a.Status == domain.AlertFinal {
			n[a.Relevance]++
		}
	}
}

func alertWords(as []domain.RulingAlert) []string {
	out := []string{}
	for _, a := range as {
		s := a.Ruling + " (" + strings.Join(a.Roles, ", ") + "): "
		if a.Status == domain.AlertProposed {
			s += "proposed: " + a.Note
		} else {
			s += a.Note
			if !a.InEffect {
				s += " (scheduled)"
			}
		}
		if a.Relevance != "" {
			s += " [" + a.Relevance + "]"
		}
		out = append(out, s)
	}
	return out
}

// demoAlerts checks each demo fixture: its date, CBP's holding on the
// current setup it gives, and the recorded run's supporting ruling.
func demoAlerts(root string, asOf time.Time) ([]map[string]any, error) {
	paths, err := filepath.Glob(filepath.Join(root, "backend", "internal", "demos", "fixtures", "*.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	out := []map[string]any{}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		var fx struct {
			ID      string `json:"id"`
			Predict struct {
				Date          string `json:"date"`
				CurrentOrigin *struct {
					Ruling string `json:"ruling"`
				} `json:"current_origin"`
			} `json:"predict"`
			Recorded *struct {
				PlanRuling string `json:"plan_ruling"`
			} `json:"recorded"`
		}
		if err := json.Unmarshal(b, &fx); err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		var refs []domain.RulingRef
		if fx.Predict.CurrentOrigin != nil && fx.Predict.CurrentOrigin.Ruling != "" {
			refs = append(refs, domain.RulingRef{ID: fx.Predict.CurrentOrigin.Ruling, Role: domain.RoleCurrentSetup})
		}
		if fx.Recorded != nil && fx.Recorded.PlanRuling != "" {
			refs = append(refs, domain.RulingRef{ID: fx.Recorded.PlanRuling, Role: domain.RolePlanSupport})
		}
		d, _ := time.Parse(time.DateOnly, fx.Predict.Date)
		as := domain.RulingAlerts(d, asOf, refs)
		ids := []string{}
		for _, r := range refs {
			ids = append(ids, r.ID)
		}
		out = append(out, map[string]any{"demo": fx.ID, "date": fx.Predict.Date, "rulings_checked": ids, "category": alertCategory(as, true), "alerts": alertWords(as)})
	}
	return out, nil
}

func alertFlipBounds(root string) (map[string]string, error) {
	out := map[string]string{}
	for _, p := range alertFlipData {
		f, err := os.Open(filepath.Join(root, p))
		if err != nil {
			return nil, err
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<26)
		for sc.Scan() {
			var r struct {
				ID       string         `json:"id"`
				Date     string         `json:"date"`
				VariantA map[string]any `json:"variant_a"`
				VariantB map[string]any `json:"variant_b"`
			}
			if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
				f.Close()
				return nil, fmt.Errorf("%s: %w", p, err)
			}
			min := r.Date
			for _, v := range []map[string]any{r.VariantA, r.VariantB} {
				if d := str(v["date"]); d != "" && (min == "" || d < min) {
					min = d
				}
			}
			out[r.ID] = min
		}
		f.Close()
		if err := sc.Err(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// alertDocs reads a .json file as one document or a .jsonl file as one per line.
func alertDocs(path string) ([]any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if !strings.HasSuffix(path, ".jsonl") {
		var d any
		if err := json.Unmarshal(b, &d); err != nil {
			return nil, err
		}
		return []any{d}, nil
	}
	var out []any
	for _, line := range bytes.Split(b, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var d any
		if err := json.Unmarshal(line, &d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

// alertWalk visits every object; visit returns false to skip its children.
func alertWalk(v any, visit func(map[string]any) bool) {
	switch x := v.(type) {
	case map[string]any:
		if !visit(x) {
			return
		}
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			alertWalk(x[k], visit)
		}
	case []any:
		for _, y := range x {
			alertWalk(y, visit)
		}
	}
}

func isCaseDoc(m map[string]any) bool {
	id, _ := m["id"].(string)
	_, hasPred := m["prediction"].(map[string]any)
	return strings.HasPrefix(id, "case_") && hasPred && str(m["date"]) != ""
}

func strList(v any) []string {
	switch x := v.(type) {
	case string:
		if x != "" {
			return []string{x}
		}
	case []any:
		var out []string
		for _, y := range x {
			if s, ok := y.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func str(v any) string { s, _ := v.(string); return s }
