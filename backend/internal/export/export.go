// Package export drafts a CBP ruling request from a case, for a licensed
// customs broker or counsel to review and file. It states facts and cites
// earlier rulings; it never states that CBP will agree.
package export

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// Input is what the draft is built from.
type Input struct {
	ProductName  string
	Facts        string // facts of the setup the request asks about
	Requested    string // the origin the requester believes applies
	PlanSummary  string // e.g. "Make Motor-fan assembly and PCBA (SMT) in Thailand"; empty for the current setup
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
	// Textile asks the 177.2 letter's question under 19 CFR 102.21.
	Textile bool
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
	fmt.Fprintf(&b, "Pursuant to 19 C.F.R. Part 177, [REQUESTER NAME] respectfully requests a binding ruling on the country of origin of %s for purposes of marking and the application of trade remedies, including Section 301 duties.\n\n", name)
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
	b.WriteString(strings.TrimSpace(in.Facts) + "\n\n")
	b.WriteString("## Issue\n\n")
	fmt.Fprintf(&b, "What is the country of origin of %s, produced as described above, under the substantial-transformation test?\n\n", name)
	b.WriteString("## Requester's position\n\n")
	if in.Requested != "" {
		fmt.Fprintf(&b, "The requester submits that the merchandise is a product of %s", in.Requested)
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
		cites = append(cites, fmt.Sprintf("- %s — a comparable change where CBP found the origin moved ([rulings.cbp.gov](%s)).", in.Supporting, domain.RulingURL(in.Supporting)))
	}
	if in.Counter != "" {
		cites = append(cites, fmt.Sprintf("- %s — a similar change that did not alter origin; distinguish it ([rulings.cbp.gov](%s)).", in.Counter, domain.RulingURL(in.Counter)))
	}
	for _, id := range in.Prediction.CitedRulings {
		if id != in.Supporting && id != in.Counter {
			cites = append(cites, fmt.Sprintf("- %s ([rulings.cbp.gov](%s)).", id, domain.RulingURL(id)))
		}
	}
	// Nothing cited (#91): list the rulings the engine read, for the broker
	// to keep or drop, before falling back to a placeholder.
	if len(cites) == 0 {
		for _, id := range in.Prediction.Precedents {
			cites = append(cites, fmt.Sprintf("- %s — read by Unde, not cited; [CONFIRM RELEVANCE] ([rulings.cbp.gov](%s)).", id, domain.RulingURL(id)))
		}
	}
	if len(cites) == 0 {
		cites = append(cites, "- [ADD RELEVANT EARLIER RULINGS]")
	}
	return cites
}

func lowerFirst(s string) string {
	if s == "" || (len(s) > 1 && unicode.IsUpper(rune(s[1]))) { // keep acronyms: PCBA, SMT
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
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
	if in.BelowBar != nil {
		notes = append(notes, fmt.Sprintf("Below the confidence bar: Unde's precedent check rated this change %.2f, under its %.2f bar. Treat the cited support as weaker and confirm with counsel.", in.BelowBar.Confidence, in.BelowBar.Bar))
	}
	return notes
}
