# Japan engine: prospective model-quality evaluation plan

Status: a plan only. Nothing in this document has been run. The campaign that
produced the changes below ran no product model inference and makes no accuracy
claim. Every number here is a rule-availability or request-size count taken
offline, not a quality result.

## 1. What changed since the archived D712 dev_eval run

The prompt text and `PromptVersion` `jp-engine-v1` are unchanged. What changed
is the evidence placed in the request, the guards after the answer, and the
validation of the answer. Merged PRs #791 to #803:

| PR | Change | Effect on what a model sees or returns | Doc |
|---|---|---|---|
| #791 | HS edition correlation (mapped, edition hint) | New evidence for codes of another edition | hs-edition-correlation.md |
| #792 | Missing-facts review text | Review message only; not model input | missing-facts-review.md |
| #793 | PSR split-row repair | Evidence changed for 51 dev rulings | psr-split-row-repair.md |
| #794 | AJCEP general validity | Codes not in HS 2017 give `not_in_schedule_edition` | ajcep-general-validity.md |
| #795 | Thailand schedule notes (0 to 20) | Evidence text changes | thailand-scope-notes.md |
| #796, #798 | JP-US TA pilot and selector | 148 of 149 dev TA cases gain rule text | jp-us-ta-pilot.md |
| #797 | Answer validation | Origin closed list; invalid answer is an error | answer-validation.md |
| #800, #801 | GSP pilot and selector | 130 of 131 dev GSP cases gain rule text; GSP never returns `qualifies` | gsp-pilot.md |
| #802 | Withheld rows narrowed | JP-EU 7007.29, Switzerland 3504.00 gain text | withheld-rows-narrowing.md |
| #799, #803 | Decision log only | None | none |

The split-row repair changed returned evidence for 51
of 2973 dev rulings (Thailand 26, India 19, Mexico 2, Philippines 2, Chile 1,
Vietnam 1); for 50 the reason stays `psr` with different text, and one went from
`psr` to `no_match`.

## 2. Candidate designs

### A. Paired rerun on dev_eval
Rerun the 400 dev_eval ids (395 eligible requests) and compare with the archived
D712 outputs.
- Compute which requests changed offline, with no model: run
  `go run ./cmd/jpeval plan -split dev_eval` at the commit of the archived D712
  run and at the frozen commit. It prints only aggregates (`requests_sha256`,
  byte totals), so equal hashes mean all requests are identical and unequal
  hashes mean at least one changed. Per-id identification needs a per-request
  hash listing, which `plan` does not print (a small offline addition is
  needed, unverified). Ids with byte-identical requests are the control
  stratum; ids with changed requests are the treatment stratum. The model and
  `-split dev_eval` plan read no environment and make no network call.
- Recorded measurement at `20b39a82`: 395 eligible requests, 5 abstained before
  any request, `request_bytes_total` 5118030, `request_bytes_max` 35072, output
  cap 8192 tokens per request (3235840 tokens total cap), `requests_sha256`
  `3b654bfe243a7e59fa88df0206a949330039fe12e646f994e2978b1aa5a01ec2`. Later
  merges (#802) can change this hash; re-measure at the commit that is frozen.
- Pro: cheapest, paired, isolates the evidence change. Con: dev_eval is a
  development set already looked at, so it shows regression and change, not
  generalisation. Identical-request ids only measure model noise, not engine
  change.

### B. Fresh prospective sample
Use newly published Japan Customs advance rulings dated after the corpus snapshot.
- Select by publication date only (a date cut-off written down first), taking all
  rulings in the window or a pre-set random seed over them. No label reading
  before the run.
- Freeze the id list, the date window, the exclusion rules, the metrics and the
  commit under test before any run; hash the id list and the protocol and record
  the hashes. Any later change makes it a new study.
- The D741 held-out and synthetic data are burned and frozen. They must not be
  read, reused as unseen, or used to size this sample.
- Pro: the only design that can speak to generalisation. Con: sample size is
  unknown until the window is counted; labels come from the source rulings and
  need their own provenance check; costs are unknown until the sample exists.

### C. Coverage-only offline diagnostics (already done)
`jpeval diagnose` counts whether a rule source can be shown, with no model.
- dev: rule reference 2589 to 2950 of 2973 (2952 after #802).
- dev_eval: 366 to 395 of 400. dev_eval is a subset of dev; the counts are not
  additive.
- GSP counts rule reference only, and a GSP `qualifies` is guarded.
- This is availability, not accuracy, and it needs no budget.

## 3. Cost model (formula only; no price, no budget)

Let `N` be requests, `B_i` request bytes of request i, `rate_in` and `rate_out`
USD per million tokens, `cap` = 8192 output tokens per request.

    input_tokens_upper  = sum(B_i)            (1 byte counted as at most 1 token;
                                              an overestimate for Japanese text)
    output_tokens_upper = N * cap
    cost_upper = (input_tokens_upper * rate_in + output_tokens_upper * rate_out) / 1e6

For design A at `20b39a82`: input_tokens_upper = 5118030, output_tokens_upper =
3235840 (N = 395). The measured token counts will be lower; this is a ceiling,
not an estimate. Two passes (rerun plus any repeat) multiply by the pass count.

`jpeval plan` computes this from flags: `-rate-in <USD per million input>`,
`-rate-out <USD per million output>` and `-rates-source <where the rates come
from>`; it prints `worst_case_usd` and the rates. Example shape (placeholders,
not prices): `go run ./cmd/jpeval plan -split dev_eval -rate-in R_IN -rate-out
R_OUT -rates-source "SOURCE"`. The user must supply the rates and the budget and
must approve a run. Design B needs the same command run on the frozen sample.
Retries, a second model or a judge pass are not in this formula.

Running anything that calls a model also needs a separate prerequisite:
authenticated live smoke stays paused and must be resumed on its own, with its
own approval, before any live check.

## 4. Metrics and pre-registration

Register before the first call, in a hashed file:
- Primary metric: agreement of the answer class (`qualifies`, `does_not_qualify`)
  with the label, over answered cases; report the denominator.
- Keep apart: abstention before request (rule absent, 5 in design A), model
  `undetermined` (an abstention, not an error), invalid answer (for example a
  rejected origin) and transport or parse error. None is counted as correct or
  wrong without being reported.
- Strata, each with its own denominator: per agreement, per reason (`psr`,
  `general`, `mapped`, `edition_hint`, `withheld`), per request-changed vs
  request-identical. Strata with few cases are listed, not tested.
- Design A test: McNemar exact test on discordant pairs among changed-request
  ids (old vs new correct), with the count of each discordant type; the
  identical-request stratum reports run-to-run noise. Report the interval, not
  only a p-value.
- Regression: a pre-set rule, for example any agreement stratum where
  new-only-wrong exceeds old-only-wrong, or any rise in invalid answers or
  errors. The threshold is written before the run.
- GSP: the guard turns a model `qualifies` into `undetermined`, so a GSP
  `qualifies` metric is structurally zero. Report GSP separately, or on
  `does_not_qualify` only; do not pool it with other agreements.
- Not measurable by this plan: translation fidelity of the Japanese-only GSP and
  other source text, correctness of a specialist review, and legal sufficiency
  of a result.

## 5. Not run

None of designs A, B or C-with-model has been run. No model was called, no
result exists, and no accuracy claim is made. Running requires user-supplied
rates, a user-approved budget and the separate live-smoke prerequisite above.
