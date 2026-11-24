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
	// Stale is set on read, never stored: the parts list was replaced (a
	// BOM update) after these facts were saved, so none of them is read and
	// they are asked again (D806).
	Stale bool `json:"stale,omitempty"`
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

// Clone copies s deeply, so a read-time flag or a handler's change cannot
// write through a stored value.
func (s *SuppliedOriginFacts) Clone() *SuppliedOriginFacts {
	if s == nil {
		return nil
	}
	c := *s
	if s.Answers != nil {
		c.Answers = make(map[string]bool, len(s.Answers))
		for k, v := range s.Answers {
			c.Answers[k] = v
		}
	}
	c.Materials = append([]SuppliedMaterial(nil), s.Materials...)
	for i, m := range c.Materials {
		if m.ValueShare != nil {
			v := *m.ValueShare
			c.Materials[i].ValueShare = &v
		}
		if m.WeightShare != nil {
			v := *m.WeightShare
			c.Materials[i].WeightShare = &v
		}
	}
	return &c
}

// CloneOriginFacts copies a by-market map deeply; nil stays nil.
func CloneOriginFacts(m map[string]*SuppliedOriginFacts) map[string]*SuppliedOriginFacts {
	if m == nil {
		return nil
	}
	out := make(map[string]*SuppliedOriginFacts, len(m))
	for k, v := range m {
		out[k] = v.Clone()
	}
	return out
}

// Empty reports whether nothing was supplied.
func (s *SuppliedOriginFacts) Empty() bool {
	return s == nil || (s.GoodHTS == "" && s.Row == "" && len(s.Answers) == 0 && len(s.Materials) == 0)
}
