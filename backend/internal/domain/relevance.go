package domain

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

// Relevance says whether a change to a ruling bears on what the case relies
// on it for (D531, D561): the answer for a cited ruling, a plan's target for
// a plan's supporting ruling, CBP's holding on the current setup for the
// current setup's ruling. It is worked out from the origin the changed ruling held before
// the change and the origin the changing ruling holds, both from the
// shipped labels (ruling_holdings.json), and the case's predicted origin.
// It never changes the answer and never removes the alert.
const (
	// RelevanceTouchesAnswer: the holding moved, and the answer's country is
	// the one it held before or the one it holds now.
	RelevanceTouchesAnswer = "touches_answer"
	// RelevanceNotAnswer: the holding moved between two countries, neither
	// of them the answer.
	RelevanceNotAnswer = "not_answer"
	// RelevanceHoldingUnchanged: the same origin before and after; the
	// change was to something else (classification or scope, for example).
	RelevanceHoldingUnchanged = "holding_unchanged"
	// RelevanceUnknown: the origin before or after, or the answer, is not
	// known.
	RelevanceUnknown = "unknown"
)

// RulingHoldings is the embedded map of held origins for the rulings in the
// supersession map, built from the shipped labels by
// `go run ./cmd/corpus holdings` (in make reproduce).
type RulingHoldings struct {
	Note string `json:"note"`
	// Holdings maps a ruling to the single origin its label gives, or ""
	// when the label gives none (several countries, or no origin holding).
	// A ruling with no label is absent.
	Holdings map[string]string `json:"holdings"`
}

//go:embed ruling_holdings.json
var rulingHoldingsJSON []byte

var rulingHoldings = func() map[string]string {
	var h RulingHoldings
	if err := json.Unmarshal(rulingHoldingsJSON, &h); err != nil {
		panic(err)
	}
	return h.Holdings
}()

// HeldOrigin is the origin a ruling in the supersession map held, from its
// label; "" when it is not known.
func HeldOrigin(id string) string { return rulingHoldings[id] }

// RelevanceBasis is what a relied-on ruling is judged against, by the role
// it plays (#626, D561): a cited (or only read) ruling against the case's
// predicted origin; a plan's supporting ruling against the target of the
// plan(s) it supports; the current setup's CBP ruling against the origin CBP
// held there.
type RelevanceBasis struct {
	// Predicted is the case's predicted origin.
	Predicted string
	// PlanTargets maps a supporting ruling to the targets of the shown plans
	// it supports.
	PlanTargets map[string][]string
	// CurrentSetup is CBP's holding on the current setup (the user's
	// CurrentOrigin.Country), "" when not known.
	CurrentSetup string
}

// WithRelevance fills each alert's held origins, the case's answer and the
// relevance label with its sentence, judging every alert against the case's
// predicted origin. Use WithRelevanceBy when the case has plans or a current
// setup.
func WithRelevance(as []RulingAlert, answer string) []RulingAlert {
	return WithRelevanceBy(as, RelevanceBasis{Predicted: answer})
}

// WithRelevanceBy fills each alert's held origins, the case's answer and the
// relevance label with its sentence. Each role the ruling plays is judged
// against what that role relies on it for; where it plays several, the most
// relevant label is kept (bears on it, then not known, then does not touch
// it) and the note names the role (RelevanceRole).
func WithRelevanceBy(as []RulingAlert, b RelevanceBasis) []RulingAlert {
	ans := NormalizeCountry(b.Predicted)
	for i := range as {
		a := &as[i]
		a.HeldBefore, a.HeldAfter, a.Answer = HeldOrigin(a.Ruling), HeldOrigin(a.By), ans
		best := -1
		for _, j := range judgements(*a, b) {
			rel, note := relevance(*a, j)
			if r := relevanceRank(rel); r > best {
				best, a.Relevance, a.RelevanceNote, a.RelevanceRole = r, rel, note, j.role
			}
		}
	}
	return as
}

// judgement is one role's basis: what the ruling is relied on for.
type judgement struct {
	role    string
	answers []string // countries the role rests on
}

func judgements(a RulingAlert, b RelevanceBasis) []judgement {
	var js []judgement
	seen := map[string]bool{}
	for _, r := range a.Roles {
		if seen[r] {
			continue
		}
		seen[r] = true
		switch r {
		case RolePlanSupport:
			var ts []string
			for _, t := range b.PlanTargets[a.Ruling] {
				if t = NormalizeCountry(t); t != "" && !contains(ts, t) {
					ts = append(ts, t)
				}
			}
			js = append(js, judgement{RolePlanSupport, ts})
		case RoleCurrentSetup:
			js = append(js, judgement{RoleCurrentSetup, nonEmpty(NormalizeCountry(b.CurrentSetup))})
		case RoleCited:
			js = append(js, judgement{RoleCited, nonEmpty(NormalizeCountry(b.Predicted))})
		}
	}
	if len(js) == 0 { // only read: compared with the answer, as before
		js = append(js, judgement{RoleRead, nonEmpty(NormalizeCountry(b.Predicted))})
	}
	return js
}

