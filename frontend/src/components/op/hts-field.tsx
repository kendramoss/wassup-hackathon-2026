"use client";

import { useEffect, useId, useState } from "react";
import { ApiError, api, type HtsSuggestResponse, type Product } from "@/lib/api/client";
import { Button, Chip, Spinner, cx } from "./ui";

// Whether the server suggests HTS codes (HTS_SUGGEST, D64). The route only
// exists when it's on, so an empty request is a 400 when on and a 404 when
// off; it runs no model call. Asked once per page load.
let available: Promise<boolean> | null = null;
function suggestAvailable(): Promise<boolean> {
  available ??= api.htsSuggest({ facts: "" }).then(
    () => true,
    (e) => !(e instanceof ApiError && e.status === 404),
  );
  return available;
}

const rulingUrl = (id: string) => `https://rulings.cbp.gov/ruling/${encodeURIComponent(id)}`;

// The optional HTS subheading, with "Suggest from earlier rulings": up to 3
// subheadings CBP applied in similar rulings. A suggestion, never filled in
// for the user: they pick one, or type their own.
export function HtsField({
  value,
  onChange,
  source,
  inputClass,
  help,
}: {
  value: string;
  onChange: (hts: string) => void;
  // What to suggest from: the pasted text or the product; null when there's
  // not enough to go on yet.
  source: { facts: string } | { product: Product } | null;
  inputClass: string;
  help?: string;
}) {
  const id = useId();
  const [on, setOn] = useState(false);
  const [busy, setBusy] = useState(false);
  const [res, setRes] = useState<HtsSuggestResponse | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let live = true;
    suggestAvailable().then((ok) => live && setOn(ok));
    return () => {
      live = false;
    };
  }, []);

  const suggest = async () => {
    if (!source) return;
    setBusy(true);
    setError(null);
    try {
      setRes(await api.htsSuggest(source));
    } catch (e) {
      if (e instanceof ApiError && e.status === 404) setOn(false);
      setError(e instanceof ApiError && e.status === 429 ? "Too many suggestions at once. Wait a moment and try again." : "Couldn't get suggestions. Try again, or type the code.");
    } finally {
      setBusy(false);
    }
  };

  const candidates = res?.candidates ?? [];
  return (
    <div>
      <label htmlFor={id} className="block text-[12.5px] text-body">
        HTS subheading <span className="text-dim">(optional)</span>
      </label>
      <div className="mt-1.5 flex flex-wrap items-center gap-2.5">
        <input
          id={id}
          className={cx(inputClass.replace("w-full", ""), "w-44 tabular-nums")}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder="e.g. 8419.89.95"
          inputMode="decimal"
        />
        {on && (
          <Button variant="ghost" onClick={suggest} disabled={!source || busy} title={source ? undefined : "Describe the product first"}>
            Suggest from earlier rulings
          </Button>
        )}
      </div>
      {help && <p className="duo mt-1.5 text-[11.5px] text-dim">{help}</p>}
      {busy && (
        <div className="mt-3">
          <Spinner label="Reading earlier rulings…" />
        </div>
      )}
      {error && (
        <p role="alert" className="mt-2 text-xs text-fail">
          {error}
        </p>
      )}
      {res && !busy && (
        <div className="mt-3 border border-line bg-sheet px-4 py-3">
          {candidates.length === 0 ? (
            <p className="text-[13px] text-dim">No earlier ruling states a subheading for a product like this. Ask your broker for the classification.</p>
          ) : (
            <ul className="flex flex-col gap-3">
              {candidates.map((c) => (
                <li key={c.hts} className="flex flex-wrap items-start gap-x-4 gap-y-1.5">
                  <div className="min-w-0 flex-1 basis-72">
                    <p className="flex flex-wrap items-center gap-2 text-[13.5px]">
                      <b className="font-medium tabular-nums text-ink">{c.hts}</b>
                      {c.duty_rates_available && <Chip tone="ok">sourced duty rates available</Chip>}
                    </p>
                    {c.reason && <p className="duo mt-1 text-[13px]">{c.reason}</p>}
                    {c.supporting_rulings?.length ? (
                      <p className="mt-1 text-xs text-dim">
                        From{" "}
                        {c.supporting_rulings.map((r, i) => (
                          <span key={r.id}>
                            {i > 0 && ", "}
                            <a href={r.url || rulingUrl(r.id ?? "")} target="_blank" rel="noreferrer" className="text-ok underline underline-offset-2" title={r.subject}>
                              {r.id}
                            </a>
                            {r.date && ` (${r.date.slice(0, 4)})`}
                          </span>
                        ))}
                      </p>
                    ) : null}
                  </div>
                  <Button variant={value.trim() === c.hts ? "primary" : "ghost"} onClick={() => onChange(c.hts ?? "")} aria-pressed={value.trim() === c.hts}>
                    {value.trim() === c.hts ? "Using this code" : "Use this code"}
                  </Button>
                </li>
              ))}
            </ul>
          )}
          {res.reasons_unavailable && <p className="mt-2 text-xs text-dim">Reasons are unavailable right now; the rulings are listed.</p>}
          <p className="mt-3 border-t border-line pt-2 text-xs text-ask">{res.note ?? "suggestion only — confirm the classification with a licensed broker"}</p>
        </div>
      )}
    </div>
  );
}
