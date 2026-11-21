"use client";

import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { useEffect, useMemo, useRef, useState } from "react";
import { api, opLock, saveEconomics, type CaseResponse, type Economics, type Scenario } from "@/lib/api/client";
import { VIEWER_REASON, caseReadOnly, writeGate } from "@/lib/roles";
import { copy } from "@/lib/copy";
import { BAR, conf, money, originConf, originConfCaption, pct } from "@/lib/format";
import { CaseShell, currentOrigin, type CaseCtx } from "../case-shell";
import { ErrorBox, Footer, Main, Next } from "../chrome";
import { MFN_ONLY, liveMfnFetched, liveMfnOrigins, pricedEconomics } from "@/lib/live-mfn";
import { DutyBreakdown } from "../duty-breakdown";
import { MoneyDrawer } from "../money-drawer";
import { opsErrorText } from "../operational-fields";
import { OpsDetail, OpsScope, OpsUnknown } from "../ops-result";
import { comparisonNetText, feasibilityText, figureText, netText } from "../ops-view";
import { opsActive } from "@/lib/ops";
import { groupName } from "../plan-bits";
import { ReviewNote } from "../review-note";
import { Term } from "../term";
import { exampleDrafts, initialDrafts, scenarioBody, type Draft } from "@/lib/scenario-drafts";
import { engineLabel, highestNet, planCheckOf, planCheckWords, readsDiffer } from "@/lib/plan-check";
import { Button, ButtonLink, Chip, Country, Flag, Fold, Panel, PanelHead, Ruler, ScrollX, Spinner, StatusChip, cx } from "../ui";

export function ScenariosView() {
  return <CaseShell tab="scenarios">{(ctx) => <Scenarios {...ctx} />}</CaseShell>;
}

