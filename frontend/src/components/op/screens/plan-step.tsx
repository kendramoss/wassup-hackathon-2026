"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { Explained, Term } from "../term";
import { useEffect, useMemo, useRef, useState } from "react";
import { api, ApiError, opLock, opsActive, saveEconomics, streamPlan, withStaleFlags, type CaseResponse, type Economics, type MoveRef, type Plan, type PlanEstimate, type PlanRequest, type PlanResult, type Product } from "@/lib/api/client";
import { VIEWER_REASON, caseReadOnly, writeGate } from "@/lib/roles";
import { copy } from "@/lib/copy";
import { demoFor } from "@/lib/demo-store";
import { BAR, conf, isProxy, money, originShort } from "@/lib/format";
import { decisionHref } from "@/lib/method";
import { useStepLabel } from "../demo-tour";
import { CaseShell, currentOrigin, type CaseCtx } from "../case-shell";
import { Elapsed, ErrorBox, Footer, Main, Next, NextSteps } from "../chrome";
import { hasAnyMoney, moneySummary } from "../money-drawer";
import { OPS_UNREADABLE, OperationalFields, opsErrorText, useOpsValidity } from "../operational-fields";
import { BandNet, OpsDetail, OpsScope, StaleMoneyBanner } from "../ops-result";
import { isCostRejection, netText } from "../ops-view";
import { PrefillBar } from "../money-prefill";
import { EconomicsEditor, MoveIncludes, PlantDiagram, TierChip, agreementWords, andList, backtestView, checkWords, moveKey, movePhrase, partsMoved, rejectionLabels, smallestSupported, type ActualChangeExtras, type BacktestView } from "../plan-bits";
import { planTarget } from "../plan-target";
import { ReviewNote } from "../review-note";
import { regimeOf } from "@/lib/regime";
import { pricedEconomics } from "@/lib/live-mfn";
import { Button, ButtonLink, Chip, Country, Dimension, Drawer, Flag, Fold, MiniRuler, Panel, PanelHead, RulingLink, Ruler, cx } from "../ui";

export function PlanStep() {
  return <CaseShell tab="plan">{(ctx) => <PlanScreen {...ctx} />}</CaseShell>;
}

type Row = { key: string; description: string; plan?: Plan; started?: boolean; advice?: boolean };

function PlanScreen(ctx: CaseCtx) {
  const regime = regimeOf(ctx.c);
  if (regime) return <NoPlanForRegime id={ctx.id} label={regime.label} />;
  return <PlanSearch {...ctx} />;
}

// Textile, USMCA and pharmaceutical cases: moves aren't modelled under those
// rules (D51, D61, D62), so there's no search to run.
function NoPlanForRegime({ id, label }: { id: string; label: string }) {
  const stepNo = useStepLabel("plan", "Step 3 of 4");
  return (
    <>
      <Main className="flex flex-col gap-3">
        <p className="eyebrow">{stepNo} · Plan</p>
        <h1 className="text-xl tracking-[-0.04em]">No plan search for {label}</h1>
        <p className="duo max-w-[72ch] text-sm text-dim">
          This case was decided under {label}. The plan search models moves under substantial transformation only, so it doesn&apos;t run here. A broker can review the origin from the
          draft.
        </p>
      </Main>
      <Footer>
        <Next>Draft the ruling request for a broker to review.</Next>
        <div className="ml-auto">
          <ButtonLink href={`/cases/${encodeURIComponent(id)}/draft`}>Draft for a broker →</ButtonLink>
        </div>
      </Footer>
    </>
  );
}

