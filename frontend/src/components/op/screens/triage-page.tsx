"use client";

import Link from "next/link";
import { useId, useRef, useState } from "react";
import { api, type BatchResponse } from "@/lib/api/client";
import { conf, seconds } from "@/lib/format";
import { Elapsed, ErrorBox, Main } from "../chrome";
import { Button, Chip, Country, ScrollX, StatusChip, cx } from "../ui";

// Portfolio triage (DESIGN.md, backend D47, locked 16 Oct, option 1 of
// c-batch.html; frames p-batch-upload, p-batch-running, p-batch).

type Row = BatchResponse["rows"][number];
type Preview = { name: string; facts: string; date: string }[];

const MAX = 20;
const OUT = /^out of scope:\s*/i;

// Enough CSV to preview the file before sending it: quoted fields may hold
// commas, quotes ("") and line breaks. The backend parses it for real.
function parseCsv(text: string): string[][] {
  const rows: string[][] = [];
  let row: string[] = [];
  let cell = "";
  let quoted = false;
  for (let i = 0; i < text.length; i++) {
    const ch = text[i];
    if (quoted) {
      if (ch === '"' && text[i + 1] === '"') {
        cell += '"';
        i++;
      } else if (ch === '"') quoted = false;
      else cell += ch;
    } else if (ch === '"') quoted = true;
    else if (ch === ",") {
      row.push(cell);
      cell = "";
    } else if (ch === "\n" || ch === "\r") {
      if (ch === "\r" && text[i + 1] === "\n") i++;
      row.push(cell);
      if (row.some((c) => c.trim())) rows.push(row);
      row = [];
      cell = "";
    } else cell += ch;
  }
  row.push(cell);
  if (row.some((c) => c.trim())) rows.push(row);
  return rows;
}

function preview(text: string): Preview {
  const [head, ...body] = parseCsv(text);
  if (!head) return [];
  const h = head.map((c) => c.trim().toLowerCase());
  const ni = h.findIndex((c) => c === "name" || c === "product" || c === "sku");
  const fi = h.findIndex((c) => c === "facts" || c === "description");
  const di = h.findIndex((c) => c === "date" || c === "case date");
  if (fi < 0) return [];
  return body.map((r) => ({
    name: (ni >= 0 ? r[ni] : "")?.trim() ?? "",
    facts: r[fi]?.trim() ?? "",
    date: (di >= 0 ? r[di] : "")?.trim() ?? "",
  }));
}

// What to do next first: more facts, expert review, out of scope, then auto;
// failures last, in red.
function rank(r: Row): number {
  if (r.error) return 4;
  if (r.out_of_scope) return 2;
  return { needs_info: 0, needs_review: 1, auto: 3 }[r.triage_route ?? "needs_review"] ?? 1;
}

// Model cost per product is cents, so three decimals: "$0.021".
const usd = (n: number | null | undefined) => (n == null ? "—" : `$${n.toFixed(3)}`);

