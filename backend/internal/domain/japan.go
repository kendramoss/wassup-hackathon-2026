package domain

// JapanInput identifies the trade agreement and finished-good tariff code
// for an assessment of preferential origin on entry into Japan.
type JapanInput struct {
	Agreement string `json:"agreement"`
	HS        string `json:"hs"`
}

// JapanResult is a preliminary trade-agreement assessment. A single
// agreement's result does not establish non-preferential origin.
type JapanResult struct {
	Agreement        string        `json:"agreement"`
	HS               string        `json:"hs"`
	Outcome          string        `json:"outcome"`
	Criterion        string        `json:"criterion,omitempty"`
	DecidingRule     string        `json:"deciding_rule,omitempty"`
	ReviewReason     string        `json:"review_reason,omitempty"`
	Sources          []JapanSource `json:"sources,omitempty"`
	PrecedentSources []JapanSource `json:"precedent_sources,omitempty"`
}

// JapanSource identifies a treaty rule or Japan Customs ruling.
type JapanSource struct {
	Citation string `json:"citation"`
	URL      string `json:"url"`
}

const JapanDisclaimer = "Not a ruling. Confirm with a licensed Japanese customs specialist."
