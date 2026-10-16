"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useEffect, useId, useState, type ReactNode } from "react";
import { api, type Component, type Economics, type ImportResponse, type Operation, type Plant, type PredictRequest, type Product } from "@/lib/api/client";
import { copy } from "@/lib/copy";
import { rememberDemo } from "@/lib/demo-store";
import { ActionSlotProvider } from "../action-bar";
import { CaseTabs, Elapsed, ErrorBox, Main, Next } from "../chrome";
import { MoneyDrawer, hasAnyMoney, hasMoney, moneySummary, productGroups } from "../money-drawer";
import { groupName } from "../plan-bits";
import { PdfTableError, pdfToBomCsv } from "@/lib/pdf-bom";
import { Button, Chip, Country, Panel, PanelHead, Spinner, cx } from "../ui";

type Tab = "parts" | "paste" | "bom";
const tabOrder: Tab[] = ["parts", "paste", "bom"];

// Operation groups of the planner templates, offered as suggestions.
const groupSuggestions = [
  "motor_fan_assembly",
  "pcba",
  "heating_element",
  "housing_moulding",
  "switch_cord_minor_parts",
  "final_assembly",
];

const today = () => new Date().toISOString().slice(0, 10);

// No empty rows on arrival: the parts tab starts with "+ Add" prompts.
const emptyProduct = (): Product => ({ name: "", plants: [], components: [], operations: [] });

const families = [
  { id: "motorised_tools_small_appliances", name: "Motorised tools, small appliances" },
  { id: "lidar_modules", name: "LiDAR sensor modules" },
];

type Tier = { key: "origin" | "plan" | "money"; label: string; state: "done" | "limited" | "missing"; note: string };

// What each kind of input unlocks (DESIGN.md step 1, the "unlocks" strip).
function unlocks(tab: Tab, facts: string, product: Product, economics: Economics | undefined): Tier[] {
  const plants = product.plants ?? [];
  const parts = product.components.filter((c) => c.name.trim() && c.made_in.trim());
  const countries = new Set(plants.map((p) => p.country.trim()).filter(Boolean));
  // What a plant could take on beyond the groups it makes now.
  const madeAt = (id: string) => new Set([...product.components, ...(product.operations ?? [])].filter((x) => x.plant === id).map((x) => x.group));
  const newGroups = (p: Plant) => (p.capabilities ?? []).filter((g) => g !== "final_assembly" && !madeAt(p.id).has(g));
  const extra = plants.filter((p) => newGroups(p).length > 0);
  const chars = facts.trim().length;
  const origin = tab === "paste" ? chars > 20 : parts.length > 0;
  const plan = tab !== "paste" && countries.size >= 2 && extra.length > 0;
  const money = hasMoney(economics);
  const ops = product.operations?.length ?? 0;
  return [
    {
      key: "origin",
      label: "Likely origin",
      state: origin ? "done" : "missing",
      note: origin
        ? tab === "paste"
          ? `Ready. The analysis reads your ${chars.toLocaleString("en-US")} characters.`
          : `Ready: ${parts.length} component${parts.length === 1 ? "" : "s"}${ops ? `, ${ops} operation${ops === 1 ? "" : "s"}` : ""}.`
        : tab === "paste"
          ? "Needs a description of a sentence or more."
          : "Needs a part and where it's made.",
    },
    {
      key: "plan",
      label: "Plan search",
      state: plan ? "done" : tab !== "paste" && countries.size === 1 ? "limited" : "missing",
      note: plan
        ? `Ready: ${extra.map((p) => `the ${p.name || p.country} could take on ${newGroups(p).length} more group${newGroups(p).length === 1 ? "" : "s"}`).join("; ")}.`
        : tab === "paste"
          ? "Needs plants and parts. Draft them from this text, then check them."
          : countries.size === 1
            ? "Only one country: add a plant in a second country."
            : countries.size < 2
              ? "Needs plants in two countries."
              : "Mark what a plant could also make.",
    },
    {
      key: "money",
      label: "Money per move",
      state: money ? "done" : "missing",
      note: money ? moneySummary(economics) + "." : "Needs imports a year, duty rates and a cost per group.",
    },
  ];
}

