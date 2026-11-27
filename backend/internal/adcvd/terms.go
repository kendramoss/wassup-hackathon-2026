package adcvd

import (
	"strings"
	"unicode"
)

// generic words carry no product meaning in an order's title, a scope or a
// product description, so they never count as a term in common.
var generic = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`a an and are as at be been being by for from has have in including into is it its of on or other
		that the their them these this those thereof to under used use uses using with within without which whether not any all each
		both either such than then there where who whose will would can may might must shall should also only more most less
		certain product products merchandise article articles item items part parts component components subject covered cover covers
		scope order orders china chinese people republic peoples country countries imported import imports importer exported
		united states u s htsus hts harmonized tariff schedule subheading subheadings heading headings number numbers
		type types kind kinds form forms various several one two three four five six seven eight nine ten
		made make makes manufactured manufacture produced produce designed design designs model models
		whether not described description following includes include included excluded exclude excludes excluded
		inch inches mm cm ft foot feet lb lbs pound pounds percent weight width length thickness diameter size sizes`) {
		generic[w] = true
	}
}

// stem makes the plural and a few endings of a word the same term.
func stem(w string) string {
	switch {
	case len(w) > 4 && strings.HasSuffix(w, "ies"):
		return w[:len(w)-3] + "y"
	case len(w) > 4 && strings.HasSuffix(w, "ses"), len(w) > 4 && strings.HasSuffix(w, "xes"), len(w) > 4 && strings.HasSuffix(w, "ches"), len(w) > 4 && strings.HasSuffix(w, "shes"):
		return w[:len(w)-2]
	case len(w) > 3 && strings.HasSuffix(w, "s") && !strings.HasSuffix(w, "ss") && !strings.HasSuffix(w, "us") && !strings.HasSuffix(w, "is"):
		return w[:len(w)-1]
	}
	return w
}

// Terms returns the product terms of a text: lower case words of at least
// three letters, without generic words, plural endings removed. Words with a
// digit (model numbers, codes) are not terms.
func Terms(s string) map[string]bool {
	out := map[string]bool{}
	words := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	for _, w := range words {
		if len(w) < 3 || generic[w] {
			continue
		}
		digit := false
		for _, r := range w {
			if unicode.IsDigit(r) {
				digit = true
				break
			}
		}
		if digit {
			continue
		}
		st := stem(w)
		if generic[st] {
			continue
		}
		out[st] = true
	}
	return out
}

// shared counts the terms two texts have in common.
func shared(a, b map[string]bool) int {
	n := 0
	for w := range a {
		if b[w] {
			n++
		}
	}
	return n
}
