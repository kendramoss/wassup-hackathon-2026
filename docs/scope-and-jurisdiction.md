# Scope and jurisdiction (issue #282)

This is engineering documentation of what the code does and does not cover. It is not legal advice, a legal determination, or a certification of compliance, and no qualified legal review of it has been done. It states a scope, not a conclusion about any shipment.

All `file:line` anchors are at commit `c0d3115` (the base of this change). The line is a hint; the symbol or text beside it is what to search for if the code has moved. Narrative in other documents is not evidence of runtime behaviour: every behaviour below was read from code and, where one exists, a test. The live market registry was checked on 23 November 2026. Per-engine settings are covered in section 8.

## 1. Summary

| Question | Answer |
|---|---|
| Which jurisdiction? | U.S. CBP origin prediction (section 2) and the [Canada marking rules check](#31-canada-marking-rules) are live. The [EU and UK non-preferential origin rules checks](#34-eu-and-uk-non-preferential-origin-rules) cover only the goods their lists name. Japan: not available yet — the [trade-agreement assessment](#32-japan-trade-agreements) is built but disabled in production. |
| Which products does the planner cover? | 26 families with templates (section 4): three earlier ones, eleven added from CBP precedent (ten in D138, one in D212), six written from Part 102 marking rulings (D580) and six picked from rulings with no template in either regime (D660). |
| EU Binding Origin Information (BOI) | Unsupported. Deferred (section 7). The EU rules check (section 3.4) applies the published rules; it does not read BOI decisions. |
| Preferential origin | The [standalone RCEP calculator](#33-rcep-preference-calculator) is live. USMCA regional value content is deferred (section 7). |
| Does "Part 102" in this repo mean USMCA qualification? | No. It means the Part 102 marking-origin rules (section 5). |
| Does the Part 177 export decide origin? | No. It lays out a ruling request letter (section 5). |
| What happens to a case outside the configured regimes? | Some paths fail closed to expert review, and some do not (section 3). The gaps are listed there. |

## 2. U.S. regimes as configured

The router labels each case with one regime (`backend/internal/router/router.go:54`, `Regimes`): `substantial_transformation`, `textile_102_21`, `usmca_part102`, `pharma_api`, `unclear`. A specialist engine exists for three of them. Each is off by default and is turned on by an environment flag (`backend/internal/config/config.go`).

| Regime label | Rule the prompt asks for | Engine (version) | Flag, default | Auto-accept bar, default | Precedent data | Code |
|---|---|---|---|---|---|---|
| `substantial_transformation` (and `unclear`) | Substantial transformation, CBP practice for non-preferential purposes (marking, Section 301) | general engine (`engine-v1`) | always on | `FAST_PATH_CONFIDENCE`, 0.92 (`config.go:18`) | `data/labels.jsonl` | `internal/engine/prompt.go:20`, `cmd/server/main.go:256` |
| `textile_102_21` | 19 CFR 102.21 | `textile-v1`, or `textile-v2` when `TEXTILE_PROMPT=v2` | `TEXTILE_ENGINE`, off (`config.go:364`); `TEXTILE_PROMPT` default `v1` (`config.go:322`) | `TEXTILE_FAST_PATH`, 0 = review every case (`config.go:321`) | `data/textile_rulings.json` (id list into the main pool) | `internal/engine/textile.go:6`, `cmd/server/main.go:276` |
| `usmca_part102` | 19 CFR 102.11 with the 102.20 tariff shifts, for marking | `usmca-v5` (since D602; `usmca-v1` before) | `USMCA_ENGINE`, off (`config.go:371`) | `USMCA_FAST_PATH`, 0 (`config.go:325`) | `data/labels_usmca.jsonl` (473 rows) | `internal/engine/usmca.go:6`, `cmd/server/main.go:302` |
| `pharma_api` | Substantial transformation, applied with attention to where the active pharmaceutical ingredient is made | `pharma-v1` | `PHARMA_ENGINE`, off (`config.go:378`) | `PHARMA_FAST_PATH`, 0 (`config.go:327`) | `data/pharma_rulings.json` (id list) via `PHARMA_RULINGS` (`config.go:326`) | `internal/engine/pharma.go:4`, `cmd/server/main.go:322` |

Related flags that affect what a user sees:

| Flag | Default | Effect | Code |
|---|---|---|---|
| `EXPORT_CFR177` | off (`config.go:404`) | Allows `format: "cfr177"` on export. When off the field is refused with a validation error. | `internal/httpapi/cases.go:1237` |
| `FAST_PATH_BY_FAMILY` | off (`config.go:328`) | Per-subfamily bars replace the global bar for the general regime. | `internal/httpapi/dispatch.go:290` |
| `FALLBACK_ENGINE_MODEL` | `gemini-3.8-flash` (`config.go:282`) | Used when the primary model fails; `FALLBACK_FAST_PATH` default 0 sends every fallback answer to review. | `internal/triage/triage.go:140` |
| `RULE_CHECK` | on | Adds the deterministic 19 CFR 102.20 rule check (`rule_check`) to a case whose product gives HTS codes for the good and a part. Display only: it never changes the answer, the triage or auto-accept (D363). | `internal/httpapi/rulecheck.go` |

The engines are language-model prompts over BM25-retrieved earlier CBP rulings. The answer a user gets comes from the model reading the prompt: tariff shifts, essential character and the textile rules are applied there (`usmca.go:12`, `textile.go:12`, `pharma.go:13`). Since D360 the 19 CFR 102.20 tariff-shift rows for chapters 73, 82, 84, 85, 87, 90 and 94 are also encoded in tested code (`internal/tariffshift`), but only as a check shown beside the answer when HTS codes are given (D363); it decides nothing on its own. The textile rules are not encoded.

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
| Router labels `unclear` | **Treated as in scope**, like `substantial_transformation`, unless the text hints at a non-U.S. market (D680, below): then `needs_review`. | `router.go`, `decide` in `dispatch.go` | `TestScope`, `TestMarketConflictAndHint` |

Since D680 the market is decided before any of the rows above (`dispatch` and `decide` in `dispatch.go`, `internal/jurisdiction`):

| Situation | Result | Test |
|---|---|---|
| The caller chose a market (`jurisdiction` on predict, batch, MCP) | That market; the text is not read for one. | `TestChosenMarketWins` |
| No choice; the text names a non-U.S. authority, law, entry into a non-U.S. place or "<market> customs" | That market (`source: detected`, with the cue). | `TestDetect`, `TestUnsupportedMarketNeverGetsAUSAnswer` |
| The market has no registered engine | No router or engine call and no prediction; `needs_review`, reason "Origin rules for <market> aren't supported yet". Plan, compare, scenarios, export, ask, watch and broker review are refused with the same reason. | `TestUnsupportedMarketNeverGetsAUSAnswer` (with router `unclear` and general), `TestFollowUpsNeedASupportedMarket`, `TestBatchRowsCarryTheMarket` |
| A non-U.S. market registered with its own engine | That engine answers; `needs_review` unless its own bar (0 by default) is met. Follow-ups stay U.S.-only. | `TestRegisteredMarketUsesItsEngine`, `TestRegistry` |
| No choice; U.S. and non-U.S. authority cues together | U.S. dispatch, but `needs_review` ("The text names more than one market"). | `TestMarketConflictAndHint` |
| No choice; only a weak non-U.S. cue (a price, "sold in") | U.S. dispatch; `needs_review` only when the router says `unclear`. | `TestMarketConflictAndHint` |
| No cue | United States, as before D680. | `TestNoCueIsUS` |

Gaps to state plainly:

- Fail closed does not cover everything. A case the router calls `substantial_transformation` or `unclear` is handled by the general engine and, with enough confidence, auto-accepted, whether or not the facts are really a U.S. non-preferential origin question.
- The market detector is a net, not a gate (D680-D682). On model-written non-U.S. questions it read 22 of 30 right and sent 4 more to review only if the router says `unclear`; the other 4 were read as U.S. and are answered as U.S. cases. It flagged none of 1,026 U.S. rulings. A caller who names the market is never second-guessed. The engine prompts still say they are for U.S. CBP (`prompt.go:20`, `usmca.go:12`).
- A router configured on the server is always built (`cmd/server/main.go:502`). The "no router" state exists only in tests (`TestDispatchScopeStates`, case "no router"): there the scope label is absent and the case counts as in scope.
- The public market registry reports market availability. On U.S. cases, the specialist switches determine whether textile, Part 102 or pharmaceutical questions go to their own engine or to the general engine for review (section 8).

### 3.1 Canada marking rules

The Canada case flow and `POST /v1/rules/canada-marking` apply the Schedule III tariff shifts and general rules in SOR/94-23 for marking goods from CUSMA countries. Supply the finished good and material tariff codes and the production country. The check runs without a model. Canada has too few published rulings for an accuracy figure (D720–D725).

### 3.2 Japan trade agreements

Japan: not available yet in production (`jp.supported=false`). The model is built for selected trade agreements. The unseen-test requests returned HTTP 403 and yielded no eligibility results (D714). The separate synthetic requests also yielded no results (D715).

For imports into Japan, supply a trade agreement and the finished good's tariff classification in that agreement's HS edition, plus the materials and production facts. The engine uses product-specific rule text and earlier rulings from Japan Customs under the same agreement. See the [sources and HS editions](../backend/internal/jprules/README.md). The result gives likely eligibility, a preferential origin where known, and the sources. It goes to specialist review in every case. The published test rulings cover goods that qualified.

If the inputs or an applicable rule are missing, the assessment stays undetermined. General Japanese origin, origin certificates, duty savings and relocation plans sit outside this scope. Review by a licensed Japanese customs specialist is tracked in [#665](https://github.com/kendramoss/wassup-hackathon-2026/issues/665).

The Japan engine is on by default; `JAPAN_ENGINE=off` (or `false`, `0`) makes Japan unavailable. With the dedicated `JAPAN_LITELLM_BASE_URL` and `JAPAN_LITELLM_API_KEY` settings, it uses that private gateway. Without them, and with the Vertex provider, it reuses the production engine model, with the same gate, retries and spend meter as the other engines. If no model is usable, Japan stays unavailable. The local fake provider can exercise the same route offline. No accuracy figure is claimed for the live route. Call `GET /v1/jurisdictions` to check availability on a deployment. Production enablement is tracked in [#694](https://github.com/kendramoss/wassup-hackathon-2026/issues/694).

### 3.3 RCEP preference calculator

`POST /v1/rcep/check` applies the RCEP Annex 3A rules in the HS 2022 tariff edition. It uses the supplied tariff codes and, where the rule requires them, values, weights and supplier origin declarations. This calculator runs without a model and has its own outcome (D690).

In the unseen Japan Customs set, all 98 published rulings qualified. The calculator returned yes on 71 and undetermined on 27, with zero refusals or input errors. The deciding criterion agreed in all 71 decisions. The synthetic tariff-code mutations were all undetermined. Refusal performance on real cases remains unmeasured. [Evaluation rows and intervals](../eval/RESULTS.md#rcep-calculator-on-japan-customs-rulings-d691-d716-d717)

### 3.4 EU and UK non-preferential origin rules

Two separate checks, one per market, with the same shape (D760-D766): `POST /v1/rules/eu-origin` and `POST /v1/rules/uk-origin`, the `check_eu_origin` and `check_uk_origin` MCP tools, and the origin page of an EU or UK case. They run without a model. Both are rules checks only, with no accuracy figure: no origin decisions with their facts are published in either market to test them against.

**Question.** The non-preferential country of origin of a good imported into the EU, or into the UK. Supply the good's HS code, the country where it was made and its materials' codes and origins. Value or weight shares, a list row and yes/no facts are asked for when a rule needs them. Not preferential (trade-agreement) origin, not tariff classification, not a ruling.

| | EU | UK |
|---|---|---|
| Rules | Union Customs Code (Regulation (EU) No 952/2013) Art. 59-63; Delegated Regulation (EU) 2015/2446 Art. 31-36 and the list rules of Annex 22-01 | Taxation (Cross-border Trade) Act 2018 s. 17; SI 2020/1433 regs 4-11; Part Four of HMRC's reference document |
| Text version | EUR-Lex consolidated text CELEX 02015R2446-20261101 (1 Nov 2026), SHA-256 `87be4f1a…818be` | Reference document v1.2, 12 April 2023 (in effect from 10 May 2023), SHA-256 `239b6b3d…b850d` |
| Who applies it | Common EU rules. Each Member State's customs authority applies them, issues Binding Origin Information and checks declarations | HMRC |
| Published decisions | Public BOI list gives outcomes only, no facts or reasoning; not used. EBTI is tariff classification; not used | HMRC publishes no Advance Origin Rulings |
| Label on every answer | "Rules check only — no accuracy figure (no published origin decisions with their facts to test against)" | "Rules check only — no accuracy figure (no published origin rulings to test against)" |

**Northern Ireland.** Under the Windsor Framework the EU's customs rules apply to many goods entering Northern Ireland. A case whose text names Northern Ireland (or Belfast, Londonderry, Derry) as the destination is not read as the UK: it gets no answer and asks the user to pick the EU or the UK market (D767). A market the user picks is used as chosen.

**Goods with list rules.** The same 37 HS chapters in both lists, with rules for these headings only (an `ex` heading covers only the goods its description names; other headings may be covered only for some subheadings or descriptions; the full rows are in `rules_eu.json` and `rules_uk.json`):

| Chapter | Headings |
|---|---|
| 02 | 0201 to 0206 |
| 04, 09, 11, 14, 17 | ex 0408; 0901 (subheadings 0901 11 to 0901 22); 1101 to 1109; ex 1404; 1701 to 1704 |
| 20, 22, 34, 35, 42, 49 | ex 2009; ex 2204, ex 2205; ex 3401, ex 3405; ex 3502; ex 4203; ex 4910 |
| 50 to 63 (textiles) | Most headings of each chapter |
| 64, 69, 71 | 6401 to 6405; ex 6911 to ex 6913; ex 7117 |
| 72, 73 | 7201 to 7229; 7301 to 7326 |
| 82 | 8201 to 8207 |
| 84 | ex 8443 (photocopiers), ex 8473 (memory modules), ex 8482 (assembled bearings) only |
| 85 | ex 8501, ex 8523 59, ex 8525 (EU: ex 8525 80, read as 8525 81 to 89), 8527, 8528, 8535, ex 8536, 8541 (split: ex 8541 (a) crystalline silicon photovoltaic cells, ex 8541 (b) other), 8542, ex 8548 (EU: ex 8548 90, read as 8548 00); the UK also has 8524 |
| 90, 91, 94 | ex 9029; ex 9113; ex 9401, ex 9403, ex 9405; the UK also has ex 9021 |

**What is decided, and how.** A change of classification (CC, CTH, CTSH and their split forms, with "except from" clauses) is decided from the codes, with the 10% value tolerance. Rules written as work done ("Manufacture from yarn", "Assembly of semi-conductor products", "Complete making-up") are decided only from a yes/no answer. When the good's code reaches rows limited by description, the caller picks the row. When the primary rule is not met, the chapter residual rules give the origin from the materials' value or weight shares. EU 446 rows and UK 452 rows; 199 and 205 alternatives decided from codes and shares, 338 and 342 from answers, 5 in each not read (D763).

**What is not decided.**
- Any good without a list rule: every heading of the other 60 chapters, and the headings of listed chapters without a row (for example 8471 computers). The answer is "needs a substantial-transformation judgement", citing Art. 60(2) UCC or TCTA 2018 s. 17(3). No origin is given (D764).
- Rules that name the origin by where work was done (meat, eggs, scrap): a "yes, done in the country of production" decides; a "no" goes to a person, because the rule then names another country. Equal shares, mixtures weighed by dry matter, and changes between splits of the same heading also go to a person.
- Two EU rows keep HS 2017 codes (ex 8525 80, ex 8548 90, added in 2018 and not renumbered for HS 2022). The check reads them as the HS 2022 subheadings they became (8525 81 to 89, 8548 00); the row text is unchanged (D767).
- UK minimal operations: the UK text gives no fallback, so a judgement. The EU check applies Art. 34 DA (D764).
- Accessories, spare parts and tools, packing, neutral elements, wholly obtained goods beyond the rows, and proof of origin.

## 4. Planner coverage

Plan search, target comparison, Ask, and the ruling-request and broker review workflows are available for U.S. cases. The Canada and RCEP rules checks have their own routes.

The relocation planner has 26 templates, loaded by `planner.LoadTemplates` (`backend/internal/planner/template.go`, embedded `templates/*.json`). Eleven were chosen and written from CBP substantial-transformation precedent (D138, D212), and six from Part 102 marking rulings dated before 2024 (D580, `"regime": "part102"`); each of their groups cites the rulings where its work conferred origin or was not enough. In the six, each movable group makes one input whose tariff heading the 102.20 rule reads. Six more were written from substantial-transformation rulings dated before 2026 and in no development, held-out or tune-check split (D660), after counting the development and pool rulings of both regimes that had no template.

| Family key | Name in the template | File |
|---|---|---|
| `motorised_tools_small_appliances` | Motorised tools & small appliances | `backend/internal/planner/templates/motorised_tools.json` |
| `lidar_modules` | LiDAR sensor modules (backup demo family) | `backend/internal/planner/templates/lidar_modules.json` |
| `led_lights` | LED lights and lamps | `backend/internal/planner/templates/led_lights.json` |
| `pcba_electronic_devices` | PCBA-based electronic devices | `backend/internal/planner/templates/pcba_electronic_devices.json` |
| `vehicle_parts` | Vehicle parts (mechanical and electromechanical) | `backend/internal/planner/templates/vehicle_parts.json` |
| `hand_tools` | Hand tools and cutting tools | `backend/internal/planner/templates/hand_tools.json` |
| `cables_harnesses` | Cables, cord sets and wire harnesses | `backend/internal/planner/templates/cables_harnesses.json` |
| `medical_devices` | Medical devices and disposables (v2, D782) | `backend/internal/planner/templates/medical_devices.json` |
| `power_supplies_chargers` | Power supplies, chargers and battery packs (v2, D782) | `backend/internal/planner/templates/power_supplies_chargers.json` |
| `furniture` | Furniture and mattresses (v2, D782) | `backend/internal/planner/templates/furniture.json` |
| `display_camera_modules` | Display modules, cameras and lenses (v2, D782) | `backend/internal/planner/templates/display_camera_modules.json` |
| `water_filters` | Water filters and filtration systems | `backend/internal/planner/templates/water_filters.json` |
| `gas_grills_cooking` | Gas grills and outdoor cooking appliances | `backend/internal/planner/templates/gas_grills_cooking.json` |
| `motors_pumps_compressors` | Motors, pumps, compressors and industrial machines (v2, D782) | `backend/internal/planner/templates/motors_pumps_compressors.json` |
| `food_preparations` | Food and beverage preparations (Part 102) | `backend/internal/planner/templates/food_preparations.json` |
| `metal_products` | Metal semi-finished and fabricated products (Part 102) | `backend/internal/planner/templates/metal_products.json` |
| `wood_panels_flooring` | Plywood, wood flooring and panels (Part 102) | `backend/internal/planner/templates/wood_panels_flooring.json` |
| `plastic_articles` | Plastic film, compounds and moulded articles (Part 102) | `backend/internal/planner/templates/plastic_articles.json` |
| `electrical_control_apparatus` | Electrical control panels and busbars (Part 102) | `backend/internal/planner/templates/electrical_control_apparatus.json` |
| `machinery_parts` | Machinery parts and subassemblies (Part 102) | `backend/internal/planner/templates/machinery_parts.json` |
| `paper_tissue_products` | Paper, tissue and paperboard products (D660) | `backend/internal/planner/templates/paper_tissue_products.json` |
| `offroad_vehicles_machinery` | Off-road vehicles and mobile machinery (D660) | `backend/internal/planner/templates/offroad_vehicles_machinery.json` |
| `semiconductor_devices` | Semiconductor devices (D660) | `backend/internal/planner/templates/semiconductor_devices.json` |
| `golf_sporting_goods` | Golf clubs and sporting goods (D660) | `backend/internal/planner/templates/golf_sporting_goods.json` |
| `toys_build_kits` | Toys and build kits (D660) | `backend/internal/planner/templates/toys_build_kits.json` |
| `hvac_heating_units` | Heat pumps, furnaces and heaters (D660) | `backend/internal/planner/templates/hvac_heating_units.json` |

Batteries, machinery, furniture, and medical or optical goods (D780–D784) have no template of their own; they are covered inside the templates below, at version 2, and a deterministic guard after the family check (`family-guard-v3`, D786, D789) sends look-alike products to review.

| Area | Template (version) | Covered by cited rulings | Not covered |
|---|---|---|---|
| Batteries and energy storage | `power_supplies_chargers` (2) | Cells and cell stacks, battery packs, power banks, portable power stations; cell origin decided in some rulings, PCBAs in another | Cell-material plants (chemicals), battery-powered devices (guarded to review), no battery template |
| Industrial machinery | `motors_pumps_compressors` (2) | Motors, pumps, compressors; a machine's working unit, frames and tooling, assembly with programming | Lifting jacks, machine tools by name, software loading on its own (no ruling) |
| Furniture | `furniture` (2) | Tabletops, frames and bases, foam and upholstery, mechanisms, knock-down packing | Finishing and upholstery work on their own; lamps, pet beds and electrical cabinets (guarded to review) |
| Medical devices | `medical_devices` (2) | Cannulas and blanks, moulded parts and tubing, PCBA and sensors, hubs and housings, sterilisation and packing (not enough) | Drugs (guarded; own engine), regulatory status (never implied) |
| Optical and precision | `display_camera_modules` (2) | Lens milling (decided), polishing and coating (not enough) | Optical instruments as a family (two citable rulings), toys with optics (guarded) |

On the 2026 held-out rulings the product's family check matched 79.0% (298 of 377) to one of the 26 families, read once (D661), against 59.4% with the first 14 (57.0% with the first 13, 4.2% with the first three; D138, D212, `eval/RESULTS.md`); no 20-family run of that set exists. With 20, it matched 68.1% of the Part 102 development rulings (27.8% with 14) and 8 of the 18 frozen held-out Part 102 rulings (6 with 14; D580, D581); with 26, 69.4% and 8 of 18 (D660, D661).

Other product families are not planned. A request for a family with no template is refused: `family %q has no template` (`internal/httpapi/cases.go:386`, `products.go:81`, `products.go:160`), and the family checker answers `none` rather than inventing one (`internal/familycheck/familycheck.go:1`). Origin prediction does not need a template; only plan search does.

Plan and country-comparison searches use the general engine, with a precedent check, except on a case routed to Part 102. `planner.New(tmpls, predictor, ...)` receives the general (possibly cached) engine and, in `planner.Options.Engines`, the Part 102 engine when `USMCA_ENGINE=on` (`cmd/server/main.go`); the plan, estimate and compare routes ask for `usmca_part102` on a Part 102 case (`planRegime` in `httpapi/planrule.go`), and the planner then predicts the current setup and every move with that engine (`engineFor` in `planner/plan.go`; D540, D541). A plan for a textile or pharmaceutical product is still not computed under that regime's rules, and the export draft says so (`exportRegimeProvisionalNote` in `httpapi/cases.go`: "Plan searches use the general engine except on Part 102 cases"). A plan on a Part 102 case carries the Part 102 engine's trace, so its draft is asked under the Part 102 rules with that engine's note. A Part 102 plan searched before D540, or with the Part 102 engine off or failed, still asks the Part 102 marking question with a note that the position is the general engine's, not a Part 102 determination (`export/regime.go`, `part102RoutedNote`; D520). The precedent check reads substantial-transformation rulings on every case. Scenario re-runs are different: they dispatch with the case's pinned router label, so a specialist engine can answer them (`httpapi/scenarios.go:280-287`).

`TestTemplateSetIsPinned` (`planner/families_test.go`) pins the 26 families, and `TestTemplateCitationsAreRealRulings` checks every cited ruling against the bundled corpus.

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

1. **Part 102 here is marking origin, not USMCA qualification and not RVC.** The `usmca_part102` engine answers "which country is the country of origin for marking" under 102.11 and 102.20. It does not decide whether a good is an originating good for a preferential duty claim, does not compute regional value content, and does not apply USMCA Chapter 4. The export letter for this regime says it is for marking and does not ask CBP about Section 301 (`export/regime.go:37`, `regime.go:55-64`; test `TestPart102DoesNotSettleSection301`, `regime_test.go:70`).
2. **The Part 177 export is a request letter, not a determination.** It marks unknown facts `[FILL IN: ...]` (`cfr177.go:12`), and its regime text says the requester is asking CBP, not stating the answer. It is a draft for a broker or counsel to complete and file.
3. **The textile rule is broader than marking** (102.21(a)). The export draft for the textile regime asks the question "for purposes of marking and Section 301 duties" (`regime.go:47-54`), which is the repo's wording and not the full reach of 102.21.

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

- Filing preferential duty claims or obtaining preferential-origin certificates. The RCEP calculator supports preliminary qualification from the supplied facts. It awaits qualified professional review (D690).
- USMCA regional value content, net cost and transaction value calculations. The RCEP calculator uses the value-content methods in that agreement.
- Certificates of origin, USMCA certification of origin, or any origin declaration for a trade agreement.
- EU Binding Origin Information, the origin of EU or UK goods without a list rule (a substantial-transformation judgement, section 3.4), EU and UK preferential origin, and other customs regimes outside the shipped checks above. Japan model activation is tracked in section 3.2.
- Duty drawback.
- Antidumping and countervailing duty (AD/CVD) circumvention or scope determinations. No such code exists.
- Section 232 and Section 301 origin determinations. Section 301 is only touched as a separate question that the Part 102 draft keeps apart (`export/regime.go:37`); the live duty note states Section 232 duties are not included (`internal/duty/live.go:371`).
- Tariff classification. HTS suggestions are separate and are suggestions (`config.go:215`).
- Plan search for any product family outside the 26 templates (section 4).
- Any final or binding determination. CBP rulings bind for the transaction and requester they describe (19 CFR 177.9 per `docs/claims-reconciliation.md` section 6; not re-fetched here).

## 7. Deferral record

Decision on the #282 acceptance criterion "implementation/defer decision, no invented international coverage": **defer both, as two separate roadmap scopes. Nothing here is implemented and no coverage is claimed.** The EU rules check of section 3.4 (D760-D766) applies the published non-preferential list rules; it reads no BOI decision and does not change scope A below. An EU BOI corpus, licence records for these two scopes, and qualified professional validation remain prerequisites.

Before any coverage claim for either scope, all of the following must exist as separate, reviewed artefacts:

| Prerequisite | EU Binding Origin Information (scope A) | USMCA regional value content and preferential origin (scope B) |
|---|---|---|
| Data source and licensing | An identified source of BOI decisions, its terms of use and redistribution rights, and its coverage and update cadence. Not selected. | Identified authoritative rule texts and any ruling or determination sets (agreement text, 19 CFR Part 182, product-specific rules), their terms, and update path. Not selected. |
| Deterministic rule encoding | Encoded EU origin rules and BOI decision structure in code or data, versioned, not left to a prompt. No EU origin schedule is encoded. | Encoded product-specific rules, RVC methods and thresholds as testable code, versioned. Not started. |
| Separate evaluation set and protocol | A held-out set of EU decisions with its own labels, split and scoring protocol, separate from the U.S. sets, frozen before scoring. | A held-out set of preferential-origin cases and RVC calculations with its own protocol, separate from the marking sets (the existing Part 102 set is marking only). |
| Review by a qualified professional | Review of rules, data use and output wording by a qualified EU customs or trade-law professional. Not done. | Review by a qualified U.S./USMCA customs or trade-law professional. Not done. |
| Product and copy changes | Jurisdiction selection and results wording that cannot be read as U.S. output: built (D680; a market picker, a `jurisdiction` field, no answer for a market without an engine). Removal of the unsupported-use-case copy for this scope only after the above. | Preference/certificate wording, separation from marking origin on every result and export, and removal of the unsupported copy only after the above. |

The prerequisites apply to each scope separately. The product wording is in the `jurisdiction.*` keys of `frontend/src/lib/copy.ts`.

## 8. What was not verified

- Live U.S. specialist switches and bars. As of 20 November 2026, `.github/workflows/deploy.yml:72` sets `TEXTILE_ENGINE=on`, `TEXTILE_FAST_PATH=0.95`, `TEXTILE_PROMPT=v2`, `PHARMA_ENGINE=on`, `PHARMA_FAST_PATH=0.85`, `USMCA_ENGINE=on`, `USMCA_FAST_PATH=0.95`, `EXPORT_CFR177=on` on deploy. That workflow records intended settings. The observed market registry reports market availability separately. The code defaults in section 2 differ from that intent (all engines off, fast paths 0). This review did not check that each deployed bar is backed by a recorded held-out run; `eval/RESULTS.md` records the held-out run for `textile-v2` as not run.
- Which office accepts an origin ruling request, and the RVC "sliding-scale" wording (section 5).
- Anything in CBP rulings retrieval (access was denied earlier and not retried).
- Market detection on real non-U.S. requests. D682 measured synthetic questions.
- Model output. Router labels, engine answers and the `unclear` outcome depend on a language model; the tests above use fakes.
- Revocations outside the corpus. Precedent checks skip rulings that a revocation or modification letter in the corpus names (D320, `backend/internal/domain/supersession.json`), but CBP revocations published only in the Customs Bulletin, or by letters that are not in the corpus, are not detected. A cited ruling can still have been revoked; check it on rulings.cbp.gov before relying on it.

## 9. Research record

Official eCFR section text for 19 CFR 102.0, 102.11, 102.21, 134.1, 177.1, 177.2 (six pages, `https://www.ecfr.gov/api/versioner/v1/full/2026-11-18/title-19.xml?part=<part>&section=<section>`) was read on 20 November 2026. No web search was made. Nothing was fetched from CBP. The saved pages are not committed to this repository; the URL pattern above reproduces them, and the eCFR may have changed since. The eCFR text is as published on that date, not a legal opinion on it.
