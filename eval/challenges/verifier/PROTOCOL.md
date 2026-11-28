# Source-grounded move-verifier challenge

Pre-registration: D974; issue #986, campaign #981. The production engine and rule source start at `54a4922e`. No gateway scoring has run when this protocol is first committed.

## Purpose and scope

Measure the existing given-move verifier on meaningful source-derived variations, not full plan search, ranking, economics or independent new CBP decisions. Develop only on the development subset. Select and commit a final candidate before the reserved test is scored. Do not change test labels, drop failures or retune on test outcomes.

## Frozen cases

`cases.json` SHA-256: `a08297baa59ada7f7ef57b4173e213f535671b135fccfcc17585a72583f0b42e`.

The generator reads the three already-verified public ruling-pair files, pins their hashes, limits selection to core-tier pairs with single-country labels, excludes the repeatedly used heat-gun demonstration and ambiguous country alternatives, and groups connected source rulings before splitting. Duplicate source endpoints and normalized engine inputs are rejected before scoring.

| Split | Cases | Positive | Negative | Incomplete facts | Source parents | Connected source families |
|---|---:|---:|---:|---:|---:|---:|
| Development | 28 | 12 | 12 | 4 | 6 | 6 |
| Reserved test | 220 | 100 | 100 | 20 | 50 | 49 |

Source ruling ids and connected source families do not cross splits. Multiple test variations share each source parent; 220 cases are not 220 independent legal examples. Cases were authored with knowledge of the code and public source corpus, so this is reserved synthetic challenge evidence, not independently authored blind validation.

## Variations and expected-label rubric

Each positive source pair supplies observed manufacturing endpoints and origins. A forward variation changes production volume without changing the manufacturing, while a reversed pair tests the opposite manufacturing move. The known starting origin is supplied, as it is in the earlier real-pair evaluation.

Contrast controls preserve a source endpoint's manufacturing while changing only ordinary disposable retail packing/barcode labels or warehousing/protective transit packing in a third country. They explicitly exclude combining articles into a new set, component replacement, finishing or any other manufacturing. These are conditional source-derived conservation expectations, not a universal assertion about every packaging or testing operation. No testing-only rule is inferred.

Incomplete-fact controls omit the decisive manufacturing move and expect the verifier to withhold shown precedent support; they do not invent a true resulting origin. Ruling ids/URLs, parent, family, semantic variation tags and expected-label rationale stay with every case. Expected labels come from the source endpoints and the stated non-transformation conditions, never from the candidate's output.

The online source search identified repackaging ruling HQ953916, but its public page fetch exposed only site navigation, not the decision. It is **not** treated as a verified extra case. The existing primary ruling snapshots and original pair verification remain the factual basis. This used both coordinator external source operations; media must reuse existing archived primary sources rather than exceed the campaign search cap.

## Retrieval and model configuration

Use the local frozen production precedent corpus, not live CROSS calls. The pre-registered intention was to exclude every source-answer ruling appearing anywhere in these development or test cases. The actual frozen exclusion list contains the selected parent endpoints; a post-score audit found unselected related-source ids that remained available, as qualified in the audit section below. Each task additionally allows only rulings strictly before the earliest source endpoint date. Record the actual corpus inputs, candidate SHA and dataset hash; never expose credential paths or gateway host.

Gateway model `gemini-4-argon`, verifier prompt v2, production panel enabled for confidence `[0.89, 0.95)`, one majority vote, six tool calls per perspective, 6000 maximum output tokens, shown-plan bar 0.92. Existing retry policy is allowed for transport failures inside a case, never for selecting a preferred model answer. Three parallel case checks; raw calls use the shared pace/concurrency gate. An 8-minute case deadline produces an explicit error, not a correct refusal.

## Scoring

- Determinate case agreement requires both the expected origin-change decision and resulting country.
- An incomplete-fact case matches only when shown support is withheld.
- Shown positive precision requires a correctly identified change and target. Wrong shown targets count as wrong shown claims. Recall uses all 100 positive test cases, including errors. Controls and incomplete-fact cases have separate reported denominators.
- Model errors remain errors in the full case denominator. A failed negative case is never a correct true negative. Missing/duplicate result ids or mixed candidate/dataset configurations invalidate summary generation.
- Store every attempt's final row, cost and latency. Resume only not-yet-attempted cases with identical candidate, dataset, split and model configuration; preserve error rows. Dataset and results remain committed; do not overwrite an existing run.
- No population confidence interval: these related generated variants are not independent random real-world cases.

## Development and spend

Estimate initial development at less than $5; use actual gateway cost headers and token-price estimates only when headers are unavailable. The entire verifier lane, including development, errors and test, has a $30 sub-cap within the programme $500 cap. The coordinator may reallocate its reserved allowance only by recording that before additional calls. Record every paid run in the shared spend ledger. Cost and latency are reported, not feature-quality acceptance thresholds.

A raw-call guard reserves $0.50 before sending each request and reconciles actual cost afterwards. This allowance is conservative for the bounded prompts/output; an unusually higher provider price or unreported spend must be disclosed. Stop before another unaffordable raw call; do not pretend an uncalled case was evaluated. Check the measured development cost before allocating the reserved test run.

Development failures may justify a general prompt/rule fix with regression tests. Keep all attempts. If no safe improvement is demonstrated, retain the production candidate rather than force a change for the word training. Before reserved scoring, commit the selected candidate and record its SHA. D975 will retain the first reserved result, even if poor.

