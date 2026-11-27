// Package atiga decides ATIGA preferential origin from a bill of materials:
// does a good made in an ASEAN Member State qualify as "originating" under
// Chapter 3 of the ASEAN Trade in Goods Agreement (Cha-am, 26 February
// 2009), so that it can claim the ATIGA preferential tariff on import into
// another Member State?
//
// It is deterministic code over the official Annex 3 product-specific rules
// (HS 2022, endorsed by the 36th AFTA Council; data/psr_hs2022.json, read
// from the ASEAN Secretariat's workbook by eval/atiga/parse_psr.py). No
// model is called. It implements:
//
//   - Art. 26 origin criteria: wholly obtained (Art. 27), or not wholly
//     obtained and meeting Art. 28;
//   - Art. 28.1 general rule for goods not listed in Annex 3: RVC of at
//     least 40 % or a change of tariff heading, at the exporter's choice;
//   - Art. 28.2 Annex 3 rules: RVC40, CC, CTH, CTSH (with "except from"
//     exclusions), RVC35 combined with CTSH, the textile process rule of
//     Attachment 1, "cut and sewn in any Member State", and the four
//     process conditions written in words (refining, wet to dry state,
//     recording on blank media, machines made from parts of the same
//     subheading), which the caller states;
//   - Art. 29 RVC, direct and indirect methods; Art. 29.3 lets the caller
//     name its Member State's single method;
//   - Art. 30.1 cumulation: materials originating in any Member State count
//     as originating;
//   - Art. 30.2 and Annex 6 partial cumulation: a material whose own RVC is
//     at least 20 % and under 40 % contributes that share of its value as
//     ASEAN content, for the RVC criterion only;
//   - Art. 31 minimal operations;
//   - Art. 33 de minimis: 10 % of FOB value, value only.
//
// This is preferential origin only. It says nothing about country-of-origin
// marking or non-preferential origin, and it is not a ruling or a
// Certificate of Origin (Form D). Not covered: Art. 28.3 and Annex 4 (ITA
// goods), Art. 32 direct consignment, Art. 34 packaging, Art. 35
// accessories, Art. 36 neutral elements, Art. 37 interchangeable materials,
// the Attachment 1 textile list itself (the caller states whether the
// process was done) and the proof-of-origin procedures; the caller must
// apply those.
package atiga