export function Scenarios({ id, c, setCase }: CaseCtx) {
  const product = c.product;
  const groups = useMemo(
    () => [...new Set([...(product?.components ?? []), ...(product?.operations ?? [])].map((x) => x.group).filter((x): x is string => Boolean(x)))].filter((g) => g !== "final_assembly"),
    [product],
  );
  // "Add as a scenario" from Ask (see withAsked).
  const add = useSearchParams().get("add")?.trim();
  const [drafts, setDrafts] = useState<Draft[]>(() =>
    initialDrafts(c, add, () => {
      try {
        return sessionStorage.getItem(`op-whatif:${c.id}:${add}`);
      } catch {
        return null;
      }
    }),
  );
  const [editing, setEditing] = useState(Boolean(add) || !(c.scenarios?.length));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  // What Try again repeats: the failed money save with the values the user
  // entered (for the case it was for), or null for a failed scenario run.
  const [retrySave, setRetrySave] = useState<{ id: string; economics: Economics } | null>(null);
  const [moneyOpen, setMoneyOpen] = useState(false);
  // The server refused the operating-cost inputs (400): the drawer reopens with
  // what was typed and the message beside the fields it names.
  const [opsFail, setOpsFail] = useState<{ economics: Economics; message: string } | null>(null);
  // The newest case this screen holds, so a slow money save or re-read never
  // writes back the case from when it started.
  // Kept current by an effect (not during render) and by commit().
  const latest = useRef(c);
  useEffect(() => {
    latest.current = c;
  }, [c]);
  // A money save and a scenario run must not overlap: the run would store
  // the economics it started with over the save.
  const lock = useRef(opLock()).current;
  const [saving, setSaving] = useState(false);
  const commit = (next: CaseResponse) => {
    latest.current = next;
    setCase(next);
  };
  const saveMoney = async (e: Economics) => {
    setMoneyOpen(false);
    const release = lock.start();
    if (!release) return;
    setSaving(true);
    setError(null);
    setRetrySave(null);
    setOpsFail(null);
    try {
      await saveEconomics(id, e, { latest: () => latest.current, apply: commit });
    } catch (err) {
      setError(err);
      setRetrySave({ id, economics: e });
      const message = opsErrorText(err);
      if (message) {
        setOpsFail({ economics: e, message });
        setMoneyOpen(true);
      }
    } finally {
      release();
      setSaving(false);
    }
  };

  const run = async () => {
    const release = lock.start();
    if (!release) return;
    setBusy(true);
    setError(null);
    setRetrySave(null);
    try {
      commit(await api.scenarios(id, { scenarios: scenarioBody(drafts, product), economics: c.economics }));
      setEditing(false);
    } catch (e) {
      setError(e);
    } finally {
      release();
      setBusy(false);
    }
  };

  const sc = c.scenarios ?? [];
  return (
    <>
      <Main className="flex flex-col gap-8">
        <div className="flex flex-wrap items-end gap-3">
          <div>
            <p className="eyebrow">Optional · Try scenarios</p>
            <h1 className="mt-1 text-xl tracking-[-0.04em]">Scenarios side by side</h1>
            <p className="duo mt-1 max-w-[80ch] text-sm text-dim">
              Try your own changes and see the likely origin of each, side by side. The case&apos;s answer doesn&apos;t change. You don&apos;t need this step:
              Plan searches the moves for you and checks each against earlier rulings.
            </p>
          </div>
          {!editing && (
            <Button variant="ghost" className="ml-auto" onClick={() => setEditing(true)} {...writeGate(caseReadOnly(c))}>
              Edit scenarios
            </Button>
          )}
        </div>

        <MoneyStrip c={c} locked={busy || saving} onEdit={c.pinned || caseReadOnly(c) ? undefined : () => setMoneyOpen(true)} />

        {editing && <Editor drafts={drafts} setDrafts={setDrafts} c={c} groups={groups} />}
        {error ? <ErrorBox error={error} onRetry={retrySave ? (retrySave.id === id ? () => void saveMoney(retrySave.economics) : undefined) : run} demoTitle={c.display_name} /> : null}
        {editing && (
          <div className="flex items-center gap-3">
            <Button onClick={run} disabled={busy || saving || drafts.length === 0 || caseReadOnly(c)} title={caseReadOnly(c) ? VIEWER_REASON : undefined}>
              {sc.length ? "Run again" : "Predict the scenarios"}
            </Button>
            {busy && <Spinner label="Predicting each scenario…" />}
            {saving && <Spinner label="Saving money inputs…" />}
            <Button
              variant="ghost"
              disabled={busy || saving}
              onClick={() => setDrafts(exampleDrafts(c))}
              title="Replace the scenarios above with the examples"
            >
              Reset to examples
            </Button>
            {sc.length > 0 && (
              <Button
                variant="ghost"
                disabled={busy || saving}
                onClick={() => {
                  setDrafts(initialDrafts(c, undefined, () => null));
                  setError(null);
                  setRetrySave(null);
                  setEditing(false);
                }}
              >
                Cancel
              </Button>
            )}
          </div>
        )}

        {!editing && sc.length > 0 && <Cards sc={sc} c={c} />}
        {!editing && sc.length > 0 && sc.some((s) => originConf(s.prediction).calibrated) && <p className="duo -mt-4 text-xs text-dim">{copy.calSteps}</p>}
        {!editing && sc.length > 0 && <ReviewNote place="scenarios" />}
        {!editing && product && sc.some((s) => s.product && (s.comparison?.moved?.length ?? 0) > 0) && (
          <div>
            <Fold title="Where each part is made" meta="today and in each scenario">
              <Matrix sc={sc} c={c} />
            </Fold>
          </div>
        )}
      </Main>
      {moneyOpen && (
        <MoneyDrawer
          value={opsFail?.economics ?? c.economics ?? undefined}
          serverError={opsFail?.message}
          readOnly={caseReadOnly(c)}
          priced={pricedEconomics(c)}
          product={product ?? undefined}
          countries={[...new Set((product?.plants ?? []).map((p) => p.country).concat(c.prediction?.origin_country ?? []).filter(Boolean))]}
          onClose={() => {
            setMoneyOpen(false);
            setOpsFail(null);
          }}
          onSave={saveMoney}
        />
      )}
      <Footer>
        <Next>{product?.plants?.length ? "Search precedent-checked plans for the move that looks best." : "Compare what-ifs, then add parts and plants to search real plans."}</Next>
        <div className="ml-auto">
          {product?.plants?.length ? <ButtonLink href={`/cases/${encodeURIComponent(id)}/plan`}>Search plans →</ButtonLink> : null}
        </div>
      </Footer>
    </>
  );
}

