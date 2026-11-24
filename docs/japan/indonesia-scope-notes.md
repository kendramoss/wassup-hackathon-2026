# JP-Indonesia EPA scope notes

The Thailand Annex 2 tables carry "Note:" cells under chapter and section headings
(see `thailand-scope-notes.md`). Indonesia has the same six-column layout, and its
notes had not been audited. This is that audit. Result: the tables carry no "Note"
cells under chapter or section headings, so no table scope note is captured and no
code or data changes. This is not a finding that the annex has no notes: its General
Notes (PDF pages 1 to 6) are not in lookup evidence today, and that is material (next
section).

## Source

JP-Indonesia EPA Annex 2 (HS 2017), `indonesia2-2.pdf`, 25 pages,
https://www.customs.go.jp/roo/text/indonesia2-2.pdf, sha256
`b82294d088899ad39d6047320a2e88a4af30d0a6efb4ca2283443ca84464f151` (the digest pinned
in `eval/jp/build_psr.py`). Checked on 2026-11-25.

## General Notes (PDF pages 1 to 6), materially absent from lookup evidence

A lookup returns the matched rows and the captured scope notes (none for Indonesia).
The following General Notes of this Annex 2 PDF are not returned:

- (b) weight means dry weight; (d) a change in tariff classification (CTC) applies to
  each non-originating material and only to non-originating materials;
- (e)(i) to (iii) the definitions of CC, CTH and CTSH;
- (e)(iv) "QVC 40" and "QVC 50": qualifying value content of at least 40 or 50 percent
  by the formula in Article 29 paragraph 4(b), and the final process of production
  performed in a Party. About 159 schedule rows use these terms;
- (g) tolerances: 10 percent by value (chapters 28 to 49 and 64 to 97) and seven
  percent by weight (chapters 50 to 63), with Notes 1 and 2 (the value terms);
- (h) definitions of chemical reaction, purification, isomer separation and
  biotechnological processes, for chapters 28 to 40 (about 47 rows);
- (i)(i) two or more of 48 listed finishing operations for dyeing or printing in
  chapters 50 to 55 and 60; (i)(ii) direct transport of materials processed in an
  ASEAN non-Party for listed headings; (i)(iii) the component rule for chapters 61 to 63;
- (j) the Information Technology Agreement clause, except subheadings 8541.10 to 8542.90.

This covers the Annex 2 PDF only. It says nothing about other provisions of the
treaty and is not a claim that no other conditions apply. See `general-notes-gap.md`.

## Audit

Every page was searched, as table cells and as page text, for "Note", "Notes" and
"for the purposes of". The only "Note" lines in the PDF:

| PDF page | Text starts | Decision |
|---|---|---|
| 2 | "Note 1: The term 'value of non-originating materials' ..." (Article 29 paragraph 6) | Omitted |
| 2 | "Note 2: The term 'value of the good' ..." (Article 29 paragraph 4(b) or 5) | Omitted |

Both sit on page 2, within the page 1 to 6 General Notes, as definitions for sub-paragraph (g) of the
whole annex. They have no chapter or section, and a captured note must be scoped to
one, so they are omitted rather than attached to a guessed scope (same reasoning as
Thailand p3 Notes 1 and 2).

The other "for the purposes of" passages are numbered General Notes paragraphs, not
note cells under a heading: (h) chapters 28 through 40 (chemical reaction), (i)(i)
chapters 50 through 55 and 60, (ii) headings 50.07 and others, (iii) a good of
chapters 61 through 63. They are not "Note" cells and the parser does not treat them
as notes. Capturing them would need a separate reviewed change. The chapter and
section pages (6 to 25) contain no "Note" cell at all.

## Check against the parser

Running `six_column` on this PDF with Indonesia added to `SCOPE_NOTE_PDFS` returned 0
notes and the same 376 rows as now. So there is nothing for `NOTE_APPLIES` to
review, and Indonesia is left out of `SCOPE_NOTE_PDFS`.

## Effect

`schedules.json` is unchanged, so development rulings, counts and returned evidence
are unchanged. No Indonesia ruling gains a note.