export function ProductStep() {
  const router = useRouter();
  const params = useSearchParams();
  const demoId = params.get("demo");
  // ?from=<case>: a pasted case's description, to turn into parts and plants
  // so the plan search can run (#81). The old case stays as it is.
  const fromId = params.get("from");

  const [tab, setTab] = useState<Tab>("parts");
  const [name, setName] = useState("");
  const [date, setDate] = useState(today);
  const [family, setFamily] = useState("motorised_tools_small_appliances");
  const [facts, setFacts] = useState("");
  const [product, setProduct] = useState<Product>(emptyProduct);
  const [economics, setEconomics] = useState<Economics | undefined>(undefined);
  const [moneyOpen, setMoneyOpen] = useState(false);
  const [extra, setExtra] = useState<Pick<PredictRequest, "exclude_rulings" | "current_origin" | "facts">>({});
  const [demoTitle, setDemoTitle] = useState<string | null>(null);
  const [imported, setImported] = useState<ImportResponse | null>(null);
  const [drafting, setDrafting] = useState<{ since: number } | null>(null);
  const [drafted, setDrafted] = useState<string[] | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);

  useEffect(() => {
    if (!demoId) return;
    api.demo(demoId).then((d) => {
      const p = d.predict;
      if (!p) return;
      setDemoTitle(d.title ?? demoId);
      setDate(p.date);
      if (p.family) setFamily(p.family);
      if (p.product) {
        setProduct(p.product);
        setName(p.product.name);
        setTab("parts");
      } else {
        setTab("paste");
        setName(d.title ?? "");
      }
      setFacts(p.facts ?? "");
      if (d.plan?.economics) setEconomics(d.plan.economics);
      setExtra({ exclude_rulings: p.exclude_rulings, current_origin: p.current_origin, facts: p.product ? p.facts : undefined });
    }, setError);
  }, [demoId]);

  useEffect(() => {
    if (!fromId || demoId) return;
    api.getCase(fromId).then((c) => {
      setName(c.product?.name || c.display_name || "");
      if (c.date) setDate(c.date);
      setFacts(c.facts ?? "");
      if (c.product?.components?.length) {
        setProduct(c.product);
        setTab("parts");
      } else setTab("paste");
    }, setError);
  }, [fromId, demoId]);

  // "Turn into parts and plants": the engine drafts the tables from the
  // text (nothing saved); they open on the Parts and plants tab to check.
  const draftParts = async () => {
    setDrafting({ since: Date.now() });
    setError(null);
    try {
      const r = await api.extract({ facts, family: family || undefined, name: name.trim() || undefined });
      if (r.product) {
        setProduct({ ...r.product, name: name.trim() || r.product.name });
        if (!name.trim() && r.product.name) setName(r.product.name);
        setExtra((x) => ({ ...x, facts }));
        setDrafted(r.warnings ?? []);
        setTab("parts");
      }
    } catch (e) {
      setError(e);
    } finally {
      setDrafting(null);
    }
  };

  const submit = async () => {
    setBusy(true);
    setError(null);
    const body: PredictRequest = { date, exclude_rulings: extra.exclude_rulings, current_origin: extra.current_origin, economics: hasAnyMoney(economics) ? economics : undefined };
    if (tab === "paste") {
      // The backend rejects a product with no components, so a pasted
      // description goes in as facts only; the name is set right after.
      body.facts = facts;
    } else {
      body.product = { ...product, name: name.trim() };
      body.family = family || undefined;
      if (extra.facts) body.facts = extra.facts; // demo or drafted: engine reads the text, product kept for the plan
    }
    try {
      const c = await api.predict(body);
      // Name the case, so the Cases list never shows truncated facts.
      if (tab === "paste" && name.trim() && c.id) await api.rename(c.id, name.trim()).catch(() => {});
      if (demoId && c.id) rememberDemo(c.id, demoId);
      router.push(`/cases/${encodeURIComponent(c.id!)}/origin`);
    } catch (e) {
      setError(e);
      setBusy(false);
    }
  };

  // The main button says what's still missing instead of just looking off.
  let missing: string | null = null;
  if (name.trim() === "") missing = "Add a product name";
  else if (!/^\d{4}-\d{2}-\d{2}$/.test(date)) missing = "Add the case date";
  else if (tab === "paste" && facts.trim().length <= 20) missing = "Paste a description (a sentence or more)";
  else if (tab !== "paste" && !product.components.some((c) => c.name.trim() && c.made_in.trim())) missing = "Add a part and where it's made";
  const ready = missing === null;
  // Not ready: the button stays focusable and takes you to what's missing.
  const showMissing = () => {
    const field = name.trim() === "" ? "[data-field=name]" : missing === "Add the case date" ? "[data-field=date]" : "#step1-panel textarea, #step1-panel input";
    document.querySelector<HTMLElement>(field)?.focus();
  };
  const tiers = unlocks(tab, facts, product, economics);
  const countries = [...new Set((product.plants ?? []).map((p) => p.country.trim()).filter(Boolean))];
  const groups = productGroups(product);

  return (
    <ActionSlotProvider>
      <CaseTabs current="product" reached={1} title={name || "New case"} />
      <Main>
        <div className="min-w-0 max-w-[1020px]">
          <p className="eyebrow">Step 1 · Product{demoTitle && <> · demo: {demoTitle}</>}</p>
          <h1 className="mt-1 text-[28px] tracking-[-0.05em]">{copy.product.title}</h1>
          <p className="duo mt-1 max-w-[80ch] text-[13px] text-dim">
            {copy.product.lede}{" "}
            {!demoId && (
              <Link href="/" className="text-ok underline underline-offset-2">
                Or load a demo.
              </Link>
            )}
          </p>

          {/* ARIA tabs (#99): one Tab stop, arrow keys move between tabs. */}
          <div role="tablist" aria-label="How to describe the product" className="mt-7 inline-flex flex-wrap border border-edge bg-sheet">
            {tabOrder.map((t) => (
              <button
                key={t}
                id={`step1-tab-${t}`}
                role="tab"
                type="button"
                aria-selected={tab === t}
                aria-controls="step1-panel"
                tabIndex={tab === t ? 0 : -1}
                onClick={() => setTab(t)}
                onKeyDown={(e) => {
                  const i = tabOrder.indexOf(t);
                  const next =
                    e.key === "ArrowRight" ? tabOrder[(i + 1) % tabOrder.length]
                    : e.key === "ArrowLeft" ? tabOrder[(i + tabOrder.length - 1) % tabOrder.length]
                    : e.key === "Home" ? tabOrder[0]
                    : e.key === "End" ? tabOrder[tabOrder.length - 1]
                    : null;
                  if (!next) return;
                  e.preventDefault();
                  setTab(next);
                  document.getElementById(`step1-tab-${next}`)?.focus();
                }}
                className={cx("min-h-10 px-4 text-[13px]", tab === t ? "bg-ink text-paper" : "text-body hover:bg-wash")}
              >
                {copy.product.tabs[t]}
              </button>
            ))}
          </div>

          {drafted && tab === "parts" && (
            <div className="mt-5 border border-ask-line bg-ask-wash px-4 py-3 text-[13px]">
              <p className="font-medium text-ask">Drafted from your text. Check these before you continue.</p>
              {drafted.length > 0 && (
                <ul className="duo mt-1.5 flex list-disc flex-col gap-1 pl-5 text-ask">
                  {drafted.map((w) => (
                    <li key={w}>{w}</li>
                  ))}
                </ul>
              )}
            </div>
          )}

          <div className="mt-7" role="tabpanel" id="step1-panel" aria-labelledby={`step1-tab-${tab}`}>
            {tab === "parts" && <PartsEditor product={product} onChange={setProduct} />}
            {tab === "paste" && (
              <div className="grid gap-5 lg:grid-cols-[minmax(0,1fr)_280px]">
                <Field label={copy.product.facts}>
                  {(id) => (
                    <div className="border border-edge bg-white shadow-[4px_4px_0_var(--shadow)] focus-within:border-ok">
                      <textarea
                        id={id}
                        className="duo block min-h-[320px] w-full resize-y bg-transparent px-4 py-3.5 text-[13.5px] leading-[1.7] outline-none placeholder:text-faint"
                        maxLength={12000}
                        value={facts}
                        placeholder="Each heat gun is assembled in Thailand using 24 components sourced from China and Thailand. The main housing, front cover and heating element are produced in Thailand…"
                        onChange={(e) => setFacts(e.target.value)}
                        aria-describedby={`${id}-count`}
                      />
                      <div className="flex items-center justify-between gap-3 border-t border-dashed border-guide px-4 py-2 text-[11.5px] text-dim">
                        <span className="duo">{copy.product.factsHelp}</span>
                        <span id={`${id}-count`} className="shrink-0 tabular-nums">
                          {facts.length.toLocaleString("en-US")} / 12,000
                        </span>
                      </div>
                    </div>
                  )}
                </Field>
                <aside className="self-start border border-line bg-sheet px-4 py-3.5 text-[12.5px] lg:mt-6">
                  <p className="eyebrow mb-2">What helps</p>
                  <ul className="duo flex list-disc flex-col gap-1.5 pl-4 text-body">
                    <li>What the product is and what it does.</li>
                    <li>Every part and the country it&apos;s made in.</li>
                    <li>What happens in each country, step by step: moulding, winding, soldering, assembly, testing.</li>
                    <li>Write it like a ruling request to CBP. A ruling&apos;s own text works.</li>
                  </ul>
                </aside>
              </div>
            )}
            {tab === "bom" && (
              <BomImport
                name={name}
                family={family}
                imported={imported}
                onImported={(r) => {
                  setImported(r);
                  setProduct(r.product);
                  if (r.family) setFamily(r.family);
                  if (!name && r.product.name) setName(r.product.name);
                }}
                onEdit={() => setTab("parts")}
                onUseText={(text) => {
                  setFacts(text.slice(0, 12000));
                  setTab("paste");
                }}
              />
            )}
          </div>

          {/* After the input: name, family and date (an import fills the name). */}
          <div className="mt-7 grid gap-4 sm:grid-cols-[minmax(0,1.3fr)_minmax(0,1fr)_200px]">
            <Field label={copy.product.name}>
              {(id) => <input id={id} data-field="name" className={input} value={name} onChange={(e) => setName(e.target.value)} placeholder="e.g. Kobalt 12.5-amp heat gun" />}
            </Field>
            <Field label={copy.product.family}>
              {(id) => (
                <span className="relative block">
                  <select id={id} className={cx(input, "appearance-none pr-9")} value={family} onChange={(e) => setFamily(e.target.value)}>
                    {families.map((f) => (
                      <option key={f.id} value={f.id}>
                        {f.name}
                      </option>
                    ))}
                    {!families.some((f) => f.id === family) && <option value={family}>{family || "None"}</option>}
                  </select>
                  <Caret />
                </span>
              )}
            </Field>
            <Field label={copy.product.date} help={copy.product.dateHelp}>
              {(id) => <input id={id} data-field="date" type="date" className={cx(input, "tabular-nums")} value={date} onChange={(e) => setDate(e.target.value)} />}
            </Field>
          </div>
          {/* Below the input it describes: what it unlocks so far. */}
          <ul aria-label="What this input unlocks" className="mt-7 grid border border-edge bg-sheet shadow-[4px_4px_0_var(--shadow)] sm:grid-cols-3">
            {tiers.map((t, i) => (
              <li key={t.key} className={cx("flex gap-3 px-4 py-3.5", i > 0 && "border-t border-dashed border-guide sm:border-l sm:border-t-0")}>
                <span
                  aria-hidden="true"
                  className={cx(
                    "mt-0.5 inline-grid size-4 shrink-0 place-items-center border text-[11px] leading-none",
                    t.state === "done" ? "border-ok bg-ok text-white" : t.state === "limited" ? "border-ask text-ask" : "border-edge",
                  )}
                >
                  {t.state === "done" ? "✓" : t.state === "limited" ? "!" : ""}
                </span>
                <div className="min-w-0 text-[12.5px]">
                  <p className="font-medium text-ink">
                    {t.label}
                    <span className="sr-only">: {t.state === "done" ? "ready" : t.state === "limited" ? "limited" : "not yet"}</span>
                  </p>
                  <p className={cx("duo mt-0.5", t.state === "done" ? "text-ok" : t.state === "limited" ? "text-ask" : "text-dim")}>{t.note}</p>
                  {t.key === "plan" && tab === "paste" && (
                    <button
                      type="button"
                      onClick={draftParts}
                      disabled={facts.trim().length <= 20 || drafting !== null}
                      className="mt-2 inline-flex min-h-8 items-center gap-1.5 border border-ok bg-white px-2.5 text-[12px] text-ok shadow-[2px_2px_0_var(--shadow)] hover:bg-ok-wash disabled:cursor-not-allowed disabled:border-line disabled:text-dim disabled:shadow-none"
                    >
                      {drafting ? (
                        <>
                          Drafting… <Elapsed since={drafting.since} />
                        </>
                      ) : (
                        "✦ Turn into parts and plants"
                      )}
                    </button>
                  )}
                  {t.key === "money" && (
                    <button type="button" onClick={() => setMoneyOpen(true)} className="mt-2 min-h-7 text-[12px] text-ok hover:underline">
                      {hasAnyMoney(economics) ? "Edit money inputs" : "+ Add money inputs"}
                    </button>
                  )}
                </div>
              </li>
            ))}
          </ul>
          {tab !== "paste" && product.components.length > 0 && (
            <p className="mt-3 text-[12px] text-dim">
              {product.components.length} components · {product.operations?.length ?? 0} operations · {product.plants?.length ?? 0} plants · {groups.length} groups to move
            </p>
          )}

          {error ? (
            <div className="mt-6">
              <ErrorBox error={error} onRetry={submit} />
            </div>
          ) : null}
          {/* Step 1 is a form: its action sits after the fields, not above the heading. */}
          <div className="mt-8 flex flex-wrap items-center gap-3 border-t border-line pt-4">
            <Next>{missing ? "Describe the product, then find its likely origin." : "Find the likely origin. It takes a few seconds."}</Next>
            <div className="ml-auto flex items-center gap-3">
              {busy ? (
                <Spinner label={copy.product.working} />
              ) : missing ? (
                <span id="go-why" className="text-[13px] text-dim">
                  {missing}
                </span>
              ) : null}
              <Button onClick={ready ? submit : showMissing} disabled={busy} aria-disabled={!ready} aria-describedby={missing ? "go-why" : undefined}>
                {copy.product.go} →
              </Button>
            </div>
          </div>
        </div>
      </Main>
      {moneyOpen && (
        <MoneyDrawer
          value={economics}
          product={product}
          countries={countries}
          onClose={() => setMoneyOpen(false)}
          onSave={(e) => {
            setEconomics(e);
            setMoneyOpen(false);
          }}
        />
      )}
    </ActionSlotProvider>
  );
}

