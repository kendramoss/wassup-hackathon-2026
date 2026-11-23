package usmca

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
)

// Digit limits for ParseDecimal, counted as written (leading zeros included).
const (
	MaxIntegerDigits  = 30
	MaxFractionDigits = 12
)

// ParseDecimal parses a strict plain decimal string into an exact big.Rat.
//
// Accepted grammar: [-]DIGITS[.DIGITS]. The leading '-' is accepted only when
// allowNegative is true. Rejected: empty input, whitespace, '+', exponents,
// NaN/Inf, hex, thousands separators, non-ASCII digits, a leading or trailing
// '.', multiple dots, and more than MaxIntegerDigits integer or
// MaxFractionDigits fractional digits. "-0" parses to zero (when negatives are
// allowed).
func ParseDecimal(s string, allowNegative bool) (*big.Rat, error) {
	if s == "" {
		return nil, errors.New("decimal is empty")
	}
	body := s
	neg := false
	if body[0] == '-' {
		if !allowNegative {
			return nil, errors.New("negative values are not allowed")
		}
		neg = true
		body = body[1:]
	}
	intPart, fracPart, hasDot := strings.Cut(body, ".")
	if intPart == "" {
		return nil, errors.New("decimal must have digits before the decimal point")
	}
	if hasDot && fracPart == "" {
		return nil, errors.New("decimal must have digits after the decimal point")
	}
	if !allDigits(intPart) || !allDigits(fracPart) {
		return nil, errors.New("decimal must contain only ASCII digits and at most one '.'")
	}
	if len(intPart) > MaxIntegerDigits {
		return nil, fmt.Errorf("decimal has more than %d integer digits", MaxIntegerDigits)
	}
	if len(fracPart) > MaxFractionDigits {
		return nil, fmt.Errorf("decimal has more than %d fractional digits", MaxFractionDigits)
	}
	n, ok := new(big.Int).SetString(intPart+fracPart, 10)
	if !ok {
		return nil, errors.New("invalid decimal")
	}
	den := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(len(fracPart))), nil)
	r := new(big.Rat).SetFrac(n, den)
	if neg {
		r.Neg(r)
	}
	return r, nil
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// Amount is a non-negative monetary value that is either Known (including a
// known zero) or Unknown. The zero value of Amount is Unknown, so an omitted
// field can never be mistaken for zero.
type Amount struct {
	known bool
	v     *big.Rat
}

// Unknown returns the unknown amount.
func Unknown() Amount { return Amount{} }

// ParseAmount parses a non-negative decimal string into a known Amount.
func ParseAmount(s string) (Amount, error) {
	r, err := ParseDecimal(s, false)
	if err != nil {
		return Amount{}, err
	}
	return Amount{known: true, v: r}, nil
}

// KnownAmount wraps r as a known Amount. r must be non-nil, non-negative and
// exactly representable as a decimal with at most MaxFractionDigits fractional
// digits (so JSON round-trips exactly). The value is copied.
func KnownAmount(r *big.Rat) (Amount, error) {
	if r == nil {
		return Amount{}, errors.New("amount is nil")
	}
	if r.Sign() < 0 {
		return Amount{}, errors.New("amount must not be negative")
	}
	if _, ok := decimalString(r); !ok {
		return Amount{}, fmt.Errorf("amount is not an exact decimal with at most %d fractional digits", MaxFractionDigits)
	}
	return Amount{known: true, v: new(big.Rat).Set(r)}, nil
}

// IsKnown reports whether the amount has a value (zero is known).
func (a Amount) IsKnown() bool { return a.known }

// Rat returns a copy of the value and true, or nil and false when unknown.
func (a Amount) Rat() (*big.Rat, bool) {
	if !a.known {
		return nil, false
	}
	return new(big.Rat).Set(a.v), true
}

// String returns the canonical decimal string, or "unknown".
func (a Amount) String() string {
	if !a.known {
		return "unknown"
	}
	s, _ := decimalString(a.v)
	return s
}

// decimalString renders r exactly, trimming trailing fractional zeros. ok is
// false when r cannot be written exactly within MaxFractionDigits.
func decimalString(r *big.Rat) (string, bool) {
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(MaxFractionDigits), nil)
	scaled := new(big.Rat).Mul(r, new(big.Rat).SetInt(scale))
	if !scaled.IsInt() {
		return "", false
	}
	s := r.FloatString(MaxFractionDigits)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimSuffix(s, ".")
	}
	return s, true
}

// MarshalJSON emits a JSON string, or null when unknown.
func (a Amount) MarshalJSON() ([]byte, error) {
	if !a.known {
		return []byte("null"), nil
	}
	s, ok := decimalString(a.v)
	if !ok {
		return nil, errors.New("amount is not an exact decimal")
	}
	return json.Marshal(s)
}

// UnmarshalJSON accepts only a JSON string holding a strict non-negative
// decimal, or null (which yields Unknown). JSON numbers, booleans, arrays and
// objects are rejected so no float rounding can occur at the boundary.
func (a *Amount) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "null" {
		*a = Amount{}
		return nil
	}
	if !strings.HasPrefix(trimmed, `"`) {
		return errors.New("amount must be a JSON string, not a number or other type")
	}
	var s string
	if err := json.Unmarshal([]byte(trimmed), &s); err != nil {
		return err
	}
	parsed, err := ParseAmount(s)
	if err != nil {
		return err
	}
	*a = parsed
	return nil
}
