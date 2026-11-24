# PSR split-row repair

The product-specific-rule (PSR) schedules in `backend/internal/jprules/data/schedules.json`
are generated from the official Japan Customs PDFs by `eval/jp/build_psr.py`. An
audit found rows cut short mid-sentence ("...through", "...from", "...,") and
phantom rows made from the wrapped remainder of another row. This note records
the causes, the fix and what is still open.

## Root causes and fixes

| | Parsers / agreements | Cause | Fix |
|---|---|---|---|
| A | text schedules: Chile, India, Malaysia, Mexico, Philippines | A wrapped line starting with a code-like token ("3901.20 from ...", "11.08 or 35.05", "35 percent") opened a new row, leaving the previous row cut and a phantom row keyed on the cited code. The same happened to wrapped "chapter 1." lines, which reset the chapter. | A line opens a row only when it starts in the classification column. The column edge is derived per PDF from the most common left edge of code-like lines (+15 pt). India stacks rule text under the code at the same left edge, so there only a line holding nothing but the code opens a row. Chapter and section headings must start with a capital letter. |
| B | six-column tables: Thailand (Indonesia checked, unchanged) | Lines of a multi-line cell with no code were dropped, so only the first physical line was kept. | Single-cell code-less lines are appended to the current rule. Notes and column headers are excluded, and so are wrapped "chapter N" lines. |
| C | five-column tables: AJCEP (HS 2022), Vietnam | Rows sharing a merged cell copied its text before later lines arrived; only the last copy was completed. A cell merged across a page break resumes in the first row of the next page. | Continuation lines go to every row sharing the cell. A first-on-page rule that begins lowercase, or follows a rule ending in a dangling word, completes the whole group. |
| D | Malaysia | The committed file had a hand repaired 3809.91-3824.90 row (574 rows); the script rebuilt 575 with a truncated row. | The script now produces the repaired row itself, equal to the text pinned in `TestMalaysiaSplitRowRepaired`. The script and the file agree. |

Only the schedules the script changes were regenerated; EU, UK, both CPTPP,
AJCEP 2017, Switzerland, Australia and Indonesia are byte-identical.

## Rows before and after

| Agreement | Rows before | Rows after | Rows changed or removed | Notes repaired |
|---|---|---|---|---|
| JP-Malaysia EPA | 574 | 339 | 422 | 4 of 7 |
| JP-Mexico EPA | 1640 | 877 | 1117 | 1 of 7 |
| JP-Philippines EPA | 1139 | 716 | 793 | 3 of 6 |
| JP-Chile EPA | 1000 | 680 | 618 | 0 |
| JP-India CEPA | 410 | 385 | 38 | 0 |
| JP-Thailand EPA | 739 | 739 | 147 | 0 |
| JP-Vietnam EPA | 1218 | 1218 | 17 | 0 |
| AJCEP (HS 2022, from 2027-02-15) | 559 | 559 | 22 | 0 |

"Changed or removed" counts old rows with no identical row in the new file.
Most removed rows are phantom rows. The Malaysia count of 575 versus 574 in the
audit was the single unrepaired 3809.91 row.

## Effect on development rulings

Development split, 2973 rulings, agreement, code and date only. Rule availability
(eligible, abstained and the per-agreement reason counts) is unchanged: 2674
eligible, 299 abstained. The returned evidence changed for 50 rulings: Thailand
26, India 19, Mexico 2, Philippines 2, Chile 1. The reason stays "psr" for all
50. For 21 of them the number of evidence rows changed, because a phantom
fragment row is gone.

## Pinned examples (tests in `splitrows_test.go`)

Page numbers are PDF pages.

- Malaysia p28: 3901.10-3901.20 is one complete rule; no phantom 3901.20 row;
  3809.10 ends "except from heading 11.08 or 35.05." with no phantom 11.08 row.
- Mexico p24 2921.41-2921.45, Philippines p21 2922.43-2924.24, Chile p55
  7325.99-7326.90.
- India p3 0902.30-0902.40 and p27 7007 ("not less than 35 percent."); no chapter 35
  row made of "percent".
- Thailand p28 6213-6217 and p9 2001.10: full multi-line cell.
- Vietnam p33 5208 and AJCEP 2022 p24 5111: complete merged cell.
- A structural test over every schedule fails on a row that ends in a dangling
  connective or comma, or starts with a lowercase or "percent" fragment. Its
  allowlist holds one entry, listed below.

## Remaining unknowns

- Six- and seven-column pages (Vietnam about 63 table rows from p15 on, AJCEP 2022
  about 48 rows from p47 on) shift the columns. The parser can read a product
  description as the rule, for example Vietnam 2804.61 ("-- Containing by weight not
  less than"). This is a separate defect, not a split row, and is not repaired here.
  It is the only allowlist entry.
- A cell merged across a page break is completed by the rule above. Other
  page-break shapes may exist; the structural test only finds those that end in a
  dangling word.
- The Indonesia PDF was fetched again to confirm its digest, and its output is
  unchanged. The structural test found no cut rows in it.
