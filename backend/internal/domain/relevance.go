package domain

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

// Relevance says whether a change to a ruling bears on the case's answer
// (D531). It is worked out from the origin the changed ruling held before
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

// WithRelevance fills each alert's held origins, the case's answer and the
// relevance label with its sentence. answer is the case's predicted origin.
func WithRelevance(as []RulingAlert, answer string) []RulingAlert {
	ans := NormalizeCountry(answer)
	for i := range as {
		a := &as[i]
		a.HeldBefore, a.HeldAfter, a.Answer = HeldOrigin(a.Ruling), HeldOrigin(a.By), ans
		a.Relevance, a.RelevanceNote = relevance(*a)
	}
	return as
}

func relevance(a RulingAlert) (string, string) {
	b, f, ans := a.HeldBefore, a.HeldAfter, a.Answer
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
	case ans == "":
		return RelevanceUnknown, moved + " This case has no answer to compare."
	case ans == b:
		return RelevanceTouchesAnswer, moved + fmt.Sprintf(" This case's answer was %s, so the change bears on the answer.", ans)
	case ans == f:
		return RelevanceTouchesAnswer, moved + fmt.Sprintf(" This case's answer was %s, the new holding, so the change bears on the answer.", ans)
	default:
		return RelevanceNotAnswer, moved + fmt.Sprintf(" This case's answer was %s, so the change does not touch the answer directly.", ans)
	}
}
