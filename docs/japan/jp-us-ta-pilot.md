# Japan-US Trade Agreement rule pilot (Japanese text)

The Japan engine now has product specific rule text for the Japan-US Trade Agreement (agreement code `JP-US TA`, in force 2020-01-01). Source: Annex I Section C (Japan's rules of origin and procedures), published only in Japanese by Japan Customs: <https://www.customs.go.jp/roo/text/us2.pdf>, 28 pages, retrieved 25 November 2026, SHA-256 `af460808179a30ce688197ac064b170f64f7d55f943bc898860771eabf9962d6`. The Japanese text is the authoritative text, so rows and notes are stored in Japanese and are not translated. Annex II (the US rules) is in English and is not embedded.

## What is embedded

| Part | Count | Where in the PDF |
|---|---|---|
| Table rows of Subsection 3 | 92 | pages 15 to 27 |
| Default rule rows (CC) | 17 | page 15 |
| Notes quoted from the PDF | 27 | pages 1 to 15 |
| Unde Labs caveat notes (no page, no link) | 5 | not treaty text |

Rows keep every branch of a cell, including three rows that continue on the next page: 1901.20 (pages 19 to 20), 2005.99 (21 to 22) and 2008.19 (22 to 23). Such a row cites the page it starts on.

The table lists the agricultural, food and chemical goods Japan opened. The head of Subsection 3 says the rule for the goods in Subsection 2 note 1(a) to (p) is CC unless the table says otherwise. A default row exists only for those goods, each quoting that sentence: chapters 1 to 2, 4 to 15, 16.01 to 16.03, chapters 17 to 21, 22.02, 22.04, 22.09, chapter 23, 2905.43 to 2905.45, 33.01, 35.01 to 35.02, 35.04 to 35.05, 3809.10, 3823.11 to 3823.70, and the national tariff item 2207.10-199. The 22.09 default is shadowed by a table row for the whole heading. A code outside these goods returns `no_match`, never a general default.

## Behaviour

- National items. 2206.00-100 and 2207.10-199 are nine digit items; the lookup keeps six digits. They are stored as nine digit rows, so any code in 2206.00, 2206 or 2207.10 returns `too_broad` and goes to review. No rule is claimed for the whole subheading.
- Date. Codes before 2020-01-01 return `no_dated_source`. No end date is set; later amendments to Annex I are not checked.
- Notes keep the scope the PDF gives them. Subsection 1 paragraphs 1 to 3, Subsection 2 notes 1 to 3 and the Subsection 3 head are attached only to codes in the goods of note 1(a) to (p). Note 4 (plants, chapters 6 to 14) applies to chapters 6 to 14 only. Notes 5 and 6 (cocoa content, confectionery) apply to heading 18.06 only. Every result also carries an Unde Labs caveat that says the embedded text is partial.
- Not embedded: Subsection 1 paragraphs 4 to 10 (fungible goods or materials, accessories, spare parts and tools, packaging, indirect materials, transport through a third country, the importer's declaration, verification and denial) and Annex II. A provision in them may change an answer; the caveat says so.
- The prompt and the answer schema are unchanged. Sources shown to the user list the PDF pages only, never the caveat.

## HS edition

The agreement names no HS edition. The citation says so ("HS edition not stated by the agreement") and does not claim HS 2017. All 92 table codes and the default ranges were checked offline against the HS 2017 subheading list (5,387 codes, SHA-256 `516da3067b86077fe0508822e70076ea7a0c6721f7bb853c6fc524030dc4eb73`, from the AJCEP validity work); every code is in it. That list is not part of this change, so the check is recorded here and not run as a test. No correlation to HS 2022 is added.

Known gap: a code that exists only in HS 2022 and falls under a chapter-level default row (for example 0709.52 to 0709.56 under chapters 6 to 14) returns the default CC sentence, because the text makes CC the default for the whole chapter. A code that exists only in HS 2022 under an explicit table range keeps that row. Whether the table's explicit rows were re-based for HS 2022 is unverified.

## Development counts (offline, no model)

`jpeval diagnose -split dev` (149 JP-US TA cases): before, 149 `unsupported_agreement`; after, 148 `psr` and 1 `too_broad` (code 2207.10). Eligible cases rose from 2,673 to 2,821 and abstained fell from 300 to 152. `dev_eval` (22 JP-US TA cases): 22 `unsupported_agreement` before, 22 `psr` after. No other agreement changed. No HS 2022-only code appears in the development cases.

## Open questions

- Should HS 2022-only codes under chapter-level defaults be withheld rather than given the default? That needs per-code default rows from an HS 2017 list, which would make heading-level codes `too_broad`.
- Annex I amendments after 2020-01-01 are not checked.
- A Japanese customs specialist has not reviewed the text.
