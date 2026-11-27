"use client";

// The business view's first step (/business/check, #893; canvas mock "Check
// a product", option A). Two short steps in plain words: the product and
// the country it is imported into, then the main parts and where it is put
// together. "Check my product" runs the same predict as the full form
// (/cases/new) and opens the answer. The full form stays one link away for
// anything more (families, dates, BOM files, plants).
//
// The step lives in the URL (?step=2) and the draft in sessionStorage, so
// Back returns to step 1 and a refresh keeps what was typed (#929). Focus
// moves to each step's heading (#928). Countries are matched to the list,
// and each problem is shown beside its field (#933).
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useId, useRef, useState, useSyncExternalStore, type FormEvent, type KeyboardEvent } from "react";
import { streamPredict, type Component, type PredictRequest } from "@/lib/api/client";
import { predictBody } from "@/lib/product-facts-reconcile";
import { isSigninRequired, canCreate } from "@/lib/roles";
import { useWorkspaceRole } from "@/lib/use-role";
import type { MarketCode } from "@/lib/market";
import { allCountries, countryCodes } from "@/lib/countries";
import { BusinessBar } from "./business-bar";
import "./business.css";

// What each market gets today, in plain words (copy.market.summary).
const marketChoices: { code: MarketCode; name: string; what: string }[] = [
  { code: "ca", name: "Canada", what: "Rules check only" },
  { code: "mx", name: "Mexico", what: "Rules check only" },
  { code: "eu", name: "European Union", what: "Rules check, some goods only" },
  { code: "uk", name: "United Kingdom", what: "Rules check, some goods only" },
  { code: "kr", name: "South Korea", what: "Rules check only" },
  { code: "au", name: "Australia", what: "Rules check only" },
];
const marketCodes = new Set<string>(["us", ...marketChoices.map((m) => m.code)]);

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

type Part = { name: string; country: string };
type Draft = { name: string; market: MarketCode; parts: Part[]; assembly: string };
type Errors = Record<string, string>;
const blank = (): Part => ({ name: "", country: "" });
const today = () => new Date().toISOString().slice(0, 10);
const DRAFT_KEY = "unde.business-check.draft";

