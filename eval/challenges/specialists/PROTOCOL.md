# Specialist challenge study (D970, D971)

Source-grounded synthetic challenge study of the four model-backed specialists
and of legal-regime routing. Tracked in #981 (campaign) and #984 (this lane).
It adds 100 or more reserved cases per specialist beside, and does not replace,
the historical real-ruling studies, which are unchanged.

## What is measured

| Feature id | Candidate (production configuration) | Reserved cases |
|---|---|---|
| `textile_specialist` | `textile-v1` over the 371 textile rulings, K=5 | 114 |
| `pharma_specialist` | `pharma-v1` over the 187 pharmaceutical rulings, K=5 | 113 |
| `part102_specialist` | `usmca-v5` (Part 102 marking rules, 19 CFR 102.11; not the USMCA preference calculator) over the Part 102 pool, K=5 | 112 |
| `japan_engine` | `jp-engine-v1` (rule rows plus Japan rulings, K=5) | 112 |
| `legal_routing` | `router-v4`, scored on the specialist cases that fit it plus general-origin negative controls, as separate denominators | 339 appropriate + 110 controls |

All four engines use `gemini-4-argon`; the router uses `gemini-3.8-flash`. Model
calls go through the approved gateway only; the runner has no other provider and
a test forbids Vertex code in it.

## Cases

Each case has an opaque id, split, source-family id, scenario family, template,
variation tags, primary-source references with locators, the rulings it is
modelled on (`exclude_rulings`, removed from retrieval for that case), a
rationale for the label, the model-visible input (`facts`, `date`, and for Japan
the agreement and tariff number) and the label. Nothing but the input reaches
the model, and a guard fails a dataset whose input contains the id, template,
rationale, source family, an excluded ruling's number or a label word.

Labels are written by the deterministic generators in this directory
(`textile.py`, `pharma.py`, `part102.py`, `japan.py`, `routing.py`, built by
`build.py`) before any model runs, from a reference oracle that applies the cited
law to a structured scenario. No model output is read by a generator.

* **Textile**: 19 CFR 102.21(c)(1)-(5) and Table 1 to paragraph (e)(1) (eCFR text
  and the 87 FR 68356 revision of row 6201-6208, both in
  `backend/internal/textilerules/source`). Families: spinning, extrusion,
  woven-fabric dyeing, printing and finishing, woven apparel assembly, knit
  apparel assembly and knit-to-shape, and made-ups.
* **Pharmaceutical**: the rules in the engine's own prompt, as CBP applies them in
  the rulings cited per case (HQ 563207, N334870, N343663, N336575, N337948,
  N353195, N355022, N349588, H341715, H340257) and 19 CFR 134.1(b). This
  measures consistent application of those rules to new facts, not validation of
  the rules.
* **Part 102**: 19 CFR 102.11(a)(2)-(3) and (b), 102.13 (7 percent de minimis),
  102.18(b)(1), and the 102.20 row text in `backend/internal/tariffshift/rules.json`
  (eCFR as of 2026-11-18), parsed by the oracle's own code. Only rows reading "A
  change to ... from any other chapter, heading or subheading" are used.
* **Japan**: the product-specific rows in `backend/internal/jprules/data/schedules.json`
  (each agreement's published annex; page and source hash recorded there) whose
  whole text is CC, CTH or CTSH, for eight agreements, in machinery, instruments
  and metal-article chapters with no chapter note. Failing materials always carry
  at least 25 percent of the ex-works price, so no tolerance or de minimis
  provision can apply. The outcome label is `qualifies`, `does_not_qualify` or
  `undetermined`.
* **Routing**: the textile, pharmaceutical and Part 102 specialist cases, whose
  expected regime is their own; and real 2026 general-origin rulings (recorded
  regime `substantial_transformation`; dev January-April, test May-September) as
  negative controls. Japan cases are not routed.

Positive, negative and uncertain cases are all present. An **uncertain** case is
one where a fact the law turns on is missing (an unstated country, an unknown
classification, an unidentified lot, a split assembly with no ranking of the
processes). Its correct result is the mechanism the product has: for the origin
specialists, routing to review (stated confidence below 0.92, the study bar;
production sends every specialist case to review, so 0.92 is a convention taken
from the general engine's auto-accept bar); for Japan, outcome `undetermined`.
No uncertain case is labelled with an invented holding.

## Grading

* Origin specialists, decided case: origin equal after `NormalizeCountry` is a
  match. The row also records whether it was confident (would auto-accept).
  Counted separately: agreement, confident decisions, confident-correct and
  confident-wrong.
* Origin specialists, uncertain case: a match only if the prediction routes to
  review. Answering confidently is a mismatch (`overconfident`).
* Japan: outcome equality. The criterion the model names is checked against the
  set of legally valid criteria on the stated facts (the rule's own criterion, any
  stricter one the facts satisfy, and PE when every material is originating); it is
  reported but never turns a correct outcome into an error. Declining a decided
  case is a mismatch (`abstained_on_decided`).
* Errors (timeouts, gateway errors, a refused retrieval of an excluded ruling, a
  spend-cap refusal) are rows and stay in every denominator; no case is retried.
* `matched`, `mismatched` and `errors` always add up to `cases`. Decided and
  uncertain cases keep their own denominators. Preflight cases (a Japan code not in
  the schedule's edition, answered before any model call) are counted apart from
  model calls.
* Routing: the label must equal the expected regime. Appropriate specialist cases
  and negative controls are reported separately, with the number of negative
  controls sent to a specialist regime.

## Development, selection and freeze

1. Datasets and labels (dev and reserved) are generated, hashed and committed
   with this protocol and decision D970 before any paid call.
2. The production configuration is scored on development first. Development
   runs are never overwritten and every one is appended to `attempts.jsonl`.
3. A change to a prompt, rule or engine is adopted only if (a) it is motivated by
   named development mismatches, each with a regression test that fails first,
   and (b) on development it raises matched cases without raising confident-wrong,
   errors or abstentions on decided cases. Ties keep production. Test labels and
   test results are not looked at. Where development shows no defect the engine is
   left unchanged and that is the result.
4. The final candidate is chosen on development only and committed. D970 is then
   amended with the candidate commit and the dataset hashes; the first reserved
   score is made at that commit with `-freeze <sha>`; the runner refuses a dirty
   tree, a different HEAD or an existing reserved result.
5. Each scope (the four specialists; routing) is scored once. Results, errors and
   abstentions are kept as they came out; no label is changed and no case removed
   after scoring. A test-set defect found later is reported, not repaired.

Spend: lane cap USD 35 across all four specialists, routing, development and test;
the runner stops sending at its `-cap` and records every refusal as an error row.
Reported gateway cost and latency (p50, p95) are recorded per run.

## Reading the numbers

Agreement with the study's oracle on constructed cases that share templates with
development. Not independent real-world accuracy, not a blind study, no confidence
interval and no claim of 100 independent legal sources. The cases are parameter
variations of a small set of rules and rulings; source-family and template counts,
and how many are shared with development, are in `summary.json`.

## Reproduce without a model

```
python3 -I -B eval/challenges/specialists/build.py --check
cd backend && go run ./cmd/specialistchallenge validate
cd backend && go run ./cmd/specialistchallenge check
```