function PlanSearch({ id, c, setCase }: CaseCtx) {
  const stepNo = useStepLabel("plan", "Step 3 of 4");
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
  // A failed discard-save, shown inside the open drawer (the page behind it is covered).
  const [moneyError, setMoneyError] = useState<unknown>(null);
  const abort = useRef<AbortController | null>(null);
  // The newest case this screen holds, so the save below never writes back the
  // case from when it started (same pattern as the scenarios view).
  const latest = useRef(c);
  useEffect(() => {
    latest.current = c;
  }, [c]);
  // One discard-save at a time: a second press while it is in flight is ignored.
  const lock = useRef(opLock()).current;
  // Both editors (inline and drawer) report here; an entry that cannot be read
  // blocks every run, since a run whole-replaces the stored money.
  const validity = useOpsValidity();

  // A demo case reuses the demo's plan request (target, illustrative
  // economics, backtest); otherwise economics start from the case's latest.
  // After a failed discard-save the user's inputs stay as they left them: a
  // re-read of the SAME stored money (a 403 triggers one) must not reseed them.
  const held = useRef<{ id: string; json: string } | null>(null);
  useEffect(() => {
    if (held.current && held.current.id === id && held.current.json === JSON.stringify(c.economics ?? null)) return;
    held.current = null;
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

  // The demo's record of the real change (which parts were already in the
  // target, and which CBP's change kept in place). A case loaded in this
  // browser has it in its plan request; the pinned demo case doesn't, so it is
  // found by the backtest's source among the demos (D310). Read-only GETs.
  const [demoChange, setDemoChange] = useState<ActualChangeExtras | null>(null);
  const btSource = (result?.backtest as { source?: unknown } | undefined)?.source;
  const reqHasChange = Boolean(req?.actual_change);
  useEffect(() => {
    if (reqHasChange || typeof btSource !== "string") return;
    let live = true;
    api.demos().then(
      async (list) => {
        for (const d of list.demos ?? []) {
          if (!live || !d.has_plan || !d.id) continue;
          const full = await api.demo(d.id).catch(() => null);
          const ac = full?.plan?.actual_change as (ActualChangeExtras & { _source?: string }) | undefined;
          if (live && ac?._source === btSource) return setDemoChange(ac);
        }
      },
      () => {},
    );
    return () => {
      live = false;
    };
  }, [btSource, reqHasChange]);

  // What the search will cost and take, before it runs (D87). It reads the
  // caches and calls no model; on any error the usual sentence stays.
  const [estimate, setEstimate] = useState<PlanEstimate | null>(null);
  useEffect(() => {
    if (!req || !target) return;
    const ctl = new AbortController();
    api.planEstimate(id, { ...req, target }, ctl.signal).then(
      (r) => setEstimate(r.estimate ?? null),
      () => setEstimate(null),
    );
    return () => ctl.abort();
  }, [id, req, target]);

  // A plan POST with annual import value 0 and no operations means "use the
  // saved money" (kept on purpose for legacy clients), so when the case STORES
  // active operating inputs and the user has since switched them off, the
  // search would quietly run in operations mode and keep them. Only then is the
  // whole current money saved first with the existing PUT /economics (the
  // server stores the body whole, so the operations go). Positive annual,
  // unchanged operations, legacy cases, and read-only or pinned cases never
  // take this path. Atomicity: the confirmed discard is saved before the
  // search; if the search then fails, the cleared inputs stay saved.
  const mustClearStored = (e: Economics) =>
    !c.pinned && !demoFor(id) && !caseReadOnly(c) && opsActive(c.economics?.operations) && !opsActive(e.operations) && !((e.annual_import_value_usd ?? 0) > 0);

  const run = (): Promise<void> => go(false);

  const go = async (fromDrawer: boolean) => {
    if (!req) return;
    if (validity.get()) {
      const e = new ApiError(OPS_UNREADABLE, 0, "invalid_input", false);
      setError(e);
      if (fromDrawer) setMoneyError(e);
      return;
    }
    const sent = economics;
    if (mustClearStored(sent)) {
      const release = lock.start();
      if (!release) return;
      const ctl = new AbortController();
      abort.current?.abort();
      abort.current = ctl;
      setError(null);
      setMoneyError(null);
      try {
        await saveEconomics(id, sent, {
          latest: () => latest.current,
          apply: (next) => {
            latest.current = next;
            setCase(next);
          },
        });
      } catch (e) {
        if (latest.current.id === id && !ctl.signal.aborted) {
          held.current = { id, json: JSON.stringify(c.economics ?? null) };
          setError(e);
          if (fromDrawer) setMoneyError(e);
          else setShowMoney(false);
        }
        return;
      } finally {
        release();
      }
      // The screen moved to another case, or was left, while the save ran.
      if (latest.current.id !== id || ctl.signal.aborted) return;
    }
    if (fromDrawer) setShowMoney(false);
    abort.current?.abort();
    abort.current = new AbortController();
    setError(null);
    setResult(null);
    setRows([]);
    setBaseline(null);
    setProgress({ done: 0, total: 0, since: Date.now() });
    // A stream that closes without "done" or "error" was cut (a timeout or a
    // dropped connection): say so, with a retry, rather than spin forever.
    let finished = false;
    try {
      await streamPlan(
        id,
        { ...req, target, economics: sent },
        (e) => {
          switch (e.event) {
            case "start":
              setRows(e.data.moves.map((m: MoveRef) => ({ key: m.key, description: m.description })));
              setProgress((p) => p && { ...p, total: e.data.total });
              break;
            case "baseline":
              setBaseline(e.data.plan);
              break;
            // With FOLLOW_ADVICE on (D85) one more move can start after the
            // listed ones; it isn't in "start", so it gets a row here and the
            // total grows by one.
            case "move_started":
              setRows((rs) =>
                rs.some((r) => r.key === e.data.key)
                  ? rs.map((r) => (r.key === e.data.key ? { ...r, started: true } : r))
                  : [...rs, { key: e.data.key, description: copy.plan.advice.pending, started: true, advice: true }],
              );
              setProgress((p) => p && { ...p, done: e.data.done, total: Math.max(p.total, e.data.total) });
              break;
            case "move_done":
              setRows((rs) =>
                rs.some((r) => r.key === e.data.key)
                  ? rs.map((r) => (r.key === e.data.key ? { ...r, plan: e.data.plan, description: r.advice ? (e.data.plan.description ?? r.description) : r.description } : r))
                  : [...rs, { key: e.data.key, description: e.data.plan.description ?? copy.plan.advice.pending, plan: e.data.plan, advice: true }],
              );
              setProgress((p) => p && { ...p, done: e.data.done, total: e.data.total });
              setLit(e.data.key);
              break;
            case "done":
              finished = true;
              if (e.data.plan) {
                setResult(e.data.plan);
                const plan = e.data.plan;
                // A plan just run is fresh by construction: its stale flag goes. The
                // stored comparisons may or may not still match the money saved with
                // this run, which only the server knows, so its flags are re-read.
                setCase((d) => (d ? { ...withStaleFlags(d, { targets_economics_stale: d.targets_economics_stale }), plan, step: d.step === "export" ? "export" : "plan", economics: sent } : d));
                if (!c.pinned)
                  api.getCase(id).then(
                    (fresh) => setCase((d) => (d && d.id === fresh.id ? withStaleFlags(d, fresh) : d)),
                    () => {},
                  );
              }
              setProgress(null);
              break;
            case "error":
              finished = true;
              setError(Object.assign(new Error(e.data.message), { retryable: e.data.retryable, code: e.data.code }));
              setProgress(null);
              break;
          }
        },
        abort.current.signal,
      );
      if (!finished) {
        setError(new ApiError(copy.plan.cut, 0, "network", true));
        setProgress(null);
      }
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

  // The demo tour (?tour=1) carries on to the draft, its step 3.
  const tour = params.get("tour") === "1";
  const draftHref = (key?: string) => {
    const q = new URLSearchParams();
    if (key) q.set("move", key);
    if (tour) q.set("tour", "1");
    const qs = q.toString();
    return `/cases/${encodeURIComponent(id)}/draft${qs ? `?${qs}` : ""}`;
  };
  const draft = (key?: string) => router.push(draftHref(key));

  // Nothing to plan towards yet: a pasted case has no plants, or every plant
  // is in the origin country. A next step, not an error (#81, #98).
  if (!target) {
    const plants = c.product?.plants?.length ?? 0;
    return (
      <>
        <Main className="grid gap-10 lg:grid-cols-[minmax(0,1fr)_320px]">
          <div className="flex flex-col gap-3">
            <p className="eyebrow">{stepNo} · Plan</p>
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
  // The stored plan was priced from money that has since changed (#281): its
  // amounts are hidden, never shown as current. The legal facts stay.
  const stale = Boolean(c.plan_economics_stale) && result !== null;
  const readOnly = caseReadOnly(c);
  const opsError = opsErrorText(error);
  const demoted = (result?.rejected ?? []).filter(isCostRejection).length;
  const best = result?.plans?.[0];
  const closest = !best ? result?.rejected?.filter((p) => p.rejection_kind === "below_confidence_bar").sort((a, b) => (b.verdict?.confidence ?? 0) - (a.verdict?.confidence ?? 0))[0] : undefined;
  const lead = best ?? closest;
  // The backtest read for people (D310): parts that really changed origin,
  // which the plan moves, and what it moves that CBP's change kept in place.
  const bt = result?.backtest
    ? backtestView(result.backtest, { change: ((req?.actual_change as ActualChangeExtras | undefined) ?? demoChange), product: c.product, move: lead?.move, recount: !best })
    : undefined;
  const from = currentOrigin(c)?.country ?? c.prediction?.origin_country ?? "the old country";

  return (
    <>
      {searching ? (
        <SearchingBand target={target} progress={progress} />
      ) : result && lead ? (
        <PlanBand
          plan={lead}
          product={c.product}
          target={target}
          below={!best}
          shown={result.plans?.length ?? 0}
          stale={stale}
          demoted={stale ? 0 : demoted}
          objective={result.objective}
          result={result}
          backtest={bt}
          from={from}
          // The search's own count; the "No change" baseline isn't a move.
          total={(result.stats as Record<string, number> | undefined)?.moves_searched ?? (result.plans?.length ?? 0) + (result.rejected ?? []).filter((p) => p.rejection_kind !== "current_setup").length}
        />
      ) : result ? (
        <NoneBand target={target} demoted={demoted > 0} stale={stale} />
      ) : (
        <StartBand target={target} c={c} />
      )}
      <Main className="flex flex-col gap-8">
        {error ? <ErrorBox error={error} onRetry={run} demoTitle={c.display_name} /> : null}
        {result && !searching && stale && <StaleMoneyBanner kind="plan" onRun={run} readOnly={readOnly} busy={!req} />}
        {result && !searching && <ReviewNote place="plan" best={Boolean(best)} />}
        {!result && !searching && (
          // The action first, then the inputs it uses: money is optional and
          // illustrative, so it folds under a one-line summary (open when
          // nothing is filled in yet).
          <div className="flex flex-col gap-6">
            <div>
              <Button onClick={run} disabled={!req || caseReadOnly(c)} title={caseReadOnly(c) ? VIEWER_REASON : undefined}>
                Search moves to <Flag country={target} size={12} /> {target}
              </Button>
              <p className="duo mt-2 max-w-[80ch] text-xs text-dim">
                Each move is predicted by the origin engine and checked against earlier CBP rulings. {estimate ? estimateWords(estimate) : copy.plan.usuallySentence} A plan is shown when its{" "}
                <Term explain={copy.terms.check.text}>precedent check</Term> is at least {BAR}.
              </p>
            </div>
            <div>
              <Fold title="Money inputs · illustrative, optional" meta={moneySummary(economics) || "none yet: plans show without a net per year"} open={!hasAnyMoney(economics)}>
                <PrefillBar product={c.product} economics={economics} ready={req !== null} onFill={setEconomics} />
                <EconomicsEditor value={economics} onChange={setEconomics} product={c.product} countries={countries} priced={pricedEconomics(c)} />
                <OperationalFields value={economics} onChange={setEconomics} product={c.product} readOnly={readOnly} serverError={opsError} className="mt-4" validity={validity} />
              </Fold>
            </div>
          </div>
        )}
        {searching && <LiveTable rows={rows} baseline={baseline} lit={lit} />}
        {/* #4: the decision first (band above), then the moves and why the
            best one is supported side by side; the diagram explains, so it
            comes after. Money inputs open in a drawer. */}
        {/* Top to bottom: the decision (band), every move searched, then why
            (left) beside what to do next (right); the rest folds (17 Oct). */}
        {result && <AllMoves result={result} onDraft={draft} stale={stale} product={c.product} asOf={c.date} />}
        {result && (
          <div className="grid gap-10 lg:grid-cols-[minmax(0,1fr)_320px]">
            <div className="flex min-w-0 flex-col gap-6">
              {lead && <WhySupported plan={lead} asOf={c.date} />}
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
                    <button type="button" className="inline-flex min-h-6 items-center text-ok underline underline-offset-2" onClick={() => setShowMoney(true)} aria-haspopup="dialog">
                      Edit
                    </button>
                  </p>
                </Fold>
                {result.backtest && bt ? (
                  // Open by default (judge panel round 4): the backtest is the
                  // strongest evidence on this screen, so it is not behind a click.
                  <Fold
                    id="backtest"
                    title="Backtest against CBP's actual ruling"
                    meta={best && bt ? backtestShort(bt) : undefined}
                    open
                  >
                    {bt ? (
                      <BacktestPanel
                        v={bt}
                        source={typeof (result.backtest as Record<string, unknown>).source === "string" ? String((result.backtest as Record<string, unknown>).source) : undefined}
                        closest={best ? undefined : closest}
                        target={target}
                        from={from}
                        tour={tour}
                        next={tour && lead ? draftHref(moveKey(lead)) : undefined}
                      />
                    ) : null}
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
          <MoneyDrawer
            onClose={() => {
              setShowMoney(false);
              setMoneyError(null);
            }}
          >
            <EconomicsEditor value={economics} onChange={setEconomics} product={c.product} countries={countries} priced={pricedEconomics(c)} />
            <OperationalFields value={economics} onChange={setEconomics} product={c.product} readOnly={readOnly} serverError={opsError} validity={validity} />
            {moneyError ? <ErrorBox error={moneyError} demoTitle={c.display_name} /> : null}
            <Button variant="ghost" {...writeGate(caseReadOnly(c))} onClick={() => go(true)}>
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
          ) : !searching ? (
            // Same place as every other step's main action.
            <Button onClick={run} disabled={!req || caseReadOnly(c)} title={caseReadOnly(c) ? VIEWER_REASON : undefined}>
              Search moves to <Flag country={target} size={12} /> {target} →
            </Button>
          ) : null}
        </div>
      </Footer>
    </>
  );
}

// "No change" is the baseline, not a move: "3 more moves and the
// no-change row" keeps the count in line with "Moves searched" (#90).
function moreLabel(hidden: number, withBaseline: boolean): string {
  const moves = withBaseline ? hidden - 1 : hidden;
  if (!withBaseline) return `${moves} more ${moves === 1 ? "move" : "moves"}`;
  return moves > 0 ? `${moves} more ${moves === 1 ? "move" : "moves"} and the no-change row` : "Show the no-change row";
}

function Band({ children, left, right }: { children: React.ReactNode; left?: React.ReactNode; right?: string }) {
  return (
    <section aria-label="Plan summary" className="border-b border-dashed border-guide">
      <div className="column relative mx-auto grid max-w-[1240px] items-end gap-8 px-4 pb-8 pt-10 sm:px-10 lg:grid-cols-[minmax(0,1fr)_280px_250px] lg:gap-11">
        {left && <Dimension className="absolute left-4 top-3 sm:left-10">{left}</Dimension>}
        {/* Two labels on one line collide on a phone (judge panel round 4,
            F1): the right one is shown from sm up only. */}
        {right && <Dimension className="absolute right-4 top-3 hidden sm:right-10 sm:block">{right}</Dimension>}
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

// "7 moves · about 1–2 min · about $0.13–0.38", or, with every move
// already checked, a replay that costs nothing.
function estimateWords(e: PlanEstimate): string {
  const moves = `${e.moves ?? 0} ${e.moves === 1 ? "move" : "moves"}`;
  if ((e.model_calls?.max ?? 1) === 0) return `${moves}. ${copy.plan.estimate.cached}`;
  const lo = e.wall_seconds?.min ?? 0;
  const hi = e.wall_seconds?.max ?? lo;
  const time =
    hi < 90 ? range(Math.round(lo), Math.round(hi), " s") : range(Math.max(1, Math.round(lo / 60)), Math.max(1, Math.round(hi / 60)), " min");
  const money = (v: number) => (v < 0.01 ? "under $0.01" : `$${v.toFixed(2)}`);
  const cmin = e.cost_usd?.min ?? 0;
  const cmax = e.cost_usd?.max ?? cmin;
  const cost = money(cmin) === money(cmax) ? money(cmax) : cmin < 0.01 ? `up to ${money(cmax)}` : `$${cmin.toFixed(2)}–${cmax.toFixed(2)}`;
  return `${moves} · about ${time} · about ${cost}.`;
}

const range = (a: number, b: number, unit: string) => (a === b ? `${a}${unit}` : `${a}–${b}${unit}`);

function sentence(p: Plan) {
  return movePhrase(p);
}

function PlanBand({
  plan,
  target,
  below,
  shown,
  total,
  stale,
  demoted,
  objective,
  product,
  backtest,
  from,
  result,
}: {
  result?: PlanResult;
  plan: Plan;
  product?: Product | null;
  backtest?: BacktestView;
  from: string;
  target: string;
  below: boolean;
  shown: number;
  total: number;
  stale?: boolean;
  demoted?: number;
  objective?: PlanResult["objective"];
}) {
  const stepNo = useStepLabel("plan", "Step 3 of 4");
  const v = plan.verdict?.confidence;
  const small = smallestSupported(result?.plans ?? [], product);
  return (
    <Band
      left={
        <>
          {below ? 0 : shown} of {total} moves cleared the <Term explain={copy.terms.bar.text}>{BAR} precedent-check bar</Term>
          {demoted ? ` (${demoted} more cleared it but not your operating inputs)` : null}
        </>
      }
      right={below ? "closest first: by precedent check" : "precedent-checked"}>
      <div>
        <p className={cx("eyebrow", below && "text-ask")}>{stepNo} · {below ? copy.plan.none : copy.plan.best}</p>
        <h1 className="duo mt-1 text-[26px] font-normal leading-tight tracking-[-0.05em] sm:text-[32px]">
          {below ? "Closest: move " : "Move "}
          <Explained text={sentence(plan)} terms={["pcba"]} /> to <To country={target} />
        </h1>
        <MoveIncludes product={product} plan={plan} className="mt-2 max-w-[80ch]" />
        <div className="mt-4 flex flex-wrap gap-2">
          {below ? <Chip tone="ask">△ Below the bar · {conf(v)}</Chip> : <Chip tone="ok">{copy.plan.supported}</Chip>}
          {!below && <TierChip plan={plan} result={result} />}
          {!below && small === plan && <Chip>{copy.plan.tier.smallest}</Chip>}
          <Chip>
            Reaches <Country name={plan.prediction?.origin_country} /> · origin {originShort(plan.prediction)}
          </Chip>
        </div>
        {!below && small && small !== plan ? (
          <p className="duo mt-2 max-w-[80ch] text-xs text-dim">{copy.plan.tier.smallestLine(movePhrase(small, product?.family), partsMoved(small, product))}</p>
        ) : null}
      </div>
      <div>
        <BandNet plan={plan} below={below} stale={stale} objective={objective} />
      </div>
      <div>
        <p className="eyebrow">
          <Term explain={copy.terms.check.text}>Precedent check</Term>
        </p>
        <Ruler value={v} caption={[`check ${conf(v)}`, agreementWords(plan.verdict?.agreement), checkWords(v)].filter(Boolean).join(" · ")} />
        {!below && backtest ? (
          // The backtest beside the check (judge panel round 4): what the
          // manufacturer really changed, from CBP's later ruling. Round 5
          // (D310): count only parts that changed origin, say what the plan
          // moves beyond that, and that the test is not blind.
          <p className="duo mt-3 text-xs">
            <span className="text-body">Backtest:</span> {backtestWords(backtest, target, from)}{" "}
            <a href="#backtest" className="whitespace-nowrap text-ok underline underline-offset-2">
              See the parts ↓
            </a>
          </p>
        ) : null}
      </div>
    </Band>
  );
}

function NoneBand({ target, demoted, stale }: { target: string; demoted?: boolean; stale?: boolean }) {
  const stepNo = useStepLabel("plan", "Step 3 of 4");
  return (
    <Band>
      <div>
        <p className="eyebrow text-ask">{stepNo} · No plan reached {target}</p>
        {demoted ? (
          <h1 className="duo mt-1 text-[26px] font-normal leading-tight tracking-[-0.05em]">{stale ? copy.stale.noneBand : copy.opsResult.noneBand}</h1>
        ) : (
          <h1 className="duo mt-1 text-[26px] font-normal leading-tight tracking-[-0.05em]">No move found precedent support for {target} with these plants.</h1>
        )}
      </div>
    </Band>
  );
}

function StartBand({ target, c }: { target: string; c: CaseResponse }) {
  const stepNo = useStepLabel("plan", "Step 3 of 4");
  const cur = currentOrigin(c);
  return (
    <Band>
      <div>
        <p className="eyebrow">{stepNo} · Plan</p>
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

// After this long the search is slower than usual (the model provider is
// busy and calls are being spaced out); say so instead of a stale estimate.
const SLOW_MS = 3 * 60 * 1000;

function SearchingBand({ target, progress }: { target: string; progress: { done: number; total: number; since: number } }) {
  const stepNo = useStepLabel("plan", "Step 3 of 4");
  const [slow, setSlow] = useState(() => Date.now() - progress.since > SLOW_MS);
  useEffect(() => {
    if (slow) return;
    const t = setTimeout(() => setSlow(true), Math.max(0, progress.since + SLOW_MS - Date.now()));
    return () => clearTimeout(t);
  }, [slow, progress.since]);
  return (
    <Band right="moves run in parallel">
      <div>
        <p className="eyebrow">{stepNo} · {copy.plan.searching}</p>
        <h1 className="duo mt-1 text-[26px] font-normal leading-tight tracking-[-0.05em] sm:text-[32px]">
          Checking each move to <To country={target} /> against earlier CBP rulings
        </h1>
      </div>
      <div>
        <p className="eyebrow">Elapsed</p>
        <p className="mt-1 text-[44px] leading-none tracking-[-0.05em]">
          <Elapsed since={progress.since} />
        </p>
        <p role="status" className={cx("duo mt-2 max-w-[34ch] text-xs", slow ? "text-ask" : "text-dim")}>
          {slow ? copy.plan.slow : copy.plan.usually}
        </p>
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
  return <Chip tone={k === "below_confidence_bar" ? "ask" : undefined}>{k === "below_confidence_bar" ? "△ " : ""}{rejectionLabels[k] ?? k}</Chip>;
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
              <td className="duo px-3.5 py-2.5">
                {r.description}
                {(r.advice || r.plan?.from_advice) && <span className="mt-0.5 block text-xs text-ask">{copy.plan.advice.tried}</span>}
              </td>
              <td className="px-3.5 py-2.5">{r.plan ? outcome(r.plan) : <span className="text-dim">{r.started ? "Checking precedent…" : "Queued"}</span>}</td>
              <td className="px-3.5 py-2.5">{r.plan ? <MiniRuler value={r.plan.verdict?.confidence} /> : null}</td>
              <td className="px-3.5 py-2.5">{r.plan ? netText(r.plan) : ""}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

// Why the lead move is (or nearly is) supported: flat, summarised; the
// verifier's full reasoning folds behind "Read the full check".
function WhySupported({ plan, asOf }: { plan: Plan; asOf?: string | null }) {
  const v = plan.verdict ?? {};
  return (
    <section aria-label="Precedent check" className="text-[13px]">
      <h3 className="eyebrow mb-1">{plan.shown ? copy.plan.whySupported : copy.plan.closestEvidence}</h3>
      <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-1">
        <dt className="text-dim">Check</dt>
        <dd>
          {[conf(v.confidence), agreementWords(v.agreement), checkWords(v.confidence)].filter(Boolean).join(" · ")}
        </dd>
        <dt className="text-dim">Supports</dt>
        <dd>{v.supporting_ruling ? <RulingLink id={v.supporting_ruling} asOf={asOf} support /> : "none found"}</dd>
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
      <p className={cx("read text-[14px] leading-relaxed text-body", !open && "line-clamp-4")}>{text}</p>
      {text.length > 280 && (
        <button type="button" className="mt-1.5 text-[13px] text-ok hover:underline" aria-expanded={open} onClick={() => setOpen((o) => !o)}>
          {open ? "Show less" : "Read the full check →"}
        </button>
      )}
    </div>
  );
}

function AllMoves({ result, onDraft, stale, product, asOf }: { result: PlanResult; onDraft: (key: string) => void; stale?: boolean; product?: Product | null; asOf?: string | null }) {
  const rejected = result.rejected ?? [];
  // Shown plans first, then the rest closest first (precedent check, then
  // net), matching the band (DESIGN.md: "every row greyed, closest first");
  // "No change" last.
  const byCheck = (a: Plan, b: Plan) => (b.verdict?.confidence ?? 0) - (a.verdict?.confidence ?? 0) || (b.net_per_year ?? 0) - (a.net_per_year ?? 0);
  const below = rejected.filter((p) => p.rejection_kind === "below_confidence_bar").sort(byCheck);
  const others = rejected.filter((p) => p.rejection_kind !== "below_confidence_bar" && p.rejection_kind !== "current_setup");
  const noChange = rejected.filter((p) => p.rejection_kind === "current_setup");
  // Operations mode: the server's order is the ranking (shown plans priced
  // first, then rejected rows with cost demotions ahead of the confidence-bar
  // rows), so the rejected rows keep it; "No change" stays last.
  const opsMode = [...(result.plans ?? []), ...rejected].some((p) => p.operations);
  const every = opsMode ? [...(result.plans ?? []), ...rejected.filter((p) => p.rejection_kind !== "current_setup"), ...noChange] : [...(result.plans ?? []), ...below, ...others, ...noChange];
  // Count moves only, as the search band does; "No change" is the baseline.
  const moves = every.length - noChange.length;
  const [open, setOpen] = useState(false);
  const all = open ? every : every.slice(0, 3);
  const small = smallestSupported(result.plans ?? [], product);
  // A result stored before D341 keeps its net-only order (it is the order
  // the stored backtest read); say so when that could differ.
  const storedOrder = !result.ranking && (result.plans?.length ?? 0) > 1;
  return (
    <section className="min-w-0">
      <h2 className="eyebrow mb-2.5">
        Moves searched · {moves} · {result.plans?.length ? (storedOrder ? "best net first" : copy.plan.tier.order) : "closest first by precedent check"}
      </h2>
      {storedOrder ? <p className="duo mb-2 text-xs text-dim">{copy.plan.tier.stored}</p> : null}
      {/* A table from sm up; on phones each move is a card, so the precedent
          check and net per year stay in view (#96). */}
      <div className="relative overflow-x-auto">
        <table className="w-full border-collapse border border-line bg-sheet text-[13px] max-sm:block sm:min-w-[560px]">
          <thead className="max-sm:sr-only">
            <tr>
              {["Move", "Outcome", "Precedent check", "Net per year", ""].map((h, i) => (
                <th key={i} scope="col" className="whitespace-nowrap border-b border-line bg-paper px-3.5 py-2.5 text-left text-[11px] font-medium uppercase tracking-[0.08em] text-dim">
                  {h || <span className="sr-only">Draft</span>}
                </th>
              ))}
            </tr>
          </thead>
          <tbody className="max-sm:block">
            {all.map((p) => {
              const key = moveKey(p);
              const grey = !p.shown;
              return (
                <tr key={key || "baseline"} className={cx("border-b border-line last:border-0 max-sm:grid max-sm:grid-cols-[1fr_auto] max-sm:items-center max-sm:gap-x-3 max-sm:gap-y-2 max-sm:px-3.5 max-sm:py-3", grey && "text-dim")}>
                  <td className="duo px-3.5 py-2.5 text-ink max-sm:col-span-2 max-sm:p-0">
                    {p.description}
                    <MoveIncludes product={product} plan={p} className="mt-0.5" />
                    <span className="mt-0.5 block text-xs text-dim">
                      {p.shown ? (
                        <>
                          Supported by {p.verdict?.supporting_ruling ? <RulingLink id={p.verdict.supporting_ruling} asOf={asOf} support /> : "an earlier ruling"}
                          {p.verdict?.counter_ruling ? (
                            <>
                              ; counter <RulingLink id={p.verdict.counter_ruling} />
                            </>
                          ) : null}
                        </>
                      ) : p.rejection_kind === "below_confidence_bar" ? (
                        <>
                          {copy.plan.belowBarRow(conf(p.verdict?.confidence))}
                          {p.verdict?.supporting_ruling ? (
                            <>
                              {" "}· supported by <RulingLink id={p.verdict.supporting_ruling} asOf={asOf} support />
                            </>
                          ) : null}
                        </>
                      ) : isCostRejection(p) ? (
                        <>
                          <b className="font-medium text-ink">{copy.opsResult.rejectedOps}.</b> {stale ? copy.opsResult.rejectedOpsStale : p.rejection_reason}
                        </>
                      ) : (
                        p.rejection_reason
                      )}
                    </span>
                    {p.operations && !stale && <OpsDetail ops={p.operations} objective={result.objective} className="mt-1.5" />}
                    {p.shown ? (
                      <span className="mt-1 flex flex-wrap gap-1.5">
                        <TierChip plan={p} result={result} />
                        {small === p ? <Chip>{copy.plan.tier.smallest}</Chip> : null}
                      </span>
                    ) : null}
                    {p.from_advice && <span className="mt-0.5 block text-xs text-ask">{copy.plan.advice.tried}</span>}
                    {p.near_miss && <NearMiss nm={p.near_miss} fallbackRuling={p.verdict?.supporting_ruling} />}
                    <CheckTrail trail={p.verdict?.trail} />
                  </td>
                  <td className="px-3.5 py-2.5 max-sm:p-0">{outcome(p)}</td>
                  <td className="px-3.5 py-2.5 max-sm:col-start-1 max-sm:p-0">
                    <span className="mb-0.5 block text-[11px] uppercase tracking-[0.06em] text-dim sm:hidden">Precedent check</span>
                    <MiniRuler value={p.verdict?.confidence} />
                    {p.verdict?.confidence != null && (
                      <span className="mt-0.5 block text-xs text-dim">
                        <span className="font-mono tabular-nums">{conf(p.verdict.confidence)}</span>
                        {agreementWords(p.verdict.agreement) ? ` · ${agreementWords(p.verdict.agreement)}` : ""}
                      </span>
                    )}
                  </td>
                  <td className="whitespace-nowrap px-3.5 py-2.5 max-sm:col-start-2 max-sm:row-start-2 max-sm:p-0 max-sm:text-right">
                    <span className="mb-0.5 block text-[11px] uppercase tracking-[0.06em] text-dim sm:hidden">Net per year</span>
                    {stale ? <span className="text-dim">{copy.stale.hidden}</span> : netText(p)}
                    {p.operations && !stale && <OpsScope ops={p.operations} className="mt-1 max-w-[30ch] whitespace-normal text-[11px]" />}
                  </td>
                  <td className="px-3.5 py-2.5 text-right max-sm:col-span-2 max-sm:p-0 max-sm:text-left empty:max-sm:hidden">
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

type Trail = NonNullable<NonNullable<Plan["verdict"]>["trail"]>;

// How the precedent check found its rulings (D80): each search with the
// rulings it returned and each ruling read, in the order it ran them (read a
// ruling, then search for a counter-ruling). Verdicts cached before D80
// carry no trail, and then there is no fold.
function CheckTrail({ trail }: { trail: Trail | undefined }) {
  if (!trail?.length) return null;
  const searches = trail.filter((t) => t.tool === "search");
  const reads = trail.filter((t) => t.tool === "read");
  const T = copy.plan.trail;
  return (
    <details className="group mt-2 text-xs text-dim">
      <summary className="min-h-6 cursor-pointer list-none py-0.5 [&::-webkit-details-marker]:hidden">
        <span aria-hidden="true" className="mr-1.5 inline-block w-2.5 text-ok transition-transform group-open:rotate-90">
          ▸
        </span>
        <span className="text-ok hover:underline">{T.title}</span> · {T.count(searches.length, reads.length)}
      </summary>
      <ol className="mt-1.5 flex flex-col gap-1 border-l border-line pl-3">
        {trail.map((t, i) =>
          t.tool === "read" ? (
            <li key={i}>
              <span className="text-ink">{T.read}</span> {t.rulings?.[0] ? <RulingLink id={t.rulings[0]} /> : `${t.query ?? ""} (${T.notFound})`}
            </li>
          ) : (
            <li key={i}>
              <span className="text-ink">{T.searched}</span> <span className="duo">“{t.query}”</span>
              <span className="block">
                {t.rulings?.length ? (
                  <>
                    {T.returned}{" "}
                    {t.rulings.map((r, j) => (
                      <span key={r}>
                        {j > 0 && ", "}
                        <RulingLink id={r} />
                      </span>
                    ))}
                  </>
                ) : (
                  T.nothing
                )}
              </span>
            </li>
          ),
        )}
      </ol>
    </details>
  );
}

// Near-miss advice (D81): on at most two rows under the bar, what differs
// from the supporting ruling and what would match it. Advice only, from one
// more model call; it changes neither the check, the bar nor the row.
function NearMiss({ nm, fallbackRuling }: { nm: NonNullable<Plan["near_miss"]>; fallbackRuling?: string }) {
  const ruling = nm.ruling || fallbackRuling;
  const N = copy.plan.nearMiss;
  if (!nm.difference && !nm.would_match) return null;
  // One line in the row; the advice opens under it (#232), so the table
  // stays scannable.
  return (
    <details className="group mt-1.5 text-xs text-dim">
      <summary className="inline-flex min-h-6 cursor-pointer list-none items-center gap-1.5 [&::-webkit-details-marker]:hidden">
        <span className="text-ask hover:underline">{N.short.replace(/:$/, "")}</span>
        <span aria-hidden="true" className="inline-block w-2.5 text-ask transition-transform group-open:rotate-90">
          ▸
        </span>
      </summary>
      <div className="mt-1 border-l-2 border-ask-line pl-3">
        <p className="mb-0.5 text-[11px] uppercase tracking-[0.06em] text-ask">{N.label}</p>
        {nm.difference && <p className="read text-[13px] leading-relaxed text-body">{nm.difference}</p>}
        {nm.would_match && (
          <p className="read mt-1 text-[13px] leading-relaxed text-body">
            <span className="text-ink">
              {N.wouldMatch} {ruling ? <RulingLink id={ruling} /> : N.theRuling} {N.if}
            </span>{" "}
            {nm.would_match}
          </p>
        )}
        <p className="mt-1 text-[11px]">{N.note}</p>
      </div>
    </details>
  );
}

const parts = (n: number) => `${n} ${n === 1 ? "part" : "parts"}`;

// The fold's short meta: "all 6 changed parts · 3 more moved".
function backtestShort(v: BacktestView): string {
  const k = v.missed.length === 0 ? `all ${parts(v.changed.length)} that changed` : `${v.moved.length} of ${parts(v.changed.length)} that changed`;
  return v.alsoKept.length ? `${k} · moves ${v.alsoKept.length} more` : k;
}

// The band's sentence: what the comparison is, and that it is not blind.
function backtestWords(v: BacktestView, target: string, from: string): string {
  const of = v.missed.length === 0 ? `all ${parts(v.changed.length)}` : `${v.moved.length} of the ${parts(v.changed.length)}`;
  const kept = v.alsoKept.length ? ` It also moves the ${andList(v.alsoKept)}, which that change kept in ${from}.` : "";
  return `moves ${of} the manufacturer's real change switched from ${from} to ${target}${v.ruling ? ` (CBP's later ruling ${v.ruling})` : ""}.${kept} Not a blind test.`;
}

// With no plan over the bar the backend compares CBP's change with nothing,
// so the view compares it with the closest move the page highlights instead.
function BacktestPanel({
  v,
  source,
  closest,
  target,
  from,
  tour,
  next,
}: {
  tour?: boolean;
  v: BacktestView;
  source?: string;
  closest?: Plan;
  target: string;
  from: string;
  next?: string;
}) {
  return (
    <div className="flex flex-col gap-2.5">
      {tour ? (
        <p className="eyebrow">
          {copy.tour.stepOf(2)} · {copy.tour.steps[1]}
        </p>
      ) : null}
      {!closest && v.matches ? (
        <p>
          <Chip tone="ok">Matches CBP&apos;s actual change</Chip>
        </p>
      ) : null}
      <p className="duo text-[13px]">
        What is compared: the parts whose origin the manufacturer&apos;s real change switched from {from} to {target}
        {v.ruling ? <> (CBP&apos;s later ruling <RulingLink id={v.ruling} />)</> : null}, against the parts this plan moves. The real change switched {parts(v.changed.length)}.{" "}
        {closest ? (
          <>
            No plan cleared the bar; the closest move ({movePhrase(closest)}) moves {v.moved.length} of them.
          </>
        ) : v.missed.length === 0 ? (
          <>This plan moves {v.changed.length === 1 ? "that part" : `all ${v.changed.length}`}.</>
        ) : (
          <>This plan moves {v.moved.length} of them.</>
        )}
      </p>
      <div className="flex flex-wrap gap-1.5" aria-label="Parts the real change switched">
        {v.changed.map((a) => (
          <span key={a} className={cx("border px-2 py-0.5 text-[11.5px]", v.moved.includes(a) ? "border-ok text-ok" : "border-line")}>
            {a}
            {v.moved.includes(a) ? null : <span className="sr-only"> (not moved by this plan)</span>}
          </span>
        ))}
      </div>
      {v.alsoKept.length ? (
        <>
          <p className="duo text-[13px]">
            The plan also moves {v.alsoKept.length === 1 ? "a part" : `${v.alsoKept.length} parts`} that CBP&apos;s real change kept in {from}: the {andList(v.alsoKept)}. A smaller
            move may have been enough. The search moves parts in template groups, and {v.alsoKept.length === 1 ? "this one sits" : "these sit"} in a group with parts that did switch
            {v.keptGroups.length ? <> ({andList(v.keptGroups.map((g) => (/^[A-Z][a-z]/.test(g) ? g[0].toLowerCase() + g.slice(1) : g)))})</> : null}.
          </p>
          <div className="flex flex-wrap gap-1.5" aria-label={`Parts the real change kept in ${from} that this plan moves`}>
            {v.alsoKept.map((a) => (
              <span key={a} className="border border-ask-line px-2 py-0.5 text-[11.5px] text-ask">
                {a}
              </span>
            ))}
          </div>
        </>
      ) : null}
      {v.already.length ? (
        <p className="text-xs text-dim">
          Not counted: the {andList(v.already)}, which the later ruling lists as made in {target} but which were already made there before the change.
        </p>
      ) : null}
      <p className="duo border-l-2 border-ask-line pl-3 text-xs text-body">
        Not a blind test. Three-group moves were added to the search after the team saw CBP&apos;s 2026 heat-gun change (
        <a href={decisionHref("D84")} target="_blank" rel="noreferrer" className="font-mono text-ok underline underline-offset-2">
          D84
        </a>
        ). The date cutoff still holds: the precedent check reads only rulings dated before the case.
      </p>
      {source ? <p className="text-xs text-dim">Source: {source}</p> : null}
      {next ? (
        <p>
          <a href={next} className="inline-flex min-h-8 items-center text-[13px] text-ok underline underline-offset-2">
            {copy.tour.next(copy.tour.steps[2])}
          </a>
        </p>
      ) : null}
    </div>
  );
}

function Stats({ stats, proxy }: { stats: Record<string, number>; proxy: boolean }) {
  return (
    <span className="text-[11px] text-dim">
      {stats.moves_searched} moves searched{stats.advice_followed ? ` (${stats.advice_followed} from advice)` : ""} · {stats.engine_calls} engine calls · {Math.round(stats.wall_seconds ?? 0)} s{proxy ? ` · ${copy.proxy}` : ""}
    </span>
  );
}
