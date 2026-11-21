package corpus

import (
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

func detect(id, subject, text string) []domain.Supersession {
	return DetectSupersession(SupersessionInput{ID: id, Date: time.Date(2025, 12, 16, 0, 0, 0, 0, time.UTC), Subject: subject, Text: text},
		func(string) (time.Time, bool) { return time.Time{}, false })
}

func edges(es []domain.Supersession) map[string]string {
	m := map[string]string{}
	for _, e := range es {
		m[e.Ruling] = e.Kind + "/" + e.Source + "/" + e.By + "/" + e.Date
	}
	return m
}

func TestDetectSupersessionReadsEffectOnOtherRulings(t *testing.T) {
	text := "HOLDING: The country of origin is China, as in NY N111111. Pursuant to 19 U.S.C. § 1625(c)(1), notice proposing to modify NY N322364 and revoke NY N322681 was published on October 12, 2022.\r\r" +
		"EFFECT ON OTHER RULINGS:\rNY N322364, dated November 18, 2021, is hereby MODIFIED. NY N322681, dated December 1, 2021, is hereby REVOKED. " +
		"In accordance with 19 U.S.C. § 1625(c), this ruling will become effective 60 days after its publication in the Customs Bulletin. Sincerely,"
	got := edges(detect("H323218", "Country of Origin of Air Purifiers", text))
	want := map[string]string{
		"N322364": "modified/effect_on_other_rulings/H323218/2025-12-16",
		"N322681": "revoked/effect_on_other_rulings/H323218/2025-12-16",
	}
	if len(got) != len(want) || got["N322364"] != want["N322364"] || got["N322681"] != want["N322681"] {
		t.Errorf("got %v, want %v (N111111 is a citation, the notice is a proposal)", got, want)
	}
}

func TestDetectSupersessionAbbreviatedDatesAndOldNumbers(t *testing.T) {
	text := "EFFECT ON OTHER RULINGS: New York Ruling Letter (NY) N261615, dated Mar. 11, 2015, is hereby modified. Sincerely," +
		" EFFECT ON OTHER RULINGS: HQ 963233, dated December 13, 2000, is hereby MODIFIED with respect to marking."
	got := edges(detect("H263571", "", text))
	if got["N261615"] == "" || got["963233"] == "" || len(got) != 2 {
		t.Errorf("got %v", got)
	}
}

func TestDetectSupersessionActiveVoiceAndSubjectFallback(t *testing.T) {
	got := edges(detect("H305371", "", "For these reasons, we hereby modify NY N303338. This ruling is effective."))
	if got["N303338"] != "modified/hereby/H305371/2025-12-16" {
		t.Errorf("active voice: %v", got)
	}
	// Subject lines count only when the text names nothing.
	got = edges(detect("H346161", "Revocation of NY N345891; Tariff Classification and Country of Origin of a Pillow Cover", ""))
	if got["N345891"] != "revoked/subject/H346161/2025-12-16" || len(got) != 1 {
		t.Errorf("subject: %v", got)
	}
	got = edges(detect("H072375", "Modification of New York Ruling Letter N057699, dated May 15, 2009; classification of filler paper", "no hereby sentence"))
	if got["N057699"] == "" || len(got) != 1 {
		t.Errorf("subject with a date: %v", got)
	}
}

func TestDetectSupersessionIgnoresBoilerplateAndAffirmations(t *testing.T) {
	text := "This ruling is being issued under 19 C.F.R. Part 177. A copy of the ruling may be revoked or modified at any time. " +
		"We affirm NY N284256. NY N284256 is hereby AFFIRMED."
	if got := detect("H302244", "Affirmation of NY N284256; country of origin of multilayer wood flooring", text); len(got) != 0 {
		t.Errorf("got %v", got)
	}
}

func TestDetectSupersessionReference(t *testing.T) {
	text := "However, please note that HQ H300226 was modified by HQ H301619, dated November 6, 2018, which amended the language."
	got := detect("H302244", "Affirmation of NY N284256", text)
	if len(got) != 1 || got[0].Ruling != "H300226" || got[0].By != "H301619" || got[0].Date != "2018-11-06" || got[0].Source != "reference" || got[0].SourceRuling != "H302244" {
		t.Errorf("got %+v", got)
	}
}
