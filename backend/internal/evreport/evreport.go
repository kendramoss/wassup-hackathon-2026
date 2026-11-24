// Package evreport is the evidence report of a case (#771, D803-D805): the
// separate legal checks run on the same product, each as its own section
// with the question it answers, its outcome, what is missing, its sources,
// its rule or data version and its limits, beside the case's facts as
// stored. Checks are never merged into one verdict or score: they answer
// different legal questions.
//
// The package holds the report's shape and its Markdown rendering only; the
// checks are run by package httpapi. No model is called here.
package evreport

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// Statuses of a Check.
const (
	// Answered: the check gave its answer (an origin, or whether a
	// preference rule is met).
	Answered = "answered"
	// NeedsInformation: the check can answer once the listed facts are
	// supplied.
	NeedsInformation = "needs_information"
	// NeedsJudgement: the rule turns on a judgement the check does not
	// model (substantial transformation, a description only a person can
	// read).
	NeedsJudgement = "needs_judgement"
	// NotRun: the check was not run (it needs a model call the user has not
	// asked for, or a code or a rule table is missing).
	NotRun = "not_run"
	// NotApplicable: the check does not cover this good.
	NotApplicable = "not_applicable"
)

// Separate is said at the top of every report.
const Separate = "These checks answer different legal questions. Each stands on its own; they are not combined into one answer or score."

// NotARuling is said at the top of every report.
const NotARuling = "Not a ruling. Confirm with a licensed customs broker or the customs authority."

// Missing is one fact a check needs.
type Missing struct {
	Label string `json:"label"`
	// Evidence names the document or input that would settle it.
	Evidence string `json:"evidence,omitempty"`
}

// Source is one source a check read.
type Source struct {
	Title   string `json:"title"`
	Version string `json:"version,omitempty"`
	URL     string `json:"url,omitempty"`
}

// Check is one legal check, on its own.
type Check struct {
	// ID is stable: us_origin, us_part102, ca_marking, eu_origin, uk_origin,
	// rcep, usmca.
	ID    string `json:"id"`
	Title string `json:"title"`
	// Question is the legal question the check answers.
	Question string `json:"question"`
	// Method is "model" (a model answer from earlier rulings) or "rules" (a
	// deterministic check of published rules).
	Method  string `json:"method"`
	Status  string `json:"status"`
	Outcome string `json:"outcome"`
	Origin  string `json:"origin,omitempty"`
	Reason  string `json:"reason,omitempty"`
	// Label is the check's standing label ("Rules check only — no accuracy
	// figure ...", "Model answer ...").
	Label string `json:"label"`
	// Review: the answer always goes to a person (rules-only markets, a
	// model answer routed to review).
	Review      bool      `json:"review"`
	Missing     []Missing `json:"missing,omitempty"`
	NotModelled []string  `json:"not_modelled,omitempty"`
	Sources     []Source  `json:"sources,omitempty"`
	RuleVersion string    `json:"rule_version,omitempty"`
	Limitations []string  `json:"limitations,omitempty"`
	// UsesSupplied: the check read facts the requester supplied (D801).
	UsesSupplied bool `json:"uses_supplied,omitempty"`
	// At is when this result was made: now for a rules check, the time of
	// the stored prediction for a reused model answer.
	At time.Time `json:"at"`
}

// CaseFacts are the case's facts exactly as stored.
type CaseFacts struct {
	Date    string          `json:"date"`
	Market  string          `json:"market"`
	Facts   string          `json:"facts"`
	Product *domain.Product `json:"product,omitempty"`
	Created time.Time       `json:"created_at"`
	Updated time.Time       `json:"updated_at"`
}

// Report is the evidence report of one case.
type Report struct {
	CaseID      string    `json:"case_id"`
	Name        string    `json:"name,omitempty"`
	GeneratedAt time.Time `json:"generated_at"`
	Separate    string    `json:"separate"`
	NotARuling  string    `json:"not_a_ruling"`
	Case        CaseFacts `json:"case"`
	// Supplied are facts the requester supplied to rules checks, by market,
	// kept apart from the case's facts (D801).
	Supplied map[string]*domain.SuppliedOriginFacts `json:"supplied,omitempty"`
	Checks   []Check                                `json:"checks"`
}

