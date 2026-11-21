# Datasheet: the CBP ruling corpus

This datasheet follows the questions of Gebru et al., *Datasheets for Datasets* (2018/2021): motivation, composition, collection, labelling, uses, distribution and maintenance. It covers the labelled corpus in `eval/data/labels.jsonl` (also bundled as `backend/data/labels.jsonl`) and the facts files in `backend/data/facts/`.

Every count here is measured and recomputed by `python3 -I eval/charts/build_trust_data.py . --check`, which writes `eval/results/corpus-stats.json`. The split counts match [EVALUATION.md §2](EVALUATION.md#2-how-the-validation-and-test-sets-are-built).

## 1. Motivation

- **Why it exists.** To predict the country of origin U.S. Customs and Border Protection (CBP) would assign, a model needs the decisions CBP has already made. The corpus is the precedent pool the engine retrieves from, and the source of the labelled test sets.
- **Who made it.** The Unde Labs team (two people), from public CBP rulings.
- **Funding.** None. Built for the Google Cloud AI Builder Cup 2026.

## 2. Composition

| | Count | Source |
|---|---:|---|
| Labelled rulings | 2,790 | `labels.jsonl` |
| Dated | 2023-01-10 to 2026-09-25 | same |
| By year | 2023: 354 · 2024: 425 · 2025: 1,358 · 2026: 653 | same |
| New York (NY) / headquarters (HQ) rulings | 2,619 / 171 | `collection` |
| Facts files bundled | 3,081 (the 2,790 above plus 291 Part 102 rulings from 2015 on, for the Part 102 engine) | `backend/data/facts/` |

**One row per ruling**: ruling number, date, collection, subject line, origin regime, whether several countries are involved, label method, CBP's held origin (`primary_origin`, empty when there is no single origin) and the holding sentences (`evidence`).

**Origin regimes** (as labelled):

| Regime | Rulings |
|---|---:|
| Substantial transformation | 1,703 |
| Part 102 tariff shift (marking origin) | 456 |
| Single country | 260 |
| Pharmaceutical active ingredient | 192 |
| U.S. components assembled abroad (9802) | 137 |
| Other | 25 |
| Marking only | 17 |

**Substantial-transformation rulings**, the main engine's pool: 1,703, of which 1,593 have a single held origin, across 93 countries. The 15 most frequent held origins:

| Origin | Rulings | Origin | Rulings | Origin | Rulings |
|---|---:|---|---:|---|---:|
| Vietnam | 266 | South Korea | 109 | Philippines | 42 |
| China | 243 | Malaysia | 82 | Japan | 33 |
| Thailand | 236 | Indonesia | 80 | India | 26 |
| United States | 150 | Taiwan | 73 | Mexico | 20 |
| | | Cambodia | 46 | Germany | 19 |
| | | | | Canada | 13 |

China and the ASEAN members together are 62.8% of those 1,593.

**Splits** (frozen 2026-10-14 in `eval/splits/splits_final.json`; only single-origin substantial-transformation rulings):

| Split | n | Dates | NY / HQ | Labels regex / manual | Most frequent held origins |
|---|---:|---|---|---|---|
| dev | 469 | 2025-07-01 to 2025-12-31 | 454 / 15 | 448 / 21 | Vietnam 102, China 80, Thailand 74, United States 58 |
| final (held out) | 377 | 2026-01-02 to 2026-09-25 | 364 / 13 | 355 / 22 | Thailand 66, China 57, United States 53, Vietnam 46 |
| pilot (never scored) | 12 | 2026-05-05 to 2026-05-15 | 10 / 2 | 12 / 0 | China 5, Thailand 3 |

Textile, pharmaceutical and Part 102 splits are built by their own committed scripts ([EVALUATION.md §2.1](EVALUATION.md#21-from-2790-rulings-to-the-splits)).

**Sensitive data.** None expected. Rulings name importers, brands and sometimes their counsel; CBP publishes them. Importers can ask CBP to keep confidential business details out of the published ruling (19 CFR 177.2(b)(7)). The corpus has no personal data beyond names that appear in those public rulings.

## 3. Collection

- **Source.** CBP's Customs Rulings Online Search System (CROSS), [rulings.cbp.gov](https://rulings.cbp.gov), through its public JSON endpoint (`https://rulings.cbp.gov/api/ruling/<id>`). `go run ./cmd/corpus fetch` downloads each ruling listed in `labels.jsonl` (about 50 minutes with the default one-second pause).
- **Which rulings.** Country-of-origin rulings from 2023 to September 2026. The list was assembled in the team's earlier research workspace; **the search queries that selected it are not in this repository**, so the selection cannot be re-run from here, only the download and the labels.
- **Raw texts are not committed.** Only the labels and the facts sections are. The facts are cut at the first legal-analysis cue, and sentences that state the origin conclusion are redacted (`corpus.SplitFacts`), so the engine sees the facts, not CBP's answer.
- **Integrity.** `eval/SHA256SUMS` pins the data files; `make verify-data` checks them and that the bundled copy matches.

## 4. Labelling

- **The label is CBP's stated holding**: the origin CBP held in the ruling, not the team's view of the right answer.
- **Method.** 2,696 labels were extracted by regular expression from the holding sentence; 94 were set by hand where the expression failed (`method`). The holding sentences are kept in `evidence`, so every label can be checked against its source text.
- **Checks.** A second extractor and a 30-item audit were run in the research workspace; their records are not in this repository (reported). Six labels the model, acting as a judge, called wrong were re-read by hand: all six matched CBP (D23). Scoring adjustments for multi-scenario and ambiguous dev rulings are in `eval/data/scoring_adjustments.json` and apply to every run.
- **Multi-country holdings.** Rulings where CBP held two or more countries are stored as `"A | B"` and left out of the dev and final splits ([EVALUATION.md §2.2](EVALUATION.md#22-the-exclusions-reconstructed)).

## 5. Known biases and gaps

- **NY rulings dominate.** 94% of the corpus (2,619 of 2,790) are New York rulings, which answer one importer's request. HQ rulings, often the harder and more reasoned cases, are 171. Held-out results say little about HQ-style questions (13 cases).
- **Recent years dominate.** 2025 alone is 49% of the corpus. Precedent for 2023–24 practice is thinner, and the year counts reflect how the list was assembled as much as CBP's output.
- **Country mix.** China and ASEAN are 62.8% of substantial-transformation holdings, matching the product's target but not CBP's whole caseload. Results for other origins rest on fewer rulings (the 26 smallest origins in the held-out set share 79 rulings).
- **Who asks.** Rulings exist only where an importer asked CBP. Setups that importers were confident about, or that they did not want to expose, are under-represented.
- **Single holdings only in the test sets.** "It depends" holdings are excluded from scoring.
- **Product mix.** Mostly assembled manufactured goods, with some food, chemicals and natural products. The engine is weakest on the latter ([MODEL-CARD.md §6](MODEL-CARD.md#6-known-failure-modes)).

## 6. Uses

- **Used for:** the engine's precedent pool (rulings dated strictly before each case), the dev and held-out test sets, the router and precedent-check evaluations, and HTS subheading suggestions (`backend/data/hts_index.json`).
- **Should not be used for:** origin questions outside U.S. CBP practice; as a statement of current CBP policy (rulings apply to their own facts and can be modified or revoked); or to infer anything about the named companies.

## 7. Distribution and licence

- CBP rulings are works of the U.S. government and are in the **public domain** in the United States (17 U.S.C. § 105).
- The labels, splits and facts files are in this repository. Raw ruling texts are re-downloaded from CBP, not redistributed.

## 8. Maintenance

- **Who maintains it.** The Unde Labs team, in this repository.
- **Updates.** New rulings are not added automatically. The Ruling watch feature re-checks saved cases against rulings CBP issued later; adding them to the labelled corpus is a manual step (`go run ./cmd/corpus fetch`, `build`, then `make bundle-corpus`).
- **Revocations (D320, built).** A revoked or modified ruling stays in the corpus with its original holding, but it is no longer cited for a case dated on or after the change. `go run ./cmd/corpus supersession` reads every ruling's text (the EFFECT ON OTHER RULINGS section and any "is hereby revoked/modified" sentence) and subject line, and writes `backend/internal/domain/supersession.json`: 35 edges, 34 revoked or modified rulings, 13 of them in a precedent pool (3 general, 10 Part 102). The precedent filter, the move verifier, the case agent's search and the prediction cache all respect it, by date: a ruling revoked after the case date was still CBP's position then and stays usable, so backtests are unchanged. The API's ruling search and lookup (`GET /v1/rulings`, `GET /v1/rulings/{id}`) and the MCP tools on them return `superseded` ("revoked by H… on …"), and the verifier's and case agent's own searches mark such a ruling "do not cite". On the website every ruling link adds "superseded by H…, date", and a superseded ruling cited as support for a case dated on or after the change is marked "not counted as support" (D343); the site has no ruling-search page of its own.
- **What the map misses.** It only knows revocations whose letter is in this corpus, or that a later ruling in it mentions. CBP also revokes rulings by notices in the Customs Bulletin and by letters about classification only, which are not fetched. A ruling found through live CROSS search is checked against the same map, so a revocation outside the corpus is not caught. The date used is the issue date of the revoking letter; CBP's change takes effect about 60 days after publication.
- **Errors.** Report a wrong label as an issue on the repository, with the ruling number and the holding sentence.
