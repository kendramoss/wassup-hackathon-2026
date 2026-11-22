package main

// Per-part HTS suggestions, evaluated as pre-registered in D381:
//
//	go run ./cmd/tariffshift parts-gold -split dev -raw DIR      # regex gold codes from the full ruling texts (no model)
//	go run ./cmd/tariffshift parts-suggest -split dev            # run the suggester (gateway, costs money)
//	go run ./cmd/tariffshift parts-eval [-check]                 # score the committed rows (no model calls)
//
// Held-out (final) gold and suggestions run once each, with -confirm-heldout,
// after the suggester code is frozen.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/hts"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/retrieval"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/tariffshift"
)

// partsDir holds the committed per-part rows.
const partsDir = evalDir + "/parts"

// familyFixtures are the per-family plan fixtures (D171); each lists parts.
const familyFixtures = "testdata/planner_families"

// EvalProduct is one product the suggester is run on.
type EvalProduct struct {
	Ruling  string         `json:"ruling"`
	Set     string         `json:"set"` // "usmca" (Part 102 rulings) or "family" (per-family fixtures)
	Date    string         `json:"date"`
	Product domain.Product `json:"product"`
}

// GoldPart is one part whose code CBP states in the ruling's full text.
type GoldPart struct {
	Ruling    string `json:"ruling"`
	Set       string `json:"set"`
	Index     int    `json:"index"`
	Part      string `json:"part"`
	HTS       string `json:"gold_hts"`
	Statement string `json:"statement_part"`
	Sentence  string `json:"sentence"`
}

// GoldRow is the gold file's row for one product: its gold parts and the
// parts that matched conflicting codes.
type GoldRow struct {
	Ruling    string     `json:"ruling"`
	Set       string     `json:"set"`
	Parts     int        `json:"parts"`
	Gold      []GoldPart `json:"gold"`
	Ambiguous []string   `json:"ambiguous,omitempty"`
	Error     string     `json:"error,omitempty"`
}

// SuggestRow is one product's committed suggestion.
type SuggestRow struct {
	Ruling  string               `json:"ruling"`
	Set     string               `json:"set"`
	Split   string               `json:"split"`
	Parts   []hts.PartSuggestion `json:"parts"`
	Good    *hts.GoodSuggestion  `json:"good,omitempty"`
	Failed  bool                 `json:"reasons_unavailable,omitempty"`
	Error   string               `json:"error,omitempty"`
	Model   string               `json:"model"`
	Prompt  string               `json:"prompt"`
	InTok   int                  `json:"input_tokens"`
	OutTok  int                  `json:"output_tokens"`
	CostUSD float64              `json:"cost_usd"`
}

// evalProducts lists the products of a split: the Part 102 rulings with a
// full-text extraction (parts and origins from it; name from the subject;
// description from the facts the engine reads, HTS numbers removed) and,
// for dev, the per-family fixtures (all dev or train).
func evalProducts(split string) ([]EvalProduct, error) {
	return evalProductsWith(split, stripCodes, stripCodes)
}

// evalProductsWith is evalProducts with given code strippers for the name
// and the description; parts-eval rebuilds the inputs the committed runs saw
// (stripCodesV1 on the description, the name as it was) (#576).
func evalProductsWith(split string, stripName, strip func(string) string) ([]EvalProduct, error) {
	ids, err := splitIDs(split)
	if err != nil {
		return nil, err
	}
	labels, err := usmcaLabels()
	if err != nil {
		return nil, err
	}
	rows, err := readInputs(filepath.Join(evalDir, fmt.Sprintf("inputs-%s-full_text.jsonl", split)))
	if err != nil {
		return nil, err
	}
	byID := map[string]InputRow{}
	for _, r := range rows {
		byID[r.Ruling] = r
	}
	var out []EvalProduct
	for _, id := range ids {
		r, ok := byID[id]
		if !ok || r.Error != "" || len(r.Input.Materials) == 0 {
			continue
		}
		facts, err := os.ReadFile(filepath.Join("data/facts", id+".txt"))
		if err != nil {
			return nil, err
		}
		p := domain.Product{Name: stripName(productName(labels[id].Subject)), Description: strip(string(facts))}
		for _, m := range r.Input.Materials {
			p.Components = append(p.Components, domain.Component{Name: m.Name, MadeIn: m.Origin})
		}
		out = append(out, EvalProduct{Ruling: id, Set: "usmca", Date: labels[id].Date, Product: p})
	}
	if split != "dev" {
		return out, nil
	}
	files, _ := filepath.Glob(filepath.Join(familyFixtures, "*.json"))
	sort.Strings(files)
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var fx struct {
			Date    string         `json:"date"`
			Product domain.Product `json:"product"`
		}
		if err := json.Unmarshal(b, &fx); err != nil {
			return nil, err
		}
		id := strings.ToUpper(strings.TrimSuffix(filepath.Base(f), ".json"))
		p := domain.Product{Name: stripName(fx.Product.Name), Description: strip(fx.Product.Description)}
		for _, c := range fx.Product.Components {
			p.Components = append(p.Components, domain.Component{Name: c.Name, MadeIn: c.MadeIn})
		}
		out = append(out, EvalProduct{Ruling: id, Set: "family", Date: fx.Date, Product: p})
	}
	return out, nil
}

