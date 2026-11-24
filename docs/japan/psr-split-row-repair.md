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

| Agreement | Rows before | Rows after | Rows changed or removed | Notes before / after |
|---|---|---|---|---|
| JP-Malaysia EPA | 574 | 339 | 422 | 7 / 9 |
| JP-Mexico EPA | 1640 | 877 | 1117 | 7 / 9 |
| JP-Philippines EPA | 1139 | 716 | 793 | 6 / 8 |
| JP-Chile EPA | 1000 | 680 | 618 | 6 / 6 |
| JP-India CEPA | 410 | 378 | 42 | 0 / 0 |
| JP-Thailand EPA | 739 | 739 | 147 | 0 / 20 (see thailand-scope-notes.md) |
| JP-Vietnam EPA | 1218 | 1212 | 67 | 0 / 0 |
| AJCEP (HS 2022, from 2027-02-15) | 559 | 559 | 44 | 0 / 0 |

"Changed or removed" counts old rows with no identical row in the new file.
Removed rows were phantoms or text read from the wrong cell, with one exception
that the first version of this repair got wrong and the numbered-notes section
below fixes. Several notes were cut after their first words and are now whole.
The Malaysia count of 575 versus 574 in the audit was the single unrepaired
3809.91 row. Merged-cell rows keep the page where the cell starts, so a row's page
can be earlier than the page that holds the end of its text.

## Numbered scope notes

Malaysia and Philippines "Note 1:" and "Note 2:" (textiles, PDF p35-36 and p33-34)
and Mexico "Note 1:" and "Note 2:" (chapter 84, PDF p65) were part of row text
in the old data, so they were found only by chance. They are now notes. Each carries
an `applies` list of code ranges, read from the note's own wording:

- Malaysia and Philippines Note 1: "chapter 50 through 55 and 60".
- Malaysia and Philippines Note 2: the heading list in the note (for example
  5007, 5106-5113 ... 6301-6310 for Malaysia; Philippines differs at 5111-5113,
  5902-5911 and 6301-6308).
- Mexico Note 1: subheading 8471.49. Mexico Note 2: subheading 8473.30.

`build_psr.py` has the table and fails if a numbered note has no entry. The Go side
validates every range when the table loads (2 to 6 digits, equal length, start not
after end), and a lookup returns the note only for a code in a range. A code of the
same chapter outside the ranges (for example 8471.30 or 5601.21) does not get it.
The scope of the note stays "chapter" under a chapter heading and "section"
otherwise. A check of every schedule found no other old row text holding a numbered
note that is now missing.

## India appendix

India p41-42 is a textile process-requirement table ("Appendix to Annex 2"), not
product rules. The parse stops at that heading, so those rows (and a 9612.10 row
that had the appendix header appended) are gone. The appendix is not embedded. No
real rule was lost: every other removed India row was a fragment such as ")" or
a code list cut from a wrapped line.

## Effect on development rulings

Development split, 2973 rulings, agreement, code and date only. Returned evidence
changed for 51 rulings: Thailand 26, India 19, Mexico 2, Philippines 2, Chile 1,
Vietnam 1. For 50 the reason stays "psr". The Vietnam ruling (code 2817.00) went
from "psr" to "no_match": its only evidence was the row for chapter 28, whose
"rule" was the chapter title, and the PDF lists no rule for 2817. So eligible is 2673
(was 2674), PSR 2572 (was 2573), abstained 300 (was 299); general and mapped counts
are unchanged.

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
  connective, comma or hyphen, or starts with a lowercase or "percent" fragment.
  Its allowlist is empty.

## Wide table pages

Vietnam p15 on (6 columns) and AJCEP 2022 p47 on (7 columns) have extra empty
columns, so the rule sits in the last column. The parser took the fifth column,
which gave 59 Vietnam and 19 AJCEP rows whose "rule" was the product description
or a chapter title. Rows now take the codes from the first three columns, the rule
from the last and the description from those between. A test fails if any
Vietnam or AJCEP 2022 row text starts with "-- ".

## Remaining unknowns

- A cell merged across a page break is completed when the first row of the next
  page starts lowercase or the cell ends in a dangling word. Other page-break shapes
  may exist.
- The structural test also treats a trailing "any" or hyphen as a cut. Its
  allowlist is empty.
- `code_column` raises when the most common code-like left edge covers under half
  of the code-like lines or lies outside 40-150 pt, so a new PDF layout fails loudly.
- Pure helpers have unit tests in `eval/jp/test_build_psr.py`
  (`python3 -m unittest test_build_psr` from `eval/jp`).
