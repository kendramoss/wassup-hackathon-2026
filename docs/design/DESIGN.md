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
- Earlier rulings disagree (`clarify.conflict`): both rulings side by side with their holdings and "what decided it".
- Needs expert review, out of scope, and a model error with retry.
- Copy around the questions makes no accuracy claim ("CBP would likely ask about this"); the measured effect of the questions isn't final.

**Plan states (step 3):** searching (~30 s), plans shown, a greyed below-bar row, no plan reached, plan unavailable for facts-only cases.

**Results page:** dev numbers labelled "proxy" until the Vertex AI re-run.

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
- Scenarios are in the demo flow (the "what if" moment: as today 0.75 needs review → motor-fan in Thailand 0.96 → motor-fan + PCBA 0.98, auto-accepted). The "no plan cleared the bar" state stays out of the video's main path.
- Scenarios (`POST /v1/cases/{id}/scenarios`, 1–3 labelled variants predicted side by side; the case's own prediction is unchanged). Likely a compare view off the Result screen.
- Ruling watch (`POST /v1/cases/{id}/watch`, `POST /v1/watch/run`; Cases rows gain `watch_status` and `scenario_count`). Likely a status column on Cases and a "re-checked with newer rulings" note on the Result screen, listing `newer_rulings`.
- The "same as CBP's ruling on this setup" check line must handle disagreement: the heat gun predicted from its structured product gives Thailand 0.75, while CBP held China (N328954); predicted from CBP's own 2022 text it gives China 0.85.

**More backend features to design (0594ba5, D33, D34)**
- BOM import on step 1 (decided 15 Oct; frames `docs/design/keyframes/p-bom-empty.html`, `p-bom-ok.html`, `p-bom-warn.html`, gallery `p-bom.html`): a third tab, "Import BOM", beside "Parts and plants" and "Paste a description". Empty: a drop zone (CSV or .xlsx, `POST /v1/products/import`) with the column guide (name and country required; plant, group, type, plant_capabilities optional) and "Download sample (heat gun)" (`backend/testdata/heatgun_bom.csv`). Imported: the file name with "replace file", counts (components, operations, plants, groups to move, the last worked out on the page from plant capabilities), a "Ready for the plan search" line, and one panel per plant listing its groups and "Could also take on". The import fills the parts-and-plants tables, so everything stays editable there; nothing is analysed until "Find the likely origin". Warnings show as an amber list, with the backend's wording, and never block continuing. A rejected file (400, row problems) shows the rows in red and imports nothing; red appears only here.
- Scenario money (designed 15 Oct, part of the scenarios draft; frame `docs/design/keyframes/p-scen.html`): a strip above the cards shows the inputs ("Money · illustrative — editable": imports a year, duty rates by country, costs per group, "Edit inputs"). Each card gets a "Net per year" block from `comparison`: the net, then "duty saved − added cost" and the duty at the scenario's rate. The scenario that moves nothing is the $0 baseline ("Nothing moves, so duty stays at China's 41.7%"), even if its predicted origin differs from CBP's (backend b23a83d: a product scenario that moves nothing reports $0 at the current origin's rate). When `comparison.origin_differs_from_current` is true, the card shows the amber "Differs from CBP" line. Facts-only scenarios have no product, so their money follows the predicted origin; when the flag is true there, label the net "if the predicted origin holds". Tags "highest net" and "highest confidence" can land on different cards (heat gun: motor-fan $380k vs motor-fan + PCBA $320k at 0.98). The part matrix ends with a net-per-year row. Null fields show "—" with "add duty rates or costs".
- Majority-of-3 precedent check (off by default): when `verdict.agreement` is present ("2/3"), show it beside the check confidence; absent by default.

**Compare target countries and ask the case (D36; designed 15 Oct for #46; frames `p-compare.html`, `p-compare-searching.html`, `p-ask.html`, `p-ask-waiting.html`, gallery `p-agent.html`)**
- **Compare countries lives in the Plan step.** "Compare countries" in the Plan footer opens a view inside step 3 (the step bar stays on Plan). The country picker takes 2–4 countries from the product's plants, defaulting to every plant country except the current origin. `POST /v1/cases/{id}/compare-targets`. Ranked table, one row per country: rank, flag and country, status tag (Plan in blue; Below the bar in amber; No supported plan / No capable plant in grey), best move with "Reaches {origin} · {confidence} · {n} moves searched", a mini ruler of the check against the 0.92 bar, net per year (illustrative, with "saved − added"), and "Draft →". Rows without a move show "—".
- The band follows the Plan screen. With a `plan` row: "Best country", that row's move, and the check ruler. When no row is `plan` (the recorded and likely live state): amber eyebrow "No country cleared the 0.92 bar", the band names the top-ranked below-bar row (ranked by check confidence, then net, as the backend returns), and the net is grey ("if a broker confirms it"). The footer says it's "Weaker than the Thailand plan (0.92)" when the case has a shown single-country plan, and offers "Draft for {country} →" (`export {move_key, target}`). The draft carries the below-bar banner as on the Plan screen.
- Searching (~30–40 s, parallel plan searches, no streaming): band "Comparing {n} countries", elapsed seconds beside "usually ~30–40 s · results arrive together", an indeterminate bar, rows in "Searching moves to {country}…". Draft disabled; Cancel aborts the fetch.
- **Demo framing (decided):** compare targets stays out of the video's main path. Every check in the recording is 0.65, far below the single-country Thailand plan (0.92+), and it would weaken the plan story. At most a 2-second cutaway after the Thailand plan ("other countries? none clears the bar"), recorded with the cache on. Live finals: show it only if asked.
- **Ask the case is a drawer on the Result screen.** "Ask about this case" (ghost button, Result footer) opens a 540 px right-hand drawer over the evidence column. The verdict band stays visible and the main column dims. Not a tab: the evidence and the conversation are needed side by side less than the verdict and the conversation. `POST /v1/cases/{id}/ask` with `{question}`; "Clear conversation" sends `{reset: true}`. On open, the drawer shows `case.chat` (earlier turns collapsed to one line: question · "{n} rulings cited · show answer"; the newest in full).
- Reply layout, top to bottom: the question; what-if cards (`what_ifs`: "What if · {change}", flag and origin, ruler with the 0.92 bar, decisive step clipped at 2 lines, "Add as a scenario →", which posts the change as a facts scenario); the answer text in duospace with ruling numbers linked in blue; "Cited · rulings the agent read" rows from `cited_rulings` only (short subject · date · held {origin} when the ruling is in the pool, otherwise the number only). `dropped_rulings` are never shown. Last, the tool steps from `steps` in faint mono ("get_case → what_if("…") · 3 model calls · 20 s") as a quiet audit trail.
- Waiting (15–40 s): the question appears at once, with a dashed box "Reading the case and earlier rulings…", elapsed seconds, "usually 15–40 s", an indeterminate bar (a segment sliding across, never a fill from the left: the endpoint reports no progress) and the composer disabled. Cited-ruling dates use the same "27 Aug 2025" form as the Result screen. Errors use the model-error pattern (retry when `retryable`).
- Suggested questions under the log (fixed copy, not generated): "Why not {target}?", "What if the {largest Chinese group} were made in {target}?", "Which ruling matters most?". Disclaimer text from the reply stays in the answer as returned.
- Not in the video's main path either. A possible 5-second beat: the what-if card returning Thailand 0.98 on the Result screen.