// StatusText is a status in plain words.
func StatusText(s string) string {
	switch s {
	case Answered:
		return "Answered"
	case NeedsInformation:
		return "Missing information"
	case NeedsJudgement:
		return "Rule not modelled: needs a person's judgement"
	case NotRun:
		return "Not run"
	case NotApplicable:
		return "Does not cover this good"
	}
	return s
}

// Markdown renders the report. Each check is its own section; nothing
// combines them.
func Markdown(r Report) string {
	var b strings.Builder
	name := r.Name
	if name == "" && r.Case.Product != nil {
		name = r.Case.Product.Name
	}
	if name == "" {
		name = r.CaseID
	}
	fmt.Fprintf(&b, "# Evidence report: %s\n\n", md(name))
	fmt.Fprintf(&b, "- Case: %s\n- Generated: %s\n- Case created: %s; last updated: %s\n\n", r.CaseID, stamp(r.GeneratedAt), stamp(r.Case.Created), stamp(r.Case.Updated))
	fmt.Fprintf(&b, "> %s\n>\n> %s\n\n", r.Separate, r.NotARuling)

	b.WriteString("## Checks\n\n")
	for _, c := range r.Checks {
		fmt.Fprintf(&b, "- %s: %s\n", c.Title, md(c.Outcome))
	}
	b.WriteString("\n")

	for _, c := range r.Checks {
		fmt.Fprintf(&b, "## %s\n\n", c.Title)
		fmt.Fprintf(&b, "**Question.** %s\n\n", md(c.Question))
		fmt.Fprintf(&b, "**Outcome.** %s (%s)\n\n", md(c.Outcome), StatusText(c.Status))
		if c.Reason != "" {
			fmt.Fprintf(&b, "**Why.** %s\n\n", md(c.Reason))
		}
		if c.Label != "" {
			fmt.Fprintf(&b, "*%s*\n\n", md(c.Label))
		}
		if c.Review {
			b.WriteString("Needs review by a person before it is relied on.\n\n")
		}
		if c.UsesSupplied {
			b.WriteString("Reads facts the requester supplied (listed under Supplied facts below), not only the case's own facts.\n\n")
		}
		if len(c.Missing) > 0 {
			b.WriteString("**Missing information.**\n\n")
			for _, m := range c.Missing {
				if m.Evidence != "" {
					fmt.Fprintf(&b, "- %s — %s\n", md(m.Label), md(m.Evidence))
				} else {
					fmt.Fprintf(&b, "- %s\n", md(m.Label))
				}
			}
			b.WriteString("\n")
		}
		if len(c.NotModelled) > 0 {
			b.WriteString("**Not modelled (a person decides).**\n\n")
			for _, n := range c.NotModelled {
				fmt.Fprintf(&b, "- %s\n", md(n))
			}
			b.WriteString("\n")
		}
		if len(c.Sources) > 0 {
			b.WriteString("**Sources.**\n\n")
			for _, s := range c.Sources {
				line := md(s.Title)
				if v := md(s.Version); v != "" {
					if !strings.HasPrefix(strings.ToLower(v), "version") {
						v = "version " + v
					}
					line += ", " + v
				}
				if s.URL != "" {
					line += " (" + s.URL + ")"
				}
				fmt.Fprintf(&b, "- %s\n", line)
			}
			b.WriteString("\n")
		}
		if c.RuleVersion != "" {
			fmt.Fprintf(&b, "**Rule or data version.** %s\n\n", md(c.RuleVersion))
		}
		if len(c.Limitations) > 0 {
			b.WriteString("**Limitations.**\n\n")
			for _, l := range c.Limitations {
				fmt.Fprintf(&b, "- %s\n", md(l))
			}
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "Result made: %s\n\n", stamp(c.At))
	}

	b.WriteString("## Case facts as stored\n\n")
	fmt.Fprintf(&b, "- Market: %s\n- Case date: %s\n\n", r.Case.Market, r.Case.Date)
	if strings.TrimSpace(r.Case.Facts) != "" {
		b.WriteString("Facts text:\n\n```text\n")
		b.WriteString(strings.ReplaceAll(strings.TrimRight(r.Case.Facts, "\n"), "```", "'''"))
		b.WriteString("\n```\n\n")
	}
	if p := r.Case.Product; p != nil {
		fmt.Fprintf(&b, "Product: %s", md(p.Name))
		if p.HTS != "" {
			fmt.Fprintf(&b, " (HS %s)", md(p.HTS))
		}
		b.WriteString("\n\n")
		if len(p.Components) > 0 {
			b.WriteString("| # | Part | HS code | Made in |\n|---|---|---|---|\n")
			for i, c := range p.Components {
				fmt.Fprintf(&b, "| %d | %s | %s | %s |\n", i+1, cell(c.Name), cell(c.HTS), cell(c.MadeIn))
			}
			b.WriteString("\n")
		}
		if len(p.Plants) > 0 {
			b.WriteString("Plants: ")
			var ps []string
			for _, pl := range p.Plants {
				ps = append(ps, md(pl.ID)+" ("+md(pl.Country)+")")
			}
			b.WriteString(strings.Join(ps, ", ") + "\n\n")
		}
		if len(p.Operations) > 0 {
			b.WriteString("Operations: ")
			var os []string
			for _, o := range p.Operations {
				os = append(os, md(o.Name)+" at "+md(o.Plant))
			}
			b.WriteString(strings.Join(os, "; ") + "\n\n")
		}
	}

	if len(r.Supplied) > 0 {
		b.WriteString("## Supplied facts\n\n")
		b.WriteString("Given by the requester to a rules check; kept apart from the case's facts, which they do not change.\n\n")
		markets := make([]string, 0, len(r.Supplied))
		for m := range r.Supplied {
			markets = append(markets, m)
		}
		sort.Strings(markets)
		for _, m := range markets {
			s := r.Supplied[m]
			if s == nil {
				continue
			}
			fmt.Fprintf(&b, "### %s check\n\n- Source: %s\n- Saved: %s\n", strings.ToUpper(m), s.Source, stamp(s.UpdatedAt))
			if s.GoodHTS != "" {
				fmt.Fprintf(&b, "- Good's code: %s\n", md(s.GoodHTS))
			}
			if s.Row != "" {
				fmt.Fprintf(&b, "- List row: %s\n", md(s.Row))
			}
			keys := make([]string, 0, len(s.Answers))
			for k := range s.Answers {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				fmt.Fprintf(&b, "- Answer %s: %s\n", md(k), yesNo(s.Answers[k]))
			}
			for _, mt := range s.Materials {
				var parts []string
				if mt.HTS != "" {
					parts = append(parts, "HS "+md(mt.HTS))
				}
				if mt.Origin != "" {
					parts = append(parts, "origin "+md(mt.Origin))
				}
				if mt.ValueShare != nil {
					parts = append(parts, "value share "+pct(*mt.ValueShare))
				}
				if mt.WeightShare != nil {
					parts = append(parts, "weight share "+pct(*mt.WeightShare))
				}
				fmt.Fprintf(&b, "- Part %d (%s): %s\n", mt.Index+1, md(mt.Name), strings.Join(parts, ", "))
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

func stamp(t time.Time) string {
	if t.IsZero() {
		return "not recorded"
	}
	return t.UTC().Format("2006-01-02 15:04 UTC")
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func pct(v float64) string {
	return strings.TrimSuffix(strings.TrimRight(fmt.Sprintf("%.2f", v*100), "0"), ".") + "%"
}

// md keeps a value on one line, so a newline in a stored value cannot start
// a new heading or list item.
func md(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// cell keeps a value inside one table cell.
func cell(s string) string {
	s = md(s)
	if s == "" {
		return "—"
	}
	return strings.ReplaceAll(s, "|", "\\|")
}
