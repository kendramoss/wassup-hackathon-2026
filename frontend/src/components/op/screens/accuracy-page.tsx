"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { api, type EvalSummary } from "@/lib/api/client";
import { copy } from "@/lib/copy";
import failureData from "@/lib/failure-cases.json";
import { ErrorBox, Main } from "../chrome";
import { ButtonLink, Chip, Fold, ScrollX, cx } from "../ui";
import { VerifierCalibration } from "../verifier-calibration";
import { KnownFailures, Reproduce } from "../failure-cases";
import { sourceLabel } from "@/lib/source-label";

// The first test window is frozen. Render its committed results and the
// recorded plan check immediately. The API supplies the additional analyses.
type Item = NonNullable<EvalSummary["items"]>[number];

const LIST_PRICE = 0.0157; // docs/NUMBERS.md: list price per prediction
const PROXY = { from: 0.899, to: 0.902, p: "1.0" }; // docs/NUMBERS.md: proxy → Vertex, McNemar
// docs/NUMBERS.md, "Field accuracy reading BOM images (synthetic)": 20
// synthetic images, 170 parts, names read 151, countries right on every one
// of them (eval/results/media-*-gemini-4-argon-litellm.json, D167, D168).
const MEDIA = { images: 20, parts: 170, read: 151, fullyRead: 15 };
// D151: the same engine and model on the development set, Vertex against the
// gateway, exact McNemar p = 0.58. Said once on the page, in the first
// Details fold that needs it (judge panel round 4).
const PARITY_HREF = "https://github.com/kendramoss/wassup-hackathon-2026/blob/main/docs/DECISIONS.md#d151--an-openai-compatible-gateway-provider-for-offline-evaluation-only-21-nov";
const GATEWAY = "Same Gemini model through an OpenAI-compatible gateway, not Vertex AI.";

const pct = (v?: number, d = 1) => (v === undefined ? "—" : `${(v * 100).toFixed(d)}%`);
const ci0 = (it?: Item) => (it?.ci_low !== undefined && it?.ci_high !== undefined ? `${Math.round(it.ci_low * 100)}–${Math.round(it.ci_high * 100)}` : "");
const ci = (it?: Item) => (it?.ci_low !== undefined && it?.ci_high !== undefined ? `${(it.ci_low * 100).toFixed(1)}–${(it.ci_high * 100).toFixed(1)}` : "");

function Card({ title, big, children, className }: { title: string; big: string; children?: React.ReactNode; className?: string }) {
  return (
    <section className={cx("border border-edge bg-sheet px-5 py-4 shadow-[4px_4px_0_var(--shadow)]", className)}>
      <h2 className="eyebrow">{title}</h2>
      <p className="mt-2 text-[34px] leading-none tracking-[-0.05em]">{big}</p>
      {children ? <p className="read mt-2.5 text-[13px] leading-snug text-dim">{children}</p> : null}
    </section>
  );
}

// The two run labels. "Official" is the run of record on Vertex AI; the
// other runs used the same model through a gateway, and say so on hover and
// in the first Details fold (D151).
function Official() {
  return (
    <span title="The run of record: Gemini on Vertex AI.">
      <Chip tone="ok">official run</Chip>
    </span>
  );
}
function Side() {
  return (
    <span title={GATEWAY}>
      <Chip>not the official run</Chip>
    </span>
  );
}

function Head({ id, title, tags, meta }: { id?: string; title: string; tags?: React.ReactNode; meta?: React.ReactNode }) {
  return (
    <div className="flex flex-wrap items-center gap-3">
      <h2 id={id} className="eyebrow">
        {title}
      </h2>
      {tags}
      {meta ? <span className="ml-auto text-[11px] text-dim">{meta}</span> : null}
    </div>
  );
}

function Details({ title = "Details", children }: { title?: string; children: React.ReactNode }) {
  return (
    <div className="mt-3">
      <Fold title={title}>
        <div className="read flex max-w-[90ch] flex-col gap-2 text-[12.5px] leading-snug text-dim">{children}</div>
      </Fold>
    </div>
  );
}

// A row of horizontal bars, one per measured figure, with its interval drawn
// as two ticks and the value beside it. The interval numbers are in Details.
function Bars({ rows }: { rows: { label: string; it?: Item; ours?: boolean; note?: string }[] }) {
  return (
    <div className="mt-4 flex flex-col gap-2.5">
      {rows
        .filter((b) => b.it)
        .map(({ label, it, ours, note }) => (
          <div key={label} className="grid items-center gap-x-4 gap-y-1 text-[12.5px] sm:grid-cols-[230px_minmax(0,1fr)_110px]">
            <span>{label}</span>
            <span className="relative h-3 bg-wash">
              <i className={cx("absolute inset-y-0 left-0", ours ? "bg-ok" : "bg-dim")} style={{ width: `${(it!.value ?? 0) * 100}%` }} />
              {it!.ci_low !== undefined && (
                <>
                  <i className="absolute -inset-y-1 w-px bg-ink" style={{ left: `${it!.ci_low * 100}%` }} />
                  <i className="absolute -inset-y-1 w-px bg-ink" style={{ left: `${(it!.ci_high ?? 0) * 100}%` }} />
                </>
              )}
            </span>
            <span className="tabular-nums sm:text-right">
              <b className={cx("font-medium", !ours && "font-normal")}>{pct(it!.value)}</b>
              {note ? <span className="text-dim"> {note}</span> : null}
            </span>
          </div>
        ))}
      <div className="grid gap-x-4 text-[11px] text-dimension sm:grid-cols-[230px_minmax(0,1fr)_110px]">
        <span />
        <span className="flex justify-between">
          {["0%", "25%", "50%", "75%", "100%"].map((t) => (
            <span key={t}>{t}</span>
          ))}
        </span>
      </div>
    </div>
  );
}

