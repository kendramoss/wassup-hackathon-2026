package main

// torn: on dev cases where the engine's votes split between origins, does
// asking the clarify questions and answering them move the vote to CBP's
// answer? Answers come from a simulated requester that reads the ruling text
// before its HOLDING section; this measures agreement with CBP on split
// cases, not accuracy (the requester has seen CBP's analysis). Dev only.

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/clarify"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/config"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/evalkit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/retrieval"
)

const requesterVersion = "requester-v1"

const requesterSystem = `You are the importer who asked U.S. Customs for a country-of-origin ruling. Customs has sent follow-up questions about your product.
You are given your request file. Answer each question with facts about the product only: what each part is, where and how it is made or processed, materials and where they come from, costs or values, and the steps performed in each country.
Rules:
- Use only facts stated in the file. If the file does not state it, answer exactly "Not stated."
- Never state or hint at a conclusion: do not say which country the product originates in, whether anything is "substantially transformed", what gives the product its character, or what Customs decided or might decide.
- For every answer that is not "Not stated.", copy a short verbatim quote (under 300 characters) from the file that supports it.
- Answer in one to three sentences, in the first person plural ("we").`

var requesterSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"answers": map[string]any{"type": "array", "items": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"question": map[string]any{"type": "string"},
				"answer":   map[string]any{"type": "string"},
				"quote":    map[string]any{"type": "string"},
			},
			"required": []string{"question", "answer", "quote"},
		}},
	},
	"required": []string{"answers"},
}

// leakWords mark an answer that states or hints at the legal conclusion.
// A component's origin ("Vietnamese-origin motors") is a fact and passes.
var leakWords = regexp.MustCompile(`(?i)country of origin|origin of the (product|item|good|merchandise|finished)|originates? in|substantial|transform|essential character|name, character|ruling|\bCBP\b|customs|\bheld\b|holding|conclu|determin`)

var holdingHeading = regexp.MustCompile(`\bHOLDING:`)

type qa struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
	Quote    string `json:"quote,omitempty"`
	Dropped  string `json:"dropped,omitempty"`  // why the requester's answer was replaced with "Not stated."
	Original string `json:"original,omitempty"` // the replaced answer, for audit
}

func normSpace(s string) string { return strings.Join(strings.Fields(strings.ToLower(s)), " ") }

// requestFile is what the simulated requester may read: an HQ ruling up to
// its HOLDING heading (facts plus analysis), or for NY rulings, which state new
// facts only inside CBP's reasoning, just the facts section the engine sees.
func requestFile(text, facts string) (file, source string) {
	if loc := holdingHeading.FindStringIndex(text); loc != nil {
		return text[:loc[0]], "pre_holding"
	}
	return facts, "facts_only"
}

// guard keeps an answer only if it is grounded in the file and states no conclusion.
func guard(a qa, file string) qa {
	a.Answer, a.Quote = strings.TrimSpace(a.Answer), strings.TrimSpace(a.Quote)
	switch {
	case strings.EqualFold(a.Answer, "Not stated."):
		return qa{Question: a.Question, Answer: "Not stated."}
	case leakWords.MatchString(a.Answer):
		a.Dropped = "conclusion word"
	case a.Quote == "" || !strings.Contains(normSpace(file), normSpace(a.Quote)):
		a.Dropped = "quote not in file"
	default:
		return a
	}
	return qa{Question: a.Question, Answer: "Not stated.", Quote: a.Quote, Dropped: a.Dropped, Original: a.Answer}
}

// split describes a vote: the winner, runner-up and whether it is torn
// (runner-up got at least two of the calls).
type split struct {
	Counts map[string]int `json:"counts"`
	Winner string         `json:"winner"`
	Second string         `json:"second,omitempty"`
}

