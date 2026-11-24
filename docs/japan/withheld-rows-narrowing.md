# Withheld rows: narrowing and inventory

A withheld (blocked) row makes a lookup return `withheld` and no text, because the
official row depends on a separate provision the table does not embed. Before this
change 41 rows were withheld in three agreements. This note records what each one
rests on, which two were narrowed, and which stay withheld. Rule: a block is
narrowed only where the official footnote or appendix proves the exact limit.

Sources (Japan Customs, all `https://www.customs.go.jp/roo/english/text/<file>`,
retrieved 2026-11-25; page numbers are PDF page indexes):

| File | Content | sha256 |
|---|---|---|
| `eu3.pdf` | Japan-EU EPA Annex 3-B | `cb7f029ad6679a70796ba45f619e1a623d8e9304ed041069a4b007cdc9633704` |
| `eu4.pdf` | Japan-EU EPA Appendix 3-B-1 (9 pages); not pinned in `build_psr.py` | `09a97d0537791b26aa44ddb9f6194660b4831e48adcac6d1acaad51aabcb71e6` |
| `tpp2.pdf` | CPTPP Annex 3-D | `c80acd1e0c458f130cc6d6602ce18192626e801623947fffc9d1ed18020873b9` |
| `tpp3.pdf` | CPTPP Annex 3-D Appendix 1 (3 pages); not pinned in `build_psr.py` | `9837bb9ff738b8148ebdc3f5a06a379dad1cac47918c99cd48065c58e88b24bd` |
| `switzerland3.pdf` | Japan-Switzerland Appendix 1 to Annex II | `ba550092a9ad93cba330b298a1e88064d060a324e271e944a1435b2503e807f5` |
| `uk3.pdf` | Japan-UK CEPA Annex 3-B | `7caabd2e36f02f6794cff41501ffa4ba339a6603b4c5c462bc4bedc7e371f9eb` |

`eu4.pdf` and `tpp3.pdf` are only read for this analysis; no text of them is embedded.

## Inventory before the change: 41 rows

| Agreement | Rows | Why blocked |
|---|---|---|
| JP-EU EPA | 4 | the code cell carries a footnote digit ("see also Appendix 3-B-1") |
| CPTPP | 12 | the code cell carries a dagger (Annex 3-D Appendix 1, optional RVC route) |
| JP-Switzerland EPA | 25 | a cell lists several codes, or an "except for" line, and was merged |

## What changed

### JP-EU 7007-7009 (eu3.pdf p55)

Footnote 1 reads: "For the products of subheadings 7007.11 and 7007.21, see also
Appendix 3-B-1." Appendix 3-B-1 names 7007 only in its Section 3 table (eu4.pdf p4:
7007.11 and 7007.21). The one row became three:

| Code | Rule (p55) | State |
|---|---|---|
| 7007.11 | CTH; MaxNOM 50 % (EXW); or RVC 55 % (FOB). | withheld |
| 7007.21 | same | withheld |
| 7007 to 7009 | same | normal row, carries the footnote as a note on page 55 (applies 7007-7009) |

The width-6 rows win for 7007.11 and 7007.21; 7007.19, 7007.29, 7008 and 7009 take
the heading row. A bare four-digit "7007" is now `too_broad` instead of `withheld`
(still no text).

### JP-Switzerland 3501-3505 (switzerland3.pdf p10)

The merged row "CC CC except from chapter 4. CTH CC except from chapter 11." is
split into the four source lines. Column (4) is empty in all four, so each rule is
column (3) alone; the table header says products satisfying field (3) or (4) are
originating, and no "or" is needed here.

| Code | Rule |
|---|---|
| 35.01 | CC |
| 3502.11-3502.19 | CC except from chapter 4. |
| 3502.20-35.04 | CTH |
| 35.05 | CC except from chapter 11. |

### JP-UK: rows that were silently missing (uk3.pdf p57, p65, p70)

The Japan-UK PDF has the same footnotes, but its code cells carry the footnote digit
glued to the code ("70.071 -70.09", "84.07-84.081", "87.011", "87.063",
"87.085-87.11"). The UK reader did not recognise them, so those rows were dropped and
a lookup returned `no_match` (for 7007 a mix of unrelated chapter-70 branch rows).
They are now rows, withheld as in the EU:

| Code | Page | State |
|---|---|---|
| 7007-7009 | 57 | withheld (the whole range; the UK is not narrowed) |
| 8407-8408 | 65 | withheld |
| 8701, 8702-8705, 8706, 8707, 8708 | 70 | withheld |
| 8709-8711 | 70 | normal row (the 87.08-87.11 cell has the footnote on 87.08 only) |

Side effects of the same fix in the UK data: the pollution of three neighbouring rows
(8401-8406, the chapter 70 "Others" branch and 7010, which had received the dropped
rows' rule text) is gone. Not fixed, out of scope: the UK header-only rows 70.06 and
70.10 still copy the previous rule instead of their own branch rules.

## What stays withheld, and why

| Rows | Why |
|---|---|
| JP-EU 8407-8408, 8701-8707, 8708 | The footnote covers the whole range and Appendix 3-B-1 has year-dependent thresholds (Section 2, from 2019-02-01) and per-subheading alternatives (Section 3). Not representable as plain evidence. |
| JP-UK same ranges and 7007-7009 | Same appendix; kept withheld as instructed. |
| CPTPP 12 dagger rows | The appendix gives an optional, more lenient RVC route (tpp3.pdf p1-3, Tables A to C). The base rule is complete, but unblocking needs a note that cites tpp3, not tpp2. No dev case. |
| JP-Switzerland other 24 | Each merged row needs a per-line split pinned to the PDF (same method as 3501-3505). |

A test lists every withheld row, so a new block must be added deliberately.

## Dev effect (`jpeval diagnose -split dev`, 2,973 cases)

JP-EU withheld 1 to psr 1 (7007.29); Switzerland withheld 1 to psr 1 (3504.00);
eligible 2,950 to 2,952; abstained 23 to 21. JP-UK unchanged. These counts show rule
availability only, not accuracy.