// Money above the cards (DESIGN.md, p-scen): the inputs every net below
// is worked out from, and the drawer to change them.
// Rates come from economics_priced (D56): what the user typed plus the
// sourced duty rates the plan search uses. Edits still go to economics.
function MoneyStrip({ c, onEdit, locked }: { c: CaseResponse; onEdit?: () => void; locked?: boolean }) {
  const e = pricedEconomics(c);
  const groups = Object.keys(e?.group_costs ?? {}).length;
  const live = liveMfnOrigins(e);
  const fetched = liveMfnFetched(e);
  return (
    <div className="flex flex-wrap items-center gap-x-6 gap-y-2 border border-edge bg-sheet px-4 py-3 text-[13px] shadow-[3px_3px_0_var(--shadow)]">
      <span className="text-[11px] uppercase tracking-[0.08em] text-dim">{copy.money}</span>
      {e?.annual_import_value_usd ? <span>{money(e.annual_import_value_usd)} imports a year</span> : <span className="text-dim">No import value yet</span>}
      {Object.entries(e?.duty_rates ?? {}).map(([k, v]) => (
        <span key={k} title={e?.duty_rates_note ?? undefined}>
          <Country name={k} /> {pct(v)}
          {live.has(k) && <span className="ml-1 text-[11px] text-ask">MFN only</span>}
        </span>
      ))}
      {live.size > 0 && (
        <span className="basis-full text-xs text-ask" title={e?.duty_rates_note ?? undefined}>
          {MFN_ONLY}: live USITC rate for {[...live].join(", ")}
          {fetched ? `, fetched ${fetched}` : ""}. Section 301, forced-labor and 232 duties may apply on top.
        </span>
      )}
      <DutyBreakdown e={e} className="basis-full" />
      {groups > 0 && <span className="text-dim">{groups} groups costed</span>}
      {opsActive(c.economics?.operations) && <span className="text-dim">operating costs entered</span>}
      {onEdit ? (
        <button type="button" onClick={onEdit} disabled={locked} title={locked ? "Wait for the current save or prediction to finish" : undefined} className="ml-auto min-h-8 text-ok hover:underline disabled:cursor-not-allowed disabled:text-dim disabled:no-underline">
          {c.economics ? "Edit inputs" : "+ Add money inputs"}
        </button>
      ) : (
        <span className="ml-auto text-xs text-dim">demo inputs · start a new case to change them</span>
      )}
    </div>
  );
}

