// Data for the /method page ("How it's built and tested"). Every figure here
// is a row of docs/NUMBERS.md (section "Method page (/method)", the
// evaluation table and the corpus row); the page fetches the rest from
// GET /v1/eval/summary. Decision links point at docs/DECISIONS.md headings
// on GitHub; scripts/method.test.mjs checks each anchor still matches a
// heading, so a renamed entry fails the tests instead of breaking a link.

export const REPO = "https://github.com/kendramoss/wassup-hackathon-2026";
export const blob = (path: string) => `${REPO}/blob/main/${path}`;

// GitHub's heading anchor: lower case, punctuation dropped, spaces to "-".
export const ghSlug = (heading: string) =>
  heading
    .toLowerCase()
    .replace(/[^\p{L}\p{N}\p{M}\s_-]/gu, "")
    .replace(/ /g, "-");

// Anchors of the decision entries the page links to.
export const DECISIONS: Record<string, string> = {
  D10: "d10--bm25-ported-exactly-including-idf-over-the-whole-corpus",
  D11: "d11--facts-splitter-port-is-byte-identical",
  D22: "d22--accuracy-options-tried-on-dev-none-adopted-16-oct",
  D32: "d32--auto-accept-bar-lowered-from-095-to-092-15-oct",
  D39: "d39--official-vertex-ai-numbers-for-the-frozen-engine-15-oct",
  D44: "d44--untuned-gemini-35-flash-on-dev-tuning-not-launched-15-oct",
  D51: "d51--textile-rules-19-cfr-10221-as-a-second-regime-off-until-evaluated-16-oct",
  D61: "d61--usmca--part-102-rules-as-a-third-regime-data-first-off-until-evaluated-17-oct",
  D62: "d62--pharmaceutical-active-ingredient-regime-off-until-evaluated-17-oct",
  D73: "d73--tuned-gemini-35-flash-as-a-fallback-when-argon-fails-16-oct",
  D74: "d74--verifier-v2-counter-ruling-search-and-evidence-tied-confidence-17-oct",
  D76: "d76--a-fixed-seed-on-every-model-call-17-oct",
  D84: "d84--three-group-moves-in-the-plan-search-18-oct",
  D86: "d86--precedent-check-calibration-on-the-accuracy-page-18-oct",
  D138: "d138--ten-more-planner-families-chosen-and-written-from-cbp-precedent-21-nov-review-follow-up",
  D150: "d150--a-second-held-out-split-from-rulings-published-after-the-first-one-21-nov",
  D151: "d151--an-openai-compatible-gateway-provider-for-offline-evaluation-only-21-nov",
  D152: "d152--model-sweep-and-engine-candidates-on-the-development-set-selection-rule-fixed-before-the-sweep-finished-21-nov",
  D153: "d153--pre-registered-candidate-engine-v1-on-gemini-4-argon-unchanged-plus-a-calibration-layer-21-nov",
  D154: "d154--the-candidate-on-the-second-held-out-split-scored-once-21-nov",
  D157: "d157--rules-v2-aimed-at-the-largest-dev-error-cluster-tried-after-d153-and-not-adopted-21-nov",
  D160: "d160--calibrated-confidence-in-the-product-the-map-and-the-auto-accept-bar-fixed-before-the-held-out-numbers-are-computed-21-nov",
  D161: "d161--calibrated-confidence-shipped-the-auto-accept-bar-is-090-on-it-21-nov",
  D163: "d163--a-second-stage-adjudicator-for-the-china-component-cluster-plan-and-pass-rule-written-before-any-run-21-nov",
  D164: "d164--the-china-cluster-adjudicator-did-not-pass-on-dev-no-held-out-split-scored-21-nov",
  D170: "d170--a-post-cutoff-flip-pair-test-for-the-precedent-check-and-the-plan-search-21-nov-review-follow-up",
  D171: "d171--per-family-plan-checks-on-development-and-train-rulings-21-nov-review-follow-up",
  D190: "d190--run-to-run-repeatability-of-the-engine-on-the-development-set-21-nov-review-follow-up",
  D192: "d192--a-post-cutoff-flip-pair-addendum-few-new-pairs-exist-21-nov-review-follow-up",
  D300: "d300--re-examining-the-092-plan-bar-on-committed-rows-rule-written-before-any-re-computation-21-nov-review-follow-up-judge-panel-round-4-item-6",
  D301: "d301--the-092-plan-bar-stays-the-dev-rule-picks-092-and-there-are-too-few-wrong-verdicts-to-place-a-lower-one-21-nov-review-follow-up-judge-panel-round-4-item-6",
  D210: "d210--post-cutoff-held-out-splits-for-the-textile-pharmaceutical-and-part-102-engines-rule-freeze-and-plan-written-before-any-run-21-nov-review-follow-up",
  D211: "d211--the-post-cutoff-regime-splits-scored-once-results-21-nov-review-follow-up",
  D230: "d230--router-its-own-splits-an-error-analysis-and-a-pass-rule-written-before-any-variant-is-run-21-nov-review-follow-up",
  D231: "d231--router-v3-fails-the-d230-rule-on-dev-router-v4-defined-before-its-run-21-nov-review-follow-up",
  D233: "d233--held-out-results-router-v4-becomes-the-production-router-the-routed-path-scored-21-nov-review-follow-up",
  D240: "d240--repeatability-on-the-held-out-splits-pre-registration-21-nov-review-follow-up",
  D241: "d241--repeatability-on-the-held-out-splits-results-21-nov-review-follow-up",
  D322: "d322--the-heat-gun-plan-run-again-three-times-results-21-nov-judge-panel-round-5-item-5",
  D330: "d330--the-heat-gun-template-splits-the-power-cord-knobs-and-covers-from-the-switch-and-wiring-22-nov-judge-panel-round-5-510-backlog",
  D332: "d332--the-heat-gun-plan-after-the-split-results-22-nov",
  D320: "d320--revoked-and-modified-rulings-are-not-cited-as-precedent-after-cbp-changes-them-21-nov-judge-panel-round-5-item-6",
  D340: "d340--pre-registration-rank-shown-plans-by-a-clear-margin-tier-then-net-label-the-smallest-supported-move-22-nov-judge-panel-round-6-item-1",
  D341: "d341--plans-rank-by-a-clear-margin-tier-then-net-the-dev-rule-picks-a-margin-of-001-22-nov-judge-panel-round-6-item-1",
  D342: "d342--a-template-group-split-later-keeps-its-former-name-for-a-product-made-before-the-split-22-nov-judge-panel-round-6-item-4",
  D343: "d343--revoked-and-modified-rulings-are-marked-wherever-the-site-shows-a-ruling-22-nov-judge-panel-round-6-item-2",
  D344: "d344--the-backtest-finds-kept-parts-by-component-name-when-a-demo-has-no-map-519-22-nov",
  D360: "d360--a-deterministic-19-cfr-10220-rule-check-beside-the-part-102-engine-22-nov-judge-panel-round-6-the-main-remaining-larger-build",
  D362: "d362--the-rule-check-on-the-committed-part-102-rulings-results-22-nov",
  D363: "d363--the-rule-check-is-shown-beside-the-model-display-only-22-nov",
  D370: "d370--a-revocation-counts-from-the-day-after-the-revoking-rulings-date-22-nov-527-judge-panel-round-7-item-1",
  D400: "d400--revocations-from-the-customs-bulletin-pre-registration-of-the-parse-22-nov-judge-panel-round-7-larger-build",
  D401: "d401--customs-bulletin-revocations-results-22-nov",
  D380: "d380--hts-codes-suggested-for-each-part-so-the-10220-rule-check-can-run-the-route-is-on-by-default-22-nov",
  D382: "d382--per-part-hts-suggestions-results-22-nov",
  D413: "d413--code-stripping-for-the-per-part-evaluation-the-leaked-inputs-reported-22-nov-576-post-freeze-correction",
  D430: "d430--the-rule-check-says-whether-its-answer-holds-under-every-suggested-part-code-22-nov-judge-panel-round-9-n2-n3-n4-n1",
  D441: "d441--each-plan-move-carries-the-10220-rule-check-display-only-22-nov-judge-panel-round-9-item-3",
  D480: "d480--revocation-alerts-on-saved-cases-22-nov-judge-panel-round-9",
  D442: "d442--the-rule-check-on-the-committed-plan-moves-results-22-nov",
  D460: "d460--pre-registration-ten-more-chapters-in-the-10220-rule-check-22-nov-judge-panel-round-9-item-4",
  D461: "d461--ten-more-chapters-in-the-rule-check-results-22-nov",
  D490: "d490--a-revoking-ruling-is-not-alerted-on-the-ruling-it-revokes-recount-of-d481-22-nov-judge-panel-round-10-n4",
  D500: "d500--pre-registration-on-a-part-102-case-rank-shown-plans-by-tier-then-the-10220-rule-check-then-net-22-nov-judge-panel-round-10-item-4",
  D501: "d501--pre-registration-three-scored-part-102-plan-searches-with-codes-22-nov-judge-panel-round-10-item-4",
  D560: "d560--the-rule-check-chapters-re-picked-without-held-out-data-five-d460-chapters-kept-and-the-held-out-figures-given-without-them-22-nov-619-judge-panel-round-12-item-2",
  D502: "d502--part-102-aware-plans-finished-the-check-in-the-draft-and-the-comparison-the-d500-order-and-the-three-d501-searches-22-nov-judge-panel-round-10-item-4",
};

