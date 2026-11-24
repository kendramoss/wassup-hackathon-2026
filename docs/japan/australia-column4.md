# JP-Australia EPA: column 4 and the General Notes

The Annex 2 table of the JP-Australia EPA (HS 2012, from 2015-01-15; PDF
`australia2.pdf`, digest pinned in `eval/jp/build_psr.py`) has four columns. The
parser kept column 3 only, so the alternative rules of column 4 were lost, and
the General Notes that define the rule codes were not in the data. This note
records what the PDF says, what is now stored and what is still open.

## What the PDF shows

- The table starts on PDF page 11. Its header row reads "Tariff Item Number
  (HS 2012)", "Description of Goods", "Product Specific Rules", and the third
  header cell spans columns 3 and 4. Right under it the PDF prints:
  "Note: Goods which satisfy the applicable rules set out in column 3 or column 4
  below shall be considered as originating goods of a Party."
- General Notes paragraph 1: the rule "is set out in column 3 or 4". Paragraph 2:
  "When a chapter, heading or subheading is subject to alternative specific rules
  of origin, the rule will be considered to be met if a good satisfies one of the
  alternatives." Both wordings say the columns are alternatives ("or"), not
  cumulative. Nothing beyond that wording is inferred.
- Paragraph 6 opens "For the purposes of column 4 of the product specific rules
  set out in this Annex:" and defines six codes: "CR" (Chemical Reaction Origin
  Rule), "D" (Distillation Rule), "P" (Purification Origin Rule), "CPS" (Change in
  Particle Size Origin Rule), "SM" (Standards Materials Origin Rule) and "IS"
  (Isomer Separation Origin Rule). Paragraph 5 opens "For the purposes of column 3
  ..." and defines "QVC 40" and the codes "(1)" to "(17)". Paragraph 7 concerns
  chapters 61 to 63.
- Column 4 is non-empty in 530 table rows, on PDF pages 21 to 52, in chapters 27
  to 40 (the audit named 27 to 39; the PDF also fills chapter 40, 30 rows, and
  paragraph 6(a) itself speaks of "chapters 27 to 40"). Rows per chapter: 27: 19,
  28: 81, 29: 163, 30: 14, 31: 5, 32: 39, 33: 14, 34: 20, 35: 13, 36: 1 (the
  chapter-level row), 37: 7, 38: 55, 39: 69, 40: 30. Every listed row of
  chapters 27 to 40 has one. The cell holds one of seven strings: "CR" (86),
  "CR or D" (5), "CR or P" (33), "CR, P or IS" (14), "CR, P or SM" (44),
  "CR, P, CPS or IS" (55), "CR, P, SM or IS" (293).
- There is no row-specific footnote in the table apart from the header note
  above. The "Note:" under the definition of QVC 40 (paragraph 5(d), Article 3.5
  paragraph 2) belongs to that definition and is kept inside its note.

## What is stored

Row text. A row with a column 4 reads
`Rule (column 3): <column 3 text>; alternative (column 4): <column 4 text>`, for
example `Rule (column 3): CTH; alternative (column 4): CR, P, SM or IS`. Both cell
texts are verbatim. "alternative" follows paragraph 2 and the header note. Rows
with an empty column 4 are unchanged (1,638 of the 2,168 rows).

Notes. 28 schedule-scope notes, each verbatim from the PDF text with the page
number removed and each pinned to the PDF page where it starts:

| Page | Note | Attached to |
|---|---|---|
| 1 | paragraphs 1 and 2 (column 3 or 4; alternatives) | chapters 27 to 40 |
| 1 | paragraph 3 (a CTC applies only to non-originating materials; each must undergo it) | rows whose column 3 or 4 uses CC, CTH or CTSH (25 ranges, 1,944 rows) |
| 2 | 5(d) "QVC 40", with its Note | rows whose column 3 uses "QVC 40" |
| 2 to 7 | 5(e) to 5(u), codes "(1)" to "(17)", one note per code | rows whose column 3 uses that code |
| 7 to 10 | 6(a) to 6(f), CR, D, P, CPS, SM, IS, one note per code | rows whose column 4 uses that code |
| 10 | paragraph 7 (chapters 61 to 63) | chapters 61 to 63 |
| 11 | the table header note quoted above | chapters 27 to 40 |