export function Editor({ drafts, setDrafts, c, groups }: { drafts: Draft[]; setDrafts: (d: Draft[]) => void; c: CaseResponse; groups: string[] }) {
  // Changing a move's groups or plant means the stored variant no longer applies.
  const set = (i: number, d: Partial<Draft>) =>
    setDrafts(drafts.map((x, j) => (j === i ? { ...x, ...d, ...("groups" in d || "plant" in d ? { stored: undefined } : {}) } : x)));
  return (
    <div className="grid grid-cols-1 gap-5 lg:grid-cols-3">
      {drafts.map((d, i) => (
        <Panel key={i}>
          <PanelHead right={<button type="button" className="text-dim hover:text-fail" aria-label={`Remove ${d.label}`} onClick={() => setDrafts(drafts.filter((_, j) => j !== i))}>×</button>}>
            Scenario {i + 1}
          </PanelHead>
          <label className="eyebrow block" htmlFor={`sl${i}`}>
            Label
          </label>
          <input id={`sl${i}`} className="mt-1.5 w-full rounded-[2px] border border-line bg-white px-3 py-2 text-sm" value={d.label} onChange={(e) => set(i, { label: e.target.value })} />
          {c.product && d.mode === "move" ? (
            <fieldset className="mt-3">
              <legend className="eyebrow">Move these groups</legend>
              <div className="mt-1.5 flex flex-col gap-1 text-[13px]">
                {groups.map((g) => (
                  <label key={g} className="flex items-center gap-2">
                    <input
                      type="checkbox"
                      checked={d.groups.includes(g)}
                      onChange={(e) => set(i, { mode: "move", groups: e.target.checked ? [...d.groups, g] : d.groups.filter((x) => x !== g) })}
                    />
                    {groupName(g)}
                  </label>
                ))}
              </div>
              <label className="eyebrow mt-3 block" htmlFor={`sp${i}`}>
                To plant
              </label>
              <select id={`sp${i}`} className="mt-1.5 w-full rounded-[2px] border border-line bg-white px-2 py-2 text-sm" value={d.plant} onChange={(e) => set(i, { plant: e.target.value })}>
                {c.product.plants?.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name || p.id} · {p.country}
                  </option>
                ))}
              </select>
              {d.groups.length === 0 && <p className="mt-2 text-xs text-dim">Nothing moves: this is the current setup.</p>}
            </fieldset>
          ) : (
            <>
              <label className="eyebrow mt-3 block" htmlFor={`sf${i}`}>
                Facts
              </label>
              <textarea id={`sf${i}`} className="duo mt-1.5 min-h-40 w-full rounded-[2px] border border-line bg-white px-3 py-2 text-[13px]" value={d.facts} onChange={(e) => set(i, { facts: e.target.value })} />
            </>
          )}
        </Panel>
      ))}
      {drafts.length < 3 && (
        <button
          type="button"
          className="min-h-40 border border-dashed border-guide text-sm text-ok hover:bg-sheet"
          onClick={() => setDrafts([...drafts, { label: `Scenario ${drafts.length + 1}`, mode: c.product ? "move" : "facts", groups: [], plant: c.product?.plants?.[0]?.id ?? "", facts: c.facts ?? "" }])}
        >
          + Add a scenario (up to 3)
        </button>
      )}
    </div>
  );
}

function prettyLabel(label?: string) {
  return (label ?? "").replace(/[a-z]+(?:_[a-z]+)+|\bpcba\b/g, (g) => groupName(g));
}

// The plan search's recommendation above the cards, so a scenario's engine
// answer is never read as the plan (D220).
function PlanLine({ c }: { c: CaseResponse }) {
  const best = c.plan?.plans?.find((p) => p.shown && (p.move?.length ?? 0) > 0);
  if (!c.plan) return null;
  const href = `/cases/${encodeURIComponent(c.id ?? "")}/plan`;
  return (
    <p className="duo border border-line bg-sheet px-4 py-3 text-[13px]">
      {best ? (
        <>
          <span className="text-ok">The plan search recommends:</span> {best.description} · precedent check {conf(best.verdict?.confidence)} · {netText(best)}
          {best.operations ? "" : "/yr"}.{" "}
        </>
      ) : (
        <>No move cleared the {BAR} precedent-check bar in the plan search. </>
      )}
      <span className="text-dim">The cards below are the engine&apos;s origin answer for each what-if. A move becomes a plan only when its precedent check reaches {BAR}.</span>{" "}
      <Link href={href} className="text-ok underline underline-offset-2">
        Open Plan →
      </Link>
    </p>
  );
}

