# JP-Indonesia EPA scope notes: negative finding

The Thailand Annex 2 tables carry "Note:" cells under chapter and section headings
(see `thailand-scope-notes.md`). Indonesia has the same six-column layout, and its
notes had not been audited. This is that audit. Result: the Indonesia Annex 2 has no
scope notes in its tables, so nothing is captured and no code or data changes.

## Source

JP-Indonesia EPA Annex 2 (HS 2017), `indonesia2-2.pdf`, 25 pages,
https://www.customs.go.jp/roo/text/indonesia2-2.pdf, sha256
`b82294d088899ad39d6047320a2e88a4af30d0a6efb4ca2283443ca84464f151` (the digest pinned
in `eval/jp/build_psr.py`). Checked on 2026-11-25.

## Audit

Every page was searched, as table cells and as page text, for "Note", "Notes" and
"for the purposes of". The only "Note" lines in the PDF:

| PDF page | Text starts | Decision |
|---|---|---|
| 2 | "Note 1: The term 'value of non-originating materials' ..." (Article 29 paragraph 6) | Omitted |
| 2 | "Note 2: The term 'value of the good' ..." (Article 29 paragraph 4(b) or 5) | Omitted |

Both sit in the page 1 to 3 General Notes, as definitions for sub-paragraph (g) of the
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
