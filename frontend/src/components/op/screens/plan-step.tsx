"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { useEffect, useMemo, useRef, useState } from "react";
import { api, streamPlan, type CaseResponse, type Economics, type MoveRef, type Plan, type PlanRequest, type PlanResult } from "@/lib/api/client";
import { copy } from "@/lib/copy";
import { demoFor } from "@/lib/demo-store";
import { BAR, conf, isProxy, money } from "@/lib/format";
import { CaseShell, currentOrigin, type CaseCtx } from "../case-shell";
import { Elapsed, ErrorBox, Footer, Main } from "../chrome";
import { EconomicsEditor, PlantDiagram, moveKey, rejectionLabels } from "../plan-bits";
import { planTarget } from "../plan-target";
import { Button, ButtonLink, Chip, Country, Dimension, Flag, MiniRuler, Panel, PanelHead, RulingLink, Ruler, cx } from "../ui";

export function PlanStep() {
  return <CaseShell step={3}>{(ctx) => <PlanScreen {...ctx} />}</CaseShell>;
}

type Row = { key: string; description: string; plan?: Plan; started?: boolean };

function PlanScreen({ id, c, setCase }: CaseCtx) {
  const params = useSearchParams();
  const router = useRouter();
  const target = params.get("target") ?? planTarget(c) ?? "";
  const [req, setReq] = useState<PlanRequest | null>(null);
  const [economics, setEconomics] = useState<Economics>({});
  const [result, setResult] = useState<PlanResult | null>(c.plan && c.plan.target === target ? c.plan : null);
  const [rows, setRows] = useState<Row[]>([]);
  const [baseline, setBaseline] = useState<Plan | null>(null);
  const [progress, setProgress] = useState<{ done: number; total: number; since: number } | null>(null);
  const [lit, setLit] = useState<string | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [showMoney, setShowMoney] = useState(false);
  const abort = useRef<AbortController | null>(null);

  // A demo case reuses the demo's plan request (target, illustrative
  // economics, backtest); otherwise economics start from the case's latest.
  useEffect(() => {
    const demo = demoFor(id);
    const fallback = () => {
      setReq({ target });
      setEconomics(c.economics ?? { illustrative: true });
    };
    if (!demo) return fallback();
    api.demo(demo).then((d) => {
      if (d.plan && (!target || d.plan.target === target)) {
        setReq(d.plan);
        setEconomics(d.plan.economics ?? c.economics ?? { illustrative: true });
      } else fallback();
    }, fallback);
  }, [id, target, c.economics]);

  useEffect(() => () => abort.current?.abort(), []);

  const run = async () => {
    if (!req) return;
    abort.current?.abort();
    abort.current = new AbortController();
    setError(null);
    setResult(null);
    setRows([]);
    setBaseline(null);
    setProgress({ done: 0, total: 0, since: Date.now() });
    try {
      await streamPlan(
        id,
        { ...req, target, economics },
        (e) => {
          switch (e.event) {
            case "start":
              setRows(e.data.moves.map((m: MoveRef) => ({ key: m.key, description: m.description })));
              setProgress((p) => p && { ...p, total: e.data.total });
              break;
            case "baseline":
              setBaseline(e.data.plan);
              break;
            case "move_started":
              setRows((rs) => rs.map((r) => (r.key === e.data.key ? { ...r, started: true } : r)));
              break;
            case "move_done":
              setRows((rs) => rs.map((r) => (r.key === e.data.key ? { ...r, plan: e.data.plan } : r)));
              setProgress((p) => p && { ...p, done: e.data.done, total: e.data.total });
              setLit(e.data.key);
              break;
            case "done":
              if (e.data.plan) {
                setResult(e.data.plan);
                setCase({ ...c, plan: e.data.plan, step: c.step === "export" ? "export" : "plan", economics });
              }
              setProgress(null);
              break;
            case "error":
              setError(Object.assign(new Error(e.data.message), { retryable: e.data.retryable, code: e.data.code }));
              setProgress(null);
              break;
          }
        },
        abort.current.signal,
      );
    } catch (e) {
      if (!(e instanceof DOMException && e.name === "AbortError")) setError(e);
      setProgress(null);
    }
  };

  const countries = useMemo(() => {
    const s = new Set<string>();
    for (const p of c.product?.plants ?? []) if (p.country) s.add(p.country);
    if (c.prediction?.origin_country) s.add(c.prediction.origin_country);
    if (target) s.add(target);
    return [...s];
  }, [c, target]);

  const draft = (key?: string) => router.push(`/cases/${encodeURIComponent(id)}/draft${key ? `?move=${encodeURIComponent(key)}` : ""}`);

  if (!target) {
    return (
      <Main>
        <ErrorBox error={new Error("No target country: add a plant in another country on the Product step.")} />
      </Main>
    );
  }

  const searching = progress !== null;
  const best = result?.plans?.[0];
  const closest = !best ? result?.rejected?.filter((p) => p.rejection_kind === "below_confidence_bar").sort((a, b) => (b.verdict?.confidence ?? 0) - (a.verdict?.confidence ?? 0))[0] : undefined;
  const lead = best ?? closest;

  return (
    <>
      {searching ? (
        <SearchingBand target={target} progress={progress} />
      ) : result && lead ? (
        <PlanBand plan={lead} target={target} below={!best} shown={result.plans?.length ?? 0} total={(result.plans?.length ?? 0) + (result.rejected?.length ?? 0)} />
      ) : result ? (
        <NoneBand target={target} />
      ) : (
        <StartBand target={target} c={c} />
      )}
      <Main className="flex flex-col gap-8">
        {error ? <ErrorBox error={error} onRetry={run} /> : null}
        {!result && !searching && (
          <div className="flex flex-col gap-6">
            <EconomicsEditor value={economics} onChange={setEconomics} product={c.product} countries={countries} />
            <div>
              <Button onClick={run} disabled={!req}>
                Search moves to <Flag country={target} size={12} /> {target}
              </Button>
              <p className="duo mt-2 text-xs text-dim">
                Each move is predicted by the origin engine and checked against earlier CBP rulings; {copy.plan.usually}. A plan is shown when its precedent check is at least {BAR}.
              </p>
            </div>
          </div>
        )}
        {searching && <LiveTable rows={rows} baseline={baseline} lit={lit} />}
        {result && lead && c.product && (
          <div className="grid gap-8 lg:grid-cols-[minmax(0,1fr)_400px]">
            <Panel>
              <PanelHead right={best ? "moved in blue" : <span className="text-ask">below the bar</span>}>fig.2 — {best ? "best plan" : "closest move"}, by plant</PanelHead>
              <PlantDiagram product={c.product} plan={lead} below={!best} />
            </Panel>
            <CheckPanel plan={lead} />
          </div>
        )}
        {result && <AllMoves result={result} onDraft={draft} />}
        {result?.backtest ? <BacktestPanel bt={result.backtest as Record<string, unknown>} /> : null}
        {result && (
          <div>
            <button type="button" className="text-[13px] text-ok hover:underline" onClick={() => setShowMoney((s) => !s)} aria-expanded={showMoney}>
              {showMoney ? "Hide money inputs" : "Inputs are illustrative · edit costs and duty"}
            </button>
            {showMoney && (
              <div className="mt-4 flex flex-col gap-4">
                <EconomicsEditor value={economics} onChange={setEconomics} product={c.product} countries={countries} />
                <div>
                  <Button variant="ghost" onClick={run}>
                    Search again with these inputs
                  </Button>
                </div>
              </div>
            )}
          </div>
        )}
      </Main>
      <Footer>
        <ButtonLink variant="ghost" href={`/cases/${encodeURIComponent(id)}/origin`}>
          ← Back to origin
        </ButtonLink>
        {result?.stats ? <Stats stats={result.stats as Record<string, number>} proxy={isProxy(c.llm_provider)} /> : null}
        <div className="ml-auto flex flex-wrap items-center gap-2.5">
          {searching && (
            <Button variant="ghost" onClick={() => abort.current?.abort()}>
              Cancel
            </Button>
          )}
          {result && (
            <ButtonLink variant="ghost" href={`/cases/${encodeURIComponent(id)}/compare`}>
              {copy.plan.compare}
            </ButtonLink>
          )}
          {result && !best && (
            <ButtonLink variant="ghost" href={`/cases/${encodeURIComponent(id)}/scenarios`}>
              Try scenarios
            </ButtonLink>
          )}
          {best ? (
            <Button onClick={() => draft(moveKey(best))}>{copy.plan.draft}</Button>
          ) : closest ? (
            <>
              <span className="hidden text-xs text-dim sm:inline">The draft says it&apos;s below the bar</span>
              <Button onClick={() => draft(moveKey(closest))}>{copy.plan.draftClosest}</Button>
            </>
          ) : (
            <Button disabled>{copy.plan.draft}</Button>
          )}
        </div>
      </Footer>
    </>
  );
}

