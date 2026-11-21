package bulletin

import (
	"os"
	"strings"
	"testing"
)

// testdata/notices.txt is cut from real notices in the Customs Bulletin
// (public domain): vol. 60 no. 28 p. 30 (its AGENCY label is missing in the
// PDF text), vol. 58 no. 32 p. 9 (a list with one "NY" prefix) and vol. 58
// no. 31 p. 42 (an old HQ letter attached for reference), joined into one
// five-page bulletin with a notice that is not about rulings and the court
// section after it.
func fixture(t *testing.T) []Notice {
	t.Helper()
	b, err := os.ReadFile("testdata/notices.txt")
	if err != nil {
		t.Fatal(err)
	}
	return Extract(Issue{Volume: 60, Number: 28, Date: "2026-09-16", URL: "https://www.cbp.gov/example.pdf"}, string(b))
}

func TestExtractFindsRulingNoticesOnly(t *testing.T) {
	ns := fixture(t)
	if len(ns) != 2 {
		t.Fatalf("want 2 ruling-letter notices (not the Lever-Rule one), got %d", len(ns))
	}
	if !strings.HasPrefix(ns[0].Title, "REVOCATION OF THREE RULING LETTERS") || ns[0].Page != 30 {
		t.Errorf("first notice: title %q, page %d (want the title above the unlabelled agency lines, printed page 30)", ns[0].Title, ns[0].Page)
	}
	if ns[1].Page != 9 {
		t.Errorf("second notice on printed page %d, want 9", ns[1].Page)
	}
	if strings.Contains(ns[1].Excerpt, "N999999") {
		t.Error("the court section after the last notice leaked into its excerpt")
	}
	if strings.Contains(ns[0].Excerpt, "ﬁ") {
		t.Error("ligatures not normalised")
	}
}

func TestParseFinalNoticeUsesTheEffectiveDate(t *testing.T) {
	r := Parse(fixture(t)[0])
	if r.Status != "final" || r.Effective != "2026-11-15" || r.Expected != 3 {
		t.Fatalf("got status %q effective %q expected %d", r.Status, r.Effective, r.Expected)
	}
	got := map[string]string{}
	for _, c := range r.Changes {
		got[c.Ruling] = c.Kind + " by " + c.By + " " + c.Effective
	}
	for _, id := range []string{"N319574", "M83046", "L83440"} {
		if got[id] != "revoked by H336925 2026-11-15" {
			t.Errorf("%s: %q", id, got[id])
		}
	}
	if len(got) != 3 {
		t.Errorf("want 3 rulings (not the style number 060403), got %v", got)
	}
	if p := r.Problem(); p != "" {
		t.Errorf("problem %q", p)
	}
}

func TestParseProposalSkipsAttachedOldLetters(t *testing.T) {
	r := Parse(fixture(t)[1])
	if r.Status != "proposed" || r.CommentsBy != "2024-09-13" {
		t.Fatalf("status %q comments by %q", r.Status, r.CommentsBy)
	}
	got := map[string]string{}
	for _, c := range r.Changes {
		got[c.Ruling] = c.By
	}
	want := []string{"N283085", "N166443", "N025447", "D80822", "H027436"}
	for _, id := range want {
		if got[id] != "H313006" {
			t.Errorf("%s: by %q, want H313006 (numbers listed after one NY prefix count)", id, got[id])
		}
	}
	if _, ok := got["966570"]; ok {
		t.Error("966570 was revoked by the attached old letter H027436 years ago, not by this notice")
	}
	if len(got) != len(want) || r.Problem() != "" {
		t.Errorf("got %v, problem %q", got, r.Problem())
	}
}

func TestProblemFlagsCountAndSubjectMismatches(t *testing.T) {
	head := "TITLE ACTION: Notice of revocation of two ruling letters and of revocation of treatment. SUMMARY: x. EFFECTIVE DATE: This action is effective for merchandise entered on or after July 22, 2024.\n"
	n := Notice{Excerpt: head + "[attachment H304419]\nRE: Revocation of HQ H303773 and NY N299944; lights\nEFFECT ON OTHER RULINGS: HQ H303733 and NY N299944 are hereby revoked\n"}
	r := Parse(n)
	if !strings.Contains(r.Problem(), "H303773") {
		t.Errorf("the subject names H303773, the text H303733: want a problem, got %q", r.Problem())
	}
	n.Excerpt = head + "[attachment H304419]\nEFFECT ON OTHER RULINGS: HQ H303733, HQ H303773 and NY N299944 are hereby revoked\n"
	if p := Parse(n).Problem(); !strings.Contains(p, "names 2") {
		t.Errorf("three rulings for a notice of two: got %q", p)
	}
	// "By operation of law" makes the ACTION count a floor.
	n.Excerpt = head + "[attachment H304419]\nRE: Revocation of HQ H303733 and HQ H303773; Revoked by Operation of Law\nEFFECT ON OTHER RULINGS: HQ H303733, HQ H303773 and NY N299944 are hereby revoked\n"
	if p := Parse(n).Problem(); p != "" {
		t.Errorf("operation of law: got %q", p)
	}
	n.Excerpt = strings.Replace(head, "EFFECTIVE DATE: This action is effective for merchandise entered on or after July 22, 2024.", "", 1) + "[attachment H304419]\nNY N299944 is hereby revoked\n"
	if r := Parse(n); r.Status != "final" || r.Problem() == "" {
		t.Errorf("a final notice without an effective date must be flagged: %q %q", r.Status, r.Problem())
	}
}

func TestResolveKeepsWhatBothReadersName(t *testing.T) {
	n := Notice{Excerpt: "ACTION: Notice of proposed revocation of two ruling letters. DATE: Comments must be received on or before October 6, 2025.\n" +
		"[attachment H272339]\nRE: Revocation of New York Ruling Letter (NY) N019900 and NY N159975; ROVs\n" +
		"EFFECT ON OTHER RULINGS: NY N019900 (dated December 4, 2007) is hereby revoked\n. NY N15997 (dated April 28, 2011) is hereby revoked\n"}
	r := Parse(n)
	if r.Problem() == "" {
		t.Fatal("the subject names N159975, the text N15997: want a problem")
	}
	got := Resolve(n, r, Read{Rulings: []ModelRuling{{"N019900", "revoked"}, {"N159975", "revoked"}, {"N999999", "revoked"}}})
	ids := map[string]string{}
	for _, c := range got {
		ids[c.Ruling] = c.By + " " + c.Status + " " + c.CommentsBy
	}
	if ids["N019900"] != "H272339 proposed 2025-10-06" || ids["N159975"] != "H272339 proposed 2025-10-06" {
		t.Errorf("got %v", ids)
	}
	if _, ok := ids["N15997"]; ok {
		t.Error("N15997: only the regex named it")
	}
	if _, ok := ids["N999999"]; ok {
		t.Error("N999999 is not in the excerpt: a model-only number must appear there")
	}
	if got := Resolve(n, r, Read{Error: "unparseable reply"}); len(got) != 0 {
		t.Errorf("a failed read keeps nothing, got %v", got)
	}
}

func TestParseDate(t *testing.T) {
	for in, want := range map[string]string{"July 22, 2024": "2024-07-22", "Sept. 3, 2025": "2025-09-03", "Dec. 1, 2026": "2026-12-01"} {
		if got, ok := ParseDate(in); !ok || got != want {
			t.Errorf("%q: %q", in, got)
		}
	}
}
