# Legal-rules challenge study: protocol (D968, result D969)

Issue #983, part of the small-benchmark campaign (#981). One lane, three
deterministic engines, no model call, no network call, cost $0.

Evidence kind: `source_grounded_synthetic`. The cases are constructed by the
implementer from legal text. They are not independent customs decisions, not
real tariff revisions, and not an i.i.d. sample. No real-world accuracy or
confidence interval is claimed.

## What is scored

| Feature id | Engine entry point | One case is |
|---|---|---|
| `usmca_rules` | `usmca.RuleSet.Check` (pilot rows and the extended GN 11(o) rows) | a request body and the expected rule status, alternative outcomes, exact RVC and what information is still missing, or a deliberate refusal |
| `textile_rules` | `textilerules.Rules.Evaluate` | structured facts and the expected status, origin country and paragraph of 19 CFR 102.21 |
| `tariff_watch_rates` | `tariffwatch.Evaluator.Watch`, `tariffwatch.Revise`, `Dataset` validation | a saved case's duty basis against a synthetic next data set (or a rendered Chapter 99 text) and the expected changes, rate deltas and dollar figures |

## Sources

No external source operation was used (0 of the 2 allowed). Every source is a
local archive already in the repository, registered with its SHA-256 in
`sources.json` and checked by the runner:

- HTSUS Revision 23 General Note 11(o) text (`eval/usmca/gn11o.txt`) and the
  General Note 11(c), (e), (n) and (p) pages (`sources/gn11-general-excerpts.txt`,
  extracted from `usmca-gn11/gn_pages.json`, GN pp. 28-35, 48-49, 163).
- 19 CFR 102.21 (eCFR point in time, 18 Nov 2026) and 87 FR 68356.
- The embedded additional-duty data, the Chapter 99 excerpts of HTS revisions
  21, 22 and 23, and the Annex V.A text of 90 FR 48320.
- Three "candidate documentation" sources (package docs) back the cases that
  test a documented abstention or refusal rather than a legal rule. They are
  labelled `candidate_documentation` and are not law.

Every case carries its source references with a locator and a rationale.

## Tariff data: real baseline, synthetic revisions

Tariff lines, heading numbers, list names and baseline rates are real (from the
embedded additional-duty data and the Chapter 99 excerpts). Every "next
revision" data set, every changed rate or date, and every rendered revision
text is SYNTHETIC: it was constructed to exercise one kind of change. None is
a government tariff action or announcement, and none states a new government
rate.

## Oracle: how the expected labels were written

The labels come from `oracle_usmca.py`, `oracle_textile.py` and
`cases_tariff.py` / `cases_revise.py`. They share no code with the engines,
never call a candidate, and are written from the legal text and the
documented behaviour. The USMCA and textile oracles contain a hand
transcription of the rule rows the cases use (from `gn11o.txt` and the eCFR
XML). That is a second transcription of the same authority text: it does not
independently validate the rule data embedded in the candidate, and a shared
misreading of the text would not be caught. Tariff expectations are arithmetic
on each case's own figures. Revision-text cases render the text from a
structured spec, so the expected data set is the spec, not a parse.

`needs` labels (what information would resolve an open question) are the
designed gap of each scenario, mapped to neutral kinds in the runner.

## Dataset

`python3 eval/challenges/legal-rules/build_cases.py` writes
`<feature>-dev.jsonl`, `<feature>-test.jsonl` and `sources.json`
deterministically. Each case keeps: id, split, source family, scenario family,
structural template, optional parent, variation tags, source references,
rationale, engine input, expected label, and `expect_invalid`.

Counts (development / reserved test): USMCA 184 / 210, textile 63 / 146,
tariff watch 107 / 108. Test source families: 12, 15 and 9; structural
templates: 9, 9 and 5.

Variation is by clause and process, not by number padding: correct, incorrect
and undetermined outcomes; threshold boundaries (at, just below, repeating
fractions, display rounding, negative RVC, zero denominator); exceptions and
excluded headings; process and ingredient differences; missing facts and
unknown claims; incomplete material lists; short and long codes; de minimis,
parts route, deemed-originating goods; dated action windows and inclusive
boundaries; rate cut, rise, tiny change, no change; lines added and removed;
exclusion windows; MFN-only changes; Section 232 flags; user-entered rates;
plan effect; and deliberately invalid inputs. The same structural scenario
set is applied to several rule rows, so the number of independent scenarios is
smaller than the number of cases. `Families` in `summary.json` counts source families,
scenario families, templates, templates shared with development and cases with
a parent. Near-duplicate robustness variants (longer codes) record a `parent`.

Guards (`go run ./cmd/legalchallenge validate`, also unit-tested):

- ids unique, split matches file, required fields present, every source id
  registered and every source file hash-verified;
- inputs are normalised (material ids and order, trailing zeros of amounts
  removed) and any duplicate inside a split, or across dev and test, is refused;
- parents must exist in the same split;
- at least 100 reserved cases per feature;
- input validity before freeze: an input not declared invalid must be accepted
  by the candidate's input validation, and an input declared invalid must be
  refused, so accidental authoring errors cannot masquerade as expected
  refusals. This reads whether an input is accepted, never a rule outcome.

