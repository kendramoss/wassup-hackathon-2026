package jpengine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jpcorpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

func avCase() domain.Case {
	return domain.Case{
		Date:  time.Date(2025, 10, 1, 14, 0, 0, 0, time.UTC),
		Japan: &domain.JapanInput{Agreement: "rcep", HS: "1704.90-010"},
		Facts: "Sugar confectionery manufactured in Vietnam under RCEP. Non-originating chocolate and sugar.",
	}
}

func avAnswer(outcome, crit, origin, rule, reasoning string, cited ...string) string {
	if cited == nil {
		cited = []string{}
	}
	b, err := json.Marshal(map[string]any{"outcome": outcome, "criterion": crit, "origin_country": origin, "deciding_rule": rule,
		"confidence": 0.5, "reasoning": reasoning, "cited_rulings": cited})
	if err != nil {
		panic(err)
	}
	return string(b)
}

func avRun(t *testing.T, text string, mut func(*domain.Case)) (engine.Result, error, *llm.Fake) {
	t.Helper()
	m := llm.FakeText(text)
	eng, err := NewDefault(m, "fake", DefaultK)
	if err != nil {
		t.Fatal(err)
	}
	c := avCase()
	if mut != nil {
		mut(&c)
	}
	res, err := eng.Predict(context.Background(), c)
	return res, err, m
}

func TestQualifyingOriginMustBeOneRealCountryOrEU(t *testing.T) {
	bad := []string{"Japan", "japan", " JAPAN ", "Asia", "ASEAN", "Various", "Multiple", "Not applicable", "N/A", "No origin",
		"Unknown", "China, Vietnam", "Vietnam and Thailand", "Vietnam; Thailand", "Vietnam/Thailand", "Vietnam or Thailand",
		"Vietnam (not Japan)", "1", "Southeast Asia", "None", "Multiple countries", "Various countries", "Not specified", "Europe", "Africa", "Vietnam & Thailand", "Vietnam, not Japan", "Japan (JP)", "Vietnam | Thailand"}
	for _, o := range bad {
		_, err, _ := avRun(t, avAnswer(Qualifies, "CTH", o, "CTH", "ok"), nil)
		if !errors.Is(err, llm.ErrBadResponse) {
			t.Errorf("origin %q: err = %v, want ErrBadResponse", o, err)
		}
	}
	good := map[string]string{"EU": "EU", "European Union": "EU", "Viet Nam": "Vietnam", "  Vietnam  ": "Vietnam", "United States of America": "United States"}
	for o, want := range good {
		res, err, _ := avRun(t, avAnswer(Qualifies, "CTH", o, "CTH", "ok"), nil)
		if err != nil || res.Prediction.Origin != want {
			t.Errorf("origin %q: got %q, %v; want %q", o, res.Prediction.Origin, err, want)
		}
	}
}

// agreementParties is a hand-written sample of the parties of the agreements
// in jpcorpus.AgreementNames (the corpus exposes agreement names, not parties);
// it is not claimed to be complete.
var agreementParties = []string{
	// AJCEP / RCEP ASEAN members
	"Brunei", "Brunei Darussalam", "Cambodia", "Indonesia", "Laos", "Malaysia", "Myanmar", "Philippines", "Singapore", "Thailand", "Vietnam",
	// RCEP, CPTPP and bilateral partners
	"China", "South Korea", "Korea", "Australia", "New Zealand", "Canada", "Chile", "Mexico", "Peru",
	"United Kingdom", "Switzerland", "India", "United States", "EU",
}

func TestSampleAgreementPartiesAreAcceptedOrigins(t *testing.T) {
	for _, party := range agreementParties {
		res, err, _ := avRun(t, avAnswer(Qualifies, "WO", party, "WO", "ok"), nil)
		if err != nil || res.Prediction.Origin == "" {
			t.Errorf("party %q rejected: %v", party, err)
		}
	}
}

