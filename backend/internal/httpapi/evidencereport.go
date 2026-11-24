package httpapi

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/evreport"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jurisdiction"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/nporigin"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/rcep"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/tariffshift"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/usmca"
)

// EvidenceReportView is the answer of GET /v1/cases/{id}/evidence-report:
// the report and the same report as Markdown.
type EvidenceReportView struct {
	evreport.Report
	Markdown string `json:"markdown"`
}

// evidenceReport answers GET /v1/cases/{id}/evidence-report (#771, D803):
// every check the server serves, run separately on the case's product as
// stored, each in its own section. The U.S. origin is the case's stored
// prediction, never re-run (no model call here). Read only: nothing is
// stored or recorded.
func (a *api) evidenceReport(w http.ResponseWriter, r *http.Request) {
	c, err := a.loadCase(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	rep := a.buildEvidenceReport(c, time.Now().UTC())
	writeJSON(w, http.StatusOK, EvidenceReportView{Report: rep, Markdown: evreport.Markdown(rep)})
}

// buildEvidenceReport runs each check on its own. Order: the case's own
// market's question first is not implied; the order is fixed so reports of
// two cases read the same.
func (a *api) buildEvidenceReport(c store.Case, now time.Time) evreport.Report {
	rep := evreport.Report{
		CaseID: c.ID, Name: c.DisplayName, GeneratedAt: now, Separate: evreport.Separate, NotARuling: evreport.NotARuling,
		Case: evreport.CaseFacts{Date: c.Date, Market: c.Jurisdiction.MarketCode(), Facts: c.Facts, Product: c.Product, Created: c.CreatedAt, Updated: c.UpdatedAt},
	}
	supplied := originFactsOf(c)
	if len(supplied) > 0 {
		rep.Supplied = supplied
	}
	rep.FamilyAnswers = a.familyAnswersOf(c)
	rep.Checks = append(rep.Checks, a.usOriginCheck(c), a.part102Check(c, now))
	if a.d.CanadaMarking != nil {
		rep.Checks = append(rep.Checks, a.canadaCheck(c, now))
	}
	for _, code := range []string{jurisdiction.EU, jurisdiction.UK} {
		if m, rules, _, ok := a.originMarketOf(code); ok {
			rep.Checks = append(rep.Checks, originReportCheck(c, supplied[code], m, rules, now))
		}
	}
	rep.Checks = append(rep.Checks, rcepReportCheck(c, now), usmcaReportCheck(c, now))
	return rep
}

func hasCodes(c store.Case) bool { return c.Product != nil && productHasCodes(*c.Product) }

var addCodes = evreport.Missing{Label: "HS codes for the product and at least one part", Evidence: nporigin.EvidenceFor(nporigin.Need{Kind: nporigin.NeedCode, Key: "materials"})}

// usOriginCheck is the case's stored U.S. answer (CBP substantial
// transformation, a model answer from earlier rulings). It is reused as
// stored; a case for another market has none, and none is made here.
func (a *api) usOriginCheck(c store.Case) evreport.Check {
	ch := evreport.Check{ID: "us_origin", Title: "U.S. country of origin (CBP substantial transformation)",
		Question: "Which country does CBP treat as the good's country of origin, by substantial transformation?",
		Method:   "model", Label: "Model answer from earlier CBP rulings. Not a ruling."}
	p := c.Prediction
	if c.Jurisdiction.MarketCode() != jurisdiction.US {
		ch.Status, ch.Outcome = evreport.NotRun, "Not run: this case is for another market"
		ch.Reason = "The U.S. answer needs a model call. Start a U.S. case with the same product to get it."
		return ch
	}
	if p.Origin == "" {
		ch.Status, ch.Outcome = evreport.NotRun, "No answer stored on the case"
		return ch
	}
	ch.Status, ch.Origin = evreport.Answered, p.Origin
	ch.Outcome = "Likely origin: " + p.Origin
	ch.Reason = firstSentences(p.Reasoning, 2)
	ch.Review = c.Triage.Route != domain.RouteAuto
	if ch.Review {
		ch.Label += " Routed to review."
	}
	ch.At = predictedAt(c)
	ch.Sources = []evreport.Source{{Title: fmt.Sprintf("CBP rulings dated before %s: %d read, %d cited", c.Date, len(p.Precedents), len(p.CitedRulings))}}
	ch.RuleVersion = strings.TrimSpace("model " + p.Trace.ModelID + ", prompt " + p.Trace.PromptVersion)
	ch.Limitations = []string{
		"Reused from the case's stored answer; not re-run for this report.",
		"Reads the facts and the rulings it retrieved; a broker should review it before it is relied on.",
	}
	return ch
}

// predictedAt is when the stored answer was made: the latest predict or
// answers event, else when the case was created.
func predictedAt(c store.Case) time.Time {
	for i := len(c.History) - 1; i >= 0; i-- {
		if k := c.History[i].Kind; k == "predicted" || k == "answered" {
			return c.History[i].At
		}
	}
	return c.CreatedAt
}

func firstSentences(s string, n int) string {
	s = strings.TrimSpace(s)
	out, count := 0, 0
	for i := 0; i+1 < len(s); i++ {
		if (s[i] == '.' || s[i] == '!' || s[i] == '?') && s[i+1] == ' ' {
			count++
			out = i + 1
			if count == n {
				return s[:out]
			}
		}
	}
	return s
}

// part102Check is the deterministic 19 CFR 102.20 tariff-shift check on the
// product's codes: informational beside the U.S. answer.
func (a *api) part102Check(c store.Case, now time.Time) evreport.Check {
	ch := evreport.Check{ID: "us_part102", Title: "U.S. tariff-shift rule (19 CFR 102.20)",
		Question: "Which country does the 19 CFR 102.20 tariff-shift rule point to, from the HTS codes?",
		Method:   "rules", Label: "Rules check from the codes given. Informational beside the U.S. answer. Not a ruling.", At: now}
	if a.d.RuleCheck == nil {
		ch.Status, ch.Outcome = evreport.NotRun, "Not served by this server"
		return ch
	}
	t := a.d.RuleCheck.Table()
	ch.RuleVersion = t.Version
	if ch.RuleVersion == "" {
		ch.RuleVersion = "eCFR text as of " + t.Source.AsOf
	}
	ch.Sources = []evreport.Source{{Title: t.Source.Title, Version: t.Source.AsOf, URL: t.Source.URL}}
	if !hasCodes(c) {
		ch.Status, ch.Outcome, ch.Missing = evreport.NotRun, "Needs HS codes", []evreport.Missing{addCodes}
		return ch
	}
	v := a.ruleCheck(c)
	if v == nil {
		ch.Status, ch.Outcome = evreport.NotRun, "Needs HS codes"
		return ch
	}
	var g *tariffshift.Guidance
	if v.Result.Status == tariffshift.Undetermined {
		g = tariffshift.Guide(a.d.RuleCheck, tariffshiftInput(*c.Product), v.Result)
	}
	tariffshiftInto(&ch, v.Result, g)
	ch.Limitations = []string{"Applies the tariff-shift rule only; it does not change the U.S. answer or what is auto-accepted.", "Reads the codes as given; it does not classify the goods."}
	return ch
}

// canadaCheck is Canada's marking rules (SOR/94-23) on the product's codes,
// with the Canada guidance (D735) as its missing facts, reused as it is.
func (a *api) canadaCheck(c store.Case, now time.Time) evreport.Check {
	ch := evreport.Check{ID: "ca_marking", Title: "Canada marking origin (CUSMA marking rules)",
		Question: "Which country of origin must the good be marked with on import into Canada from the United States or Mexico (SOR/94-23)?",
		Method:   "rules", Label: CanadaLabel, Review: true, At: now}
	t := a.d.CanadaMarking.Table()
	ch.RuleVersion = t.Version
	ch.Sources = []evreport.Source{{Title: t.Source.Title, Version: t.Source.AsOf, URL: t.Source.URL}}
	ch.Limitations = []string{"Marking origin only; not CUSMA preferential origin or tariff treatment.", "Reads the codes as given; it does not classify the goods."}
	if !hasCodes(c) {
		ch.Status, ch.Outcome, ch.Missing = evreport.NotRun, "Needs HS codes", []evreport.Missing{addCodes}
		return ch
	}
	in := tariffshiftInput(*c.Product)
	res := a.d.CanadaMarking.Evaluate(in)
	var g *tariffshift.Guidance
	if res.Status == tariffshift.Undetermined {
		g = tariffshift.Guide(a.d.CanadaMarking, in, res)
	}
	tariffshiftInto(&ch, res, g)
	return ch
}

// tariffshiftInput reads the product like the case's U.S. rule check.
func tariffshiftInput(p domain.Product) tariffshift.Input {
	country, _ := productionCountry(p)
	in := tariffshift.Input{GoodHTS: p.HTS, Country: country}
	for _, comp := range p.Components {
		in.Materials = append(in.Materials, tariffshift.Material{Name: comp.Name, HTS: comp.HTS, Origin: comp.MadeIn})
	}
	return in
}

// tariffshiftInto reads a tariff-shift result, and its guidance when there
// is one, into a check.
func tariffshiftInto(ch *evreport.Check, res tariffshift.Result, g *tariffshift.Guidance) {
	ch.Reason = sentenceOf(res.Reason)
	switch res.Status {
	case tariffshift.Decided:
		ch.Status, ch.Origin, ch.Outcome = evreport.Answered, res.Origin, "Origin: "+res.Origin
		return
	}
	ch.Status, ch.Outcome = evreport.NeedsInformation, "Not decided from the codes given"
	if g == nil {
		return
	}
	for _, it := range g.Missing {
		if it.Kind == tariffshift.KindReview {
			ch.NotModelled = append(ch.NotModelled, it.Label)
			continue
		}
		kind := map[string]string{tariffshift.KindFact: nporigin.NeedFact, tariffshift.KindCode: nporigin.NeedCode, tariffshift.KindShare: nporigin.NeedShare}[it.Kind]
		ch.Missing = append(ch.Missing, evreport.Missing{Label: it.Label, Evidence: nporigin.EvidenceFor(nporigin.Need{Kind: kind, Key: it.Key})})
	}
	if g.Review != nil {
		ch.NotModelled = append(ch.NotModelled, g.Review.Reason)
	}
	if len(ch.Missing) == 0 && len(ch.NotModelled) > 0 {
		ch.Status, ch.Outcome = evreport.NeedsJudgement, "Needs a person's reading of the rule"
	}
}

func sentenceOf(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	s = strings.ToUpper(s[:1]) + s[1:]
	if !strings.HasSuffix(s, ".") {
		s += "."
	}
	return s
}

// originReportCheck is the EU or UK non-preferential origin check on the
// product, with the facts the requester supplied to it (D801).
func originReportCheck(c store.Case, s *domain.SuppliedOriginFacts, m originMarket, rules *nporigin.Rules, now time.Time) evreport.Check {
	ch := evreport.Check{ID: m.code + "_origin", Method: "rules", Label: OriginLabel(m.code), Review: true, At: now}
	switch m.code {
	case jurisdiction.EU:
		ch.Title = "EU non-preferential origin (Annex 22-01)"
		ch.Question = "Which country is the good's non-preferential origin on import into the EU (UCC Art. 60, Annex 22-01)?"
	default:
		ch.Title = "UK non-preferential origin (Part Four)"
		ch.Question = "Which country is the good's non-preferential origin on import into the UK (TCTA 2018 s. 17, Part Four)?"
	}
	v := originViewOf(m, rules)
	ch.RuleVersion = v.TableVersion
	ch.Sources = []evreport.Source{{Title: v.Source.Title, Version: v.Source.Version, URL: v.Source.URL}}
	ch.Limitations = []string{v.Administration, "Reads the codes as given; it does not classify the goods.", "Non-preferential origin only; not a trade-agreement preference."}
	if !hasCodes(c) {
		ch.Status, ch.Outcome, ch.Missing = evreport.NotRun, "Needs HS codes", []evreport.Missing{addCodes}
		return ch
	}
	// Read leniently: answers that no longer fit are left out and said so,
	// never an error (D806).
	got, err := originCheck(m, rules, *c.Product, s, nporigin.Input{}, false)
	if err != nil {
		ch.Status, ch.Outcome, ch.Reason = evreport.NotRun, "The product's codes could not be read", err.Error()
		return ch
	}
	ch.UsesSupplied = got.Supplied != nil && !got.StaleAll
	switch {
	case got.StaleAll:
		ch.Limitations = append(ch.Limitations, "Answers given before the parts list was replaced were not used; answer again.")
	case len(got.Stale) > 0 || len(got.Dropped) > 0:
		ch.Limitations = append(ch.Limitations, "Some answers no longer fit the parts list or the product code and were not used.")
	}
	res := got.Result
	ch.Reason = sentenceOf(res.Reason)
	switch res.Status {
	case nporigin.Decided:
		ch.Status, ch.Origin, ch.Outcome = evreport.Answered, res.Origin, "Origin: "+res.Origin
		if res.Basis != "" {
			ch.Outcome += " (" + res.Basis + ")"
		}
		for _, as := range res.Assumes {
			ch.Limitations = append(ch.Limitations, "Assumes "+as+".")
		}
	case nporigin.NeedsJudgement:
		ch.Status, ch.Outcome = evreport.NeedsJudgement, "Rule not modelled: needs a substantial-transformation judgement"
		ch.NotModelled = []string{strings.TrimSpace(res.Basis + ": " + firstSentences(res.Reason, 1))}
	default:
		ch.Status, ch.Outcome = evreport.NeedsInformation, "Not decided"
		names := partNames(c.Product)
		for _, n := range res.Needs {
			label := needLabel(n, names)
			if n.Kind == nporigin.NeedReview {
				ch.NotModelled = append(ch.NotModelled, strings.TrimSpace(label+" "+oneLineDetail(n.Detail)))
				continue
			}
			ch.Missing = append(ch.Missing, evreport.Missing{Label: label, Evidence: n.Evidence})
		}
		if len(ch.Missing) == 0 && len(ch.NotModelled) > 0 {
			ch.Status, ch.Outcome = evreport.NeedsJudgement, "Needs a person's reading of the rule"
		}
	}
	return ch
}

func partNames(p *domain.Product) []string {
	if p == nil {
		return nil
	}
	out := make([]string, len(p.Components))
	for i, c := range p.Components {
		out[i] = c.Name
	}
	return out
}

// needLabel names the part a need is about when the label does not.
func needLabel(n nporigin.Need, parts []string) string {
	if n.Material == nil || *n.Material >= len(parts) {
		return n.Label
	}
	name := strings.TrimSpace(parts[*n.Material])
	if name == "" || strings.Contains(n.Label, name) {
		return n.Label
	}
	return n.Label + " (" + name + ")"
}

func oneLineDetail(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 200 {
		s = string(r[:197]) + "…"
	}
	return s
}

// rcepReportCheck runs the RCEP preference check on the parts list as it
// stands: the parts' originating status and values are not on the case, so
// a "no" from codes alone is reported as missing information, not as a
// finding that the good fails (Art. 3.5.4 counts unknown parts as
// non-originating).
func rcepReportCheck(c store.Case, now time.Time) evreport.Check {
	ch := evreport.Check{ID: "rcep", Title: "RCEP preference qualification",
		Question: "Does the good qualify as RCEP originating, for the RCEP preferential tariff between RCEP countries?",
		Method:   "rules", Label: "Rules check from the codes given. Not a certificate of origin.", At: now}
	ch.Sources = []evreport.Source{{Title: "RCEP Agreement, Chapter 3 and Annex 3A (product-specific rules)", Version: rcep.Default().Version}}
	ch.RuleVersion = rcep.Default().Version
	ch.Limitations = []string{
		"The case does not record which parts are RCEP originating or their values, so every part is read as not known to be originating (Art. 3.5.4).",
		"Preference qualification only; not the marking or non-preferential origin.",
	}
	if c.Product == nil || len(tariffshift.Digits(c.Product.HTS)) < 4 {
		ch.Status, ch.Outcome, ch.Missing = evreport.NotRun, "Needs the product's HS code", []evreport.Missing{{Label: "The product's HS code", Evidence: nporigin.EvidenceFor(nporigin.Need{Kind: nporigin.NeedCode, Key: "good_code"})}}
		return ch
	}
	country, _ := productionCountry(*c.Product)
	in := rcep.Input{HS: c.Product.HTS, ProducedIn: country}
	for _, comp := range c.Product.Components {
		in.Materials = append(in.Materials, rcep.Material{Name: comp.Name, HS: comp.HTS, Country: comp.MadeIn, Status: rcep.StatusUnknown})
	}
	res, err := rcep.Check(in)
	if err != nil {
		ch.Status, ch.Outcome, ch.Reason = evreport.NotRun, "The product's codes could not be read", err.Error()
		return ch
	}
	if res.Rule != nil {
		ch.Sources[0].Title += ", row " + res.Rule.Code
	}
	ch.Reason = strings.Join(res.Reasons, " ")
	switch res.Outcome {
	case "yes":
		ch.Status, ch.Outcome = evreport.Answered, "Qualifies"
		if res.Applied != "" {
			ch.Outcome += " (" + res.Applied + ")"
		}
	default:
		ch.Status, ch.Outcome = evreport.NeedsInformation, "Not shown to qualify from the parts list alone"
		ch.Missing = []evreport.Missing{
			{Label: "Which parts are RCEP originating", Evidence: "Supplier declarations or RCEP certificates of origin for the parts"},
			{Label: "The parts' values and the good's FOB value", Evidence: "The supplier's bill of materials with values, and the good's FOB price"},
		}
		for _, f := range res.Failing {
			ch.Missing = append(ch.Missing, evreport.Missing{Label: strings.TrimSpace(f.Name + ": " + f.Reason), Evidence: "The part's HS code on the supplier's invoice, and the supplier's declaration or RCEP certificate of origin"})
		}
	}
	return ch
}

// usmcaReportCheck runs the USMCA selected-rule check (pilot goods) on the
// product's codes, with nothing the case does not record: originating
// status, values and the scope confirmations stay unknown.
func usmcaReportCheck(c store.Case, now time.Time) evreport.Check {
	ch := evreport.Check{ID: "usmca", Title: "USMCA preference (pilot goods)",
		Question: "Is the selected USMCA product-specific rule met, for USMCA preference on import into the United States?",
		Method:   "rules", Label: "Rules check of one selected rule. Not a certification of origin.", At: now, Limitations: usmca.Limits()}
	if c.Product == nil {
		ch.Status, ch.Outcome = evreport.NotRun, "Needs the product's HS code"
		return ch
	}
	good := tariffshift.Digits(c.Product.HTS)
	if n := len(good); n > 10 || n < 4 {
		ch.Status, ch.Outcome = evreport.NotRun, "Needs the product's HS code"
		return ch
	} else if n%2 == 1 {
		good = good[:n-1]
	}
	in := usmca.Input{Destination: usmca.DestinationUS, Regime: usmca.RegimeUSMCA, GoodHS: good}
	for _, comp := range c.Product.Components {
		hs := tariffshift.Digits(comp.HTS)
		if len(hs)%2 == 1 {
			hs = hs[:len(hs)-1]
		}
		if len(hs) > 10 {
			hs = hs[:10]
		}
		in.Materials = append(in.Materials, usmca.Material{HS: hs, Originating: usmca.ClaimUnknown})
	}
	if len(in.Materials) > usmca.MaxMaterials {
		in.Materials = in.Materials[:usmca.MaxMaterials]
	}
	out, err := usmca.Check(in)
	if err != nil {
		ch.Status, ch.Outcome, ch.Reason = evreport.NotRun, "The product's codes could not be read", err.Error()
		return ch
	}
	ch.RuleVersion = out.Source.DataVersion
	ch.Sources = []evreport.Source{{Title: out.Source.Snapshot.Source, Version: out.Source.Snapshot.Revision}}
	if out.Source.Row != nil {
		ch.Sources[0].Title += ", row " + out.Source.Row.Code
	}
	switch out.Status {
	case usmca.RuleMet:
		ch.Status, ch.Outcome = evreport.Answered, "Rule met"
	case usmca.RuleNotMet:
		ch.Status, ch.Outcome = evreport.Answered, "Rule not met (as modelled)"
	case usmca.RuleNotSupported:
		ch.Status, ch.Outcome = evreport.NotApplicable, "Not covered by the pilot rules"
		ch.Reason = sentenceOf(strings.Join(out.UnsupportedReasons, "; "))
	default:
		ch.Status, ch.Outcome = evreport.NeedsInformation, "Not decided"
		for _, m := range out.MissingFacts {
			ch.Missing = append(ch.Missing, evreport.Missing{Label: m.Message, Evidence: usmcaEvidence(m)})
		}
	}
	return ch
}

// usmcaEvidence names the document that settles a USMCA missing fact.
func usmcaEvidence(m usmca.MissingFact) string {
	switch {
	case strings.HasSuffix(m.Field, ".originating"):
		return "The supplier's USMCA certification of origin for the material, or a supplier declaration"
	case strings.HasSuffix(m.Field, ".hs"), m.Field == "good_hs":
		return nporigin.EvidenceFor(nporigin.Need{Kind: nporigin.NeedCode, Key: "material_hts"})
	case m.Field == "materials" || m.Field == "profile.materials_list_complete":
		return "The complete bill of materials, each line with its HS code and origin"
	case strings.HasPrefix(m.Field, "profile."):
		return "The producer's statement on how the good is made and what it includes"
	case m.Field == "vnm":
		return "The supplier's bill of materials with the value of each non-originating material"
	case m.Field == "rvc_method", m.Field == "transaction_value", m.Field == "net_cost":
		return "The producer's transaction value or net cost from its accounts"
	}
	return ""
}
