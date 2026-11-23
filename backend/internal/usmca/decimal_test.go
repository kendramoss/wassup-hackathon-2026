package usmca

import (
	"encoding/json"
	"math/big"
	"strings"
	"testing"
)

func TestParseDecimalValid(t *testing.T) {
	cases := map[string]string{
		"0":      "0",
		"0.0":    "0",
		"12":     "12",
		"12.50":  "25/2",
		"0.001":  "1/1000",
		"007":    "7",
		"-0":     "0",
		"-1.5":   "-3/2",
		"100000": "100000",
	}
	for in, want := range cases {
		got, err := ParseDecimal(in, true)
		if err != nil {
			t.Errorf("ParseDecimal(%q): %v", in, err)
			continue
		}
		if got.RatString() != want {
			t.Errorf("ParseDecimal(%q)=%s want %s", in, got.RatString(), want)
		}
	}
}

func TestParseDecimalRejects(t *testing.T) {
	bad := []string{"", " ", "1 ", " 1", "1e3", "1E3", "NaN", "Inf", "+Inf", "-Inf", "+1", "0x10",
		"1.2.3", ".5", "5.", ".", "-", "-.5", "--1", "1,000", "1_0", "１２", "1\n", "\t1", "0b1", "1/2", "−1"}
	for _, in := range bad {
		if _, err := ParseDecimal(in, true); err == nil {
			t.Errorf("ParseDecimal(%q) accepted", in)
		}
	}
}

func TestParseDecimalNegativeDisallowed(t *testing.T) {
	if _, err := ParseDecimal("-1", false); err == nil {
		t.Error("negative accepted when disallowed")
	}
	if _, err := ParseDecimal("-0", false); err == nil {
		t.Error("-0 accepted when negatives disallowed")
	}
}

func TestParseDecimalLengthLimits(t *testing.T) {
	ok := strings.Repeat("9", MaxIntegerDigits) + "." + strings.Repeat("9", MaxFractionDigits)
	if _, err := ParseDecimal(ok, false); err != nil {
		t.Errorf("max length rejected: %v", err)
	}
	if _, err := ParseDecimal(strings.Repeat("9", MaxIntegerDigits+1), false); err == nil {
		t.Error("too many integer digits accepted")
	}
	if _, err := ParseDecimal("1."+strings.Repeat("9", MaxFractionDigits+1), false); err == nil {
		t.Error("too many fraction digits accepted")
	}
	if _, err := ParseDecimal(strings.Repeat("0", 10000), false); err == nil {
		t.Error("huge zero string accepted")
	}
}

func TestAmountTriState(t *testing.T) {
	var zeroVal Amount
	if zeroVal.IsKnown() {
		t.Error("zero value Amount must be unknown")
	}
	if Unknown().IsKnown() {
		t.Error("Unknown known")
	}
	z, err := ParseAmount("0")
	if err != nil || !z.IsKnown() {
		t.Fatalf("zero amount: %v %v", z, err)
	}
	r, ok := z.Rat()
	if !ok || r.Sign() != 0 {
		t.Error("zero amount value wrong")
	}
	if _, ok := Unknown().Rat(); ok {
		t.Error("Unknown().Rat() ok")
	}
	if _, err := ParseAmount("-1"); err == nil {
		t.Error("negative amount accepted")
	}
	if _, err := KnownAmount(big.NewRat(-1, 1)); err == nil {
		t.Error("negative KnownAmount accepted")
	}
	if _, err := KnownAmount(nil); err == nil {
		t.Error("nil KnownAmount accepted")
	}
}

func TestAmountRatIsCopy(t *testing.T) {
	a, _ := ParseAmount("5")
	r, _ := a.Rat()
	r.SetInt64(99)
	r2, _ := a.Rat()
	if r2.Cmp(big.NewRat(5, 1)) != 0 {
		t.Error("Amount mutated through returned Rat")
	}
}

func TestAmountJSON(t *testing.T) {
	type in struct {
		TV Amount `json:"tv"`
	}
	var v in
	if err := json.Unmarshal([]byte(`{"tv":"12.5"}`), &v); err != nil {
		t.Fatal(err)
	}
	if !v.TV.IsKnown() {
		t.Fatal("not known")
	}
	for _, bad := range []string{`{"tv":12.5}`, `{"tv":12}`, `{"tv":1e3}`, `{"tv":true}`, `{"tv":[]}`, `{"tv":{}}`,
		`{"tv":""}`, `{"tv":"-1"}`, `{"tv":"1e3"}`, `{"tv":" 1"}`} {
		var w in
		if err := json.Unmarshal([]byte(bad), &w); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
	// null and absent are unknown, explicitly.
	var n in
	if err := json.Unmarshal([]byte(`{"tv":null}`), &n); err != nil || n.TV.IsKnown() {
		t.Errorf("null: %v known=%v", err, n.TV.IsKnown())
	}
	var absent in
	if err := json.Unmarshal([]byte(`{}`), &absent); err != nil || absent.TV.IsKnown() {
		t.Error("absent should be unknown")
	}
	// zero string is known zero
	var z in
	if err := json.Unmarshal([]byte(`{"tv":"0"}`), &z); err != nil || !z.TV.IsKnown() {
		t.Error("\"0\" must be known")
	}
}

func TestAmountMarshalJSON(t *testing.T) {
	a, _ := ParseAmount("12.50")
	b, err := json.Marshal(a)
	if err != nil || string(b) != `"12.5"` {
		t.Errorf("marshal known: %s %v", b, err)
	}
	b, _ = json.Marshal(Unknown())
	if string(b) != "null" {
		t.Errorf("marshal unknown: %s", b)
	}
	// round trip of a repeating-free exact value
	var back Amount
	if err := json.Unmarshal([]byte(`"12.5"`), &back); err != nil || !back.IsKnown() {
		t.Error("round trip")
	}
}
