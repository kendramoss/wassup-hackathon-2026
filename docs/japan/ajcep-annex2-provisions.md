# AJCEP Annex 2 provisions returned with rule rows

Applies to AJCEP, HS 2017 Annex 2 window (case date 2023-01-01 up to, not including, 2027-02-15).

## What changed

A rule hit used to return only the matched rows and the one Section XI note. The Annex 2 text that defines how the rows are read sat in `data/ajcep_general.json` and was returned only when no row matched. A hit now ends with these entries, after the rows and notes, in this order:

| Entry | Source and page | Why it is returned |
|---|---|---|
| `annex2-1-3a` | Annex 2 (asean2-1.pdf), page 1 | Defines RVC 40%, CC, CTH, CTSH, WO; paragraph 3(a) sets the 10% tolerance for 1803.10, 1803.20, 1805.00 |
| `annex2-3b` | asean2-1.pdf, page 2 | Paragraph 3(b): 7% tolerance for 2103.90 |
| `art25-intro`, `art25-a-i`, `art25-j-k` | Chapter 3 (asean1.pdf), pages 3 to 5 | Annex 2 paragraph 1(e): WO means wholly obtained "as defined in Article 25" |
| `art27-1-3`, `art27-4-6` | asean1.pdf, pages 6 and 7 | Annex 2 paragraph 1(a): RVC "calculated using the formula set out in Article 27", and its note applies Article 27(2)(a) |
| `art28-1a`, `art28-1b-2` | asean1.pdf, pages 7 and 8 | Annex 2 paragraph 3 operates "for the purposes of subparagraph 1(b) of Article 28"; the chapter ranges, the value (FOB) and weight bases and paragraph 2 (tolerance value still counts for RVC) are in the same Article |
| `note-psr-scope` (Unde Labs note) | none | Says, citing Article 26(2), that a listed rule applies in place of the general criteria of Article 26(1), that Article 28 concerns CTC-based rules, and what is not supplied |

Citation, page and URL of each treaty entry are exactly those of the bundle. The list is data: `psr_provisions` (ids, validated against `provisions`, treaty entries only, no duplicates) and `psr_notes` in `ajcep_general.json`.

## What is not returned

- Article 26 (including 26(1) general RVC or CTH criteria). Annex 2 does not cite it, and returning it next to a listed rule could read as an alternative to a stricter rule. The note states that the listed rule applies in place of the general criteria.
- Articles 23, 24, 29, 30, 32 to 35 and Annex 3: not cited by Annex 2 paragraphs 1 to 3. The note names Articles 24 (including 24(c)), 29 (accumulation) and 30 (non-qualifying operations) as not supplied and not verified, as it does Article 31 and Annex 4.
- The no-row fallback is unchanged byte for byte (it does not carry the new note).
- Other agreements are unchanged.

## Tolerance boundaries

The texts are returned as quoted; nothing is paraphrased into a rule. The chapter and code boundaries a reader needs are in the quoted text: Article 28(1)(a) chapters 16, 19, 20, 22, 23, 28 to 49 and 64 to 97 (value, 10% of FOB); 28(1)(b) chapters 18 and 21 (10% or 7%, as specified in Annex 2); 28(1)(c) chapters 50 to 63 (weight, 10% of total weight); Annex 2 paragraph 3 for 1803.10, 1803.20, 1805.00 and 2103.90. A code in a chapter that none of these lists (for example chapter 9 or 15) has no tolerance in the returned text, and an RVC-based rule is not relaxed by Article 28 (paragraph 2). Whether a tolerance applies to a given good stays a case-fact question.

## HS 2022 gap

The bundle is bound to the HS 2017 window by an equality check. The HS 2022 Annex 2 schedule (from 2027-02-15) returns its rows only, with no provisions. The HS 2022 PDF (asean2-2.pdf) carries the same paragraph text according to the audit, but the Chapter 3 pages and tolerance code lists for that edition are not verified here, so nothing is attached. A bundle for that window is a separate change.

## Size effect (offline `jpeval plan -split dev_eval`, 396 eligible requests)

Request bytes total 6,653,925 before and 7,197,105 after (+543,180, +8.2%); max request 35,072 and min 6,318 unchanged. Each AJCEP rule hit adds about 9.1 KB of evidence text (about 60 such requests in the split). Development diagnose counts (dev and dev_eval) are identical; only the bundle digest in the report changes.

## Checks

A lookup hash over every four-digit heading and sampled subheadings, for each agreement at six dates, changes only for AJCEP. `TestOtherEvidenceByteIdentical` still pins AJCEP rows and notes to the earlier hash and adds a second hash for the evidence with provisions.
