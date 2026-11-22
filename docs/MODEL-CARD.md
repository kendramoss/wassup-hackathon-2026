# Model card: Unde origin engine and planner

This card follows the sections of Mitchell et al., *Model Cards for Model Reporting* (2019). Unde does not train a model. It is a system built on Gemini models used through Vertex AI, with retrieval over public CBP rulings. The first held-out split and production run on Vertex AI; some newer evaluation runs (the second held-out split, the post-cutoff planner test, the model sweep and the image import) went through an OpenAI-compatible LiteLLM gateway to the same Gemini models. Gateway and Vertex AI agree on the development set (same engine and model, exact McNemar p = 0.58, D151); the gateway runs were not repeated on Vertex AI. The card describes that system as a user meets it.

Headline numbers are kept in one place, [NUMBERS.md](NUMBERS.md), and explained in [EVALUATION.md](EVALUATION.md). This card links to them instead of copying them, so it cannot drift. The failure analysis in section 6 is computed for this card from the official held-out run by `eval/charts/build_trust_data.py`.

Status words: **built** means in the code and running; **measured** means recomputed from committed rows; **reported** means stated in a committed document but not recomputable here; **planned** means not done.

## 1. Model details

| | |
|---|---|
| What it is | A system that predicts the country of origin U.S. Customs and Border Protection (CBP) would likely assign to a product made in several countries, cites the earlier CBP rulings it relied on, and searches production moves that change that origin |
| Who built it | Unde Labs, a team of two, for the Google Cloud AI Builder Cup 2026 (Manufacturing theme) |
| Version | origin engine prompt `engine-v1`, router `router-v2` (router `router-v4` in production since 21 Nov, D233; the held-out engine figures were measured with router-v2), precedent check `verifier-v2`, frozen 2026-10-14 for the held-out run ([splits_final.json](../eval/splits/splits_final.json)) |
| Models (built) | Origin engine: Gemini 4 Argon. Router: Gemini 3.8 Flash. Precedent check: an ADK agent on Gemini 4 Argon that searches and reads CBP rulings. Ask-the-case agent: the engine model unless `CASE_AGENT_MODEL` is set. Fallback engine on a 429, 503, 504 or deadline: Gemini 3.8 Flash, whose answers never auto-accept. Defaults in `backend/internal/config/config.go` |
| Rule check (built, no model) | A deterministic 19 CFR 102.20 tariff-shift check for chapters 15, 21, 39, 40, 44, 48, 71, 73, 76, 82, 83, 84, 85, 87, 90, 94 and 95 (`backend/internal/tariffshift`, D360; ten chapters added in D460). It runs only when the product gives HTS codes, answers "undetermined" rather than guess, and is display only: it never changes the engine's answer, the triage or auto-accept (D363) |
| Rule check on plan moves (built, no model) | The same 102.20 check on each plan move: the product as it would be after the move, finished where its final assembly would be (`backend/internal/planrule`, D441). Shown on every move of a Part 102 case, and on a substantial-transformation case only when every part has a code, for comparison. It flags a move an earlier ruling supports but the rule does not, and never changes a verdict or which plans are shown. On a Part 102 case it orders shown plans within each tier: moves the rule says shift first, moves it gives to another country last (D500). The broker draft and the country comparison show it (D502). Three scored Part 102 plan searches (development, D502): on the actuator the rule picks the motor moves CBP's reasoning implies; on two reverse probes it flags moves precedent leaned to |
| HTS codes for parts (built, model) | Up to 3 codes per part from sentences of earlier CBP rulings that classify a similar part, or the router model's own reading, each labelled with its source (`parts-hts-suggest-v1`, D380). Unverified suggestions the user accepts per part; they let the rule check run. On by default in production, rate-limited and spend-capped like the other model routes |
| Retrieval | BM25 over the facts sections of 2,790 labelled CBP rulings, the 5 most similar rulings dated strictly before the case (D10) |
| Decoding | temperature 0 and a fixed seed (20261017) on every call (D76) |
| Training | None. No fine-tuned model is in use. A fine-tuning experiment on Gemini 3.5 Flash was tried and not adopted ([EVALUATION.md §3.3](EVALUATION.md#33-model-size-against-cost-and-latency)) |
| Licence of the data | CBP rulings are U.S. government works, public domain. See [DATASHEET.md](DATASHEET.md) |
| Questions | open an issue on the repository |

## 2. Intended use

**Primary use.** A trade-compliance or industrialisation lead at a contract manufacturer, usually in ASEAN, checks a planned production move before it is made. They describe the product (bill of materials, where each part is made, which plant does each step) and get:

- the origin CBP would likely assign, a confidence and the rulings it read;
- production moves that reach a target origin, each shown only if an earlier CBP ruling backs it;
- a draft binding-ruling request for a licensed customs broker.

**Primary users.** The lead above, their COO or supply-chain head, the U.S. brand that sources from them, and the customs broker or trade lawyer who reviews the result. The roles are described in the [README](../README.md#who-uses-it-and-who-benefits).

**How the answer is meant to be used.** As a first check that decides whether a move is worth taking to a broker. At calibrated confidence 0.90 or more (the same cases as a stated 0.92), with the router placing the case in scope, the answer is auto-accepted (the fast path); everything else is marked for review. Result screens, exports and API responses say the result is not a ruling.

## 3. Out-of-scope uses

- **As a ruling, or in place of one.** Only a CBP binding ruling (19 CFR Part 177) binds CBP, and only for the facts in that request. Unde predicts from earlier rulings.
- **Filing or declaring origin** on an entry without a broker's review.
- **Other countries' origin rules**: EU Binding Origin Information, USMCA regional value content, preferential-origin claims, certificates of origin. Nothing detects or blocks such a question ([scope-and-jurisdiction.md](scope-and-jurisdiction.md)).
- **Tariff classification.** The HTS subheading is an input. Unde suggests candidates but does not decide it.
- **Duty drawback, antidumping or countervailing duty questions, Section 232 duties.**
- **Planning for product families without a template.** Plans exist for the families listed in [DETAILS.md](DETAILS.md#limitations); other products get an origin prediction but no plan.
- **Any decision about a person.** The system has no personal inputs and is not for that.

## 4. Factors

The factors that change accuracy, from the evaluation and the failure analysis below:

- **The origin CBP held.** Accuracy differs by country. It is lowest where CBP held China (section 6), which is also the case that matters most for a move out of China.
- **Confidence.** The model's stated confidence runs high (94.7% stated against 90.2% right on the 377 held-out rulings). For the general engine on Gemini 4 Argon the product reports a calibrated confidence instead, from a map fitted on the development set (D160): on the same 377 it averages 90.2%. Auto-accept needs calibrated 0.90, the same cases as a stated 0.92. Other engines report stated confidence only (section 5).
- **Type of good.** The engine is built and tuned for assembled manufactured goods. Food, natural products and single-material processing are where it is weakest (section 6).
- **Ruling collection.** Most rulings are New York (NY) rulings; headquarters (HQ) rulings, which are usually harder, are only 13 of the 377 held-out cases, too few to report separately with a useful interval.
- **Origin regime.** Textiles, Part 102 marking origin and pharmaceutical active ingredients have their own engines and their own, smaller held-out tests ([NUMBERS.md](NUMBERS.md)).

## 5. Metrics and evaluation

| What | Where the number is | How to recompute |
|---|---|---|
| Origin accuracy on 377 held-out 2026 rulings, with the 95% interval | [NUMBERS.md](NUMBERS.md), evaluation table, first row | `cd backend && go run ./cmd/claimscheck` |
| Fast path: share auto-accepted and its accuracy | same table | same |
| Accuracy against coverage at every confidence bar | [EVALUATION.md §3.1](EVALUATION.md#31-accuracy-against-coverage-the-confidence-bar) | `python3 -I eval/charts/build_evaluation.py . --check` |
| Calibration: expected calibration error, Brier score | [NUMBERS.md](NUMBERS.md), calibration rows; [EVALUATION.md](EVALUATION.md) | same |
| Precedent check: precision, recall, specificity | [EVALUATION.md §3.2](EVALUATION.md#32-precision-against-recall-in-the-planners-precedent-check) | `GET /v1/eval/summary`, built by `make eval-summary` |
| Cost and latency per prediction | [NUMBERS.md](NUMBERS.md) | from the same held-out rows |
| Failure cases, accuracy by origin | section 6 below; the Accuracy page | `python3 -I eval/charts/build_trust_data.py . --check` |
| Rule check coverage and agreement on the Part 102 rulings | [EVALUATION.md §9](EVALUATION.md#9-rule-check-19-cfr-10220-tariff-shift-offline-22-nov-2026-d360-d362); from the facts it decides 0 of 72 development and 1 of 45 held-out rulings; from the full ruling text 8 of 72 development and 4 of 32 held out, all as CBP held (17 chapters, [§13](EVALUATION.md#13-ten-more-chapters-in-the-rule-check-22-nov-2026-d460-d461); with the first 7: 0 and 0 from the facts, 2 and 2 from the full text) | `cd backend && go run ./cmd/tariffshift eval -check` |
| Rule check on plan moves | [EVALUATION.md §11](EVALUATION.md#11-the-rule-check-on-each-plan-move-22-nov-2026-d440-d442); 25 of 57 per-family moves decided with development suggestions, 21 of 23 agree with the precedent check; 3 scored Part 102 plan searches (§11.1, development): the actuator's 6 motor moves shift under the rule and its shown plan is one of them, and 2 moves precedent leaned to are flagged; no later ruling on a plan move to compare with | `cd backend && go run ./cmd/tariffshift plans-eval -check`, `plans102-eval -check` |
| Per-part HTS suggestions: top-1 against CBP's stated part codes, and how often the rule check then decides | [EVALUATION.md §10](EVALUATION.md#10-hts-codes-suggested-for-each-part-and-the-rule-check-they-unlock-22-nov-2026-d380-d382); dev 20 of 27, held out 2 of 3; with suggested part codes the rule check decides dev 16 of 69 and held out 14 of 29 (8 and 4 without), 17 chapters (D461; 9 and 7 with the first 7); D413 corrects one held-out input | `cd backend && go run ./cmd/tariffshift parts-eval -check` |

The recomputed rows and ledger checks run with `make reproduce`, with no model calls and no credentials. Confidence intervals, label counts by method, cost and latency figures are reported from `eval/RESULTS.md`, not recomputed there.

**Calibration finding (measured).** The engine is overconfident: its mean stated confidence is above its accuracy, by about 4.5 points on the held-out set (the calibration row of [NUMBERS.md](NUMBERS.md)). Selecting by confidence still works: accepted cases are more accurate than the rest. Treat the stated confidence as a ranking, not as a probability. The calibrated confidence the product shows for the general engine on Gemini 4 Argon is fitted to be read as a probability (D160): on the 377 it averages 90.2% against 90.2% right, ECE 0.028. The gap between fast-path accuracy and overall accuracy is not evidence of calibration ([claims-reconciliation.md](claims-reconciliation.md)).

**Evaluation data.** CBP rulings dated 2026-01-02 to 2026-09-25, frozen before scoring and scored once with the frozen configuration. A second held-out split of 83 rulings (28 Sep–13 Nov 2026) was scored once through the OpenAI-compatible LiteLLM gateway to the same Gemini models; it is reported in `eval/RESULTS.md` and [EVALUATION.md §6](EVALUATION.md#6-the-second-held-out-split-21-nov-2026). Development data: Jul–Dec 2025. How the splits are built, the exclusions and the leakage controls: [EVALUATION.md §2](EVALUATION.md#2-how-the-validation-and-test-sets-are-built).

**Known limits of the evaluation.**

- Earlier held-out rulings can be retrieved as precedent for later ones, as in real use; a case never sees its own ruling or a later one.
- The model's pre-training may have seen some 2026 rulings. This cannot be excluded.
- Rulings where CBP held two or more countries are left out, so the benchmark is silent on "it depends" answers.
- The simple-rule baselines were recomputed from committed facts with no model (`go run ./cmd/eval baselines`, D155; `eval/results/baselines-final.json`): first country named 239/377 = 63.4%, most-mentioned 229/377 = 60.7%, last-mentioned 89/377 = 23.6%, majority country 46/377 = 12.2%. Two research figures remain unverified because their write-up is not in this repository: final assembly 40.6% and last-mentioned 39.8% (a different definition we cannot reconstruct; kept only as history). The research most-mentioned figure, 60.5%, is one case from the recomputed 60.7% and is likewise labelled unverified where quoted.
- The planner's precedent check is also measured on development pairs through a proxy, and its 2026 pairs are not a clean held-out test ([DETAILS.md](DETAILS.md#limitations)). The clean post-cutoff test exists but is small: **18 frozen flip pairs** (12 real changes, 6 controls; D170 + D192), read by one person and run 3 times through the gateway; with the starting origin given, the 3 runs gave the same figures (D354). Starting-origin-given precision was 9/9 shown moves and recall 9/12 real changes (75.0%); denominators are shown moves and real-change pairs, not all 18. Recompute with `python3 -I -B eval/planner/summarize.py . --check` and `python3 -I -B eval/planner/summarize_addendum.py . --check`.

## 6. Known failure modes

Every held-out ruling the engine got wrong is listed, with a link to CBP's ruling, on the Accuracy page under "Known failure cases" and in `frontend/src/lib/failure-cases.json`. Nothing is selected out. The numbers in this section come from that file, built from `eval/results/final-gemini-4-argon-prec-engine-v1-vertex.jsonl` (measured).

**Accuracy by the origin CBP held** (held-out, Wilson 95% interval):

| Origin CBP held | Right | Accuracy |
|---|---:|---|
| Thailand | 60 / 66 | 90.9% (81.6–95.8) |
| **China** | **44 / 57** | **77.2% (64.8–86.2)** |
| United States | 51 / 53 | 96.2% (87.3–99.0) |
| Vietnam | 43 / 46 | 93.5% (82.5–97.8) |
| Malaysia | 34 / 35 | 97.1% (85.5–99.5) |
| South Korea | 20 / 22 | 90.9% (72.2–97.5) |
| Taiwan | 18 / 19 | 94.7% (75.4–99.1) |
| 26 other countries | 70 / 79 | 88.6% (79.8–93.9) |

**Which way the misses go.** Of 37 misses, 13 are cases where CBP held China and Unde said another country; 7 of those were auto-accepted. That is the costly direction for this product: it tells a manufacturer that a setup is not Chinese when CBP says it is. 18 misses go the other way (Unde said China, CBP held another country; 4 auto-accepted), and 6 involve neither.

**Patterns in the misses** (our reading of the rulings; a description, not a measurement):

| Pattern | Real held-out examples |
|---|---|
| Machining or assembly in the last country credited as a transformation, where CBP held that the imported core part decides | Wheel hub assemblies [N357817](https://rulings.cbp.gov/ruling/N357817) and [N358371](https://rulings.cbp.gov/ruling/N358371): CBP held China, Unde said Malaysia at 0.95, both auto-accepted. Deadbolt lockset [N363908](https://rulings.cbp.gov/ruling/N363908): CBP held China, Unde said Taiwan at 0.92 |
| Simple processing of one material credited as a transformation | Marking crayon [N361141](https://rulings.cbp.gov/ruling/N361141): wax melted and cast in Malaysia; CBP held China, Unde said Malaysia at 0.98. Worked abalone shells [N358060](https://rulings.cbp.gov/ruling/N358060): CBP held China, Unde said Thailand at 0.95 |
| Origin by count of subassemblies, where CBP held one essential part decides | Air compressor [N362856](https://rulings.cbp.gov/ruling/N362856): CBP held Brazil, the origin of the motor; Unde said China at 0.92. Fracking unit [N360430](https://rulings.cbp.gov/ruling/N360430): CBP held the UAE; Unde said China at 0.92 |
| Food and natural products, outside the target domain but placed in scope by the router | Red chili pepper powder [N356879](https://rulings.cbp.gov/ruling/N356879): CBP held South Korea, Unde said China at 0.98. Frozen squid [N361524](https://rulings.cbp.gov/ruling/N361524): CBP held China, Unde said South Korea at 0.99 |
| Kits, where CBP rules item by item and the label follows one item | Range Doctor kit [N362197](https://rulings.cbp.gov/ruling/N362197): the label is the scraper's origin, China; Unde said South Korea at 0.92 |
| Close calls sent to review | 25 of the 37 misses were below the bar and marked for review, for example the LiDAR module [N363700](https://rulings.cbp.gov/ruling/N363700) (CBP held Thailand, Unde said China at 0.78) |

**Other failure modes of the system** (built behaviour, documented):

- **Overconfidence** (section 5): high confidence is not a guarantee. 12 of the 300 auto-accepted held-out answers were wrong.
- **No jurisdiction guard.** A question about another country's rules gets a U.S. answer.
- **Planner recall.** The precedent check is strict: about 30% of real origin changes clear its bar on the development pairs. A move that would work may be shown as "below the bar" with its closest ruling.
- **Run-to-run variation.** Temperature 0 and a seed reduce it but do not remove it; plan verdicts are cached so a repeat search is stable.
- **Inputs are taken as true.** If the bill of materials is wrong, the answer is wrong. Imports from images and scans are drafts the user must check. Measured once on 20 synthetic images: 151/170 part names read exactly and 151/170 countries correct; 15/20 images fully read; gateway, Gemini 4 Argon. Accuracy on real photos, scans and handwriting is not measured ([MULTIMODAL.md](MULTIMODAL.md)).

## 7. Ethical and legal considerations

- **Not a ruling, and not legal advice.** Every result, export and API response carries that statement. A licensed customs broker or a CBP binding ruling confirms before anything is filed or any production moves.
- **Who carries the risk.** The U.S. importer of record carries the duty and the penalty risk if origin is declared wrong. A wrong auto-accepted answer that names a non-Chinese origin is the most harmful error; section 6 counts it separately for that reason.
- **Traceable answers.** Each prediction lists the rulings it read. Each precedent-check verdict stores its trail: every search it ran, the rulings each search returned and the rulings it read, in order (`domain.Verdict.Trail`, [DECISIONS.md](DECISIONS.md)). A tenant-scoped audit trail of account actions is built and switched on by configuration (`AUDIT_ACTOR_KEY`; [design/audit-trail.md](design/audit-trail.md)); it is not a compliance certification.
- **No personal data in the model inputs.** Product facts only. Logging rules: [design/data-handling.md](design/data-handling.md).
- **Public data only.** The corpus is public CBP rulings. No customer data was used to build or test the system.
- **Automation bias.** The fast path removes a human check for most cases. The bar was set so accepted cases stay at or above 95% accurate on the development set, and the remaining error rate is stated beside it, not hidden.

## 8. Caveats and recommendations

- Read the confidence as a ranking. Send anything near the bar, any food or natural product, any kit and any case where CBP might hold China to a broker regardless of the route.
- Re-check saved cases as CBP issues new rulings (Ruling watch). Origin practice changes.
- Re-check duty rates before acting; they change.
- The team has no customers, pilots or letters of intent. Every result here comes from public CBP rulings and can be reproduced from this repository with `make reproduce`.