const { n, correct, auto_accepted: auto, bar, calibrated_bar: calibratedBar, misses } = failureData;
const autoWrong = misses.filter((row) => row.auto_accepted).length;
const autoRight = auto - autoWrong;
const review = n - auto;
const reviewRight = correct - autoRight;
const reviewWrong = review - reviewRight;
// D155: first country mentioned, scored on the same 377 rulings.
const FIRST_COUNTRY_BASELINE = 0.634;

// These counts are for each of the three same-pair runs with the starting
// origin given (D354). The figures are pinned against the served summary.
const planOutcomes = [
  { label: "Plan precision", correct: 9, total: 9, interval: "70.1–100%" },
  { label: "Real shifts found", correct: 9, total: 12, interval: "46.8–91.1%" },
  { label: "False positives on unchanged goods", correct: 0, total: 6, interval: "0–39.0%" },
];

const outcomeGroups = [
  { key: "auto-right", label: "Auto-accepted, right", count: autoRight, tone: "border-ok bg-ok" },
  { key: "auto-wrong", label: "Auto-accepted, wrong", count: autoWrong, tone: "border-fail bg-fail" },
  { key: "review-right", label: "Human review, right", count: reviewRight, tone: "border-ask-line bg-wash" },
  { key: "review-wrong", label: "Human review, wrong", count: reviewWrong, tone: "border-dashed border-fail bg-fail-wash" },
];

function Kpi({ title, value, lead, children }: { title: string; value: string; lead?: boolean; children: React.ReactNode }) {
  return (
    <div className={cx(
      "flex min-w-0 flex-col rounded-[2px] border p-4 sm:p-5",
      lead ? "border-ink bg-ink text-paper" : "border-edge bg-sheet text-ink",
    )}>
      <dt className="text-sm font-medium leading-snug">{title}</dt>
      <dd className="mt-3 text-[clamp(2.55rem,6.2vw,4.5rem)] font-bold leading-none tracking-[-0.075em] tabular-nums">{value}</dd>
      <dd className={cx("duo mt-3 text-sm leading-relaxed", lead ? "text-paper" : "text-dim")}>{children}</dd>
    </div>
  );
}

function OutcomeMatrix() {
  return (
    <section aria-labelledby="acc-outcomes" className="mt-5 rounded-[2px] border border-edge bg-sheet px-4 py-5 sm:px-6">
      <Head id="acc-outcomes" title="Where every ruling landed" meta="one tile per CBP ruling" />
      <div className="mt-4 grid items-start gap-5 md:grid-cols-[minmax(0,1.55fr)_minmax(0,1fr)]">
        <div
          className="grid grid-cols-[repeat(29,minmax(0,1fr))] gap-[2px] sm:gap-[3px]"
          role="img"
          aria-label={`${n} rulings. ${outcomeGroups.map(({ label, count }) => `${label}: ${count}`).join("; ")}.`}
        >
          {outcomeGroups.flatMap(({ key, count, tone }) =>
            Array.from({ length: count }, (_, i) => (
              <span key={`${key}-${i}`} data-outcome={key} className={cx("h-2.5 rounded-[1px] border sm:h-3", tone)} aria-hidden="true" />
            )),
          )}
        </div>
        <ul className="grid gap-1.5" aria-label="Outcome counts">
          {outcomeGroups.map(({ key, label, count, tone }) => (
            <li key={key} className="grid grid-cols-[12px_1fr_auto] items-center gap-2 border-b border-dashed border-line py-1.5 text-xs sm:text-sm">
              <span className={cx("size-3 rounded-[1px] border", tone)} aria-hidden="true" />
              <span>{label}</span>
              <strong className="tabular-nums">{count}</strong>
            </li>
          ))}
        </ul>
      </div>
      <Details title="Test set and confidence intervals">
        <p>
          The 95% Wilson intervals are 86.8–92.8% overall and 93.1–97.7% for auto-accepted cases. They describe sampling uncertainty in the official Gemini on Vertex AI run. Repeat runs are measured separately.
        </p>
        <p>
          The displayed confidence bar of {calibratedBar.toFixed(2)} selects the same in-scope cases as the stated bar of {bar.toFixed(2)}. The original threshold selection also considered these rulings. The current displayed threshold comes from development rulings. Prompts were not tuned on them.
        </p>
        <p>
          The case verdict stayed out of the evidence for each prediction. Earlier dated rulings could be retrieved. Gemini’s pretraining exposure is unknown. The first-country baseline used the same cases. The earlier LiteLLM proxy run scored {pct(PROXY.from)} against {pct(PROXY.to)} on Vertex AI (McNemar p = {PROXY.p}).
        </p>
      </Details>
    </section>
  );
}

type VerifierEvidence = EvalSummary["verifier_calibration"];

