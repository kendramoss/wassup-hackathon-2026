"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { api, type CaseSummary, type DemoList, type WatchRunResponse } from "@/lib/api/client";
import { copy } from "@/lib/copy";
import { formatDate, stepIndex, stepNames } from "@/lib/format";
import { ErrorBox, Main } from "../chrome";
import { Button, ButtonLink, Chip, Country, Spinner, StatusChip, cx } from "../ui";

export function CasesHome() {
  const [cases, setCases] = useState<CaseSummary[] | null>(null);
  const [demos, setDemos] = useState<DemoList["demos"]>([]);
  const [error, setError] = useState<unknown>(null);
  const [checking, setChecking] = useState(false);
  const [checked, setChecked] = useState<WatchRunResponse | null>(null);

  const load = () => {
    setError(null);
    api.listCases().then((r) => setCases(r.cases), setError);
  };
  useEffect(() => {
    api.listCases().then((r) => setCases(r.cases), setError);
    api.demos().then((r) => setDemos(r.demos ?? []), () => setDemos([]));
  }, []);

  const recheck = async () => {
    setChecking(true);
    try {
      setChecked(await api.watchAll());
      load();
    } catch (e) {
      setError(e);
    } finally {
      setChecking(false);
    }
  };

  return (
    <Main>
      <div className="flex flex-wrap items-end gap-3">
        <div>
          <h1 className="text-[28px] tracking-[-0.05em]">{copy.cases.title}</h1>
          <p className="duo mt-1 text-dim">{cases ? `${cases.length} ${cases.length === 1 ? "case" : "cases"}, newest first` : " "}</p>
        </div>
        <div className="ml-auto flex flex-wrap items-center gap-2.5">
          {checked && (
            <span role="status" className="border border-edge bg-white px-3 py-2 text-[12.5px] shadow-[3px_3px_0_var(--shadow)]">
              ✓ Re-checked {checked.checked} · {checked.changed.length} changed · {checked.failed.length} failed
            </span>
          )}
          <Button variant="ghost" onClick={recheck} disabled={checking || !cases?.length}>
            {checking ? "Re-checking…" : copy.cases.recheckAll}
          </Button>
          <ButtonLink href="/cases/new">{copy.cases.newCase}</ButtonLink>
        </div>
      </div>

      {error ? (
        <div className="mt-6">
          <ErrorBox error={error} onRetry={load} />
        </div>
      ) : null}

      {cases === null && !error && (
        <div className="mt-8">
          <Spinner label="Loading cases…" />
        </div>
      )}

      {cases && cases.length === 0 && (
        <p className="duo mt-8 border border-dashed border-guide bg-sheet px-5 py-4 text-dim">{copy.cases.empty}</p>
      )}

      {cases && cases.length > 0 && (
        <div className="relative mt-6 overflow-x-auto">
          <table className="w-full min-w-[760px] border-collapse border border-edge bg-sheet text-[13px] shadow-[4px_4px_0_var(--shadow)]">
            <thead>
              <tr>
                {copy.cases.columns.map((h) => (
                  <th key={h} scope="col" className="border-b border-line bg-paper px-3.5 py-2.5 text-left text-[10.5px] font-medium uppercase tracking-[0.08em] text-dim">
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {cases.map((c) => {
                const n = stepIndex[c.step] ?? 1;
                const href = `/cases/${encodeURIComponent(c.id)}/${["product", "origin", "origin", "plan", "draft"][n]}`;
                return (
                  <tr key={c.id} className="border-b border-line last:border-0 hover:bg-paper">
                    <td className="px-3.5 py-3">
                      <Link href={href} className="block font-medium hover:underline">
                        {c.display_name || c.product_name}
                        {c.pinned ? <Chip className="ml-2 align-middle">Demo</Chip> : null}
                      </Link>
                      <span className="text-xs text-dim">Case date {formatDate(c.date)}</span>
                    </td>
                    <td className="px-3.5 py-3 text-[17px] tracking-[-0.03em]">
                      {c.predicted_origin ? <Country name={c.predicted_origin} size={13} /> : <span className="text-sm text-dim">{copy.cases.notAnalysed}</span>}
                    </td>
                    <td className="px-3.5 py-3">
                      <StatusChip route={c.triage_route} />
                    </td>
                    <td className={cx("px-3.5 py-3 text-xs", c.watch_status === "changed" ? "font-medium text-ask" : c.watch_status ? "text-ok" : "text-dim")}>
                      {c.watch_status === "changed" ? "Changed" : c.watch_status === "unchanged" ? "Unchanged" : "—"}
                    </td>
                    <td className="px-3.5 py-3">{c.scenario_count || "—"}</td>
                    <td className="px-3.5 py-3 text-dim">
                      Step {n} of 4 · {stepNames[n - 1]}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      {demos && demos.length > 0 && (
        <section className="mt-12">
          <h2 className="text-lg tracking-[-0.04em]">{copy.cases.demos}</h2>
          <div className="mt-3.5 grid grid-cols-1 gap-3.5 sm:grid-cols-2 lg:grid-cols-4">
            {demos.map((d) => (
              <Link
                key={d.id}
                href={`/cases/new?demo=${encodeURIComponent(d.id ?? "")}`}
                className="flex flex-col gap-2 rounded-[2px] border border-edge bg-sheet p-4 text-sm hover:shadow-[4px_4px_0_var(--shadow)]"
              >
                <b className="font-medium leading-snug">{d.title}</b>
                <span className="duo text-[13px] text-dim">{d.description}</span>
                {d.cbp_held && <span className="text-xs text-dim">CBP held {d.cbp_held}</span>}
                <span className="mt-auto flex items-center gap-2 text-[13px] text-ok">
                  Load demo →{d.has_plan && <Chip>has a plan</Chip>}
                </span>
              </Link>
            ))}
          </div>
        </section>
      )}
    </Main>
  );
}