// Inputs read as drawing-sheet fields: a firm edge on white, a blue edge and
// a hard offset shadow when focused (the focus ring stays for keyboards).
const input =
  "h-11 w-full rounded-[2px] border border-edge bg-white px-3.5 text-[13.5px] text-ink placeholder:text-faint transition-shadow focus:border-ok focus:shadow-[3px_3px_0_var(--shadow)]";

// Table cells edit in place: quiet until hovered or focused.
const cell =
  "h-9 w-full rounded-[2px] border border-transparent bg-transparent px-2 text-[13px] text-ink placeholder:text-faint hover:border-line focus:border-ok focus:bg-white";

function Caret() {
  return (
    <svg aria-hidden="true" viewBox="0 0 10 6" className="pointer-events-none absolute right-3.5 top-1/2 size-2.5 -translate-y-1/2 text-dim">
      <path d="M0 0h10L5 6z" fill="currentColor" />
    </svg>
  );
}

function Field({ label, help, children }: { label: string; help?: string; children: (id: string) => ReactNode }) {
  const id = useId();
  return (
    <div>
      <label htmlFor={id} className="block text-[12.5px] text-body">
        {label}
      </label>
      <div className="mt-1.5">{children(id)}</div>
      {help && <p className="duo mt-1.5 text-[11.5px] text-dim">{help}</p>}
    </div>
  );
}

