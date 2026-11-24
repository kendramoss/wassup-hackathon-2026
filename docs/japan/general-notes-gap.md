# General Notes gap

Each Annex (product specific rules) opens with General Notes: definitions of the
abbreviations used in the rows, value and tolerance rules, and special clauses. A
lookup returns the matched rows plus the captured schedule, section and chapter
notes. For most agreements the General Notes are not captured, so the definitions of
terms used in the returned rows are materially absent from the evidence. Audit date
2026-11-25, against the Japan Customs PDFs pinned in `eval/jp/build_psr.py`. Counts
are rows of the embedded schedule that use a term defined only in the absent text,
and development rulings (the 2,973 dev ids in the split file; the 400-ruling dev-eval set is drawn from them) whose directly matched row uses
such a term. Nothing here changes lookup output.

| Agreement | General Notes (PDF page) | Represented today | Materially absent | Rows using absent-only terms | Dev rulings |
|---|---|---|---|---|---|
| JP-EU EPA | Annex 3-A (separate PDF `eu2.pdf`, not embedded) | Four section notes and one chapter note that point to it | MaxNOM, RVC, EXW, FOB definitions and formulas (Note 4), CC/CTH/CTSH, Notes 3 and 5 to 8 (textile, 10 percent and 8 percent tolerances) | 157 | 457 |
| JP-UK CEPA | Annex 3-A (`uk2.pdf`, not embedded) | Same pointer cells | Same as EU, plus Note 3.4 | 197 | 15 |
| JP-Thailand EPA | 1 to 3 | 20 table notes | (b), (d)(v) to (ix), (f) tolerances, Notes 1 and 2, (g) | 417 (QVC 40) | 6 |
| AJCEP HS 2017 | 1 to 2 | One section XI note; the Chapter 3 and Annex 2 text exists in a separate bundle | The bundle is returned only when no row matches, not with rows; tolerance (seven and 10 percent) applies to many rows | 162 (RVC 40 percent); tolerance-eligible rows 227 | 2 (298 rulings sit on tolerance-eligible rows) |
| AJCEP HS 2022 (from 2027-02-15) | 1 to 2 | None | No bundle at all | 175 | 0 |
| JP-Indonesia EPA | 1 to 6 | None (no table notes) | (b), (d), (e)(i) to (iv) QVC 40 and 50, (g) tolerance, Notes 1 and 2, (h), (i)(i) to (iii), (j) | 159 (QVC) and 47 (chemical terms) | 1 and 1 |
| JP-Australia EPA | 1 to 10 | None | QVC 40 (955 rows), textile codes "(1)" to "(17)" (224 rows hold only a code), CR/D/P/CPS/SM/IS definitions, ch61 to 63 clause | 955 and 224 | 0 |
| JP-Vietnam EPA | 1 to 2 | None | LVC 40 percent, paragraph 3 tolerance | 725 | 0 |
| JP-Chile EPA | 1 to 2 | Six table notes (checked: six in `schedules.json`; the PDF has eight "Note" lines, two of them the General Notes Notes 1 and 2 on page 2, which are not table cells) | Build-up and build-down definitions, Notes 1 and 2 | 133 | 0 |
| JP-Switzerland EPA | Introductory notes 1(a), (b) | None | VNM definition | 13 | 0 |
| JP-India CEPA | 1 | Already represented | none identified | 0 | 0 |
| CPTPP 3-D and 4-A | Section A (pages 1 to 2) | 23 (3-D) and 3 (4-A) table notes in `schedules.json` | Interpretive paragraphs (excluded materials must be originating, alternatives, multiple requirements, group rules) and the dagger marker | 139 (3-D) and 68 (4-A) rows contain "except" | 142 of 595 CPTPP dev rulings have a first covering row with "except" |

Malaysia, Philippines, Mexico and the US TA already have most notes captured or spell
out their value terms in the rows; their remaining absences (tolerance, Notes 1 and
2, IT clause) are listed in the audit and are smaller.

## Australia column 4

The Australia Annex 2 has a fourth column for chapters 27 to 39 holding alternative
origin rules (CR chemical reaction, P purification, SM or IS separation, D
distillation, CPS particle size). It is absent from every row: for example the row
for heading 2901 carries "CTSH" where the PDF reads "CTSH | CR, P, SM or IS". So the
alternative rules are lost, not only their definitions. This needs a parser fix
before a note can be added. Up to about 500 rows are affected; one dev ruling.

## CPTPP dagger marker and appendix

Rows that carry a dagger in the PDF are 12 code ranges: 8407.33-8407.34, 8408.20,
8701.10-8701.30, 87.02-87.05, 87.06, 8708.10-8708.21, 8708.29, 8708.30-8708.40,
8708.50, 8708.80, 8708.94 and 8708.95-8708.99 (PDF pages 96 to 97 and 157 to 161).
They point to the optional regional value content methodology in Appendix 1. The
marker is dropped from every row and the appendix is not returned. Basis: the dagger
lines were read from the PDF text; 12 rows of the embedded Annex 3-D schedule start in
those ranges. Of the 595 CPTPP rulings among the 2,973 dev ids, none is in chapter 84
or 87 and none falls in a dagger range (agreement, code and id only). The "except"
counts above are rows whose text contains the word, and the 142 dev rulings use the
first row that covers the ruling's code; neither count is a claim about outcome.

## EU and UK Annex 3-A

The EU and UK definitions sit in Annex 3-A (`eu2.pdf`, `uk2.pdf`), which is not
embedded. A note is currently tied to the URL of its schedule, so evidence links every
note to the Annex 3-B URL; a note taken from Annex 3-A would be linked to the wrong
document and page.

## Planned follow-ups (not done)

- A per-note source field, so EU and UK notes can cite Annex 3-A pages.
- Schedule-scope notes for the text-layer annexes (Thailand, Indonesia, Vietnam, Chile
  and others), quoted exactly, with PDF page.
- Returning the AJCEP bundle provisions with matched rows, and adding it for HS 2022.
- Australia column 4 parsing, then the General Notes.

Scope notes for two annexes: [thailand-scope-notes.md](thailand-scope-notes.md),
[indonesia-scope-notes.md](indonesia-scope-notes.md). Date windows: [date-windows.md](date-windows.md).
