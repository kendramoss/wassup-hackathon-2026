package rcep

import (
	"encoding/json"
	"strings"
	"testing"
)

// The embedded table is the parse of the RCEP Secretariat PDF; these
// numbers are the parser's coverage line (docs/NUMBERS.md).
func TestTableCoverage(t *testing.T) {
	tb := Default()
	c := tb.Coverage
	if c.Rows != 2452 || c.RowsWithRule != 2189 || c.Parsed != 2181 || c.Manual != 8 || c.Unreadable != 0 || c.Chapters != 96 {
		t.Fatalf("coverage changed: %+v", c)
	}
	if c.Parsed+c.Manual != c.RowsWithRule || c.RowsWithRule+c.SplitWithoutRule != c.Rows {
		t.Fatalf("coverage does not add up: %+v", c)
	}
	var src struct {
		Primary    struct{ SHA256 string } `json:"primary"`
		CrossCheck struct{ SHA256 string } `json:"cross_check"`
	}
	if err := json.Unmarshal(tb.Source, &src); err != nil || len(src.Primary.SHA256) != 64 || len(src.CrossCheck.SHA256) != 64 {
		t.Fatalf("source record incomplete: %v %+v", err, src)
	}
}

// Every row with a rule has alternatives (parsed or hand-encoded), and
// every criterion is one the engine knows.
func TestEveryRuleIsUsable(t *testing.T) {
	known := map[string]bool{"WO": true, "CC": true, "CTH": true, "CTSH": true, "RVC": true, "CR": true}
	n := 0
	for _, r := range Default().Rules {
		if r.Status == "split" {
			continue
		}
		n++
		sets := [][]Alternative{r.Alternatives}
		if len(r.Variants) > 0 {
			sets = nil
			for _, v := range r.Variants {
				sets = append(sets, v.Alternatives)
			}
		}
		for _, alts := range sets {
			if len(alts) == 0 {
				t.Errorf("%s has no alternatives", r.Code)
			}
			for _, a := range alts {
				if !known[a.Criterion] {
					t.Errorf("%s: unknown criterion %q", r.Code, a.Criterion)
				}
				if a.Criterion == "RVC" && a.Threshold != 40 {
					t.Errorf("%s: RVC threshold %d; RCEP uses 40", r.Code, a.Threshold)
				}
				for _, x := range a.Except {
					if levelDigits[x.Level] != len(digits(x.From)) || len(digits(x.From)) != len(digits(x.To)) || x.From > x.To {
						t.Errorf("%s: bad exclusion %+v", r.Code, x)
					}
				}
			}
		}
	}
	if n != 2189 {
		t.Errorf("checked %d rows with a rule, want 2189", n)
	}
}

func TestManualRowsAreHandEncoded(t *testing.T) {
	got := 0
	for _, r := range Default().Rules {
		if r.Status == "manual" {
			got++
			if _, ok := manualRules[r.Code]; !ok {
				t.Errorf("%s has no hand encoding", r.Code)
			}
		}
	}
	if got != len(manualRules) {
		t.Errorf("%d manual rows, %d hand encodings", got, len(manualRules))
	}
}

// A changed row text must stop Load, so a new table cannot reuse a stale
// hand encoding.
func TestLoadRejectsChangedManualText(t *testing.T) {
	b := strings.Replace(string(psrJSON), "for of insect", "for insects", -1)
	if _, err := Load([]byte(b)); err == nil {
		t.Fatal("Load accepted a changed manual row")
	}
}

func TestLookup(t *testing.T) {
	cases := []struct {
		hs, code, text, need string
	}{
		{"0901.21", "0901.21", "RVC40", ""},
		{"090121", "0901.21", "RVC40", ""},
		{"0901.21.00.10", "0901.21", "RVC40", ""}, // national digits are ignored
		{"1102.10", "11.02", "CC except from Chapter 10", ""},
		{"1101.00", "1101.00", "CC", ""},
		{"1101", "1101.00", "CC", ""}, // "11.01 1101.00" sits on one row
		{"0101.21", "01", "WO", ""},   // chapter rule
		{"8504", "", "", "give 6 digits"},
		{"8504.99", "8504.90", "", "no rule found"},
		{"77", "", "", "at least 4 digits"},
	}
	for _, c := range cases {
		r, ok, need := Default().Lookup(c.hs)
		if c.need != "" {
			if ok || !strings.Contains(need, c.need) {
				t.Errorf("%s: want not found (%q), got ok=%v need=%q", c.hs, c.need, ok, need)
			}
			continue
		}
		if !ok || r.Code != c.code || r.Text != c.text {
			t.Errorf("%s: got %v %+v, want %s %q", c.hs, ok, r, c.code, c.text)
		}
	}
}

func TestParty(t *testing.T) {
	for in, want := range map[string]string{
		"Viet Nam": "VN", "vietnam": "VN", "VN": "VN", "Korea": "KR", "Republic of Korea": "KR",
		"Lao PDR": "LA", "Brunei Darussalam": "BN", "PRC": "CN", "Japan": "JP", "Burma": "MM",
		"United States": "", "Taiwan": "", "India": "", "US": "", "": "",
	} {
		if got := Party(in); got != want {
			t.Errorf("Party(%q) = %q, want %q", in, got, want)
		}
	}
}