func newSplit(counts map[string]int, winner string) split {
	s := split{Counts: counts, Winner: winner}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		if k != winner {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		return counts[keys[i]] > counts[keys[j]] || (counts[keys[i]] == counts[keys[j]] && keys[i] < keys[j])
	})
	if len(keys) > 0 {
		s.Second = keys[0]
	}
	return s
}

func (s split) torn() bool    { return s.Second != "" && s.Counts[s.Second] >= 2 }
func (s split) settled() bool { return s.Counts[s.Winner] >= 4 }

func tornCmd(args []string) error {
	fl := flag.NewFlagSet("torn", flag.ExitOnError)
	in := fl.String("in", "../eval/results/dev-gemini-4-argon-prec-engine-v1-litellm.jsonl", "single-call engine result file (dev)")
	below := fl.Float64("below", 0.95, "candidate cases: first-call confidence below this")
	votes := fl.Int("votes", 5, "calls per vote")
	adjustFile := fl.String("adjust", "../eval/data/scoring_adjustments.json", "scoring adjustments")
	labelsPath := fl.String("labels", "../eval/data/labels.jsonl", "labels")
	factsDir := fl.String("facts", "../data/corpus/facts", "facts")
	rawDir := fl.String("raw", "../data/corpus/raw", "raw ruling JSON (for the simulated requester)")
	limit := fl.Int("limit", 0, "max candidate cases (0 = all)")
	only := fl.String("ids", "", "comma-separated rulings to run (default: all candidates)")
	_ = fl.Parse(args)
	rows, err := readRows(*in, nil)
	if err != nil {
		return err
	}
	if len(rows) > 0 && rows[0].Split != "dev" {
		return errors.New("torn reads CBP's analysis through the simulated requester: dev only")
	}
	adj, err := evalkit.LoadAdjustments(*adjustFile)
	if err != nil {
		return err
	}
	rows = adj.Apply(rows)
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx := context.Background()
	model, err := newModel(ctx, cfg, slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})))
	if err != nil {
		return err
	}
	labels, err := corpus.LoadLabels(*labelsPath)
	if err != nil {
		return err
	}
	store, err := corpus.Load(labels, *factsDir)
	if err != nil {
		return err
	}
	// VoteBelow above 1 makes every case vote.
	eng := engine.New(model, retrieval.NewBM25(store.Rulings(), engine.PrecedentText), engine.Options{Model: cfg.EngineModel,
		K: cfg.PrecedentK, MaxOutputTokens: cfg.MaxOutputTokens, Votes: *votes, VoteBelow: 1.01})
	cl := clarify.New(model, cfg.EngineModel, 3000)

	var cands []evalkit.Row
	for _, r := range rows {
		if *only != "" && !slices.Contains(strings.Split(*only, ","), r.Ruling) {
			continue
		}
		if r.Error == "" && r.Confidence < *below {
			cands = append(cands, r)
		}
	}
	if *limit > 0 && *limit < len(cands) {
		cands = cands[:*limit]
	}
	outPath := filepath.Join(filepath.Dir(*in), fmt.Sprintf("torn-dev-%s-%s-%s-%s.jsonl", clarify.PromptVersion, requesterVersion, clarify.AnswersVersion, cfg.LLMProvider))
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()

	type result struct {
		gold                   string
		alts                   []string
		before, control, after split
		dropped, notStated, n  int
	}
	var (
		mu      sync.Mutex
		wg      sync.WaitGroup
		results []result
		calls   int
		usage   llm.Usage
	)
	sem := make(chan struct{}, 4)
	vote := func(c domain.Case) (split, error) {
		res, err := eng.Predict(ctx, c)
		if err != nil {
			return split{}, err
		}
		mu.Lock()
		calls += res.Votes
		usage.InputTokens += res.Usage.InputTokens
		usage.OutputTokens += res.Usage.OutputTokens
		mu.Unlock()
		counts := res.VoteCounts
		if counts == nil {
			counts = map[string]int{res.Prediction.Origin: 1}
		}
		return newSplit(counts, res.Prediction.Origin), nil
	}
	for _, r := range cands {
		wg.Add(1)
		sem <- struct{}{}
		go func(r evalkit.Row) {
			defer wg.Done()
			defer func() { <-sem }()
			c, _ := store.Get(r.Ruling)
			base := domain.Case{ID: c.ID, Date: c.Date, Facts: c.Facts}
			before, err := vote(base)
			if err != nil {
				fmt.Fprintln(os.Stderr, r.Ruling, err)
				return
			}
			line := map[string]any{"ruling": r.Ruling, "gold": r.Gold, "before": before, "torn": before.torn()}
			res := result{gold: r.Gold, alts: adj.Alternatives[r.Ruling], before: before}
			if before.torn() {
				if err := runTorn(ctx, r, c, before, cl, model, cfg, *rawDir, vote, store, line, &res.control, &res.after, &res.dropped, &res.notStated, &res.n); err != nil {
					fmt.Fprintln(os.Stderr, r.Ruling, err)
					return
				}
			}
			b, _ := json.Marshal(line)
			mu.Lock()
			defer mu.Unlock()
			_, _ = f.Write(append(b, '\n'))
			results = append(results, res)
		}(r)
	}
	wg.Wait()

	var torn []result
	for _, r := range results {
		if r.before.torn() && r.after.Winner != "" {
			torn = append(torn, r)
		}
	}
	fmt.Printf("## Split votes and follow-up questions (dev, first-call confidence < %.2f, %d calls per vote, %s)\n\n", *below, *votes, cfg.LLMProvider)
	fmt.Printf("Candidates: %d. Torn (runner-up got ≥2 of %d calls): %d.\n\n", len(results), *votes, len(torn))
	if len(torn) == 0 {
		return nil
	}
	agrees := func(r result, pred string) bool {
		if len(r.alts) == 0 {
			return domain.NormalizeCountry(pred) == r.gold
		}
		parts := domain.CountryParts(domain.NormalizeCountry(pred))
		for _, p := range parts {
			if !slices.Contains(r.alts, p) {
				return false
			}
		}
		return len(parts) > 0
	}
	match := func(get func(result) split) (int, int) {
		m, s := 0, 0
		for _, r := range torn {
			if agrees(r, get(r).Winner) {
				m++
			}
			if get(r).settled() {
				s++
			}
		}
		return m, s
	}
	fmt.Println("| Vote | majority = CBP (95% CI) | settled (≥4 of 5 agree) |\n|---|---|---|")
	for _, x := range []struct {
		name string
		get  func(result) split
	}{
		{"before (original facts)", func(r result) split { return r.before }},
		{"control (original facts, re-voted)", func(r result) split { return r.control }},
		{"after answers", func(r result) split { return r.after }},
	} {
		m, s := match(x.get)
		p, lo, hi := evalkit.Wilson(m, len(torn))
		fmt.Printf("| %s | %d/%d = %.0f%% (%.0f–%.0f) | %d/%d |\n", x.name, m, len(torn), 100*p, 100*lo, 100*hi, s, len(torn))
	}
	b, c := 0, 0 // control right & after wrong, control wrong & after right
	answers, dropped, notStated := 0, 0, 0
	for _, r := range torn {
		cr, ar := agrees(r, r.control.Winner), agrees(r, r.after.Winner)
		if cr && !ar {
			b++
		}
		if !cr && ar {
			c++
		}
		answers += r.n
		dropped += r.dropped
		notStated += r.notStated
	}
	fmt.Printf("\nAfter vs control: %d fixed, %d broken (exact McNemar p = %.3f).\n", c, b, mcnemarExact(b, c))
	fmt.Printf("Answers: %d; \"Not stated.\" %d (of which dropped by the leak/grounding guard %d).\n", answers, notStated, dropped)
	fmt.Printf("Calls: %d engine + clarify + requester; cost $%.2f (promo, engine model prices). Rows: %s\n", calls,
		evalkit.Cost(evalkit.PromoPrices, cfg.EngineModel, usage.InputTokens, usage.OutputTokens), outPath)
	return nil
}

