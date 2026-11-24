# General Notes of the text-layer Annexes, embedded

Nine agreements (JP-Thailand, JP-Indonesia, JP-Malaysia, JP-Philippines, JP-Mexico,
JP-Chile, JP-Vietnam, JP-Switzerland, CPTPP Annex 3-D and 4-A) open their product
specific rules with General Notes that define terms the rows use or change how a rule
qualifies. They were not in the evidence (see [general-notes-gap.md](general-notes-gap.md)).
46 of them are now schedule notes, each a verbatim chunk of its own annex PDF with its
own page, `source_url` and `citation`.

## How a note is built

- Source: the schedule's own PDF (digest pinned in `DIGESTS` of `eval/jp/build_psr.py`,
  re-verified against fresh data-only copies on 2026-11-25). The note's `source_url` is
  that PDF and the evidence links to `#page=N`.
- Text: the page text as the PDF text layer gives it, line breaks and hyphenated breaks
  joined (`normalize`), cut between two anchors (`GENERAL_NOTES`, `quote_general`). The
  start must be unique on its page and the end must occur exactly once after it, so a
  changed PDF fails the build. No page footer, page number or table row is in a note.
  Nothing is paraphrased.
- Citation: `<agreement> <annex> General Notes, <paragraph>`, then `PDF page N`. A
  paragraph that continues on the next page continues in the next note, which names the
  same paragraph.
- Scope: schedule scope for all of them. `applies` is set only where the paragraph's own
  words name chapters, headings or subheadings; otherwise the note covers the whole annex.
- Where a provision points to the main Agreement (Article 28, 29, 30, 27 and so on, a
  separate PDF) the note quotes the Annex words and does not supply the Article.
- Precedence: none of these notes is a rule. The ITA clauses concern materials used in
  another good, not an alternative rule for the good looked up; the CPTPP interpretive
  paragraphs say how the listed rule is read, not what replaces it.

## What is embedded (pages are PDF pages)

| Agreement | Notes | Provisions (page) | `applies` |
|---|---|---|---|
| Thailand | 7 | (b)+(c) CTC only on non-originating materials, dry weight (1); (d)(v) QVC 40 (1); (d)(vi) to (ix) CC, CTH, CTSH, WO (2); (f) tolerance 7% and 10% value (2) and 10% weight (3); Notes 1 and 2 (3); (g) ITA (3) | (f): ch 19-24, 28-97; (f)(iii): 50-63 |
| Indonesia | 13 | (b) dry weight (1); (d), (e)(i)-(ii) (1); (e)(iii)-(iv) CTSH, QVC 40/50 (2); (g) tolerance (2); Notes 1 and 2 (2); (h) chemical reaction, purification, isomer separation, biotechnological (2-3); (i)(i) dyeing/printing with two finishing operations, 48 items (3-5); (i)(ii) direct transport (6); (i)(iii) ch 61-63 component (6); (j) ITA (6) | (g): 28-97; (h): 28-40; (i)(i): 50-55, 60; (i)(ii): the headings and chapters it lists; (i)(iii): 61-63 |
| Malaysia | 4 | (b) dry weight; (e) tolerance; Notes 1 and 2; (f) ITA with 8541.10-8542.90 CTC | (e): 28-97 |
| Philippines | 5 | (b)+(c) dry weight, no priority among alternatives; (f) tolerance (1-2); Notes 1 and 2; (g) ITA | (f): 28-97 |
| Mexico | 5 | (b) CTC only on non-originating; (c) dry weight; (f), (f)(i)-(iii), (g) the 200,000,000 USD annual aggregate alternative for ch 61-63 | (c): 01-24; (f), (g): 61-63 |
| Chile | 4 | (b)+(c) dry weight, CTC only on non-originating; (d) Build-down and Build-up methods; (f) tolerance; Notes 1 and 2 | (f): ch 19, 2001.10-2008.91, 2008.92, 2008.99-2009.90, 21, 28-97 |
| Vietnam | 3 | 1 LVC 40% with its Note, CC, CTH, CTSH, WO (1); 3 tolerance of 10% and seven percent of the FOB (1-2) | 3: 0901.21-.22, 1803.10-.20, 1805.00, 2103.90; 3(b): 2103.90 |
| Switzerland | 2 | 1(a) VNM, CC, CTH, CTSH, WO (1); 1(b) non-textile materials disregarded (2) | 1(b): 61-63 |
| CPTPP 3-D | 2 | 3(c)-(g) CTC only on non-originating, excluded materials must be originating, alternatives, multiple requirements, group rule (2); 5 optional RVC methodology for the dagger goods of ch 84 and 87 (2) | 5: 84, 87 |
| CPTPP 4-A | 1 | 3(c)-(g) as above, and the Short Supply List is read with the rules (2) | none |

