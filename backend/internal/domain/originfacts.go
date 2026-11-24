package domain

import "time"

// SourceUserSupplied attributes a fact the requester typed into an origin
// check, not one read from the case's product (#770, D801).
const SourceUserSupplied = "user-supplied"

// SuppliedOriginFacts are the facts a requester gave one rules check of a
// case (the EU or UK non-preferential origin check) to settle what it asked
// for. They are inputs to that check only: the case's facts and product stay
// exactly as stored, and these sit beside them, attributed and timestamped
// (D801).
type SuppliedOriginFacts struct {
	// Source is always SourceUserSupplied.
	Source string `json:"source"`
	// UpdatedAt is when they were last saved; Actor the signed-in user who
	// saved them, when there is one.
	UpdatedAt time.Time `json:"updated_at"`
	Actor     string    `json:"actor,omitempty"`
	// GoodHTS is a longer code for the good that starts with the product's
	// own code (the subheading a split heading needs).
	GoodHTS string `json:"good_hts,omitempty"`
	// Row is the list row the good is, or "none".
	Row string `json:"row,omitempty"`
	// Answers are yes/no facts keyed as the check's needs name them.
	Answers map[string]bool `json:"answers,omitempty"`
	// Materials add to the product's parts, by position.
	Materials []SuppliedMaterial `json:"materials,omitempty"`
}

// SuppliedMaterial adds facts to one part of the product. Name and PartHTS
// are the part as it stood when the facts were given: when the parts list
// changes so they no longer match, the facts are not applied.
type SuppliedMaterial struct {
	Index   int    `json:"index"`
	Name    string `json:"name"`
	PartHTS string `json:"part_hts,omitempty"`
	// HTS is a code for a part that has none, or a longer code that starts
	// with the part's own.
	HTS string `json:"hts,omitempty"`
	// Origin is for a part with no origin.
	Origin      string   `json:"origin,omitempty"`
	ValueShare  *float64 `json:"value_share,omitempty"`
	WeightShare *float64 `json:"weight_share,omitempty"`
}

// Empty reports whether nothing was supplied.
func (s *SuppliedOriginFacts) Empty() bool {
	return s == nil || (s.GoodHTS == "" && s.Row == "" && len(s.Answers) == 0 && len(s.Materials) == 0)
}
