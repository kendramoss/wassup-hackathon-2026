// Package verifier checks a proposed production move against earlier CBP
// rulings. An ADK agent (Gemini) searches and reads rulings dated strictly
// before the case and answers "precedent-supported" only when it finds an
// earlier ruling where CBP held the same kind of change decisive; it also
// reports a counter-ruling where a similar change did not alter origin.
// Precedent support is not a ruling.
package verifier
