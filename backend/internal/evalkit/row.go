package evalkit

// Row is one engine prediction on one labelled ruling, written as JSONL to
// eval/results/. Fields are kept stable so runs stay comparable.
type Row struct {
	Ruling        string   `json:"ruling"`
	Date          string   `json:"date"`
	Split         string   `json:"split"`
	LabelMethod   string   `json:"label_method"`
	Gold          string   `json:"gold"`
	Pred          string   `json:"pred"`
	Correct       bool     `json:"correct"`
	Confidence    float64  `json:"confidence"`
	KeyFactors    []string `json:"key_factors"`
	DecisiveStep  string   `json:"decisive_step"`
	Flip          string   `json:"flip"`
	Reasoning     string   `json:"reasoning"`
	Precedents    []string `json:"precedents"`
	Cited         []string `json:"cited"`
	TriageRoute   string   `json:"triage_route"`
	InScope       bool     `json:"in_scope"`
	RouterRoute   string   `json:"router_route,omitempty"`
	RouterRegime  string   `json:"router_regime,omitempty"`
	RouterFamily  string   `json:"router_subfamily,omitempty"`
	Provider      string   `json:"llm_provider"`
	ModelID       string   `json:"model_id"`
	PromptVersion string   `json:"prompt_version"`
	Retrieval     string   `json:"retrieval,omitempty"` // bm25 (default), hybrid, vector
	Votes         int      `json:"votes,omitempty"`     // model calls voted on (0/1 = single call)
	InputTokens   int      `json:"input_tokens"`
	OutputTokens  int      `json:"output_tokens"`
	LatencyMS     int64    `json:"latency_ms"`
	CostUSD       float64  `json:"cost_usd"`
	CostUSDList   float64  `json:"cost_usd_list"`
	Error         string   `json:"error,omitempty"`
}
