package tariffwatch

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/duty"
)

// Report says what Revise did and what it left alone.
type Report struct {
	// Applied lists each change made to the data set.
	Applied []string `json:"applied"`
	// NotModelled lists the headings whose text changed between the two
	// revisions but whose duty the duty table does not model (the IEEPA and
	// Section 232 headings, carve-outs with no additive rate): a case priced
	// on them is not re-checked for the change.
	NotModelled []string `json:"not_modelled"`
	Warnings    []string `json:"warnings,omitempty"`
}

// section301 reports whether a heading is one of the China Section 301
// headings of subchapter III the duty table models: 9903.88 (the 2018 and
// 2019 actions and their exclusions) and 9903.91 (the maritime action).
func section301(num string) bool {
	return strings.HasPrefix(num, "9903.88.") || strings.HasPrefix(num, "9903.91.")
}

// Revise makes the data set of a newer HTS revision from the data set of an
// older one and the text of the two revisions' Chapter 99 PDFs (the
// "HTS revision loader"). oldText may be empty: the exclusion windows are
// then not refreshed, and every heading in newText counts as new.
//
// Two things are carried over, both read from the headings themselves:
//
//   - A Section 301 exclusion heading ("each covered by an exclusion granted
//     by the U.S. Trade Representative") whose last entry day moved: every
//     stored exclusion that ended on the old day now ends on the new one.
//     When two exclusion headings moved the same old day to different new
//     days, nothing is applied and a warning says so.
//   - A China Section 301 heading (9903.88 and 9903.91 are China actions by
//     definition; a heading need not say "China") with an "on or after" day,
//     a "+ N%" rate and the tariff lines it names: a dated action. The same heading in the
//     older data set is updated, so a suspension that moves the start day is
//     carried over.
//
// Anything else that changed is listed in the report, not applied.
func Revise(base *Dataset, oldText, newText string) (*Dataset, Report, error) {
	var rep Report
	rev := RevisionName(newText)
	if rev == "" {
		return nil, rep, fmt.Errorf("tariffwatch: no revision header (\"Revision N (YYYY)\") in the new text")
	}
	out := *base
	out.Revision = rev
	out.Additional.Exclusions = append([]duty.Exclusion(nil), base.Additional.Exclusions...)
	out.Actions = append([]Action(nil), base.Actions...)
	out.MFN = append([]MFNLine(nil), base.MFN...)
	out.Additional.Revision = rev

	oldH := map[string]Heading{}
	for _, h := range ParseHeadings(oldText) {
		oldH[h.Number] = h
	}
	newH := ParseHeadings(newText)

	// Exclusion windows.
	moves := map[string]map[string]bool{} // old end day -> new end days
	haveOld := oldText != ""
	for _, h := range newH {
		if !h.IsExclusion() {
			continue
		}
		o, ok := oldH[h.Number]
		if !ok || !o.IsExclusion() || o.EndDate() == "" || h.EndDate() == "" || o.EndDate() == h.EndDate() {
			continue
		}
		if moves[o.EndDate()] == nil {
			moves[o.EndDate()] = map[string]bool{}
		}
		moves[o.EndDate()][h.EndDate()] = true
	}
	oldDays := make([]string, 0, len(moves))
	for d := range moves {
		oldDays = append(oldDays, d)
	}
	sort.Strings(oldDays)
	for _, od := range oldDays {
		if len(moves[od]) != 1 {
			rep.Warnings = append(rep.Warnings, fmt.Sprintf("exclusion headings moved the end day %s to different days; the stored exclusions cannot be told apart, so none was changed", od))
			continue
		}
		var nd string
		for d := range moves[od] {
			nd = d
		}
		n := 0
		for i := range out.Additional.Exclusions {
			if out.Additional.Exclusions[i].EndDate == od {
				out.Additional.Exclusions[i].EndDate = nd
				n++
			}
		}
		rep.Applied = append(rep.Applied, fmt.Sprintf("%d exclusions: end day %s -> %s", n, od, nd))
	}
	if !haveOld {
		rep.Warnings = append(rep.Warnings, "no older text given: exclusion windows were not refreshed")
	}

	// Dated actions.
	byHeading := map[string]int{}
	for i, a := range out.Actions {
		byHeading[a.Heading] = i
	}
	for _, h := range newH {
		if o, ok := oldH[h.Number]; ok && o.Description == h.Description && o.General == h.General {
			continue // unchanged text
		}
		if h.IsExclusion() && section301(h.Number) {
			continue // handled above
		}
		rate, hasRate := h.AddRate()
		subs, start := h.Subheadings(), h.StartDate()
		if !(section301(h.Number) && hasRate && start != "" && len(subs) > 0) {
			rep.NotModelled = append(rep.NotModelled, h.Number)
			continue
		}
		a := Action{Kind: KindSection301, Heading: h.Number, Name: "Section 301 China, heading " + h.Number, Origin: "China", Rate: rate,
			EffectiveFrom: start, EffectiveThrough: h.EndDate(), Subheadings: subs,
			Note: "dated Chapter 99 action read from " + rev}
		if i, ok := byHeading[h.Number]; ok {
			prev := out.Actions[i]
			if prev.EffectiveFrom == a.EffectiveFrom && prev.EffectiveThrough == a.EffectiveThrough && prev.Rate == a.Rate && strings.Join(prev.Subheadings, ",") == strings.Join(a.Subheadings, ",") {
				continue
			}
			out.Actions[i] = a
			rep.Applied = append(rep.Applied, fmt.Sprintf("%s: start day %s -> %s, rate %s", h.Number, prev.EffectiveFrom, a.EffectiveFrom, pct(a.Rate)))
			continue
		}
		byHeading[h.Number] = len(out.Actions)
		out.Actions = append(out.Actions, a)
		rep.Applied = append(rep.Applied, fmt.Sprintf("%s: new action, +%s on %d lines from %s", h.Number, pct(a.Rate), len(subs), start))
	}
	sort.Strings(rep.NotModelled)
	if err := out.Validate(); err != nil {
		return nil, rep, err
	}
	return &out, rep, nil
}
