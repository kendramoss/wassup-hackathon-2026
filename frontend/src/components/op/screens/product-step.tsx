"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { useEffect, useId, useState, type ReactNode } from "react";
import { api, type Component, type ImportResponse, type Operation, type Plant, type PredictRequest, type Product } from "@/lib/api/client";
import { copy } from "@/lib/copy";
import { rememberDemo } from "@/lib/demo-store";
import { ErrorBox, Footer, Main, StepBar } from "../chrome";
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

const emptyProduct = (): Product => ({
  name: "",
  plants: [{ id: "p1", name: "", country: "", capabilities: [] }],
  components: [{ name: "", made_in: "" }],
  operations: [],
});

export function ProductStep() {
  const router = useRouter();
  const params = useSearchParams();
  const demoId = params.get("demo");

  const [tab, setTab] = useState<Tab>("parts");
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

  const ready =
    name.trim() !== "" &&
    /^\d{4}-\d{2}-\d{2}$/.test(date) &&
    (tab === "paste" ? facts.trim().length > 20 : product.components.some((c) => c.name.trim() && c.made_in.trim()));

  return (
    <>
      <StepBar current={1} reached={1} title={name || "New case"} />
      <Main>
        <div className="max-w-[880px]">
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
            {(Object.keys(copy.product.tabs) as Tab[]).map((t) => (
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
          {busy && <Spinner label={copy.product.working} />}
          <Button onClick={submit} disabled={!ready || busy}>
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

function PartsEditor({ product, onChange, family, onFamily }: { product: Product; onChange: (p: Product) => void; family: string; onFamily: (f: string) => void }) {
  const plants = product.plants ?? [];
  const ops = product.operations ?? [];
  const setPlants = (p: Plant[]) => onChange({ ...product, plants: p });
  const setComps = (c: Component[]) => onChange({ ...product, components: c });
  const setOps = (o: Operation[]) => onChange({ ...product, operations: o });
  const listId = useId();

  return (
    <div className="flex flex-col gap-7">
      <datalist id={listId}>
        {groupSuggestions.map((g) => (
          <option key={g} value={g} />
        ))}
      </datalist>
      <Panel>
        <PanelHead right={`${plants.length} plants`}>Plants</PanelHead>
        <Rows
          head={["ID", "Name", "Country", "Can also make (groups, ; separated)"]}
          rows={plants.map((p, i) => [
            <Cell key="id" label="Plant ID" value={p.id} onChange={(v) => setPlants(plants.map((x, j) => (j === i ? { ...x, id: v } : x)))} />,
            <Cell key="n" label="Plant name" value={p.name ?? ""} onChange={(v) => setPlants(plants.map((x, j) => (j === i ? { ...x, name: v } : x)))} />,
            <Cell key="c" label="Plant country" value={p.country} onChange={(v) => setPlants(plants.map((x, j) => (j === i ? { ...x, country: v } : x)))} />,
            <Cell
              key="cap"
              label="Capabilities"
              value={(p.capabilities ?? []).join("; ")}
              onChange={(v) => setPlants(plants.map((x, j) => (j === i ? { ...x, capabilities: v.split(";").map((s) => s.trim()).filter(Boolean) } : x)))}
            />,
          ])}
          onRemove={(i) => setPlants(plants.filter((_, j) => j !== i))}
          onAdd={() => setPlants([...plants, { id: `p${plants.length + 1}`, name: "", country: "", capabilities: [] }])}
          addLabel="+ Add plant"
        />
      </Panel>
      <Panel>
        <PanelHead right={`${product.components.length} parts`}>Parts</PanelHead>
        <Rows
          head={["Part", "Made in", "Plant", "Group"]}
          rows={product.components.map((c, i) => [
            <Cell key="n" label="Part name" value={c.name} onChange={(v) => setComps(product.components.map((x, j) => (j === i ? { ...x, name: v } : x)))} />,
            <Cell key="m" label="Made in" value={c.made_in} onChange={(v) => setComps(product.components.map((x, j) => (j === i ? { ...x, made_in: v } : x)))} />,
            <Cell key="p" label="Plant ID" value={c.plant ?? ""} onChange={(v) => setComps(product.components.map((x, j) => (j === i ? { ...x, plant: v || undefined } : x)))} />,
            <Cell key="g" label="Group" list={listId} value={c.group ?? ""} onChange={(v) => setComps(product.components.map((x, j) => (j === i ? { ...x, group: v || undefined } : x)))} />,
          ])}
          onRemove={(i) => setComps(product.components.filter((_, j) => j !== i))}
          onAdd={() => setComps([...product.components, { name: "", made_in: "" }])}
          addLabel="+ Add part"
        />
      </Panel>
      <Panel>
        <PanelHead right={`${ops.length} steps`}>Processing at each plant</PanelHead>
        <Rows
          head={["Operation", "Plant", "Group"]}
          rows={ops.map((o, i) => [
            <Cell key="n" label="Operation" value={o.name} onChange={(v) => setOps(ops.map((x, j) => (j === i ? { ...x, name: v } : x)))} />,
            <Cell key="p" label="Plant ID" value={o.plant} onChange={(v) => setOps(ops.map((x, j) => (j === i ? { ...x, plant: v } : x)))} />,
            <Cell key="g" label="Group" list={listId} value={o.group ?? ""} onChange={(v) => setOps(ops.map((x, j) => (j === i ? { ...x, group: v || undefined } : x)))} />,
          ])}
          onRemove={(i) => setOps(ops.filter((_, j) => j !== i))}
          onAdd={() => setOps([...ops, { name: "", plant: plants[0]?.id ?? "" }])}
          addLabel="+ Add operation"
        />
      </Panel>
      <div className="max-w-sm">
        <Field label="Product family" help="Picks the planner template for the plan search.">
          {(id) => <input id={id} className={input} value={family} onChange={(e) => onFamily(e.target.value)} />}
        </Field>
      </div>
    </div>
  );
}

function Cell({ label, value, onChange, list }: { label: string; value: string; onChange: (v: string) => void; list?: string }) {
  return <input aria-label={label} list={list} className={cx(input, "py-1.5")} value={value} onChange={(e) => onChange(e.target.value)} />;
}

function Rows({ head, rows, onRemove, onAdd, addLabel }: { head: string[]; rows: ReactNode[][]; onRemove: (i: number) => void; onAdd: () => void; addLabel: string }) {
  return (
    <div className="overflow-x-auto">
      <table className="w-full min-w-[620px] border-collapse text-sm">
        <thead>
          <tr>
            {head.map((h) => (
              <th key={h} scope="col" className="pb-2 pr-2 text-left text-[10.5px] font-medium uppercase tracking-[0.08em] text-dim">
                {h}
              </th>
            ))}
            <th className="w-8" />
          </tr>
        </thead>
        <tbody>
          {rows.map((cells, i) => (
            <tr key={i}>
              {cells.map((c, j) => (
                <td key={j} className="py-1 pr-2">
                  {c}
                </td>
              ))}
              <td>
                <button type="button" aria-label="Remove row" onClick={() => onRemove(i)} className="px-2 text-dim hover:text-fail">
                  ×
                </button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      <button type="button" onClick={onAdd} className="mt-2 text-[13px] text-ok hover:underline">
        {addLabel}
      </button>
    </div>
  );
}

function BomImport({
  name,
  family,
  imported,
  onImported,
  onEdit,
}: {
  name: string;
  family: string;
  imported: ImportResponse | null;
  onImported: (r: ImportResponse) => void;
  onEdit: () => void;
}) {
  const [file, setFile] = useState<File | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [drag, setDrag] = useState(false);
  const inputId = useId();

  const upload = async (f: File) => {
    setFile(f);
    setBusy(true);
    setError(null);
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
        <input id={inputId} type="file" accept=".csv,.xlsx,text/csv,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" className="sr-only" onChange={(e) => e.target.files?.[0] && upload(e.target.files[0])} />
      </label>
      {busy && <Spinner label="Importing…" />}
      {error ? <ErrorBox error={error} /> : null}
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