function Band({ children, left, right }: { children: React.ReactNode; left: string; right?: string }) {
  return (
    <div className="border-b border-dashed border-guide">
      <div className="column relative mx-auto grid max-w-[1240px] items-end gap-8 px-4 pb-8 pt-10 sm:px-10 lg:grid-cols-[minmax(0,1fr)_280px_250px] lg:gap-11">
        <Dimension className="absolute left-4 top-3 sm:left-10">{left}</Dimension>
        {right && <Dimension className="absolute right-4 top-3 sm:right-10">{right}</Dimension>}
        {children}
      </div>
    </div>
  );
}

function To({ country }: { country: string }) {
  return (
    <span className="inline-flex items-center gap-2.5 whitespace-nowrap">
      <Flag country={country} size={26} />
      {country}
    </span>
  );
}

function sentence(p: Plan) {
  const d = p.description ?? "";
  return d.replace(/^Make /, "make ").replace(/ in [A-Z][\w ]+$/, "");
}

function PlanBand({ plan, target, below, shown, total }: { plan: Plan; target: string; below: boolean; shown: number; total: number }) {
  const v = plan.verdict?.confidence;
  return (
    <Band left={below ? `fig.1 — no plan cleared the bar · 0 of ${total} moves shown` : `fig.1 — best plan · ${shown} of ${total} moves shown`} right={below ? "closest first: by precedent check" : "precedent-checked"}>
      <div>
        <p className={cx("eyebrow", below && "text-ask")}>{below ? copy.plan.none : copy.plan.best}</p>
        <p className="duo mt-1 text-[26px] leading-tight tracking-[-0.05em] sm:text-[32px]">
          {below ? "Closest: " : ""}
          {sentence(plan)} in <To country={target} />
        </p>
        <div className="mt-4 flex flex-wrap gap-2">
          {below ? <Chip tone="ask">● Below the bar · {conf(v)}</Chip> : <Chip tone="ok">{copy.plan.supported}</Chip>}
          <Chip>
            Reaches <Country name={plan.prediction?.origin_country} /> · {conf(plan.prediction?.confidence)}
          </Chip>
        </div>
      </div>
      <div>
        <p className="eyebrow">Net per year · illustrative</p>
        <p className={cx("mt-1 text-[44px] leading-none tracking-[-0.05em]", below && "text-dim")}>{money(plan.net_per_year)}</p>
        <p className="mt-2 text-xs text-dim">
          {below && (
            <>
              if a broker confirms it
              <br />
            </>
          )}
          {money(plan.duty_effect_per_year)} duty saved − {money(plan.added_cost_per_year)} added cost
        </p>
      </div>
      <div>
        <p className="eyebrow">Precedent check</p>
        <Ruler value={v} caption={`check confidence ${conf(v)}${below ? " · below the bar" : ""}`} />
      </div>
    </Band>
  );
}

