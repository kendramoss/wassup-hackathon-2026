package main

// Feature is one capability of the specialist challenge study.
type Feature struct {
	ID    string // summary feature id
	File  string // dataset file stem under eval/challenges/specialists
	Label string
}

// Features are the four model-backed specialists, in summary order.
var Features = []Feature{
	{"textile_specialist", "textile", "Textile origin (19 CFR 102.21)"},
	{"pharma_specialist", "pharma", "Pharmaceutical origin"},
	{"part102_specialist", "part102", "Part 102 marking origin (usmca-v5)"},
	{"japan_engine", "japan", "Japan preference engine"},
}

// Study bar: a prediction is "confident" (would be accepted without review)
// when its stated confidence reaches this value. It is the general engine's
// auto-accept bar (config.DefaultFastPath); production sends every specialist
// case to review (fast path 0), so this is a study convention, not a deployed bar.
const ConfidentBar = 0.92

// Source is a primary-source reference behind a case.
type Source struct {
	ID      string `json:"id"`
	Locator string `json:"locator"`
}

// JapanInput is the model-visible Japan request.
type JapanInput struct {
	Agreement string `json:"agreement"`
	HS        string `json:"hs"`
}

// Input is everything a candidate may see: facts, a date and, for Japan, the
// agreement and tariff number the user chose.
type Input struct {
	Facts string      `json:"facts"`
	Date  string      `json:"date"`
	Japan *JapanInput `json:"japan,omitempty"`
}

// Expected is the label, written before any scoring. Kind is "decided" or
// "uncertain" (the correct result is review or, for Japan, undetermined).
type Expected struct {
	Kind      string   `json:"kind"`
	Origin    string   `json:"origin,omitempty"`
	Rule      string   `json:"rule,omitempty"`
	Para      string   `json:"para,omitempty"`
	Outcome   string   `json:"outcome,omitempty"`
	Criteria  []string `json:"criteria,omitempty"`
	Preflight bool     `json:"preflight,omitempty"`
}

// Case is one synthetic, source-grounded case.
type Case struct {
	ID             string   `json:"id"`
	Feature        string   `json:"feature"`
	Split          string   `json:"split"`
	SourceFamily   string   `json:"source_family"`
	ScenarioFamily string   `json:"scenario_family"`
	Template       string   `json:"template"`
	VariationTags  []string `json:"variation_tags"`
	Sources        []Source `json:"sources"`
	Parents        []string `json:"parents"`
	ExcludeRulings []string `json:"exclude_rulings"`
	Rationale      string   `json:"rationale"`
	Synthetic      bool     `json:"synthetic"`
	Input          Input    `json:"input"`
	Expected       Expected `json:"expected"`
}

// Row is one scored attempt. Errors stay in the denominator.
type Row struct {
	ID            string   `json:"id"`
	Feature       string   `json:"feature"`
	Split         string   `json:"split"`
	Error         string   `json:"error,omitempty"`
	CalledModel   bool     `json:"called_model"`
	Provider      string   `json:"provider,omitempty"`
	ModelID       string   `json:"model_id,omitempty"`
	PromptVersion string   `json:"prompt_version,omitempty"`
	Origin        string   `json:"origin,omitempty"`
	Confidence    float64  `json:"confidence"`
	Route         string   `json:"route,omitempty"`
	Outcome       string   `json:"outcome,omitempty"`
	Criterion     string   `json:"criterion,omitempty"`
	Retrieved     []string `json:"retrieved,omitempty"`
	Verdict       string   `json:"verdict"`
	Class         string   `json:"class"`
	CriterionOK   *bool    `json:"criterion_valid,omitempty"`
	LatencyMS     int64    `json:"latency_ms"`
	CostUSD       float64  `json:"cost_usd"`
	InputTokens   int      `json:"input_tokens"`
	OutputTokens  int      `json:"output_tokens"`
}

// Verdicts.
const (
	Match    = "match"
	Mismatch = "mismatch"
	ErrorV   = "error"
)

// Counts of one split of one feature.
type Counts struct {
	Cases      int `json:"cases"`
	Matched    int `json:"matched"`
	Mismatched int `json:"mismatched"`
	Errors     int `json:"errors"`
}

// DecidedStats keeps the denominators of the cases whose label is a decision.
type DecidedStats struct {
	Cases            int `json:"cases"`
	Agreed           int `json:"agreed"`
	Wrong            int `json:"wrong"`
	Abstained        int `json:"abstained"`
	Errors           int `json:"errors"`
	Confident        int `json:"confident"`
	ConfidentCorrect int `json:"confident_correct"`
	ConfidentWrong   int `json:"confident_wrong"`
}

// UncertainStats keeps the denominators of the cases whose label is review.
type UncertainStats struct {
	Cases              int `json:"cases"`
	AbstainedCorrectly int `json:"abstained_correctly"`
	Overconfident      int `json:"overconfident"`
	Errors             int `json:"errors"`
}

// SplitSummary is Counts plus the separate decided and uncertain denominators.
type SplitSummary struct {
	Counts
	Decided          DecidedStats   `json:"expected_decided"`
	Uncertain        UncertainStats `json:"expected_uncertain"`
	ModelCalls       int            `json:"model_calls"`
	Preflight        int            `json:"preflight_cases"`
	CriterionValid   int            `json:"criterion_valid,omitempty"`
	CriterionInvalid int            `json:"criterion_invalid,omitempty"`
}

// FeatureSummary is one entry of summary.json.
type FeatureSummary struct {
	ID                string       `json:"id"`
	Label             string       `json:"label"`
	CandidateRevision string       `json:"candidate_revision"`
	DatasetSHA256     string       `json:"dataset_sha256"`
	DevDatasetSHA256  string       `json:"dev_dataset_sha256"`
	SourceFamilies    int          `json:"source_families"`
	SharedFamilies    int          `json:"source_families_shared_with_dev"`
	Development       SplitSummary `json:"development"`
	Test              SplitSummary `json:"test"`
	CostUSD           float64      `json:"cost_usd"`
	LatencyP50MS      int64        `json:"latency_p50_ms"`
	LatencyP95MS      int64        `json:"latency_p95_ms"`
	Limitations       []string     `json:"limitations"`
}
