"use client";

// The business view's first step (/business/check, #893; canvas mock "Check
// a product", option A). Two short steps in plain words: the product and
// the country it is imported into, then the main parts and where it is put
// together. "Check my product" runs the same predict as the full form
// (/cases/new) and opens the answer. A parts list file dropped beside the
// form (CSV, Excel, PDF) fills it in with the same import as the full form,
// and while the list is unchanged the check sends the whole imported product
// (plants, steps, family, money), as the technical page does. The full form
// stays one link away for anything more (dates, photos and scans, plants).
//
// The step lives in the URL (?step=2) and the draft in sessionStorage, so
// Back returns to step 1 and a refresh keeps what was typed (#929). Focus
// moves to each step's heading (#928). Countries are matched to the list,
// and each problem is shown beside its field (#933).
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useId, useRef, useState, useSyncExternalStore, type FormEvent, type KeyboardEvent } from "react";
import { streamPredict, type Component, type ImportResponse, type PredictRequest, type PredictStage } from "@/lib/api/client";
import { predictBody } from "@/lib/product-facts-reconcile";
import { isSigninRequired, canCreate } from "@/lib/roles";
import { useWorkspaceRole } from "@/lib/use-role";
import type { MarketCode } from "@/lib/market";
import { allCountries, countryCodes } from "@/lib/countries";
import { BusinessBar } from "./business-bar";
import { BomDrop } from "./business-bom-drop";
import "./business.css";

// The countries a product can be checked for, by region (#945, canvas
// Y-Step1), with what each gets today in plain words (copy.market.summary).
// Several can be ticked: each gets its own check, run at the same time.
type Choice = { code: MarketCode; name: string; what: string };
const regions: { name: string; items: Choice[] }[] = [
  {
    name: "North America",
    items: [
      { code: "us", name: "United States", what: "Full check, with the import tax and changes that could lower it" },
      { code: "ca", name: "Canada", what: "Rules check only" },
      { code: "mx", name: "Mexico", what: "Rules check only" },
    ],
  },
  {
    name: "Europe",
    items: [
      { code: "eu", name: "European Union", what: "Rules check, some goods only" },
      { code: "uk", name: "United Kingdom", what: "Rules check, some goods only" },
    ],
  },
  {
    name: "Asia and Pacific",
    items: [
      { code: "kr", name: "South Korea", what: "Rules check only" },
      { code: "au", name: "Australia", what: "Rules check only" },
    ],
  },
];
const choices = regions.flatMap((r) => r.items);
const marketCodes = new Set<string>(choices.map((m) => m.code));
export const marketName = (code: string) => choices.find((m) => m.code === code)?.name ?? code;
// Ticked countries in the list's order, the United States first.
const ordered = (xs: MarketCode[]) => choices.map((m) => m.code).filter((c) => xs.includes(c));

// Limits (#935): the API takes a display name up to 200 characters; parts
// have no API cap, so 40 keeps the page and the request reasonable.
const NAME_MAX = 120;
const PART_MAX = 120;
const PARTS_MAX = 40;

// Country names people type, matched to the list's own names (#933). The
// list's names, its aliases (USA, UK, Viet Nam), and a few common others.
const byLower = new Map<string, string>();
for (const [n] of allCountries) byLower.set(n.toLowerCase(), n);
const nameOfCode = new Map(allCountries.map(([n, c]) => [c, n]));
for (const [alias, code] of Object.entries(countryCodes)) {
  const n = nameOfCode.get(code);
  if (n && !byLower.has(alias.toLowerCase())) byLower.set(alias.toLowerCase(), n);
}
for (const [alias, code] of [
  ["us", "us"], ["u.s.", "us"], ["u.s.a.", "us"], ["america", "us"], ["united states of america", "us"],
  ["prc", "cn"], ["mainland china", "cn"], ["people's republic of china", "cn"],
  ["korea", "kr"], ["republic of korea", "kr"], ["great britain", "gb"], ["england", "gb"], ["holland", "nl"],
] as const) {
  const n = nameOfCode.get(code);
  if (n) byLower.set(alias, n);
}
export function matchCountry(typed: string): string | null {
  const t = typed.trim().replace(/\s+/g, " ").toLowerCase();
  return t ? (byLower.get(t) ?? null) : null;
}

