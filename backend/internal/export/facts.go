package export

import "strings"

// requestBoilerplate are phrases of CBP's reply to a ruling request ("In your
// letter dated…, you requested…", "With regard to your request for…,"). A
// case's facts are often pasted from a CBP ruling, so they carry CBP's voice;
// a request letter from the requester must not (D180).
var requestBoilerplate = []string{
	"in your letter dated",
	"in your letter of",
	"this is in response to your letter",
	"this is in reply to your letter",
	"this is in response to your request",
	"this is in reply to your request",
	"you requested a",
	"you have requested a",
	"your request was",
	"with regard to your request",
	"in regard to your request",
	"with respect to your request",
	"in response to your request",
}

// CleanFacts drops the sentences of facts that are CBP's reply boilerplate
// rather than facts about the goods, and a dangling last fragment that ends
// in a comma (where the ruling text was cut before CBP's analysis). Every
// other sentence is kept word for word, in order.
func CleanFacts(facts string) string {
	var paras []string
	for _, p := range strings.Split(strings.TrimSpace(facts), "\n\n") {
		var keep []string
		for _, s := range sentences(p) {
			l := strings.ToLower(strings.TrimSpace(s))
			if l == "" || boilerplate(l) {
				continue
			}
			keep = append(keep, s)
		}
		if t := strings.TrimSpace(strings.Join(keep, "")); t != "" {
			paras = append(paras, t)
		}
	}
	if n := len(paras); n > 0 && strings.HasSuffix(paras[n-1], ",") {
		paras = paras[:n-1]
	}
	if len(paras) == 0 {
		return strings.TrimSpace(facts) // never empty a case's facts
	}
	return strings.Join(paras, "\n\n")
}

func boilerplate(l string) bool {
	for _, b := range requestBoilerplate {
		if strings.Contains(l, b) {
			return true
		}
	}
	return false
}

// sentences splits a paragraph after ". ", keeping each sentence's own text
// and trailing space, so joining them gives the paragraph back.
func sentences(p string) []string {
	return strings.SplitAfter(p, ". ")
}
