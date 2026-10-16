"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { useEffect, useMemo, useRef, useState } from "react";
import { api, streamPlan, type CaseResponse, type Economics, type MoveRef, type Plan, type PlanRequest, type PlanResult, type Product } from "@/lib/api/client";
import { copy } from "@/lib/copy";
import { demoFor } from "@/lib/demo-store";
import { BAR, conf, isProxy, money } from "@/lib/format";
import { CaseShell, currentOrigin, type CaseCtx } from "../case-shell";
import { Elapsed, ErrorBox, Footer, Main, Next, NextSteps } from "../chrome";
import { hasAnyMoney } from "../money-drawer";
import { EconomicsEditor, PlantDiagram, checkWords, moveKey, movePhrase, rejectionLabels } from "../plan-bits";
import { planTarget } from "../plan-target";
import { Button, ButtonLink, Chip, Country, Dimension, Drawer, Flag, Fold, MiniRuler, Panel, PanelHead, RulingLink, Ruler, cx } from "../ui";

export function PlanStep() {
  return <CaseShell tab="plan">{(ctx) => <PlanScreen {...ctx} />}</CaseShell>;
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
        // A case that already has money (edited on step 1) keeps it. The demo
        // fixture is only the starting point.
        const saved = c.economics;
        setEconomics(saved && hasAnyMoney(saved) ? saved : (d.plan.economics ?? { illustrative: true }));
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

  // Nothing to plan towards yet: a pasted case has no plants, or every plant
  // is in the origin country. A next step, not an error (#81, #98).
  if (!target) {
    const plants = c.product?.plants?.length ?? 0;
    return (
      <>
        <Main className="grid gap-10 lg:grid-cols-[minmax(0,1fr)_320px]">
          <div className="flex flex-col gap-3">
            <p className="eyebrow">Step 3 of 4 · Plan</p>
            <h1 className="text-xl tracking-[-0.04em]">Plan a move</h1>
            <p className="duo max-w-[72ch] text-sm text-dim">
              {plants
                ? "The plan search moves work between your plants. Every plant on this case is in the origin country, so there is nowhere to move to. Add a plant in another country."
                : "The plan search moves work between your plants, so it needs parts and plants. This case was started from a pasted description. Turn the description into parts and plants on a new case; this one stays as it is."}
            </p>
          </div>
          <NextSteps
            steps={[{ label: "Draft for the current setup", href: `/cases/${encodeURIComponent(id)}/draft` }]}
            suggestions={[{ label: "Try a what-if scenario", href: `/cases/${encodeURIComponent(id)}/scenarios` }]}
          />
        </Main>
        <Footer>
          <Next>{plants ? "Add a plant in another country." : "Turn the description into parts and plants."}</Next>
          <div className="ml-auto">
            <ButtonLink href={`/cases/new?from=${encodeURIComponent(id)}`}>{plants ? "Add a plant →" : "Add parts and plants →"}</ButtonLink>
          </div>
        </Footer>
      </>
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
        <PlanBand
          plan={lead}
          target={target}
          below={!best}
          shown={result.plans?.length ?? 0}
          // The search's own count; the "No change" baseline isn't a move.
          total={(result.stats as Record<string, number> | undefined)?.moves_searched ?? (result.plans?.length ?? 0) + (result.rejected ?? []).filter((p) => p.rejection_kind !== "current_setup").length}
        />
      ) : result ? (
        <NoneBand target={target} />
      ) : (
        <StartBand target={target} c={c} />
      )}
      <Main className="flex flex-col gap-8">
        {error ? <ErrorBox error={error} onRetry={run} demoTitle={c.display_name} /> : null}
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
        {/* #4: the decision first (band above), then the moves and why the
            best one is supported side by side; the diagram explains, so it
            comes after. Money inputs open in a drawer. */}
        {/* Top to bottom: the decision (band), every move searched, then why
            (left) beside what to do next (right); the rest folds (17 Oct). */}
        {result && <AllMoves result={result} onDraft={draft} />}
        {result && (
          <div className="grid gap-10 lg:grid-cols-[minmax(0,1fr)_320px]">
            <div className="flex min-w-0 flex-col gap-6">
              {lead && <WhySupported plan={lead} />}
              <div>
                {lead && c.product && (
                  <Fold title={`${best ? "Best plan" : "Closest move"}, by plant`} meta={best ? "moved in blue" : "below the bar"}>
                    <PlantDiagram product={c.product} plan={lead} below={!best} />
                  </Fold>
                )}
                <Fold title="Money · illustrative" meta={economics.annual_import_value_usd ? `${money(economics.annual_import_value_usd)} imports a year` : "no import value yet"}>
                  <p className="duo text-[13px]">
                    {economics.annual_import_value_usd ? `${money(economics.annual_import_value_usd)} imports a year` : "No import value yet"}
                    {Object.keys(economics.duty_rates ?? {}).length ? ` · duty ${Object.entries(economics.duty_rates ?? {}).map(([k, v]) => `${k} ${Math.round(v * 1000) / 10}%`).join(", ")}` : ""}{" "}
                    <button type="button" className="text-ok underline underline-offset-2" onClick={() => setShowMoney(true)} aria-haspopup="dialog">
                      Edit
                    </button>
                  </p>
                </Fold>
                {result.backtest ? (
                  <Fold title="Backtest against CBP's actual ruling">
                    <BacktestPanel bt={result.backtest as Record<string, unknown>} closest={best ? undefined : closest} product={c.product ?? undefined} />
                  </Fold>
                ) : null}
                {result.stats ? (
                  <Fold title="Search details" meta={`${(result.stats as Record<string, number>).moves_searched} moves searched`}>
                    <Stats stats={result.stats as Record<string, number>} proxy={isProxy(c.llm_provider)} />
                  </Fold>
                ) : null}
              </div>
            </div>
            <NextSteps
              steps={[{ label: copy.plan.compare, href: `/cases/${encodeURIComponent(id)}/compare` }]}
              suggestions={[
                // Only when the nets can't be worked out yet.
                ...(!economics.annual_import_value_usd ? [{ label: "Add money inputs to see the net per year", onClick: () => setShowMoney(true) }] : []),
                { label: "Try your own what-ifs", href: `/cases/${encodeURIComponent(id)}/scenarios` },
              ]}
            />
          </div>
        )}
        {showMoney && (
          <MoneyDrawer onClose={() => setShowMoney(false)}>
            <EconomicsEditor value={economics} onChange={setEconomics} product={c.product} countries={countries} />
            <Button
              variant="ghost"
              onClick={() => {
                setShowMoney(false);
                run();
              }}
            >
              Search again with these inputs
            </Button>
          </MoneyDrawer>
        )}
      </Main>
      <Footer>
        <Next>
          {searching
            ? "Checking each move against earlier CBP rulings. This can take a minute or two."
            : best
              ? "Draft the ruling request for your broker."
              : closest
                ? "Draft for the closest move. The letter says it's below the bar."
                : result
                  ? "No move found support. Draft for the current setup, or compare other countries."
                  : `Search moves to ${target}.`}
        </Next>
        <div className="ml-auto flex flex-wrap items-center gap-2.5">
          {searching && (
            <Button variant="ghost" onClick={() => abort.current?.abort()}>
              Cancel
            </Button>
          )}
          {best ? (
            <Button onClick={() => draft(moveKey(best))}>{copy.plan.draft}</Button>
          ) : closest ? (
            <>
              <Button onClick={() => draft(moveKey(closest))}>{copy.plan.draftClosest}</Button>
            </>
          ) : result ? (
            // Nothing cleared or came close: the letter can still go out for
            // today's setup (#84), as the Draft tab already allowed.
            <ButtonLink href={`/cases/${encodeURIComponent(id)}/draft`}>Draft for the current setup →</ButtonLink>
          ) : (
            null
          )}
        </div>
      </Footer>
    </>
  );
}

