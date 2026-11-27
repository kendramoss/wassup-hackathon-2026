package domain

import (
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// componentHTS accepts a heading ("7616") to a full 10-digit code
// ("8504.40.9580"), with dots or spaces.
var componentHTS = regexp.MustCompile(`^[0-9]{4}(?:[. ]?[0-9]{2}){0,3}$`)

// Plant is a site where operations can be performed.
type Plant struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Country string `json:"country"`
	// Capabilities lists the operation groups this plant can perform (the
	// plant capability matrix). Empty means it can only keep what it does now.
	Capabilities []string `json:"capabilities,omitempty"`
}

// Can reports whether the plant can perform an operation group.
func (p Plant) Can(group string) bool {
	for _, g := range p.Capabilities {
		if g == group {
			return true
		}
	}
	return false
}

// Component is a part of the product, made (or bought) in a country.
// Plant is the ID of the product's own plant that makes it, or empty when the
// component is bought in from a supplier in MadeIn.
type Component struct {
	Name   string `json:"name"`
	MadeIn string `json:"made_in"`
	Plant  string `json:"plant,omitempty"`
	// Group is the operation group that produces this component (e.g.
	// "motor_fan_assembly"), so moving the group moves the component.
	Group string `json:"group,omitempty"`
	// HTS is the component's own tariff code (heading or subheading),
	// optional. Only the 19 CFR 102.20 rule check reads it (D363); it is not
	// part of the facts the engine reads.
	HTS string `json:"hts,omitempty"`
	// HTSAlternatives are the other codes suggested for the part when HTS
	// was accepted from a suggestion (D380), at most MaxHTSAlternatives.
	// Only the rule check reads them, to say whether its answer is the same
	// under each suggested code (D430); not part of the facts.
	HTSAlternatives []string `json:"hts_alternatives,omitempty"`
}

// MaxHTSAlternatives caps a part's other suggested codes: the suggester
// gives at most three codes, one of which the part carries.
const MaxHTSAlternatives = 2

// Operation is a processing step performed at one of the product's plants.
type Operation struct {
	Name  string `json:"name"`
	Plant string `json:"plant"`
	Group string `json:"group,omitempty"`
}

// Product is the structured description of a case: what is made, from which
// components, with which operations at which plants.
type Product struct {
	Name        string      `json:"name"`
	Family      string      `json:"family,omitempty"`
	Description string      `json:"description,omitempty"`
	Plants      []Plant     `json:"plants"`
	Components  []Component `json:"components"`
	Operations  []Operation `json:"operations"`
	// HTS is the product's tariff subheading (e.g. 8419.89.95), used to look
	// up sourced duty rates (D52). Optional; not part of the facts.
	HTS string `json:"hts,omitempty"`
	// Textile holds what the 19 CFR 102.21 rule check (TEXTILE_RULES, D951)
	// needs and the operations do not say. Optional; not part of the facts.
	Textile *TextileFacts `json:"textile,omitempty"`
}

// TextileFacts are the facts about a textile or apparel good that the
// 102.21(e) rules turn on. Each is "yes", "no" or empty (not stated).
type TextileFacts struct {
	KnitToShape         string `json:"knit_to_shape,omitempty"`
	TwoOrMoreComponents string `json:"two_or_more_components,omitempty"`
	// CottonWool16 is "yes" when the good is of cotton or wool, or a blend
	// of 16 percent or more cotton by weight.
	CottonWool16 string `json:"cotton_wool_16,omitempty"`
	FabricOfWool string `json:"fabric_of_wool,omitempty"`
	// FibreForm is "filaments" or "staple" for the goods that are yarn,
	// rope or trimmings.
	FibreForm string `json:"fibre_form,omitempty"`
}

// PlantByID returns the plant with the given ID.
func (p Product) PlantByID(id string) (Plant, bool) {
	for _, pl := range p.Plants {
		if pl.ID == id {
			return pl, true
		}
	}
	return Plant{}, false
}

// Validate checks that the product is complete and that every plant reference
// resolves.
func (p Product) Validate() error {
	var errs problems
	if strings.TrimSpace(p.Name) == "" {
		errs.add("product name is required")
	}
	if len(p.Components) == 0 {
		errs.add("at least one component is required")
	}
	seen := map[string]bool{}
	for i, pl := range p.Plants {
		switch {
		case pl.ID == "":
			errs.add("plants[%d]: id is required", i)
		case seen[pl.ID]:
			errs.add("plants[%d]: duplicate id %q", i, pl.ID)
		}
		seen[pl.ID] = true
		if strings.TrimSpace(pl.Country) == "" {
			errs.add("plants[%d]: country is required", i)
		}
	}
	for i, c := range p.Components {
		if strings.TrimSpace(c.Name) == "" {
			errs.add("components[%d]: name is required", i)
		}
		if strings.TrimSpace(c.MadeIn) == "" {
			errs.add("components[%d]: made_in is required", i)
		}
		if c.Plant != "" && !seen[c.Plant] {
			errs.add("components[%d]: unknown plant %q", i, c.Plant)
		}
		if h := strings.TrimSpace(c.HTS); h != "" && !componentHTS.MatchString(h) {
			errs.add("components[%d]: hts %q is not an HTS heading or subheading", i, c.HTS)
		}
		if len(c.HTSAlternatives) > MaxHTSAlternatives {
			errs.add("components[%d]: at most %d hts_alternatives", i, MaxHTSAlternatives)
		}
		for _, a := range c.HTSAlternatives {
			if !componentHTS.MatchString(strings.TrimSpace(a)) {
				errs.add("components[%d]: hts_alternatives %q is not an HTS heading or subheading", i, a)
			}
		}
	}
	if t := p.Textile; t != nil {
		yn := func(name, v string) {
			if v != "" && v != "yes" && v != "no" {
				errs.add("textile.%s must be yes or no, got %q", name, v)
			}
		}
		yn("knit_to_shape", t.KnitToShape)
		yn("two_or_more_components", t.TwoOrMoreComponents)
		yn("cotton_wool_16", t.CottonWool16)
		yn("fabric_of_wool", t.FabricOfWool)
		if t.FibreForm != "" && t.FibreForm != "filaments" && t.FibreForm != "staple" {
			errs.add("textile.fibre_form must be filaments or staple, got %q", t.FibreForm)
		}
	}
	for i, o := range p.Operations {
		if strings.TrimSpace(o.Name) == "" {
			errs.add("operations[%d]: name is required", i)
		}
		if !seen[o.Plant] {
			errs.add("operations[%d]: unknown plant %q", i, o.Plant)
		}
	}
	return errs.err()
}

