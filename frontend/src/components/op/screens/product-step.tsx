"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { useEffect, useId, useState, type ReactNode } from "react";
import { api, type Component, type ImportResponse, type Operation, type Plant, type PredictRequest, type Product } from "@/lib/api/client";
import { copy } from "@/lib/copy";
import { rememberDemo } from "@/lib/demo-store";
import { CaseTabs, ErrorBox, Footer, Main } from "../chrome";
import { groupName } from "../plan-bits";
import { PdfTableError, pdfToBomCsv } from "@/lib/pdf-bom";
import { Button, Chip, Country, Panel, PanelHead, Spinner, cx } from "../ui";

type Tab = "parts" | "paste" | "bom";

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

type Tier = { key: string; label: string; state: "done" | "limited" | "missing"; note: string };

// What each kind of input unlocks (DESIGN.md step 1, the "unlocks" strip).
function unlocks(tab: Tab, facts: string, product: Product, hasMoney: boolean): Tier[] {
  const plants = product.plants ?? [];
  const parts = product.components.filter((c) => c.name.trim() && c.made_in.trim());
  const countries = new Set(plants.map((p) => p.country.trim()).filter(Boolean));
  const extra = plants.some((p) => (p.capabilities ?? []).length > 0);
  const origin = tab === "paste" ? facts.trim().length > 20 : parts.length > 0;
  const plan = tab !== "paste" && countries.size >= 2 && extra;
  return [
    {
      key: "origin",
      label: "Likely origin",
      state: origin ? "done" : "missing",
      note: origin ? (tab === "paste" ? "from your description" : `${parts.length} parts`) : tab === "paste" ? "paste a description" : "add a part and where it's made",
    },
    {
      key: "plan",
      label: "Plan search",
      state: plan ? "done" : tab !== "paste" && countries.size === 1 ? "limited" : "missing",
      note: plan
        ? `${countries.size} countries`
        : tab === "paste"
          ? "needs parts and plants"
          : countries.size < 2
            ? "add a plant in a second country"
            : "mark what a plant could also make",
    },
    { key: "money", label: "Money per move", state: hasMoney ? "done" : "missing", note: hasMoney ? "illustrative" : "add later on the Plan step" },
  ];
}