type usmcaLabel struct {
	Date, Subject string
	Origin        *string
}

func usmcaLabels() (map[string]usmcaLabel, error) {
	b, err := os.ReadFile("../eval/data/labels_usmca.jsonl")
	if err != nil {
		return nil, err
	}
	out := map[string]usmcaLabel{}
	for _, line := range bytes.Split(b, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var r struct {
			Ruling, Date, Subject string
			Origin                *string `json:"primary_origin"`
		}
		if err := json.Unmarshal(line, &r); err != nil {
			return nil, err
		}
		out[r.Ruling] = usmcaLabel{r.Date, r.Subject, r.Origin}
	}
	return out, nil
}

var subjectKeys = []string{"origin", "classification", "marking", "usmca)", "nafta)", "agreement", "eligibility", "purposes"}

// productName is what a subject line says the product is: the words after
// its last "country of origin" / "classification" / "marking" phrase.
func productName(subject string) string {
	l := strings.ToLower(subject)
	cut := -1
	for _, k := range subjectKeys {
		if i := strings.LastIndex(l, k); i >= 0 && i+len(k) > cut {
			cut = i + len(k)
		}
	}
	name := subject
	if cut >= 0 && cut < len(subject) {
		name = subject[cut:]
	}
	for {
		t := strings.TrimLeft(name, " ,;:")
		lt := strings.ToLower(t)
		for _, w := range []string{"and ", "of ", "for ", "an ", "a ", "the "} {
			if strings.HasPrefix(lt, w) {
				t = t[len(w):]
				break
			}
		}
		if t == name {
			break
		}
		name = t
	}
	if name = strings.TrimSpace(name); len(name) < 3 {
		name = subject
	}
	if r := []rune(name); len(r) > 200 {
		name = string(r[:200])
	}
	return name
}

// HTS numbers in facts text (D381, fixed by D413 for #576). A code may be
// glued to the word before it ("subheading8419.34.0000", a PDF-extraction
// artifact) and a heading word may lead a list ("headings 8539 and 8540",
// "headings 4015, 4016, and 8484").
var (
	codeNum  = `\d{4}(?:\.\d{2,4})*`
	listTail = `(?:\s*(?:,\s*(?:and\s+|or\s+)?|\s(?:and|or|through|to)\s+)` + codeNum + `\b)*`
	// a heading word, then a code and any codes listed after it
	headingCode = regexp.MustCompile(`(?i)\b(?:sub)?headings?\s*` + codeNum + `\b` + listTail)
	// a dotted code anywhere, even glued to a word before it
	dottedCode = regexp.MustCompile(`\d{4}\.\d{2}(?:\.\d{2,4})*\b` + listTail)
	// a bare 4-digit heading (or a list of them) then ", HTSUS" (a year
	// such as "the 2024 HTSUS" has no comma and stays)
	htsusCode = regexp.MustCompile(`\b\d{4}` + listTail + `(,\s+HTSU?S?\b)`)
)

// stripCodes removes HTS numbers from a product's name and description, so
// the suggester cannot copy a code the facts state.
func stripCodes(s string) string {
	s = headingCode.ReplaceAllString(s, "[code removed]")
	s = dottedCode.ReplaceAllString(s, "[code removed]")
	return htsusCode.ReplaceAllString(s, "[code removed]$1")
}

// stripCodesV1 is the stripper the committed suggestion runs used (D381,
// frozen). It missed codes glued to a word, every code of a list but the
// first, the last digits of 10-digit codes, and product names (#576).
// parts-eval uses it to find which committed inputs carried a code.
var codeLikeV1 = regexp.MustCompile(`(?i)(?:sub)?headings?\s+\d{4}(?:\.\d{2})*(?:\.\d{2,4})?|\b\d{4}\.\d{2}(?:\.\d{2,4})*\b`)

func stripCodesV1(s string) string { return codeLikeV1.ReplaceAllString(s, "[code removed]") }