export const decisionHref = (id: string) => `${blob("docs/DECISIONS.md")}#${DECISIONS[id]}`;

// The protocol in order, from the dates in docs/DECISIONS.md and the split
// files. Process facts: plain text, no figures (those are in the charts).
// period: a label shown once above the first step that carries it. The 21 Nov
// steps followed an external expert review that day (judge panel round 4:
// "Since 21 Nov" read as last-minute; the dates stay exact, the reason is said).
export type Step = { date: string; what: string; refs: string[]; files?: { label: string; path: string }[]; period?: string };
export const POST_REVIEW = "Post-review hardening after an external expert review (21 Nov 2026)";
export const TIMELINE: Step[] = [
  {
    date: "By 14 Oct 2026",
    what: "Corpus built from public CBP rulings. The facts section of each ruling is split from its legal analysis, and CBP's holding becomes the label. Retrieval is BM25.",
    refs: ["D10", "D11"],
    files: [{ label: "datasheet", path: "docs/DATASHEET.md" }],
  },
  {
    date: "14 Oct 2026",
    what: "Development and held-out splits frozen by date and pinned by checksum. The research prototype before this repository had already scored the same held-out rulings once.",
    refs: [],
    files: [
      { label: "splits_final.json", path: "eval/splits/splits_final.json" },
      { label: "EVALUATION.md §2.1", path: "docs/EVALUATION.md#21-from-2790-rulings-to-the-splits" },
    ],
  },
  {
    date: "15 Oct 2026",
    what: "Prompt, retrieval and the auto-accept bar chosen. The held-out split is scored once on Vertex AI with that frozen engine.",
    refs: ["D32", "D39"],
  },
  {
    date: "16–17 Oct 2026",
    what: "More options tried on the development set, including a fine-tuned Gemini 3.5 Flash. None adopted, so the held-out split is not scored again. A fixed seed goes on every model call.",
    refs: ["D22", "D44", "D73", "D76"],
  },
  {
    date: "21 Nov 2026",
    period: POST_REVIEW,
    what: "A second held-out window built from rulings published after the first, frozen before scoring, then scored once.",
    refs: ["D150", "D154"],
  },
  {
    date: "21 Nov 2026",
    what: "New experiments on the development set, each with its pass rule written before the run: newer models, a calibration map, a second-stage adjudicator, a new router.",
    refs: ["D152", "D160", "D163", "D230"],
    period: POST_REVIEW,
  },
  {
    date: "21 Nov 2026",
    what: "Repeat runs of the frozen engine, on the development set and on both held-out splits.",
    refs: ["D190", "D240", "D241"],
    period: POST_REVIEW,
  },
];