// Case is one origin question: a product as of a date. Facts is the free-text
// description the engine reads; when Product is set, Facts may be rendered
// from it. Date bounds precedent retrieval: only rulings dated strictly before
// it may be used.
type Case struct {
	ID      string    `json:"id"`
	Date    time.Time `json:"date"`
	Facts   string    `json:"facts"`
	Product *Product  `json:"product,omitempty"`
	// Exclude lists rulings about this very case (e.g. its own earlier
	// ruling); they are never used as precedent.
	Exclude []string `json:"exclude_rulings,omitempty"`
	// Jurisdiction is the market whose entry the question is about (D680).
	// Nil reads as the United States, the only market before D680.
	Jurisdiction *Jurisdiction `json:"jurisdiction,omitempty"`
	Japan        *JapanInput   `json:"japan,omitempty"`
}

// Jurisdiction is the market a case's origin question is asked for, and how
// it was set (D680). Package jurisdiction holds the markets, the detector and
// the registry of supported markets.
type Jurisdiction struct {
	// Code is the market: us, eu, uk, ca, jp, kr, au, mx or other.
	Code string `json:"code"`
	// Source is "chosen" (the caller named it), "detected" (a cue in the
	// text) or "default" (no cue: the United States).
	Source string `json:"source"`
	// Cue is the text that set a detected market, e.g. "HMRC".
	Cue string `json:"cue,omitempty"`
	// Hint is a non-U.S. market the text suggests but that was not taken:
	// a weak cue (a currency, "sold in"), or one contradicted by a U.S. cue.
	Hint string `json:"hint,omitempty"`
	// Conflict is set when the text names a non-U.S. market and U.S. entry
	// with equal weight; the case is then always reviewed.
	Conflict bool `json:"conflict,omitempty"`
}

// MarketCode is the case's market code; a nil jurisdiction is "us".
func (j *Jurisdiction) MarketCode() string {
	if j == nil || j.Code == "" {
		return "us"
	}
	return j.Code
}

// MaxFactsRunes caps free-text facts so one request can't send an unbounded
// prompt. The research harness truncated facts at 12,000 characters.
const MaxFactsRunes = 12000

// CanonicalFacts is the one form of free-text facts that is predicted, shown,
// stored and re-submitted: surrounding whitespace removed and exactly one
// terminal newline, as predict feeds the engine (D20). The limit is on the
// text itself, so the terminal newline is not counted. ok is false when the
// text is empty or longer than MaxFactsRunes runes; callers must reject, never
// truncate. Applying it to its own result changes nothing.
func CanonicalFacts(s string) (facts string, runes int, ok bool) {
	t := strings.TrimSpace(s)
	runes = utf8.RuneCountInString(t)
	if t == "" || runes > MaxFactsRunes {
		return "", runes, false
	}
	return t + "\n", runes, true
}

// MinFactsRunes is the shortest free-text description accepted without a
// structured product: the Step 1 form asks for more than 20 characters.
const MinFactsRunes = 20

// Validate checks the case has a date and either facts or a valid product.
func (c Case) Validate() error {
	var errs problems
	if c.Date.IsZero() {
		errs.add("date is required (it bounds which earlier rulings may be cited)")
	}
	if strings.TrimSpace(c.Facts) == "" && c.Product == nil {
		errs.add("facts or product is required")
	} else if c.Product == nil && len([]rune(strings.TrimSpace(c.Facts))) < MinFactsRunes {
		// A word or two can't describe a product; refuse before any model
		// call, as the Step 1 form does (#184).
		errs.add("facts is too short to describe a product; write at least %d characters", MinFactsRunes)
	}
	// The date bounds which rulings may be cited, so a future date would mean
	// "every ruling". A day of slack covers time zones (#184).
	if !c.Date.IsZero() && c.Date.After(time.Now().AddDate(0, 0, 1)) {
		errs.add("date %s is in the future; use the case date (today or earlier)", c.Date.Format("2006-01-02"))
	}
	// The terminal newline of canonical facts is not part of the limit.
	if n := utf8.RuneCountInString(strings.TrimSuffix(c.Facts, "\n")); n > MaxFactsRunes {
		errs.add("facts is %d characters; max %d", n, MaxFactsRunes)
	}
	if c.Product != nil {
		if err := c.Product.Validate(); err != nil {
			errs = append(errs, err.(*ValidationError).Problems...)
		}
	}
	return errs.err()
}