function csvCell(v: unknown): string {
  const s = v == null ? "" : String(v);
  return /[",\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s;
}

export function TriagePage() {
  const inputId = useId();
  const [file, setFile] = useState<File | null>(null);
  const [rows, setRows] = useState<Preview | null>(null);
  const [drag, setDrag] = useState(false);
  const [since, setSince] = useState<number | null>(null);
  const [result, setResult] = useState<BatchResponse | null>(null);
  const [error, setError] = useState<unknown>(null);
  const abort = useRef<AbortController | null>(null);

  const pick = async (f: File) => {
    setError(null);
    setResult(null);
    setFile(f);
    setRows(/\.csv$/i.test(f.name) || f.type === "text/csv" ? preview(await f.text()) : null);
  };

  const loadSample = async () => {
    const r = await fetch("/samples/portfolio_sample.csv");
    await pick(new File([await r.blob()], "portfolio_sample.csv", { type: "text/csv" }));
  };

  const run = async () => {
    if (!file) return;
    setError(null);
    setSince(Date.now());
    abort.current = new AbortController();
    try {
      // A CSV is already parsed for the preview. Send those rows, including
      // one with empty facts, so that row comes back failed. The file upload
      // drops empty facts rows. Excel has no preview and still goes as a file.
      const result = rows
        ? await api.batchItems(
            rows.map((r) => ({ name: r.name, facts: r.facts, ...(r.date ? { date: r.date } : {}) })),
            abort.current.signal,
          )
        : await api.batch(file, abort.current.signal);
      setResult(result);
    } catch (e) {
      if (!(e instanceof DOMException && e.name === "AbortError")) setError(e);
    } finally {
      setSince(null);
    }
  };

  const reset = () => {
    setFile(null);
    setRows(null);
    setResult(null);
    setError(null);
  };

  const tooMany = rows !== null && rows.length > MAX;

  return (
    <Main>
      <Link href="/" className="text-[13px] text-ok hover:underline">
        ← Cases
      </Link>
      <h1 className="mt-3 text-[28px] tracking-[-0.05em]">Triage a portfolio</h1>

      {result ? (
        <Results file={file} result={result} onAgain={reset} />
      ) : since !== null ? (
        <Running count={rows?.length} since={since} onCancel={() => abort.current?.abort()} />
      ) : (
        <>
          <p className="duo mt-1 max-w-[70ch] text-dim">
            Up to {MAX} products at once. Each gets a likely origin and goes to auto-accept, more facts or review, and is saved as a case you can open.
          </p>

          {file && (
            <section className="mt-8 border border-edge bg-sheet shadow-[4px_4px_0_var(--shadow)]" aria-label="File to triage">
              <header className="flex flex-wrap items-center gap-3 border-b border-line bg-paper px-4 py-2.5 text-[13px]">
                <b className="font-medium">{file.name}</b>
                {rows && <span className="text-dim">{rows.length} products</span>}
                <label htmlFor={inputId} className="ml-auto cursor-pointer text-ok hover:underline">
                  replace file
                </label>
              </header>
              {rows ? (
                <ol>
                  {rows.slice(0, MAX + 5).map((r, i) => (
                    <li key={i} className="grid grid-cols-[2rem_minmax(0,14rem)_minmax(0,1fr)] gap-3 border-b border-line px-4 py-2.5 text-[13px] last:border-b-0">
                      <span className="text-dim">{i + 1}</span>
                      <b className={cx("truncate font-medium", !r.name && "text-dim")}>{r.name || "no name"}</b>
                      <span className={cx("duo truncate", r.facts ? "text-dim" : "text-fail")}>{r.facts.split("\n")[0] || "no facts: this row will fail"}</span>
                    </li>
                  ))}
                </ol>
              ) : (
                <p className="duo px-4 py-3 text-[13px] text-dim">Excel file: the products are read when you start.</p>
              )}
            </section>
          )}

          {file && (
            <div className="mt-5 flex flex-wrap items-center gap-4">
              <Button onClick={run} disabled={tooMany || rows?.length === 0}>
                Triage {rows ? `${rows.length} product${rows.length === 1 ? "" : "s"}` : "the file"}
              </Button>
              <span className="duo text-[13px] text-dim">
                {tooMany
                  ? `This file has ${rows!.length} products; the limit is ${MAX}. Split it and triage each part.`
                  : "About 15–45 s and $0.07 for five products · each becomes a case"}
              </span>
            </div>
          )}
          {error ? (
            <div className="mt-5">
              <ErrorBox error={error} onRetry={run} />
            </div>
          ) : null}

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
              if (f) pick(f);
            }}
            className={cx("mt-8 block cursor-pointer border border-dashed px-6 py-7 text-center", drag ? "border-ok bg-ok-wash" : "border-guide bg-sheet")}
          >
            <span className="text-sm">{file ? "Or drop another file" : "Drop a CSV or Excel file, or choose one"}</span>
            <span className="duo mt-1 block text-xs text-dim">One row per product. Up to {MAX}.</span>
            <input id={inputId} type="file" accept=".csv,.xlsx,text/csv,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" className="sr-only" onChange={(e) => e.target.files?.[0] && pick(e.target.files[0])} />
          </label>
          {!file && (
            <button type="button" onClick={loadSample} className="mt-3 text-[13px] text-ok hover:underline">
              Try the sample (5 products)
            </button>
          )}

          <table className="mt-6 w-full max-w-[640px] border-collapse text-[13px]">
            <caption className="eyebrow mb-2 text-left">Columns</caption>
            <tbody>
              {[
                ["name", "required", "what you call the product"],
                ["facts", "required", "where parts are made and what happens where"],
                ["date", "", "case date; defaults to today"],
              ].map(([c, req, what]) => (
                <tr key={c} className="border-t border-line">
                  <th scope="row" className="py-2 pr-4 text-left font-medium">
                    {c} {req && <span className="text-[11px] font-normal text-dim">{req}</span>}
                  </th>
                  <td className="duo py-2 text-dim">{what}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </>
      )}
    </Main>
  );
}

function Running({ count, since, onCancel }: { count?: number; since: number; onCancel: () => void }) {
  return (
    <section className="mt-8 border border-edge bg-sheet px-6 py-6 shadow-[4px_4px_0_var(--shadow)]" aria-live="polite">
      <p className="eyebrow">Triaging{count ? ` ${count} products` : ""}</p>
      <p className="mt-2 text-[44px] leading-none tracking-[-0.05em]">
        <Elapsed since={since} />
      </p>
      <div className="relative mt-5 h-1 overflow-hidden bg-wash" role="progressbar" aria-label="Triaging">
        <i className="absolute inset-y-0 w-1/3 animate-[slide_1.6s_ease-in-out_infinite] bg-ok" />
      </div>
      <p className="duo mt-3 text-[13px] text-dim">Each product is predicted, triaged and saved as a case. Usually 15–45 s for five; no partial results.</p>
      <Button variant="ghost" className="mt-4" onClick={onCancel}>
        Cancel
      </Button>
    </section>
  );
}

function Results({ file, result, onAgain }: { file: File | null; result: BatchResponse; onAgain: () => void }) {
  const s = result.summary ?? {};
  const rows = [...(result.rows ?? [])].sort((a, b) => rank(a) - rank(b));
  const total = s.total ?? rows.length;
  const failed = s.failed ?? 0;
  const auto = s.auto ?? 0;
  const out = s.out_of_scope ?? 0;
  const look = total - auto - failed;
  const split = [
    { n: auto, label: "auto-accepted", cls: "bg-ok" },
    { n: s.needs_info ?? 0, label: "needs more facts", cls: "bg-ask" },
    { n: (s.needs_review ?? 0) - out, label: "needs expert review", cls: "bg-ask-line" },
    { n: out, label: "out of scope", cls: "bg-dim" },
    { n: failed, label: "failed", cls: "bg-fail" },
  ].filter((x) => x.n > 0);

  const download = () => {
    const head = ["name", "case_id", "predicted_origin", "confidence", "triage_route", "out_of_scope", "reason", "cost_usd", "error"];
    const lines = [head.join(","), ...rows.map((r) => head.map((k) => csvCell(r[k as keyof Row])).join(","))];
    const url = URL.createObjectURL(new Blob([lines.join("\n") + "\n"], { type: "text/csv" }));
    const a = document.createElement("a");
    a.href = url;
    a.download = `triage-${(file?.name ?? "portfolio").replace(/\.\w+$/, "")}.csv`;
    a.click();
    URL.revokeObjectURL(url);
  };

  return (
    <>
      <p className="mt-1 text-[13px] text-dim">
        {file?.name ? `${file.name} · ` : ""}
        {total} products
      </p>
      <section className="mt-6 grid gap-6 border border-edge bg-sheet px-6 py-5 shadow-[4px_4px_0_var(--shadow)] lg:grid-cols-[minmax(0,1fr)_260px]">
        <div>
          <dl className="flex flex-wrap gap-x-10 gap-y-3">
            {[
              [total, "products"],
              [auto, "auto-accepted"],
              [look, "need a look"],
              [failed, "failed"],
            ].map(([n, l]) => (
              <div key={l}>
                <dd className={cx("text-[40px] leading-none tracking-[-0.05em]", l === "failed" && Number(n) > 0 && "text-fail")}>{n}</dd>
                <dt className="mt-1 text-xs text-dim">{l}</dt>
              </div>
            ))}
          </dl>
          <div className="mt-5 flex h-2 w-full overflow-hidden bg-wash" aria-hidden="true">
            {split.map((x) => (
              <i key={x.label} className={x.cls} style={{ width: `${(x.n / Math.max(total, 1)) * 100}%` }} />
            ))}
          </div>
          <p className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-dim">
            {split.map((x) => (
              <span key={x.label} className="inline-flex items-center gap-1.5">
                <i className={cx("inline-block size-2", x.cls)} aria-hidden="true" />
                {x.label} {x.n}
              </span>
            ))}
          </p>
        </div>
        <div className="border-t border-dashed border-guide pt-4 text-[13px] lg:border-l lg:border-t-0 lg:pl-6 lg:pt-0">
          <p className="duo">On 377 held-out rulings, 79.6% were auto-accepted.</p>
          <p className="duo mt-1 text-xs text-dim">A few products can land far from that; it isn&apos;t the fast-path rate.</p>
          <p className="mt-3 text-xs text-dim">
            {usd(s.cost_usd)} · {s.latency_ms != null ? seconds(s.latency_ms) : "—"}
          </p>
        </div>
      </section>

      <section className="mt-10" aria-labelledby="look-h">
        <h2 id="look-h" className="eyebrow mb-3">
          Needs a look first <span className="normal-case tracking-normal text-dim">· sorted by what to do next</span>
        </h2>
        <ScrollX label="Triage results">
          <table className="w-full min-w-[760px] border-collapse border border-edge bg-sheet text-[13px] shadow-[4px_4px_0_var(--shadow)]">
            <thead>
              <tr>
                {["Product", "Likely origin", "Status", "Cost", ""].map((h, i) => (
                  <th key={i} scope="col" className="border-b border-line bg-paper px-3.5 py-2.5 text-left text-[11px] font-medium uppercase tracking-[0.08em] text-dim">
                    {h || <span className="sr-only">Open</span>}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rows.map((r, i) => (
                <tr key={r.case_id ?? i} className={cx("border-b border-line align-top", r.error && "bg-fail-wash")}>
                  <td className="px-3.5 py-3">
                    <b className="font-medium">{r.name}</b>
                    <p className={cx("duo mt-0.5 text-xs", r.error ? "text-fail" : "text-dim")}>{r.error ?? r.reason?.replace(OUT, "")}</p>
                  </td>
                  <td className="whitespace-nowrap px-3.5 py-3">
                    {r.predicted_origin ? (
                      <>
                        <Country name={r.predicted_origin} /> <span className="text-dim">{conf(r.confidence)}</span>
                      </>
                    ) : (
                      "—"
                    )}
                  </td>
                  <td className="px-3.5 py-3">{r.error ? <Chip tone="fail">Failed</Chip> : r.out_of_scope ? <Chip>Out of scope</Chip> : <StatusChip route={r.triage_route} />}</td>
                  <td className="px-3.5 py-3 text-dim">{r.cost_usd != null ? usd(r.cost_usd) : "—"}</td>
                  <td className="whitespace-nowrap px-3.5 py-3 text-right">
                    {r.case_id && (
                      <Link href={`/cases/${encodeURIComponent(r.case_id)}/origin`} className="text-ok hover:underline">
                        Open case →
                      </Link>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </ScrollX>
      </section>

      <div className="mt-6 flex flex-wrap items-center gap-3">
        <Button onClick={download}>Download results (CSV)</Button>
        <Button variant="ghost" onClick={onAgain}>
          Triage another file
        </Button>
        <span className="text-[13px] text-dim">
          All {total - failed} are saved on{" "}
          <Link href="/" className="text-ok hover:underline">
            Cases
          </Link>
        </span>
      </div>
    </>
  );
}