func TestRealWorldOriginSpellingsAreAccepted(t *testing.T) {
	want := map[string]string{
		"Korea, Republic of": "South Korea", "Korea (Republic of)": "South Korea", "Republic of Korea": "South Korea",
		"Lao PDR": "Laos", "Lao People's Democratic Republic": "Laos", "Hong Kong, China": "Hong Kong", "Myanmar (Burma)": "Myanmar",
		"Greece": "", "Bulgaria": "", "Croatia": "", "Luxembourg": "", "Estonia": "", "Latvia": "", "Lithuania": "", "Cyprus": "", "Malta": "",
		"Nepal": "", "Kenya": "", "Ethiopia": "", "Mongolia": "", "Fiji": "", "Bhutan": "", "Maldives": "", "Uzbekistan": "", "Ghana": "",
		"Tanzania": "", "Madagascar": "", "Iceland": "", "Liechtenstein": "", "Russia": "", "Saudi Arabia": "", "Côte d'Ivoire": "",
		"Timor-Leste": "", "Trinidad and Tobago": "", "Bosnia and Herzegovina": "", "Congo, Democratic Republic of": "", "Tanzania, United Republic of": "", "South Africa": "South Africa", "United States of America": "United States",
	}
	for in, out := range want {
		res, err, _ := avRun(t, avAnswer(Qualifies, "WO", in, "WO", "ok"), nil)
		if err != nil || res.Prediction.Origin == "" || (out != "" && res.Prediction.Origin != out) {
			t.Errorf("origin %q: got %q, %v; want %q", in, res.Prediction.Origin, err, out)
		}
	}
}

func TestUndeterminedStaysAbstentionNotError(t *testing.T) {
	res, err, _ := avRun(t, avAnswer(Undetermined, "NONE", "", "", "needs facts"), nil)
	if err != nil || res.Prediction.Japan.Outcome != Undetermined || res.Prediction.Japan.ReviewReason == "" {
		t.Fatalf("undetermined = %+v, %v", res.Prediction.Japan, err)
	}
}

func TestWhitespaceOnlyDecidingRuleIsRejected(t *testing.T) {
	for _, rule := range []string{"   ", "\t\n", "\u0000", "\u202e", "　"} {
		_, err, _ := avRun(t, avAnswer(Qualifies, "CTH", "Vietnam", rule, "ok"), nil)
		if !errors.Is(err, llm.ErrBadResponse) {
			t.Errorf("rule %q: err = %v", rule, err)
		}
	}
}