function NoneBand({ target }: { target: string }) {
  return (
    <Band left="fig.1 — no supported plan">
      <div>
        <p className="eyebrow text-ask">No plan reached {target}</p>
        <p className="duo mt-1 text-[26px] leading-tight tracking-[-0.05em]">No move found precedent support for {target} with these plants.</p>
      </div>
    </Band>
  );
}

function StartBand({ target, c }: { target: string; c: CaseResponse }) {
  const cur = currentOrigin(c);
  return (
    <Band left="fig.1 — plan a move">
      <div>
        <p className="eyebrow">Step 3 · Plan</p>
        <p className="duo mt-1 text-[26px] leading-tight tracking-[-0.05em] sm:text-[32px]">
          Which operations would have to move for the origin to become <To country={target} />?
        </p>
        <p className="duo mt-3 text-sm text-dim">
          Today: <Country name={cur?.country ?? c.prediction?.origin_country} /> {cur?.ruling ? <>(CBP, <RulingLink id={cur.ruling} />)</> : "(likely origin)"}. Moves only go to plants your product lists.
        </p>
      </div>
    </Band>
  );
}

function SearchingBand({ target, progress }: { target: string; progress: { done: number; total: number; since: number } }) {
  return (
    <Band left="fig.1 — searching" right="moves run in parallel">
      <div>
        <p className="eyebrow">{copy.plan.searching}</p>
        <p className="duo mt-1 text-[26px] leading-tight tracking-[-0.05em] sm:text-[32px]">
          Checking each move to <To country={target} /> against earlier CBP rulings
        </p>
      </div>
      <div>
        <p className="eyebrow">Elapsed</p>
        <p className="mt-1 text-[44px] leading-none tracking-[-0.05em]">
          <Elapsed since={progress.since} />
        </p>
        <p className="mt-2 text-xs text-dim">{copy.plan.usually}</p>
      </div>
      <div>
        <p className="eyebrow">
          {progress.done} of {progress.total || "…"} moves checked
        </p>
        <div className="relative mt-3 h-1 bg-wash" role="progressbar" aria-valuemin={0} aria-valuemax={progress.total || 1} aria-valuenow={progress.done}>
          <i className="absolute inset-y-0 left-0 bg-ok transition-[width]" style={{ width: `${progress.total ? (progress.done / progress.total) * 100 : 3}%` }} />
        </div>
      </div>
    </Band>
  );
}