// The backend's steps (D71) in plain words, in the order they run (#930).
const STAGES: { name: PredictStage["name"]; text: string }[] = [
  { name: "router", text: "Checking which rules apply" },
  { name: "precedents", text: "Finding past Customs decisions on similar products" },
  { name: "engine", text: "Comparing your parts with those decisions" },
  { name: "clarify", text: "Checking what Customs may ask about" },
];

type Part = { name: string; country: string };
// An imported parts list: what the file gave, and the table it filled in.
// While the table still matches, the check sends the file's own product.
type Bom = { file: string; product: ImportResponse["product"]; family: string; economics?: ImportResponse["economics"]; parts: Part[]; assembly: string };
type Draft = { name: string; markets: MarketCode[]; parts: Part[]; assembly: string; bom?: Bom | null };
type Run = { code: MarketCode; state: "checking" | "done" | "failed" };
type Errors = Record<string, string>;
const blank = (): Part => ({ name: "", country: "" });
const today = () => new Date().toISOString().slice(0, 10);
const DRAFT_KEY = "unde.business-check.draft";

function readDraft(): Draft | null {
  try {
    const d = JSON.parse(sessionStorage.getItem(DRAFT_KEY) ?? "null") as (Draft & { market?: MarketCode }) | null;
    if (!d || typeof d.name !== "string" || !Array.isArray(d.parts)) return null;
    // A draft from before #945 has one market.
    const ms = Array.isArray(d.markets) ? d.markets : d.market ? [d.market] : ["us"];
    return {
      name: d.name.slice(0, NAME_MAX),
      markets: ordered(ms.filter((m): m is MarketCode => marketCodes.has(m))),
      parts: d.parts.slice(0, PARTS_MAX).map((p) => ({ name: String(p?.name ?? "").slice(0, PART_MAX), country: String(p?.country ?? "").slice(0, PART_MAX) })),
      assembly: typeof d.assembly === "string" ? d.assembly.slice(0, PART_MAX) : "",
      bom: d.bom && Array.isArray(d.bom.product?.components) && Array.isArray(d.bom.parts) ? d.bom : null,
    };
  } catch {
    return null;
  }
}
// The table a parts list fills in: each component and its country, and
// where it is put together (the plant that runs most of the steps).
function fromImport(r: ImportResponse): { parts: Part[]; assembly: string } {
  const p = r.product;
  const parts = p.components.slice(0, PARTS_MAX).map((c) => ({ name: c.name.slice(0, PART_MAX), country: matchCountry(c.made_in) ?? c.made_in }));
  const count = new Map<string, number>();
  for (const o of p.operations ?? []) if (o.plant) count.set(o.plant, (count.get(o.plant) ?? 0) + 1);
  const top = [...count].sort((a, b) => b[1] - a[1])[0]?.[0];
  const country = p.plants?.find((x) => x.id === top)?.country ?? "";
  return { parts, assembly: matchCountry(country) ?? country };
}
const sameTable = (a: { parts: Part[]; assembly: string }, b: { parts: Part[]; assembly: string }) => {
  const key = (x: { parts: Part[]; assembly: string }) =>
    JSON.stringify([x.parts.filter((p) => p.name.trim()).map((p) => [p.name.trim().toLowerCase(), matchCountry(p.country)]), matchCountry(x.assembly)]);
  return key(a) === key(b);
};

const stepInUrl = (): 1 | 2 => (new URLSearchParams(window.location.search).get("step") === "2" ? 2 : 1);

