"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { api, type EvalSummary } from "@/lib/api/client";
import { copy } from "@/lib/copy";
import { ErrorBox, Main } from "../chrome";
import { Chip, ScrollX, Spinner, cx } from "../ui";
import { VerifierCalibration } from "../verifier-calibration";
import { KnownFailures, Reproduce } from "../failure-cases";
import { sourceLabel } from "@/lib/source-label";

// "How accurate is it?" (DESIGN.md, locked 16 Oct, frame p-accuracy.html):
// every held-out ruling as a square beside the three headline cards, then
// rules of thumb, cost and time, what we don't claim, and every number.
// All figures come from GET /v1/eval/summary except the two marked below,
// which docs/NUMBERS.md records and the endpoint doesn't carry.

type Item = NonNullable<EvalSummary["items"]>[number];

const LIST_PRICE = 0.0157; // docs/NUMBERS.md: list price per prediction
const PROXY = { from: 0.899, to: 0.902, p: "1.0" }; // docs/NUMBERS.md: proxy → Vertex, McNemar
// docs/NUMBERS.md, "Field accuracy reading BOM images (synthetic)": 20
// synthetic images, 170 parts, names read 151, countries right on every one
// of them (eval/results/media-*-gemini-4-argon-litellm.json, D167, D168).
const MEDIA = { images: 20, parts: 170, read: 151, fullyRead: 15 };
// D151: the same engine and model on the development set, Vertex against the
// gateway, exact McNemar p = 0.58. Said once on the page, before the first
// gateway section (judge panel round 4: it was repeated on five cards,
// including the photo import, where engine parity is beside the point).
const PARITY_HREF = "https://github.com/kendramoss/wassup-hackathon-2026/blob/main/docs/DECISIONS.md#d151--an-openai-compatible-gateway-provider-for-offline-evaluation-only-21-nov";


const pct = (v?: number, d = 1) => (v === undefined ? "—" : `${(v * 100).toFixed(d)}%`);
const ci0 = (it?: Item) => (it?.ci_low !== undefined && it?.ci_high !== undefined ? `${Math.round(it.ci_low * 100)}–${Math.round(it.ci_high * 100)}` : "");
const ci = (it?: Item) => (it?.ci_low !== undefined && it?.ci_high !== undefined ? `${(it.ci_low * 100).toFixed(1)}–${(it.ci_high * 100).toFixed(1)}` : "");

function Card({ title, big, children, className }: { title: string; big: string; children: React.ReactNode; className?: string }) {
  return (
    <section className={cx("border border-edge bg-sheet px-5 py-4 shadow-[4px_4px_0_var(--shadow)]", className)}>
      <h2 className="eyebrow">{title}</h2>
      <p className="mt-2 text-[34px] leading-none tracking-[-0.05em]">{big}</p>
      <p className="read mt-2.5 text-[13px] leading-snug text-dim">{children}</p>
    </section>
  );
}

