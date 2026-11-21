// Package export drafts a CBP ruling request from a case, for a licensed
// customs broker or counsel to review and file. It states facts and cites
// earlier rulings; it never states that CBP will agree.
package export

import (
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// Input is what the draft is built from.
type Input struct {
	ProductName  string
	Facts        string // facts of the setup the request asks about
	Requested    string // the origin the requester believes applies
	PlanSummary  string // e.g. "Move the motor-fan assembly and the PCBA (SMT) to Thailand"; empty for the current setup
	Prediction   domain.Prediction
	Supporting   string
	Counter      string
	CurrentRuled string // e.g. "China (N328954)"
	// BelowBar, when set, marks a plan whose precedent check scored under the
	// plan bar; the draft says so near the top.
	BelowBar *BelowBar
	// Product, when the case has one, lists components, plants and
	// operations in the 19 CFR 177.2 letter (with a plan's move applied).
	Product *domain.Product
	// Regime is the legal regime the prediction behind Requested was actually
	// made under, using the router's identifiers (router.RegimeTextile,
	// RegimeUSMCA, RegimePharma). Empty or "substantial_transformation" means
	// the general engine. It must name the engine that produced Prediction,
	// not merely the router's label: a draft must not claim a specialist
	// determination when only a general result exists.
	Regime string
	// Textile is the older flag for the textile regime. It applies only when
	// Regime is empty; Regime always wins. See EffectiveRegime.
	Textile bool
	// Rulings describes the cited rulings the corpus knows (date, subject,
	// CBP's holding), so a citation names the ruling instead of a bare number
	// (D220). A ruling missing here is cited by number alone.
	Rulings map[string]RulingRef
}

// RulingRef is what the draft says about one earlier ruling.
type RulingRef struct {
	Date    string // YYYY-MM-DD
	Subject string // CBP's subject line, e.g. "The country of origin of an air fryer"
	Origin  string // the origin CBP held
}

// rulingName is how the draft names an earlier ruling: "CBP ruling N346397
// (25 March 2025, an air fryer; CBP held Vietnam)", or "CBP ruling N346397"
// when nothing more is known.
func (in Input) rulingName(id string) string {
	name := "CBP ruling " + id
	r, ok := in.Rulings[id]
	if !ok {
		return name
	}
	var bits []string
	if d, err := time.Parse(time.DateOnly, r.Date); err == nil {
		bits = append(bits, d.Format("2 January 2006"))
	}
	if subj := shortSubject(r.Subject); subj != "" {
		bits = append(bits, subj)
	}
	inner := strings.Join(bits, ", ")
	if r.Origin != "" {
		if inner != "" {
			inner += "; "
		}
		inner += "CBP held " + r.Origin
	}
	if inner == "" {
		return name
	}
	return name + " (" + inner + ")"
}

// shortSubject drops CBP's "The country of origin of" opening.
func shortSubject(s string) string {
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "."))
	for _, p := range []string{"The country of origin of ", "the country of origin of ", "Country of origin of "} {
		if strings.HasPrefix(s, p) {
			return strings.TrimSpace(s[len(p):])
		}
	}
	return s
}

// BelowBar is the precedent check's confidence and the bar it missed.
type BelowBar struct {
	Confidence float64
	Bar        float64
}

// Markdown renders the draft.
func Markdown(in Input) string {
	var b strings.Builder
	b.WriteString("U.S. Customs and Border Protection  \nNational Commodity Specialist Division  \nNew York, NY\n\n")
	name := in.ProductName
	if name == "" {
		name = "the subject merchandise"
	}
	fmt.Fprintf(&b, "**RE: Request for a binding country of origin ruling — %s**\n\n", name)
	b.WriteString("Dear Sir or Madam:\n\n")
	rt := in.regimeText()
	fmt.Fprintf(&b, "Pursuant to 19 C.F.R. Part 177, [REQUESTER NAME] respectfully requests a binding ruling on the country of origin of %s %s.\n\n", name, rt.mdPurpose)
	if in.CurrentRuled != "" {
		fmt.Fprintf(&b, "CBP previously ruled on the current production setup: origin %s. ", in.CurrentRuled)
		if in.PlanSummary != "" {
			fmt.Fprintf(&b, "This request concerns a proposed change to that setup: %s.", lowerFirst(in.PlanSummary))
		}
		b.WriteString("\n\n")
	} else if in.PlanSummary != "" {
		fmt.Fprintf(&b, "This request concerns a proposed production setup: %s.\n\n", lowerFirst(in.PlanSummary))
	}
	b.WriteString("## Facts\n\n")
	b.WriteString(CleanFacts(in.Facts) + "\n\n")
	b.WriteString("## Issue\n\n")
	fmt.Fprintf(&b, "What is the country of origin of %s, produced as described above, %s?\n\n", name, rt.mdQuestion)
	if rt.separate != "" {
		b.WriteString(rt.separate + "\n\n")
	}
	b.WriteString("## Requester's position\n\n")
	if in.Requested != "" {
		fmt.Fprintf(&b, "The requester submits that the merchandise is a product of %s%s", in.Requested, rt.positionTail)
	} else {
		b.WriteString("The requester submits that origin should be determined as follows")
	}
	if len(in.Prediction.KeyFactors) > 0 {
		fmt.Fprintf(&b, ", based on these key factors: %s", listPhrase(in.Prediction.KeyFactors))
	}
	b.WriteString(". [REQUESTER TO CONFIRM AND EXPAND WITH COUNSEL.]\n\n")
	b.WriteString("## Earlier CBP rulings considered\n\n")
	b.WriteString(strings.Join(citations(in), "\n") + "\n\n")
	b.WriteString("## Attachments\n\n- Bill of materials with country of origin of each component\n- Process flow and photographs of each manufacturing step\n- [OTHER SUPPORTING DOCUMENTS]\n\n")
	// One line each, as a letter is signed; "  " ends a Markdown line (18 Oct).
	b.WriteString("Sincerely,\n\n[NAME]  \n[TITLE]  \n[COMPANY]  \n[ADDRESS]  \n[CONTACT]\n")
	return b.String()
}

