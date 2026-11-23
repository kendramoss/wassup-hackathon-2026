// Command rcepeval scores the deterministic Annex 3A calculator under D691.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jpcorpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/rcep"
	"golang.org/x/text/unicode/norm"
)

const corpusDir = "../data/jp"
const reportDir = "../eval/rcep"

type outcome struct {
	Status    string   `json:"status"`
	Criterion string   `json:"criterion,omitempty"`
	Rule      string   `json:"rule,omitempty"`
	Reasons   []string `json:"reasons,omitempty"`
}

type negative struct {
	MaterialIndex int     `json:"material_index"`
	ReplacementHS string  `json:"replacement_hs"`
	Result        outcome `json:"result"`
}

type row struct {
	ID             string     `json:"id"`
	Date           string     `json:"date"`
	URL            string     `json:"url"`
	Input          rcep.Input `json:"input"`
	Criterion      string     `json:"published_criterion,omitempty"`
	PublishedScope string     `json:"published_tariff_scope,omitempty"`
	PublishedRow   string     `json:"published_annex_row,omitempty"`
	Result         outcome    `json:"result"`
	Synthetic      *negative  `json:"synthetic_negative,omitempty"`
}

type ruling struct {
	ID        string           `json:"id"`
	URL       string           `json:"url"`
	Date      string           `json:"date"`
	HS        string           `json:"hs"`
	Agreement string           `json:"agreement"`
	Process   string           `json:"process_ja"`
	Inputs    []jpcorpus.Input `json:"inputs"`
	Reasoning string           `json:"reasoning_ja"`
}

var producers = map[string]string{
	"オーストラリア": "Australia", "豪州": "Australia", "ブルネイ": "Brunei",
	"カンボジア": "Cambodia", "中国": "China", "中華人民共和国": "China",
	"インドネシア": "Indonesia", "日本": "Japan", "韓国": "South Korea", "大韓民国": "South Korea",
	"ラオス": "Laos", "マレーシア": "Malaysia", "ミャンマー": "Myanmar",
	"ニュージーランド": "New Zealand", "NZ": "New Zealand", "フィリピン": "Philippines",
	"シンガポール": "Singapore", "タイ": "Thailand", "ベトナム": "Vietnam",
}

func madeIn(process string) string {
	prefix, _, found := strings.Cut(norm.NFKC.String(strings.TrimSpace(process)), "において")
	if !found {
		return ""
	}
	return producers[strings.TrimSuffix(strings.TrimSpace(prefix), "国内")]
}

var nonDigits = regexp.MustCompile(`[^0-9]`)
var explicitOrigin = regexp.MustCompile(`(?:^|[ (、,])(?:RCEP協定上の)?(?:[一-龯々ァ-ヶー]+)?原産(?:材料|品)?(?:[)、, ]|$)`)

func inputOf(r ruling) rcep.Input {
	in := rcep.Input{HS: r.HS, ProducedIn: madeIn(r.Process), Materials: make([]rcep.Material, 0, len(r.Inputs))}
	for _, m := range r.Inputs {
		mat := rcep.Material{Name: m.NameJA, HS: materialHS(m), Status: rcep.StatusUnknown}
		if len(m.Countries) == 1 {
			mat.Country = m.Countries[0]
		}
		// Require a supplier-style originating statement in this material's
		// entry. A manufacturing country alone never supplies such a claim.
		source := norm.NFKC.String(m.TextJA)
		if explicitOrigin.MatchString(source) && !strings.Contains(source, "非原産") {
			mat.Status = rcep.StatusOriginating
		}
		in.Materials = append(in.Materials, mat)
	}
	return in
}

func materialHS(m jpcorpus.Input) string {
	var codes []string
	for _, ch := range m.Chapters {
		codes = append(codes, fmt.Sprintf("%02d", ch))
	}
	for _, h := range append(slices.Clone(m.Headings), m.Subheadings...) {
		codes = append(codes, nonDigits.ReplaceAllString(h, ""))
	}
	var best string
	for _, code := range codes {
		if len(code) > len(best) {
			best = code
		}
	}
	for _, code := range codes {
		if !strings.HasPrefix(best, code) {
			return "" // conflicting classifications for one parsed material
		}
	}
	return best
}

var annex = regexp.MustCompile(`附属書\s*3\s*A`)
var scopeCode = regexp.MustCompile(`関税(?:率)?表第\s*([0-9]{2,4}(?:\.[0-9]{2})?)\s*(?:類|項|号)に分類される(?:産品|本品|品物)`)
var rowCode = regexp.MustCompile(`附属書\s*3\s*A\s*(?:の|に規定する)\s*第?\s*([0-9]{2,4}(?:\.[0-9]{2})?)\s*(?:類|項|号)`)

