package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
)

// diagnosticTolerance is the largest difference accepted between two runs of
// the same candidate in the RVC figures an engine reports (actual.rvc_down and
// actual.rvc_up). Percentages are sums and quotients of float64 values, so
// amd64 and arm64 can differ in the last bits (57.93000000000001 against
// 57.92999999999999, about 2e-14). 1e-9 is five orders above that noise and
// seven below the 0.05 half-width of the smallest expected RVC range, so a
// changed formula, value or threshold still shows. Nothing else is
// tolerated: outcomes, statuses, counts, ids, hashes, errors and every other
// number must be identical.
const diagnosticTolerance = 1e-9

func decodeAny(b []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

func isDiagnosticFloat(path []string) bool {
	n := len(path)
	return n >= 2 && path[n-2] == "actual" && (path[n-1] == "rvc_down" || path[n-1] == "rvc_up")
}

func equivalent(a, b any, path []string) error {
	where := func() string { return fmt.Sprint(path) }
	switch x := a.(type) {
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok || len(x) != len(y) {
			return fmt.Errorf("%s: shape differs", where())
		}
		for k, v := range x {
			w, ok := y[k]
			if !ok {
				return fmt.Errorf("%s: key %q missing", where(), k)
			}
			if err := equivalent(v, w, append(path, k)); err != nil {
				return err
			}
		}
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return fmt.Errorf("%s: length differs", where())
		}
		for i := range x {
			if err := equivalent(x[i], y[i], append(path, fmt.Sprint(i))); err != nil {
				return err
			}
		}
	case json.Number:
		y, ok := b.(json.Number)
		if !ok {
			return fmt.Errorf("%s: type differs", where())
		}
		if x == y {
			return nil
		}
		if isDiagnosticFloat(path) {
			f, e1 := x.Float64()
			g, e2 := y.Float64()
			if e1 == nil && e2 == nil && math.Abs(f-g) <= diagnosticTolerance {
				return nil
			}
		}
		return fmt.Errorf("%s: %s differs from %s", where(), x, y)
	default:
		if a != b {
			return fmt.Errorf("%s: %v differs from %v", where(), a, b)
		}
	}
	return nil
}

// EquivalentResults reports whether two results files are the same apart from
// last-bit noise in the diagnostic RVC figures.
func EquivalentResults(got, want []byte) error {
	if bytes.Equal(got, want) {
		return nil
	}
	a, err := decodeAny(got)
	if err != nil {
		return err
	}
	b, err := decodeAny(want)
	if err != nil {
		return err
	}
	return equivalent(a, b, nil)
}