function Cards({ sc, c }: { sc: Scenario[]; c: CaseResponse }) {
  const cur = currentOrigin(c)?.country ?? c.prediction?.origin_country;
  const live = liveMfnOrigins(pricedEconomics(c));
  const confs = sc.map((s) => s.prediction?.confidence ?? 0);
  const bestConf = Math.max(...confs);
  // A chip only marks a clear winner; on a tie it says nothing (#89). A move
  // the plan search did not show is never "highest net" (D220).
  const oneBestConf = confs.filter((x) => x === bestConf).length === 1;
  const top = highestNet(sc);
  return (
    <div className="flex flex-col gap-5">
    <PlanLine c={c} />
    <div className="grid grid-cols-1 gap-5 lg:grid-cols-3">
      {sc.map((s, i) => {
        const p = s.prediction ?? {};
        const cmp = s.comparison;
        const nothingMoves = s.product && (cmp?.moved?.length ?? 0) === 0;
        const pc = planCheckOf(s);
        const words = pc ? planCheckWords(pc) : null;
        const differs = nothingMoves ? readsDiffer(c, s) : null;
        return (
          <Panel key={i} className="flex flex-col gap-3">
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="text-[15px] font-medium">{prettyLabel(s.label)}</h2>
              {top?.index === i && <Chip tone={top.checked ? "ok" : undefined}>{top.checked ? "highest net" : "highest net · not precedent-checked"}</Chip>}
              {confs[i] === bestConf && oneBestConf && sc.length > 1 && <Chip>highest engine confidence</Chip>}
            </div>
            <p className="flex items-center gap-3 text-[40px] leading-none tracking-[-0.06em]">
              <Flag country={p.origin_country} size={30} />
              {p.origin_country}
            </p>
            <span className="flex flex-wrap items-center gap-2 self-start">
              {s.triage?.route === "auto" ? <Chip>{engineLabel(s)}</Chip> : <StatusChip route={s.triage?.route} />}
            </span>
            {words && (
              <div className={cx("border px-3 py-2 text-xs", words.supported ? "border-ok bg-ok-wash" : "border-dashed border-ask-line bg-ask-wash")}>
                <p className="flex flex-wrap items-center gap-2">
                  <span className="eyebrow">Plan search</span>
                  <Chip tone={words.tone}>{words.chip}</Chip>
                </p>
                <p className={cx("duo mt-1", words.supported ? "text-ok" : "text-ask")}>
                  {words.line}
                  {pc?.supporting_ruling ? <> · closest support {pc.supporting_ruling}</> : null}.
                </p>
              </div>
            )}
            {differs && <p className="duo text-xs text-dim">{differs}</p>}
            {(cmp?.moved?.length ?? 0) > 0 && (
              <p className="flex flex-wrap items-center gap-1.5 text-xs">
                <span className="text-dim">Moves</span>
                {cmp!.moved!.map((m) => (
                  <Chip key={`${m.group}-${m.to_plant}`} tone="ok">
                    {groupName(m.group)}
                  </Chip>
                ))}
                {cmp!.moved![0]?.to_plant && (
                  <span className="text-dim">
                    to {c.product?.plants?.find((pl) => pl.id === cmp!.moved![0].to_plant)?.name ?? cmp!.moved![0].to_plant}
                  </span>
                )}
              </p>
            )}
            {(() => {
              const o = originConf(p);
              return (
                <Ruler
                  value={o.value}
                  bar={o.bar}
                  label={`${o.bar.toFixed(2)} auto-accept`}
                  width={220}
                  caption={o.calibrated ? <Term explain={copy.terms.calibrated.text}>{originConfCaption(o)}</Term> : originConfCaption(o)}
                />
              );
            })()}
            {p.decisive_step && <p className="duo clamp-2 text-[13px]">{p.decisive_step}</p>}
            {cmp?.origin_differs_from_current && cur && (nothingMoves || !s.product) && (
              <p className="duo text-[13px] text-ask">
                △ Differs from CBP: <Country name={cur} />
              </p>
            )}
            <div className="mt-auto border-t border-line pt-3">
              <p className="eyebrow">Net per year · illustrative</p>
              {cmp?.operations ? (
                <>
                  <p className={cx("mt-1 leading-none tracking-[-0.05em]", cmp.net_usd ? "text-[28px]" : "text-[20px]")}>{comparisonNetText(cmp)}</p>
                  <p className="mt-1.5 text-xs text-dim">
                    {nothingMoves
                      ? `Nothing moves, so duty stays at ${cur ?? "the current origin"}'s ${pct(cmp.duty_rate)}`
                      : `${figureText(cmp.operations.components.duty_effect_per_year, cmp.operations, ["duty"])} duty effect · ${figureText(cmp.operations.components.operating_delta_per_year, cmp.operations, ["operating", "group_cost", "labour", "logistics"])} operating change · ${figureText(cmp.amortized_one_time_usd, cmp.operations, ["amortized_one_time", "one_time", "capex", "retooling", "horizon"])} one-time a year · duty ${pct(cmp.duty_rate)}`}
                  </p>
                  {cmp.feasibility && <p className="mt-1 text-xs text-dim">{feasibilityText(cmp.operations)}</p>}
                  <OpsScope ops={cmp.operations} className="mt-1.5" />
                  <OpsUnknown ops={cmp.operations} className="mt-1.5" />
                  <OpsDetail ops={cmp.operations} className="mt-1.5" />
                </>
              ) : cmp && cmp.net_usd !== null && cmp.net_usd !== undefined ? (
                <>
                  <p className="mt-1 text-[28px] leading-none tracking-[-0.05em]">{money(cmp.net_usd)}</p>
                  <p className="mt-1.5 text-xs text-dim">
                    {nothingMoves
                      ? `Nothing moves, so duty stays at ${cur ?? "the current origin"}'s ${pct(cmp.duty_rate)}`
                      : `${money(cmp.duty_savings_usd)} duty saved − ${money(cmp.added_cost_usd)} added cost · duty ${pct(cmp.duty_rate)}`}
                    {(nothingMoves ? cur && live.has(cur) : p.origin_country && live.has(p.origin_country)) ? ` (${MFN_ONLY})` : ""}
                    {!s.product && cmp.origin_differs_from_current ? " · if the predicted origin holds" : ""}
                  </p>
                </>
              ) : (
                <p className="mt-1 text-xs text-dim">— add duty rates or costs</p>
              )}
            </div>
          </Panel>
        );
      })}
    </div>
    </div>
  );
}