## Development candidate preregistration, after the baseline development run

Baseline v2 on the 28 development cases: 26 matched, 2 mismatched, no model errors; 3 of 12 positive moves shown, no false-positive controls or shown incomplete cases. Cost $1.2275622, all raw-call costs reported. The two wrong origins conflate an unfinished blank with a completed functional article; one occurs in the reverse direction. These are development observations only; the 220 reserved model outcomes have not been scored or inspected.

Test development candidate v4 once on the same development cases. It keeps precedent-first search, counter-search, the same evidence-based confidence scale and shown bar, but makes functional completion and directionality explicit rather than treating generic process names as categorical exclusions. It gets a separate prompt/cache identity; no model fine-tuning. Public defaults remain v2 during this experiment.

Select v4 for the reserved run only if it reaches at least 27 of 28 development expectations, has no shown negative controls or shown incomplete cases, no model errors, and shows at least the baseline 3 of 12 correct changes. Otherwise keep v2 and record the negative result. This is development selection, not a statistically established improvement. Freeze the selected candidate before the first reserved run. Never lower the 0.92 bar to manufacture coverage.

The user raised the cumulative programme cap to $1,000 during this work. The verifier's initial $30 lane allowance still applies to baseline, candidate development and reserved scoring unless the coordinator records a reallocation first.

## Candidate selection, before reserved scoring

Development v4 at commit `9d90ad74` matched 24/28 expectations, with 4 mismatches and no model errors; 3/12 positive moves were shown, no controls or incomplete cases were shown. Cost $1.0717433; every call had a reported cost. It **failed** the preregistered requirement of at least 27/28 and is rejected. Both development runs remain recorded, including the worsening result. The rejected prompt is retained as a text artifact and in its original commit, not left active in the production verifier library.

Select the unchanged production **v2** behavior, panel on and bar 0.92, for the first 220-case reserved score. No confidence threshold, label, source family or expected outcome is changed. Paid development total is $2.2993055; allocate up to $27.70 of the initial lane allowance to the reserved run. The dataset hash remains unchanged. `retrieval-manifest.json` pins the label corpus and a digest of the exact local facts/raw inputs and will be checked after the study. The candidate freeze commit immediately following this selection record identifies the selected runnable code; the raw test rows will record its full SHA.

## First reserved result (D975)

Selected candidate `59780443341e87dc77b07a90d43d83d79562d898`, unchanged production v2 behavior, was scored once on all 220 reserved cases. No labels, rows or cases were removed or retried after seeing their result.

- 199/220 declared expectations matched (90.5%); 20 mismatches and one model error.
- Shown positive precision 47/47; positive coverage/recall 47/100. The 53 unshown positive cases remain in recall's denominator.
- Negative controls 99/100 correctly handled, with the failed model response retained rather than called a correct refusal.
- Incomplete-fact controls: support withheld 20/20; none shown.
- Test latency p50 32.055 seconds, p95 79.290 seconds, including every row. Test cost $12.3137152; entire study including both development candidates $14.6130207. Every call had a reported gateway cost.

The one error is `test-BA-N364414-N365093-retail-packaging`, retained with `model_error`; no preferred-answer retry. One packing-control facts_b field is 7,093 characters, so the production 7,000-character limit omits its final 93 characters on tuning/testing/quality assurance. Its original A facts are complete and B explicitly preserves that manufacturing; its recorded answer is retained, with this input-limit qualification disclosed.

The retrieval manifest matched after the run, so the read-only precedent archive stayed unchanged. Raw `test-results.jsonl` and its cost sidecar retain the first outputs; `test-summary.json` is rebuilt offline by the Go scorer, while `summarize.py` exports the common campaign `summary.json` and checks dataset/candidate/split/row/cost consistency. The larger test reveals limited willingness to show a supported change despite high conditional precision; it does not establish the old small real-pair recall carries over. No production prompt change was adopted and no population confidence interval is claimed.

## Post-score source and tooling audit

The intended source-answer isolation was **not fully achieved**. The frozen exclusion list contains all selected endpoints, but not the complete closure of related source rulings. `N357930` and `N366550`, endpoints of unselected pairs in used families, were not excluded. `N357930` appears in five test inputs and was cited as a supporting ruling in three recorded results; none of those three was shown at the frozen bar. This does not establish that only those rows were affected. The first results and labels remain unchanged, with no rescore to assert a causal impact. Do not describe this as a source-clean or independently held-out study. A future round needs a separately frozen full-family exclusion closure and new protocol.

Of the 53 unshown positives, 33 had the correct target/change decision but insufficient shown support; 20 had a wrong change decision. Incomplete-fact errors must never count as successful withholding. The export helper was corrected and regression-tested for that latent case; this study's 20/20 is unchanged because none of its incomplete cases errored.

Known CLI limits not exercised in this run: a budget-blocked zero-call row is currently retained as a budget error and cannot automatically be resumed as an unattempted case; do not use such a run to claim all cases received model calls. The clean-tree gate covers the evaluator, verifier and model-client source directories, not every transitive source file. Rows record the principal model/prompt/panel/bar; the fixed votes/tool-cap/panel-band are in this protocol. The retrieval manifest was checked manually before and after the run. No budget-blocked row occurred here.