var numTok = regexp.MustCompile(`\d{4}(?:\.\d{2,4})*`)

// leftCodes lists the numbers of 4 digits or more that are in the input
// the committed run saw (v1) but not in the input stripCodes makes now:
// the HTS numbers that reached the suggester. A year such as "2024" is in
// both and is not listed, nor are the 2 last digits of a 10-digit code v1
// left behind ("[code removed]00").
func leftCodes(v1, now domain.Product) []string {
	have := map[string]int{}
	for _, t := range numTok.FindAllString(now.Name+" "+now.Description, -1) {
		have[t]++
	}
	var out []string
	for _, t := range numTok.FindAllString(v1.Name+" "+v1.Description, -1) {
		if have[t] > 0 {
			have[t]--
			continue
		}
		out = append(out, t)
	}
	return out
}

// inputCodes maps each product of a split whose committed input carried an
// HTS number to those numbers.
func inputCodes(split string) (map[string][]string, error) {
	v1, err := evalProductsWith(split, func(s string) string { return s }, stripCodesV1)
	if err != nil {
		return nil, err
	}
	now, err := evalProducts(split)
	if err != nil {
		return nil, err
	}
	out := map[string][]string{}
	for i := range v1 {
		if v1[i].Ruling != now[i].Ruling {
			return nil, fmt.Errorf("parts: products out of order: %s, %s", v1[i].Ruling, now[i].Ruling)
		}
		if l := leftCodes(v1[i].Product, now[i].Product); len(l) > 0 {
			out[v1[i].Ruling] = l
		}
	}
	return out, nil
}

// ---- gold ----

