"use client";

import { useEffect, useState } from "react";
import { api, type EvalSummary } from "@/lib/api/client";
import { ErrorBox, Main } from "../chrome";
import { Chip, ScrollX, Spinner, cx } from "../ui";

// "How accurate is it?" (DESIGN.md, locked 16 Oct, frame p-accuracy.html):
// every held-out ruling as a square beside the three headline cards, then
// rules of thumb, cost and time, what we don't claim, and every number.
// All figures come from GET /v1/eval/summary except the two marked below,
// which docs/NUMBERS.md records and the endpoint doesn't carry.

type Item = NonNullable<EvalSummary["items"]>[number];

const LIST_PRICE = 0.0157; // docs/NUMBERS.md: list price per prediction
const PROXY = { from: 0.899, to: 0.902, p: "1.0" }; // docs/NUMBERS.md: proxy → Vertex, McNemar

const pct = (v?: number, d = 1) => (v === undefined ? "—" : `${(v * 100).toFixed(d)}%`);
const ci0 = (it?: Item) => (it?.ci_low !== undefined && it?.ci_high !== undefined ? `${Math.round(it.ci_low * 100)}–${Math.round(it.ci_high * 100)}` : "");
const ci = (it?: Item) => (it?.ci_low !== undefined && it?.ci_high !== undefined ? `${(it.ci_low * 100).toFixed(1)}–${(it.ci_high * 100).toFixed(1)}` : "");