// Plant IDs are made for the user ("th", "th2", …) and never shown.
function plantId(taken: string[]) {
  let n = taken.length + 1;
  while (taken.includes(`p${n}`)) n++;
  return `p${n}`;
}

function PartsEditor({ product, onChange }: { product: Product; onChange: (p: Product) => void }) {
  const plants = product.plants ?? [];
  const ops = product.operations ?? [];
  const setPlants = (p: Plant[]) => onChange({ ...product, plants: p });
  const setComps = (c: Component[]) => onChange({ ...product, components: c });
  const setOps = (o: Operation[]) => onChange({ ...product, operations: o });
  const groups = [...new Set([...groupSuggestions, ...[...product.components, ...ops].map((x) => x.group).filter((g): g is string => Boolean(g))])];
  // Picking where a part is made fills its country from the plant.
  const pickPlant = (i: number, id: string) => {
    const pl = plants.find((p) => p.id === id);
    setComps(product.components.map((c, j) => (j === i ? { ...c, plant: id || undefined, made_in: pl?.country || c.made_in } : c)));
  };

  return (
    <div className="flex min-w-0 flex-col gap-6">
      <Panel className="min-w-0">
        <PanelHead right={plants.length ? `${plants.length} plants` : undefined}>Plants</PanelHead>
        {plants.length === 0 && <p className="duo mb-1 text-[13px] text-dim">Where the product, or any part of it, is made: your plants and your suppliers.</p>}
        <ul className="flex flex-col gap-3">
          {plants.map((p, i) => (
            <li key={p.id} className="border border-line bg-white p-3">
              <div className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto]">
                <input
                  aria-label="Plant name"
                  placeholder="Plant name, e.g. Rayong plant"
                  className={cell}
                  value={p.name ?? ""}
                  onChange={(e) => setPlants(plants.map((x, j) => (j === i ? { ...x, name: e.target.value } : x)))}
                />
                <input
                  aria-label="Plant country"
                  placeholder="Country"
                  className={cell}
                  value={p.country}
                  onChange={(e) => setPlants(plants.map((x, j) => (j === i ? { ...x, country: e.target.value } : x)))}
                />
                <RemoveButton label={`Remove ${p.name || "plant"}`} onClick={() => setPlants(plants.filter((_, j) => j !== i))} />
              </div>
              <fieldset className="mt-2.5">
                <legend className="text-[11px] text-dim">Could also make (used by the plan search)</legend>
                <div className="mt-1.5 flex flex-wrap gap-1.5">
                  {groups.map((g) => {
                    const on = (p.capabilities ?? []).includes(g);
                    return (
                      <button
                        key={g}
                        type="button"
                        aria-pressed={on}
                        onClick={() =>
                          setPlants(plants.map((x, j) => (j === i ? { ...x, capabilities: on ? (x.capabilities ?? []).filter((c) => c !== g) : [...(x.capabilities ?? []), g] } : x)))
                        }
                        className={cx("min-h-7 rounded-[2px] border px-2 text-xs", on ? "border-ok bg-ok-wash text-ok" : "border-line text-dim hover:text-ink")}
                      >
                        {on ? "✓ " : ""}
                        {groupName(g)}
                      </button>
                    );
                  })}
                </div>
              </fieldset>
            </li>
          ))}
        </ul>
        <AddButton onClick={() => setPlants([...plants, { id: plantId(plants.map((x) => x.id)), name: "", country: "", capabilities: [] }])}>+ Add a plant</AddButton>
      </Panel>

      <Panel className="min-w-0">
        <PanelHead right={product.components.length ? `${product.components.length} parts` : undefined}>Parts</PanelHead>
        {product.components.length === 0 && <p className="duo mb-1 text-[13px] text-dim">Each component and where it&apos;s made.</p>}
        {product.components.length > 0 && (
          <div className="relative overflow-x-auto">
            <table className="w-full min-w-[620px] border-collapse text-sm">
              <thead>
                <tr>
                  {["Part", "Made at", "Country", "Group"].map((h) => (
                    <th key={h} scope="col" className="pb-2 pr-2 text-left text-[11px] font-medium uppercase tracking-[0.08em] text-dim">
                      {h}
                    </th>
                  ))}
                  <th className="w-8">
                    <span className="sr-only">Remove</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {product.components.map((c, i) => (
                  <tr key={i}>
                    <td className="py-1 pr-2">
                      <input aria-label="Part name" className={cell} value={c.name} onChange={(e) => setComps(product.components.map((x, j) => (j === i ? { ...x, name: e.target.value } : x)))} />
                    </td>
                    <td className="py-1 pr-2">
                      <select aria-label="Made at" className={cell} value={c.plant ?? ""} onChange={(e) => pickPlant(i, e.target.value)}>
                        <option value="">A supplier not listed</option>
                        {plants.map((p) => (
                          <option key={p.id} value={p.id}>
                            {p.name || "Unnamed plant"}
                            {p.country ? ` · ${p.country}` : ""}
                          </option>
                        ))}
                      </select>
                    </td>
                    <td className="py-1 pr-2">
                      <input aria-label="Country it's made in" className={cell} value={c.made_in} onChange={(e) => setComps(product.components.map((x, j) => (j === i ? { ...x, made_in: e.target.value } : x)))} />
                    </td>
                    <td className="py-1 pr-2">
                      <select
                        aria-label="Operation group"
                        className={cell}
                        value={c.group ?? ""}
                        onChange={(e) => setComps(product.components.map((x, j) => (j === i ? { ...x, group: e.target.value || undefined } : x)))}
                      >
                        <option value="">None</option>
                        {groups.map((g) => (
                          <option key={g} value={g}>
                            {groupName(g)}
                          </option>
                        ))}
                      </select>
                    </td>
                    <td>
                      <RemoveButton label={`Remove ${c.name || "part"}`} onClick={() => setComps(product.components.filter((_, j) => j !== i))} />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        <AddButton onClick={() => setComps([...product.components, { name: "", made_in: "" }])}>+ Add a part</AddButton>
      </Panel>

      <details className="border border-line bg-sheet px-5 py-3" open={ops.length > 0}>
        <summary className="cursor-pointer text-[13px] text-dim">Operations at each plant (optional){ops.length ? ` · ${ops.length} steps` : ""}</summary>
        <ul className="mt-3 flex flex-col gap-2">
          {ops.map((o, i) => (
            <li key={i} className="grid gap-2 sm:grid-cols-[minmax(0,2fr)_minmax(0,1fr)_minmax(0,1fr)_auto]">
              <input aria-label="Processing step" placeholder="e.g. solder, test and pack" className={cell} value={o.name} onChange={(e) => setOps(ops.map((x, j) => (j === i ? { ...x, name: e.target.value } : x)))} />
              <select aria-label="At plant" className={cell} value={o.plant} onChange={(e) => setOps(ops.map((x, j) => (j === i ? { ...x, plant: e.target.value } : x)))}>
                {plants.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name || "Unnamed plant"}
                  </option>
                ))}
              </select>
              <select aria-label="Operation group" className={cell} value={o.group ?? ""} onChange={(e) => setOps(ops.map((x, j) => (j === i ? { ...x, group: e.target.value || undefined } : x)))}>
                <option value="">None</option>
                {groups.map((g) => (
                  <option key={g} value={g}>
                    {groupName(g)}
                  </option>
                ))}
              </select>
              <RemoveButton label="Remove step" onClick={() => setOps(ops.filter((_, j) => j !== i))} />
            </li>
          ))}
        </ul>
        {plants.length > 0 ? <AddButton onClick={() => setOps([...ops, { name: "", plant: plants[0].id }])}>+ Add a processing step</AddButton> : <p className="mt-2 text-xs text-dim">Add a plant first.</p>}
      </details>
    </div>
  );
}

function AddButton({ onClick, children }: { onClick: () => void; children: ReactNode }) {
  return (
    <button type="button" onClick={onClick} className="mt-3 min-h-8 text-[13px] text-ok hover:underline">
      {children}
    </button>
  );
}

function RemoveButton({ label, onClick }: { label: string; onClick: () => void }) {
  return (
    <button type="button" aria-label={label} onClick={onClick} className="inline-grid size-8 place-items-center text-dim hover:text-fail">
      ×
    </button>
  );
}

function BomImport({
  name,
  family,
  imported,
  onImported,
  onEdit,
  onUseText,
}: {
  name: string;
  family: string;
  imported: ImportResponse | null;
  onImported: (r: ImportResponse) => void;
  onEdit: () => void;
  onUseText: (text: string) => void;
}) {
  const [file, setFile] = useState<File | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [pdfText, setPdfText] = useState<string | null>(null);
  const [drag, setDrag] = useState(false);
  const inputId = useId();

  const upload = async (f: File) => {
    const productName = name.trim() || f.name.replace(/\.[^.]+$/, "").replace(/[-_]+/g, " ");
    setFile(f);
    setBusy(true);
    setError(null);
    setPdfText(null);
    // PDFs are read in the browser: the table becomes CSV for the same import.
    if (f.type === "application/pdf" || f.name.toLowerCase().endsWith(".pdf")) {
      try {
        const { csv } = await pdfToBomCsv(f);
        onImported(await api.importCsv({ csv, name: productName, family: family || undefined }));
      } catch (e) {
        if (e instanceof PdfTableError) setPdfText(e.text);
        setError(e);
      } finally {
        setBusy(false);
      }
      return;
    }
    const form = new FormData();
    form.append("file", f);
    form.append("name", productName);
    if (family) form.append("family", family);
    try {
      onImported(await api.importBom(form));
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

  const p = imported?.product;
  const groups = new Set([...(p?.components ?? []), ...(p?.operations ?? [])].map((x) => x.group).filter(Boolean));

  return (
    <div className="flex flex-col gap-5">
      <label
        htmlFor={inputId}
        onDragOver={(e) => {
          e.preventDefault();
          setDrag(true);
        }}
        onDragLeave={() => setDrag(false)}
        onDrop={(e) => {
          e.preventDefault();
          setDrag(false);
          const f = e.dataTransfer.files[0];
          if (f) upload(f);
        }}
        className={cx("block cursor-pointer border border-dashed px-6 py-8 text-center", drag ? "border-ok bg-ok-wash" : "border-guide bg-sheet")}
      >
        <span className="text-sm">{file ? `${file.name} · replace file` : copy.product.bomDrop}</span>
        <span className="duo mt-2 block text-xs text-dim">{copy.product.bomGuide}</span>
        <input id={inputId} type="file" accept=".csv,.xlsx,.pdf,text/csv,application/pdf,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" className="sr-only" onChange={(e) => e.target.files?.[0] && upload(e.target.files[0])} />
      </label>
      {busy && <Spinner label="Importing…" />}
      {error ? <ErrorBox error={error} /> : null}
      {pdfText && pdfText.trim().length > 20 && (
        <p className="duo text-[13px]">
          No parts table found, but the PDF has text.{" "}
          <button type="button" className="text-ok underline underline-offset-2" onClick={() => onUseText(pdfText)}>
            Use its text as the description
          </button>
        </p>
      )}
      <p className="text-xs text-dim">
        Test files (heat gun, 17 rows):{" "}
        <a className="text-ok underline underline-offset-2" href="/samples/heatgun-bom.csv" download>
          CSV
        </a>{" "}
        ·{" "}
        <a className="text-ok underline underline-offset-2" href="/samples/heatgun-bom.xlsx" download>
          Excel
        </a>{" "}
        ·{" "}
        <a className="text-ok underline underline-offset-2" href="/samples/heatgun-bom.pdf" download>
          PDF
        </a>
      </p>
      {imported && p && (
        <Panel>
          <PanelHead right="Ready for the plan search">Imported</PanelHead>
          <p className="text-sm">
            {p.components.length} components · {p.operations?.length ?? 0} operations · {p.plants?.length ?? 0} plants · {groups.size} groups
          </p>
          {imported.warnings.length > 0 && (
            <ul className="duo mt-3 flex flex-col gap-1 border-l-2 border-ask-line pl-3 text-[13px] text-ask">
              {imported.warnings.map((w) => (
                <li key={w}>{w}</li>
              ))}
            </ul>
          )}
          <div className="mt-4 grid gap-3 sm:grid-cols-2">
            {(p.plants ?? []).map((pl) => (
              <div key={pl.id} className="border border-line bg-white p-3 text-[13px]">
                <Country name={pl.country} /> <span className="text-dim">· {pl.name || pl.id}</span>
                <div className="mt-1 flex flex-wrap gap-1">
                  {(pl.capabilities ?? []).map((c) => (
                    <Chip key={c}>{c}</Chip>
                  ))}
                </div>
              </div>
            ))}
          </div>
          <button type="button" onClick={onEdit} className="mt-4 text-[13px] text-ok hover:underline">
            Edit in Parts and plants →
          </button>
        </Panel>
      )}
    </div>
  );
}