// Where the rulings sit in time (docs/NUMBERS.md: corpus row, evaluation
// table, "Fine-tuning split"). Dates are ISO; n is the rulings in the band.
export type Band = { key: string; label: string; from: string; to: string; n: string; note: string; tone: "pool" | "train" | "dev" | "test" };
export const AXIS = { from: "2023-01-01", to: "2026-12-31" };
export const BANDS: Band[] = [
  { key: "pool", label: "Precedent pool", from: "2023-01-10", to: "2026-09-25", n: "2,790 rulings", note: "every labelled ruling; a case may only read ones dated before it", tone: "pool" },
  { key: "tune", label: "Fine-tuning split", from: "2023-01-10", to: "2025-06-30", n: "632 + 67", note: "train and check; used only by the dropped fine-tuning experiment", tone: "train" },
  { key: "dev", label: "Development", from: "2025-07-01", to: "2025-12-31", n: "469 (463 scored)", note: "prompts, thresholds and calibration chosen here; the router has its own 729-ruling dev split (Jan 2024 – Dec 2025)", tone: "dev" },
  { key: "final", label: "Held-out 1", from: "2026-01-02", to: "2026-09-25", n: "377", note: "frozen 14 Oct 2026, official run on Vertex AI", tone: "test" },
  { key: "final2", label: "Held-out 2", from: "2026-09-28", to: "2026-11-13", n: "83", note: "frozen 21 Nov 2026, published after everything above", tone: "test" },
];

