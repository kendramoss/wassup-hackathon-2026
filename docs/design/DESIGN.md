# Origin Proof — frontend design decisions

Built step by step (process from Figma "Design systems 102", Disher's front-end design process and Shakuro's frontend architecture guide, scaled to a 2-day build):

1. Goals and principles — **decided**
2. User flow and states — **decided**
3. Foundations (colour, type, spacing, elevation, icons, accessibility) — **decided**
4. Components and layouts (prototype the key screen first, 10-second test) — next
5. Architecture and build (feature-organised Next.js app, typed API client)

## 1. Goals and principles (decided 15 Oct)

**Audience:** both — judges first (2:45 video, live finals Q&A), the real user underneath (trade-compliance lead at a China-plus-one contract manufacturer).

**Goal:** a judge understands each screen in 10 seconds; a compliance lead finds enough evidence to trust it and send the plan to a broker.

**Principles**
1. **Headline first, evidence one click away.** Each screen leads with one big answer (the origin, the best plan, 89.1%). Rulings, reasoning and full tables sit directly under it or in a side panel.
2. **Confident where measured, gated where not.** Lead with what we measured (overall accuracy and accuracy on auto-accepted cases, as in `docs/NUMBERS.md`; the auto-accept bar is 0.92 since D32). Present uncertainty as the safety system — "auto" vs "sent to an expert", "precedent-supported" with a counter-ruling — the way winning entries framed their human approval gates. Caveats sit one click away, never on the headline, and we never claim more than we measured.
3. **Document, not dashboard.** Calm, readable, printable, like a CBP ruling letter. No decorative charts.
4. **One decision per screen.** Input: describe the product. Result: is this the origin? Plan: which change? Results: can I trust it?

**Success checks**
- 10-second test: someone new can say the origin and the recommended change after 10 seconds on each screen.
- Every on-screen number traces to `docs/NUMBERS.md`.

**Background for principle 2:** past Google hackathon winners (Gemini 3 Hackathon: Globot, Aegis; Cloud Run Hackathon) presented confident self-reported numbers and did not foreground limitations, but repeatedly won with human-approval gates and audit trails (`hackathon-research/evidence/idea-finder-mining.json`).

## 2. User flow and states (decided 15 Oct)

Key frames: `docs/design/keyframes/` (gallery `index.html`, run with the `design-keyframes` launch config). Colour and type in them are placeholders until step 3. Heat-gun content is the recorded response in `eval/results/predict-heatgun-2022-litellm.json` (proxy run).

**Layout: separate steps with a progress bar** (option b; one growing page and a tabbed case page were rejected).
- Four steps per case: Product → Origin → Plan → Draft for broker. One decision per screen; the next step sits in a fixed footer.
- The verdict owns the left column; evidence (key factors, earlier rulings, closest ruling) sits in a right-hand panel.
- **Cases** in the top bar is home: one row per case from `GET /v1/cases` (product, likely origin, status, where you left off; newest first) plus demo cards from `GET /v1/demos`. "+ New case" starts step 1.
- The case name in the step bar opens a switcher (other cases, All cases, + New case); each case reopens on its own `step`.
- "How accurate is it?" (Results) is reachable from the top bar at any time.

**Status labels** (from `triage.route`): auto = "Auto-accepted", needs_review = "Needs expert review", needs_info = "Needs more facts", none yet = "Draft".