func cmdPartsGold(args []string) error {
	fl := flag.NewFlagSet("parts-gold", flag.ExitOnError)
	split := fl.String("split", "dev", "dev or final")
	raw := fl.String("raw", "", "directory of raw CROSS ruling JSON (not committed)")
	confirm := fl.Bool("confirm-heldout", false, "required for final (D381: once, after the freeze)")
	_ = fl.Parse(args)
	if *split != "dev" && !*confirm {
		return errors.New("held-out gold is read once (D381); pass -confirm-heldout")
	}
	if *raw == "" {
		return errors.New("-raw is required")
	}
	out := filepath.Join(partsDir, fmt.Sprintf("gold-%s.jsonl", *split))
	if _, err := os.Stat(out); err == nil && *split != "dev" {
		return fmt.Errorf("%s exists; held-out gold is read once", out)
	}
	prods, err := evalProducts(*split)
	if err != nil {
		return err
	}
	var rows []GoldRow
	for _, p := range prods {
		row := GoldRow{Ruling: p.Ruling, Set: p.Set, Parts: len(p.Product.Components), Gold: []GoldPart{}}
		text, err := readText(p.Ruling, *split, "full_text", *raw)
		if err != nil {
			row.Error = "no full text"
			rows = append(rows, row)
			continue
		}
		st := hts.PartStatements(text)
		for i, c := range p.Product.Components {
			g, ok, amb := goldFor(c.Name, st)
			if amb {
				row.Ambiguous = append(row.Ambiguous, c.Name)
			}
			if ok {
				row.Gold = append(row.Gold, GoldPart{Ruling: p.Ruling, Set: p.Set, Index: i, Part: c.Name, HTS: g.HTS, Statement: g.Part, Sentence: g.Sentence})
			}
		}
		rows = append(rows, row)
	}
	if err := os.MkdirAll(partsDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(out, jsonlOf(rows), 0o644); err != nil {
		return err
	}
	n, prodsWith := 0, 0
	for _, r := range rows {
		n += len(r.Gold)
		if len(r.Gold) > 0 {
			prodsWith++
		}
	}
	fmt.Printf("%s: %d products, %d with a gold part, %d gold parts -> %s\n", *split, len(rows), prodsWith, n, out)
	return nil
}

// goldFor finds the code CBP states for a part (D381): the classifying
// sentences whose thing matches the part's name (one's content words, plural
// "s" dropped, a subset of the other's); their codes must agree (each a
// prefix of the longest), and the longest is the gold. ok false when none
// match; amb true when the matches disagree.
func goldFor(name string, st []hts.Statement) (hts.Statement, bool, bool) {
	pw := words(name)
	if len(pw) == 0 {
		return hts.Statement{}, false, false
	}
	var match []hts.Statement
	for _, s := range st {
		sw := words(s.Part)
		if len(sw) > 0 && (subset(sw, pw) || subset(pw, sw)) {
			match = append(match, s)
		}
	}
	if len(match) == 0 {
		return hts.Statement{}, false, false
	}
	best := match[0]
	for _, m := range match[1:] {
		if len(digits(m.HTS)) > len(digits(best.HTS)) {
			best = m
		}
	}
	for _, m := range match {
		if !strings.HasPrefix(digits(best.HTS), digits(m.HTS)) {
			return hts.Statement{}, false, true
		}
	}
	return best, true, false
}

var parens = regexp.MustCompile(`\([^)]*\)`)

func words(s string) []string {
	var out []string
	for _, w := range retrieval.Tokens(parens.ReplaceAllString(s, " ")) {
		if len(w) > 3 && strings.HasSuffix(w, "s") && !strings.HasSuffix(w, "ss") {
			w = w[:len(w)-1]
		}
		if !slices.Contains(out, w) {
			out = append(out, w)
		}
	}
	return out
}

func subset(a, b []string) bool {
	for _, w := range a {
		if !slices.Contains(b, w) {
			return false
		}
	}
	return true
}

func digits(s string) string { return digitsOnly(s) }

// ---- suggest ----

func cmdPartsSuggest(args []string) error {
	fl := flag.NewFlagSet("parts-suggest", flag.ExitOnError)
	split := fl.String("split", "dev", "dev or final")
	model := fl.String("model", "gemini-3.8-flash", "gateway model (the router model the server uses)")
	workers := fl.Int("workers", 4, "parallel products")
	confirm := fl.Bool("confirm-heldout", false, "required for final (D381: once, after the freeze)")
	tag := fl.String("tag", "", "development run tag (dev only); the file is suggest-<split>[-<tag>].jsonl")
	limit := fl.Int("limit", 0, "development only: first N products")
	_ = fl.Parse(args)
	if *split != "dev" && (!*confirm || *tag != "" || *limit != 0) {
		return errors.New("held-out suggestions run once on the frozen code (D381); pass -confirm-heldout and no -tag or -limit")
	}
	base, key := os.Getenv("LITELLM_BASE_URL"), os.Getenv("LITELLM_API_KEY")
	if base == "" || key == "" {
		return errors.New("needs LITELLM_BASE_URL and LITELLM_API_KEY")
	}
	name := "suggest-" + *split
	if *tag != "" {
		name += "-" + *tag
	}
	out := filepath.Join(partsDir, name+".jsonl")
	if _, err := os.Stat(out); err == nil {
		return fmt.Errorf("%s exists; each run is kept", out)
	}
	prods, err := evalProducts(*split)
	if err != nil {
		return err
	}
	if *limit > 0 && *limit < len(prods) {
		prods = prods[:*limit]
	}
	ll := llm.NewLiteLLM(base, key, 180*time.Second)
	ll.SkipCache(true)
	x, err := hts.LoadIndex("data/hts_index.json")
	if err != nil {
		return err
	}
	products, err := hts.New(ll, *model, x, "data/facts", nil)
	if err != nil {
		return err
	}
	px, err := hts.LoadPartIndex("data/part_hts_index.json")
	if err != nil {
		return err
	}
	// As served, minus the two live lookups (CROSS search for the good, the
	// USITC check of model codes): the evaluation makes no web fetches.
	ps := hts.NewParts(ll, *model, px, products, nil)
	rows := make([]SuggestRow, len(prods))
	var wg sync.WaitGroup
	sem := make(chan struct{}, *workers)
	for i, p := range prods {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			before, _ := time.Parse(time.DateOnly, p.Date)
			t := &llm.CostTally{}
			ctx := llm.ContextWithCostTally(context.Background(), t)
			res, err := ps.Suggest(ctx, p.Product, hts.PartsOptions{Good: true, Before: before, Exclude: p.Ruling, NoCache: true})
			row := SuggestRow{Ruling: p.Ruling, Set: p.Set, Split: *split, Parts: res.Parts, Good: res.Good, Failed: res.ReasonsUnavailable,
				Model: *model, Prompt: hts.PartsPromptVersion, CostUSD: round4(t.USD())}
			if err != nil {
				row.Error = err.Error()
			}
			if res.Trace != nil {
				row.InTok, row.OutTok = res.Trace.InputTokens, res.Trace.OutputTokens
			}
			rows[i] = row
		}()
	}
	wg.Wait()
	if err := os.MkdirAll(partsDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(out, jsonlOf(rows), 0o644); err != nil {
		return err
	}
	cost, failed := 0.0, 0
	for _, r := range rows {
		cost += r.CostUSD
		if r.Error != "" {
			failed++
		}
	}
	fmt.Printf("%s: %d products, %d failed, $%.4f -> %s\n", *split, len(rows), failed, cost, out)
	return appendRunlog(map[string]any{"run": "parts-hts-" + name, "time": time.Now().Format(time.RFC3339), "model": *model,
		"llm_provider": "litellm", "llm_no_cache": true, "prereg": "D381", "held_out": *split != "dev", "git_revision": revision(),
		"prompt": hts.PartsPromptVersion, "cases": len(rows), "errors": failed, "cost_usd": round4(cost), "output": "eval/tariffshift/parts/" + name + ".jsonl"})
}

// ---- eval ----

// PartsCell is one split × set × run summary.
type PartsCell struct {
	Run       string `json:"run"`
	Split     string `json:"split"`
	Set       string `json:"set"`
	Products  int    `json:"products"`
	Failed    int    `json:"failed_calls"`
	Parts     int    `json:"parts"`
	Suggested int    `json:"parts_with_a_suggestion"`
	// Gold parts: CBP states the part's code in the ruling (regex, D381).
	Gold        int         `json:"gold_parts"`
	GoldRulings int         `json:"gold_products"`
	Top1        int         `json:"top1"`
	Top1CI      *[2]float64 `json:"top1_ci,omitempty"`
	Top3        int         `json:"top3"`
	Top3CI      *[2]float64 `json:"top3_ci,omitempty"`
	Top1Heading int         `json:"top1_same_heading"`
	// By the top suggestion's source.
	Top1FromRulings      int `json:"top1_from_rulings"`
	Top1FromRulingsRight int `json:"top1_from_rulings_right"`
	Top1FromModel        int `json:"top1_from_model"`
	Top1FromModelRight   int `json:"top1_from_model_right"`
	// The good (usmca only: CBP's code from the full-text extraction).
	GoodGold  int     `json:"good_gold"`
	GoodTop1  int     `json:"good_top1"`
	GoodTop3  int     `json:"good_top3"`
	CostUSD   float64 `json:"cost_usd"`
	CostPer   float64 `json:"cost_per_product_usd"`
	InTokens  int     `json:"input_tokens"`
	OutTokens int     `json:"output_tokens"`
	// The rule check with the top suggestions (usmca only).
	RuleCheck []RuleCheckCell `json:"rule_check,omitempty"`
	// InputCodes lists the products whose committed input still carried an
	// HTS number after the v1 stripper, with those numbers; Clean is the
	// cell without them (post-freeze correction, #576, D413).
	InputCodes []string     `json:"input_codes,omitempty"`
	Clean      *CleanCounts `json:"without_input_codes,omitempty"`
}

// CleanCounts are a cell's counts without the products in InputCodes.
type CleanCounts struct {
	Products int `json:"products"`
	Gold     int `json:"gold_parts"`
	Top1     int `json:"top1"`
	Top3     int `json:"top3"`
	GoodGold int `json:"good_gold"`
	GoodTop1 int `json:"good_top1"`
	GoodTop3 int `json:"good_top3"`
}

// RuleClean is a rule-check cell without the products in InputCodes.
type RuleClean struct {
	N          int `json:"n"`
	OutOfScope int `json:"out_of_scope"`
	Decided    int `json:"decided"`
	AgreeCBP   int `json:"agree_cbp"`
}

// RuleCheckCell is the rule check on one input variant.
type RuleCheckCell struct {
	Input      string         `json:"input"`
	N          int            `json:"n"`
	OutOfScope int            `json:"out_of_scope"`
	Decided    int            `json:"decided"`
	AgreeCBP   int            `json:"agree_cbp"`
	Reasons    map[string]int `json:"undetermined_reasons"`
	Rulings    []string       `json:"decided_rulings,omitempty"`
	Disagree   []string       `json:"disagree_cbp_rulings,omitempty"`
	Clean      *RuleClean     `json:"without_input_codes,omitempty"`
	// Robustness (D430, post-freeze, offline): for the variants that use
	// suggested part codes, the decided rulings re-checked over every
	// combination of each part's top-3 suggestions.
	Robustness *RobustCounts `json:"robustness,omitempty"`
}

// RobustCounts says how many decided rulings keep their answer under every
// combination of each part's suggested codes (D430).
type RobustCounts struct {
	Decided int `json:"decided"`
	// Stable: the same answer under every combination.
	Stable      int `json:"stable"`
	StableAgree int `json:"stable_agree_cbp"`
	// Depends: the answer changes with some part's code.
	Depends      []string     `json:"depends_rulings,omitempty"`
	Sampled      int          `json:"sampled"`
	MaxEvaluated int          `json:"max_combinations_evaluated"`
	Clean        *RobustClean `json:"without_input_codes,omitempty"`
	cleanCounts  RobustClean  `json:"-"`
}

// RobustClean is RobustCounts without the products in InputCodes.
type RobustClean struct {
	Decided     int `json:"decided"`
	Stable      int `json:"stable"`
	StableAgree int `json:"stable_agree_cbp"`
}

func cmdPartsEval(args []string) error {
	fl := flag.NewFlagSet("parts-eval", flag.ExitOnError)
	check := fl.Bool("check", false, "recompute and compare with the committed summary; write nothing")
	_ = fl.Parse(args)
	rules, err := tariffshift.Default()
	if err != nil {
		return err
	}
	labels, err := usmcaLabels()
	if err != nil {
		return err
	}
	files, _ := filepath.Glob(filepath.Join(partsDir, "suggest-*.jsonl"))
	sort.Strings(files)
	var cells []PartsCell
	for _, f := range files {
		run := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(f), "suggest-"), ".jsonl")
		split, _, _ := strings.Cut(run, "-")
		sugg, err := readJSONL[SuggestRow](f)
		if err != nil {
			return err
		}
		gold, err := readJSONL[GoldRow](filepath.Join(partsDir, "gold-"+split+".jsonl"))
		if err != nil {
			return fmt.Errorf("gold for %s: %w", run, err)
		}
		goldBy := map[string]GoldRow{}
		for _, g := range gold {
			goldBy[g.Ruling] = g
		}
		leaks, err := inputCodes(split)
		if err != nil {
			return err
		}
		inputs, err := readInputs(filepath.Join(evalDir, fmt.Sprintf("inputs-%s-full_text.jsonl", split)))
		if err != nil {
			return err
		}
		inBy := map[string]InputRow{}
		for _, r := range inputs {
			inBy[r.Ruling] = r
		}
		for _, set := range []string{"usmca", "family"} {
			c := PartsCell{Run: run, Split: split, Set: set}
			var rc []RuleCheckCell
			if set == "usmca" {
				rc = []RuleCheckCell{{Input: "CBP's code for the good, CBP's stated part codes (D362 full text, for reference)", Reasons: map[string]int{}},
					{Input: "CBP's code for the good, top suggestion for every part", Reasons: map[string]int{}},
					{Input: "top suggestion for the good and every part", Reasons: map[string]int{}}}
			}
			cl := &CleanCounts{}
			rcl := make([]RuleClean, len(rc))
			for _, s := range sugg {
				if s.Set != set {
					continue
				}
				leaked := len(leaks[s.Ruling]) > 0
				if leaked {
					c.InputCodes = append(c.InputCodes, s.Ruling+": "+strings.Join(leaks[s.Ruling], ", "))
				} else {
					cl.Products++
				}
				c.Products++
				c.CostUSD += s.CostUSD
				c.InTokens += s.InTok
				c.OutTokens += s.OutTok
				if s.Error != "" {
					c.Failed++
				}
				top := map[int][]hts.PartCandidate{}
				for _, p := range s.Parts {
					c.Parts++
					if len(p.Candidates) > 0 {
						c.Suggested++
					}
					top[p.Index] = p.Candidates
				}
				g := goldBy[s.Ruling]
				if len(g.Gold) > 0 {
					c.GoldRulings++
				}
				for _, gp := range g.Gold {
					c.Gold++
					if !leaked {
						cl.Gold++
					}
					cs := top[gp.Index]
					for k, cand := range cs {
						if !codeMatch(cand.HTS, gp.HTS) {
							continue
						}
						if k == 0 {
							c.Top1++
							if !leaked {
								cl.Top1++
							}
						}
						c.Top3++
						if !leaked {
							cl.Top3++
						}
						break
					}
					if len(cs) > 0 {
						if len(digits(cs[0].HTS)) >= 4 && len(digits(gp.HTS)) >= 4 && digits(cs[0].HTS)[:4] == digits(gp.HTS)[:4] {
							c.Top1Heading++
						}
						right := codeMatch(cs[0].HTS, gp.HTS)
						if cs[0].Source == hts.SourceRulings {
							c.Top1FromRulings++
							if right {
								c.Top1FromRulingsRight++
							}
						} else {
							c.Top1FromModel++
							if right {
								c.Top1FromModelRight++
							}
						}
					}
				}
				if set != "usmca" {
					continue
				}
				in := inBy[s.Ruling]
				if cbp := in.Input.GoodHTS; cbp != "" && s.Good != nil {
					c.GoodGold++
					if !leaked {
						cl.GoodGold++
					}
					for k, cand := range s.Good.Candidates {
						if codeMatch(cand.HTS, cbp) {
							if k == 0 {
								c.GoodTop1++
								if !leaked {
									cl.GoodTop1++
								}
							}
							c.GoodTop3++
							if !leaked {
								cl.GoodTop3++
							}
							break
						}
					}
				}
				first := func(cs []hts.PartCandidate) string {
					if len(cs) == 0 {
						return ""
					}
					return cs[0].HTS
				}
				goodTop := ""
				if s.Good != nil {
					goodTop = first(s.Good.Candidates)
				}
				variants := []tariffshift.Input{in.Input, withParts(in.Input, in.Input.GoodHTS, top), withParts(in.Input, goodTop, top)}
				for vi, v := range variants {
					before := rc[vi]
					scoreRule(&rc[vi], rules, v, s.Ruling, labels[s.Ruling].Origin)
					if vi > 0 {
						robustRule(&rc[vi], rules, v, others(in.Input, top), s.Ruling, labels[s.Ruling].Origin, leaked)
					}
					if !leaked {
						rcl[vi].N++
						rcl[vi].OutOfScope += rc[vi].OutOfScope - before.OutOfScope
						rcl[vi].Decided += rc[vi].Decided - before.Decided
						rcl[vi].AgreeCBP += rc[vi].AgreeCBP - before.AgreeCBP
					}
				}
			}
			if c.Products == 0 {
				continue
			}
			c.CostUSD = round4(c.CostUSD)
			c.CostPer = round4(c.CostUSD / float64(c.Products))
			if c.Gold > 0 {
				lo, hi := wilson(c.Top1, c.Gold)
				c.Top1CI = &[2]float64{lo, hi}
				lo, hi = wilson(c.Top3, c.Gold)
				c.Top3CI = &[2]float64{lo, hi}
			}
			if len(c.InputCodes) > 0 {
				c.Clean = cl
				for i := range rc {
					rc[i].Clean = &rcl[i]
					if rb := rc[i].Robustness; rb != nil {
						rb.Clean = &rb.cleanCounts
					}
				}
			}
			c.RuleCheck = rc
			cells = append(cells, c)
		}
	}
	b, _ := json.MarshalIndent(map[string]any{
		"note":  "Per-part HTS suggestions (D380), scored as pre-registered in D381. A suggestion matches when it equals CBP's stated code at the level CBP stated (up to 8 digits). Gold: hts.PartStatements on the full ruling text, matched to the part by name. Suggestions see only rulings dated before the scored ruling, never the ruling itself; no CROSS search and no USITC check (offline). input_codes and without_input_codes (post-freeze, #576, D413): products whose committed input still carried an HTS number after the v1 code stripper, and each cell without them. robustness (post-freeze, D430, no model calls): each decided ruling re-checked over every combination of each part's top-3 suggested codes (past 81 combinations, a deterministic sample of 81); stable means the same answer under all of them. Recompute: cd backend && go run ./cmd/tariffshift parts-eval -check",
		"cells": cells,
	}, "", " ")
	if err := writeOrCheck(filepath.Join(partsDir, "summary.json"), append(b, '\n'), *check); err != nil {
		return err
	}
	for _, c := range cells {
		fmt.Printf("%-14s %-6s products=%d parts=%d suggested=%d gold=%d top1=%d top3=%d heading=%d top1_rulings=%d/%d top1_model=%d/%d good=%d/%d/%d cost=$%.4f ($%.4f/product)\n",
			c.Run, c.Set, c.Products, c.Parts, c.Suggested, c.Gold, c.Top1, c.Top3, c.Top1Heading, c.Top1FromRulingsRight, c.Top1FromRulings,
			c.Top1FromModelRight, c.Top1FromModel, c.GoodTop1, c.GoodTop3, c.GoodGold, c.CostUSD, c.CostPer)
		if c.Clean != nil {
			fmt.Printf("    input carried a code (v1 strip, #576): %s\n    without them: products=%d gold=%d top1=%d top3=%d good=%d/%d/%d\n",
				strings.Join(c.InputCodes, "; "), c.Clean.Products, c.Clean.Gold, c.Clean.Top1, c.Clean.Top3, c.Clean.GoodTop1, c.Clean.GoodTop3, c.Clean.GoodGold)
		}
		for _, r := range c.RuleCheck {
			fmt.Printf("    rule check [%s]: n=%d out_of_scope=%d decided=%d agree_cbp=%d %v\n", r.Input, r.N, r.OutOfScope, r.Decided, r.AgreeCBP, r.Reasons)
			if r.Clean != nil {
				fmt.Printf("      without input codes: n=%d out_of_scope=%d decided=%d agree_cbp=%d\n", r.Clean.N, r.Clean.OutOfScope, r.Clean.Decided, r.Clean.AgreeCBP)
			}
			if rb := r.Robustness; rb != nil {
				fmt.Printf("      over the top-3 suggestions (D430): decided=%d stable=%d stable_agree_cbp=%d depends=%v sampled=%d\n", rb.Decided, rb.Stable, rb.StableAgree, rb.Depends, rb.Sampled)
				if rb.Clean != nil {
					fmt.Printf("        without input codes: decided=%d stable=%d stable_agree_cbp=%d\n", rb.Clean.Decided, rb.Clean.Stable, rb.Clean.StableAgree)
				}
			}
		}
	}
	return nil
}