// What was tried and dropped, development set, 463 rulings after the D23
// adjustments (docs/NUMBERS.md: model sweep, method-page rows, adjudicator).
// vs: what the row is paired against; p is the exact McNemar test.
// runs: separate runs of the same arm; v is then their mean and the chart draws
// each run as a dot over their range instead of one run's interval.
export type Arm = { label: string; v: number; lo?: number; hi?: number; runs?: number[]; vs?: string; note: string; ref?: boolean; refs: string[] };
export const ARMS: { group: string; rows: Arm[] }[] = [
  {
    group: "In use",
    rows: [
      { label: "Gemini 4 Argon, Vertex AI", v: 0.911, lo: 0.882, hi: 0.934, note: "the production engine", ref: true, refs: ["D39"] },
      // docs/NUMBERS.md "Development accuracy over three runs": 91.8, 91.1,
      // 91.1, mean 91.3. The paired test is on the first run (D151).
      {
        label: "Gemini 4 Argon, gateway",
        v: 0.913,
        runs: [0.918, 0.911, 0.911],
        vs: "fixed 8, broke 5, p = 0.58 (first run, 91.8%)",
        note: "mean of 3 runs; same model by another path",
        ref: true,
        refs: ["D151", "D190"],
      },
    ],
  },
  {
    group: "Other models",
    rows: [
      { label: "Gemini 3.5 Flash, fine-tuned", v: 0.879, lo: 0.846, hi: 0.906, vs: "fixed 15, broke 30, p = 0.036", note: "significantly worse; dropped", refs: ["D73"] },
      { label: "Gemini 3.5 Flash", v: 0.905, lo: 0.875, hi: 0.928, vs: "fixed 16, broke 19, p = 0.736", note: "no gain", refs: ["D44"] },
      { label: "Gemini 3.8 Flash", v: 0.901, lo: 0.87, hi: 0.925, vs: "fixed 13, broke 18, p = 0.473", note: "kept only as the fallback", refs: ["D73"] },
      { label: "Gemini 4 Carbon", v: 0.881, lo: 0.849, hi: 0.908, vs: "fixed 6, broke 20, p = 0.009", note: "gateway run; 27 failed calls, counted wrong", refs: ["D153"] },
      { label: "Gemini 3.1 Pro preview", v: 0.873, lo: 0.839, hi: 0.9, vs: "fixed 12, broke 30, p = 0.008", note: "gateway run; 10 failed calls, counted wrong", refs: ["D153"] },
      { label: "Majority of three models", v: 0.916, lo: 0.887, hi: 0.938, vs: "fixed 8, broke 6, p = 0.79", note: "gateway runs; five times the cost", refs: ["D153"] },
    ],
  },
  {
    group: "Changes around the engine",
    rows: [
      { label: "rules-v2 prompt", v: 0.909, lo: 0.88, hi: 0.932, vs: "fixed 9, broke 13, p = 0.52 (vs the first gateway run)", note: "not adopted", refs: ["D157"] },
      { label: "Adjudicator, narrow", v: 0.901, vs: "fixed 2, broke 7, p = 0.18", note: "failed its rule", refs: ["D164"] },
      { label: "Adjudicator, symmetric", v: 0.909, vs: "fixed 9, broke 10, p = 1.0", note: "failed its rule", refs: ["D164"] },
      { label: "Adjudicator, moves away from China only", v: 0.92, vs: "fixed 7, broke 3, p = 0.34", note: "picked after seeing the two above, so not a fair test; failed its rule", refs: ["D164"] },
    ],
  },
];

