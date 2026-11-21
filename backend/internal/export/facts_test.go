package export

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

// D180, D250: the draft's facts drop CBP's reply boilerplate pasted with a
// ruling's facts, and keep every fact sentence as written.
func TestCleanFacts(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{
			name: "blank lines",
			in: "In your letter dated October 25, 2022, you requested a country of origin ruling determination on a heat gun.\n\n" +
				"The item under consideration is the Kobalt 12.5-amp heat gun. The main housing is produced in Thailand.\n\n" +
				"Each heat gun is assembled in Thailand. Each heat gun is tested before it is labeled and packaged.\n\n" +
				"With regard to your request for the appropriate country of origin of the heat gun,",
			want: "The item under consideration is the Kobalt 12.5-amp heat gun. The main housing is produced in Thailand.\n\n" +
				"Each heat gun is assembled in Thailand. Each heat gun is tested before it is labeled and packaged.",
		},
		{
			name: "abbreviations in the boilerplate sentence (#434)",
			in:   "In your letter dated May 1, 2024, on behalf of Acme Mfg. Co. Inc. of Detroit, Michigan, you requested a country of origin ruling on a steering knuckle.\n\nThe knuckle is cast in China and machined in Mexico.",
			want: "The knuckle is cast in China and machined in Mexico.",
		},
		{
			name: "trailing comma drops only the fragment (#434)",
			in:   "The fan is assembled in Thailand.\n\nThe housing is molded in Vietnam. The motor is wound in China. You ask about the origin of the fan,",
			want: "The fan is assembled in Thailand.\n\nThe housing is molded in Vietnam. The motor is wound in China.",
		},
		{
			name: "one paragraph (#434)",
			in:   "In your letter dated May 1, 2024, you requested a country of origin ruling on a heat gun. The heat gun is assembled in Thailand from Chinese parts. You ask whether the heat gun,",
			want: "The heat gun is assembled in Thailand from Chinese parts.",
		},
		{
			name: "de C.V. with a double period, CR breaks",
			in:   "In your letter dated April 14, 2021 you requested a country of origin ruling on behalf of your client, Melecs Electronics, S.A. de C.V..\r\rThe motor is assembled in Mexico.",
			want: "The motor is assembled in Mexico.",
		},
		{
			name: "a date on its own line and no space after the period",
			in:   "In your letter dated \r\n April 12, 2026\r\n , you requested a country of origin ruling on behalf of your client, Acme (Thailand) Co., Ltd. to apply current trade remedies.The product at issue is a coil unit.",
			want: "The product at issue is a coil unit.",
		},
		{
			name: "company suffix ends the boilerplate sentence",
			in:   "In your letter dated May 1, 2024, you requested a ruling on behalf of John J. Smith Inc. The motor is wound in China.",
			want: "The motor is wound in China.",
		},
		{
			name: "a single letter ends the boilerplate sentence",
			in:   "In your letter dated May 1, 2024, you requested a ruling on a panel for Tesla Model Y. The panel is cut in Canada.",
			want: "The panel is cut in Canada.",
		},
		{
			name: "U.S. as an adjective",
			in:   "This is in response to your letter dated May 1, 2020, to U.S. Customs and Border Protection, requesting a ruling. The pump is made in Japan.",
			want: "The pump is made in Japan.",
		},
		{
			name: "closing bracket before a missing space",
			in:   "In your letter dated November 3, 2010, you requested a ruling on behalf of La Renaissance (Quebec, Canada).You indicate that live lobster is shipped to Canada.",
			want: "You indicate that live lobster is shipped to Canada.",
		},
		{
			name: "single line breaks become paragraphs",
			in:   "The motor is wound in China.\nThe PCBA is populated in Thailand.",
			want: "The motor is wound in China.\n\nThe PCBA is populated in Thailand.",
		},
		{
			name: "a list after one line break keeps its items (#465)",
			in:   "In your letter dated May 1, 2024, you requested a country of origin ruling on a heat gun.\n- The motor is wound in China.\n- The heat gun is assembled in Thailand.",
			want: "- The motor is wound in China.\n\n- The heat gun is assembled in Thailand.",
		},
		{
			name: "a bulleted list with CRLF breaks (#465)",
			in:   "In your letter dated May 1, 2024, you requested a country of origin ruling on a heat gun.\r\n• The motor is wound in China.\r\n• The heat gun is assembled in Thailand.",
			want: "• The motor is wound in China.\n\n• The heat gun is assembled in Thailand.",
		},
		{
			name: "a boilerplate item inside a list is dropped alone (#465)",
			in:   "- The motor is wound in China.\n- You requested a ruling on the origin of the heat gun.\n- The heat gun is assembled in Thailand.",
			want: "- The motor is wound in China.\n\n- The heat gun is assembled in Thailand.",
		},
		{
			name: "unpunctuated list items are separate sentences (#478)",
			in:   "In your letter dated May 1, 2024, you requested a country of origin ruling on a heat gun.\n- The motor is wound in China\n- You requested a ruling on the origin of the heat gun\n- The heat gun is assembled in Thailand",
			want: "- The motor is wound in China\n\n- The heat gun is assembled in Thailand",
		},
		{
			name: "an unpunctuated list needs no lead-in (#478)",
			in:   "- The motor is wound in China\n- You requested a ruling on the origin of the heat gun\n- The heat gun is assembled in Thailand",
			want: "- The motor is wound in China\n\n- The heat gun is assembled in Thailand",
		},
		{
			name: "other common bullet characters (#478)",
			in:   "In your letter dated May 1, 2024, you requested a country of origin ruling on a heat gun.\n+ The motor is wound in China.\n+ You requested a ruling on the origin of the heat gun.\n+ The heat gun is assembled in Thailand.",
			want: "+ The motor is wound in China.\n\n+ The heat gun is assembled in Thailand.",
		},
		{
			name: "a colon lead-in does not take the first item (#478)",
			in:   "In your letter dated May 1, 2024, you requested a country of origin ruling on a heat gun, which is described as follows:\n- The motor is wound in China.\n- The heat gun is assembled in Thailand.",
			want: "- The motor is wound in China.\n\n- The heat gun is assembled in Thailand.",
		},
		{
			name: "a dropped numbered item leaves no number (#478)",
			in:   "In your letter dated May 1, 2024, you requested a country of origin ruling on a heat gun.\n1. The motor is wound in China.\n2. You requested a ruling on the origin.\n3. The heat gun is assembled in Thailand.",
			want: "1. The motor is wound in China.\n\n3. The heat gun is assembled in Thailand.",
		},
		{
			name: "multi-letter roman numerals stay with their items (#478)",
			in:   "In your letter dated May 1, 2024, you requested a country of origin ruling on a heat gun.\nii. The motor is wound in China.\niii. You requested a ruling on the origin.\niv) The heat gun is assembled in Thailand.",
			want: "ii. The motor is wound in China.\n\niv) The heat gun is assembled in Thailand.",
		},
		{
			name: "nothing but boilerplate gives a placeholder",
			in:   "In your letter dated 1 May, you requested a ruling.",
			want: NoFactsPlaceholder,
		},
		{
			name: "a lone comma fragment that is a fact is kept",
			in:   "Steel brackets are stamped in Vietnam,",
			want: "Steel brackets are stamped in Vietnam,",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := CleanFacts(tc.in); got != tc.want {
				t.Errorf("CleanFacts =\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
}

// Every common pasted bullet marker separates items (#478), including Word's
// second-level "o" and the small square/diamond markers.
func TestCleanFactsBulletMarkers(t *testing.T) {
	lead := "In your letter dated May 1, 2024, you requested a country of origin ruling on a heat gun."
	for _, b := range []string{"+", "o", "◦", "▪", "‣"} {
		in := lead + "\n" + b + " The motor is wound in China.\n" + b +
			" You requested a ruling on the origin of the heat gun.\n" + b + " The heat gun is assembled in Thailand."
		want := b + " The motor is wound in China.\n\n" + b + " The heat gun is assembled in Thailand."
		if got := CleanFacts(in); got != want {
			t.Errorf("marker %q: CleanFacts =\n%q\nwant\n%q", b, got, want)
		}
	}
}

// Facts with no boilerplate, abbreviations or line breaks come back as written.
func TestCleanFactsKeepsPlainFacts(t *testing.T) {
	for _, in := range []string{
		"Steel brackets are stamped in Vietnam from Chinese coil and zinc plated in Vietnam.",
		"You state that the motor is wound in China. The PCBA is populated in Thailand.",
		"The knuckle is made by Acme Mfg. Co. Inc. of Detroit. It is cast in China.",
		"The casings were made in several countries such as the U.S.A., Mexico, Brazil and Japan. They are retreaded in Canada.",
		"The plant manager, J. Smith, states that No. 5 motors are wound in China, e.g. the M5. The rest come from S.A. de C.V. plants in Mexico.",
		"Salicylate is measured in serum, i.e. the reagent is a diagnostic. Each kit holds 2 vials.",
	} {
		if got := CleanFacts(in); got != in {
			t.Errorf("CleanFacts(%q) = %q", in, got)
		}
	}
}

// The heat-gun demo's own facts, with each kind of paragraph break (#434):
// both letter formats keep every fact sentence and drop CBP's wording.
func TestDraftHeatGunFactsAnyLineBreaks(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "demos", "fixtures", "heatgun_2022.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fx struct {
		Predict struct {
			Facts string `json:"facts"`
		} `json:"predict"`
	}
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatal(err)
	}
	f := fx.Predict.Facts
	if !strings.Contains(f, "\n\n") {
		t.Fatal("fixture facts no longer use blank lines")
	}
	facts := []string{
		"The item under consideration is the Kobalt 12.5-amp heat gun, model number 5208257, which features two fan speeds, a variable temperature control dial, and is rated at 1500 watts.",
		"Each heat gun is assembled in Thailand using 24 components sourced from China and Thailand.",
		"Each heat gun is tested before it is labeled and packaged.",
	}
	for name, breaks := range map[string]string{
		"LF LF": "\n\n", "LF": "\n", "CRLF CRLF": "\r\n\r\n", "CRLF": "\r\n", "CR CR": "\r\r", "CR": "\r",
	} {
		in := Input{ProductName: "heat gun", Facts: strings.ReplaceAll(f, "\n\n", breaks), Requested: "Thailand"}
		for format, out := range map[string]string{"markdown": Markdown(in), "cfr177": CFR177(in)} {
			for _, bad := range []string{"In your letter", "you requested", "With regard to your request", "\r"} {
				if strings.Contains(out, bad) {
					t.Errorf("%s/%s: draft contains %q", name, format, bad)
				}
			}
			for _, s := range facts {
				if !strings.Contains(out, s) {
					t.Errorf("%s/%s: draft lost %q", name, format, s)
				}
			}
		}
		if got, want := CleanFacts(in.Facts), CleanFacts(f); got != want {
			t.Errorf("%s: CleanFacts differs from the blank-line facts:\n%q\nwant\n%q", name, got, want)
		}
	}
}

// #465 asked for the heat-gun demo facts as a bulleted list too: the pasted
// lead-in is dropped alone and every fact survives, with LF or CRLF breaks.
func TestDraftHeatGunFactsAsBullets(t *testing.T) {
	lead := "In your letter dated May 1, 2024, you requested a country of origin ruling on a heat gun."
	items := []string{
		"The item under consideration is the Kobalt 12.5-amp heat gun, model number 5208257.",
		"Each heat gun is assembled in Thailand using 24 components sourced from China and Thailand.",
		"Each heat gun is tested before it is labeled and packaged.",
	}
	for _, bullet := range []string{"-", "•"} {
		for _, br := range []string{"\n", "\r\n"} {
			facts := lead + br + bullet + " " + strings.Join(items, br+bullet+" ")
			in := Input{ProductName: "heat gun", Facts: facts, Requested: "Thailand"}
			for format, out := range map[string]string{"markdown": Markdown(in), "cfr": CFR177(in)} {
				if strings.Contains(out, "you requested") {
					t.Errorf("%q/%s: draft contains the lead-in: %s", bullet, format, out)
				}
				for _, s := range items {
					if !strings.Contains(out, s) {
						t.Errorf("%q/%s: draft lost %q: %s", bullet, format, s, out)
					}
				}
			}
		}
	}
}

// The corpus of committed CBP facts (#434). Each file that carries a
// boilerplate phrase must come back without one, with no paragraph that
// starts with a fragment, and with at least 99.5% of its other sentences
// kept. The reference sentences come from a naive split after every end
// mark, so a reference "sentence" that is part of a dropped boilerplate
// sentence (cut at "Inc." or "U.S.") counts as lost: at D250 the rate is
// 32,973 of 33,046 (99.78%), and the rest are those pieces and the dangling
// "With regard to your request…," fragments. Files without a phrase keep
// their text, apart from a dangling comma fragment.
func TestCleanFactsCorpus(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", "data", "facts", "*.txt"))
	if err != nil || len(files) < 3000 {
		t.Fatalf("facts corpus: %d files, %v", len(files), err)
	}
	naive := regexp.MustCompile(`[.!?]+["'”)\]]*\s+`)
	paraSplit := regexp.MustCompile(`\n\s*\n`)
	collapse := func(s string) string { return strings.Join(strings.Fields(s), " ") }
	var withPhrase, ref, kept int
	for _, fn := range files {
		raw, err := os.ReadFile(fn)
		if err != nil {
			t.Fatal(err)
		}
		in, id := string(raw), filepath.Base(fn)
		out := CleanFacts(in)
		norm, outN := collapse(in), collapse(out)
		if strings.ContainsAny(out, "\r\v\f") {
			t.Errorf("%s: output keeps a CR, VT or FF", id)
		}
		if !boilerplate(norm) {
			if outN != norm && !(strings.HasSuffix(norm, ",") && strings.HasPrefix(norm, outN)) {
				t.Errorf("%s: facts without boilerplate changed", id)
			}
			continue
		}
		withPhrase++
		if boilerplate(out) {
			t.Errorf("%s: boilerplate kept: %.80q", id, out)
		}
		for _, p := range paraSplit.Split(out, -1) {
			first := strings.ToLower(strings.TrimSuffix(nextWord(p), "."))
			if mayEnd[first] || !startsCleanly(in, p) {
				t.Errorf("%s: paragraph starts with a fragment: %.60q", id, p)
			}
		}
		for _, s := range naive.Split(norm, -1) {
			if len(strings.Fields(s)) < 5 || boilerplate(s) {
				continue
			}
			ref++
			if strings.Contains(outN, s) {
				kept++
			}
		}
	}
	if withPhrase < 2500 {
		t.Fatalf("only %d files carry a boilerplate phrase", withPhrase)
	}
	if rate := float64(kept) / float64(ref); rate < 0.995 {
		t.Errorf("kept %d of %d reference sentences (%.2f%%), want at least 99.5%%", kept, ref, 100*rate)
	}
	t.Logf("%d files with a boilerplate phrase; kept %d of %d reference sentences", withPhrase, kept, ref)
}

// startsCleanly says whether paragraph p of the output starts where the
// input starts a line or a sentence: at the start, after a line break, or
// after an end mark and a space. A lower-case or digit start is fine there
// (a list item, "10% Calcium Chloride Injection…"); anywhere else it is a
// fragment cut from a sentence.
func startsCleanly(in, p string) bool {
	if p == NoFactsPlaceholder {
		return true
	}
	in = normaliseBreaks(in)
	rs := []rune(strings.TrimSpace(strings.SplitN(p, "\n", 2)[0]))
	prefix := string(rs[:min(len(rs), 30)])
	for off := 0; ; {
		i := strings.Index(in[off:], prefix)
		if i < 0 {
			return false
		}
		i += off
		before := strings.TrimRightFunc(in[:i], func(r rune) bool { return r != '\n' && unicode.IsSpace(r) })
		if before == "" || strings.HasSuffix(before, "\n") {
			return true
		}
		if i > len(before) { // a space before the paragraph
			r, _ := utf8.DecodeLastRuneInString(before)
			if strings.ContainsRune(`.!?)"”’`, r) {
				return true
			}
		}
		if i == len(before) && i > 0 { // "ruling.The"
			r, _ := utf8.DecodeLastRuneInString(before)
			if strings.ContainsRune(`.!?)`, r) {
				return true
			}
		}
		off = i + 1
	}
}
