# Scope and jurisdiction (issue #282)

This is engineering documentation of what the code does and does not cover. It is not legal advice, a legal determination, or a certification of compliance, and no qualified legal review of it has been done. It states a scope, not a conclusion about any shipment.

All `file:line` anchors are at commit `c0d3115` (the base of this change). The line is a hint; the symbol or text beside it is what to search for if the code has moved. Narrative in other documents is not evidence of runtime behaviour: every behaviour below was read from code and, where one exists, a test. Deployed values were not inspected (section 8).

## 1. Summary

| Question | Answer |
|---|---|
| Which jurisdiction? | United States only: the U.S. Customs and Border Protection (CBP) country-of-origin question for non-preferential purposes, under four regimes (section 2). No other jurisdiction is encoded. |
| Which products does the planner cover? | 14 families with templates (section 4): three earlier ones and eleven added from CBP precedent (ten in D138, one in D212). |
| EU Binding Origin Information (BOI) | Unsupported. Deferred (section 7). |
| USMCA regional value content (RVC) and any preferential-origin qualification | Unsupported. Deferred (section 7). |
| Does "Part 102" in this repo mean USMCA qualification? | No. It means the Part 102 marking-origin rules (section 5). |
| Does the Part 177 export decide origin? | No. It lays out a ruling request letter (section 5). |
| What happens to a case outside the configured regimes? | Some paths fail closed to expert review, and some do not (section 3). The gaps are listed there. |

## 2. U.S. regimes as configured

The router labels each case with one regime (`backend/internal/router/router.go:54`, `Regimes`): `substantial_transformation`, `textile_102_21`, `usmca_part102`, `pharma_api`, `unclear`. A specialist engine exists for three of them. Each is off by default and is turned on by an environment flag (`backend/internal/config/config.go`).

| Regime label | Rule the prompt asks for | Engine (version) | Flag, default | Auto-accept bar, default | Precedent data | Code |
|---|---|---|---|---|---|---|
| `substantial_transformation` (and `unclear`) | Substantial transformation, CBP practice for non-preferential purposes (marking, Section 301) | general engine (`engine-v1`) | always on | `FAST_PATH_CONFIDENCE`, 0.92 (`config.go:18`) | `data/labels.jsonl` | `internal/engine/prompt.go:20`, `cmd/server/main.go:256` |
| `textile_102_21` | 19 CFR 102.21 | `textile-v1`, or `textile-v2` when `TEXTILE_PROMPT=v2` | `TEXTILE_ENGINE`, off (`config.go:364`); `TEXTILE_PROMPT` default `v1` (`config.go:322`) | `TEXTILE_FAST_PATH`, 0 = review every case (`config.go:321`) | `data/textile_rulings.json` (id list into the main pool) | `internal/engine/textile.go:6`, `cmd/server/main.go:276` |
| `usmca_part102` | 19 CFR 102.11 with the 102.20 tariff shifts, for marking | `usmca-v1` | `USMCA_ENGINE`, off (`config.go:371`) | `USMCA_FAST_PATH`, 0 (`config.go:325`) | `data/labels_usmca.jsonl` (473 rows) | `internal/engine/usmca.go:6`, `cmd/server/main.go:302` |
| `pharma_api` | Substantial transformation, applied with attention to where the active pharmaceutical ingredient is made | `pharma-v1` | `PHARMA_ENGINE`, off (`config.go:378`) | `PHARMA_FAST_PATH`, 0 (`config.go:327`) | `data/pharma_rulings.json` (id list) via `PHARMA_RULINGS` (`config.go:326`) | `internal/engine/pharma.go:4`, `cmd/server/main.go:322` |

Related flags that affect what a user sees:

| Flag | Default | Effect | Code |
|---|---|---|---|
| `EXPORT_CFR177` | off (`config.go:404`) | Allows `format: "cfr177"` on export. When off the field is refused with a validation error. | `internal/httpapi/cases.go:1237` |
| `FAST_PATH_BY_FAMILY` | off (`config.go:328`) | Per-subfamily bars replace the global bar for the general regime. | `internal/httpapi/dispatch.go:290` |
| `FALLBACK_ENGINE_MODEL` | `gemini-3.8-flash` (`config.go:282`) | Used when the primary model fails; `FALLBACK_FAST_PATH` default 0 sends every fallback answer to review. | `internal/triage/triage.go:140` |

The engines are language-model prompts over BM25-retrieved earlier CBP rulings. No regime has deterministic rule encoding: tariff shifts, essential character and the textile rules are applied by the model reading the prompt, not by code. The prompts are the only place the rules are written down (`usmca.go:12`, `textile.go:12`, `pharma.go:13`).