// router-v3 against router-v2, router development split (D231): general
// rulings kept on the general engine. The D230 rule allowed 1.0 point less.
export const ROUTER_V3 = { n: 547, v2: 536, v3: 528, limit: "1.0 point" };

// Leak path, measured on the official held-out run (docs/NUMBERS.md,
// method-page row; claims-reconciliation.md 4.2).
export const LEAK = { slots: 1885, heldOutSlots: 414, cases: 377, casesTouched: 209, afterCase: 0 };

// How plans are searched and checked (judge panel round 4). Measured rows
// from docs/NUMBERS.md: "Precedent check, D170 + addendum (18 pairs)", its
// "Same, starting origin not given" row and "Per-family plan checks".
// k of n, with the 95% interval in percent as NUMBERS.md gives it.
export type PlanBar = { label: string; k: number; n: number; lo: number; hi: number; lowerIsBetter?: boolean };
export const PLAN_POST: { title: string; note: string; rows: PlanBar[] }[] = [
  {
    title: "CBP's starting origin given (how the plan search runs)",
    note: "18 pairs",
    rows: [
      { label: "moves shown that were real changes", k: 9, n: 9, lo: 70.1, hi: 100 },
      { label: "real changes found at the bar", k: 9, n: 12, lo: 46.8, hi: 91.1 },
      { label: "no-change pairs shown as a plan", k: 0, n: 6, lo: 0, hi: 39.0, lowerIsBetter: true },
    ],
  },
  {
    title: "Starting origin not given",
    note: "same pairs; one control run failed",
    rows: [
      { label: "moves shown that were real changes", k: 5, n: 6, lo: 43.7, hi: 97.0 },
      { label: "real changes found at the bar", k: 5, n: 12, lo: 19.3, hi: 68.1 },
    ],
  },
];
export const PLAN_FAMILIES = { k: 15, n: 23, lo: 44.9, hi: 81.2, supports: 9, partial: 4, not: 2 };
// docs/NUMBERS.md: "Full plan, heat gun" (proxy run) and "Per-family plan checks".
export const PLAN_COST = { heatGunUsd: "$0.22", heatGunSeconds: 33, engineCalls: 7, verifierRuns: 6, perSearchUsd: "$0.083" };
