"use client";

import { useSearchParams } from "next/navigation";
import { useMemo, useState } from "react";
import { api, type CaseResponse, type Economics, type Product, type Scenario } from "@/lib/api/client";
import { copy } from "@/lib/copy";
import { BAR, conf, money, pct } from "@/lib/format";
import { CaseShell, currentOrigin, type CaseCtx } from "../case-shell";
import { ErrorBox, Footer, Main, Next } from "../chrome";
import { MFN_ONLY, liveMfnFetched, liveMfnOrigins, pricedEconomics } from "@/lib/live-mfn";
import { MoneyDrawer } from "../money-drawer";
import { groupName } from "../plan-bits";
import { Button, ButtonLink, Chip, Country, Flag, Fold, Panel, PanelHead, Ruler, ScrollX, Spinner, StatusChip, cx } from "../ui";

export function ScenariosView() {
  return <CaseShell tab="scenarios">{(ctx) => <Scenarios {...ctx} />}</CaseShell>;
}

type Draft = { label: string; mode: "move" | "facts"; groups: string[]; plant: string; facts: string };

// Moves the chosen operation groups to a plant: their parts take that plant
// and its country, their operations that plant.
function variant(product: Product, groups: string[], plantId: string): Product {
  const plant = product.plants?.find((p) => p.id === plantId);
  if (!plant || groups.length === 0) return product;
  const g = new Set(groups);
  return {
    ...product,
    components: product.components.map((c) => (c.group && g.has(c.group) ? { ...c, plant: plant.id, made_in: plant.country } : c)),
    operations: product.operations?.map((o) => (o.group && g.has(o.group) ? { ...o, plant: plant.id } : o)),
  };
}

function Scenarios({ id, c, setCase }: CaseCtx) {
  const product = c.product;
  const groups = useMemo(
    () => [...new Set([...(product?.components ?? []), ...(product?.operations ?? [])].map((x) => x.group).filter((x): x is string => Boolean(x)))].filter((g) => g !== "final_assembly"),
    [product],
  );
  const origin = c.prediction?.origin_country;
  const otherPlant = product?.plants?.find((p) => p.country !== origin) ?? product?.plants?.[0];
  const other = otherPlant?.id ?? "";
  // Only groups that are not already made at that plant: moving the rest
  // changes nothing (#83, the heat gun's housing and heating element).
  const movable = groups.filter((g) =>
    [...(product?.components ?? []), ...(product?.operations ?? [])].some((x) => x.group === g && x.plant !== other),
  );
  const to = otherPlant?.country ? ` to ${otherPlant.country}` : "";
  // "Add as a scenario" from Ask: the what-if becomes a facts draft, kept
  // beside the first two so the case stays at three scenarios at most.
  const add = useSearchParams().get("add")?.trim();
  const [drafts, setDrafts] = useState<Draft[]>(() => {
    const base: Draft[] = product?.plants?.length
      ? [
          { label: "As today", mode: "move" as const, groups: [], plant: other, facts: "" },
          ...(movable.length >= 1 ? [{ label: `${groupName(movable[0])} moved${to}`, mode: "move" as const, groups: movable.slice(0, 1), plant: other, facts: "" }] : []),
          ...(movable.length >= 2
            ? [{ label: `${groupName(movable[0])} + ${groupName(movable[1])} moved${to}`, mode: "move" as const, groups: movable.slice(0, 2), plant: other, facts: "" }]
            : []),
        ]
      : [{ label: "Scenario 1", mode: "facts", groups: [], plant: "", facts: c.facts ?? "" }];
    if (!add) return base;
    const label = add.length > 60 ? add.slice(0, 59) + "…" : add;
    // From Ask: the revised facts the what-if ran on, when the drawer left
    // them; otherwise the original facts with the change appended (#115).
    let revised: string | null = null;
    try {
      revised = sessionStorage.getItem(`op-whatif:${c.id}:${add}`);
    } catch {}
    return [...base.slice(0, 2), { label, mode: "facts", groups: [], plant: "", facts: revised ?? `${(c.facts ?? "").trim()}\n\nChange: ${add}`.trim() }];
  });
  const [editing, setEditing] = useState(Boolean(add) || !(c.scenarios?.length));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [moneyOpen, setMoneyOpen] = useState(false);
  const saveMoney = async (e: Economics) => {
    setMoneyOpen(false);
    setError(null);
    try {
      setCase(await api.putEconomics(id, e));
    } catch (err) {
      setError(err);
    }
  };

  const run = async () => {
    setBusy(true);
    setError(null);
    try {
      const body = drafts.map((d) =>
        d.mode === "facts" || !product ? { label: d.label, facts: d.facts } : { label: d.label, product: variant(product, d.groups, d.plant) },
      );
      setCase(await api.scenarios(id, { scenarios: body, economics: c.economics }));
      setEditing(false);
    } catch (e) {
      setError(e);
    } finally {
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
            <Button variant="ghost" className="ml-auto" onClick={() => setEditing(true)}>
              Edit scenarios
            </Button>
          )}
        </div>

        <MoneyStrip c={c} onEdit={c.pinned ? undefined : () => setMoneyOpen(true)} />

        {editing && <Editor drafts={drafts} setDrafts={setDrafts} c={c} groups={groups} />}
        {error ? <ErrorBox error={error} onRetry={run} demoTitle={c.display_name} /> : null}
        {editing && (
          <div className="flex items-center gap-3">
            <Button onClick={run} disabled={busy || drafts.length === 0}>
              {sc.length ? "Run again" : "Predict the scenarios"}
            </Button>
            {busy && <Spinner label="Predicting each scenario…" />}
          </div>
        )}

        {!editing && sc.length > 0 && <Cards sc={sc} c={c} />}
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
          value={c.economics ?? undefined}
          product={product ?? undefined}
          countries={[...new Set((product?.plants ?? []).map((p) => p.country).concat(c.prediction?.origin_country ?? []).filter(Boolean))]}
          onClose={() => setMoneyOpen(false)}
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
function MoneyStrip({ c, onEdit }: { c: CaseResponse; onEdit?: () => void }) {
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
      {groups > 0 && <span className="text-dim">{groups} groups costed</span>}
      {onEdit ? (
        <button type="button" onClick={onEdit} className="ml-auto min-h-8 text-ok hover:underline">
          {c.economics ? "Edit inputs" : "+ Add money inputs"}
        </button>
      ) : (
        <span className="ml-auto text-xs text-dim">demo inputs · start a new case to change them</span>
      )}
    </div>
  );
}

