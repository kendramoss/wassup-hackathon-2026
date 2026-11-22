# Judge panel round 17 (read-only review), 2026-11-23

This is a read-only review of the product UI/copy as shipped in this repo (main branch context at review time). No production traffic changes. Scope: main pages at **1280px** and **390px** (desktop + phone), plus coverage of **new markets**: Canada rules check, RCEP calculator, market picker, Japan notes in copy/docs.

## Method (what I did)

- Code/copy review of: landing (`frontend/src/components/op/landing/*`, `frontend/src/lib/copy.ts`), `/cases` shell/list flows, `/accuracy`, `/method`, `/developers` page + sections, product/case intake (`product-step`, `market-picker`, `origin-step`, RCEP/Canada components), and relevant docs (`docs/scope-and-jurisdiction.md` mentioned indirectly via copy).
- Reasoned viewport behavior from responsive classes in components (no live browser automation run in this session). Findings call out likely 1280 vs 390 issues where layout patterns differ.
- Scored **verbosity** and **jargon** for a **first-time visitor** (1–5, 5 = best). Also technical/impact/innovation/UX from three personas (1–5).

## Pages walked (by route/component)

| Route / area | Primary files (repo) |
|---|---|
| `/` landing | `components/op/landing/landing.tsx`, `landing.css`, `copy.ts` (`landing.*`) |
| `/cases`, new case, case tabs | `cases/*`, `screens/product-step.tsx`, `market-picker.tsx` |
| `/accuracy` | `screens/accuracy-page.tsx`, `copy.ts` (`accuracy.*`) |
| `/method` | `screens/method-page.tsx` |
| `/developers` | `app/developers/page.tsx` (+ related copy) |
| Market features (in case flow) | `rcep-check.tsx`, `canada-check.tsx`, `origin-step.tsx`, `copy.ts` (`market.*`, `rcep.*`, `canadaCheck.*`) |

## Scores (three personas)

### Persona A — Technical judge (ML/engineering)
- **Technical:** 4/5 — Strong grounding (calibration steps, eval links, clear “not a ruling”). Some advanced terms still appear in method/accuracy deep sections.
- **Impact:** 4/5 — Concrete demos and numbers story is compelling.
- **Innovation:** 4/5 — Precedent + plan framing is differentiated; market checks add breadth carefully.
- **UX:** 3/5 — Dense long-form pages (/method, /accuracy) can overwhelm; navigation is clear but scanning is work.
- **Verbosity (first-time):** 3/5 — Improved in spots (RCEP/disclaimer tightened), but long explanatory blocks remain.
- **Jargon (first-time):** 3/5 — “calibrated confidence”, “tariff-shift”, “Part 102”, “precedent check” appear early without always being immediately plain.

### Persona B — Business judge (trade/compliance buyer)
- **Technical:** 3/5 — Understands “rules check vs model” in principle but wants faster “what do I do?” cues.
- **Impact:** 4/5 — Cost/savings language and demos are persuasive when found.
- **Innovation:** 4/5 — RCEP + Canada rules checks alongside origin story are useful if labeled honestly (they are).
- **UX:** 3/5 — Market picker and “U.S. only for now” messaging exist but can be missed; case flow has many steps.
- **Verbosity (first-time):** 3/5 — Landing is relatively focused; method/accuracy still long.
- **Jargon (first-time):** 2/5 — “RCEP”, “FOB/HS code” expectations, “marking rules”, “jurisdiction” appear without short primers in some places.

### Persona C — Design/UX judge
- **Technical:** 3/5 — Copy is mostly consistent; some mixed punctuation/encoding artifacts were seen historically (watch for mojibake in edited strings).
- **Impact:** 4/5 — Visual hierarchy on landing is strong.
- **Innovation:** 3/5 — Feature breadth is good; presentation could better surface “new markets” value.
- **UX:** 3/5 — Mobile (390px): long tables/folds need care; ensure tap targets and that critical actions aren’t buried. Desktop (1280px): wide margins are fine but side-by-side comparisons can feel text-heavy.
- **Verbosity (first-time):** 3/5 — Repeats “not a ruling / illustrative” in many places (necessary legally, but could be consolidated visually).
- **Jargon (first-time):** 3/5 — Inconsistent: some areas plain (“Check RCEP”), others dense (“precedent-supported at most”).

## New markets coverage (required)