func publishedRule(reasoning string) (scope, exact string) {
	s := norm.NFKC.String(reasoning)
	if !annex.MatchString(s) {
		return "", ""
	}
	for re, dst := range map[*regexp.Regexp]*string{scopeCode: &scope, rowCode: &exact} {
		for _, match := range re.FindAllStringSubmatch(s, -1) {
			key := nonDigits.ReplaceAllString(match[1], "")
			if *dst != "" && *dst != key {
				*dst = ""
				break
			}
			*dst = key
		}
	}
	return scope, exact
}

func applied(result rcep.Result) string {
	if strings.HasPrefix(result.Applied, "RVC") {
		return "RVC"
	}
	a, _, _ := strings.Cut(result.Applied, " ")
	return a
}

func calculate(in rcep.Input) outcome {
	result, err := rcep.Check(in)
	if err != nil {
		return outcome{Status: "error", Reasons: []string{err.Error()}}
	}
	o := outcome{Status: result.Outcome, Criterion: applied(result), Reasons: result.Reasons}
	if result.Rule != nil {
		o.Rule = nonDigits.ReplaceAllString(result.Rule.Code, "")
	}
	return o
}

func shift(criterion string) bool {
	return criterion == "CC" || criterion == "CTH" || criterion == "CTSH"
}

func counterexample(in rcep.Input, original outcome) *negative {
	if original.Status != rcep.Yes || !shift(original.Criterion) {
		return nil
	}
	hs := nonDigits.ReplaceAllString(in.HS, "")
	if len(hs) < 4 {
		return nil
	}
	for i, m := range in.Materials {
		if m.Status == rcep.StatusOriginating && (m.Country == "" || rcep.Party(m.Country) != "") {
			continue
		}
		in.Materials = slices.Clone(in.Materials)
		in.Materials[i].HS = hs[:4]
		return &negative{MaterialIndex: i, ReplacementHS: hs[:4], Result: calculate(in)}
	}
	return nil
}

func readLines[T any](path string) ([]T, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 65536), 2<<20)
	var rows []T
	for s.Scan() {
		var r T
		if err := json.Unmarshal(s.Bytes(), &r); err != nil {
			return nil, fmt.Errorf("%s row %d: %w", path, len(rows)+1, err)
		}
		rows = append(rows, r)
	}
	return rows, s.Err()
}

func score(split, path string) error {
	if split == "heldout" {
		if _, err := os.Stat(filepath.Join(reportDir, "japan-dev.jsonl")); err != nil {
			return fmt.Errorf("development must be scored first: %w", err)
		}
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("a recording already exists or its path is unavailable: %s", path)
	}
	rs, err := readLines[ruling](filepath.Join(corpusDir, "rulings.jsonl"))
	if err != nil {
		return err
	}
	sp, err := jpcorpus.LoadSplits(filepath.Join(corpusDir, "splits.json"))
	if err != nil {
		return err
	}
	ids, labelPath := sp.Dev, "labels-dev.jsonl"
	if split == "heldout" {
		ids, labelPath = sp.HeldOut, "labels-heldout.jsonl"
	}
	labels, err := jpcorpus.LoadLabels(filepath.Join(corpusDir, labelPath))
	if err != nil {
		return err
	}
	byID := make(map[string]ruling, len(rs))
	for _, r := range rs {
		if _, exists := byID[r.ID]; exists {
			return fmt.Errorf("duplicate ruling %s", r.ID)
		}
		byID[r.ID] = r
	}
	var rows []row
	for _, id := range ids {
		r, ok := byID[id]
		if !ok {
			return fmt.Errorf("unknown ruling %s in frozen %s split", id, split)
		}
		if r.Agreement != "RCEP" {
			continue
		}
		l, ok := labels[id]
		if !ok || l.Agreement != "RCEP" || l.Outcome != "positive" {
			return fmt.Errorf("ruling %s has no matching positive RCEP label", id)
		}
		in := inputOf(r)
		o := calculate(in)
		scope, exact := publishedRule(r.Reasoning)
		rows = append(rows, row{ID: id, Date: r.Date, URL: r.URL, Input: in, Criterion: l.Criterion,
			PublishedScope: scope, PublishedRow: exact, Result: o, Synthetic: counterexample(in, o)})
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	for _, row := range rows {
		if err := enc.Encode(row); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(b.Bytes())
	return errors.Join(writeErr, f.Close())
}

func run() error {
	split := flag.String("split", "", "dev or heldout; produce one recording")
	check := flag.Bool("check", false, "check the summary from committed recordings without scoring")
	flag.Parse()
	if *split == "dev" || *split == "heldout" {
		if *check {
			return errors.New("choose either a scoring split or a summary check")
		}
		return score(*split, filepath.Join(reportDir, "japan-"+*split+".jsonl"))
	}
	if *split != "" {
		return fmt.Errorf("unknown split %q", *split)
	}
	return summarize(*check)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
