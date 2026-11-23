package usmca

import (
	"errors"
	"math/big"
	"testing"
)

func amt(t *testing.T, s string) Amount {
	t.Helper()
	a, err := ParseAmount(s)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestParseMethod(t *testing.T) {
	if m, err := ParseMethod("transaction_value"); err != nil || m != MethodTransactionValue {
		t.Error("tv")
	}
	if m, err := ParseMethod("net_cost"); err != nil || m != MethodNetCost {
		t.Error("nc")
	}
	if _, err := ParseMethod(""); !errors.Is(err, ErrMethodRequired) {
		t.Errorf("empty: %v", err)
	}
	for _, bad := range []string{"TV", "Net_Cost", "best", "most_favorable", " net_cost"} {
		if _, err := ParseMethod(bad); err == nil || errors.Is(err, ErrMethodRequired) {
			t.Errorf("%q: %v", bad, err)
		}
	}
}

func TestCalculateRequiresMethod(t *testing.T) {
	_, err := CalculateRVC(RVCInput{Denominator: amt(t, "100"), VNM: AggregateVNM{Value: amt(t, "40")}})
	if !errors.Is(err, ErrMethodRequired) {
		t.Errorf("got %v", err)
	}
	_, err = CalculateRVC(RVCInput{Method: Method("x"), Denominator: amt(t, "100")})
	if err == nil {
		t.Error("invalid method accepted")
	}
}

func TestCalculateTV(t *testing.T) {
	r, err := CalculateRVC(RVCInput{Method: MethodTransactionValue, Denominator: amt(t, "100"), VNM: AggregateVNM{amt(t, "40")}})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != StatusComputed || r.Ratio.Cmp(big.NewRat(60, 1)) != 0 || r.Display != "60.00" {
		t.Errorf("%+v", r)
	}
	if r.Method != MethodTransactionValue || r.DisplayNote == "" {
		t.Error("method/note")
	}
}

func TestCalculateNC(t *testing.T) {
	r, _ := CalculateRVC(RVCInput{Method: MethodNetCost, Denominator: amt(t, "80"), VNM: AggregateVNM{amt(t, "20")}})
	if r.Status != StatusComputed || r.Ratio.Cmp(big.NewRat(75, 1)) != 0 {
		t.Errorf("%+v", r)
	}
}

func TestCalculateZeroVNMIsKnown(t *testing.T) {
	r, _ := CalculateRVC(RVCInput{Method: MethodTransactionValue, Denominator: amt(t, "50"), VNM: AggregateVNM{amt(t, "0")}})
	if r.Status != StatusComputed || r.Ratio.Cmp(big.NewRat(100, 1)) != 0 {
		t.Errorf("%+v", r)
	}
}

func TestCalculateNegativeNotClamped(t *testing.T) {
	r, _ := CalculateRVC(RVCInput{Method: MethodTransactionValue, Denominator: amt(t, "100"), VNM: AggregateVNM{amt(t, "125")}})
	if r.Status != StatusComputed || r.Ratio.Cmp(big.NewRat(-25, 1)) != 0 || r.Display != "-25.00" {
		t.Errorf("%+v", r)
	}
}

func TestCalculateZeroDenominator(t *testing.T) {
	for _, m := range []Method{MethodTransactionValue, MethodNetCost} {
		for _, v := range []string{"0", "5"} {
			r, err := CalculateRVC(RVCInput{Method: m, Denominator: amt(t, "0"), VNM: AggregateVNM{amt(t, v)}})
			if err != nil {
				t.Fatal(err)
			}
			if r.Status != StatusUndefinedDenominator || r.Ratio != nil || r.Display != "" || len(r.Problems) != 1 {
				t.Errorf("%v %s: %+v", m, v, r)
			}
			if r.Problems[0].Code != ProblemZeroDenominator {
				t.Errorf("code %q", r.Problems[0].Code)
			}
			if r.Meets("0", true) != Unknown3 {
				t.Error("undefined must not meet/fail")
			}
		}
	}
}

func TestCalculateNeedsInformation(t *testing.T) {
	cases := []RVCInput{
		{Method: MethodTransactionValue, Denominator: amt(t, "100")},      // VNM unknown
		{Method: MethodTransactionValue, VNM: AggregateVNM{amt(t, "10")}}, // denom unknown
		{Method: MethodNetCost},                           // both
		{Method: MethodNetCost, Denominator: amt(t, "0")}, // zero denom + unknown VNM: unknown VNM wins? see below
	}
	for i, in := range cases[:3] {
		r, err := CalculateRVC(in)
		if err != nil || r.Status != StatusNeedsInformation || r.Ratio != nil {
			t.Errorf("%d: %+v %v", i, r, err)
		}
		if len(r.Problems) == 0 {
			t.Errorf("%d: no problems", i)
		}
		if r.Meets("60", true) != Unknown3 {
			t.Errorf("%d: meets", i)
		}
	}
	// Zero denominator is determinable regardless of VNM, so it is reported as undefined.
	r, _ := CalculateRVC(cases[3])
	if r.Status != StatusUndefinedDenominator {
		t.Errorf("%+v", r)
	}
}

func TestProblemFieldsNameDenominator(t *testing.T) {
	r, _ := CalculateRVC(RVCInput{Method: MethodTransactionValue})
	fields := map[string]bool{}
	for _, p := range r.Problems {
		fields[p.Field] = true
	}
	if !fields["transaction_value"] || !fields["vnm"] {
		t.Errorf("%+v", r.Problems)
	}
	r, _ = CalculateRVC(RVCInput{Method: MethodNetCost})
	fields = map[string]bool{}
	for _, p := range r.Problems {
		fields[p.Field] = true
	}
	if !fields["net_cost"] {
		t.Errorf("%+v", r.Problems)
	}
}

func TestDisplayNeverDrivesClassification(t *testing.T) {
	// ratio = 59.996 -> displays 60.00 but must fail a 60 threshold.
	r, _ := CalculateRVC(RVCInput{Method: MethodTransactionValue, Denominator: amt(t, "100000"), VNM: AggregateVNM{amt(t, "40004")}})
	if r.Display != "60.00" {
		t.Fatalf("display %s", r.Display)
	}
	if r.Meets("60", true) != False {
		t.Error("display rounding leaked into classification")
	}
	if r.Meets("59.996", true) != True || r.Meets("59.996", false) != False {
		t.Error("exact boundary")
	}
}

func TestMeetsBoundaries(t *testing.T) {
	R := func(n, d int64) *big.Rat { return big.NewRat(n, d) }
	cases := []struct {
		name      string
		ratio     *big.Rat
		th        string
		inclusive bool
		want      bool
	}{
		{"exact inclusive", R(60, 1), "60", true, true},
		{"exact exclusive", R(60, 1), "60", false, false},
		{"just below", R(599999999999, 10000000000), "60", true, false},
		{"just above", R(600000000001, 10000000000), "60", false, true},
		{"one third vs 33.33", R(100, 3), "33.33", true, true},
		{"one third vs 33.34", R(100, 3), "33.34", true, false},
		{"one third below 33.333333333333", R(100, 3), "33.333333333333", false, true},
		{"two thirds below 66.666666666667", R(200, 3), "66.666666666667", true, false},
		{"negative below zero", R(-1, 3), "0", true, false},
		{"negative vs zero exclusive", R(-1, 1000000), "0", false, false},
		{"zero inclusive zero", R(0, 1), "0", true, true},
		{"huge", new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(60), nil)), "999999999999999999999999999999.999999999999", true, true},
		{"huge below", R(-1, 1), "999999999999999999999999999999.999999999999", true, false},
	}
	for _, c := range cases {
		got, err := Meets(c.ratio, c.th, c.inclusive)
		if err != nil || got != c.want {
			t.Errorf("%s: got %v err %v want %v", c.name, got, err, c.want)
		}
	}
}

