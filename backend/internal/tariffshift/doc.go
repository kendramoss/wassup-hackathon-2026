// Package tariffshift is a deterministic check of the 19 CFR Part 102
// marking-origin rules for non-textile goods: the tariff-shift rows of
// 102.20 for the chapters in ScopeChapters (D360, D460), applied through
// 102.11(a), 102.13 and 102.11(b) with 102.18(b)(1) (D360).
//
// It does not classify goods: it needs the HTS code of the good and of each
// material. When a rule turns on something a code cannot show (simple
// assembly, GRI 2(a), a goods description, a value for de minimis), the
// answer is undetermined with the reason, never a guess. It runs beside the
// Part 102 engine as a cross-check and never changes the engine's answer.
package tariffshift
