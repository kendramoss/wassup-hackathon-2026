# Japan-UK CEPA: heading rows with an empty rule cell

Source: Japan-UK CEPA Annex 3-B, `https://www.customs.go.jp/roo/english/text/uk3.pdf`
(sha256 `7caabd2e36f02f6794cff41501ffa4ba339a6603b4c5c462bc4bedc7e371f9eb`, retrieved
2026-11-25; page numbers are PDF page indexes). This is a source and parser
correctness fix, not an accuracy result.

## Defect

In the PDF a heading such as 70.06 has an empty rule cell; its rules are the "-"
lines below it. `uk()` in `eval/jp/build_psr.py` copied the previous row's rule onto
38 such headings and stored the "-" lines as chapter-keyed rows (94 in total), which
the copied heading row shadowed because a lookup returns only the most specific
width. Three headings (61.01-61.17, 63.01-63.04, 1901.10) were dropped, so their
branches fell back to chapter-wide rows. Seven copied rows cited the previous row's
page (1515, 5602, 5902, 6202, 6212, 6217, 7010).

Example, 70.06 (p57): the PDF has "Coated glass-plate substrates: CTH; or Production
from non-coated glass-plate substrate of heading 70.06." and "Others: CTH except from
headings 70.02 to 70.05." The table returned the 7001-7005 rule instead.

## Fix

A code row with an empty rule cell is a heading. It never takes the previous row's
rule. Each "-" line below it becomes one row keyed to the heading's codes, with the
line label as description (a lookup shows "label: rule") and the line's own page. The
build now fails on a heading without rule lines, a rule line without a rule, a rule
line that follows neither a heading nor a chapter title, and any row without text.
Merged-cell copying in the other parsers (AJCEP, Vietnam, five-column and six-column
layouts) is untouched. uk3.pdf has no merged or continued rule cells: every code
row has its own cell, and the one cell shared by several codes (87.085-87.11, footnote
5) is split by `UK_FOOTNOTES` with its own text.

## Counts (JP-UK schedule only)

| Item | Count |
|---|---|
| Rows before / after | 462 / 424 |
| Copied heading rows removed | 38 |
| Chapter-keyed "-" line rows re-keyed to their heading (same text, label, page) | 92 |
| Chapter-keyed rows kept (chapter 3 tuna lines, directly under the chapter title) | 2 |
| Headings restored (6101-6117, 6301-6304, 190110) | 3, with 7 branch rows |
| Rows added / removed (exact multiset diff) | 92 / 130 |
| Other schedules | JSON-identical to main |

Old 94 chapter-keyed rows map to 2 unchanged plus 92 re-keyed. The 130 removed rows
are those 92 (old key) plus the 38 copied rows. Footnote and withheld rows (7007-7009,
8407-8408, 8701-8708) are unchanged.

Lookups over all 5,387 HS 2017 codes, two dates: 198 codes (3.7 %) return different
UK evidence, all across the 41 heading spans, and every one is `psr` before and
after. No other agreement changes. Example: 1901.10 returned 10 mixed rows (chapter
19 branches of 1901.10, 1901.20 and 1901.90) and now returns its two rows; 70.06
returned one wrong rule and now returns two. Dev diagnosis (2,973 cases) is
unchanged: eligible 2,952, JP-UK 24 psr. 0 of 24 dev UK rulings hit a copied row;
1240583 (6116.91) returns the same two branch rows with the same page, now
keyed to 61.01-61.17 instead of chapter 61.

## Remaining gaps

- A lookup for a heading now returns two or more branch rows; choosing the branch
  from the product description is left to the reader of the evidence.
- 63.01-63.04: the PDF cell reads "- Others -- Embroidered" followed by
  "-- Others"; the labels are stored as printed, not interpreted.
- No heading was withheld: every heading's lines parsed unambiguously.
- Not checked: other agreements' text-layout parsers beyond the code-path review in
  the inherited-rows audit.
