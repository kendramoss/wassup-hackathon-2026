# General Notes gap

Each Annex (product specific rules) opens with General Notes: definitions of the
abbreviations used in the rows, value and tolerance rules, and special clauses. A
lookup returns the matched rows plus the captured schedule, section and chapter
notes. For most agreements the General Notes are not captured, so the definitions of
terms used in the returned rows are materially absent from the evidence. Audit date
2026-11-25, against the Japan Customs PDFs pinned in `eval/jp/build_psr.py`. Counts
are rows of the embedded schedule that use a term defined only in the absent text,
and development rulings (dev and dev-eval ids, 3,373) whose directly matched row uses
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
| JP-Chile EPA | 1 to 2 | Six table notes | Build-up and build-down definitions, Notes 1 and 2 | 133 | 0 |
| JP-Switzerland EPA | Introductory notes 1(a), (b) | None | VNM definition | 13 | 0 |
| JP-India CEPA | 1 | Already represented | none identified | 0 | 0 |
| CPTPP 3-D and 4-A | Section A (pages 1 to 2) | 23 and 3 table notes | Interpretive paragraphs (excluded materials must be originating, alternatives, multiple requirements, group rules) and the dagger marker | See below | See below |

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

Rows that carry a dagger in the PDF (about 12 code ranges, mainly chapters 84 and 87)
point to the optional regional value content methodology in the appendix (PDF page 96
of the Annex 3-D file). The marker is dropped from every row and the appendix is not
returned. 160 rows use "except" or RVC wording and rely on Section A paragraph 3
interpretation; no dev ruling was found needing the dagger rows.

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