function PlanEvidence({ calibration }: { calibration?: VerifierEvidence }) {
  return (
    <section id="plan-checks" aria-labelledby="acc-plan" className="mt-9 scroll-mt-20 rounded-[2px] border border-edge bg-sheet px-4 py-5 sm:px-6">
      <h2 id="acc-plan" className="text-xl font-semibold tracking-tight">Production-move plan evaluation</h2>
      <p className="duo mt-1 text-sm text-dim">18 before-and-after CBP pairs across 3 runs. CBP&apos;s starting origin was supplied.</p>
      <ul className="mt-5 grid gap-4">
        {planOutcomes.map(({ label, correct, total }) => (
          <li key={label} className="grid grid-cols-[minmax(0,1fr)_auto] items-baseline gap-x-3 gap-y-1.5 text-sm">
            <span>{label}</span>
            <strong className="text-right tabular-nums">{correct}/{total} <span className="font-normal text-dim">({pct(correct / total, 0)})</span></strong>
            <span className="col-span-2 h-2.5 overflow-hidden rounded-[2px] bg-wash" aria-hidden="true">
              <span className="block h-full bg-ok" style={{ width: pct(correct / total, 4) }} />
            </span>
          </li>
        ))}
      </ul>
      <p className="duo mt-4 text-sm">
        <strong>Stability:</strong> 27/27 repeat checks were right across 3 runs. They repeat the same 9 distinct pairs, so the independent result is 9/9 distinct pairs (95% interval 70.1–100%).
      </p>
      <p className="duo mt-2 text-sm text-dim">Precedent checking on selected moves, with CBP&apos;s starting origin supplied. Without the starting origin, 5 of 12 real shifts were found in each run.</p>
      <Details title="Strict Multi-Run & Interval Details">
        <p>
          95% intervals per run: {planOutcomes.map(({ label, interval }) => `${label} ${interval}`).join(" · ")}.
          The same 18 pairs were measured three times through the LiteLLM gateway. All 27 shown checks were right across the three runs. Repeats reused the same nine shown moves, so they are not 27 independent pairs; the interval for the 9 distinct pairs is 70.1–100%.
        </p>
        <p>
          Strict count: 17/59 real changes passed in every run on the earlier development and 2026 pairs. That count includes checks with and without CBP&apos;s starting origin. Parts of the earlier 2026 test informed the prompt.
        </p>
        <p>
          The plan runs used Gemini through an OpenAI-compatible gateway. The official 377-ruling origin figures used Vertex AI. This evaluates the precedent check on selected moves. Full plan search, ranking and economics were outside the test.
        </p>
        {calibration?.splits?.length ? <VerifierCalibration c={calibration} /> : null}
      </Details>
    </section>
  );
}

// D691, D716 and D717. The first-render counts are pinned to
// eval/rcep/japan-summary.json by the Accuracy copy test.
const rcepEvidence = {
  rulings: 98,
  yes: 71,
  undetermined: 27,
  refusals: 0,
  errors: 0,
  criterionMatches: 71,
  criteriaCompared: 71,
  syntheticDevelopment: 162,
  syntheticUnseen: 71,
};

function MarketEvidence() {
  const result = rcepEvidence;

  return (
    <section id="market-evidence" aria-labelledby="acc-markets" className="mt-8 scroll-mt-20 rounded-[2px] border border-edge bg-sheet px-4 py-5 sm:px-6">
      <h2 id="acc-markets" className="text-xl font-semibold tracking-tight">Market evidence</h2>
      <div className="mt-4 grid gap-5 md:grid-cols-[minmax(0,3fr)_minmax(0,2fr)]">
        <div>
          <h3 className="text-sm font-semibold">RCEP preference calculator</h3>
          <p className="read mt-1 text-sm text-dim">All {result.rulings} unseen Japan Customs rulings qualified in the published decisions.</p>
          <div className="mt-3 h-2.5 overflow-hidden rounded-[2px] bg-wash" aria-hidden="true">
            <div className="h-full bg-ok" style={{ width: pct(result.yes / result.rulings, 4) }} />
          </div>
          <p className="mt-1.5 flex flex-wrap gap-x-4 gap-y-1 text-sm tabular-nums">
            <span><strong>{result.yes}/{result.rulings}</strong> yes</span>
            <span><strong>{result.undetermined}</strong> undetermined</span>
            <span className="text-dim">{result.refusals} refusals · {result.errors} input errors</span>
          </p>
          <p className="read mt-2 text-sm text-dim">{result.criterionMatches}/{result.criteriaCompared} deciding criteria matched. Refusal performance on real cases remains unmeasured. Customs specialist review is pending.</p>
          <Details title="RCEP evaluation details">
            <p>
              The calculator applies published rules without a model. All {result.syntheticDevelopment} development and {result.syntheticUnseen} unseen synthetic tariff-code variants were undetermined.
            </p>
            <p>
              <a href={`${copy.landing.repo}/blob/main/eval/RESULTS.md#rcep-calculator-on-japan-customs-rulings-d691-d716-d717`} className="text-ok underline underline-offset-2">
                See evaluation results and intervals →
              </a>
            </p>
          </Details>
        </div>
        <dl className="grid content-start gap-3 border-t border-line pt-3 md:border-l md:border-t-0 md:pl-5 md:pt-0">
          <div>
            <dt className="text-sm font-semibold">Canada marking</dt>
            <dd className="read mt-0.5 text-sm text-dim">Rules check only. No accuracy figure.</dd>
          </div>
          <div>
            <dt className="text-sm font-semibold">Japan model</dt>
            <dd className="read mt-0.5 text-sm text-dim">{copy.japan.notAvailable} The unseen-test requests yielded no measurement.</dd>
          </div>
        </dl>
      </div>
    </section>
  );
}

