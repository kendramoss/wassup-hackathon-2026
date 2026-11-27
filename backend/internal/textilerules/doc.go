// Package textilerules is a deterministic check of the 19 CFR 102.21
// textile and apparel rules of origin (D951): the (c)(1) to (c)(5)
// hierarchy with the specific rules of paragraph (e) encoded from the text
// of the regulation.
//
// It needs the HTS code of the good and facts about where each operation took
// place (where the fabric was formed, where the good was assembled, and so
// on). When a rule turns on a fact that is not given, or on a judgment (the
// "most important" process of (c)(4)), the answer is undetermined with the
// reason, never a guess. It runs beside the model as a cross-check, is shown
// and never changes an answer, and is behind TEXTILE_RULES (off).
package textilerules