function Card({ title, big, children, className }: { title: string; big: string; children: React.ReactNode; className?: string }) {
  return (
    <section className={cx("border border-edge bg-sheet px-5 py-4 shadow-[4px_4px_0_var(--shadow)]", className)}>
      <h2 className="eyebrow">{title}</h2>
      <p className="mt-2 text-[34px] leading-none tracking-[-0.05em]">{big}</p>
      <p className="duo mt-2.5 text-xs text-dim">{children}</p>
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
  const change = get("change_detection_heldout");
  const changeBase = get("change_detection_always_change_baseline");
  const movePrec = get("verifier_unknown-a_precision");
  // Other regimes (D51, D53, D60): textile held-out and the router's routing.
  const texAcc = get("textile_final_accuracy");
  const texAuto = get("textile_final_conf95_accuracy");
  const texCov = get("textile_final_conf95_coverage");
  const texV2 = get("textile_v2_dev_accuracy");
  const routeOut = get("router_out_of_scope_recall");
  const routeTex = get("router_textile_routing");
  const routeFalse = get("router_false_out_of_scope");
  const moveRec = get("verifier_unknown-a_recall");

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
    { label: "Origin Proof (Gemini, Vertex AI)", it: acc, ours: true },
    { label: "Most-mentioned country", it: get("baseline_most_mentioned") },
    { label: "Country of final assembly", it: get("baseline_final_assembly") },
    { label: "Last-mentioned country", it: get("baseline_last_mentioned") },
    { label: "Always the most common origin", it: get("baseline_majority") },
  ].filter((b) => b.it);

  const status = (it: Item): { label: string; tone?: "ok" | "ask" } =>
    it.key?.startsWith("baseline")
      ? { label: "baseline, no model" }
      : it.key?.startsWith("change_detection")
        ? { label: "weak" }
        : it.development_only
          ? { label: "development set", tone: "ask" }
          : { label: "official", tone: "ok" };
  const lead = ["final_accuracy", "final_fastpath_coverage", "final_fastpath_accuracy", "baseline_most_mentioned", "dev_accuracy", "final_cost_per_case_usd", "final_latency_p50_s", "textile_final_accuracy", "router_out_of_scope_recall", "change_detection_heldout", "verifier_unknown-a_precision"];
  const rows = all ? items : lead.map(get).filter((x): x is Item => Boolean(x));
  const value = (it: Item) =>
    it.key?.endsWith("_usd") ? `$${it.value?.toFixed(4).replace(/0+$/, "")}` : it.key?.endsWith("_s") ? `${it.value?.toFixed(1)} s` : pct(it.value);

  return (
    <Main>
      <h1 className="text-[34px] tracking-[-0.05em]">How accurate is it?</h1>
      <p className="duo mt-2 max-w-[68ch] text-[14px] text-dim">
        We gave Origin Proof {n} real CBP origin rulings from January to September 2026, none of which it had seen, and compared its answer with CBP&apos;s. Every number below has
        its interval and its source.
      </p>
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
            <div className="flex flex-col gap-5">
              <Card title="Overall" big={pct(acc.value)}>
                {right} of {n} right ({ci(acc)}).
              </Card>
              <Card title="Auto-accepted (confidence ≥ 0.92)" big={pct(fpAcc?.value)}>
                right, on {pct(fpCov?.value)} of cases ({ci(fpAcc)}). This is the 0.92 bar on every Result screen.
              </Card>
              <Card title="Sent to review" big={String(review)}>
                cases, {reviewRight} right ({pct(reviewRight / Math.max(review, 1), 0)}, against {pct(fpAcc?.value, 0)} when it&apos;s sure), so a person checks them.
              </Card>
            </div>
          </div>

          <section className="mt-6 border border-edge bg-sheet px-5 py-4 shadow-[4px_4px_0_var(--shadow)]">
            <div className="flex flex-wrap items-center gap-3">
              <h2 className="eyebrow">Against rules of thumb</h2>
              <span className="ml-auto text-[11px] text-dim">same {n} rulings · 95% interval</span>
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
              median in the held-out run; {p95?.value?.toFixed(1)} s for 95%. A live prediction on this site adds the scope check and queueing, usually under a minute.
            </Card>
            <Card title="Against the proxy run" big={`p = ${PROXY.p}`}>
              no difference ({pct(PROXY.from)} → {pct(PROXY.to)}, McNemar).
            </Card>
          </div>

          {(texAcc || routeOut) && (
            <section className="mt-6">
              <div className="flex flex-wrap items-center gap-3">
                <h2 className="eyebrow">Beyond substantial transformation</h2>
                <span className="ml-auto text-[11px] text-dim">textile held-out · router on the development window</span>
              </div>
              <div className="mt-3 grid gap-5 md:grid-cols-3">
                {texAcc && (
                  <Card title="Textile rules (19 CFR 102.21)" big={pct(texAcc.value)}>
                    on {texAcc.n} held-out 2026 textile rulings ({ci(texAcc)}). {pct(texAuto?.value)} right when sure enough to auto-accept, on {pct(texCov?.value, 0)} of them.
                    {texV2 ? ` The prompt now in use scored ${pct(texV2.value)} on the development set.` : ""}
                  </Card>
                )}
                {routeOut && (
                  <Card title="Knows what it can't answer" big={pct(routeOut.value)}>
                    of pharmaceutical and USMCA rulings kept away from the origin engine ({ci(routeOut)}, n {routeOut.n}). Those go to a person.
                  </Card>
                )}
                {routeTex && (
                  <Card title="Textiles to the textile rules" big={pct(routeTex.value)}>
                    of textile rulings routed to the textile engine ({ci(routeTex)}, n {routeTex.n}); {pct(routeFalse?.value, 0)} of ordinary cases wrongly sent out of scope.
                  </Card>
                )}
              </div>
            </section>
          )}

          <section className="mt-6 border border-edge bg-sheet px-5 py-4 shadow-[4px_4px_0_var(--shadow)]">
            <h2 className="eyebrow">What we don&apos;t claim</h2>
            <ul className="duo mt-3 flex list-disc flex-col gap-2 pl-5 text-[13px]">
              {change && (
                <li>
                  Spotting that a change flips the origin, from scratch: {pct(change.value, 0)} ({ci0(change)}) on {change.n} pairs, against {pct(changeBase?.value)} for always
                  saying &ldquo;it changes&rdquo;. Weak; shown only with both baselines.
                </li>
              )}
              {movePrec && (
                <li>
                  Move checks on the Plan step: {movePrec.n} of {movePrec.n} flagged moves were right ({ci0(movePrec)}) on a {get("verifier_unknown-a_cost_usd")?.n ?? 54}-pair
                  development set, finding {pct(moveRec?.value, 0)} of real flips. On the 2026 pairs every precedent-supported check was a real change too (12 of 12),
                  but part of the check&apos;s prompt was written after the research team saw those pairs, so it isn&apos;t a clean held-out test.
                </li>
              )}
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
                          {it.source?.split(/[/ ]/).filter((x) => x.includes(".")).pop() ?? it.source}
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
            {data?.status} {dev ? `Development split: ${pct(dev.value)} on ${dev.n}.` : ""} Generated {data?.generated_at?.slice(0, 10)} from GET /v1/eval/summary; every row traces to
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
  { n: 1, name: "Router", model: "Gemini 3.8 Flash", does: "Checks the question is a substantial-transformation case; pharma and USMCA rulings go to a person." },
  { n: 2, name: "Origin engine", model: "Gemini 4 Argon", does: "Reads the 5 most similar CBP rulings dated before the case date (BM25 over the public rulings) and predicts the origin CBP would likely assign, with its reasons." },
  { n: 3, name: "Triage", model: "0.92 bar", does: "At or above the bar the answer is auto-accepted; below it, it asks CBP-style questions and goes to review." },
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
            <p className="duo mt-1.5 text-[12.5px] text-body">{s.does}</p>
          </li>
        ))}
      </ol>
      <p className="duo mt-4 max-w-[95ch] text-[12.5px] text-dim">
        Product facts in, a broker-ready plan out: every answer cites the rulings it read, nothing reads rulings dated after the case, and a daily spend cap keeps model costs
        bounded. The engine, router and verifier are measured above; the held-out numbers use rulings the system never saw.
      </p>
    </section>
  );
}