Accuracy evidence for each regime is in `eval/RESULTS.md` (sections "Textile regime", "USMCA / Part 102 rules on dev", "Pharmaceutical regime", "Pharmaceutical and USMCA held-out") and the figures for the general regime are in `docs/claims-reconciliation.md` (#286). This document repeats none of those numbers. Two scope limits matter here: the textile held-out run used `textile-v1` and the held-out score for `textile-v2` was not run (`eval/RESULTS.md`, "Textile prompt variants", "Held-out was not run"); and the USMCA and pharmaceutical held-out runs were proxy runs with small splits.

## 3. What happens outside the configured regimes

The decision is made in `backend/internal/httpapi/dispatch.go`: `dispatch` (line 132) runs the router and engines, `decide` (line 245) applies triage. The routing outcomes are:

| Situation | Result | Where | Test |
|---|---|---|---|
| Router labels a specialist regime and its engine is on and answers | The specialist's prediction, triaged by that regime's bar. With bar 0 every case is `needs_review` ("a new regime without a held-out result yet"). | `dispatch.go:193-219`, `dispatch.go:253-264`, `triage.go:95-134` | `TestUSMCA`, `TestPharma` (`triage_test.go:36`, `:57`) |
| Router labels a specialist regime and its engine is off | Prediction comes from the general engine, route `needs_review`, reason starts `out of scope:` (textile, Part 102 and pharmaceutical texts at `router.go:97-101`). Never auto-accepted. | `dispatch.go:197-199`, `dispatch.go:279-283`, `router.go:97-109` | `TestUnconfiguredSpecialistReviewsPerRegime`, `TestScope` |
| Specialist engine enabled but errors | Provisional general prediction, `needs_review`, reason "specialist unavailable". | `dispatch.go:220-229`, `dispatch.go:265-266` | `TestSpecialistFailureKeepsProvisionalGeneralForReview` |
| Router errors | `needs_review`, reason "scope unavailable". | `dispatch.go:184-186`, `dispatch.go:271-276`, `cases.go:424` | `TestDispatchScopeStates` |
| Re-check of a stored case that has no stored label | `needs_review`, reason "scope unavailable: the case has no stored scope label". | `dispatch.go:47`, `dispatch.go:268-270` | `TestLabelMissingNeverAutoAccepts` |
| Answer came from the fallback model | `needs_review` unless `FALLBACK_FAST_PATH` is positive and met. | `dispatch.go:287-289`, `triage.go:140-149` | `TestFallbackAlwaysReviewsUnlessFastPathSet` (`triage_test.go:82`), `TestFallbackPredictionNeedsReview` (`httpapi/cases_test.go:256`) |
| Router labels `substantial_transformation` | In scope. The general engine answers and can be auto-accepted at confidence 0.92 or more. | `router.go:104-109`, `dispatch.go:293` | `TestScope` |
| Router labels `unclear` | **Treated as in scope**, like `substantial_transformation`. It is not sent to review because of the label. | `router.go:104-109` | `TestScope` (asserts `unclear` is in scope) |

Gaps to state plainly:

- Fail closed does not cover everything. A case the router calls `substantial_transformation` or `unclear` is handled by the general engine and, with enough confidence, auto-accepted, whether or not the facts are really a U.S. non-preferential origin question.
- There is no jurisdiction detector. Nothing in the code checks that the question is about U.S. entry. A question about an EU or other non-U.S. requirement is not recognised as outside scope; it would be routed by the same labels, most likely to the general engine. The engine prompts say they are for U.S. CBP (`prompt.go:20`, `usmca.go:12`), which constrains the framing but is not a gate.
- A router configured on the server is always built (`cmd/server/main.go:502`). The "no router" state exists only in tests (`TestDispatchScopeStates`, case "no router"): there the scope label is absent and the case counts as in scope.
- Deployed values are not known (section 8). Whether a specialist engine is on in the live service decides whether a textile, Part 102 or pharmaceutical case is answered by its own engine or goes to the general engine for review.

## 4. Planner coverage

The relocation planner has 14 templates, loaded by `planner.LoadTemplates` (`backend/internal/planner/template.go`, embedded `templates/*.json`). The last eleven were chosen and written from CBP precedent; each of their groups cites the rulings where its work conferred origin or was not enough (D138, D212).

| Family key | Name in the template | File |
|---|---|---|
| `motorised_tools_small_appliances` | Motorised tools & small appliances | `backend/internal/planner/templates/motorised_tools.json` |
| `lidar_modules` | LiDAR sensor modules (backup demo family) | `backend/internal/planner/templates/lidar_modules.json` |
| `led_lights` | LED lights and lamps | `backend/internal/planner/templates/led_lights.json` |
| `pcba_electronic_devices` | PCBA-based electronic devices | `backend/internal/planner/templates/pcba_electronic_devices.json` |
| `vehicle_parts` | Vehicle parts (mechanical and electromechanical) | `backend/internal/planner/templates/vehicle_parts.json` |
| `hand_tools` | Hand tools and cutting tools | `backend/internal/planner/templates/hand_tools.json` |
| `cables_harnesses` | Cables, cord sets and wire harnesses | `backend/internal/planner/templates/cables_harnesses.json` |
| `medical_devices` | Medical devices and disposables | `backend/internal/planner/templates/medical_devices.json` |
| `power_supplies_chargers` | Power supplies, adapters and chargers | `backend/internal/planner/templates/power_supplies_chargers.json` |
| `furniture` | Furniture and mattresses | `backend/internal/planner/templates/furniture.json` |
| `display_camera_modules` | Display modules and cameras | `backend/internal/planner/templates/display_camera_modules.json` |
| `water_filters` | Water filters and filtration systems | `backend/internal/planner/templates/water_filters.json` |
| `gas_grills_cooking` | Gas grills and outdoor cooking appliances | `backend/internal/planner/templates/gas_grills_cooking.json` |
| `motors_pumps_compressors` | Electric motors, pumps and compressors | `backend/internal/planner/templates/motors_pumps_compressors.json` |

On the 2026 held-out rulings the product's family check matched 59.4% to one of these families (57.0% with the first 13, 4.2% with the first three; D138, D212, `eval/RESULTS.md`).

Other product families are not planned. A request for a family with no template is refused: `family %q has no template` (`internal/httpapi/cases.go:386`, `products.go:81`, `products.go:160`), and the family checker answers `none` rather than inventing one (`internal/familycheck/familycheck.go:1`). Origin prediction does not need a template; only plan search does.

Plan and country-comparison searches use the general engine only, with a precedent check. `planner.New(tmpls, predictor, ...)` receives the general (possibly cached) engine (`cmd/server/main.go:508`, `main.go:394`) and the planner calls it (`planner/plan.go:253`, `plan.go:376`); the compare call is `Planner.Plan` (`httpapi/compare.go:77`). So a plan for a textile, Part 102 or pharmaceutical product is not computed under that regime's rules. The export draft says so (`cases.go:1364`, `exportRegimeProvisionalNote`: "A plan search always uses the general engine"). Scenario re-runs are different: they dispatch with the case's pinned router label, so a specialist engine can answer them (`httpapi/scenarios.go:280-287`).

`TestTemplateSetIsPinned` (`planner/families_test.go`) pins the 14 families, and `TestTemplateCitationsAreRealRulings` checks every cited ruling against the bundled corpus.

## 5. Legal references and what each is

Checked against the Code of Federal Regulations (eCFR) text of 19 CFR as published on ecfr.gov (section 9 lists the pages).

| Reference | What it is | What the code does with it |
|---|---|---|
| 19 CFR 134.1(b) | Defines "country of origin" for marking. Work added in another country must effect a substantial transformation; but "for a good of a NAFTA or USMCA country, the marking rules set forth in part 102 ... will determine the country of origin". | General engine and pharmaceutical engine apply substantial transformation (`prompt.go:20`, `pharma.go:15`). |
| 19 CFR 102.0 | Part 102 sets rules for the purposes in NAFTA Annex 311 (marking, and the tariff-elimination rate and staging category). Sections 102.1 to 102.18 and 102.20 also determine country of origin for **marking** under USMCA. Section 102.21 applies to textile and apparel "for purposes of the Customs laws and the administration of quantitative restrictions". | `usmca_part102` engine = 102.11 and 102.20 for marking (`usmca.go:6`). Textile engine = 102.21 (`textile.go:8`). |
| 19 CFR 102.11 | General rules for goods "other than textile and apparel products covered by § 102.21": wholly obtained, domestic materials, or tariff shift under 102.20; then essential character, sets, and last production. | `usmca.go:12-18` follows this order. |
| 19 CFR 102.21 | Textile and apparel country-of-origin rules. | `textile.go:12`. |
| 19 CFR 177.1, 177.2 | Part 177 is the **ruling request procedure**: who may request a ruling (177.1(c)), that a ruling is a written statement applying the law to specific facts (177.1(d)), prospective transactions (177.1(a)), and the contents of a request (177.2). It contains no rule for deciding origin. | `export.CFR177` lays out a request letter on 177.2 headings (`export/cfr177.go:12-18`). Test: `TestCFR177Contents` (`export/export_test.go:127`). |

Consequences for this repo:

1. **Part 102 here is marking origin, not USMCA qualification and not RVC.** The `usmca_part102` engine answers "which country is the country of origin for marking" under 102.11 and 102.20. It does not decide whether a good is an originating good for a preferential duty claim, does not compute regional value content, and does not apply USMCA Chapter 4. The export letter for this regime says it is for marking and does not ask CBP about Section 301 (`export/regime.go:24`, `regime.go:42-51`; test `TestPart102DoesNotSettleSection301`, `regime_test.go:70`).
2. **The Part 177 export is a request letter, not a determination.** It marks unknown facts `[FILL IN: ...]` (`cfr177.go:12`), and its regime text says the requester is asking CBP, not stating the answer. It is a draft for a broker or counsel to complete and file.
3. **The textile rule is broader than marking** (102.21(a)). The export draft for the textile regime asks the question "for purposes of marking and Section 301 duties" (`regime.go:34-41`), which is the repo's wording and not the full reach of 102.21.

### Audit wording that is wrong or not supported

The two audits are kept unchanged as historical records (#286). Their scope sentence is at `unde_labs_technical_audit.md:66` and `unde_labs_technical_operational_audit.md:122`: "Scoped to U.S. CBP rulings (19 CFR Part 102 / Part 177) and does not incorporate EU Binding Origin Information (BOI) or USMCA Regional Value Content (RVC) sliding-scale rules."

| Wording | Finding | Basis |
|---|---|---|
| "19 CFR Part 102 / Part 177" as the corpus scope | **Wrong.** Part 177 is a procedure for requesting a ruling, not an origin rule or a corpus boundary. Part 102 is the marking-origin rule set and is only one of the rules the repo handles; the general and pharmaceutical regimes use substantial transformation (134.1(b)). The corpus is CBP rulings from the public ruling system, not "Part 102 / Part 177". | eCFR 177.1, 177.2, 102.0, 134.1; `docs/claims-reconciliation.md` section 6 already records the mislabel. |
| "does not incorporate ... USMCA Regional Value Content" | Correct that RVC is absent (`rg` over `backend/` and `frontend/src` finds no regional-value-content logic; the term appears only in ruling subjects). | Code search at `c0d3115`. |
| "RVC **sliding-scale** rules" | **Not verified.** The USMCA RVC rules are in the agreement and 19 CFR Part 182; those pages were not fetched (research cap) and the repo does not describe them. Treat "sliding-scale" as unverified. | Section 9. |
| "does not incorporate EU Binding Origin Information" | Correct: no EU data, code or prompt exists. | `rg -i "binding origin\|BOI"` over code finds no match. |
| "substantial transformation test (name, character, and use)" | **Not verified here.** 134.1(b) as fetched says "substantial transformation" only; the name, character and use formulation comes from case law that was not retrieved. | eCFR 134.1. |
| Engine described only as substantial transformation | **Incomplete.** The code also has flag-gated textile (102.21), Part 102 (102.11, 102.20) and pharmaceutical engines. | Section 2. |

Three items in this repo's own wording that this review could not settle and should not be read as confirmed:

- The Part 102 engine prompt says the rules apply to "the marking of goods processed in Canada or Mexico (USMCA) and other non-textile goods" (`usmca.go:12`). Under 134.1(b) and 102.0 the Part 102 rules govern the marking origin of a good of a NAFTA or USMCA country; the router label is narrower (`router.go:41` sends non-USMCA marking and Section 301 questions to `substantial_transformation`). The prompt sentence is looser than the regulation. Not edited here.
- The router label description for `usmca_part102` includes cases where "the request asks whether the good qualifies for USMCA or NAFTA preference" (`router.go:41`). The engine behind that label answers a marking question (above). A preference question that reaches that label therefore gets a marking-origin answer, always reviewed while the bar is 0, and must not be read as a preference answer.
- The CFR177 letter is addressed to the Director, National Commodity Specialist Division (`cfr177.go:27`). The eCFR text of 177.2(a) names that office for tariff classification requests; which office takes an origin request was not verified from official sources in this review.

## 6. Unsupported use cases

None of the following is implemented. A result from this system must not be used for them:

- Preferential duty claims under USMCA or any trade agreement, and any preferential-origin or "originating good" qualification.
- Regional value content, net cost, transaction value, tolerances or other RVC calculation.
- Certificates of origin, USMCA certification of origin, or any origin declaration for a trade agreement.
- Non-U.S. jurisdictions and their origin rules, including EU Binding Origin Information and EU, UK or other non-preferential and preferential origin regimes.
- Duty drawback.
- Antidumping and countervailing duty (AD/CVD) circumvention or scope determinations. No such code exists.
- Section 232 and Section 301 origin determinations. Section 301 is only touched as a separate question that the Part 102 draft keeps apart (`export/regime.go:24`); the live duty note states Section 232 duties are not included (`internal/duty/live.go:371`).
- Tariff classification. HTS suggestions are separate and are suggestions (`config.go:215`).
- Plan search for any product family outside the 14 templates (section 4).
- Any final or binding determination. CBP rulings bind for the transaction and requester they describe (19 CFR 177.9 per `docs/claims-reconciliation.md` section 6; not re-fetched here).

## 7. Deferral record

Decision on the #282 acceptance criterion "implementation/defer decision, no invented international coverage": **defer both, as two separate roadmap scopes. Nothing here is implemented and no coverage is claimed.** The repository contains no EU data, no EU or USMCA RVC licence record, and no record of validation by a qualified legal professional, and none is invented here.

Before any coverage claim for either scope, all of the following must exist as separate, reviewed artefacts:

| Prerequisite | EU Binding Origin Information (scope A) | USMCA regional value content and preferential origin (scope B) |
|---|---|---|
| Data source and licensing | An identified source of BOI decisions, its terms of use and redistribution rights, and its coverage and update cadence. Not selected. | Identified authoritative rule texts and any ruling or determination sets (agreement text, 19 CFR Part 182, product-specific rules), their terms, and update path. Not selected. |
| Deterministic rule encoding | Encoded EU origin rules and BOI decision structure in code or data, versioned, not left to a prompt. The current engines have none for any regime (section 2). | Encoded product-specific rules, RVC methods and thresholds as testable code, versioned. Not started. |
| Separate evaluation set and protocol | A held-out set of EU decisions with its own labels, split and scoring protocol, separate from the U.S. sets, frozen before scoring. | A held-out set of preferential-origin cases and RVC calculations with its own protocol, separate from the marking sets (the existing Part 102 set is marking only). |
| Review by a qualified professional | Review of rules, data use and output wording by a qualified EU customs or trade-law professional. Not done. | Review by a qualified U.S./USMCA customs or trade-law professional. Not done. |
| Product and copy changes | Jurisdiction selection, results wording that cannot be read as U.S. output, and removal of the unsupported-use-case copy for this scope only after the above. | Preference/certificate wording, separation from marking origin on every result and export, and removal of the unsupported copy only after the above. |

The two scopes are independent: finishing one gives no claim for the other. Until both are met for a scope, the product copy in section 6 stays. The product wording for what the tool is for is in the `jurisdiction.*` keys of `frontend/src/lib/copy.ts`; this document does not change the product.

## 8. What was not verified

- Deployed values. As of 20 November 2026, `.github/workflows/deploy.yml:72` sets `TEXTILE_ENGINE=on`, `TEXTILE_FAST_PATH=0.95`, `TEXTILE_PROMPT=v2`, `PHARMA_ENGINE=on`, `PHARMA_FAST_PATH=0.85`, `USMCA_ENGINE=on`, `USMCA_FAST_PATH=0.95`, `EXPORT_CFR177=on` on deploy. That file is intended configuration, not proof of the live service, and the live service was not inspected: **LIVE UNVERIFIED**. The code defaults in section 2 differ from that intent (all engines off, fast paths 0). This review did not check that each deployed bar is backed by a recorded held-out run; `eval/RESULTS.md` records the held-out run for `textile-v2` as not run.
- Which office accepts an origin ruling request, and the RVC "sliding-scale" wording (section 5).
- Anything in CBP rulings retrieval (access was denied earlier and not retried).
- Behaviour on non-U.S. text: no test submits non-U.S. facts to the router or engines.
- Model output. Router labels, engine answers and the `unclear` outcome depend on a language model; the tests above use fakes.

## 9. Research record

Official eCFR section text for 19 CFR 102.0, 102.11, 102.21, 134.1, 177.1, 177.2 (six pages, `https://www.ecfr.gov/api/versioner/v1/full/2026-11-18/title-19.xml?part=<part>&section=<section>`) was read on 20 November 2026. No web search was made. Nothing was fetched from CBP. The saved pages are not committed to this repository; the URL pattern above reproduces them, and the eCFR may have changed since. The eCFR text is as published on that date, not a legal opinion on it.
