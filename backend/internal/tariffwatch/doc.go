// Package tariffwatch re-checks saved cases when the tariff basis they were
// priced on changes (#917, D938).
//
// A case is priced from three things the duty code computes: the MFN rate
// (HTS column 1 general), the Chapter 99 Section 301 duties (the China lists,
// the 2026 country-wide duties and their dated actions) and, when a data set
// carries them, Section 232 coverage. Basis is the snapshot of what a case
// was priced on. A Dataset is one HTS revision's worth of that data. Compare
// diffs a snapshot against a newer data set and says, per origin, what
// changed and by how much; Watch turns that into the change in the case's
// annual duty figure and in its plan's duty effect.
//
// Everything here is deterministic and offline: no network call, no model.
// Revise reads the Chapter 99 text of two HTS revisions and produces the
// newer data set from the older one.
package tariffwatch
