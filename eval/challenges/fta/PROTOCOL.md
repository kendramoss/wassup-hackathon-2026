# FTA calculators: source-grounded challenge study (ATIGA, AKFTA, ACFTA, RCEP)

Issue #982. Pre-registered in D966, result in D967 (`docs/DECISIONS.md`).

## What this measures

Whether the deterministic origin checks (`atiga`, `aseanfta`, `rcep`, no model calls, $0) return the answer the cited rule rows and treaty clauses give, on cases built to cross thresholds, hit exceptions and leave facts out.

It does not measure real-world accuracy. The cases are synthetic, written by the implementer who also knows the engine interface. They are reserved from tuning, not blind. They are not customs rulings, and the near-duplicate series (boundary neighbours, variations of a published worked example) are not independent observations, so no population confidence interval is claimed.

## Files

| File | Content |
|---|---|
| `<feature>-dev.json` | development cases, used to find defects |
| `<feature>-test.json` | reserved test cases, scored once per candidate |
| `results/<feature>-dev-run1.json` | first development run, before the engine fix |
| `results/<feature>-dev.json` | development run on the frozen candidate |
| `results/<feature>-test.json` | per-case reserved test results (every mismatch and error retained) |
| `acfta-posthoc.json`, `results/acfta-posthoc.json`, `posthoc.py` | post-hoc supplement written after the first reserved run, scored separately, never in the reserved totals |
| `summary.json` | the shared evidence contract, one entry per feature |
| `generate.py`, `oracle.py`, `rules.py`, `sources.py` | dataset generator and label oracle (standard library Python) |
| `backend/cmd/ftachallenge` | scorer, validator and reproducer |

Each case keeps: id, split, `source_family_id`, `scenario_family`, `parent_id` (for variations of an existing worked example), variation tags, primary-source references with locators, the rule row, a rationale, the engine input, the expected checks, and `expected_abstention`.

## How labels were made

`oracle.py` computes each label from the cited rule row (read from the pinned Annex 3 / Appendix 2 / Attachment B / Annex 3A tables) and from treaty clauses written out in the oracle: regional value content by build-down (and build-up for the direct method), tariff shift by prefix with "except from" ranges, de minimis at 10 %, wholly obtained, produced from originating materials, cumulation between Parties, minimal operations, provisos on materials, process conditions. It never calls the Go engines. Hand-specified rows (RCEP product-dependent rows, heading 38.08) are written in `generate.py` from the row text and marked in the case `rule` field.

Conventions the oracle follows where a text is silent. They are stated so a disagreement can be traced to them:

- A material of unknown status, or declared originating but made outside the agreement, counts as non-originating.
- Facts as given: with no method stated and no costs, a failed build-down is a "no", not "undetermined". A stated `method` of `direct` is binding on ATIGA and AKFTA.
- A good identified only by its four-digit heading is decided only if every subheading's rule gives the same answer; otherwise undetermined. The engines answer "undetermined" whenever the heading has subheading rows, so cases where all subheadings agree measure over-abstention. This is a scope limitation, not a wrong answer, and was not changed.
- ACFTA Art. 7 minimal operations speak only to wholly obtained goods, so on a non-WO claim the product rule still decides. ATIGA Art. 31, AKFTA Rule 8 and RCEP Art. 3.6 stop the claim.
- ATIGA de minimis is by value only; AKFTA by weight only for chapters 50 to 63; ACFTA and RCEP accept weight or value for chapters 50 to 63.
- RCEP Chapter 3 article numbers (3.2 to 3.7) were not verified against the treaty text. The local RCEP PDF, read by rendering and OCR, is the Annex 3A rule tables, not Chapter 3. Three source operations were spent (a chapter URL that returned 404, the RCEP Secretariat rules-of-origin page, which links no chapter text, and an official-mirror URL that timed out). The numbers follow the engine's citations and the ABF guide; no clause text is claimed read.

