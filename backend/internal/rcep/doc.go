// Package rcep decides RCEP preferential origin from a bill of materials:
// does a good made in an RCEP Party qualify as "originating" under Chapter 3
// of the Regional Comprehensive Economic Partnership Agreement, so that it
// can claim the RCEP preferential tariff on import into another Party?
//
// It is deterministic code over the official Annex 3A product-specific rules
// (HS 2022 transposition, data/psr_hs2022.json, parsed by
// eval/rcep/parse_psr.py). No model is called. It implements:
//
//   - Art. 3.2 originating goods: wholly obtained (Art. 3.3), produced only
//     from originating materials, or meeting the Annex 3A rule;
//   - Annex 3A rules: CC, CTH, CTSH (with "except from" exclusions), RVC40,
//     CR, and the eight rows the parser leaves to hand encoding;
//   - Art. 3.4 cumulation: materials originating in any Party count as
//     originating; work in another Party on a non-originating material
//     does not;
//   - Art. 3.5 RVC, indirect (build-down) and direct (build-up) formulas;
//     materials of undetermined origin count as non-originating (3.5.4);
//   - Art. 3.6 minimal operations;
//   - Art. 3.7 de minimis: 10 % of FOB value, or for chapters 50 to 63 also
//     10 % of the good's weight.
//
// This is preferential origin only. It says nothing about country-of-origin
// marking or non-preferential origin (the U.S. engines elsewhere in the
// repo), and it is not a ruling or a certificate of origin. Not covered:
// Art. 3.8 packaging, 3.9 accessories, 3.10 indirect materials, 3.11
// fungible materials, 3.15 direct consignment and the proof-of-origin
// procedures; the caller must apply those.
package rcep