function outcome(p: Plan | undefined) {
  if (!p) return null;
  if (p.shown) return <Chip tone="ok">Plan</Chip>;
  const k = p.rejection_kind ?? "";
  return <Chip tone={k === "below_confidence_bar" ? "ask" : undefined}>{rejectionLabels[k] ?? k}</Chip>;
}

function LiveTable({ rows, baseline, lit }: { rows: Row[]; baseline: Plan | null; lit: string | null }) {
  return (
    <div className="overflow-x-auto">
      <table className="w-full min-w-[640px] border-collapse border border-edge bg-sheet text-[13px] shadow-[4px_4px_0_var(--shadow)]">
        <thead>
          <tr>
            {["Move", "Outcome", "Precedent check", "Net per year"].map((h) => (
              <th key={h} scope="col" className="border-b border-line bg-paper px-3.5 py-2.5 text-left text-[10.5px] font-medium uppercase tracking-[0.08em] text-dim">
                {h}
              </th>
            ))}
          </tr>
        </thead>
        <tbody aria-live="polite">
          <tr className="border-b border-line text-dim">
            <td className="px-3.5 py-2.5">Current setup</td>
            <td className="px-3.5 py-2.5">{baseline ? <Chip>Baseline</Chip> : "…"}</td>
            <td className="px-3.5 py-2.5">—</td>
            <td className="px-3.5 py-2.5">{baseline ? "$0" : ""}</td>
          </tr>
          {rows.map((r) => (
            <tr key={r.key} className={cx("border-b border-line last:border-0 transition-colors", lit === r.key && "bg-ok-wash")}>
              <td className="duo px-3.5 py-2.5">{r.description}</td>
              <td className="px-3.5 py-2.5">{r.plan ? outcome(r.plan) : <span className="text-dim">{r.started ? "Checking precedent…" : "Queued"}</span>}</td>
              <td className="px-3.5 py-2.5">{r.plan ? <MiniRuler value={r.plan.verdict?.confidence} /> : null}</td>
              <td className="px-3.5 py-2.5">{r.plan ? money(r.plan.net_per_year) : ""}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function CheckPanel({ plan }: { plan: Plan }) {
  const v = plan.verdict ?? {};
  return (
    <Panel>
      <PanelHead right={`verifier · ${conf(v.confidence)} · bar ${BAR}`}>Precedent check</PanelHead>
      <div className="border-b border-line py-2.5 text-[13px]">
        <p className="text-[10.5px] font-medium uppercase tracking-[0.06em] text-ok">Supports</p>
        {v.supporting_ruling ? <RulingLink id={v.supporting_ruling} /> : <span className="text-dim">none found</span>}
      </div>
      <div className="border-b border-line py-2.5 text-[13px]">
        <p className="text-[10.5px] font-medium uppercase tracking-[0.06em] text-ask">Counter-ruling · distinguish it</p>
        {v.counter_ruling ? <RulingLink id={v.counter_ruling} /> : <span className="text-dim">none found</span>}
      </div>
      {v.reasoning && <p className="duo mt-3 text-[13px] leading-relaxed text-[#3d3c37]">{v.reasoning}</p>}
    </Panel>
  );
}

function AllMoves({ result, onDraft }: { result: PlanResult; onDraft: (key: string) => void }) {
  const all = [...(result.plans ?? []), ...(result.rejected ?? [])];
  return (
    <section>
      <h2 className="eyebrow mb-2.5">Every move searched · best first</h2>
      <div className="overflow-x-auto">
        <table className="w-full min-w-[760px] border-collapse border border-edge bg-sheet text-[13px] shadow-[4px_4px_0_var(--shadow)]">
          <thead>
            <tr>
              {["Move", "Outcome", "Why", "Precedent check", "Net per year", ""].map((h, i) => (
                <th key={i} scope="col" className="border-b border-line bg-paper px-3.5 py-2.5 text-left text-[10.5px] font-medium uppercase tracking-[0.08em] text-dim">
                  {h}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {all.map((p) => {
              const key = moveKey(p);
              const grey = !p.shown;
              return (
                <tr key={key || "baseline"} className={cx("border-b border-line last:border-0", grey && "text-dim")}>
                  <td className="duo px-3.5 py-2.5 text-ink">{p.description}</td>
                  <td className="px-3.5 py-2.5">{outcome(p)}</td>
                  <td className="duo max-w-[34ch] px-3.5 py-2.5 text-xs">
                    {p.shown ? (
                      <>
                        Supported by {p.verdict?.supporting_ruling ? <RulingLink id={p.verdict.supporting_ruling} /> : "an earlier ruling"}
                        {p.verdict?.counter_ruling ? (
                          <>
                            ; counter <RulingLink id={p.verdict.counter_ruling} />
                          </>
                        ) : null}
                      </>
                    ) : (
                      p.rejection_reason
                    )}
                  </td>
                  <td className="px-3.5 py-2.5">
                    <MiniRuler value={p.verdict?.confidence} />
                  </td>
                  <td className="whitespace-nowrap px-3.5 py-2.5">{money(p.net_per_year)}</td>
                  <td className="px-3.5 py-2.5 text-right">
                    {(p.shown || p.rejection_kind === "below_confidence_bar") && key ? (
                      <button type="button" className="whitespace-nowrap text-xs text-ok hover:underline" onClick={() => onDraft(key)}>
                        Draft{p.shown ? "" : " (below bar)"} →
                      </button>
                    ) : null}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
      {result.duty_rates_note && <p className="duo mt-2 text-xs text-dim">Duty rates: {result.duty_rates_note}</p>}
    </section>
  );
}

function BacktestPanel({ bt }: { bt: Record<string, unknown> }) {
  const list = (k: string) => (Array.isArray(bt[k]) ? (bt[k] as string[]) : []);
  const moved = list("moved_by_plan");
  const actual = list("actual_moved");
  return (
    <Panel>
      <PanelHead right={typeof bt.source === "string" ? bt.source : undefined}>fig.3 — backtest against CBP&apos;s actual ruling</PanelHead>
      <p className="duo text-[13px]">
        The manufacturer&apos;s real change moved {actual.length} parts. This plan moves {moved.length} of them.
      </p>
      <div className="mt-2 flex flex-wrap gap-1.5">
        {actual.map((a) => (
          <span key={a} className={cx("border px-2 py-0.5 text-[11.5px]", moved.includes(a) ? "border-ok text-ok" : "border-line")}>
            {a}
          </span>
        ))}
      </div>
    </Panel>
  );
}

function Stats({ stats, proxy }: { stats: Record<string, number>; proxy: boolean }) {
  return (
    <span className="text-[11px] text-dim">
      {stats.moves_searched} moves searched · {stats.engine_calls} engine calls · {Math.round(stats.wall_seconds ?? 0)} s{proxy ? ` · ${copy.proxy}` : ""}
    </span>
  );
}