### Canada rules check
- **Where:** Case/origin flow shows a Canada-specific path when jurisdiction is Canada (`origin-step.tsx`: `CanadaMarket`, `CanadaCheck` auto). Copy in `copy.canadaCheck` (title, rules-check-only label, states).
- **Strengths:** Clear “rules check only”, no accuracy score claimed. States are understandable.
- **Gaps (first-time):** “Marking rules (SOR/94-23)” is mentioned in comments/copy context elsewhere but the UI label could briefly say “country-of-origin marking for Canada” in plain text on first sight. Needs HS/product+part codes—error/help copy is short but could be more action-oriented (“Add HS codes to both the product and at least one part”).

### RCEP calculator/check
- **Where:** `rcep-check.tsx` in origin step; copy in `copy.rcep`. Asks made-in, FOB, per-part originating/value inputs.
- **Strengths:** Honest framing (“not reviewed by customs experts”, “May qualify / Doesn’t qualify / Need more info”). Separate from origin answer.
- **Gaps:** “RCEP” not spelled out on first use in UI strings seen (Relating to Regional Comprehensive Economic Partnership) — first-time users may not know the acronym. FOB and “regional value content 40%” appear in notes; could add a one-line “plain English” summary above the form. Mobile: table layout for parts may be cramped at 390px (needs manual verification).

### Market picker
- **Where:** `product-step.tsx` uses `MarketPicker`; `copy.market` has labels/help/unsupported messaging.
- **Strengths:** “Market” label, unsupported states point users to U.S. flow.
- **Gaps:** Default/behavior when untouched should be obvious; “Pick the market when you start” appears in `noDetector`—good, but ensure the picker is prominent and the choice is summarized in case header/tabs.

### Japan
- **Where:** Mentioned in repo/docs context (issues #661/#682/#666) and may appear in roadmap/scope text; not fully surfaced as an end-user “market mode” in the same way as Canada in the reviewed UI paths except through copy/support language in places.
- **Gaps:** If Japan is “coming soon” in UI, label it consistently (and avoid implying parity with US/Canada/RCEP tooling). Ensure `/method` or `/accuracy` states Japan status plainly if referenced.

## Verbosity & jargon notes (first-time visitor)

- **Repeated disclaimers:** “not a ruling”, “illustrative”, “rules check only” appear many times. Consolidate into a short global note pattern where possible; keep critical ones at decision points.
- **Jargon hotspots:** “precedent check”, “calibrated confidence”, “tariff-shift”, “Part 102”, “substantial transformation”, “HTS”, “FOB”, “RVC 40%”, “SOR/94-23”. Recommend short tooltips or one glossary link per page section, not walls of text.
- **Positive:** RCEP/Canada copy was recently tightened in `copy.ts` (this session) toward plainer verbs (“May qualify”, “Couldn’t check”).

## Top 5 fixable items (turn into issues)

1. **(P2, UX/copy)** Add a plain-language explainer for **RCEP** on first use in the RCEP panel (“trade deal between … countries”) and a one-sentence “what you need” (product HS code, where made, part origins/values). *Affects: `rcep-check.tsx`, `copy.rcep`.*
2. **(P2, UX)** **Canada marking rules**: replace/augment label with “Country-of-origin marking for goods sold in Canada (rules check)” and make missing-codes message a clear checklist. *`copy.canadaCheck`, `canada-check.tsx`.*
3. **(P2, Responsive)** Verify and fix **RCEP parts table** at **390px** (stack rows or horizontal scroll with affordance; large tap targets for checkboxes). *`rcep-check.tsx` + CSS.*
4. **(P2, Copy consistency)** **Japan messaging**: if mentioned, use one short line everywhere (“Japan: not available yet” or link to status) and avoid implying live checks. *Audit `copy.ts`, method/accuracy/docs links.*
5. **(P3, IA)** Add a single **“Markets” / “What’s supported”** micro-summary near market picker and in case header (US live, Canada rules check, RCEP calculator, others not yet) to reduce confusion. *`market-picker.tsx`, `origin-step.tsx`, `copy.market`.*

## Suggested issue titles (if filing)

- “RCEP panel: add plain-English acronym + inputs summary for first-time users”
- “Canada marking check: plain label + actionable missing-data guidance”
- “RCEP check mobile layout: parts table usable at 390px”
- “Japan: consistent ‘not yet’ copy across UI and method/accuracy”
- “Case flow: show supported markets summary next to picker”

## Notes

- This document is the Round 17 artifact for #684. Filing the top 5 as GitHub issues is a follow-up action (requires `gh` issue create); I did not file them in this run unless you ask me to.
- Re-run a quick manual pass at 1280/390 in browser after any copy/layout PRs (especially RCEP table and market picker visibility).