func runTorn(ctx context.Context, r evalkit.Row, c domain.Ruling, before split, cl *clarify.Clarifier, model llm.Model, cfg config.Config,
	rawDir string, vote func(domain.Case) (split, error), store *corpus.Store, line map[string]any,
	control, after *split, dropped, notStated, n *int) error {
	base := domain.Case{ID: c.ID, Date: c.Date, Facts: c.Facts}
	ctrl, err := vote(base)
	if err != nil {
		return err
	}
	*control = ctrl
	var ps []domain.Ruling
	for _, id := range r.Precedents {
		if p, ok := store.Get(id); ok {
			ps = append(ps, p)
		}
	}
	// clarify-v1 sees one prediction; the split goes in the reasoning so the
	// questions target what separates the two candidate origins.
	reason := fmt.Sprintf("%s Repeated analysis split: %s in %d of %d calls, %s in %d.", r.Reasoning,
		before.Winner, before.Counts[before.Winner], total(before.Counts), before.Second, before.Counts[before.Second])
	rev, err := cl.Review(ctx, c.Facts, domain.Prediction{Origin: before.Winner, Confidence: r.Confidence, KeyFactors: r.KeyFactors, Reasoning: reason}, ps)
	if err != nil {
		return err
	}
	line["control"], line["clarify"] = ctrl, rev
	if len(rev.Questions) == 0 {
		*after = ctrl
		line["after"] = ctrl
		return nil
	}
	raw, err := os.ReadFile(filepath.Join(rawDir, c.ID+".json"))
	if err != nil {
		return err
	}
	var doc struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return err
	}
	file, source := requestFile(doc.Text, c.Facts)
	line["requester_file"] = source
	var qs strings.Builder
	for i, q := range rev.Questions {
		fmt.Fprintf(&qs, "%d. %s\n", i+1, q)
	}
	resp, err := model.Generate(ctx, llm.Request{Model: cfg.EngineModel, System: requesterSystem,
		Messages:   []llm.Message{{Role: llm.RoleUser, Content: "YOUR REQUEST FILE:\n" + corpus.Truncate(file, 14000) + "\n\nQUESTIONS FROM CUSTOMS:\n" + qs.String()}},
		JSONSchema: requesterSchema, SchemaName: "answers", MaxOutputTokens: 3000})
	if err != nil {
		return err
	}
	var out struct {
		Answers []qa `json:"answers"`
	}
	if err := resp.DecodeJSON(&out); err != nil {
		return err
	}
	as := make([]qa, 0, len(rev.Questions))
	for i, q := range rev.Questions {
		a := qa{Question: q, Answer: "Not stated."}
		if i < len(out.Answers) {
			a = guard(qa{Question: q, Answer: out.Answers[i].Answer, Quote: out.Answers[i].Quote}, file)
		}
		if a.Dropped != "" {
			*dropped++
		}
		if a.Answer == "Not stated." {
			*notStated++
		}
		as = append(as, a)
	}
	*n = len(as)
	line["answers"] = as
	cas := make([]clarify.Answer, len(as))
	for i, a := range as {
		cas[i] = clarify.Answer{Question: a.Question, Answer: a.Answer}
	}
	facts, answered := clarify.AppendAnswers(c.Facts, cas)
	if !answered { // nothing to add: the vote is the control vote
		*after = ctrl
		line["after"] = ctrl
		return nil
	}
	aft, err := vote(domain.Case{ID: c.ID, Date: c.Date, Facts: facts})
	if err != nil {
		return err
	}
	*after = aft
	line["answers"], line["after"], line["requester_version"], line["answers_version"] = as, aft, requesterVersion, clarify.AnswersVersion
	return nil
}

func total(m map[string]int) int {
	n := 0
	for _, v := range m {
		n += v
	}
	return n
}
