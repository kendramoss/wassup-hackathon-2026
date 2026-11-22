package hts

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// D471: the plausibility check measured offline on the committed per-part
// suggestion rows of D381/D382 (no model call, no network). The result is
// eval/parts-check/summary.json; this test recomputes it and fails when it
// differs. PARTS_CHECK_UPDATE=1 writes it.

var checkRuns = []struct{ Name, Rows, Gold string }{
	{"dev", "suggest-dev-v1.jsonl", "gold-dev.jsonl"},
	// Held out (D471): measured once, after the check was frozen at
	// commit bda20b64.
	{"final", "suggest-final.jsonl", "gold-final.jsonl"},
}

const repoRoot = "../../.."

type checkCell struct {
	Products        int `json:"products"`
	Parts           int `json:"parts"`
	Candidates      int `json:"candidates"`
	Flagged         int `json:"flagged"`
	FlaggedFamily   int `json:"flagged_far_from_family"`
	FlaggedChapter  int `json:"flagged_far_from_product_chapter"`
	FlaggedMaterial int `json:"flagged_material"`
	PartsAllFlagged int `json:"parts_with_every_code_flagged"`
	// On parts with a gold code (D381's rule): every candidate is right or
	// wrong against it.
	GoldParts       int `json:"gold_parts"`
	GoldCandidates  int `json:"gold_part_candidates"`
	GoldWrong       int `json:"gold_part_candidates_wrong"`
	GoldFlagged     int `json:"gold_part_candidates_flagged"`
	FlaggedWrong    int `json:"flagged_and_wrong"`
	FlaggedRight    int `json:"flagged_but_right"`
	UnflaggedWrong  int `json:"wrong_not_flagged"`
	GoldTop1Flagged int `json:"gold_part_top1_flagged"`
}

type checkExample struct {
	Ruling string `json:"ruling"`
	Set    string `json:"set"`
	Part   string `json:"part"`
	HTS    string `json:"hts"`
	Gold   string `json:"gold,omitempty"`
	Note   string `json:"note"`
}

type checkSummary struct {
	Source   string                          `json:"_source"`
	Cells    map[string]map[string]checkCell `json:"cells"` // run -> set -> cell
	Flagged  map[string][]checkExample       `json:"flagged"`
	Families map[string]string               `json:"families"` // family-fixture ruling -> family
}

func readJSONL(t *testing.T, path string, each func([]byte)) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 0, 1<<20), 1<<24)
	for sc.Scan() {
		if l := bytes.TrimSpace(sc.Bytes()); len(l) > 0 {
			each(l)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
}

func fixtureFamilies(t *testing.T) map[string]string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(repoRoot, "backend", "testdata", "planner_families", "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("family fixtures: %v", err)
	}
	out := map[string]string{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var fx struct {
			Family string `json:"family"`
		}
		if err := json.Unmarshal(b, &fx); err != nil {
			t.Fatal(err)
		}
		out[strings.ToUpper(strings.TrimSuffix(filepath.Base(f), ".json"))] = fx.Family
	}
	return out
}

// goldMatch is D381's rule: the suggestion equals the gold at the gold's
// level, up to 8 digits; a shorter suggestion never matches.
func goldMatch(sugg, gold string) bool {
	d := func(s string) string {
		return strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, s)
	}
	s, g := d(sugg), d(gold)
	if len(g) > 8 {
		g = g[:8]
	}
	return len(g) >= 4 && len(s) >= len(g) && s[:len(g)] == g
}

