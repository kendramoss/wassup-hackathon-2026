// Package aseanfta decides preferential origin under the two ASEAN plus one
// agreements that follow the ATIGA's shape: the ASEAN-Korea Free Trade Area
// (AKFTA) and the ASEAN-China Free Trade Area (ACFTA, as upgraded). It reuses
// the ATIGA engine (package atiga): the same bill-of-materials input, the
// same rule table format and the same checks, read for each agreement's own
// text through a Profile.
//
// Deterministic code, no model call. Sources, pinned in data/ and in
// DECISIONS D949:
//
//   - AKFTA: Annex 3 (Rules of Origin) of the Agreement on Trade in Goods
//     under the Framework Agreement on Comprehensive Economic Cooperation
//     between Korea and ASEAN, with the Product Specific Rules of its
//     Appendix 2 transposed to HS 2022 and endorsed by the AKFTA
//     Implementing Committee (in force 1 May 2026): Rule 2 (origin criteria),
//     Rule 3 (wholly obtained), Rule 4 (RVC 40 or a change of tariff heading
//     for goods Appendix 2 does not list; build-up or build-down), Rule 5
//     (Appendix 2 rules: WO, WO in any Party, CC, CTH, CTSH, RVC 35 to 70,
//     conditions on materials, product-dependent branches), Rule 7
//     (accumulation), Rule 8 (the fifteen non-qualifying operations) and Rule
//     10 (de minimis: 10 % of FOB value, or of weight for chapters 50 to 63).
//   - ACFTA: Annex 1 (Rules of Origin) of the 2015 Upgrading Protocol, in
//     force 15 August 2019, with Attachment B (Product Specific Rules, HS
//     2017): Art. 2 (originating goods), Art. 3 (wholly obtained), Art. 4
//     (RVC 40 with the final process in a Party, or a change of tariff
//     heading for the chapters it lists; Attachment B otherwise), Art. 5
//     (build-down RVC only), Art. 6 (accumulation), Art. 7 (minimal
//     operations, which speak only to wholly obtained goods) and Art. 9 (de
//     minimis: 10 % of FOB value; for chapters 50 to 63 of weight or value).
//
// Preferential origin only. It says nothing about marking or non-preferential
// origin and it is not a ruling or a Certificate of Origin (Form AK, Form E).
// The ACFTA 3.0 Upgrade Protocol (signed 28 October 2025; reported to enter
// into force on 1 December 2026) has its own Rules of Origin chapter with
// regional cumulation and revised product-specific rules; it is not read
// here, so the ACFTA check follows the 2019 rules until it is.
package aseanfta
