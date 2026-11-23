package usmca

import (
	"errors"
	"fmt"
	"math/big"
)

// Method is the explicitly elected RVC method.
type Method string

// RVC methods.
const (
	MethodTransactionValue Method = "transaction_value"
	MethodNetCost          Method = "net_cost"
)

// ErrMethodRequired is returned when no RVC method was supplied. The package
// never elects a method (for example the most favorable one) on the caller's
// behalf.
var ErrMethodRequired = errors.New("rvc method must be explicitly supplied (transaction_value or net_cost)")

// ParseMethod validates an exact method name. Empty input yields
// ErrMethodRequired.
func ParseMethod(s string) (Method, error) {
	switch Method(s) {
	case MethodTransactionValue, MethodNetCost:
		return Method(s), nil
	case "":
		return "", ErrMethodRequired
	default:
		return "", fmt.Errorf("unknown rvc method %q", s)
	}
}

// Status is the outcome class of an RVC calculation.
type Status string

// RVC calculation statuses.
const (
	StatusComputed             Status = "computed"
	StatusUndefinedDenominator Status = "undefined_denominator"
	StatusNeedsInformation     Status = "needs_information"
)

// Problem codes.
const (
	ProblemZeroDenominator = "zero_denominator"
	ProblemUnknownValue    = "unknown_value"
)

// Problem is a structured calculation/input problem.
type Problem struct {
	Code    string `json:"code"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

// AggregateVNM is a directly supplied total value of non-originating
// materials (VNM), including materials of undetermined origin where the law
// requires. An unknown Value is unknown, never zero; a known "0" is zero.
// The caller is responsible for valuation basis and for not double counting.
type AggregateVNM struct {
	Value Amount `json:"value"`
}

// RVCInput carries one RVC calculation. Denominator is the transaction value
// for MethodTransactionValue or the net cost for MethodNetCost.
type RVCInput struct {
	Method      Method
	Denominator Amount
	VNM         AggregateVNM
}

// DisplayDecimals is the number of decimals in Result.Display.
const DisplayDecimals = 2

// DisplayNoteText explains Result.Display.
const DisplayNoteText = "Display value is rounded to 2 decimal places (half away from zero) for readability only; the rule comparison uses the exact ratio."

// Result is an RVC calculation outcome. Ratio is the exact percentage
// (for example 60 means 60%) and is nil unless Status is StatusComputed. A
// negative Ratio (VNM greater than the denominator) is reported as-is, never
// clamped.
type Result struct {
	Method      Method
	Status      Status
	Ratio       *big.Rat
	Display     string
	DisplayNote string
	Problems    []Problem
}

// CalculateRVC computes (D - VNM) / D * 100 exactly, where D is the
// transaction value or net cost per in.Method. The method must be explicit
// (ErrMethodRequired otherwise). Zero denominators are StatusUndefinedDenominator
// (not 100%, not a failure). Unknown VNM or unknown denominator is
// StatusNeedsInformation, except that a known zero denominator is reported as
// undefined regardless of VNM.
func CalculateRVC(in RVCInput) (Result, error) {
	m, err := ParseMethod(string(in.Method))
	if err != nil {
		return Result{}, err
	}
	field := "transaction_value"
	if m == MethodNetCost {
		field = "net_cost"
	}
	res := Result{Method: m}

	d, dKnown := in.Denominator.Rat()
	if dKnown && d.Sign() == 0 {
		res.Status = StatusUndefinedDenominator
		res.Problems = []Problem{{Code: ProblemZeroDenominator, Field: field,
			Message: field + " is zero, so RVC is undefined (not 100% and not a failure)"}}
		return res, nil
	}
	vnm, vKnown := in.VNM.Value.Rat()
	if !dKnown {
		res.Problems = append(res.Problems, Problem{Code: ProblemUnknownValue, Field: field,
			Message: field + " is not provided"})
	}
	if !vKnown {
		res.Problems = append(res.Problems, Problem{Code: ProblemUnknownValue, Field: "vnm",
			Message: "value of non-originating materials is not provided"})
	}
	if !dKnown || !vKnown {
		res.Status = StatusNeedsInformation
		return res, nil
	}

	ratio := new(big.Rat).Sub(d, vnm)
	ratio.Quo(ratio, d)
	ratio.Mul(ratio, big.NewRat(100, 1))
	res.Status = StatusComputed
	res.Ratio = ratio
	res.Display = ratio.FloatString(DisplayDecimals)
	res.DisplayNote = DisplayNoteText
	return res, nil
}

// Meets reports whether the exact ratio satisfies the threshold, given as a
// non-negative decimal string in percent: ratio >= threshold when inclusive,
// ratio > threshold otherwise. ratio is not modified.
func Meets(ratio *big.Rat, threshold string, inclusive bool) (bool, error) {
	if ratio == nil {
		return false, errors.New("ratio is nil")
	}
	th, err := ParseDecimal(threshold, false)
	if err != nil {
		return false, fmt.Errorf("threshold: %w", err)
	}
	c := ratio.Cmp(th)
	if inclusive {
		return c >= 0, nil
	}
	return c > 0, nil
}

// Meets compares the result's exact ratio to threshold. It returns Unknown3
// when the result has no computed ratio (undefined denominator or missing
// information) or when the threshold is malformed; it never classifies from the
// rounded Display value.
func (r Result) Meets(threshold string, inclusive bool) Tri {
	if r.Status != StatusComputed || r.Ratio == nil {
		return Unknown3
	}
	ok, err := Meets(r.Ratio, threshold, inclusive)
	if err != nil {
		return Unknown3
	}
	if ok {
		return True
	}
	return False
}