**Result states (step 2)**
- Auto-accepted: verdict and evidence only.
- Follow-up questions: `clarify.questions` (up to 3) can come back on any case below the fast path, `needs_review` as well as `needs_info` (the heat gun is `needs_review` with 3 questions). Shown as returned under "CBP would likely ask about this", with "Add answers and re-run" (`POST /v1/cases/{id}/answers`). "Plan anyway" stays available.
- Needs more facts (`needs_info`): no earlier ruling on point; same question box.
- Each question has an "I don't know" option. "Unknown" answers are kept on the case but left out of the re-analysis; if no answer adds a fact, `/answers` returns 400, so the button stays disabled until at least one real answer is typed. Answers are stored and shown exactly as typed; the backend strips hedge sentences ("…is not stated") only for the re-analysis (answers-v2), so the UI never edits them.
- Earlier rulings disagree (`clarify.conflict`; locked 16 Oct, frame `p-res-conflict.html`, option 1 of `c-res.html`): an amber-labelled panel above the questions. The two rulings as halves (subject, ruling number, "held" + flag + country, holding as returned); the one named by `conflict.leans` ("a"/"b", requested from the backend; no marker when null) is tinted blue with "this case follows"; "What CBP relied on" shows `deciding_difference` as returned. Only endpoint text; no labels of ours. The questions follow.
- Needs expert review without questions (`p-res-review.html`, heat pump N351860, China 0.90): an amber panel "Below the 0.92 bar · worth a broker's look" with the held-out rates (68% under the bar, 96% over), the key factor, "Draft a note for the broker" and "Add facts"; the reasoning as returned below; "Plan a move" stays available.
- Out of scope (`p-res-scope.html`, knit gaiters N357543): detected by `triage.reason` starting "out of scope: " (regimes textile_102_21, usmca_part102, pharma_api; "unclear" is in scope; router absent = in scope). Predict returns a normal case (D7). The band leads with the rule set ("Textile rules") and the reason, the prediction is subordinate ("Likely China, 0.99, not auto-accepted") and repeated "for reference" with its reasoning; no questions; plan search off with the reason; primary action "Draft for a broker". Never show answer-key facts on screen.
- Model error (`p-res-error.html`): the band stays a Draft with placeholders; a red panel names the code, says nothing is lost and offers "Try again" (model_unavailable, model_bad_response) with the try count; internal shows the API message and the case ID. Plan search stays off.
- Copy around the questions makes no accuracy claim ("CBP would likely ask about this"); the measured effect of the questions isn't final.

**Plan states (step 3):** searching (~30 s), plans shown, a greyed below-bar row, no plan reached, plan unavailable for facts-only cases.

**Results page:** Vertex AI numbers are official (backend `feat/origin-proof-vertex-eval` 39eda27; source `docs/NUMBERS.md`; `GET /v1/eval/summary` prefers them). Held-out, 377 rulings: 90.2% (86.8–92.8); the fast path (confidence ≥ 0.92, in scope) covers 79.6% of cases at 96.0% (93.1–97.7). Dev, 463 rulings: 91.1% (88.2–93.4); fast path 79.5% at 96.5% (94.1–97.9). Per case $0.0078 promo / $0.0157 list; latency p50 4.6 s, p95 5.7 s. No significant difference from the proxy run (held-out 89.9% → 90.2%, McNemar p = 1.0). Lead with held-out; always show the interval; no "proxy" label. Locked 16 Oct (frame `p-accuracy.html`, concepts `c-acc.html`): the intro says what was tested ("377 real CBP origin rulings from January to September 2026, none of which it had seen"); hero is every held-out ruling as a square (auto-accepted right 288 / wrong 12, sent to review right 52 / wrong 25, worked out from the rates) beside cards for overall 90.2%, auto-accepted 96.0% on 79.6% (the 0.92 bar) and review 77 at 68%; then bars against rules of thumb with intervals (most-mentioned 60.5%, final assembly 40.6%, last-mentioned 39.8%, majority 12.2%); cost, time and the proxy comparison; "What we don't claim" (change detection 68% on 22 pairs, weak; move checks on a development set; not a ruling); and "Every number", a table with interval, n, status tag (official, baseline, weak, development set) and source file.

**Rules for the build**
- Show endpoint text as returned. Real lengths: questions median 166 characters (max 445), "what decided it" median 324 (dev clarify run).
- Step 1 always sends a product name, also on "Paste a description", so the Cases list never shows truncated facts.
- Cases persist on local disk (`CASES_DIR`); on Cloud Run they are lost on restart until Firestore (D25).

## 3. Foundations (decided 15 Oct; revised 15 Oct to paper mono)

Key frames: `docs/design/keyframes/p.html` (P next to the earlier choice F), `step3.html` (the first six directions). Tokens and font: `docs/design/keyframes/_shared/theme-p.css`.