Each definition note starts with its paragraph's opening line ("5. For the purposes
of column 3 ..." or "6. For the purposes of column 4 ...") so it reads on its own.
A note's `applies` list holds 6-digit ranges built from the rows that use the
code (consecutive rows merge, and a range never covers a row that does not use the
code); a test checks, for every row, that a definition is attached exactly when the
row's own text uses the code. A lookup therefore returns only the definitions a
row needs: a "CR or D" row gets CR and D but not P, CPS, SM or IS.

All-schedule definitions (paragraph 5(d), the codes) are schedule scope as
required; none is a row footnote. Paragraph 3 changes how CC, CTH and CTSH read
(each non-originating material must undergo the CTC; the requirement applies only
to non-originating materials), so it is embedded and attached to every row whose
rule text uses one of them. Rows that use only a "(n)" code do not name CC, CTH or
CTSH in their own text and do not get it, although the definitions of those codes
use them.

Still omitted, with reasons: paragraph 4 (definitions of chapter, heading, section
and subheading as digit prefixes; the lookup already works on those digits);
paragraphs 5(a) to 5(c) (CC, CTH, CTSH meanings; the prompt gives a generic gloss
of them, and the PDF text is not embedded here); paragraph 8 (the Annex is based
on HS 2012, which the schedule citation already states).

## Build

`eval/jp/build_psr.py` (`australia`, `australia_general_notes`,
`australia_schedule`) produces the rows and notes. Only the Australia entry of
`schedules.json` was regenerated and spliced into the file from current main; the
other 17 schedules are JSON-equal to main. `eval/jp/australia_column4_truth.py`
dumps the raw column 4 cells (no parser logic) to
`backend/internal/jprules/testdata/australia-column4.tsv`, which the Go test uses
as the PDF truth: each of the 530 cells must sit in exactly the row of its page,
and no other row may carry a label. `python3 -m unittest test_build_psr` runs the
helpers; with `JP_PDF_DIR` set to the directory holding the PDF it also checks the
parser against the PDF.

## Effect

- Lookup hash diff against main over all 5,387 HS 2017 codes, every agreement and
  three dates (274,737 lookups): only JP-Australia EPA changed (12,855 lookups);
  no other agreement changed.
- Evidence size per Australia lookup, in characters of rule and note text added:
  chapters 27 to 40 (530 rows) mean 3,764, max 5,054; chapters 50 to 63 rows
  with a "(n)" code (299 rows) mean 523, max 663;
  other rows that use "QVC 40" about 520; the mean over all 2,168 rows is 1,409 (it was 1,101 before paragraph 3), and the 224 rows without CC, CTH or CTSH in their text add less. The notes hold
  15,360 characters in all.
- Development rulings, `jpeval diagnose`: 18 JP-Australia rulings in `dev` (2,973),
  1 of them in chapters 27 to 40 (ruling 1251362, code 3504.00); the other 17 are
  chapters 02, 04, 09 and 10. Eligible, abstained and reason counts are identical
  before and after for `dev` and `dev_eval`. Evidence grows for the chapter 27 to
  40 ruling (row plus four notes) and by the paragraph 3 note (343 characters) for
  rulings whose rule text uses CC, CTH or CTSH. `jpeval plan -split dev_eval`
  (1 Australia case, 0902.20): request bytes total 5,130,506 before, 5,130,908
  after (+402), max request unchanged at 35,072, `requests_sha256` changes. These
  counts show evidence availability only, not accuracy.

## Not done

- The meaning of a rule combination such as "CTH or QVC 40" next to a column 4 is
  not interpreted; both are shown as the PDF prints them.
- The listed exceptions inside the P, SM and IS definitions (code lists) are
  inside the note text and are not turned into per-row logic.
