package domain

import "time"

// SuppliedFamilyFacts are the requester's answers to a case's family
// questions (#770, D810): where a part or step the family's cited rulings
// turn on was made, or whether a step was done. Like SuppliedOriginFacts
// they sit beside the case, attributed and timestamped; the case's facts
// text, product and parts list are never changed by them (D811).
type SuppliedFamilyFacts struct {
	// Source is always SourceUserSupplied.
	Source    string    `json:"source"`
	UpdatedAt time.Time `json:"updated_at"`
	Actor     string    `json:"actor,omitempty"`
	// Family and TemplateVersion are the template the questions came from.
	Family          string `json:"family"`
	TemplateVersion string `json:"template_version"`
	// Answers are keyed by question id.
	Answers map[string]FamilyAnswer `json:"answers"`
	// Stale is set on read, never stored: the parts list was replaced, or
	// the case's family changed, after these were saved, so none of them is
	// read and the questions are asked again (D812).
	Stale bool `json:"stale,omitempty"`
}

// FamilyAnswer answers one family question: a country for a "where" question,
// yes or no for a "was this done" question.
type FamilyAnswer struct {
	Country string `json:"country,omitempty"`
	Yes     *bool  `json:"yes,omitempty"`
}

// Clone copies s deeply, so a read-time flag cannot write through a stored
// value.
func (s *SuppliedFamilyFacts) Clone() *SuppliedFamilyFacts {
	if s == nil {
		return nil
	}
	c := *s
	if s.Answers != nil {
		c.Answers = make(map[string]FamilyAnswer, len(s.Answers))
		for k, v := range s.Answers {
			if v.Yes != nil {
				y := *v.Yes
				v.Yes = &y
			}
			c.Answers[k] = v
		}
	}
	return &c
}

// Empty reports whether nothing was supplied.
func (s *SuppliedFamilyFacts) Empty() bool { return s == nil || len(s.Answers) == 0 }