**Direction P, paper mono** (after [Paper Mono's site](https://paper.design/mono)). It replaced F (dark reading room) because UI/UX is judged: P is the most distinctive direction and reads as engineering paper, which suits manufacturing. F is kept in the key frames for reference only.

**Type:** Paper Mono (SIL OFL 1.1, by Paper, based on Vercel's Geist Mono; v1.0, 7 Oct 2026) for everything, bundled as WOFF2 (Light 300, Regular 400, Medium 500, SemiBold 600). Long text (questions, reasoning, the decisive step, the broker letter, form text areas) uses the duospace set (`font-feature-settings: "ss02"`), which the font provides for reading. Labels, numbers, dates and ruling numbers use strict mono so they line up. Scale: verdict 84 (weight 400, tracking −0.06em), page title 28, section 18 (500), lead 17, body 14, decisive step 15 (two lines at most, then clipped), label 12 caps (500, +0.06em), dimension labels 10.

**Colour** (contrast on paper; all text passes WCAG AA, also over the grid lines)

| Token | Hex | Use | Contrast |
|---|---|---|---|
| paper | `#f3f3ee` | page, content column | — |
| sheet | `#f9f9f5` | panels | — |
| wash | `#ebebe4` | hover, disabled | — |
| grid | `#dde4f3` | graph grid, margins only | — |
| line | `#cfd3dc` | hairlines | — |
| panel edge | `#b9c6e6` | panel borders | — |
| shadow | `#a8bcef` | hard offset shadows | — |
| ink | `#232320` | text, primary button | 14.2:1 |
| dim | `#5c5b55` | secondary text | 6.1:1 |
| faint | `#7f7d76` | placeholders only | 3.7:1 |
| ok (blue) | `#3a5fc4` | supported, cited, links, done steps | 5.2:1 (4.6 on grid) |
| ask (amber) | `#965606` | needs review, questions | 5.2:1 (4.5 on grid) |

Blue is the only accent: supported, cited, links, done steps and the "Auto-accepted" status. No green anywhere. Amber is for "look again"; red only for real failures. (A combination with Tensorlake's dark terminal look and green accent, `pt.html`, was tried and dropped.)

**Drawing-sheet details**
- The graph grid (20 px) shows only outside the content column; the column, panels and the letter are solid paper, so text never sits on grid lines.
- Panels are spec dialogs: 1 px blue-grey edge, 2 px corners, a hard 4 px blue offset shadow (amber for the question box). Primary buttons get a 3 px offset shadow; ghost buttons none.
- Dashed blue guides frame the content column and the verdict band, with small dimension labels such as "fig.1 — likely origin" and "route: needs_review".
- Confidence is a ruler (0, .5, 1) with a marker at the value and a dashed blue line at the bar: 0.92 everywhere: auto-accept (`triage`) on Origin and Scenarios, the precedent check on Plan (both 0.92 since 15 Oct).
- Flags keep their rules (see step 4), with a dark 1 px ring instead of a light one.

**Spacing and shape:** 4 px base (4, 8, 12, 16, 24, 32, 48, 64); 2 px corners everywhere.

**Broker draft:** the letter is a light page with a hard shadow on the grid, set in Paper Mono with duospace, section headings in caps. It prints as shown; no separate light view is needed now. Layout as decided: form left, live letter right.

**Video:** P is a light theme, so contrast in the recording comes from the hard shadows, the ruler and the large verdict.

## 4. Components and layouts (in progress)

Key frames: `docs/design/keyframes/step4.html` (Result screen compositions) and `test10.html` (10-second test).

**Result screen: verdict band (k2, decided 15 Oct; frame `step4-k2.html`).** A full-width band carries the whole answer in one row: flag and origin, then the decisive step with the check against CBP's ruling on the current setup, then status and the confidence meter (0.92 auto-accept tick, D32). Below it, two columns: the follow-up questions left, key factors, earlier rulings and the audit line right. The decisive step is limited to two lines. "Plan a move to {target} →" is always enabled ("Answering is optional").

**Flags.** Every country name gets its flag: 58 px tall next to the verdict, 11–12 px inline (rulings list, buttons, Cases list). Source: `flag-icons` (MIT, 4:3 SVGs, bundled, no CDN at runtime); 2–6 px corners and a 1 px light ring so dark-red flags hold their edge on the dark page. Flags are decorative (`alt=""`): the name is always written next to them. The API returns country names, so the frontend maps name → ISO code; "Indonesia/Malaysia" shows both flags; an unknown name or a null origin shows no flag.

**Earlier-rulings list** reads `precedents_detail` (same order as `prediction.precedents`; cited ones first and marked "cited"). Each row: short subject · date · "held {origin}", plus the ruling number linking to `url`.
- `origin` null → leave out "held …". Empty `date`/`subject` (ruling not in the pool) → show the number only.
- Short subject, from CBP's full subject line (full line in a tooltip): take the part before the first ";"; take the text after "origin [determination] of|for|on", or else after the last "for|on|of a/an/the"; drop a leading article or count word ("a", "the", "certain", "five"); drop a trailing "from {Country}" and full stop; capitalise; cut at 48 characters on a word boundary with "…". Checked on all 2,790 corpus subjects (longest result 48 characters), e.g. "The tariff classification and country of origin of a plastic scrub sponge" → "Plastic scrub sponge".

**Plan screen: band, diagram, then table (q1, decided 15 Oct; frame `docs/design/keyframes/p-plan-q1.html`).**
- Band: "Best plan", the move as a sentence with the target flag, chips "Precedent-supported" and "Reaches {target} · {confidence}", net per year (illustrative, with the duty-saved and added-cost breakdown), and the precedent check as a ruler against the 0.92 bar.
- fig.2 operations by plant: one column per plant (from the product), operation groups from the planner template, moved groups in blue with "moved" and dashed arrows.
- Plan bar: a plan is shown when its precedent check is at least 0.92 (the same bar as auto-accept; was 0.95). Backend done: 7ca5ad8, D35. The recorded `.sse` predates it, so it still marks the 0.92 move below the bar; live and cached runs show it.
- Precedent check panel: supporting ruling, counter-ruling ("distinguish it"), verifier reasoning, "Read the full check".
- Table of every searched move, best first: shown plan, then greyed rows by `rejection_kind` (below bar, no precedent, other origin, current setup), each with its reason, supports/counter, a mini ruler against 0.92 and net per year. Moves that save more but sit below the bar stay visible and greyed.
- fig.3 backtest (demos with `backtest` only): CBP's actual ruling and which actually-moved parts the plan covers.
- Footer: back, search stats (moves, calls, seconds), "Inputs are illustrative · edit costs and duty", "Draft the ruling request →".
- Searching state (decided 15 Oct; concept `docs/design/keyframes/p-plan-search-live.html`, option A; backend e68df83, D30; recording `eval/results/plan-stream-heatgun-2022-litellm.sse`): `POST /v1/cases/{id}/plan` with `Accept: text/event-stream`, read with fetch and a ReadableStream (EventSource can't POST); 400/404 arrive as plain JSON first; ": keep-alive" lines every 10 s. Events: `start` {target, moves[{key, description, move}], total} draws every row; `move_started` {key, done, total} fires for all moves at once (they run in parallel), so rows show "Checking precedent…"; `baseline` {plan} fills the current-setup row; `move_done` {key, plan, done, total} lands that row with its outcome tag and a short blue highlight and advances the done/total bar; `done` (the normal response body) re-sorts best first, swaps the band and enables "Draft the ruling request"; `error` {code, message, retryable} shows "Try again" only when retryable. Elapsed time beside "usually ~30 s" (heat gun: rows land 16–28 s).

**Plan screen when no plan clears the bar (decided 15 Oct; frame `docs/design/keyframes/p-plan-none.html`, from the recorded run, with the move that now clears 0.92 left out, so the closest is motor-fan assembly and switch, cord and minor parts at 0.90; illustrative of the state).** Amber eyebrow "No plan cleared the 0.92 bar"; the band names the closest move (highest precedent-check confidence among `below_confidence_bar` rows) with "Below the bar · {check}" and its net per year in grey ("if a broker confirms it"). Diagram and evidence show that move, marked below the bar. Every row is greyed, closest first. Footer: "Try scenarios" and "Draft for the closest move →". The draft opens with the backend's notice ("Below the confidence bar: … rated this change 0.90, under its 0.92 bar …"), shown as an amber banner at the top of the letter. Request: `POST /v1/cases/{id}/export` with `{"move_key": …}` (2e4a502, D31); other rejection kinds return 400, so the button only appears for `below_confidence_bar`.

**Recording the demo:** verdicts are cached (`VERIFIER_CACHE_DIR`), so repeat plan runs are identical but finish in about 1 s. Record the searching screen on the first, uncached run (or clear the cache first), then replay with the cache warm for the rest.

**Still to design (new backend features, d617903, D29)**
- Draft designs, not locked until the backend's higher-confidence work lands; re-render from fresh recordings then. Frames: `docs/design/keyframes/p-scen.html`, `p-watch.html` (gallery `p-extras.html`).
- Scenarios are in the demo flow (the "what if" moment: as today 0.75 needs review → motor-fan in Thailand 0.96 → motor-fan + PCBA 0.98, auto-accepted). Video path changed 16 Oct (backend `feat/origin-proof-demo` 4721005, D40, `make demo-seed`, runbook `docs/DEMO.md`): in the seeded run no heat-gun or LiDAR plan cleared 0.92, so the main path shows "No plan cleared the bar" with "Draft for the closest move" (`p-plan-none.html`), and Scenarios carries the "move X saves $Y" beat (motor-fan to Thailand 0.95, net $380k). Recorded outputs: `eval/results/demo-seed-litellm.json`.
- Scenarios (`POST /v1/cases/{id}/scenarios`, 1–3 labelled variants predicted side by side; the case's own prediction is unchanged). Likely a compare view off the Result screen.
- Ruling watch (`POST /v1/cases/{id}/watch`, `POST /v1/watch/run`; Cases rows gain `watch_status` and `scenario_count`). Likely a status column on Cases and a "re-checked with newer rulings" note on the Result screen, listing `newer_rulings`.
- The "same as CBP's ruling on this setup" check line must handle disagreement: the heat gun predicted from its structured product gives Thailand 0.75, while CBP held China (N328954); predicted from CBP's own 2022 text it gives China 0.85.

**More backend features to design (0594ba5, D33, D34)**
- BOM import on step 1 (decided 15 Oct; frames `docs/design/keyframes/p-bom-empty.html`, `p-bom-ok.html`, `p-bom-warn.html`, gallery `p-bom.html`): a third tab, "Import BOM", beside "Parts and plants" and "Paste a description". Empty: a drop zone (CSV or .xlsx, `POST /v1/products/import`) with the column guide (name and country required; plant, group, type, plant_capabilities optional) and "Download sample (heat gun)" (`backend/testdata/heatgun_bom.csv`). Imported: the file name with "replace file", counts (components, operations, plants, groups to move, the last worked out on the page from plant capabilities), a "Ready for the plan search" line, and one panel per plant listing its groups and "Could also take on". The import fills the parts-and-plants tables, so everything stays editable there; nothing is analysed until "Find the likely origin". Warnings show as an amber list, with the backend's wording, and never block continuing. A rejected file (400, row problems) shows the rows in red and imports nothing; red appears only here.
- Scenario money (designed 15 Oct, part of the scenarios draft; frame `docs/design/keyframes/p-scen.html`): a strip above the cards shows the inputs ("Money · illustrative — editable": imports a year, duty rates by country, costs per group, "Edit inputs"). Each card gets a "Net per year" block from `comparison`: the net, then "duty saved − added cost" and the duty at the scenario's rate. The scenario that moves nothing is the $0 baseline ("Nothing moves, so duty stays at China's 41.7%"), even if its predicted origin differs from CBP's (backend b23a83d: a product scenario that moves nothing reports $0 at the current origin's rate). When `comparison.origin_differs_from_current` is true, the card shows the amber "Differs from CBP" line. Facts-only scenarios have no product, so their money follows the predicted origin; when the flag is true there, label the net "if the predicted origin holds". Tags "highest net" and "highest confidence" can land on different cards (heat gun: motor-fan $380k vs motor-fan + PCBA $320k at 0.98). The part matrix ends with a net-per-year row. Null fields show "—" with "add duty rates or costs".
- Majority-of-3 precedent check (off by default): when `verdict.agreement` is present ("2/3"), show it beside the check confidence; absent by default.

**Case history (D41, backend `feat/origin-proof-demo` 4721005; not framed yet)**
- Every case response has `history: [{at, kind, summary, llm_provider, model_id, prompt_version}]`, oldest first; `kind` is predicted, answered, plan_searched, scenarios, targets_compared, watched, asked or exported. The Result screen's audit line shows the latest entry; a small timeline (case switcher or a "History" link from the audit line) lists all of them, `summary` shown as returned.
- BOM import also accepts `type: plant` rows for plants with no parts yet (sample `backend/testdata/lidar_bom.csv`); they appear as plant panels with only "Could also take on".

**Step 1, Product (locked 16 Oct, B + C after an Altana comparison; frames `p-product-empty.html`, `p-product-parts.html`, `p-product-paste.html`, `p-product-extract.html`, the three `p-bom-*` frames, gallery `p-product.html`, concepts `c-step1.html`)**
- Three tabs: Parts and plants, Paste a description, Import BOM. Product name, family and case date sit above all three.
- An "unlocks" strip under them on every tab and state, computed on the page: **Likely origin** (a name plus one component, or pasted text), **Plan search** (two plants, and at least one capability beyond what a plant makes now), **Money per move** (imports a year, duty rates, a cost per group). Done tiers are blue with a tick, missing ones dashed with what they need and a link to the fix, limited ones amber with "!" and the reason (BOM with one plant). Counts (components, operations, plants, groups to move) sit in a line under the strip.
- Parts and plants: plants table (name with flag, country, "can make" chips: ink = makes now, blue = could also take on), components (name, made in, plant, group), operations (text, plant, group). Cells edit in place; "+ Add" under each table.
- Paste: the text the engine reads (max 12,000 characters, counted), a "What helps" guide, and on the plan tier "Turn into parts and plants". That drafts the tables from the text (`POST /v1/products/extract {facts, family?, name?}`, backend `feat/origin-proof-demo` 10618bc, D42; nothing saved; ~15 s, so a waiting state, `p-product-extract-wait.html`, with the text still readable and placeholder tables): the source text on the left with the row's sentence highlighted, drafted plants, components and operations on the right, each row citing its paragraph; guessed fields in amber (`name_guessed`, `group_guessed`); each of `questions` as chips to tick (what else a plant could make), which fill the plant's capabilities; `warnings` as returned in a "Check these" box, first two shown and the rest folded ("1 more"). Heat gun: 15 parts, 5 operations, "The text says 24 components; it names 15." "Keep as parts and plants" or "Discard"; nothing is saved before. Predict then sends both `facts` and `product`. The source panel shows paragraphs split on blank lines, numbered to match `source_paragraph`.
- Money inputs (locked 16 Oct, option C of `c-money.html`; frames `p-money-quick.html`, `p-money-each.html`): "+ Add money inputs" opens a 600 px drawer, "illustrative — editable". Imports a year; a duty rate per country already in the product, in % with its source note (sent as fractions, 41.7% → 0.417); then cost per group a year, opening on **Quick estimate** (one China total and "Rayong costs more by N%", with a slider, filling every group in proportion and saying the result is rough) with **Each group** for the full group × plant table and the extra cost of each move worked out on the page (heat gun: motor-fan +$120k, PCBA +$60k, switch and cord +$35k). Saved with `economics` on predict, or `PUT /v1/cases/{id}/economics` once the case exists (no model call; adds an "economics" history entry; backend 9b1af86, D43). 400s (rate outside 0–1, negative amount) show under the field. The same drawer opens wherever money is edited: Scenarios, Plan, compare targets.
- Altana (docs only, no screenshots) lists Good / Better / Best data per result in its help pages and reviews AI-extracted data before saving; we show the tiers live and cite the source of every drafted row.

**Backend D45 (`feat/origin-proof-demo` 43315ef; locked 16 Oct, concepts `c-d45.html`)**
- Rate limits: model-calling endpoints may return 429 `rate_limited` with `Retry-After` and `details.retry_after_s` (20/min for predict, answers, ask, extract, scenarios, watch; 4/min for plan and compare-targets). Show a calm inline note where the action was, not the red error panel: "Too many requests. Try again in 30 s", counting down, the button re-enabling at zero. Not a failure; nothing is lost.
- Pinned demos: `pinned` on Cases rows and the case. Pinned cases list first on Cases home under a "Demos" group, each with a small "Demo" tag. `DELETE /v1/cases/{id}` (204) is offered from a row menu for unpinned cases only; pinned ones return 403, so the menu item is absent. No pin UI (admin only).
- "Re-check all" (`POST /v1/watch/run`) is admin-only now (403): remove it from Cases home. Per-case watch on the Result screen is unchanged.
- Cost per step: history events may carry `input_tokens`, `output_tokens`, `cost_usd`, `latency_ms` (absent when no model ran). The case history list shows them muted per step ("$0.019 · 13.6 s"); the audit line can sum the case so far.
- Locked: Cases home option 2 (`p-cases.html`, `p-cases-edit.html`): pinned demos as four cards (origin with flag, status, confidence, CBP's holding, the plan result), then "Your cases" as a table with a ⋯ menu (Open, Rename, Delete case…); Rename edits the name inline (Enter saves, Esc cancels; `PATCH /v1/cases/{id}` with `{display_name}`, 1–200 characters, backend a47abc0, D48). Cases home and the step bar show `display_name` (falls back to `product_name`, which stays as the engine read it); renaming adds a "renamed" history event and doesn't reorder the list, Delete asks in an inline red confirm row. Demos have no menu. Rate-limit note as drawn (`p-rate-plan.html`, `p-rate-extract.html`). Case history option 1 (`p-history.html`): a drawer from the audit line, one row per event with time, kind, summary, model, cost · time and tokens, "no model" where none ran, and the case total at the bottom.

**Demo seed runs (16 Oct; backend 457ba8e, `docs/DEMO.md` section 4). Both outcomes are designed; the recording-day Vertex run decides which the video shows.**
- Result: run 1 and run 2 route `needs_info` ("Needs more facts", `p-k2-info.html`: run 2 adds a conflict between N346397 air fryer, held Vietnam, and N351782 fans, held Thailand, with `leans` null so no marker, then the three questions as returned); an earlier single predict gave `needs_review` (`p-k2.html`). Both are valid at 0.85: the on-point judgement isn't stable on borderline cases.
- **Superseded 17 Oct by verifier-v2 (D74), production run:** no heat-gun plan clears 0.92. The closest move is the motor-fan assembly to Thailand, check 0.90 (under the bar), N325810 for, no counter-ruling, $380k/yr, so the Plan screen is the no-plan state (`p-plan-none.html`) with "Draft for the closest move". The landing's fig.1 moves only the motor-fan assembly and labels it the closest move. The run-2 notes below are history.
- Plan: run 2 shows motor-fan + PCBA at check 0.92, exactly the bar (`p-plan-q1.html`, re-rendered from `eval/results/demo-seed-heatgun-plan-litellm.json`; one supporting ruling N350304, no counter-ruling returned; say "exactly at the bar", never a confident pass). Run 1 cleared nothing (closest PCBA to Thailand 0.75) → `p-plan-none.html` (its table is from an earlier proxy run; re-render if the recording-day run lands there).
- Scenarios (`p-scen.html`, run 2): as today China 0.65, same as CBP; motor-fan in Thailand 0.96, $380k; motor-fan + PCBA 0.98, $320k.
- Conflict `leans` is live (clarify-v2, D46, ce320a9): the marker shows only for "a" or "b".

**Portfolio triage (backend D47, `feat/origin-proof-demo` f011159; locked 16 Oct, option 1 of `c-batch.html`; frames `p-batch-upload.html`, `p-batch-running.html`, `p-batch.html`)**
- `POST /v1/cases/batch`, up to 20 products as JSON `{items: [{name, facts, date?}]}` or a CSV/xlsx upload (name, facts, date). Returns `rows[]` (name, case_id, predicted_origin, confidence, triage_route, out_of_scope, reason, cost_usd, error) and `summary` (total, auto, needs_review, needs_info, out_of_scope, failed, cost_usd, latency_ms). Each row is a normal case. ~16 s for 5 products, no stream: indeterminate progress. Sample `backend/testdata/portfolio_sample.csv`, output `eval/results/batch-portfolio-sample-litellm.json` (2 auto, 1 needs_info, 2 needs_review incl. one out-of-scope textile). "Triage a portfolio" is a ghost button beside "+ New case" on Cases home. Upload: the file's products with the first line of their facts and "Triage N products" with the last run's time and cost; the column guide below for another file. Running: indeterminate bar and elapsed seconds. Results: counts (products, auto-accepted, need a look, failed) with a bar split by route and, beside it, the held-out context ("On 377 held-out rulings, 79.6% were auto-accepted") so a small sample isn't read as the fast-path rate; then one table sorted by what to do next (needs more facts, expert review, out of scope from `out_of_scope`, then auto), each row with origin + confidence, status chip, `reason` as returned (minus the "out of scope: " prefix), cost and "Open case". Failed rows (`error`) show in red with the message. "Download results (CSV)" and "Triage another file".
- `GET /health?deep=1` is ops only; no UI.