// "No change" is the baseline, not a move: "3 more moves and No change"
// keeps the count in line with "Moves searched" (#90).
function moreLabel(hidden: number, withBaseline: boolean): string {
  const moves = withBaseline ? hidden - 1 : hidden;
  if (!withBaseline) return `${moves} more ${moves === 1 ? "move" : "moves"}`;
  return moves > 0 ? `${moves} more ${moves === 1 ? "move" : "moves"} and No change` : "Show No change";
}

function Band({ children, left, right }: { children: React.ReactNode; left?: string; right?: string }) {
  return (
    <section aria-label="Plan summary" className="border-b border-dashed border-guide">
      <div className="column relative mx-auto grid max-w-[1240px] items-end gap-8 px-4 pb-8 pt-10 sm:px-10 lg:grid-cols-[minmax(0,1fr)_280px_250px] lg:gap-11">
        {left && <Dimension className="absolute left-4 top-3 sm:left-10">{left}</Dimension>}
        {right && <Dimension className="absolute right-4 top-3 sm:right-10">{right}</Dimension>}
        {children}
      </div>
    </section>
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
  return movePhrase(p);
}

function PlanBand({ plan, target, below, shown, total }: { plan: Plan; target: string; below: boolean; shown: number; total: number }) {
  const v = plan.verdict?.confidence;
  return (
    <Band left={`${below ? 0 : shown} of ${total} moves shown`} right={below ? "closest first: by precedent check" : "precedent-checked"}>
      <div>
        <p className={cx("eyebrow", below && "text-ask")}>Step 3 of 4 · {below ? copy.plan.none : copy.plan.best}</p>
        <h1 className="duo mt-1 text-[26px] font-normal leading-tight tracking-[-0.05em] sm:text-[32px]">
          {below ? "Closest: make " : "Make "}
          {sentence(plan)} in <To country={target} />
        </h1>
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
        <Ruler value={v} caption={`check ${conf(v)} · ${checkWords(v)}`} />
      </div>
    </Band>
  );
}

function NoneBand({ target }: { target: string }) {
  return (
    <Band>
      <div>
        <p className="eyebrow text-ask">Step 3 of 4 · No plan reached {target}</p>
        <h1 className="duo mt-1 text-[26px] font-normal leading-tight tracking-[-0.05em]">No move found precedent support for {target} with these plants.</h1>
      </div>
    </Band>
  );
}

function StartBand({ target, c }: { target: string; c: CaseResponse }) {
  const cur = currentOrigin(c);
  return (
    <Band>
      <div>
        <p className="eyebrow">Step 3 of 4 · Plan</p>
        <h1 className="duo mt-1 text-[26px] font-normal leading-tight tracking-[-0.05em] sm:text-[32px]">
          Which operations would have to move for the origin to become <To country={target} />?
        </h1>
        <p className="duo mt-3 text-sm text-dim">
          Today: <Country name={cur?.country ?? c.prediction?.origin_country} /> {cur?.ruling ? <>(CBP, <RulingLink id={cur.ruling} />)</> : "(likely origin)"}. Moves only go to plants your product lists.
        </p>
      </div>
    </Band>
  );
}

function SearchingBand({ target, progress }: { target: string; progress: { done: number; total: number; since: number } }) {
  return (
    <Band right="moves run in parallel">
      <div>
        <p className="eyebrow">Step 3 of 4 · {copy.plan.searching}</p>
        <h1 className="duo mt-1 text-[26px] font-normal leading-tight tracking-[-0.05em] sm:text-[32px]">
          Checking each move to <To country={target} /> against earlier CBP rulings
        </h1>
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
    <div className="relative overflow-x-auto">
      <table className="w-full min-w-[640px] border-collapse border border-edge bg-sheet text-[13px] shadow-[4px_4px_0_var(--shadow)]">
        <thead>
          <tr>
            {["Move", "Outcome", "Precedent check", "Net per year"].map((h) => (
              <th key={h} scope="col" className="border-b border-line bg-paper px-3.5 py-2.5 text-left text-[11px] font-medium uppercase tracking-[0.08em] text-dim">
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

// Why the lead move is (or nearly is) supported: flat, summarised; the
// verifier's full reasoning folds behind "Read the full check".
function WhySupported({ plan }: { plan: Plan }) {
  const v = plan.verdict ?? {};
  return (
    <section aria-label="Precedent check" className="text-[13px]">
      <h3 className="eyebrow mb-1">Why it&apos;s supported</h3>
      <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-1">
        <dt className="text-dim">Check</dt>
        <dd>
          {conf(v.confidence)} · {checkWords(v.confidence)}
        </dd>
        <dt className="text-dim">Supports</dt>
        <dd>{v.supporting_ruling ? <RulingLink id={v.supporting_ruling} /> : "none found"}</dd>
        <dt className="text-dim">Counter</dt>
        <dd>{v.counter_ruling ? <RulingLink id={v.counter_ruling} /> : "none found"}</dd>
      </dl>
      {v.reasoning && <Reasoning text={v.reasoning} />}
    </section>
  );
}

function MoneyDrawer({ onClose, children }: { onClose: () => void; children: React.ReactNode }) {
  return (
    <Drawer
      label="Money inputs"
      onClose={onClose}
      width={600}
      header={
        <>
          <b className="font-medium text-ink">◆ Money inputs</b>
          <span>illustrative — editable</span>
        </>
      }
    >
      <div className="flex flex-col gap-4 p-5">{children}</div>
    </Drawer>
  );
}

// The verifier's reasoning runs to several hundred words: show the first
// four lines and fold the rest behind "Read the full check".
function Reasoning({ text }: { text: string }) {
  const [open, setOpen] = useState(false);
  return (
    <div className="mt-3">
      <p className={cx("duo text-[13px] leading-relaxed text-body", !open && "line-clamp-4")}>{text}</p>
      {text.length > 280 && (
        <button type="button" className="mt-1.5 text-[13px] text-ok hover:underline" aria-expanded={open} onClick={() => setOpen((o) => !o)}>
          {open ? "Show less" : "Read the full check →"}
        </button>
      )}
    </div>
  );
}

function AllMoves({ result, onDraft }: { result: PlanResult; onDraft: (key: string) => void }) {
  const rejected = result.rejected ?? [];
  // Shown plans first, then the rest closest first (precedent check, then
  // net), matching the band (DESIGN.md: "every row greyed, closest first");
  // "No change" last.
  const byCheck = (a: Plan, b: Plan) => (b.verdict?.confidence ?? 0) - (a.verdict?.confidence ?? 0) || (b.net_per_year ?? 0) - (a.net_per_year ?? 0);
  const below = rejected.filter((p) => p.rejection_kind === "below_confidence_bar").sort(byCheck);
  const others = rejected.filter((p) => p.rejection_kind !== "below_confidence_bar" && p.rejection_kind !== "current_setup");
  const noChange = rejected.filter((p) => p.rejection_kind === "current_setup");
  const every = [...(result.plans ?? []), ...below, ...others, ...noChange];
  // Count moves only, as the search band does; "No change" is the baseline.
  const moves = every.length - noChange.length;
  const [open, setOpen] = useState(false);
  const all = open ? every : every.slice(0, 3);
  return (
    <section className="min-w-0">
      <h2 className="eyebrow mb-2.5">
        Moves searched · {moves} · {result.plans?.length ? "best first" : "closest first by precedent check"}
      </h2>
      <div className="relative overflow-x-auto">
        <table className="w-full min-w-[560px] border-collapse border border-line bg-sheet text-[13px]">
          <thead>
            <tr>
              {["Move", "Outcome", "Precedent check", "Net per year", ""].map((h, i) => (
                <th key={i} scope="col" className="border-b border-line bg-paper px-3.5 py-2.5 text-left text-[11px] font-medium uppercase tracking-[0.08em] text-dim">
                  {h || <span className="sr-only">Draft</span>}
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
                  <td className="duo px-3.5 py-2.5 text-ink">
                    {p.description}
                    <span className="mt-0.5 block text-xs text-dim">
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
                    </span>
                  </td>
                  <td className="px-3.5 py-2.5">{outcome(p)}</td>
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
      {every.length > 3 && (
        <button type="button" className="mt-2 text-[13px] text-ok hover:underline" aria-expanded={open} onClick={() => setOpen((o) => !o)}>
          {open ? "Show the top 3" : moreLabel(every.length - 3, noChange.length > 0)}
        </button>
      )}
      {open && result.duty_rates_note && <p className="duo mt-2 text-xs text-dim">Duty rates: {result.duty_rates_note}</p>}
    </section>
  );
}

// With no plan over the bar the backend compares CBP's change with nothing,
// so compare it with the closest move the page highlights instead.
function BacktestPanel({ bt, closest, product }: { bt: Record<string, unknown>; closest?: Plan; product?: Product }) {
  const list = (k: string) => (Array.isArray(bt[k]) ? (bt[k] as string[]) : []);
  const actual = list("actual_moved");
  const groups = new Set((closest?.move ?? []).map((m) => m.group));
  const moved = closest
    ? (product?.components ?? []).filter((c) => c.group && groups.has(c.group)).map((c) => c.name).filter((n) => actual.includes(n))
    : list("moved_by_plan");
  return (
    <div>
      <p className="duo text-[13px]">
        The manufacturer&apos;s real change moved {actual.length} parts.{" "}
        {closest ? (
          <>
            No plan cleared the bar; the closest move ({movePhrase(closest)}) moves {moved.length} of them.
          </>
        ) : (
          <>This plan moves {moved.length} of them.</>
        )}
      </p>
      <div className="mt-2 flex flex-wrap gap-1.5">
        {actual.map((a) => (
          <span key={a} className={cx("border px-2 py-0.5 text-[11.5px]", moved.includes(a) ? "border-ok text-ok" : "border-line")}>
            {a}
          </span>
        ))}
      </div>
      {typeof bt.source === "string" ? <p className="mt-2 text-xs text-dim">Source: {bt.source}</p> : null}
    </div>
  );
}

function Stats({ stats, proxy }: { stats: Record<string, number>; proxy: boolean }) {
  return (
    <span className="text-[11px] text-dim">
      {stats.moves_searched} moves searched · {stats.engine_calls} engine calls · {Math.round(stats.wall_seconds ?? 0)} s{proxy ? ` · ${copy.proxy}` : ""}
    </span>
  );
}