// citations lists the earlier rulings as Markdown bullets: the plan's
// supporting and counter rulings, then the rest the engine cited.
func citations(in Input) []string {
	var cites []string
	if in.Supporting != "" {
		cites = append(cites, fmt.Sprintf("- %s — a comparable change where CBP found the origin moved ([rulings.cbp.gov](%s)).", in.rulingName(in.Supporting), domain.RulingURL(in.Supporting)))
	}
	if in.Counter != "" {
		cites = append(cites, fmt.Sprintf("- %s — a similar change that did not alter origin; distinguish it ([rulings.cbp.gov](%s)).", in.rulingName(in.Counter), domain.RulingURL(in.Counter)))
	}
	for _, id := range in.Prediction.CitedRulings {
		if id != in.Supporting && id != in.Counter {
			cites = append(cites, fmt.Sprintf("- %s ([rulings.cbp.gov](%s)).", in.rulingName(id), domain.RulingURL(id)))
		}
	}
	// Nothing cited (#91): list the rulings the engine read, for the broker
	// to keep or drop, before falling back to a placeholder.
	if len(cites) == 0 {
		for _, id := range in.Prediction.Precedents {
			cites = append(cites, fmt.Sprintf("- %s — read by Unde, not cited; [CONFIRM RELEVANCE] ([rulings.cbp.gov](%s)).", in.rulingName(id), domain.RulingURL(id)))
		}
	}
	if len(cites) == 0 {
		cites = append(cites, "- [ADD RELEVANT EARLIER RULINGS]")
	}
	return cites
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	first, size := utf8.DecodeRuneInString(s)
	if first == utf8.RuneError && size == 1 { // malformed UTF-8: leave it alone
		return s
	}
	rest := s[size:]
	if second, _ := utf8.DecodeRuneInString(rest); second != utf8.RuneError && unicode.IsUpper(second) {
		return s // keep acronyms: PCBA, SMT
	}
	return string(unicode.ToLower(first)) + rest
}

// listPhrase joins key factors as "a, b and c", lowercasing each first letter;
// factors that contain commas are joined with semicolons instead.
func listPhrase(items []string) string {
	sep := ", "
	for _, it := range items {
		if strings.Contains(it, ",") {
			sep = "; "
		}
	}
	xs := make([]string, len(items))
	for i, it := range items {
		xs[i] = lowerFirst(strings.TrimRight(strings.TrimSpace(it), "."))
	}
	if len(xs) == 1 {
		return xs[0]
	}
	last := " and "
	if sep == "; " {
		last = "; and "
	}
	return strings.Join(xs[:len(xs)-1], sep) + last + xs[len(xs)-1]
}

// Notes are what the reviewer should know about a draft: that it is a draft
// for a broker, how the 177.2 letter marks gaps, and a move under the bar.
// They are shown beside the letter, never in it: the letter, its print and
// its download are only what would be filed (18 Oct).
func Notes(in Input, cfr177 bool) []string {
	notes := []string{"Draft for review by a licensed customs broker or counsel. " + domain.Disclaimer +
		" This draft was generated by Unde from the facts below; check every fact and citation before filing."}
	if cfr177 {
		notes = append(notes, "Laid out on 19 C.F.R. § 177.2. Every [FILL IN: …] is something Unde does not know: complete it or delete it before filing.")
	}
	if n := in.regimeText().note; n != "" {
		notes = append(notes, n)
	}
	if in.BelowBar != nil {
		notes = append(notes, fmt.Sprintf("Below the confidence bar: Unde's precedent check rated this change %.2f, under its %.2f bar. Treat the cited support as weaker and confirm with counsel.", in.BelowBar.Confidence, in.BelowBar.Bar))
	}
	return notes
}