func nonEmpty(s string) []string {
	if s == "" {
		return nil
	}
	return []string{s}
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

// relevanceRank orders labels for a ruling with several roles: a change that
// bears on any of them is reported as bearing; one whose bearing is not known
// for some role is never reported as not touching.
func relevanceRank(rel string) int {
	switch rel {
	case RelevanceTouchesAnswer:
		return 3
	case RelevanceUnknown:
		return 2
	case RelevanceNotAnswer:
		return 1
	}
	return 0
}

func relevance(a RulingAlert, j judgement) (string, string) {
	b, f := a.HeldBefore, a.HeldAfter
	what := fmt.Sprintf("%s %s %s", a.By, a.Kind, a.Ruling) // "H344999 modified N344694"
	if a.Status == AlertProposed {
		what = fmt.Sprintf("CBP proposed (%s) that %s be %s", a.By, a.Ruling, a.Kind)
	}
	moved := fmt.Sprintf("%s, and the origin held moved from %s to %s.", what, b, f)
	switch {
	case b == "" || f == "":
		known := ""
		switch {
		case b != "":
			known = fmt.Sprintf(" %s held %s, but the origin %s holds is not in the labels.", a.Ruling, b, a.By)
		case f != "":
			known = fmt.Sprintf(" %s holds %s, but the origin %s held is not in the labels.", a.By, f, a.Ruling)
		default:
			known = fmt.Sprintf(" The origins %s and %s hold are not in the labels.", a.Ruling, a.By)
		}
		return RelevanceUnknown, "Whether this change bears on the answer is not known." + known
	case b == f:
		return RelevanceHoldingUnchanged, fmt.Sprintf("By the labels, %s held %s and %s holds %s too, so the origin held did not change; the change is to something else, such as classification or scope.", a.Ruling, b, a.By, f)
	}
	switch j.role {
	case RolePlanSupport:
		if len(j.answers) == 0 {
			return RelevanceUnknown, moved + " A shown plan rests on it, and its target is not known."
		}
		plan := "The plan to " + strings.Join(j.answers, " and the plan to ") + " rests on it"
		if len(j.answers) > 1 {
			plan = "The plans to " + strings.Join(j.answers, " and ") + " rest on it"
		}
		switch {
		case contains(j.answers, b):
			return RelevanceTouchesAnswer, moved + fmt.Sprintf(" %s, so the change bears on the plan: the ruling no longer holds %s.", plan, b)
		case contains(j.answers, f):
			return RelevanceTouchesAnswer, moved + fmt.Sprintf(" %s, and %s is the new holding, so the change bears on the plan.", plan, f)
		}
		return RelevanceNotAnswer, moved + fmt.Sprintf(" %s, so the change does not touch the plan directly.", plan)
	case RoleCurrentSetup:
		if len(j.answers) == 0 {
			return RelevanceUnknown, moved + " It is CBP's holding on the current setup, and the origin given for that setup is not known."
		}
		c := j.answers[0]
		switch c {
		case b:
			return RelevanceTouchesAnswer, moved + fmt.Sprintf(" It is CBP's holding on the current setup (%s), so the change bears on the current setup.", c)
		case f:
			return RelevanceTouchesAnswer, moved + fmt.Sprintf(" It is CBP's holding on the current setup, given as %s, the new holding, so the change bears on the current setup.", c)
		}
		return RelevanceNotAnswer, moved + fmt.Sprintf(" It is CBP's holding on the current setup, given as %s, so the change does not touch the current setup directly.", c)
	}
	if len(j.answers) == 0 {
		return RelevanceUnknown, moved + " This case has no answer to compare."
	}
	ans := j.answers[0]
	switch ans {
	case b:
		return RelevanceTouchesAnswer, moved + fmt.Sprintf(" This case's answer was %s, so the change bears on the answer.", ans)
	case f:
		return RelevanceTouchesAnswer, moved + fmt.Sprintf(" This case's answer was %s, the new holding, so the change bears on the answer.", ans)
	}
	return RelevanceNotAnswer, moved + fmt.Sprintf(" This case's answer was %s, so the change does not touch the answer directly.", ans)
}