function readDraft(): Draft | null {
  try {
    const d = JSON.parse(sessionStorage.getItem(DRAFT_KEY) ?? "null") as Draft | null;
    if (!d || typeof d.name !== "string" || !Array.isArray(d.parts)) return null;
    return {
      name: d.name.slice(0, NAME_MAX),
      market: marketCodes.has(d.market) ? d.market : "us",
      parts: d.parts.slice(0, PARTS_MAX).map((p) => ({ name: String(p?.name ?? "").slice(0, PART_MAX), country: String(p?.country ?? "").slice(0, PART_MAX) })),
      assembly: typeof d.assembly === "string" ? d.assembly.slice(0, PART_MAX) : "",
    };
  } catch {
    return null;
  }
}
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
  const [market, setMarket] = useState<MarketCode>(draft?.market ?? "us");
  const [parts, setParts] = useState<Part[]>(draft?.parts.length ? draft.parts : [blank(), blank(), blank()]);
  const [assembly, setAssembly] = useState(draft?.assembly ?? "");
  const [busy, setBusy] = useState(false);
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
      sessionStorage.setItem(DRAFT_KEY, JSON.stringify({ name, market, parts, assembly } satisfies Draft));
    } catch {
      /* private mode or full: the draft just isn't kept */
    }
  }, [name, market, parts, assembly]);

  // Focus the new step's heading after the user changes step (#928), not on
  // first load.
  useEffect(() => {
    if (!moved.current) return;
    moved.current = false;
    heading.current?.focus();
  }, [step]);

  const goTo = (s: 1 | 2) => {
    moved.current = true;
    setProblem(null);
    setErrors({});
    if (s === 2) window.history.pushState(null, "", `${window.location.pathname}?step=2`);
    else if (stepInUrl() === 2) window.history.back();
    setStep(s);
  };

  const focusField = (fid: string) => {
    const el = document.getElementById(fid);
    el?.focus();
    el?.scrollIntoView?.({ block: "center" });
  };

  const next = (e: FormEvent) => {
    e.preventDefault();
    if (!name.trim()) {
      setErrors({ [`${id}-name`]: "Add the product name." });
      focusField(`${id}-name`);
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
    setBusy(true);
    // The assembly country becomes one plant with one final assembly step,
    // so the engine reads it as a structured fact, as from the full form.
    const components: Component[] = parts
      .filter((p) => p.name.trim())
      .map((p) => ({ name: p.name.trim(), made_in: matchCountry(p.country)! }));
    const product = {
      name: name.trim(),
      plants: [{ id: "assembly", name: "Assembly plant", country: matchCountry(assembly)! }],
      components,
      operations: [{ name: "Final assembly", plant: "assembly" }],
    };
    const body: PredictRequest = {
      ...predictBody({ date: today(), tab: "parts", name, hts: "", family: "", facts: "", product, extra: {} }),
      jurisdiction: market,
    };
    try {
      const c = await streamPredict(body, () => {});
      try {
        sessionStorage.removeItem(DRAFT_KEY);
      } catch {
        /* ignore */
      }
      router.push(`/cases/${encodeURIComponent(c.id!)}/origin`);
    } catch (err) {
      sending.current = false;
      setBusy(false);
      setProblem(
        isSigninRequired(err)
          ? "Sign in with Google to check a product. The button is at the top right."
          : "The check did not finish. Nothing is lost. Try again in a moment.",
      );
    }
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
              <fieldset className="biz-markets">
                <legend>Which country will it be imported into?</legend>
                <p className="biz-help">That country’s rules decide the import tax. This is not where it is made.</p>
                <label className="biz-market wide">
                  <input type="radio" name="market" checked={market === "us"} onChange={() => setMarket("us")} />
                  <span>
                    <b>United States</b>
                    <span>Full check: the likely country, the import tax, and which production changes could lower it</span>
                  </span>
                </label>
                <div className="biz-market-grid">
                  {marketChoices.map((m) => (
                    <label key={m.code} className="biz-market">
                      <input type="radio" name="market" checked={market === m.code} onChange={() => setMarket(m.code)} />
                      <span>
                        <b>{m.name}</b>
                        <span>{m.what}</span>
                      </span>
                    </label>
                  ))}
                </div>
                <p className="biz-help">
                  Importing into Japan? Japan checks need a trade agreement and a tariff code, so they use the{" "}
                  <Link href="/cases/new?via=business" className="biz-link">full form</Link>.
                </p>
              </fieldset>
              <div className="biz-form-actions">
                <button type="submit" className="biz-btn accent">Next: your parts</button>
                <Link href="/cases/new?via=business" className="biz-link biz-push">Use the full form instead</Link>
              </div>
            </form>
          </div>
        ) : (
          <div className="biz-check-row">
            <div className="biz-check-col">
              <div className="biz-check-head">
                <p className="biz-eyebrow biz-eyebrow-name">
                  <span className="mono">2 of 2</span>Your parts · {name.trim()}
                </p>
                <h1 ref={heading} tabIndex={-1}>List the main parts</h1>
                <p className="biz-sub">For each part, say which country makes it. Start with the biggest parts.</p>
              </div>
              <form className="biz-form" onSubmit={check} noValidate aria-busy={busy}>
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
                  <span className="biz-help">
                    Have a parts list file (Excel, CSV, PDF or a photo)? <Link href="/cases/new?via=business" className="biz-link">Upload it in the full form</Link>.
                  </span>
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
            </div>
            <aside aria-label="What happens next" className="biz-next">
              <h2>What happens next</h2>
              <ol>
                <li><span className="mono">1</span>You see the likely country and the import tax. It takes about 5 seconds.</li>
                <li><span className="mono">2</span>Unde may ask a few questions about how the parts are made. Answering is optional.</li>
                <li><span className="mono">3</span>{market === "us" ? "You see which production changes could lower the tax." : "Plans for production changes are for the United States only for now."}</li>
              </ol>
              <p>An estimate. A customs broker confirms it.</p>
            </aside>
          </div>
        )}
      </main>
    </div>
  );
}
