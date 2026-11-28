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

Use the local frozen production precedent corpus, not live CROSS calls. Every source-answer ruling appearing anywhere in these development or test cases is excluded from retrieval. Each task additionally allows only rulings strictly before the earliest source endpoint date. Record the actual corpus inputs, candidate SHA and dataset hash; never expose credential paths or gateway host.

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