// The draft and the step exist only in the browser, so the form is drawn
// there, starting from them; the server sends the bar and an empty page.
const noSubscribe = () => () => {};
export function BusinessCheck() {
  const inBrowser = useSyncExternalStore(noSubscribe, () => true, () => false);
  if (!inBrowser)
    return (
      <div className="biz">
        <BusinessBar />
        <main id="main" className="biz-check" aria-busy="true" />
      </div>
    );
  const draft = readDraft();
  const step = stepInUrl() === 2 && draft?.name.trim() ? 2 : 1;
  return <CheckForm draft={draft} initialStep={step} />;
}

function CheckForm({ draft, initialStep }: { draft: Draft | null; initialStep: 1 | 2 }) {
  const router = useRouter();
  const noCreate = !canCreate(useWorkspaceRole());
  const id = useId();
  const [step, setStep] = useState<1 | 2>(initialStep);
  const [name, setName] = useState(draft?.name ?? "");
  const [markets, setMarkets] = useState<MarketCode[]>(draft?.markets ?? ["us"]);
  const [runs, setRuns] = useState<Run[]>([]);
  const [parts, setParts] = useState<Part[]>(draft?.parts.length ? draft.parts : [blank(), blank(), blank()]);
  const [assembly, setAssembly] = useState(draft?.assembly ?? "");
  const [bom, setBom] = useState<Bom | null>(draft?.bom ?? null);
  const fromFile = bom !== null && sameTable(bom, { parts, assembly });
  const [busy, setBusy] = useState(false);
  const [stages, setStages] = useState<PredictStage[]>([]);
  const [since, setSince] = useState(0);
  // Set before the request starts, so clicks in the same moment can't send
  // a second check before the button re-renders as disabled (#936).
  const sending = useRef(false);
  const [problem, setProblem] = useState<string | null>(null);
  const [errors, setErrors] = useState<Errors>({});
  const heading = useRef<HTMLHeadingElement>(null);
  const moved = useRef(false);

  // Step 2 in the URL without a draft (a shared link, a cleared tab): drop
  // it, so Back doesn't lead to an empty step. Back and Forward then move
  // between the steps.
  useEffect(() => {
    if (initialStep === 1 && stepInUrl() === 2) window.history.replaceState(null, "", window.location.pathname);
    const onPop = () => {
      moved.current = true;
      setProblem(null);
      setErrors({});
      setStep(stepInUrl() === 2 && (readDraft()?.name.trim() ?? "") ? 2 : 1);
    };
    window.addEventListener("popstate", onPop);
    return () => window.removeEventListener("popstate", onPop);
  }, [initialStep]);

  useEffect(() => {
    try {
      sessionStorage.setItem(DRAFT_KEY, JSON.stringify({ name, markets, parts, assembly, bom } satisfies Draft));
    } catch {
      /* private mode or full: the draft just isn't kept */
    }
  }, [name, markets, parts, assembly, bom]);

  // Focus the new step's heading after the user changes step (#928), not on
  // first load.
  useEffect(() => {
    if (!moved.current) return;
    moved.current = false;
    heading.current?.focus();
  }, [step, busy]);

  const goTo = (s: 1 | 2) => {
    moved.current = true;
    setProblem(null);
    setErrors({});
    if (s === 2) window.history.pushState(null, "", `${window.location.pathname}?step=2`);
    else if (stepInUrl() === 2) window.history.back();
    setStep(s);
  };

  // A parts list fills in the name (unless one was typed), the parts and the
  // assembly country, then shows them on step 2 to check.
  const imported = (r: ImportResponse, file: string) => {
    const t = fromImport(r);
    const n = name.trim() || r.product.name.slice(0, NAME_MAX);
    setName(n);
    setParts(t.parts.length ? t.parts : [blank()]);
    setAssembly(t.assembly);
    setBom({ file, product: r.product, family: r.family, economics: r.economics, ...t });
    setErrors({});
    setProblem(null);
    if (step === 1 && n.trim()) goTo(2);
  };

  const focusField = (fid: string) => {
    const el = document.getElementById(fid);
    el?.focus();
    el?.scrollIntoView?.({ block: "center" });
  };

  const next = (e: FormEvent) => {
    e.preventDefault();
    const errs: Errors = {};
    if (!name.trim()) errs[`${id}-name`] = "Add the product name.";
    if (markets.length === 0) errs[`${id}-markets`] = "Tick at least one country.";
    setErrors(errs);
    const first = Object.keys(errs)[0];
    if (first) {
      focusField(first === `${id}-markets` ? `${id}-m-us` : first);
      return;
    }
    goTo(2);
  };

  // Every problem on step 2, keyed by field id, in page order.
  const validate = (): Errors => {
    const errs: Errors = {};
    const filled = parts.map((p, i) => ({ p, i })).filter(({ p }) => p.name.trim() || p.country.trim());
    if (filled.length === 0) errs[`${id}-p0-name`] = "Add at least one part and the country that makes it.";
    const seen = new Map<string, number>();
    for (const { p, i } of filled) {
      const key = p.name.trim().toLowerCase();
      if (!key) errs[`${id}-p${i}-name`] = "Name this part, or clear the row.";
      else if (seen.has(key)) errs[`${id}-p${i}-name`] = `This part is already on row ${seen.get(key)! + 1}.`;
      else seen.set(key, i);
      if (!p.country.trim()) errs[`${id}-p${i}-country`] = "Add the country that makes this part.";
      else if (!matchCountry(p.country)) errs[`${id}-p${i}-country`] = "Choose a country from the list.";
    }
    if (!assembly.trim()) errs[`${id}-assembly`] = "Add the country where the product is put together.";
    else if (!matchCountry(assembly)) errs[`${id}-assembly`] = "Choose a country from the list.";
    return errs;
  };

  const check = async (e: FormEvent) => {
    e.preventDefault();
    if (sending.current) return;
    const errs = validate();
    setErrors(errs);
    const first = Object.keys(errs)[0];
    if (first) {
      const n = Object.keys(errs).length;
      setProblem(n === 1 ? "One thing to fix before the check." : `${n} things to fix before the check.`);
      focusField(first);
      return;
    }
    if (noCreate) {
      setProblem("Your workspace role can view products but not check new ones.");
      return;
    }
    sending.current = true;
    setProblem(null);
    setStages([]);
    setSince(Date.now());
    moved.current = true;
    setBusy(true);
    // The assembly country becomes one plant with one final assembly step,
    // so the engine reads it as a structured fact, as from the full form.
    // An unchanged parts list goes as the file gave it, with its money, as
    // from the full form. An edited one goes as the table, still with the
    // file's family and tariff code (as the full form keeps them after edits).
    const components: Component[] = parts
      .filter((p) => p.name.trim())
      .map((p) => ({ name: p.name.trim(), made_in: matchCountry(p.country)! }));
    const product = fromFile
      ? { ...bom.product, name: name.trim() }
      : {
          name: name.trim(),
          plants: [{ id: "assembly", name: "Assembly plant", country: matchCountry(assembly)! }],
          components,
          operations: [{ name: "Final assembly", plant: "assembly" }],
        };
    const base = predictBody({ date: today(), tab: "parts", name, hts: bom?.product.hts ?? "", family: bom?.family ?? "", facts: "", product, extra: {} });
    // One check per ticked country, all at the same time (#945). The steps
    // shown are the first country's; each country's state is listed below.
    const order = ordered(markets);
    setRuns(order.map((code) => ({ code, state: "checking" })));
    const mark = (code: MarketCode, state: Run["state"]) => setRuns((xs) => xs.map((r) => (r.code === code ? { ...r, state } : r)));
    const results = await Promise.allSettled(
      order.map((code, i) => {
        const body: PredictRequest = { ...base, economics: fromFile && code === "us" ? bom.economics : undefined, jurisdiction: code };
        const onStage = (st: PredictStage) =>
          setStages((xs) => (xs.some((x) => x.name === st.name) ? xs.map((x) => (x.name === st.name ? st : x)) : [...xs, st]));
        return streamPredict(body, i === 0 ? onStage : () => {}).then(
          (c) => {
            mark(code, "done");
            return c;
          },
          (e) => {
            mark(code, "failed");
            throw e;
          },
        );
      }),
    );
    const ok = results.flatMap((r, i) => (r.status === "fulfilled" && r.value.id ? [{ code: order[i], id: r.value.id }] : []));
    if (ok.length === 0) {
      const err = results.find((r) => r.status === "rejected")?.reason;
      sending.current = false;
      moved.current = true;
      setBusy(false);
      setProblem(
        isSigninRequired(err)
          ? "Sign in with Google to check a product. The button is at the top right."
          : "The check did not finish. Nothing is lost. Try again in a moment.",
      );
      return;
    }
    // One country or several: the plain answers page, one row per country,
    // so a business visitor stays in the business view (#932). The draft
    // stays, so "Check the same parts for another country" starts from it.
    const failed = order.filter((c) => !ok.some((x) => x.code === c));
    const q = new URLSearchParams({ ids: ok.map((x) => x.id).join(",") });
    if (failed.length) q.set("failed", failed.join(","));
    router.push(`/business/answers?${q}`);
  };

  // Any edit clears that field's message and the summary, so neither names a
  // gap already fixed.
  const clear = (fid: string) => {
    setProblem(null);
    setErrors((x) => {
      if (!(fid in x)) return x;
      const rest = { ...x };
      delete rest[fid];
      return rest;
    });
  };
  const setPart = (i: number, k: keyof Part, v: string) => {
    clear(`${id}-p${i}-${k === "name" ? "name" : "country"}`);
    setParts((xs) => xs.map((p, j) => (j === i ? { ...p, [k]: v } : p)));
  };
  const addPart = (focus: boolean) => {
    if (parts.length >= PARTS_MAX) return;
    const i = parts.length;
    setParts((xs) => [...xs, blank()]);
    if (focus) setTimeout(() => document.getElementById(`${id}-p${i}-name`)?.focus(), 0);
  };
  const removePart = (i: number) => {
    setErrors({});
    setProblem(null);
    setParts((xs) => xs.filter((_, j) => j !== i));
  };

  // Enter in the parts table moves to the next field, and on the last row
  // adds a row, instead of running the check (#934).
  const onTableKey = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key !== "Enter" || e.nativeEvent.isComposing) return;
    e.preventDefault();
    const inputs = [...(e.currentTarget.closest("tbody")?.querySelectorAll<HTMLInputElement>("input") ?? [])];
    const at = inputs.indexOf(e.currentTarget);
    if (at < inputs.length - 1) inputs[at + 1].focus();
    else if (parts.length < PARTS_MAX) addPart(true);
    else document.getElementById(`${id}-assembly`)?.focus();
  };

  const err = (fid: string) => errors[fid];
  const described = (fid: string) => (errors[fid] ? `${fid}-err` : undefined);
  const fieldError = (fid: string) => (errors[fid] ? <span id={`${fid}-err`} className="biz-field-error">{errors[fid]}</span> : null);

  return (
    <div className="biz">
      <BusinessBar />
      <main id="main" className="biz-check">
        <datalist id={`${id}-countries`}>
          {allCountries.map(([n]) => (
            <option key={n} value={n} />
          ))}
        </datalist>

        {step === 1 ? (
          <div className="biz-check-row">
          <div className="biz-check-col">
            <div className="biz-check-head">
              <p className="biz-eyebrow">
                <span className="mono">1 of 2</span>Your product
              </p>
              <h1 ref={heading} tabIndex={-1}>Check your product</h1>
              <p className="biz-sub">Two short steps. Then you see the likely country and the import tax.</p>
            </div>
            <form className="biz-form" onSubmit={next} noValidate>
              <div className="biz-field">
                <label htmlFor={`${id}-name`}>What is the product?</label>
                <input
                  id={`${id}-name`}
                  type="text"
                  value={name}
                  maxLength={NAME_MAX}
                  aria-invalid={err(`${id}-name`) ? true : undefined}
                  aria-describedby={described(`${id}-name`) ?? `${id}-name-help`}
                  onChange={(e) => {
                    setName(e.target.value);
                    clear(`${id}-name`);
                  }}
                  placeholder="For example: Heat gun, 12.5 A"
                  autoComplete="off"
                />
                {fieldError(`${id}-name`)}
                <span id={`${id}-name-help`} className="biz-help">A short name is enough.</span>
              </div>
              <fieldset className="biz-markets" aria-invalid={err(`${id}-markets`) ? true : undefined} aria-describedby={described(`${id}-markets`) ?? `${id}-markets-help`}>
                <legend>Which countries will it be imported into?</legend>
                <p id={`${id}-markets-help`} className="biz-help">
                  Tick all that apply. Each country gets its own answer. That country’s rules decide the import tax, not where it is made.
                </p>
                {fieldError(`${id}-markets`)}
                <div className="biz-regions">
                  {regions.map((r) => (
                    <div key={r.name} className="biz-region" role="group" aria-labelledby={`${id}-r-${r.name}`}>
                      <h3 id={`${id}-r-${r.name}`}>{r.name}</h3>
                      {r.items.map((m) => (
                        <label key={m.code} className="biz-market">
                          <input
                            id={`${id}-m-${m.code}`}
                            type="checkbox"
                            checked={markets.includes(m.code)}
                            onChange={() => {
                              clear(`${id}-markets`);
                              setMarkets((xs) => (xs.includes(m.code) ? xs.filter((x) => x !== m.code) : ordered([...xs, m.code])));
                            }}
                          />
                          <span>
                            <b>{m.name}</b>
                            <span>{m.what}</span>
                          </span>
                        </label>
                      ))}
                      {r.name === "Asia and Pacific" && (
                        <p className="biz-help">
                          Japan needs a trade agreement and a tariff code, so it uses the{" "}
                          <Link href="/cases/new?via=business" className="biz-link">full form</Link>.
                        </p>
                      )}
                    </div>
                  ))}
                </div>
                <details className="biz-soon">
                  <summary>Other countries: coming soon</summary>
                  <p>
                    The United States check uses past decisions by U.S. Customs (CBP) on similar products. The other countries here are
                    checked against their own written origin rules. Countries not listed, such as China, India or Vietnam, are not checked yet.
                  </p>
                </details>
              </fieldset>
              <div className="biz-form-actions">
                <button type="submit" className="biz-btn accent">Next: your parts</button>
                <span className="biz-help" aria-live="polite">
                  {markets.length > 1 ? `${markets.length} countries: ${markets.length} answers, run together` : ""}
                </span>
                <Link href="/cases/new?via=business" className="biz-link biz-push">Use the full form instead</Link>
              </div>
            </form>
          </div>
          <div className="biz-side">
            <BomDrop name={name} onImported={imported} />
          </div>
          </div>
        ) : (
          <div className="biz-check-row">
            <div className="biz-check-col">
              <div className="biz-check-head">
                <p className="biz-eyebrow biz-eyebrow-name">
                  <span className="mono">2 of 2</span>Your parts · {name.trim()}
                </p>
                <h1 ref={heading} tabIndex={-1}>{busy ? "Checking your product" : "List the main parts"}</h1>
                <p className="biz-sub">
                  {busy ? "This usually takes about 5 seconds. Your parts are kept if you go back." : "For each part, say which country makes it. Start with the biggest parts."}
                </p>
              </div>
              {busy ? (
                <CheckProgress stages={stages} since={since} parts={parts.filter((p) => p.name.trim())} runs={runs} />
              ) : (
                <form className="biz-form" onSubmit={check} noValidate aria-busy={busy}>
                  {bom && (
                    <p role="status" className="biz-filled">
                      {fromFile
                        ? `Filled in from ${bom.file}: ${bom.parts.length} parts. Check them and change anything. The check uses the whole file, with its plants and steps.`
                        : `You changed the list from ${bom.file}, so the check uses this table, not the file.`}
                    </p>
                  )}
                  <table className="biz-ptable">
                    <thead>
                      <tr>
                        <th scope="col">Part</th>
                        <th scope="col">Made in</th>
                        <th scope="col"><span className="sr-only">Remove</span></th>
                      </tr>
                    </thead>
                    <tbody>
                      {parts.map((p, i) => {
                        const nf = `${id}-p${i}-name`;
                        const cf = `${id}-p${i}-country`;
                        return (
                          <tr key={i}>
                            <td>
                              <input id={nf} type="text" aria-label={`Part ${i + 1} name`} value={p.name} maxLength={PART_MAX} aria-invalid={err(nf) ? true : undefined} aria-describedby={described(nf)} onChange={(e) => setPart(i, "name", e.target.value)} onKeyDown={onTableKey} placeholder={i === 0 ? "For example: Motor" : ""} autoComplete="off" />
                              {fieldError(nf)}
                            </td>
                            <td>
                              <input id={cf} type="text" aria-label={`Country that makes part ${i + 1}`} list={`${id}-countries`} value={p.country} maxLength={PART_MAX} aria-invalid={err(cf) ? true : undefined} aria-describedby={described(cf)} onChange={(e) => setPart(i, "country", e.target.value)} onKeyDown={onTableKey} placeholder={i === 0 ? "For example: China" : ""} autoComplete="off" />
                              {fieldError(cf)}
                            </td>
                            <td>
                              <button type="button" className="biz-icon-btn" aria-label={`Remove part ${i + 1}`} disabled={parts.length === 1} onClick={() => removePart(i)}>
                                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" aria-hidden="true">
                                  <path d="M18 6 6 18" />
                                  <path d="m6 6 12 12" />
                                </svg>
                              </button>
                            </td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                  <div className="biz-tools">
                    {parts.length < PARTS_MAX ? (
                      <button type="button" className="biz-btn soft" onClick={() => addPart(true)}>+ Add a part</button>
                    ) : (
                      <span className="biz-help">{PARTS_MAX} parts is the most here. For a longer list, use the full form.</span>
                    )}
                    <span className="biz-help">Or drop a parts list file on the right.</span>
                  </div>
                  <div className="biz-field">
                    <label htmlFor={`${id}-assembly`}>Where is the product put together?</label>
                    <input
                      id={`${id}-assembly`}
                      type="text"
                      list={`${id}-countries`}
                      value={assembly}
                      maxLength={PART_MAX}
                      aria-invalid={err(`${id}-assembly`) ? true : undefined}
                      aria-describedby={described(`${id}-assembly`)}
                      onChange={(e) => {
                        setAssembly(e.target.value);
                        clear(`${id}-assembly`);
                      }}
                      placeholder="For example: Thailand"
                      autoComplete="off"
                    />
                    {fieldError(`${id}-assembly`)}
                  </div>
                  {problem && <p role="alert" className="biz-problem">{problem}</p>}
                  <div className="biz-form-actions ruled">
                    <button type="button" className="biz-link plain" onClick={() => goTo(1)} disabled={busy}>Back</button>
                    <button type="submit" className="biz-btn accent biz-push" disabled={busy}>{busy ? "Checking…" : "Check my product"}</button>
                  </div>
                </form>
              )}
            </div>
            <div className="biz-side">
            {!busy && <BomDrop name={name} onImported={imported} />}
            {busy ? (
              <aside aria-label="Your answer, loading" aria-busy="true" className="biz-next biz-skel">
                <h2>Your likely answer</h2>
                <span className="sk stamp" />
                <span className="biz-skel-row">
                  <span className="sk" style={{ width: 120, height: 36 }} />
                  <span className="sk" style={{ width: 70, height: 14 }} />
                </span>
                <span className="sk" style={{ width: "100%", height: 12 }} />
                <span className="sk" style={{ width: "86%", height: 12 }} />
                <span className="sk" style={{ width: "60%", height: 12 }} />
              </aside>
            ) : (
              <aside aria-label="What happens next" className="biz-next">
                <h2>What happens next</h2>
                <ol>
                  <li><span className="mono">1</span>{markets.length > 1 ? `You see an answer for each of the ${markets.length} countries. It takes about 5 seconds.` : "You see the likely country and the import tax. It takes about 5 seconds."}</li>
                  <li><span className="mono">2</span>UndeLabs may ask a few questions about how the parts are made. Answering is optional.</li>
                  <li><span className="mono">3</span>{markets.includes("us") ? "You see which production changes could lower the U.S. tax." : "Plans for production changes are for the United States only for now."}</li>
                </ol>
                <p>An estimate. A customs broker confirms it.</p>
              </aside>
            )}
            </div>
          </div>
        )}
      </main>
    </div>
  );
}

// The check in progress (#930): the backend's steps as they stream, with
// the seconds so far, and the parts that were sent.
function CheckProgress({ stages, since, parts, runs }: { stages: PredictStage[]; since: number; parts: Part[]; runs: Run[] }) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(t);
  }, []);
  const secs = Math.max(0, Math.round((now - since) / 1000));
  // Before any step reports, the first step shows as under way.
  const firstOpen = STAGES.findIndex((x) => stages.find((y) => y.name === x.name)?.status !== "done");
  const anyStarted = stages.some((y) => y.status === "started");
  const state = (name: PredictStage["name"], i: number) => {
    const st = stages.find((x) => x.name === name);
    if (st?.status === "done") return "done";
    if (st) return "now";
    return i === firstOpen && !anyStarted ? "now" : "todo";
  };
  return (
    <>
      <section aria-label="Progress" className="biz-form biz-progress">
        <ol role="status" aria-live="polite">
          {STAGES.map((x, i) => {
            const st = state(x.name, i);
            const ms = stages.find((y) => y.name === x.name && y.status === "done")?.ms;
            return (
              <li key={x.name} className={st}>
                <span className="dot" aria-hidden="true">
                  {st === "done" && (
                    <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="#fff" strokeWidth="3" strokeLinecap="round" strokeLinejoin="round">
                      <path d="M20 6 9 17l-5-5" />
                    </svg>
                  )}
                </span>
                <span className="t">
                  {x.text}
                  <span className="sr-only">{st === "done" ? " (done)" : st === "now" ? " (in progress)" : ""}</span>
                </span>
                <span className="mono ms">{ms !== undefined ? `${(ms / 1000).toFixed(1)} s` : ""}</span>
              </li>
            );
          })}
        </ol>
        <p className="biz-progress-foot">
          <span className="mono">{secs} s</span>
          {secs >= 20 && <span>Taking longer than usual. Still working.</span>}
        </p>
      </section>
      {runs.length > 1 && (
        <div className="biz-sent">
          <span>Countries · the steps above are for {marketName(runs[0].code)}</span>
          <ul aria-live="polite">
            {runs.map((r) => (
              <li key={r.code}>
                <span>{marketName(r.code)}</span>
                <span>{r.state === "done" ? "Done" : r.state === "failed" ? "Did not finish" : "Checking…"}</span>
              </li>
            ))}
          </ul>
        </div>
      )}
      {parts.length > 0 && (
        <div className="biz-sent">
          <span>What you sent</span>
          <ul>
            {parts.map((p, i) => (
              <li key={i}>
                <span>{p.name.trim()}</span>
                <span>{matchCountry(p.country) ?? p.country}</span>
              </li>
            ))}
          </ul>
        </div>
      )}
    </>
  );
}