function Matrix({ sc: all, c }: { sc: Scenario[]; c: CaseResponse }) {
  const parts = c.product!.components;
  // The fixed "Today" column already shows the current setup, so a product
  // scenario that moves nothing would repeat it.
  // A description-only scenario has no parts to show.
  const sc = all.filter((s) => s.product && (s.comparison?.moved?.length ?? 0) > 0);
  if (sc.length === 0) return null;
  return (
    <section>
      <ScrollX label="Where each part is made">
        <table className="w-full min-w-[640px] border-collapse border border-edge bg-sheet text-[13px] shadow-[4px_4px_0_var(--shadow)]">
          <thead>
            <tr>
              <th scope="col" className="border-b border-line bg-paper px-3.5 py-2.5 text-left text-[11px] font-medium uppercase tracking-[0.08em] text-dim">
                Part
              </th>
              <th scope="col" className="border-b border-line bg-paper px-3.5 py-2.5 text-left text-[11px] font-medium uppercase tracking-[0.08em] text-dim">
                Today
              </th>
              {sc.map((s, i) => (
                <th key={i} scope="col" className="border-b border-line bg-paper px-3.5 py-2.5 text-left text-[11px] font-medium uppercase tracking-[0.08em] text-dim">
                  {prettyLabel(s.label)}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {parts.map((p, r) => (
              <tr key={r} className="border-b border-line">
                <th scope="row" className="px-3.5 py-2 text-left font-normal">
                  {p.name}
                </th>
                <td className="px-3.5 py-2">
                  <Country name={p.made_in} />
                </td>
                {sc.map((s, i) => {
                  const m = s.product?.components?.find((x) => x.name === p.name)?.made_in;
                  const diff = m && m !== p.made_in;
                  return (
                    <td key={i} className={cx("px-3.5 py-2", diff && "bg-ok-wash text-ok")}>
                      {m ? <Country name={m} /> : <span className="text-dim">—</span>}
                    </td>
                  );
                })}
              </tr>
            ))}
            <tr>
              <th scope="row" className="px-3.5 py-2 text-left font-medium">
                Net per year
              </th>
              <td className="px-3.5 py-2">$0</td>
              {sc.map((s, i) => (
                <td key={i} className="px-3.5 py-2 font-medium">
                  {comparisonNetText(s.comparison)}
                </td>
              ))}
            </tr>
          </tbody>
        </table>
      </ScrollX>
    </section>
  );
}