export function AccuracyPage() {
  const [data, setData] = useState<EvalSummary | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [all, setAll] = useState(false);
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
  const fpAcc = get("final_fastpath_accuracy");
  const fpCov = get("final_fastpath_coverage");
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
  const postPrec = get("planner_post_combined_known-a_precision_at_bar");
  const postRec = get("planner_post_combined_known-a_recall_at_bar");
  // The precedent check's stricter recall (D86 pool): real-change pairs shown
  // in every run, so the plan-check card can say why it differs (round 5).
  const calSplits = data?.verifier_calibration?.splits ?? [];
  const calPairs = calSplits.reduce((n, x) => n + (x.pair_level.real_change_pairs ?? 0), 0);
  const calEvery = calSplits.reduce((n, x) => n + (x.pair_level.shown_every_run ?? 0), 0);
  const postCtl = get("planner_post_combined_known-a_controls_shown");
  const postPairs = (postRec?.n ?? 0) + (postCtl?.n ?? 0);
  const postCtlShown = Math.round((postCtl?.value ?? 0) * (postCtl?.n ?? 0));
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
  const postShown = postPrec?.n ?? 0;
  const postRight = Math.round((postPrec?.value ?? 0) * postShown);
  const postReal = postRec?.n ?? 0;
  const postFound = Math.round((postRec?.value ?? 0) * postReal);
  // The precedent-check section's pair-level line (verifier-calibration.tsx).
  const vcs = data?.verifier_calibration?.splits ?? [];
  const vcPairs = vcs.reduce((n, x) => n + (x.pair_level.real_change_pairs ?? 0), 0);
  const vcEvery = vcs.reduce((n, x) => n + (x.pair_level.shown_every_run ?? 0), 0);
  const pairEvery = vcPairs ? `${Math.round((vcEvery / vcPairs) * 100)}%` : "—";

  // The squares, worked out from the rates (DESIGN.md: 288 / 12 / 52 / 25).
  const n = acc?.n ?? 377;
  const right = Math.round((acc?.value ?? 0) * n);
  const auto = fpAcc?.n ?? Math.round((fpCov?.value ?? 0) * n);
  const autoRight = Math.round((fpAcc?.value ?? 0) * auto);
  const autoWrong = auto - autoRight;
  const review = n - auto;
  const reviewRight = right - autoRight;
  const reviewWrong = review - reviewRight;
  const squares = [
    ...Array(autoRight).fill("ar"),
    ...Array(autoWrong).fill("aw"),
    ...Array(reviewRight).fill("rr"),
    ...Array(reviewWrong).fill("rw"),
  ] as ("ar" | "aw" | "rr" | "rw")[];
  const sq = { ar: "bg-ok", aw: "border border-ink bg-white", rr: "bg-ok/25", rw: "border border-dashed border-dim bg-white" };

  const bars = [
    { label: "Unde (Gemini, Vertex AI)", it: acc, ours: true },
    { label: "First country named", it: get("baseline_first_mentioned") },
    { label: "Most-mentioned country", it: get("baseline_most_mentioned") },
    { label: "Country of final assembly*", it: get("baseline_final_assembly") },
    { label: "Last-mentioned country*", it: get("baseline_last_mentioned") },
    { label: "Always the most common origin", it: get("baseline_majority") },
  ].filter((b) => b.it);

  // Rows whose source is the research write-up cannot be recomputed from this
  // repository (docs/NUMBERS.md), so they never read "official". Recomputed
  // baselines and the second held-out split say what they are (D154, D155).
  const status = (it: Item): { label: string; tone?: "ok" | "ask" } => {
    const kind = it.key?.startsWith("baseline") ? "baseline, no model" : it.key?.startsWith("change_detection") ? "weak" : "research run";
    if (it.source?.startsWith("hackathon-research/") || it.source?.startsWith("unavailable:")) return { label: `${kind} · reported, unverified` };
    if (it.key?.startsWith("baseline")) return { label: "baseline, no model · recomputed" };
    // D241: held-out repeat runs (the official run, then gateway repeats).
    if (it.key?.startsWith("repeat_final")) return { label: "held-out · repeatability, gateway repeats" };
    // D354: the post-cutoff flip pairs were run three times, same figures.
    if (it.key?.startsWith("planner_post_")) return { label: "held-out · gateway, same in 3 runs" };
    if (it.key?.startsWith("final2_") || it.key?.includes("_final2_")) return { label: "held-out · gateway, scored once" };
    if (it.key === "all_regimes_heldout_accuracy") return { label: "held-out · pooled, mixed paths" };
    if (it.key?.startsWith("router_final")) return { label: "held-out · gateway, scored once" };
    if (!it.development_only && it.source?.includes("litellm")) return { label: "held-out · LiteLLM proxy, not Vertex AI" };
    if (it.key?.startsWith("change_detection")) return { label: "weak" };
    return it.development_only ? { label: "development set" } : { label: "official", tone: "ok" };
  };
  const lead = ["final_accuracy", "final_fastpath_coverage", "final_fastpath_accuracy", "final2_accuracy", "baseline_first_mentioned", "baseline_most_mentioned", "dev_accuracy_adjusted", "final_cost_per_case_usd", "final_latency_p50_s", "textile_final_accuracy", "router_final_routing", "change_detection_heldout", "verifier_unknown-a_precision"];
  const rows = all ? items : lead.map(get).filter((x): x is Item => Boolean(x));
  const value = (it: Item) =>
    it.key?.endsWith("_flips") ? `${it.value} of ${it.n}` : it.key?.endsWith("_usd") ? `$${it.value?.toFixed(4).replace(/0+$/, "")}` : it.key?.endsWith("_s") ? `${it.value?.toFixed(1)} s` : pct(it.value);

  return (
    <Main>
      <h1 className="text-[34px] tracking-[-0.05em]">How accurate is it?</h1>
      <p className="read mt-2 max-w-[68ch] text-[15px] text-dim">
        We gave Unde {n} real CBP origin rulings from January to September 2026, none of them used to tune it, and compared its answer with CBP&apos;s. As in real use, it
        could read earlier 2026 rulings as precedent, never the case&apos;s own ruling or a later one. Every number below has its interval and its source, and the{" "}
        <a href="#misses" className="text-ok underline underline-offset-2">
          rulings it got wrong
        </a>{" "}
        are listed by name. The intervals resample these same scored rows with a fixed seed; they describe sampling error conditional on one run, not model-run variance, label error, calibration fitting or split construction. Repeat runs are measured separately, on the development set and on both held-out splits (see Run it again below).
      </p>
      {/* Below md the headline numbers come before the reproduce block, so a
          phone sees the result first (judge panel round 2, D220). */}
      <div className="flex flex-col">
      <div className="max-md:order-2">
        <Reproduce />
      </div>
      <div className="max-md:order-1">
      {error ? (
        <div className="mt-6">
          <ErrorBox error={error} onRetry={load} />
        </div>
      ) : null}
      {!data && !error && (
        <div className="mt-6">
          <Spinner label="Loading the numbers…" />
        </div>
      )}

      {acc && (
        <>
          <div className="mt-8 grid gap-5 lg:grid-cols-[minmax(0,1fr)_300px]">
            <section className="border border-edge bg-sheet px-5 py-4 shadow-[4px_4px_0_var(--shadow)]">
              <div className="flex flex-wrap items-center gap-3">
                <h2 className="eyebrow">Every held-out ruling</h2>
                <Chip tone="ok">official · Vertex AI</Chip>
                <span className="ml-auto text-[11px] text-dim">one square = one CBP ruling</span>
              </div>
              <div
                className="mt-4 grid gap-[3px]"
                style={{ gridTemplateColumns: "repeat(29, minmax(0, 1fr))" }}
                role="img"
                aria-label={`${n} rulings: ${autoRight} auto-accepted and right, ${autoWrong} auto-accepted and wrong, ${reviewRight} sent to review and right, ${reviewWrong} sent to review and wrong.`}
              >
                {squares.map((k, i) => (
                  <i key={i} className={cx("aspect-square", sq[k])} />
                ))}
              </div>
              <ul className="mt-4 grid gap-x-6 gap-y-2 text-xs text-body sm:grid-cols-2">
                {(
                  [
                    ["ar", `auto-accepted, right · ${autoRight}`],
                    ["aw", `auto-accepted, wrong · ${autoWrong}`],
                    ["rr", `sent to review, right · ${reviewRight}`],
                    ["rw", `sent to review, wrong · ${reviewWrong}`],
                  ] as const
                ).map(([k, l]) => (
                  <li key={k} className="flex items-center gap-2">
                    <i className={cx("inline-block size-2.5", sq[k])} aria-hidden="true" />
                    {l}
                  </li>
                ))}
              </ul>
            </section>
            <div className="flex flex-col gap-5 max-lg:order-first">
              <Card title="Overall" big={pct(acc.value)}>
                {right} of {n} right ({ci(acc)}).
              </Card>
              <Card title="Auto-accepted (calibrated ≥ 0.90)" big={pct(fpAcc?.value)}>
                right, on {pct(fpCov?.value)} of cases ({ci(fpAcc)}). This is the auto-accept bar on every Result screen. It takes the same cases as a stated confidence of 0.92.
              </Card>
              <Card title="Sent to review" big={String(review)}>
                cases, {reviewRight} right ({pct(reviewRight / Math.max(review, 1), 0)}, against {pct(fpAcc?.value, 0)} when it&apos;s sure), so a person checks them.
              </Card>
            </div>
          </div>
        </>
      )}
      </div>
      </div>

      {acc && (
        <>

          <section className="mt-6 border border-edge bg-sheet px-5 py-4 shadow-[4px_4px_0_var(--shadow)]">
            <div className="flex flex-wrap items-center gap-3">
              <h2 className="eyebrow">Against rules of thumb</h2>
              <span className="ml-auto text-[11px] text-dim">same {n} rulings · 95% interval · recomputed with no model (go run ./cmd/eval baselines) · * reported, unverified: not reproducible from this repository</span>
            </div>
            <div className="mt-4 flex flex-col gap-2.5">
              {bars.map(({ label, it, ours }) => (
                <div key={label} className="grid items-center gap-x-4 gap-y-1 text-[12.5px] sm:grid-cols-[230px_minmax(0,1fr)_130px]">
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
                    <b className={cx("font-medium", !ours && "font-normal")}>{pct(it!.value)}</b> <span className="text-dim">({ci(it)})</span>
                  </span>
                </div>
              ))}
              <div className="grid gap-x-4 text-[11px] text-dimension sm:grid-cols-[230px_minmax(0,1fr)_130px]">
                <span />
                <span className="flex justify-between">
                  {["0%", "25%", "50%", "75%", "100%"].map((t) => (
                    <span key={t}>{t}</span>
                  ))}
                </span>
              </div>
            </div>
          </section>

          <div className="mt-6 grid gap-5 md:grid-cols-3">
            <Card title="Cost per prediction" big={cost ? `$${cost.value}` : "—"}>
              promo; ${LIST_PRICE} list. Gemini on Vertex AI.
            </Card>
            {/* The eval times one engine call; a live prediction also runs the
                router and waits on Vertex, so say both (#183). */}
            <Card title="Engine time per ruling" big={p50 ? `${p50.value?.toFixed(1)} s` : "—"}>
              median in the held-out run; {p95?.value?.toFixed(1)} s for 95%. A live prediction on this site also runs the scope check and may wait in a queue; that end-to-end time is not measured here.
            </Card>
            <Card title="Against the proxy run" big={`p = ${PROXY.p}`}>
              no difference ({pct(PROXY.from)} → {pct(PROXY.to)}, McNemar).
            </Card>
          </div>

          <p id="gateway" className="read mt-6 max-w-[90ch] scroll-mt-20 text-[13px] leading-snug text-dim">
            Sections marked &ldquo;gateway&rdquo; were run through an OpenAI-compatible gateway to the same Gemini model, not Vertex AI. On the development set the two agree
            (same engine and model, exact McNemar p = 0.58,{" "}
            <a href={PARITY_HREF} target="_blank" rel="noreferrer" className="text-ok underline underline-offset-2">
              D151
            </a>
            ).
          </p>

          {f2Acc && (
            <section className="mt-6">
              <div className="flex flex-wrap items-center gap-3">
                <h2 className="eyebrow">Rulings published after the test set</h2>
                <span className="ml-auto text-[11px] text-dim">second held-out split, Sep–Nov 2026 · frozen before scoring · same Gemini model through a gateway, not Vertex AI</span>
              </div>
              <div className="mt-3 grid gap-5 md:grid-cols-2">
                <Card title="Second held-out split" big={pct(f2Acc.value)}>
                  on {f2Acc.n} CBP rulings from 28 Sep to 13 Nov 2026 ({ci(f2Acc)}), the same engine scored once. It is small and easier for simple rules, so it shows the
                  engine holds up after its cutoff, not that it improved.
                </Card>
                {f2FpAcc && (
                  <Card title="Auto-accepted there" big={pct(f2FpAcc.value)}>
                    right, on {pct(f2FpCov?.value)} of those rulings ({ci(f2FpAcc)}).
                  </Card>
                )}
              </div>
            </section>
          )}

          {postPrec && postRec && (
            <section id="plan-checks" className="mt-6 scroll-mt-20">
              <div className="flex flex-wrap items-center gap-3">
                <h2 className="eyebrow">Plan checks on rulings published after the test set</h2>
                <span className="ml-auto text-[11px] text-dim">{postPairs} CBP flip pairs, each with a ruling from Sep–Nov 2026 · each pair frozen before scoring · same figures in 3 runs · gateway, not Vertex AI · small</span>
              </div>
              <div className="mt-3 grid gap-5 md:grid-cols-2">
                <Card title="Moves shown that were real changes" big={`${postRight} of ${postShown}`}>
                  at the 0.92 precedent-check bar ({ci0(postPrec)}). {postCtl ? `${postCtlShown} of the ${postCtl.n} pairs where CBP kept the origin were shown.` : ""} The 15 pairs of the first set plus 3 added later (D192); three runs gave the same moves (D354). With so few pairs the interval
                  is wide.
                </Card>
                <Card title="Later real changes found, starting origin given" big={pct(postRec.value, 0)}>
                  {postFound} of {postReal} pairs where CBP&apos;s later ruling changed the origin cleared the bar ({ci0(postRec)}), the same in each of 3 runs, with CBP&apos;s starting
                  origin given. The rest go to a broker as below the bar or without a supporting ruling.{calPairs ? (
                    <>
                      {" "}The precedent check below says about {pct(calEvery / calPairs, 0)} instead ({calEvery} of {calPairs}): that counts earlier pairs (the development pairs and the first 2026 pairs), and only those
                      that cleared the bar in every one of several runs, with and without the starting origin.
                    </>
                  ) : null}
                </Card>
              </div>
              <p className="duo mt-3 max-w-[80ch] text-xs text-dim">
                Revoked rulings are not cited. A ruling CBP later revoked or modified is never cited as support for a case dated after the change, and every ruling link on
                the site says &quot;superseded by&quot; when one is known (D320, D370). The corpus records 35 such changes to 34 rulings. Before the guard, 23 of 13,166 committed
                rows had shown one and 8 had cited one; the effect on accuracy has not been measured.
              </p>
            </section>
          )}

          {reps.length === 3 && repMean && (
            <section className="mt-6" aria-labelledby="acc-repeat">
              <div className="flex flex-wrap items-center gap-3">
                <h2 id="acc-repeat" className="eyebrow">
                  Run it again
                </h2>
                <span className="ml-auto text-[11px] text-dim">development set, {repMean.n} rulings · three runs, no cache · gateway, not Vertex AI{heldRepOK ? "" : " · not measured on held-out rulings"}</span>
              </div>
              <div className="mt-3 grid gap-5 md:grid-cols-2">
                <Card title="Three runs of the same engine" big={pct(repMean.value)}>
                  mean of {reps.map((r) => pct(r.value)).join(", ")} on the same {repMean.n} development rulings. The same model was called fresh each time.
                </Card>
                <Card title="Answers that changed" big={repFlips ? `${repFlips.value} of ${repFlips.n}` : "—"}>
                  rulings got a different country in one run than in another ({repFlips?.n ? pct((repFlips.value ?? 0) / repFlips.n) : "—"}). Everything else gave the same answer three times.
                </Card>
              </div>
              {heldRepOK && (
                <p className="read mt-4 max-w-[90ch] text-[13px] leading-snug text-dim">
                  On held-out rulings, two more runs of the same engine: {heldRep[0].off?.n} rulings, official {pct(heldRep[0].off?.value)} against {pct(heldRep[0].r1?.value)} and{" "}
                  {pct(heldRep[0].r2?.value)}; {heldRep[1].off?.n} later rulings, official {pct(heldRep[1].off?.value)} against {pct(heldRep[1].r1?.value)} and {pct(heldRep[1].r2?.value)}. The
                  answer changed on {heldRep[0].flips?.value} and {heldRep[1].flips?.value} rulings. Repeats through the gateway; the official 377 run is Vertex AI.
                </p>
              )}
            </section>
          )}

          {calMean && calStated && (
            <section className="mt-6">
              <div className="flex flex-wrap items-center gap-3">
                <h2 className="eyebrow">Calibrated confidence</h2>
                <span className="ml-auto text-[11px] text-dim">map fitted on the development set only · checked once on the {calMean.n} held-out rulings · no model calls</span>
              </div>
              <div className="mt-3 grid gap-5 md:grid-cols-2">
                <Card title="What the model says" big={pct(calStated.value)}>
                  average stated confidence, against {pct(acc?.value)} right. Stated confidence runs high{eceStated?.value !== undefined ? ` (calibration error ${eceStated.value.toFixed(3)} on its own bins; ${eceSameBins?.value !== undefined ? `${eceSameBins.value.toFixed(4)} on the map's bins` : "different bins from the calibrated error"})` : ""}.
                </Card>
                <Card title="What the screens show" big={pct(calMean.value)}>
                  average calibrated confidence, against the same {pct(acc?.value)}{eceCal?.value !== undefined ? ` (calibration error ${eceCal.value.toFixed(3)} on the map's bins)` : ""}. Results show this value; the textile, Part 102 and pharmaceutical rules show
                  the model&apos;s own.
                </Card>
              </div>
              <p className="read mt-3 max-w-[90ch] text-[12.5px] text-dim">The two calibration errors do not use the same bins: the map pools confidence levels, so some of the drop is bin merging. The same-bin figure above scores the stated confidence on the calibrated bins; Brier does not depend on bins.</p>
              <p className="read mt-3 max-w-[90ch] text-[12.5px] text-dim">{copy.calSteps}</p>
            </section>
          )}

          <section className="mt-6" aria-labelledby="acc-media">
            <div className="flex flex-wrap items-center gap-3">
              <h2 id="acc-media" className="eyebrow">
                Import from a photo or scan
              </h2>
              <span className="ml-auto text-[11px] text-dim">synthetic images, illustrative · measured once · gateway, not Vertex AI · small</span>
            </div>
            <div className="mt-3 grid gap-5 md:grid-cols-2">
              <Card title="Part names read" big={`${MEDIA.read} of ${MEDIA.parts}`}>
                parts on {MEDIA.images} synthetic BOM images (tables, tilted photos and scans we rendered). {MEDIA.fullyRead} of {MEDIA.images} images were read in full. The{" "}
                {MEDIA.parts - MEDIA.read} misses are names that did not match exactly, mostly a part number added to the name.
              </Card>
              <Card title="Wrong countries" big="0">
                Every part name it read came with the right country. It says nothing about real photos, scans or handwriting, which are not measured yet. Every imported row opens as a
                draft for you to check.
              </Card>
            </div>
          </section>

          {(texAcc || route1) && (
            <section className="mt-6">
              <div className="flex flex-wrap items-center gap-3">
                <h2 className="eyebrow">Beyond substantial transformation</h2>
                <span className="ml-auto text-[11px] text-dim">textile, pharmaceutical and Part 102 marking held-out, two windows · router held-out, two windows</span>
              </div>
              <div className="mt-3 grid gap-5 md:grid-cols-3">
                {texAcc && (
                  <Card title="Textile rules (19 CFR 102.21)" big={pct(texAcc.value)}>
                    on {texAcc.n} held-out 2026 textile rulings ({ci(texAcc)}). {pct(texAuto?.value)} right when sure enough to auto-accept, on {pct(texCov?.value, 0)} of them.
                    {texV2 ? ` The prompt now in use scored ${pct(texV2.value)} on the development set.` : ""}
                    {later(tex2)}
                  </Card>
                )}
                {phAcc && (
                  <Card title="Pharmaceutical rules" big={pct(phAcc.value)}>
                    on {phAcc.n} held-out 2026 pharmaceutical rulings ({ci(phAcc)}). {pct(phAuto?.value)} right at confidence 0.95 or above, on {pct(phCov?.value, 0)} of them.
                    {later(ph2)}
                    {ph2 && (ph2.n ?? 0) < 10 ? " Too few to read much into." : ""}
                  </Card>
                )}
                {usAcc && (
                  <Card title="Part 102 marking origin" big={pct(usAcc.value)}>
                    on {usAcc.n} held-out 2026 Part 102 marking rulings ({ci(usAcc)}). {pct(usAuto?.value)} right at confidence 0.95 or above, on {pct(usCov?.value, 0)} of them.
                    {later(us2)}
                  </Card>
                )}
                {allReg && (
                  <Card title={`All regimes · ${allReg.n} rulings, each once`} big={pct(allReg.value)}>
                    on {allReg.n} held-out rulings from January to November 2026 ({ci(allReg)}), all four rule sets, each ruling counted once. Runs on Vertex AI, the earlier
                    proxy and the gateway are pooled.
                  </Card>
                )}
                {route1 && (
                  <Card title="Sent to the right rules" big={pct(route1.value)}>
                    of {route1.n} held-out 2026 rulings the router sent to the engine of their legal test ({ci(route1)}); {pct(route1Spec?.value)} of the {route1Spec?.n}{" "}
                    pharmaceutical, Part 102 and textile ones.
                    {route2 && route2Spec
                      ? ` On the ${route2.n} later rulings: ${pct(route2.value)}, and ${pct(route2Spec.value)} of the ${route2Spec.n} specialist ones (${ci(route2Spec)}).`
                      : ""}
                    {routeOut2 ? ` The router before it kept only ${pct(routeOut2.value)} of later pharmaceutical and Part 102 rulings away from the general engine.` : ""}
                    {" "}Caveat: the &quot;trade programs&quot; cue in router-v4 came from reading these later misses, not from development, so this is not clean held-out evidence; without the 6 cue rulings the specialist comparison is 14/17 vs 12/17 (fixed 2, broke 0).
                  </Card>
                )}
                {routed && (
                  <Card title={`Routed product · ${routed.n} later rulings`} big={pct(routed.value)}>
                    on {routed.n} later rulings (28 Sep to 13 Nov 2026), all four rule sets, each answered by the engine the router picked ({ci(routed)}),{" "}
                    {Math.round((routed.value ?? 0) * (routed.n ?? 0))} right.
                    {routedOld ? ` With the router before it: ${Math.round((routedOld.value ?? 0) * (routedOld.n ?? 0))} of ${routedOld.n} right.` : ""} Each ruling sent straight to its own rules scores a little higher; the gap is the cost
                    of routing. Caveat: the &quot;trade programs&quot; cue in router-v4 came from reading these later misses; without the 6 cue rulings router-v4 is 90/101, below router-v2&apos;s 91/101, so this is not clean held-out evidence.
                  </Card>
                )}
              </div>
            </section>
          )}

          {data?.verifier_calibration?.splits?.length ? <VerifierCalibration c={data.verifier_calibration} /> : null}

          <KnownFailures />

          <section className="mt-6 border border-edge bg-sheet px-5 py-4 shadow-[4px_4px_0_var(--shadow)]">
            <h2 className="eyebrow">What we don&apos;t claim</h2>
            <ul className="read mt-3 flex list-disc flex-col gap-2 pl-5 text-[14px]">
              {change && (
                <li>
                  Spotting that a change flips the origin, from scratch: {pct(change.value, 0)} ({ci0(change)}) on {change.n} pairs, against {pct(changeBase?.value)} for always
                  saying &ldquo;it changes&rdquo;. Weak; shown only with both baselines.
                </li>
              )}
              {movePrec && (
                <li>
                  Move checks on the Plan step: {movePrec.n} of {movePrec.n} flagged moves were right ({ci0(movePrec)}) on a {get("verifier_unknown-a_cost_usd")?.n ?? 54}-pair
                  development set, finding {pct(moveRec?.value, 0)} of real flips. On the 2026 pairs every precedent-supported check was a real change too (12 of 12 in the first
                  scored run; 23 of 23 across all four runs in the precedent-check table), but part of the check&apos;s prompt was written after the research team saw those
                  pairs, so it isn&apos;t a clean held-out test.
                  {postRec ? ` The clean test on ${postReal + (postCtl?.n ?? 0)} later pairs is small; it is not a measure of plan quality.` : ""} The three recall figures on this
                  page count different things: the {pct(moveRec?.value, 0)} here is one development run, each pair once; &ldquo;about {pairEvery}&rdquo; in the precedent-check section is
                  real changes that cleared the bar in every run;{postRec ? ` the ${pct(postRec.value, 0)} is the later pairs, with CBP's starting origin given.` : ""}
                </li>
              )}
              <li>{copy.jurisdiction.notCovered}</li>
              <li>{copy.jurisdiction.noDetector}</li>
              <li>Not a ruling. A licensed broker confirms before anything is filed.</li>
            </ul>
          </section>

          <section className="mt-6 border border-edge bg-sheet px-5 py-4 shadow-[4px_4px_0_var(--shadow)]">
            <div className="flex flex-wrap items-center gap-3">
              <h2 className="eyebrow">Every number</h2>
              <span className="ml-auto text-[11px] text-dim">from GET /v1/eval/summary</span>
            </div>
            <ScrollX label="Every number">
              <table className="mt-3 w-full min-w-[760px] border-collapse text-[12.5px]">
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
          </section>

          <p className="duo mt-6 max-w-[90ch] text-[11.5px] text-dim">
            {data?.status} {dev ? `Development split: ${pct(dev.value)} on all ${dev.n} rulings, unadjusted` : ""}
            {devAdj ? `; ${pct(devAdj.value)} on ${devAdj.n} after the scoring adjustments.` : dev ? "." : ""} Generated {data?.generated_at?.slice(0, 10)} from GET /v1/eval/summary; every row traces to
            docs/NUMBERS.md.
          </p>
        </>
      )}
      <HowBuilt />
    </Main>
  );
}

// How the pieces fit, for anyone judging the Gen AI (#153). Model names and
// settings are the backend defaults (internal/config); the Ask agent is ADK.
const stages: { n: number; name: string; model: string; does: string }[] = [
  { n: 1, name: "Router", model: "Gemini 3.8 Flash", does: "Checks which origin rules apply. Textile, pharmaceutical and Part 102 marking rulings go to their own rules; other questions outside substantial transformation go to a person." },
  { n: 2, name: "Origin engine", model: "Gemini 4 Argon", does: "Reads the 5 most similar CBP rulings dated before the case date (BM25 over the public rulings) and predicts the origin CBP would likely assign, with its reasons." },
  { n: 3, name: "Triage", model: "calibrated 0.90 bar", does: "Maps the model's confidence to a calibrated one (fitted on the development set). At or above the bar the answer is auto-accepted; below it, it asks CBP-style questions and goes to review." },
  { n: 4, name: "Planner", model: "search", does: "Enumerates moves of operation groups between the plants you list and re-runs the engine on each." },
  { n: 5, name: "Verifier agent", model: "Gemini 4 Argon", does: "For every move, searches for an earlier ruling that supports it and one that contradicts it. Only moves it supports past the bar are shown as plans." },
  { n: 6, name: "Ask agent", model: "Gemini · ADK", does: "Answers questions about the case with tools to read the case and rulings, and re-runs the engine for what-ifs." },
];

function HowBuilt() {
  return (
    <section id="how-its-built" aria-labelledby="how-h" className="mt-10 border border-edge bg-sheet px-5 py-5 shadow-[4px_4px_0_var(--shadow)]">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <h2 id="how-h" className="eyebrow">
          How it&apos;s built
        </h2>
        <span className="text-[11.5px] text-dim">Gemini on Vertex AI · Go backend on Cloud Run · Next.js frontend on Cloud Run</span>
      </div>
      <ol className="mt-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {stages.map((s) => (
          <li key={s.n} className="relative border border-line bg-white px-4 py-3">
            <p className="flex items-center gap-2 text-[13px] font-medium text-ink">
              <span aria-hidden="true" className="inline-grid size-5 place-items-center rounded-full border border-ink text-[11px]">
                {s.n}
              </span>
              {s.name}
              <span className="ml-auto text-[11px] font-normal text-ok">{s.model}</span>
            </p>
            <p className="read mt-1.5 text-[13.5px] text-body">{s.does}</p>
          </li>
        ))}
      </ol>
      <p className="read mt-4 max-w-[95ch] text-[13.5px] text-dim">
        Product facts in, a broker-ready plan out: every answer cites the rulings it read, nothing reads rulings dated after the case, and a daily spend cap keeps model costs
        bounded. The engine, router and verifier are measured above. The held-out rulings were never used to tune prompts or retrieval; the general engine&apos;s auto-accept bar is fitted on development rulings, while the textile, pharmaceutical and Part 102 bars came from their first held-out windows.
      </p>
      <p className="read mt-3 text-[13.5px]">
        <Link href="/method" className="text-ok underline underline-offset-2">
          How it&apos;s built and tested →
        </Link>{" "}
        <span className="text-dim">what the model is, what it reads, how each setting was chosen, how the test sets are kept clean and what did not work.</span>
      </p>
    </section>
  );
}
