package jpengine

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// maxQuestions caps the questions on one result.
const maxQuestions = 5

// YesNoUnsure are the answer buttons of a yes/no question. "Not sure" adds
// no fact when it is sent as an answer.
var YesNoUnsure = []string{"Yes", "No", "Not sure"}

// agreementNames are the agreements as a question names them.
var agreementNames = map[string]string{
	"JP-EU EPA": "the Japan–EU agreement", "AJCEP": "the ASEAN–Japan agreement", "JP-US TA": "the Japan–U.S. agreement",
	"JP-India CEPA": "the Japan–India agreement", "JP-Thailand EPA": "the Japan–Thailand agreement",
	"JP-Indonesia EPA": "the Japan–Indonesia agreement", "JP-Australia EPA": "the Japan–Australia agreement",
	"JP-UK CEPA": "the Japan–UK agreement", "JP-Vietnam EPA": "the Japan–Vietnam agreement",
	"JP-Malaysia EPA": "the Japan–Malaysia agreement", "JP-Mexico EPA": "the Japan–Mexico agreement",
	"JP-Philippines EPA": "the Japan–Philippines agreement", "JP-Switzerland EPA": "the Japan–Switzerland agreement",
	"JP-Chile EPA": "the Japan–Chile agreement",
}

func agreementName(code string) string {
	if n, ok := agreementNames[code]; ok {
		return n
	}
	return code
}

// Questions lists, deterministically, the facts the user can give to settle
// an undetermined Japan result (D917): for each part, where it is made when
// that is blank, else whether it has proof of origin under the agreement;
// the production steps when none are listed. A case with no parts list gets
// one typed question. A question already answered on the case (its text is
// in the facts as "Q: ...") is not asked again. GSP is left out: its answer
// turns on the beneficiary country, not on facts about the parts.
func Questions(c domain.Case, agreement string) []domain.JapanQuestion {
	if agreement == "" || agreement == "GSP" || agreement == "Non-preferential" {
		return nil
	}
	name := agreementName(agreement)
	var out []domain.JapanQuestion
	add := func(id, text string, options []string) {
		if len(out) >= maxQuestions || strings.Contains(c.Facts, "Q: "+text+"\n") {
			return
		}
		out = append(out, domain.JapanQuestion{ID: id, Text: text, Options: options})
	}
	p := c.Product
	if p == nil || len(p.Components) == 0 {
		add("materials", fmt.Sprintf("Which materials have no proof of origin under %s, and what are their tariff codes?", name), nil)
		return out
	}
	for i, part := range p.Components {
		n := partPhrase(cleanName(part.Name, fmt.Sprintf("part %d", i+1)))
		if strings.TrimSpace(part.MadeIn) == "" {
			add(fmt.Sprintf("made_in:%d", i), "Where is "+n+" made?", nil)
			continue
		}
		add(fmt.Sprintf("originating:%d", i), fmt.Sprintf("Does %s have proof of origin under %s?", n, name), YesNoUnsure)
	}
	if len(p.Operations) == 0 {
		add("steps", "What are the production steps, and in which country?", nil)
	}
	return out
}

// partPhrase reads a part name inside a sentence: "Leather upper" becomes
// "the leather upper". A name whose second letter is a capital (an acronym,
// "PCB") keeps its case.
func partPhrase(n string) string {
	r := []rune(n)
	if len(r) > 1 && unicode.IsUpper(r[0]) && !unicode.IsUpper(r[1]) {
		r[0] = unicode.ToLower(r[0])
	}
	return "the " + string(r)
}
