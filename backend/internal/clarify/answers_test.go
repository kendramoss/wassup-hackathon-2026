package clarify

import (
	"strings"
	"testing"
)

func TestAppendAnswersDropsHedgesAndUnknowns(t *testing.T) {
	out, ok := AppendAnswers("Facts.\n", []Answer{
		{Question: "Is it pre-blended?", Answer: "We blend everything in Australia. Whether any ingredient is pre-blended is not explicitly stated."},
		{Question: "Resin source?", Answer: "Unknown"},
		{Question: "Value share?", Answer: "Not stated."},
	})
	if !ok || !strings.Contains(out, "A: We blend everything in Australia.\n") || strings.Contains(out, "not stated") ||
		strings.Contains(out, "Resin source") || strings.Contains(out, "Value share") || !strings.Contains(out, "not evidence for or against") {
		t.Errorf("AppendAnswers =\n%s", out)
	}
	if out, ok := AppendAnswers("Facts.\n", []Answer{{Question: "q", Answer: "I don't know."}}); ok || out != "Facts.\n" {
		t.Errorf("all-unknown answers changed facts: %q %v", out, ok)
	}
}

func TestFactualSplitsMixedClauses(t *testing.T) {
	for _, c := range []struct{ name, in, want string }{
		{"but", "The motor is made in Thailand, but its value share is unknown.", "The motor is made in Thailand."},
		{"but no comma", "The motor is made in Thailand but its value share is unknown.", "The motor is made in Thailand."},
		{"semicolon", "The motor is made in Thailand; the firmware origin is not stated.", "The motor is made in Thailand."},
		{"and", "The housing is moulded in Vietnam and the resin source is unknown.", "The housing is moulded in Vietnam."},
		{"unknown first", "The firmware origin is unknown, but the motor is made in Thailand.", "The motor is made in Thailand."},
		{"keeps both sides", "The motor is made in Thailand, the value share is unknown, and the housing is cast in Vietnam.", "The motor is made in Thailand. The housing is cast in Vietnam."},
		{"two sentences", "The motor is made in Thailand, but its share is unknown. We assemble it in Mexico.", "The motor is made in Thailand. We assemble it in Mexico."},
		{"negation kept", "The motor is not made in China, but the firmware origin is unknown.", "The motor is not made in China."},
		{"known qualifier", "The motor is from a well-known Thai supplier, but its price is unknown.", "The motor is from a well-known Thai supplier."},
		{"terminal punctuation kept", "The motor is made in Thailand but its price is unknown!", "The motor is made in Thailand!"},
		{"question sentence is its own sentence", "Is it new? The motor is made in Thailand but its price is unknown.", "Is it new? The motor is made in Thailand."},
	} {
		if got := factual(c.in); got != c.want {
			t.Errorf("%s: factual(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

func TestFactualNeverKeepsUnknownClauses(t *testing.T) {
	for _, in := range []string{
		"Unknown",
		"Not stated.",
		"I don't know.",
		"The origin of the firmware is unknown.",
		"The motor origin and the firmware origin are unknown.",
		"The motor's origin, the firmware origin and the value share are not known.",
		"The motor is unknown, but the value share is not stated.",
		"Not sure, and no idea.",
		"I do not know which country the motor is from.",
		"The motor, which is made somewhere that is unclear, ships monthly.",
		"The motor is made in a country that is not stated.",
		"Whether it is pre-blended is not explicitly stated.",
		"N/A",
	} {
		if got := factual(in); got != "" {
			t.Errorf("factual(%q) = %q, want nothing", in, got)
		}
	}
}

func TestAppendAnswersMixedAcrossQuestions(t *testing.T) {
	out, ok := AppendAnswers("Facts.\n", []Answer{
		{Question: "Where is the motor made?", Answer: "The motor is made in Thailand, but its value share is unknown."},
		{Question: "Firmware?", Answer: "Unknown, but I think it is flashed in-house."}, // qualified: dropped whole
		{Question: "Resin?", Answer: "Not stated."},
	})
	if !ok || !strings.Contains(out, "A: The motor is made in Thailand.\n") || strings.Contains(out, "value share") ||
		strings.Contains(out, "Resin") || strings.Contains(out, "Firmware?") || strings.Contains(out, "flashed") {
		t.Errorf("AppendAnswers =\n%s", out)
	}
	if _, ok := AppendAnswers("Facts.\n", []Answer{{Question: "a", Answer: "Unknown."}, {Question: "b", Answer: "The share is not stated, and the origin is unclear."}}); ok {
		t.Errorf("all-unknown mixed-delimiter answers were accepted")
	}
}

func TestFactualBareAndNeedsClauseOnBothSides(t *testing.T) {
	for _, in := range []string{
		"The motor is made in Thailand and the firmware origin and the share are unknown.",
		"The motor, the firmware and the share are unknown.",
	} {
		if got := factual(in); strings.Contains(got, "firmware") || strings.Contains(got, "share") {
			t.Errorf("factual(%q) = %q kept an unknown subject", in, got)
		}
	}
}

// Splitting must never turn a qualified, conditional or referential claim into
// a plain fact: such sentences are dropped whole (the conservative side).
func TestFactualDoesNotPromoteQualifiedClauses(t *testing.T) {
	for _, in := range []string{
		"The motor is made in Thailand, though I am not sure, and the housing is unknown.",
		"If the motor is made in Thailand, the share is unknown, and the housing is cast in Vietnam.",
		"The motor was made in Thailand, unless it is unknown, and packed in Vietnam.",
		"The motor is probably made in Thailand, but the share is unknown.",
		"I think the motor is made in Thailand, but the share is unknown.",
		"The motor is made in Thailand, but that is unknown.",
		"The motor is made in Thailand, but it is not stated.",
		"The motor is made in Thailand only when the share is unknown.",
		"Whether the motor is made in Thailand, the share is unknown.",
		"The motor is made in Thailand, but its share is not unknown.",
	} {
		if got := factual(in); got != "" {
			t.Errorf("factual(%q) = %q, want it dropped whole", in, got)
		}
	}
}

func TestFactualMixedPositiveControlsKeepNegation(t *testing.T) {
	for in, want := range map[string]string{
		"The motor is not made in China, and the firmware origin is unknown.":         "The motor is not made in China.",
		"The firmware origin is unknown, but the motor is never imported from China.": "The motor is never imported from China.",
	} {
		if got := factual(in); got != want {
			t.Errorf("factual(%q) = %q, want %q", in, got, want)
		}
	}
}

// An unknown piece without its own subject qualifies the claim before it.
func TestFactualEllipticalUnknownDropsWhole(t *testing.T) {
	for _, in := range elliptical {
		if got := factual(in); got != "" {
			t.Errorf("factual(%q) = %q, want it dropped whole", in, got)
		}
	}
	if got := factual("The motor is made in Thailand but its share is unknown."); got != "The motor is made in Thailand." {
		t.Errorf("positive control = %q", got)
	}
}

var elliptical = []string{
	"The motor is made in Thailand, but unknown.",
	"The motor is made in Thailand, though unknown.",
	"The motor is made in Thailand; unclear.",
	"The motor is made in Thailand, but not stated.",
	"The motor is made in Thailand, although unclear, and the housing is cast in Vietnam.",
	"The motor is made in Thailand, but not stated where it is made.",
	"The motor is made in Thailand, but unclear which country it is.",
	"The motor is made in Thailand, but unknown who is the maker.",
	"The motor is made in Thailand, but which country it is is unclear.",
	"The motor is made in Thailand, but where it is made is not stated.",
	"The motor is made in Thailand, but what the maker is is unknown.",
}

func TestFactualIndependentUnknownSubjectStillSplits(t *testing.T) {
	for _, in := range []string{
		"The motor is made in Thailand, but its share is unknown.",
		"The motor is made in Thailand, but the maker of the firmware is not stated.",
		"The motor is made in Thailand; the housing origin is unclear.",
	} {
		if got := factual(in); got != "The motor is made in Thailand." {
			t.Errorf("factual(%q) = %q", in, got)
		}
	}
}
