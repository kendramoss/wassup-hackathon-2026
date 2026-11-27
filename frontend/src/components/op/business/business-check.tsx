"use client";

// The business view's first step (/business/check, #893; canvas mock "Check
// a product", option A). Two short steps in plain words: the product and
// where it is sold, then the main parts and where the product is put
// together. "Check my product" runs the same predict as the full form
// (/cases/new) and opens the answer. The full form stays one link away for
// anything more (families, dates, BOM files, plants).
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useId, useState, type FormEvent } from "react";
import { streamPredict, type Component, type PredictRequest } from "@/lib/api/client";
import { predictBody } from "@/lib/product-facts-reconcile";
import { isSigninRequired, canCreate } from "@/lib/roles";
import { useWorkspaceRole } from "@/lib/use-role";
import type { MarketCode } from "@/lib/market";
import { countryOptions } from "../combobox";
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

type Part = { name: string; country: string };
const blank = (): Part => ({ name: "", country: "" });
const today = () => new Date().toISOString().slice(0, 10);

export function BusinessCheck() {
  const router = useRouter();
  const noCreate = !canCreate(useWorkspaceRole());
  const id = useId();
  const [step, setStep] = useState<1 | 2>(1);
  const [name, setName] = useState("");
  const [market, setMarket] = useState<MarketCode>("us");
  const [parts, setParts] = useState<Part[]>([blank(), blank(), blank()]);
  const [assembly, setAssembly] = useState("");
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState<string | null>(null);

  const filled = parts.filter((p) => p.name.trim() || p.country.trim());
  const missing =
    filled.length === 0
      ? "Add at least one part and the country that makes it."
      : filled.some((p) => !p.name.trim())
        ? "Name each part, or clear the row."
        : filled.some((p) => !p.country.trim())
          ? "Add the country that makes each part."
          : !assembly.trim()
            ? "Add the country where the product is put together."
            : null;

  const next = (e: FormEvent) => {
    e.preventDefault();
    if (!name.trim()) {
      setProblem("Add the product name.");
      document.getElementById(`${id}-name`)?.focus();
      return;
    }
    setProblem(null);
    setStep(2);
  };

  const check = async (e: FormEvent) => {
    e.preventDefault();
    if (missing) {
      setProblem(missing);
      return;
    }
    if (noCreate) {
      setProblem("Your workspace role can view products but not check new ones.");
      return;
    }
    setProblem(null);
    setBusy(true);
    // The assembly country becomes one plant with one final assembly step,
    // so the engine reads it as a structured fact, as from the full form.
    const components: Component[] = filled.map((p) => ({ name: p.name.trim(), made_in: p.country.trim() }));
    const product = {
      name: name.trim(),
      plants: [{ id: "assembly", name: "Assembly plant", country: assembly.trim() }],
      components,
      operations: [{ name: "Final assembly", plant: "assembly" }],
    };
    const body: PredictRequest = {
      ...predictBody({ date: today(), tab: "parts", name, hts: "", family: "", facts: "", product, extra: {} }),
      jurisdiction: market,
    };
    try {
      const c = await streamPredict(body, () => {});
      router.push(`/cases/${encodeURIComponent(c.id!)}/origin`);
    } catch (err) {
      setBusy(false);
      setProblem(
        isSigninRequired(err)
          ? "Sign in with Google to check a product. The button is at the top right."
          : "The check did not finish. Nothing is lost. Try again in a moment.",
      );
    }
  };

  // Any edit clears the last message, so it never names a gap already fixed.
  const setPart = (i: number, k: keyof Part, v: string) => {
    setProblem(null);
    setParts((xs) => xs.map((p, j) => (j === i ? { ...p, [k]: v } : p)));
  };

  return (
    <div className="biz">
      <BusinessBar />
      <main id="main" className="biz-check">
        <datalist id={`${id}-countries`}>
          {countryOptions.map((o) => (
            <option key={o.value} value={o.value} />
          ))}
        </datalist>

        {step === 1 ? (
          <div className="biz-check-col">
            <div className="biz-check-head">
              <p className="biz-eyebrow">
                <span className="mono">1 of 2</span>Your product
              </p>
              <h1>Check your product</h1>
              <p className="biz-sub">Two short steps. Then you see the likely country and the import tax.</p>
            </div>
            <form className="biz-card" onSubmit={next} noValidate>
              <div className="biz-field">
                <label htmlFor={`${id}-name`}>What is the product?</label>
                <input id={`${id}-name`} type="text" value={name} onChange={(e) => { setName(e.target.value); setProblem(null); }} placeholder="For example: Heat gun, 12.5 A" autoComplete="off" />
                <span className="biz-help">A short name is enough.</span>
              </div>
              <fieldset className="biz-markets">
                <legend>Where do you sell it?</legend>
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
                <p className="biz-help">Selling in Japan? Japan checks need a trade agreement and a tariff code, so they use the <Link href="/cases/new" className="biz-link">full form</Link>.</p>
              </fieldset>
              {problem && <p role="alert" className="biz-problem">{problem}</p>}
              <div className="biz-card-actions">
                <button type="submit" className="biz-btn accent">Next: your parts</button>
                <Link href="/cases/new" className="biz-link biz-push">Use the full form instead</Link>
              </div>
            </form>
          </div>
        ) : (
          <div className="biz-check-row">
            <div className="biz-check-col">
              <div className="biz-check-head">
                <p className="biz-eyebrow">
                  <span className="mono">2 of 2</span>Your parts · {name.trim()}
                </p>
                <h1>List the main parts</h1>
                <p className="biz-sub">For each part, say which country makes it. Start with the biggest parts.</p>
              </div>
              <form className="biz-card" onSubmit={check} noValidate aria-busy={busy}>
                <table className="biz-ptable">
                  <thead>
                    <tr>
                      <th scope="col">Part</th>
                      <th scope="col">Made in</th>
                      <th scope="col"><span className="sr-only">Remove</span></th>
                    </tr>
                  </thead>
                  <tbody>
                    {parts.map((p, i) => (
                      <tr key={i}>
                        <td>
                          <input type="text" aria-label={`Part ${i + 1} name`} value={p.name} onChange={(e) => setPart(i, "name", e.target.value)} placeholder={i === 0 ? "For example: Motor" : ""} autoComplete="off" />
                        </td>
                        <td>
                          <input type="text" aria-label={`Country that makes part ${i + 1}`} list={`${id}-countries`} value={p.country} onChange={(e) => setPart(i, "country", e.target.value)} placeholder={i === 0 ? "For example: China" : ""} autoComplete="off" />
                        </td>
                        <td>
                          <button type="button" className="biz-icon-btn" aria-label={`Remove part ${i + 1}`} disabled={parts.length === 1} onClick={() => setParts((xs) => xs.filter((_, j) => j !== i))}>
                            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" aria-hidden="true">
                              <path d="M18 6 6 18" />
                              <path d="m6 6 12 12" />
                            </svg>
                          </button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
                <div className="biz-row">
                  <button type="button" className="biz-btn soft" onClick={() => setParts((xs) => [...xs, blank()])}>+ Add a part</button>
                  <span className="biz-help">Have a parts list file (Excel, CSV, PDF or a photo)? <Link href="/cases/new" className="biz-link">Upload it in the full form</Link>.</span>
                </div>
                <div className="biz-field">
                  <label htmlFor={`${id}-assembly`}>Where is the product put together?</label>
                  <input id={`${id}-assembly`} type="text" list={`${id}-countries`} value={assembly} onChange={(e) => { setAssembly(e.target.value); setProblem(null); }} placeholder="For example: Thailand" autoComplete="off" />
                </div>
                {problem && <p role="alert" className="biz-problem">{problem}</p>}
                <div className="biz-card-actions ruled">
                  <button type="button" className="biz-link plain" onClick={() => { setProblem(null); setStep(1); }} disabled={busy}>Back</button>
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