## Development, and the candidate

The candidate is this repository's engines at the freeze commit. Development
cases (separate source rows and, where feasible, separate headings from the
test) were run first against the base commit `54a4922e`'s engines.

Baseline development run on the final development labels
(`results-dev-baseline.jsonl`): 348 of 354 matched; 6 did not (2 USMCA
mismatches, 3 textile mismatches, 1 tariff-watch error).

Defects found and fixed, each with a regression test that failed first:

1. USMCA de minimis returned `met` on a listed total inside 10 percent although
   the materials list was unconfirmed (unlisted materials could push the total
   over). Now `needs_information`, asking to confirm the list.
2. USMCA parts route returned `not_met` because no listed part shared the
   good's classification although the list was unconfirmed. Now
   `needs_information`.
3. Textile headings 6309 to 6310: the packaging requirement never matched the
   facts' `packing` stage, so the rule always answered undetermined.
4. Textile 6309 to 6310 with a complete single-country manufacture packed in
   another country: the single-country shortcut hid the packing country the
   rule names.
5. Tariff watch: a dated action with a rate above 100 percent passed
   `Validate` (limit 500 percent) but could not be evaluated, because the
   duty package bounds a list rate at 100 percent. The evaluation now carries
   the action's own rate. (The root bound is in `internal/duty`, outside this
   lane's allowed packages.)

Two label corrections were made on development data before the baseline run
above: the oracle omitted the (b)(5) entries for 4202 and 9619 (a transcription
error found by dev cases 061 and 062), and the oracle first accepted a de
minimis result on an unconfirmed list (corrected, which exposed defect 1).
The corrected oracle was used for the reserved test labels too. After the
fixes all 354 development cases match (`results-dev.jsonl`).

The reserved test labels were generated by the same oracle after these
corrections and before any candidate was run on the reserved cases. The
reserved cases share engine mechanisms with development (de minimis, parts
route, dated-action windows, exclusions, process rules); shared structural
templates and shared source families are counted in `summary.json` rather than
hidden. Before the freeze only the acceptance of the reserved inputs by the
candidate's input validation was checked, never a rule outcome. This is not a
blind study.

## Freeze (D968) and the single test run

The candidate, datasets, labels, sources, runner, this protocol and the D968
entry are committed together before the reserved cases are scored. The
reserved split is scored once:

`go run ./cmd/legalchallenge run -split test -freeze <full sha of the freeze commit>`

The runner refuses unless HEAD is that commit, the candidate and data trees are
clean, and no `results-test.jsonl` exists. Every first result is retained:
matches, mismatches and errors each have a row. No label, engine or case is
changed after the run to improve a result. Anything found afterwards is
reported as a post-hoc observation, separate from the measured figures.

## Metrics and denominators

`summary.json` follows the shared evidence contract. For the reserved test:
`cases = matched + mismatched + errors`; an error row is a case, not an
omission. Extra named fields keep their own denominators:

- USMCA: decisive means `met` or `not_met`, abstaining means
  `needs_information` or `not_supported`. Reported: expected and engine
  decisive, decisive correct, precision (correct over engine decisive), recall
  (correct over expected decisive), false decisive (engine decisive where an
  abstention was expected), unexpected abstain, expected abstain and abstain
  correct, deliberate refusals expected and correctly refused, errors.
  Expected abstention that the engine returns is a correct result.
- Textile: decided versus undetermined versus out of scope, with wrong origin,
  false decided, unexpected undetermined, reason class agreement among
  expected-undetermined cases, errors.
- Tariff watch: kind counts (watch, revise, dataset refusal), the confusion
  matrix of the `affected` flag, fully matched rate-change cases and dollar
  figures with their expected counts, errors.

A match requires every compared field to agree: for USMCA the status, the
listed alternative outcomes, exact RVC and status, the information the
scenario was designed around (as a subset of what the engine asks for); for
textile status, origin and paragraph; for tariff watch the whole canonical
result. Textile reason classes are reported separately and are not part of a
match.

## Reproduction and portability

Rows hold only strings and integers: rates in whole units of 0.0001 and
dollars in whole cents, rounded from the engine's floats through a fixed
canonicalisation, so a last-bit float difference between platforms cannot
change a stored result or a comparison. Case data avoids half-unit boundaries.
Measured latency (p50, p95 per case) is stored in `summary.json` and
`latency-*.json` and is excluded from the byte comparison. `go run
./cmd/legalchallenge check` and the committed-results test re-score both splits
and require byte-identical rows, then recompute `summary.json` from the raw
rows (ignoring only latency).

## Limits

- The study measures agreement with an implementer-written oracle on
  constructed cases, not decisions of customs authorities or tariff agencies.
- Free-text operation names (`FromProduct`) and the live tariff data path are
  not exercised; textile facts are given as structured stages.
- Pure tariff-shift textile rows and the (c)(4) most-important-process
  judgment are documented abstentions; expecting `undetermined` there tests the
  documented behaviour, not a legal answer.
- Chapters 50 to 63 and the rayon exception of GN 11(n) are outside the
  USMCA cases.