// codeMatch is true when a suggestion equals the gold code at the gold's
// level (up to 8 digits); a shorter suggestion never matches.
func codeMatch(sugg, gold string) bool {
	s, g := digits(sugg), digits(gold)
	if len(g) > 8 {
		g = g[:8]
	}
	return len(g) >= 4 && len(s) >= len(g) && s[:len(g)] == g
}

// withParts is the extraction's input with the good's code and every part's
// top suggestion (nothing when there is none; "outside heading X" dropped).
func withParts(in tariffshift.Input, good string, top map[int][]hts.PartCandidate) tariffshift.Input {
	out := in
	out.GoodHTS = good
	out.Materials = make([]tariffshift.Material, len(in.Materials))
	for i, m := range in.Materials {
		m.HTS, m.NotIn = "", ""
		if cs := top[i]; len(cs) > 0 {
			m.HTS = cs[0].HTS
		}
		out.Materials[i] = m
	}
	return out
}

// others is, for each material, the codes suggested after the top one.
func others(in tariffshift.Input, top map[int][]hts.PartCandidate) [][]string {
	out := make([][]string, len(in.Materials))
	for i := range in.Materials {
		for _, c := range top[i] {
			out[i] = append(out[i], c.HTS)
		}
		if len(out[i]) > 0 {
			out[i] = out[i][1:]
		}
	}
	return out
}