function Editor({ drafts, setDrafts, c, groups }: { drafts: Draft[]; setDrafts: (d: Draft[]) => void; c: CaseResponse; groups: string[] }) {
  const set = (i: number, d: Partial<Draft>) => setDrafts(drafts.map((x, j) => (j === i ? { ...x, ...d } : x)));
  return (
    <div className="grid gap-5 lg:grid-cols-3">
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

function Cards({ sc, c }: { sc: Scenario[]; c: CaseResponse }) {
  const cur = currentOrigin(c)?.country ?? c.prediction?.origin_country;
  const live = liveMfnOrigins(pricedEconomics(c));
  const nets = sc.map((s) => s.comparison?.net_usd ?? -Infinity);
  const confs = sc.map((s) => s.prediction?.confidence ?? 0);
  const bestNet = Math.max(...nets);
  const bestConf = Math.max(...confs);
  // A chip only marks a clear winner; on a tie it says nothing (#89).
  const oneBestConf = confs.filter((x) => x === bestConf).length === 1;
  const oneBestNet = nets.filter((x) => x === bestNet).length === 1;
  return (
    <div className="grid gap-5 lg:grid-cols-3">
      {sc.map((s, i) => {
        const p = s.prediction ?? {};
        const cmp = s.comparison;
        const nothingMoves = s.product && (cmp?.moved?.length ?? 0) === 0;
        return (
          <Panel key={i} className="flex flex-col gap-3">
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="text-[15px] font-medium">{prettyLabel(s.label)}</h2>
              {nets[i] === bestNet && oneBestNet && Number.isFinite(bestNet) && bestNet > 0 && <Chip tone="ok">highest net</Chip>}
              {confs[i] === bestConf && oneBestConf && sc.length > 1 && <Chip>highest confidence</Chip>}
            </div>
            <p className="flex items-center gap-3 text-[40px] leading-none tracking-[-0.06em]">
              <Flag country={p.origin_country} size={30} />
              {p.origin_country}
            </p>
            <span className="self-start">
              <StatusChip route={s.triage?.route} />
            </span>
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
            <Ruler value={p.confidence} label={`${BAR} auto-accept`} width={220} caption={`confidence ${conf(p.confidence)}`} />
            {p.decisive_step && <p className="duo clamp-2 text-[13px]">{p.decisive_step}</p>}
            {cmp?.origin_differs_from_current && cur && (nothingMoves || !s.product) && (
              <p className="duo text-[13px] text-ask">
                △ Differs from CBP: <Country name={cur} />
              </p>
            )}
            <div className="mt-auto border-t border-line pt-3">
              <p className="eyebrow">Net per year · illustrative</p>
              {cmp && cmp.net_usd !== null && cmp.net_usd !== undefined ? (
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
                  {money(s.comparison?.net_usd)}
                </td>
              ))}
            </tr>
          </tbody>
        </table>
      </ScrollX>
    </section>
  );
}