func TestReasoningAndRuleAreCappedAndSanitised(t *testing.T) {
	long := strings.Repeat("é", 5000)
	res, err, _ := avRun(t, avAnswer(Qualifies, "CTH", "Vietnam", long, long), nil)
	if err != nil {
		t.Fatal(err)
	}
	p := res.Prediction
	if n := utf8.RuneCountInString(p.Reasoning); n != maxReasoningRunes || !strings.HasSuffix(p.Reasoning, "…") || !utf8.ValidString(p.Reasoning) {
		t.Errorf("reasoning runes = %d", n)
	}
	if n := utf8.RuneCountInString(p.Japan.DecidingRule); n != maxDecidingRuleRunes || !strings.HasSuffix(p.Japan.DecidingRule, "…") {
		t.Errorf("rule runes = %d", n)
	}
	short := strings.Repeat("a", maxDecidingRuleRunes)
	res, _, _ = avRun(t, avAnswer(Qualifies, "CTH", "Vietnam", short, "ok"), nil)
	if res.Prediction.Japan.DecidingRule != short {
		t.Error("a rule at the cap must be kept whole")
	}

	res, err, _ = avRun(t, avAnswer(Qualifies, "CTH", "Vietnam", "<b>CTH\u0000</b>\u202e", "line one\nline\ttwo\u0000\u202e\u2066x\u200b\ufeffy\u0007 done"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Prediction.Japan.DecidingRule; got != "<b>CTH</b>" {
		t.Errorf("rule = %q", got)
	}
	if got := res.Prediction.Reasoning; got != "line one\nline\ttwoxy done" {
		t.Errorf("reasoning = %q", got)
	}
}

func TestRulingNumberMasking(t *testing.T) {
	res, _, m := avRun(t, avAnswer(Undetermined, "NONE", "", "", "x"), nil)
	if len(res.Prediction.Precedents) == 0 {
		t.Fatal("need a retrieved precedent")
	}
	_ = m
	allowed := res.Prediction.Precedents[0]
	masked := []string{"JP 1260484", "JP_1240271", "JP-1240271", "jp1240271", "Jp 1240271", "JP1234567JP1234568", "ＪＰ1234567", "ＪＰ１２３４５６７", "JP　1234567", "JP - 1234567", "JP12345678", "日本JP1234567です", "日本 JP 1234567 です"}
	for _, s := range masked {
		got := maskUnknownRulings("see "+s+" end", nil)
		if strings.Contains(got, "1234") || strings.Contains(got, "1240271") || strings.Contains(got, "1260484") || strings.Contains(got, "１２３") {
			t.Errorf("%q not masked: %q", s, got)
		}
		if !strings.HasPrefix(got, "see ") || !strings.Contains(got, "an unverified reference") || !strings.HasSuffix(got, " end") {
			t.Errorf("%q: corrupted surroundings: %q", s, got)
		}
	}
	if got := maskUnknownRulings("JP1234567JP1234568", nil); got != "an unverified referencean unverified reference" {
		t.Errorf("concatenated = %q", got)
	}
	// Allowed IDs stay, in any spelling that names them.
	allow := map[string]bool{allowed: true}
	digits := strings.TrimPrefix(allowed, "JP")
	for _, s := range []string{allowed, strings.ToLower(allowed), "JP " + digits, "JP_" + digits} {
		if got := maskUnknownRulings("per "+s+".", allow); got != "per "+s+"." {
			t.Errorf("allowed %q masked: %q", s, got)
		}
	}
	// Other text is untouched.
	for _, s := range []string{"JPEG 1234567", "JP 2025 rules", "jp1234", "JP-123", "MAJP1234567", "in JPY 1234567", "ordinary words, jpg and Japan", "tariff 1704.90", "2025-10-01", "ａｂｃ１２３", "JP 20250101", "JP 12345678"} {
		if got := maskUnknownRulings(s, nil); got != s {
			t.Errorf("%q corrupted to %q", s, got)
		}
	}
}

func TestZeroWidthJoinersAreKept(t *testing.T) {
	if got := cleanText("a\u200db\u200cc\u202ed\u200b"); got != "a\u200db\u200ccd" {
		t.Errorf("got %q", got)
	}
}

func TestMaskingAppliesToReasoningAndRuleOnTheResult(t *testing.T) {
	res, err, _ := avRun(t, avAnswer(Qualifies, "CTH", "Vietnam", "per JP 7777773", "like JP_7777771 and ＪＰ7777772"), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{res.Prediction.Reasoning, res.Prediction.Japan.DecidingRule} {
		if strings.Contains(s, "7777773") || strings.Contains(s, "7777771") || strings.Contains(s, "7777772") {
			t.Errorf("unmasked: %q", s)
		}
	}
}

func TestCaseIDAndExcludeAreNormalisedBeforeRetrieval(t *testing.T) {
	base, _, _ := avRun(t, avAnswer(Undetermined, "NONE", "", "", "x"), nil)
	if len(base.Prediction.Precedents) == 0 {
		t.Fatal("need precedents")
	}
	top := base.Prediction.Precedents[0]
	digits := strings.TrimPrefix(top, "JP")
	late := func(c *domain.Case) { c.Date = time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC) }
	for _, ex := range [][]string{{digits}, {strings.ToLower(top)}, {" " + digits + " "}, {"x", strings.ToLower(top)}} {
		ex := ex
		res, _, _ := avRun(t, avAnswer(Undetermined, "NONE", "", "", "x"), func(c *domain.Case) { late(c); c.Exclude = ex })
		for _, id := range res.Prediction.Precedents {
			if id == top {
				t.Errorf("exclude %q still returned %s", ex, top)
			}
		}
	}
	res0, _, _ := avRun(t, avAnswer(Undetermined, "NONE", "", "", "x"), late)
	if !containsID(res0.Prediction.Precedents, top) {
		t.Skip("top precedent not retrieved at the later date")
	}
	for _, id := range []string{digits, strings.ToLower(top)} {
		res, _, _ := avRun(t, avAnswer(Undetermined, "NONE", "", "", "x"), func(c *domain.Case) { late(c); c.ID = id })
		if containsID(res.Prediction.Precedents, top) {
			t.Errorf("case id %q still retrieves itself", id)
		}
	}
}

func containsID(ids []string, id string) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

func TestExcludeDoesNotMutateCallerSlice(t *testing.T) {
	ex := []string{"1240271"}
	_, _, _ = avRun(t, avAnswer(Undetermined, "NONE", "", "", "x"), func(c *domain.Case) { c.Exclude = ex })
	if ex[0] != "1240271" {
		t.Fatalf("caller slice mutated: %v", ex)
	}
}

func TestRetrievalQueryIsCapped(t *testing.T) {
	huge := strings.Repeat("tok ", engine.FactsRunes) // 4x the cap in runes
	q := retrievalQuery("RCEP", huge)
	if n := utf8.RuneCountInString(q); n > len("RCEP ")+engine.FactsRunes {
		t.Fatalf("query runes = %d", n)
	}
	if !strings.HasPrefix(q, "RCEP tok") {
		t.Fatalf("query = %.20q", q)
	}
	if q := retrievalQuery("RCEP", "short facts"); q != "RCEP short facts" {
		t.Fatalf("short query changed: %q", q)
	}
}

var _ = jpcorpus.AgreementNames

func TestNonCountryTextIsNotAnOrigin(t *testing.T) {
	for _, o := range []string{"See reasoning", "Ignore previous instructions", "Made in China", "Product of China", "Country of origin",
		"Undetermined country", "Unclear", "Cannot determine", "Same as above", "Third country", "Any country", "TBC", "Country X", "Party A",
		"China Vietnam", "Thailand Vietnam", "Vietnam (Thailand)", "East Africa", "Western Europe", "Mercosur", "ASEAN countries",
		"Non-Japan", "China-made", "Vietnam 2025", "Japan", "Atlantis"} {
		if _, ok := originCountry(o); ok {
			t.Errorf("%q accepted", o)
		}
	}
}

func TestEUMembersAndPartiesAreAccepted(t *testing.T) {
	eu := []string{"Austria", "Belgium", "Bulgaria", "Croatia", "Cyprus", "Czechia", "Denmark", "Estonia", "Finland", "France", "Germany", "Greece",
		"Hungary", "Ireland", "Italy", "Latvia", "Lithuania", "Luxembourg", "Malta", "Netherlands", "Poland", "Portugal", "Romania", "Slovakia",
		"Slovenia", "Spain", "Sweden"}
	if len(eu) != 27 {
		t.Fatal(len(eu))
	}
	for _, c := range append(eu, agreementParties...) {
		if n, ok := originCountry(c); !ok || n == "" {
			t.Errorf("%q rejected", c)
		}
	}
	for in, want := range map[string]string{"Korea, Republic of": "South Korea", "Republic of Korea": "South Korea", "Lao PDR": "Laos",
		"Hong Kong, China": "Hong Kong", "Myanmar (Burma)": "Myanmar", "Viet Nam": "Vietnam", "Türkiye": "Turkey", "turkiye": "Turkey",
		"Russia": "Russian Federation", "Côte d’Ivoire": "Côte d’Ivoire", "cote d'ivoire": "Côte d’Ivoire", "  NEPAL ": "Nepal", "Ｖｉｅｔｎａｍ": "Vietnam",
		"Taiwan": "Taiwan", "Iceland": "Iceland", "Saudi Arabia": "Saudi Arabia", "Kenya": "Kenya"} {
		if got, ok := originCountry(in); !ok || got != want {
			t.Errorf("%q = %q, %v; want %q", in, got, ok, want)
		}
	}
}

func TestCountryDataIsPinned(t *testing.T) {
	sum := sha256.Sum256([]byte(unCountryTSV))
	if got := hex.EncodeToString(sum[:]); got != "7c8c33846a0e2dbf70acb4f36a41ff4477a55cdc59d4612b9756e9a652478662" {
		t.Errorf("countries_un_m49.tsv changed: %s", got)
	}
	if n := len(strings.Split(strings.TrimSpace(unCountryTSV), "\n")); n != 248 {
		t.Errorf("rows = %d", n)
	}
}