export function ProductStep() {
  const router = useRouter();
  const params = useSearchParams();
  const demoId = params.get("demo");

  const [tab, setTab] = useState<Tab>("paste");
  const [name, setName] = useState("");
  const [date, setDate] = useState(today);
  const [family, setFamily] = useState("motorised_tools_small_appliances");
  const [facts, setFacts] = useState("");
  const [product, setProduct] = useState<Product>(emptyProduct);
  const [extra, setExtra] = useState<Pick<PredictRequest, "exclude_rulings" | "current_origin" | "facts">>({});
  const [demoTitle, setDemoTitle] = useState<string | null>(null);
  const [imported, setImported] = useState<ImportResponse | null>(null);
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
      setExtra({ exclude_rulings: p.exclude_rulings, current_origin: p.current_origin, facts: p.product ? p.facts : undefined });
    }, setError);
  }, [demoId]);

  const submit = async () => {
    setBusy(true);
    setError(null);
    const body: PredictRequest = { date, exclude_rulings: extra.exclude_rulings, current_origin: extra.current_origin };
    if (tab === "paste") {
      body.facts = facts;
      // Step 1 always sends a product name, so the Cases list never shows
      // truncated facts.
      body.product = { name: name.trim(), components: [] };
    } else {
      body.product = { ...product, name: name.trim() };
      body.family = family || undefined;
      if (extra.facts) body.facts = extra.facts; // demo: engine reads CBP's own text, product kept for the plan
    }
    try {
      const c = await api.predict(body);
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
  const tiers = unlocks(tab, facts, product, Boolean(extra.facts));

  return (
    <>
      <CaseTabs current="product" reached={1} title={name || "New case"} />
      <Main>
        <div className="min-w-0 max-w-[880px]">
          <p className="eyebrow">Step 1 · Product{demoTitle && <> · demo: {demoTitle}</>}</p>
          <h1 className="mt-1 text-[28px] tracking-[-0.05em]">{copy.product.title}</h1>
          <p className="duo mt-1 text-dim">{copy.product.lede}</p>

          <div className="mt-6 grid gap-4 sm:grid-cols-[minmax(0,1fr)_200px]">
            <Field label={copy.product.name}>
              {(id) => <input id={id} className={input} value={name} onChange={(e) => setName(e.target.value)} placeholder="e.g. Kobalt 12.5-amp heat gun" />}
            </Field>
            <Field label={copy.product.date} help={copy.product.dateHelp}>
              {(id) => <input id={id} type="date" className={input} value={date} onChange={(e) => setDate(e.target.value)} />}
            </Field>
          </div>

          <div role="tablist" aria-label="How to describe the product" className="mt-8 flex flex-wrap gap-1 border-b border-line">
            {(["paste", "parts", "bom"] as Tab[]).map((t) => (
              <button
                key={t}
                role="tab"
                type="button"
                aria-selected={tab === t}
                onClick={() => setTab(t)}
                className={cx("-mb-px border px-4 py-2 text-sm", tab === t ? "border-line border-b-paper bg-paper font-medium" : "border-transparent text-dim hover:text-ink")}
              >
                {copy.product.tabs[t]}
              </button>
            ))}
          </div>

          <ul aria-label="What your input unlocks" className="mt-4 grid gap-2 sm:grid-cols-3">
            {tiers.map((t) => (
              <li
                key={t.key}
                className={cx(
                  "border px-3 py-2 text-[12.5px]",
                  t.state === "done" && "border-ok bg-ok-wash",
                  t.state === "limited" && "border-ask-line bg-ask-wash",
                  t.state === "missing" && "border-dashed border-guide",
                )}
              >
                <b className={cx("block font-medium", t.state === "done" ? "text-ok" : t.state === "limited" ? "text-ask" : "text-dim")}>
                  {t.state === "done" ? "✓ " : t.state === "limited" ? "! " : ""}
                  {t.label}
                </b>
                <span className="text-dim">{t.note}</span>
              </li>
            ))}
          </ul>

          <div className="mt-6" role="tabpanel">
            {tab === "parts" && <PartsEditor product={product} onChange={setProduct} family={family} onFamily={setFamily} />}
            {tab === "paste" && (
              <Field label={copy.product.facts} help={copy.product.factsHelp}>
                {(id) => (
                  <textarea
                    id={id}
                    className={cx(input, "duo min-h-[260px] leading-relaxed")}
                    maxLength={12000}
                    value={facts}
                    onChange={(e) => setFacts(e.target.value)}
                  />
                )}
              </Field>
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
          {error ? (
            <div className="mt-6">
              <ErrorBox error={error} onRetry={submit} />
            </div>
          ) : null}
        </div>
      </Main>
      <Footer>
        <span className="text-xs text-dim">Nothing is analysed until you continue</span>
        <div className="ml-auto flex items-center gap-3">
          {busy ? (
            <Spinner label={copy.product.working} />
          ) : missing ? (
            <span id="go-why" className="text-[13px] text-dim">
              {missing}
            </span>
          ) : null}
          <Button onClick={submit} disabled={!ready || busy} aria-describedby={missing ? "go-why" : undefined}>
            {copy.product.go} →
          </Button>
        </div>
      </Footer>
    </>
  );
}

const input = "w-full rounded-[2px] border border-line bg-white px-3 py-2 text-sm placeholder:text-faint";

function Field({ label, help, children }: { label: string; help?: string; children: (id: string) => ReactNode }) {
  const id = useId();
  return (
    <div>
      <label htmlFor={id} className="eyebrow block">
        {label}
      </label>
      <div className="mt-1.5">{children(id)}</div>
      {help && <p className="duo mt-1 text-xs text-dim">{help}</p>}
    </div>
  );
}

// Plant IDs are made for the user ("th", "th2", …) and never shown.
function plantId(taken: string[]) {
  let n = taken.length + 1;
  while (taken.includes(`p${n}`)) n++;
  return `p${n}`;
}

function PartsEditor({ product, onChange, family, onFamily }: { product: Product; onChange: (p: Product) => void; family: string; onFamily: (f: string) => void }) {
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
                  className={cx(input, "py-1.5")}
                  value={p.name ?? ""}
                  onChange={(e) => setPlants(plants.map((x, j) => (j === i ? { ...x, name: e.target.value } : x)))}
                />
                <input
                  aria-label="Plant country"
                  placeholder="Country"
                  className={cx(input, "py-1.5")}
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
                    <th key={h} scope="col" className="pb-2 pr-2 text-left text-[10.5px] font-medium uppercase tracking-[0.08em] text-dim">
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
                      <input aria-label="Part name" className={cx(input, "py-1.5")} value={c.name} onChange={(e) => setComps(product.components.map((x, j) => (j === i ? { ...x, name: e.target.value } : x)))} />
                    </td>
                    <td className="py-1 pr-2">
                      <select aria-label="Made at" className={cx(input, "py-1.5")} value={c.plant ?? ""} onChange={(e) => pickPlant(i, e.target.value)}>
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
                      <input aria-label="Country it's made in" className={cx(input, "py-1.5")} value={c.made_in} onChange={(e) => setComps(product.components.map((x, j) => (j === i ? { ...x, made_in: e.target.value } : x)))} />
                    </td>
                    <td className="py-1 pr-2">
                      <select
                        aria-label="Operation group"
                        className={cx(input, "py-1.5")}
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
        <summary className="cursor-pointer text-[13px] text-dim">Processing at each plant and product family (optional){ops.length ? ` · ${ops.length} steps` : ""}</summary>
        <ul className="mt-3 flex flex-col gap-2">
          {ops.map((o, i) => (
            <li key={i} className="grid gap-2 sm:grid-cols-[minmax(0,2fr)_minmax(0,1fr)_minmax(0,1fr)_auto]">
              <input aria-label="Processing step" placeholder="e.g. solder, test and pack" className={cx(input, "py-1.5")} value={o.name} onChange={(e) => setOps(ops.map((x, j) => (j === i ? { ...x, name: e.target.value } : x)))} />
              <select aria-label="At plant" className={cx(input, "py-1.5")} value={o.plant} onChange={(e) => setOps(ops.map((x, j) => (j === i ? { ...x, plant: e.target.value } : x)))}>
                {plants.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name || "Unnamed plant"}
                  </option>
                ))}
              </select>
              <select aria-label="Operation group" className={cx(input, "py-1.5")} value={o.group ?? ""} onChange={(e) => setOps(ops.map((x, j) => (j === i ? { ...x, group: e.target.value || undefined } : x)))}>
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
        <div className="mt-4 max-w-sm">
          <Field label="Product family" help="Picks the planner template for the plan search.">
            {(id) => <input id={id} className={input} value={family} onChange={(e) => onFamily(e.target.value)} />}
          </Field>
        </div>
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
    setFile(f);
    setBusy(true);
    setError(null);
    setPdfText(null);
    // PDFs are read in the browser: the table becomes CSV for the same import.
    if (f.type === "application/pdf" || f.name.toLowerCase().endsWith(".pdf")) {
      try {
        const { csv } = await pdfToBomCsv(f);
        onImported(await api.importCsv({ csv, name: name || undefined, family: family || undefined }));
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
    if (name) form.append("name", name);
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