Readings worth checking. Notes 1 and 2 define the two values the formulas and tolerance
use and carry no chapter in their wording, so they are schedule notes without `applies`.
The first note of a paragraph that continues (Thailand (f), Vietnam paragraph 3, Indonesia
(i)(i)) carries the lead-in, so it has the union of the ranges named after it. The CPTPP
dagger is not on any row (see the gap note), so paragraph 5 is attached by its own words,
to chapters 84 and 87, not to the 12 dagger ranges. Chile's `applies` use the PDF's HS 2002
wording; the lookup compares digits and does no HS edition conversion.

## Cut, with reasons

- Every "adjacent rule" paragraph ((a) or 3(a)) and every digit-prefix definition of
  section, chapter, heading and subheading: the lookup already works on the digits and
  neither changes qualification.
- Every HS edition statement ((e), (f), 2, 1(d)): the schedule citation states the edition.
- CPTPP paragraph 1 (definitions), 2 (what an originating good is; a general criterion
  that must not read as an alternative to the listed rule), 3(a), 3(b) (chapter and
  heading notes are read with the rules; the notes returned state their own content) and
  4 (textile goods are in Annex 4-A; a routing statement).
- Australia and India were not in scope: Australia has its own note
  ([australia-column4.md](australia-column4.md)); India's Annex notes define nothing the
  rows use. AJCEP is not text-layer and is not covered here.

Borderline kept: the dry weight clauses (the rows or the embedded tolerance count weight).
Borderline dropped: CPTPP 3(b), as above.

## Effect

`jpeval plan -split dev_eval`, offline, 396 eligible requests, before and after:

| | before | after |
|---|---|---|
| total request bytes | 6,654,327 | 6,777,520 (+123,193) |
| average request | 16,804 | 17,115 |
| largest request | 35,072 | 35,072 |
| requests that changed | | 93 (83 CPTPP, 4 Indonesia, 3 Thailand, 1 Malaysia, 1 Mexico, 1 Philippines) |

Bytes added per changed request: CPTPP 1,135 to 1,154; Indonesia 3,610; Thailand 2,854 to
3,658; Malaysia 2,202; Philippines 1,712; Mexico 445. No Chile, Vietnam or Switzerland
case is in `dev_eval`. Note text added to `schedules.json`, in characters and as JSON
bytes: Thailand 3,076 and 4,587; Indonesia 6,998 and 10,091; Malaysia 1,889 and 2,721;
Philippines 1,969 and 3,089; Mexico 1,507 and 2,671; Chile 1,377 and 2,321; Vietnam 1,925
and 2,601; Switzerland 1,619 and 2,094; CPTPP 3-D 1,391 and 1,858; CPTPP 4-A 1,055 and
1,274 (22,806 characters, 33,307 JSON bytes in all). This is an evidence addition. No
accuracy change is claimed or measured.

`jpeval diagnose` on `dev` and `dev_eval` gives identical counts before and after (only
the revision and the schedules digest differ). A lookup hash over all 5,387 HS 2017 codes,
every agreement and three dates (258,576 lookups) is identical to main for rule rows and
old notes; only the nine agreements gained the notes above (JP-Thailand 5,386 lookups,
Indonesia 5,386, Vietnam 6,476, Switzerland 2,114, CPTPP 10,566, Malaysia 16,020, Philippines
15,948, Chile 15,969, Mexico 15,867). In `TestOtherEvidenceByteIdentical` the old golden
hashes still hold with the General Notes left out; `TestGeneralNotesEvidenceHashes` pins
the nine new hashes (the only golden values that changed, because their lookups gained
evidence) and the unchanged hashes of the other agreements.

## Build

`eval/jp/build_psr.py` (`GENERAL_NOTES`, `general_notes`, `quote_general`) appends the notes
after each schedule's existing notes. Only the nine agreements' `notes` were regenerated and
appended to the `schedules.json` of current main: every other schedule, and every row and
old note, is JSON-equal to main. `python3 -m unittest test_build_psr` runs the helpers;
with `JP_PDF_DIR` set to the directory holding the PDFs it checks every note against its
PDF (digest, text start and end, page, citation, source).