Two kinds of case have something other than yes or no as the correct answer: malformed input must be refused (`refused`), and missing facts must give `undetermined`. A correct abstention counts as a match; an abstention where a decision was expected is a mismatch and is counted in `abstention.abstained_when_a_decision_was_expected`.

## Development and test separation

- Rows: development uses goods in odd-numbered chapters, test uses even-numbered chapters, so no HS row is shared. Normalized inputs (names dropped, HS digits only, text lower-cased) are disjoint; the scorer refuses overlap.
- Worked-example parents are disjoint: development varies one set of published examples, test another.
- Structure: test cases add originating "distractor" components and more material lines than development cases.
- Test-only scenario families: ATIGA partial cumulation, textile de minimis; AKFTA de minimis with incomplete facts, textile de minimis; ACFTA general rule for unlisted goods, textile de minimis; RCEP chemical reaction, product variants, heading 38.08, textile de minimis. All other families appear in both splits and share their templates. Development therefore cannot be called blind, and the shared-family case count is in `summary.json`.

## Scoring

A case is `matched` only if every stated check agrees: outcome, applied rule, alternative-rule outcomes, accumulation flag, de minimis flag and numeric RVC ranges where the case states them. A wrong check is `mismatched`. An engine error that is not an expected refusal is an `error`. All three stay in the denominator. Per-case results keep the expected checks, the actual answer and every failed check. No pooled headline across features; counts are per feature.

Extra metrics: expected-outcome counts, a confusion table, abstention counts, "yes" precision and recall with their own denominators, decided-only agreement, scenario-family totals, source families, parent groups, formatting-only cases.

## Commands

From `backend/`:

```sh
python3 ../eval/challenges/fta/generate.py
go run ./cmd/ftachallenge -mode run -split dev  -candidate <sha>
go run ./cmd/ftachallenge -mode run -split test -candidate <sha>
go run ./cmd/ftachallenge -mode summary -candidate <sha>
go run ./cmd/ftachallenge -mode verify
```

`verify` scores both splits again and requires the committed results to match byte for byte, except that `actual.rvc_down` and `actual.rvc_up` may differ by up to 1e-9 (amd64 and arm64 differ in the last bits: 57.93000000000001 against 57.92999999999999 for `atiga` result 150; the frozen file keeps the arm64 value). Outcomes, statuses, errors, counts, ids, hashes and every other number must be identical; the expected RVC ranges are 0.1 wide, so a real change is far above the tolerance. The summary must match apart from the measured latency. The runner refuses a candidate whose engine sources differ from the stated commit.

## Source operations

Three external operations, all unsuccessful (404, a page with no chapter link, a timeout). All other sources are archived primary documents already held locally (ATIGA text, Annex 5 and 6, ATIGA PSR workbook, the AKFTA Annex 3 and ACFTA Annex 1 copies pinned in D949, the Singapore handbook) and the parsed rule tables embedded in the engines.

## Limitations

- Not independent of the engines on rule data: `rules.py` reads the same embedded rule-row JSON the engines use, and its ACFTA chapter and heading lists (Art. 4(1)(b)) and ATIGA/AKFTA general rule mirror `aseanfta/profiles.go`. A wrong table transcription or legal-list choice would be copied into the labels, so this study does not independently validate either. The ACFTA general-rule family tests the engine against that mirrored list only.
- The six ACFTA errors in `authority-variants` are authoring defects: their input carries `method: direct`, which ACFTA refuses, so the correct expected class is `refused`, not the `no` they were labelled. The frozen labels and the first run (147/154) are unchanged; the corrected copies are scored separately in the post-hoc supplement.
- The oracle has no input-validity check for ACFTA, the blind spot that let those cases through.
- Synthetic, same-author, not blind. Not independent customs decisions.
- Rule tables are read from the engines' embedded parse of the official tables. Rows were spot-checked in D927 and D949, not re-keyed here.
- AKFTA Appendix 2 provisos and ACFTA 3.0 are scored only as far as the cases state the needed facts.
- Direct consignment, packaging, accessories, neutral elements and the proof-of-origin procedures are not scored.
- Latency is the measured time of the in-process check, not a service latency.