func computeCheckSummary(t *testing.T) checkSummary {
	t.Helper()
	fams := fixtureFamilies(t)
	out := checkSummary{
		Source: "D471: hts.PlausibilityNote on the committed per-part suggestion rows (eval/tariffshift/parts, D381/D382), gold by D381's rule. " +
			"Family rows take the fixture's family; Part 102 rows have none and use the good's top suggestion as the product's chapter. " +
			"Recomputed by backend/internal/hts/plausible_eval_test.go (go test).",
		Cells: map[string]map[string]checkCell{}, Flagged: map[string][]checkExample{}, Families: fams,
	}
	dir := filepath.Join(repoRoot, "eval", "tariffshift", "parts")
	for _, run := range checkRuns {
		gold := map[string]map[int]string{}
		readJSONL(t, filepath.Join(dir, run.Gold), func(l []byte) {
			var g struct {
				Ruling string `json:"ruling"`
				Gold   []struct {
					Index int    `json:"index"`
					HTS   string `json:"gold_hts"`
				} `json:"gold"`
			}
			if err := json.Unmarshal(l, &g); err != nil {
				t.Fatal(err)
			}
			for _, x := range g.Gold {
				if gold[g.Ruling] == nil {
					gold[g.Ruling] = map[int]string{}
				}
				gold[g.Ruling][x.Index] = x.HTS
			}
		})
		cells := map[string]checkCell{}
		readJSONL(t, filepath.Join(dir, run.Rows), func(l []byte) {
			var row struct {
				Ruling string           `json:"ruling"`
				Set    string           `json:"set"`
				Parts  []PartSuggestion `json:"parts"`
				Good   *GoodSuggestion  `json:"good"`
			}
			if err := json.Unmarshal(l, &row); err != nil {
				t.Fatal(err)
			}
			family := ""
			if row.Set == "family" {
				family = fams[row.Ruling]
				if family == "" {
					t.Fatalf("no fixture family for %s", row.Ruling)
				}
			}
			good := 0
			if row.Good != nil && len(row.Good.Candidates) > 0 {
				good = Chapter(row.Good.Candidates[0].HTS)
			}
			c := cells[row.Set]
			c.Products++
			for _, p := range row.Parts {
				c.Parts++
				g, hasGold := gold[row.Ruling][p.Index]
				if hasGold {
					c.GoldParts++
				}
				all := len(p.Candidates) > 0
				for k, cand := range p.Candidates {
					c.Candidates++
					note := PlausibilityNote(family, good, p.Name, cand.HTS)
					flagged := note != ""
					all = all && flagged
					if flagged {
						c.Flagged++
						switch {
						case strings.Contains(note, "product family"):
							c.FlaggedFamily++
						case strings.Contains(note, "product's chapter"):
							c.FlaggedChapter++
						default:
							c.FlaggedMaterial++
						}
						ex := checkExample{Ruling: row.Ruling, Set: row.Set, Part: p.Name, HTS: cand.HTS, Note: note}
						if hasGold {
							ex.Gold = g
						}
						out.Flagged[run.Name] = append(out.Flagged[run.Name], ex)
					}
					if !hasGold {
						continue
					}
					c.GoldCandidates++
					right := goldMatch(cand.HTS, g)
					switch {
					case !right:
						c.GoldWrong++
						if flagged {
							c.FlaggedWrong++
						} else {
							c.UnflaggedWrong++
						}
					case flagged:
						c.FlaggedRight++
					}
					if flagged {
						c.GoldFlagged++
						if k == 0 {
							c.GoldTop1Flagged++
						}
					}
				}
				if all {
					c.PartsAllFlagged++
				}
			}
			cells[row.Set] = c
		})
		out.Cells[run.Name] = cells
		sort.SliceStable(out.Flagged[run.Name], func(i, j int) bool {
			a, b := out.Flagged[run.Name][i], out.Flagged[run.Name][j]
			return a.Ruling+a.Part+a.HTS < b.Ruling+b.Part+b.HTS
		})
	}
	return out
}

func TestPartsCheckSummaryIsCurrent(t *testing.T) {
	got, err := json.MarshalIndent(computeCheckSummary(t), "", " ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	path := filepath.Join(repoRoot, "eval", "parts-check", "summary.json")
	if os.Getenv("PARTS_CHECK_UPDATE") == "1" {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (PARTS_CHECK_UPDATE=1 writes it)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s is stale: run PARTS_CHECK_UPDATE=1 go test ./internal/hts -run TestPartsCheckSummaryIsCurrent", path)
	}
}
