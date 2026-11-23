// Package usmca holds the source-independent arithmetic and logic used by the
// USMCA origin-rule check: exact decimal parsing, tri-state monetary amounts,
// regional value content (RVC) calculation, exact threshold comparison and
// Kleene three-valued logic.
//
// The arithmetic and logic files hold no rule data. The versioned pilot rows
// (8202.31, 8207.13, 9503.00) live in the embedded data directory and are read
// by the rule engine; nothing else here carries thresholds or HS data.
//
// # Precision limits
//
//   - All values are exact math/big.Rat. No float64 is used anywhere.
//   - Input decimal strings are limited to MaxIntegerDigits integer digits and
//     MaxFractionDigits fractional digits (counted as written, including leading
//     zeros) so that request cost is bounded.
//   - RVC ratios are rational and may repeat (for example 200/3). The Display
//     string is rounded to DisplayDecimals places, half away from zero, for
//     humans only. Rule classification must use the exact Ratio via Meets and
//     never the Display string.
//   - Values are currency-agnostic: the caller must supply TV, NC and VNM in the
//     same currency and valuation basis. This package does not verify that.
//   - Originating status of materials and the legal validity of a caller's
//     TV/NC/VNM figures are caller-supplied facts; nothing here certifies them.
package usmca