func TestMeetsErrors(t *testing.T) {
	if _, err := Meets(nil, "60", true); err == nil {
		t.Error("nil ratio")
	}
	for _, bad := range []string{"", "-1", "1e2", "abc", "60%"} {
		if _, err := Meets(big.NewRat(1, 1), bad, true); err == nil {
			t.Errorf("threshold %q accepted", bad)
		}
	}
}

func TestMeetsDoesNotMutate(t *testing.T) {
	r := big.NewRat(1, 3)
	_, _ = Meets(r, "1", true)
	if r.Cmp(big.NewRat(1, 3)) != 0 {
		t.Error("mutated")
	}
}

func TestResultMeetsBadThresholdIsUnknown(t *testing.T) {
	r, _ := CalculateRVC(RVCInput{Method: MethodNetCost, Denominator: amt(t, "10"), VNM: AggregateVNM{amt(t, "1")}})
	if r.Meets("bad", true) != Unknown3 {
		t.Error("bad threshold must not classify")
	}
}

func TestRepeatingDecimalExact(t *testing.T) {
	// (3-1)/3*100 = 200/3
	r, _ := CalculateRVC(RVCInput{Method: MethodNetCost, Denominator: amt(t, "3"), VNM: AggregateVNM{amt(t, "1")}})
	if r.Ratio.Cmp(big.NewRat(200, 3)) != 0 || r.Display != "66.67" {
		t.Errorf("%+v", r)
	}
}

func TestResultRatioIsIndependentCopy(t *testing.T) {
	r, _ := CalculateRVC(RVCInput{Method: MethodNetCost, Denominator: amt(t, "10"), VNM: AggregateVNM{amt(t, "1")}})
	r.Ratio.SetInt64(0)
	r2, _ := CalculateRVC(RVCInput{Method: MethodNetCost, Denominator: amt(t, "10"), VNM: AggregateVNM{amt(t, "1")}})
	if r2.Ratio.Sign() == 0 {
		t.Error("shared state")
	}
}
