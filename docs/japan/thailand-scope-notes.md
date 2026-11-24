# JP-Thailand EPA scope notes

The Annex 2 tables of the JP-Thailand EPA (HS 2017, from 2023-01-01) carry "Note:"
and "Note N:" cells under chapter and section headings. The six-column parser
dropped them: Thailand had 0 notes. `eval/jp/build_psr.py` now captures them for
this file (`SCOPE_NOTE_PDFS`). A note runs from its "Note" line to the next rule
row or heading, so its sub-lists "(a)", "(b)", "(i)" are kept, and a note cut by a
page break is joined. The PDF is `thailand2-2.pdf` (digest pinned in the script).
Thailand rows are unchanged and every other schedule is byte-identical.

## Captured: 20 notes

| PDF page | Scope | Text starts | Applies (from the note's own wording) |
|---|---|---|---|
| 4 | chapter 03 | "For the purposes of subheadings 0306.16, 0306.17 ..." | 0306.16, 0306.17, 0306.35, 0306.36, 0306.95 |
| 5 | chapter 07 | "... subheadings 0710.22 and 0710.29" | those two |
| 6 | chapter 16 | Note 1, "subheading 1604.14" | 1604.14 |
| 6 | chapter 16 | Note 2, "subheadings 1604.13, 1604.15, 1604.20, 1605.21 and 1605.29" | those five |
| 7 | chapter 18 | "subheading 1803.20 and heading 18.05" | 1803.20, 1805 |
| 8 | chapter 19 | "subheading 1905.90" | 1905.90 |
| 9 | chapter 20 | "subheadings 2001.10 through 2005.60, 2005.80 through 2007.10, 2008.11, 2008.19, 2008.40 through 2008.91, 2009.61, 2009.69, 2009.81 and 2009.89" | exactly those ranges |
| 12 | chapter 27 | "this chapter, the term chemical reaction" | whole chapter |
| 12 | section VI | "chapters 28 through 38" | 28-38 |
| 20 | section VII | "chapters 39 and 40" | 39-40 |
| 23 | section XI | "chapters 50 through 55 and 60" | 50-55, 60 |
| 26 | chapter 61 | Note 1, "a good of this chapter" | whole chapter |
| 27 | chapter 61 | Note 2, "this chapter" | whole chapter |
| 27 | chapter 62 | Note 1 and Note 2, "this chapter" | whole chapter |
| 28 | chapter 63 | "a good of this chapter" | whole chapter |
| 29 | chapter 71 | "this chapter" (pearls) | whole chapter |
| 29 | chapter 72 | "this chapter" | whole chapter |
| 30 | chapter 73 | "this chapter" | whole chapter |
| 35 | chapter 87 | "headings 87.02 through 87.04" | 8702-8704 |

Where a note names codes, a code of the same chapter outside them does not get the
note (for example 0306.19, 1604.19, 2005.70, 8705). A note that says "this chapter"
applies to the whole chapter. A note under a section heading whose text names a
chapter range gets the section scope with that range. Every note needs an entry in
`NOTE_APPLIES` (keyed file, page, label, heading); the build fails otherwise.

## Not captured: 2 notes

PDF page 3, "Note 1:" ("value of non-originating materials", Article 28 paragraph 6)
and "Note 2:" ("value of the good", Article 28 paragraph 4(b) or 5). They sit in
Section 1 General Notes, sub-paragraph (f), and define terms for the whole annex.
They have no chapter or section, and a note of the table can only be scoped to one,
so they are left out rather than attached to a guessed scope.

Indonesia (same six-column layout) is not changed here; its notes were not audited.

## Effect on development rulings

Development split, 2973 rulings, `diagnose -split dev` before and after: eligible,
abstained, reason and agreement counts are identical. Returned evidence changed for
25 rulings, all Thailand "psr" (of 33 Thailand development rulings), 41 note
entries added. Before, 3 development rulings returned any note (Philippines 2,
Mexico 1).