// robustRule re-checks a decided ruling over every combination of its
// parts' suggested codes (D430).
func robustRule(c *RuleCheckCell, rules *tariffshift.Rules, in tariffshift.Input, alts [][]string, ruling string, gold *string, leaked bool) {
	if c.Robustness == nil {
		c.Robustness = &RobustCounts{}
	}
	rc := c.Robustness
	if rules.Evaluate(in).Status != tariffshift.Decided {
		return
	}
	rc.Decided++
	if !leaked {
		rc.cleanCounts.Decided++
	}
	rb := rules.Robust(in, alts)
	if rb != nil {
		rc.MaxEvaluated = max(rc.MaxEvaluated, rb.Evaluated)
		if rb.Sampled {
			rc.Sampled++
		}
	}
	if rb != nil && !rb.Stable {
		rc.Depends = append(rc.Depends, ruling+": "+rb.Summary)
		return
	}
	rc.Stable++
	agree := gold != nil && *same(rules.Evaluate(in).Origin, *gold)
	if agree {
		rc.StableAgree++
	}
	if !leaked {
		rc.cleanCounts.Stable++
		if agree {
			rc.cleanCounts.StableAgree++
		}
	}
}

func scoreRule(c *RuleCheckCell, rules *tariffshift.Rules, in tariffshift.Input, ruling string, gold *string) {
	c.N++
	r := rules.Evaluate(in)
	switch r.Status {
	case tariffshift.Decided:
		c.Decided++
		c.Rulings = append(c.Rulings, ruling)
		if gold != nil && *same(r.Origin, *gold) {
			c.AgreeCBP++
		} else {
			c.Disagree = append(c.Disagree, ruling+": "+r.Origin)
		}
	case tariffshift.OutOfScope:
		c.OutOfScope++
	default:
		c.Reasons[reasonKey(r)]++
	}
}

func readJSONL[T any](f string) ([]T, error) {
	b, err := os.ReadFile(f)
	if err != nil {
		return nil, err
	}
	var rows []T
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 1<<20), 1<<26)
	for sc.Scan() {
		var r T
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		rows = append(rows, r)
	}
	return rows, sc.Err()
}

func jsonlOf[T any](rows []T) []byte {
	var buf bytes.Buffer
	for _, r := range rows {
		b, _ := json.Marshal(r)
		buf.Write(append(b, '\n'))
	}
	return buf.Bytes()
}
