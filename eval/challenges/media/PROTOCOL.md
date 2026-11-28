# Source-grounded BOM image challenges (D972, #985)

## Scope and source

Evaluate the production `extract.ExtractMedia` entry point through `cmd/mediacheck`, not an independent OCR service. This measures transcription into the app's confirmed-parts draft, not country-of-origin legal judgement. No foundation-model fine-tuning.

`cases.json` contains 24 development images from three source families and 132 reserved images from eleven different source families. All fourteen are existing ruling-derived illustrative demo fixtures, cited by fixture path, source description and byte hash. Their original primary CBP rulings were not newly reread for this image study. Expected answers are the text rendered onto the new synthetic sheets; these are not official CBP BOMs or newly collected real documents. Source fixture descriptions can simplify or group components. A line item is scored as the printed description, including a grouped assembly, not as separate inferred nouns.

Source-family split, chosen before model calls: PCR analyzer, AGV and power-bank fixtures are development; the other eleven structured-product fixtures are reserved. No source family or exact component-list input crosses that split. Related images within a family are not independent observations.

## Variations

Twelve rendering families: ordinary table, country-first reordered columns, parts list, dense small print, lower contrast, skew/blur, reduced-resolution speckled grayscale scan, JPEG compression, missing origin with a supplier-address distractor, quantities/line codes with country abbreviations, bilingual French/English headers, and a separate assembly/testing footer. Development uses the first eight; the final four are reserved-only. Row order and counts vary where the source has enough parts. Bilingual variation is headers only; component names remain English. Images are deterministic with the recorded Pillow version and bundled font. No handwriting, real camera photos or physical printing/scanning.

Opaque image IDs are hashes of source-family and rendering identity, not expected answers. Images contain no ruling numbers, gold labels, test split or hidden answer metadata. The model sees the pixels plus the production file type/dimensions, not the manifest, filename, source IDs, expected country, legal holding or scorer rationale. The renderer/expected list is independent of the extractor and makes no model calls.

## Scoring, fixed before any call

Case/whitespace-normalized component names match exactly; name substrings or a model judge are not accepted. A fixed country-alias map accepts country codes and common aliases. This strict row matching can reject a semantically reasonable paraphrase or split of a grouped description; retain it and report the scoring limitation, do not relabel after observing a result.

For printed known-country rows, count names read and correct countries. Extra rows, duplicates and wrong countries are separate. The production API does not retain missing-country rows among confirmed components: it emits a missing-country warning. Thus an unknown row is correct only if its name appears in that production warning and it has no invented confirmed-country entry. Unknown withheld, unknown documented and expected unknown are separate counts; a call error is never a correct withholding.

A whole document passes only if all printed names are represented, all known origins agree, every unknown is warned about and withheld, and there are no extra confirmed rows. Errors remain in the document denominator and score zero fields. Each summary also retains field numerators/denominators, extra rows, wrong countries, cost and latency. No synthetic population confidence interval or pooled real/synthetic accuracy claim.

## Development, candidate and reserved run

Initial model is the configured production extraction model, `gemini-4-argon`, and prompt `extract-image-v1`. First run the 24 development sheets. Improve only if a development result demonstrates a defensible implementation defect; retain every attempted candidate and score, including regressions. No minimum score is invented to excuse hidden failures, and no specialist review gate is introduced. An unchanged candidate is allowed if development does not justify a change.

Before the first reserved call, commit `freeze.json` with selected candidate revision and the full dataset byte hash. The binary is built from that exact clean candidate; subsequent documentation/result-only commits do not change it. Each reserved document is scored once. Per-image first output is written immutably, with image and candidate hashes. Resume can only fill unattempted images with matching provenance; ambiguous pending outputs require inspection, not automatic retry. No post-test candidate tuning, input correction, label correction or polishing rescore is part of this first study.

## Execution and budget

Only the approved gateway process environment is accepted by `run.py`; fake and Vertex results are not eligible evidence. No credentials or their locations are committed or printed. Initial media allowance is $15 across development plus test, within the user's $1,000 cumulative programme cap. Estimate: 156 short image calls at roughly $0.03 each = $4.68; a conservative $0.08 per image = $12.48. Check shared spend before scoring; record actual gateway costs after each paid run. A $0.50 pending-call reservation limits concurrency and prevents starting work beyond the remaining run allowance. This is a conservative estimate, not a contractual bound against arbitrary model price changes; actual cost and any missing reports are disclosed. Maximum three concurrent images, one extraction call per image.

A local fake-model Go regression test proves the evaluator retains missing-country warnings; it is test wiring, not any study's measured score. Study results require `provider: litellm` and `simulated: false`. No production endpoint, customer document, production DB or deploy configuration is changed.

## Reproduction

- `python3 -I -B eval/challenges/media/test_study.py` (standard library)
- With the already available Pillow environment: `python3 -I -B eval/challenges/media/study.py generate --check`
- After the recorded calls: `python3 -I -B eval/challenges/media/study.py summarize --check`
- `cd backend && go test ./cmd/mediacheck`

Reproduction is offline and does not add trials. The bounded run-of-record images and their deterministic definitions are retained; do not duplicate raw image lanes.

## Development amendment, before candidate-v2 scoring

First baseline: 21/24 documents, 140/151 names and stated origins matched, 10 extra confirmed rows, no wrong matched-country values or model errors; cost $0.195032. Three failures on power-bank documents: origin suffixes copied into every name, JSON field text fused into a name, and whitespace before closing parentheses. All first outputs remain in `results/dev-baseline/`, and the original strict scorer is unchanged. Candidate v2 adds generic name/column/row separation instructions and uses prompt version extract-image-v2. Accept v2 only if it matches at least 21/24 documents and at least 140/151 stated origins, with no increase in model errors or wrong-country values. This criterion is recorded before its development score. No reserved result has been observed.