export function AccuracyPage() {
  const [data, setData] = useState<EvalSummary | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [all, setAll] = useState(false);
  const [allMisses, setAllMisses] = useState(true);
  const failures = useRef<HTMLDetailsElement>(null);
  const openFailures = () => {
    setAllMisses(true);
    if (failures.current) failures.current.open = true;
  };
  useEffect(() => {
    const revealLinkedFailures = () => {
      if (window.location.hash === "#misses" && failures.current) failures.current.open = true;
    };
    revealLinkedFailures();
    window.addEventListener("hashchange", revealLinkedFailures);
    return () => window.removeEventListener("hashchange", revealLinkedFailures);
  }, []);
  const load = () => {
    setError(null);
    api.evalSummary().then(setData, setError);
  };
  useEffect(() => {
    api.evalSummary().then(setData, setError);
  }, []);

  const items = data?.items ?? [];
  const get = (k: string) => items.find((i) => i.key === k);
  const acc = get("final_accuracy");
  const cost = get("final_cost_per_case_usd");
  const p50 = get("final_latency_p50_s");
  const p95 = get("final_latency_p95_s");
  const dev = get("dev_accuracy");
  const devAdj = get("dev_accuracy_adjusted");
  const f2Acc = get("final2_accuracy");
  const f2FpAcc = get("final2_fastpath_accuracy");
  const f2FpCov = get("final2_fastpath_coverage");
  // D160: the calibration map the server applies, checked on the held-out rows.
  const calStated = get("final_calibration_mean_stated");
  const calMean = get("final_calibration_mean_calibrated");
  const eceStated = get("final_calibration_ece_stated");
  const eceCal = get("final_calibration_ece_calibrated");
  // D262 / #493: the two errors above use different bins, so the page shows
  // the stated value rescored on the calibrated partition beside them.
  const eceSameBins = get("final_calibration_ece_stated_same_bins");
  const change = get("change_detection_heldout");
  const changeBase = get("change_detection_always_change_baseline");
  const movePrec = get("verifier_unknown-a_precision");
  // Other regimes (D51, D53, D60): textile held-out and the router's routing.
  const texAcc = get("textile_final_accuracy");
  const texAuto = get("textile_final_conf95_accuracy");
  const texCov = get("textile_final_conf95_coverage");
  const texV2 = get("textile_v2_dev_accuracy");
  // D61, D62: USMCA and pharmaceutical held-out, scored once each.
  const phAcc = get("pharma_final_accuracy");
  const phAuto = get("pharma_final_conf95_accuracy");
  const phCov = get("pharma_final_conf95_coverage");
  const usAcc = get("usmca_final_accuracy");
  const usAuto = get("usmca_final_conf95_accuracy");
  const usCov = get("usmca_final_conf95_coverage");
  // D210: each regime's post-cutoff split, scored once through the gateway,
  // and every regime pooled across both held-out windows, each ruling once.
  const tex2 = get("textile_final2_accuracy");
  const ph2 = get("pharma_final2_accuracy");
  const us2 = get("usmca_final2_accuracy");
  // D602: the Part 102 engine since 22 Nov (usmca-v5) on the same two
  // windows, scored after usmca-v1, as a candidate picked on development.
  const usV5 = get("usmca_v5_final_accuracy");
  const usV52 = get("usmca_v5_final2_accuracy");
  // D620: usmca-v5's own share and accuracy at stated 0.95 (D613), so the
  // card does not give usmca-v1's 47% as the current auto-accept share.
  const usV5Auto = get("usmca_v5_final_conf95_accuracy");
  const usV5Cov = get("usmca_v5_final_conf95_coverage");
  const usV52Auto = get("usmca_v5_final2_conf95_accuracy");
  const usV52Cov = get("usmca_v5_final2_conf95_coverage");
  const count = (it?: Item) => Math.round((it?.value ?? 0) * (it?.n ?? 0));
  const allReg = get("all_regimes_heldout_accuracy");
  const later = (it?: Item) => (it ? ` On ${it.n} later rulings (28 Sep to 13 Nov 2026, scored once): ${pct(it.value)} (${ci(it)}).` : "");
  // D233: the router (router-v4) on the held-out windows, and the routed
  // path: each later ruling answered by the engine the router picks.
  const routeOut2 = get("router_final2_out_of_scope_recall");
  const route1 = get("router_final_routing");
  const route1Spec = get("router_final_specialist_routing");
  const route2 = get("router_final2_routing");
  const route2Spec = get("router_final2_specialist_routing");
  const routed = get("all_regimes_final2_routed_accuracy");
  const routedOld = get("all_regimes_final2_routed_accuracy_router_v2");
  const moveRec = get("verifier_unknown-a_recall");
  // D170 + D192: the precedent check on flip pairs from rulings published
  // after the corpus cut-off (the 15 D170 pairs plus the 3-pair addendum),
  // each frozen before scoring and run 3 times with the same figures (D354),
  // CBP's starting origin given.
  const postRec = get("planner_post_combined_known-a_recall_at_bar");
  // The precedent check's stricter recall (D86 pool): real-change pairs shown
  // in every run, so the plan-check card can say why it differs (round 5).
  const calSplits = data?.verifier_calibration?.splits ?? [];
  const calPairs = calSplits.reduce((n, x) => n + (x.pair_level.real_change_pairs ?? 0), 0);
  const calEvery = calSplits.reduce((n, x) => n + (x.pair_level.shown_every_run ?? 0), 0);
  const postCtl = get("planner_post_combined_known-a_controls_shown");
  // D190: three development runs of the same engine, no cache.
  const reps = [1, 2, 3].map((i) => get(`repeat_dev_accuracy_run${i}`)).filter((x): x is Item => Boolean(x));
  const repMean = get("repeat_dev_accuracy_mean");
  const repFlips = get("repeat_dev_answer_flips");
  // D241: two repeats of the frozen engine on each held-out split, beside the
  // official single run. Shown as one line, not new headline numbers.
  const heldRep = (["final", "final2"] as const).map((sp) => ({
    off: get(`repeat_${sp}_accuracy_official`),
    r1: get(`repeat_${sp}_accuracy_repeat1`),
    r2: get(`repeat_${sp}_accuracy_repeat2`),
    flips: get(`repeat_${sp}_answer_flips`),
  }));
  const heldRepOK = heldRep.every((h) => h.off && h.r1 && h.r2 && h.flips);
  const postReal = postRec?.n ?? 0;
  const pairEvery = calPairs ? `${Math.round((calEvery / calPairs) * 100)}%` : "—";

  const baselines = [
    { label: "Unde", it: acc, ours: true },
    { label: "First country named", it: get("baseline_first_mentioned") },
    { label: "Most-mentioned country", it: get("baseline_most_mentioned") },
    { label: "Country of final assembly*", it: get("baseline_final_assembly") },
    { label: "Last-mentioned country*", it: get("baseline_last_mentioned") },
    { label: "Always the most common origin", it: get("baseline_majority") },
  ];
  const regimes = [
    { label: "Textile", it: texAcc, note: texAcc ? `· ${texAcc.n}` : "" },
    { label: "Pharmaceutical", it: phAcc, note: phAcc ? `· ${phAcc.n}` : "" },
    { label: "Marking rules, earlier engine", it: usAcc, note: usAcc ? `· ${usAcc.n}` : "" },
    { label: "All rule sets, each ruling once", it: allReg, ours: true, note: allReg ? `· ${allReg.n}` : "" },
  ];

  // Rows whose source is the research write-up cannot be recomputed from this
  // repository (docs/NUMBERS.md), so they never read "official". Recomputed
  // baselines and the second held-out split say what they are (D154, D155).
  const status = (it: Item): { label: string; tone?: "ok" | "ask" } => {
    const kind = it.key?.startsWith("baseline") ? "baseline, no model" : it.key?.startsWith("change_detection") ? "weak" : "research run";
    if (it.source?.startsWith("hackathon-research/") || it.source?.startsWith("unavailable:")) return { label: `${kind} · reported, unverified` };
    if (it.key?.startsWith("baseline")) return { label: "baseline, no model · recomputed" };
    // D241: held-out repeat runs (the official run, then gateway repeats).
    if (it.key?.startsWith("repeat_final")) return { label: "unseen · repeats, gateway" };
    // D354: the post-cutoff flip pairs were run three times, same figures.
    if (it.key?.startsWith("planner_post_")) return { label: "unseen · gateway, same in 3 runs" };
    if (it.key?.startsWith("final2_") || it.key?.includes("_final2_")) return { label: "unseen · gateway, scored once" };
    if (it.key === "all_regimes_heldout_accuracy") return { label: "unseen · pooled, mixed runs" };
    if (it.key?.startsWith("router_final")) return { label: "unseen · gateway, scored once" };
    if (!it.development_only && it.source?.includes("litellm")) return { label: "unseen · LiteLLM proxy, not Vertex AI" };
    if (it.key?.startsWith("change_detection")) return { label: "weak" };
    return it.development_only ? { label: "development set" } : { label: "official", tone: "ok" };
  };
  const lead = ["final_accuracy", "final_fastpath_coverage", "final_fastpath_accuracy", "final2_accuracy", "baseline_first_mentioned", "baseline_most_mentioned", "dev_accuracy_adjusted", "final_cost_per_case_usd", "final_latency_p50_s", "textile_final_accuracy", "router_final_routing", "change_detection_heldout", "verifier_unknown-a_precision"];
  const rows = all ? items : lead.map(get).filter((x): x is Item => Boolean(x));
  const value = (it: Item) =>
    it.key?.endsWith("_flips") ? `${it.value} of ${it.n}` : it.key?.endsWith("_usd") ? `$${it.value?.toFixed(4).replace(/0+$/, "")}` : it.key?.endsWith("_s") ? `${it.value?.toFixed(1)} s` : pct(it.value);

  return (
    <Main>
      <header>
        <h1 className="text-[clamp(2.25rem,5vw,3.75rem)] font-semibold leading-tight tracking-[-0.06em]">How accurate is it?</h1>
        <p className="read mt-2 max-w-[65ch] text-base text-dim">U.S. origin predictions compared with 377 real CBP rulings, January–September 2026: single-origin, multi-country U.S. substantial-transformation rulings, not accuracy across every market or rule set.</p>
      </header>
      <dl className="mt-6 grid gap-3 md:grid-cols-3">
        <Kpi title="Overall accuracy" value={pct(correct / n)} lead>
          {correct} / {n} right on the unseen test set. {pct(FIRST_COUNTRY_BASELINE)} for the first-country baseline.
        </Kpi>
        <Kpi title="Auto-accepted accuracy" value={pct(autoRight / auto)}>
          {autoRight} / {auto} right at ≥ {bar.toFixed(2)} stated confidence and in scope.
        </Kpi>
        <Kpi title="Human escalation rate" value={pct(review / n)}>
          {review} sent to review. The model had {reviewRight} right and {reviewWrong} wrong.
        </Kpi>
      </dl>

      <OutcomeMatrix />
      <PlanEvidence calibration={data?.verifier_calibration} />
      <MarketEvidence />

      <section className="mt-8" aria-labelledby="acc-limits">
        <h2 id="acc-limits" className="eyebrow">Test limits</h2>
        <ul className="read mt-2 flex max-w-[92ch] list-disc flex-col gap-1.5 pl-5 text-sm text-dim">
          <li>{copy.jurisdiction.notCovered}</li>
          <li>{copy.jurisdiction.noDetector}</li>
          <li>Not a ruling. A licensed broker confirms before anything is filed.</li>
        </ul>
      </section>

      <div className="mt-6">
        <Fold title="More evaluation results and sources">
          {error ? <ErrorBox error={error} onRetry={load} /> : null}
          {!data && !error ? <div className="mt-3 h-12 border border-line bg-wash" aria-busy="true" aria-label="Additional results" /> : null}
          {acc && (
            <>
          <section className="mt-8" aria-labelledby="acc-thumb">
            <Head id="acc-thumb" title="Against simple rules of thumb" meta={<>same {n} rulings · * reported, not reproducible here</>} />
            <Bars rows={baselines} />
            <Details>
              <p>
                95% intervals:{" "}
                {baselines
                  .filter((b) => b.it)
                  .map((b) => `${b.label.replace("*", "")} ${ci(b.it)}`)
                  .join("; ")}
                . The rules of thumb are recomputed with no model (go run ./cmd/eval baselines). Rows marked * come from the research write-up and are reported,
                unverified: not reproducible from this repository.
              </p>
            </Details>
          </section>

          <div className="mt-8 grid gap-5 md:grid-cols-2">
            <Card title="Cost per prediction" big={cost ? `$${cost.value}` : "—"}>
              promo price; ${LIST_PRICE} list.
            </Card>
            {/* The eval times one engine call; a live prediction also runs the
                router and waits on Vertex, so say both (#183). */}
            <Card title="Time per ruling" big={p50 ? `${p50.value?.toFixed(1)} s` : "—"}>
              median; 95% within {p95?.value?.toFixed(1)} s. Engine only.
            </Card>
          </div>
          <Details>
            <p>Gemini on Vertex AI, in the official run. A live prediction on this site also runs the scope check and may wait in a queue; that end-to-end time is not measured here.</p>
          </Details>

          {f2Acc && (
            <section className="mt-8" aria-labelledby="acc-later">
              <Head id="acc-later" title="Newer rulings, Sep–Nov 2026" tags={<Side />} meta="small · scored once" />
              <div className="mt-3 grid gap-5 md:grid-cols-2">
                <Card title="Overall" big={pct(f2Acc.value)}>
                  {count(f2Acc)} of {f2Acc.n} right.
                </Card>
                {f2FpAcc && (
                  <Card title="When it's sure" big={pct(f2FpAcc.value)}>
                    right, on {pct(f2FpCov?.value, 0)} of them.
                  </Card>
                )}
              </div>
              <p className="read mt-3 max-w-[80ch] text-[13px] text-dim">Small, and easier for simple rules: it shows the engine holds up after its cutoff, not that it improved.</p>
              <Details>
                <p>
                  {f2Acc.n} CBP rulings from 28 Sep to 13 Nov 2026, a second unseen test set frozen before scoring; the same engine scored once. 95% intervals: {ci(f2Acc)}
                  {f2FpAcc ? `; when sure ${ci(f2FpAcc)}` : ""}.
                </p>
                <p id="gateway" className="scroll-mt-20">
                  Runs marked &ldquo;not the official run&rdquo; used the same Gemini model through an OpenAI-compatible gateway, not Vertex AI. On the development set the two
                  agree (same engine and model, exact McNemar p = 0.58,{" "}
                  <a href={PARITY_HREF} target="_blank" rel="noreferrer" className="text-ok underline underline-offset-2">
                    D151
                  </a>
                  ).
                </p>
              </Details>
            </section>
          )}

          {reps.length === 3 && repMean && (
            <section className="mt-8" aria-labelledby="acc-repeat">
              <Head id="acc-repeat" title="Run it again" tags={<Side />} meta={`development set, ${repMean.n} rulings · 3 runs`} />
              <div className="mt-3 grid gap-5 md:grid-cols-2">
                <Card title="Three runs, same engine" big={pct(repMean.value)}>
                  on average ({reps.map((r) => pct(r.value)).join(", ")}).
                </Card>
                <Card title="Answers that changed" big={repFlips ? `${repFlips.value} of ${repFlips.n}` : "—"}>
                  Every other ruling got the same country three times.
                </Card>
              </div>
              <Details>
                <p>The same model was called fresh each time, with no cache.{heldRepOK ? "" : " Not measured on unseen rulings."}</p>
                {heldRepOK && (
                  <p>
                    On the unseen test sets, two more runs of the same engine: {heldRep[0].off?.n} rulings, official {pct(heldRep[0].off?.value)} against {pct(heldRep[0].r1?.value)}{" "}
                    and {pct(heldRep[0].r2?.value)}; {heldRep[1].off?.n} newer rulings, official {pct(heldRep[1].off?.value)} against {pct(heldRep[1].r1?.value)} and{" "}
                    {pct(heldRep[1].r2?.value)}. The answer changed on {heldRep[0].flips?.value} and {heldRep[1].flips?.value} rulings. Repeats through the gateway; the
                    official 377 run is Vertex AI.
                  </p>
                )}
              </Details>
            </section>
          )}

          {calMean && calStated && (
            <section className="mt-8" aria-labelledby="acc-conf">
              <Head id="acc-conf" title="Is its confidence honest?" tags={<Official />} meta={`same ${calMean.n} rulings · no model calls`} />
              <div className="mt-3 grid gap-5 md:grid-cols-2">
                <Card title="Model's own confidence" big={pct(calStated.value)}>
                  on average, against {pct(acc?.value)} right. It runs high.
                </Card>
                <Card title="Confidence the screens show" big={pct(calMean.value)}>
                  on average, against the same {pct(acc?.value)}. Adjusted on separate rulings.
                </Card>
              </div>
              <Details>
                <p>
                  The adjusted (calibrated) confidence comes from a map fitted on the development set only and checked once on these {calMean.n} unseen rulings. Results
                  show this value; the textile, Part 102 and pharmaceutical rules show the model&apos;s own.
                </p>
                {eceStated?.value !== undefined && (
                  <p>
                    Calibration error: stated {eceStated.value.toFixed(3)} on its own bins
                    {eceSameBins?.value !== undefined ? `, ${eceSameBins.value.toFixed(4)} on the map's bins` : ""}
                    {eceCal?.value !== undefined ? `; calibrated ${eceCal.value.toFixed(3)} on the map's bins` : ""}. The two calibration errors do not use the same bins: the map
                    pools confidence levels, so some of the drop is bin merging. The same-bin figure scores the stated confidence on the calibrated bins; Brier does not
                    depend on bins.
                  </p>
                )}
                <p>{copy.calSteps}</p>
              </Details>
            </section>
          )}

          <section className="mt-8" aria-labelledby="acc-media">
            <Head id="acc-media" title="Import from a photo or scan" tags={<Side />} meta="synthetic images, illustrative · small" />
            <div className="mt-3 grid gap-5 md:grid-cols-2">
              <Card title="Part names read" big={`${MEDIA.read} of ${MEDIA.parts}`}>
                on {MEDIA.images} synthetic parts-list images.
              </Card>
              <Card title="Wrong countries" big="0">
                Real photos, scans and handwriting are not measured yet.
              </Card>
            </div>
            <Details>
              <p>
                The images are tables, tilted photos and scans we rendered; measured once. {MEDIA.fullyRead} of {MEDIA.images} images were read in full. The{" "}
                {MEDIA.parts - MEDIA.read} misses are names that did not match exactly, mostly a part number added to the name. Every part name it read came with the
                right country. Every imported row opens as a draft for you to check.
              </p>
            </Details>
          </section>

          {(texAcc || route1) && (
            <section className="mt-8" aria-labelledby="acc-rules">
              <Head
                id="acc-rules"
                title="Other rule sets"
                tags={
                  <span title="Textile ran on Vertex AI. The others used the same Gemini model through the gateway or the earlier LiteLLM proxy.">
                    <Chip>mixed runs</Chip>
                  </span>
                }
                meta="unseen 2026 rulings · number of rulings beside each"
              />
              <Bars rows={regimes} />
              {(route1 || routed) && (
                <div className="mt-5 grid gap-5 md:grid-cols-2">
                  {route1 && (
                    <Card title="Sent to the right rules" big={pct(route1.value)}>
                      of {route1.n} rulings. Partly tuned on newer rulings, so not a clean test.
                    </Card>
                  )}
                  {routed && (
                    <Card title="Routed, newer rulings" big={pct(routed.value)}>
                      {count(routed)} of {routed.n} right. Partly tuned on these rulings, so not a clean test.
                    </Card>
                  )}
                </div>
              )}
              <Details>
                {texAcc && (
                  <p>
                    Textile rules (19 CFR 102.21): {pct(texAcc.value)} on {texAcc.n} unseen 2026 textile rulings ({ci(texAcc)}). {pct(texAuto?.value)} right when sure enough to
                    auto-accept, on {pct(texCov?.value, 0)} of them.
                    {texV2 ? ` The prompt now in use scored ${pct(texV2.value)} on the development set.` : ""}
                    {later(tex2)}
                  </p>
                )}
                {phAcc && (
                  <p>
                    Pharmaceutical rules: {pct(phAcc.value)} on {phAcc.n} unseen 2026 pharmaceutical rulings ({ci(phAcc)}). {pct(phAuto?.value)} right at confidence 0.95 or above,
                    on {pct(phCov?.value, 0)} of them.
                    {later(ph2)}
                    {ph2 && (ph2.n ?? 0) < 10 ? " Too few to read much into." : ""}
                  </p>
                )}
                {usAcc && (
                  <p>
                    Part 102 marking origin: {pct(usAcc.value)} on {usAcc.n} unseen 2026 Part 102 marking rulings ({ci(usAcc)}), scored first, with the earlier engine (usmca-v1). That engine was {pct(usAuto?.value)} right at confidence 0.95 or above, on {pct(usCov?.value, 0)} of them.
                    {later(us2)} {us2 ? " The later window's Part 102 runs had only the 15 window-2 rulings as precedents, not the full pool (D612)." : ""}
                    {usV5 && usV52
                      ? ` Since 22 Nov the engine in use is usmca-v5, which fills a short 102.11 checklist first; it was picked on development rulings. On the same rulings, scored after: ${count(usV5)} of ${usV5.n} and ${count(usV52)} of ${usV52.n}, ${count(usV5) + count(usV52)} of ${(usV5.n ?? 0) + (usV52.n ?? 0)} against ${count(usAcc) + count(us2)} for usmca-v1. That is a candidate comparison, not a fresh test (p = 0.22).`
                      : ""}
                    {usV5Auto && usV5Cov && usV52Auto && usV52Cov
                      ? ` It auto-accepts fewer answers: confidence 0.95 or above on ${usV5Auto.n} of ${usV5Cov.n} (${pct(usV5Cov.value, 0)}), ${count(usV5Auto)} right; over both windows ${(usV5Auto.n ?? 0) + (usV52Auto.n ?? 0)} of ${(usV5Cov.n ?? 0) + (usV52Cov.n ?? 0)}, ${count(usV5Auto) + count(usV52Auto)} right.`
                      : ""}
                  </p>
                )}
                {usV5 && (
                  <p>
                    In the Part 102 plan candidate comparison, 6 of 18 rulings had a plan template with usmca-v1, which agreed with the rule on 3 of 6 decided moves.
                    The pass with usmca-v5 and 20 families found 8 of 18 templates and 6 of 6 agreements. Five of those six moves keep the country CBP held. Each pass showed one move that the rule says shifts origin. The second pass is a candidate comparison on cases this engine had already been scored on.{" "}
                    <Link href="/examples/rule-check" className="text-ok underline underline-offset-2">See the recorded rule check →</Link>
                  </p>
                )}
                {allReg && (
                  <p>
                    All rule sets: {pct(allReg.value)} on {allReg.n} unseen rulings from January to November 2026 ({ci(allReg)}), all four rule sets, each ruling counted once. Runs
                    on Vertex AI, the earlier proxy and the gateway are pooled.
                  </p>
                )}
                {route1 && (
                  <p>
                    Routing: {pct(route1.value)} of {route1.n} unseen 2026 rulings the router sent to the engine of their legal test ({ci(route1)}); {pct(route1Spec?.value)} of the{" "}
                    {route1Spec?.n} pharmaceutical, Part 102 and textile ones.
                    {route2 && route2Spec
                      ? ` On the ${route2.n} later rulings: ${pct(route2.value)}, and ${pct(route2Spec.value)} of the ${route2Spec.n} specialist ones (${ci(route2Spec)}).`
                      : ""}
                    {routeOut2 ? ` The router before it kept only ${pct(routeOut2.value)} of later pharmaceutical and Part 102 rulings away from the general engine.` : ""}
                    {" "}Caveat: the &quot;trade programs&quot; cue in router-v4 came from reading these later misses, not from development, so this is not clean held-out evidence; without the 6 cue rulings the specialist comparison is 14/17 vs 12/17 (fixed 2, broke 0).
                  </p>
                )}
                {routed && (
                  <p>
                    Routed product: {pct(routed.value)} on {routed.n} later rulings (28 Sep to 13 Nov 2026), all four rule sets, each answered by the engine the router picked ({ci(routed)}),{" "}
                    {count(routed)} right.
                    {routedOld ? ` With the router before it: ${Math.round((routedOld.value ?? 0) * (routedOld.n ?? 0))} of ${routedOld.n} right.` : ""} Each ruling sent straight to its own
                    rules scores a little higher; the gap is the cost of routing. Caveat: the &quot;trade programs&quot; cue in router-v4 came from reading these later misses;
                    without the 6 cue rulings router-v4 is 90/101, below router-v2&apos;s 91/101, so this is not clean held-out evidence.
                  </p>
                )}
              </Details>
            </section>
          )}

          <section className="mt-8 border border-edge bg-sheet px-5 py-4 shadow-[4px_4px_0_var(--shadow)]">
            <h2 className="eyebrow">What we don&apos;t claim</h2>
            <ul className="read mt-3 flex list-disc flex-col gap-2 pl-5 text-[14px]">
              {change && (
                <li>
                  Spotting from scratch that a change flips the origin: {pct(change.value, 0)}, against {pct(changeBase?.value, 0)} for always saying &ldquo;it changes&rdquo;. Weak.
                </li>
              )}
              {movePrec && <li>Plan move checks are measured on few pairs, and partly not a clean test. They are not a measure of plan quality.</li>}
            </ul>
            <Details>
              {change && (
                <p>
                  Change spotting: {pct(change.value, 0)} ({ci0(change)}) on {change.n} pairs, against {pct(changeBase?.value)} for always saying &ldquo;it changes&rdquo;; shown only
                  with both baselines.
                </p>
              )}
              {movePrec && (
                <p>
                  Move checks on the Plan step: {movePrec.n} of {movePrec.n} flagged moves were right ({ci0(movePrec)}) on a {get("verifier_unknown-a_cost_usd")?.n ?? 54}-pair
                  development set, finding {pct(moveRec?.value, 0)} of real flips. On the 2026 pairs every precedent-supported check was a real change too (12 of 12 in the first
                  scored run; 23 of 23 across all four runs in the precedent-check table), but part of the check&apos;s prompt was written after the research team saw those
                  pairs, so it isn&apos;t a clean held-out test.
                  {postRec ? ` The clean test on ${postReal + (postCtl?.n ?? 0)} later pairs is small; it is not a measure of plan quality.` : ""} The three recall figures on this
                  page count different things: the {pct(moveRec?.value, 0)} here is one development run, each pair once; &ldquo;about {pairEvery}&rdquo; in the precedent-check section is
                  real changes that cleared the bar in every run;{postRec ? ` the ${pct(postRec.value, 0)} is the later pairs, with CBP's starting origin given.` : ""}
                </p>
              )}
            </Details>
          </section>

          <Reproduce />

          <section className="mt-6" aria-label="Every number">
            <Fold title="Every number" meta={`${items.length} figures, each with its source`}>
              <ScrollX label="Every number">
                <table className="mt-1 w-full min-w-[760px] border-collapse text-[12.5px]">
                  <thead>
                    <tr>
                      {["What", "Value", "95% interval", "n", "Status", "Source"].map((h) => (
                        <th key={h} scope="col" className="border-b border-line py-2 pr-3 text-left text-[11px] font-medium uppercase tracking-[0.08em] text-dim">
                          {h}
                        </th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {rows.map((it) => {
                      const s = status(it);
                      return (
                        <tr key={it.key} className="border-b border-dashed border-line align-top">
                          <td className="duo py-2 pr-3">{it.label}</td>
                          <td className="whitespace-nowrap py-2 pr-3 tabular-nums">{value(it)}</td>
                          <td className="whitespace-nowrap py-2 pr-3 tabular-nums text-dim">{ci(it)}</td>
                          <td className="py-2 pr-3 tabular-nums">{it.n ?? ""}</td>
                          <td className="py-2 pr-3">
                            <Chip tone={s.tone}>{s.label}</Chip>
                          </td>
                          <td className="max-w-[24ch] truncate py-2 text-[11px] text-dim" title={it.source}>
                            {sourceLabel(it.source)}
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </ScrollX>
              <button type="button" onClick={() => setAll(!all)} className="mt-3 min-h-8 text-[12.5px] text-ok hover:underline">
                {all ? "Show the main numbers" : `Show all ${items.length} numbers`}
              </button>
              <p className="duo mt-3 max-w-[90ch] text-[11.5px] text-dim">
                {data?.status} {dev ? `Development split: ${pct(dev.value)} on all ${dev.n} rulings, unadjusted` : ""}
                {devAdj ? `; ${pct(devAdj.value)} on ${devAdj.n} after the scoring adjustments.` : dev ? "." : ""} Generated {data?.generated_at?.slice(0, 10)} from GET /v1/eval/summary;
                every row traces to docs/NUMBERS.md.
              </p>
            </Fold>
          </section>
            </>
          )}
        </Fold>
      </div>

      <nav aria-label="Explore the evidence" className="mt-8 flex flex-wrap gap-3 border-t border-line pt-5">
        <ButtonLink href="#misses" onClick={openFailures}>Inspect all {misses.length} known failures →</ButtonLink>
        <ButtonLink href="/method" variant="ghost">See system architecture →</ButtonLink>
      </nav>
      <details ref={failures} id="misses" className="group mt-4 scroll-mt-20 border-b border-line">
        <summary className="flex min-h-11 list-none items-center gap-2 text-sm text-ink hover:text-ok [&::-webkit-details-marker]:hidden">
          <span aria-hidden="true" className="inline-block w-3 text-dim group-open:rotate-90">▸</span>
          Failure table and patterns
        </summary>
        <KnownFailures all={allMisses} onAllChange={setAllMisses} />
      </details>
    </Main>
  );
}
