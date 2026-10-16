"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { api, type CaseResponse, type CaseSummary, type DemoList } from "@/lib/api/client";
import { copy } from "@/lib/copy";
import { BAR, conf, formatDate, money, stepIndex, stepNames } from "@/lib/format";
import { currentOrigin } from "../case-shell";
import { checkWords, movePhrase } from "../plan-bits";
import { ErrorBox, Main } from "../chrome";
import { Button, Chip, Country, ScrollX, Spinner, StatusChip, cx } from "../ui";

const caseHref = (c: Pick<CaseSummary, "id" | "step">) =>
  `/cases/${encodeURIComponent(c.id)}/${["product", "product", "origin", "plan", "draft"][stepIndex[c.step] ?? 2]}`;

// Cases home (DESIGN.md, locked 16 Oct, option 2): pinned demos as cards,
// then "Your cases" as a table with a ⋯ menu (Open, Rename, Delete).
export function CasesHome() {
  const [cases, setCases] = useState<CaseSummary[] | null>(null);
  const [demos, setDemos] = useState<DemoList["demos"]>([]);
  const [error, setError] = useState<unknown>(null);

  const load = () => {
    setError(null);
    api.listCases().then((r) => setCases(r.cases), setError);
  };
  useEffect(() => {
    api.listCases().then((r) => setCases(r.cases), setError);
    api.demos().then((r) => setDemos(r.demos ?? []), () => setDemos([]));
  }, []);

  const pinned = cases?.filter((c) => c.pinned) ?? [];
  const yours = cases?.filter((c) => !c.pinned) ?? [];

  return (
    <Main>
      <div className="flex flex-wrap items-end gap-3">
        <div>
          <h1 className="text-[28px] tracking-[-0.05em]">{copy.cases.title}</h1>
          <p className="duo mt-1 text-dim">Predict the origin CBP would likely assign, then plan a move that earlier rulings support.</p>
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

      {cases && (
        <section className="mt-8" aria-labelledby="yours-h">
          <h2 id="yours-h" className="eyebrow mb-3">
            Your cases {yours.length ? `· ${yours.length}` : ""}
          </h2>
          {yours.length === 0 ? (
            <p className="duo border border-dashed border-guide bg-sheet px-5 py-4 text-dim">
              {copy.cases.empty}{" "}
              <Link href="/cases/new" className="text-ok underline underline-offset-2">
                Start a case
              </Link>
            </p>
          ) : (
            <YourCases cases={yours} onChange={(next) => setCases([...pinned, ...next])} />
          )}
        </section>
      )}
      {pinned.length > 0 && (
        <section className="mt-12" aria-labelledby="demos-h">
          <h2 id="demos-h" className="eyebrow mb-3">
            Demos · real CBP facts
          </h2>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
            {pinned.map((c) => (
              <DemoCard key={c.id} c={c} />
            ))}
          </div>
        </section>
      )}

      {cases && pinned.length === 0 && demos && demos.length > 0 && (
        <section className="mt-12" aria-labelledby="load-h">
          <h2 id="load-h" className="eyebrow mb-3">
            {copy.cases.demos}
          </h2>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
            {demos.map((d) => (
              <Link
                key={d.id}
                href={`/cases/new?demo=${encodeURIComponent(d.id ?? "")}`}
                className="flex flex-col gap-2 rounded-[2px] border border-edge bg-sheet p-4 text-sm hover:shadow-[4px_4px_0_var(--shadow)]"
              >
                <b className="font-medium leading-snug">{d.title}</b>
                <span className="duo text-[13px] text-dim">{d.description}</span>
                {d.cbp_held && <span className="text-xs text-dim">CBP held {d.cbp_held}</span>}
                <span className="mt-auto text-[13px] text-ok">Load demo →</span>
              </Link>
            ))}
          </div>
        </section>
      )}

    </Main>
  );
}

function DemoCard({ c }: { c: CaseSummary }) {
  const [full, setFull] = useState<CaseResponse | null>(null);
  useEffect(() => {
    api.getCase(c.id).then(setFull, () => {});
  }, [c.id]);
  const cur = full ? currentOrigin(full) : undefined;
  const best = full?.plan?.plans?.[0];
  return (
    <Link href={caseHref(c)} className="flex flex-col gap-2.5 rounded-[2px] border border-edge bg-sheet p-4 text-sm shadow-[4px_4px_0_var(--shadow)] hover:bg-white">
      <span className="flex items-start justify-between gap-2">
        <b className="font-medium leading-snug">{c.display_name}</b>
        <Chip className="shrink-0">Demo</Chip>
      </span>
      <span className="text-[26px] leading-none tracking-[-0.05em]">
        <Country name={c.predicted_origin} size={18} />
      </span>
      <span className="flex flex-wrap items-center gap-2">
        <StatusChip route={c.triage_route} />
        <span className="text-xs text-dim">confidence {conf(c.confidence)}</span>
      </span>
      {cur?.country && (
        <span className="text-xs text-dim">
          CBP held <Country name={cur.country} /> {cur.ruling ? `(${cur.ruling})` : ""}
        </span>
      )}
      <span className="duo mt-auto border-t border-line pt-2 text-xs">
        {best ? (
          <>
            <span className="text-ok">Plan:</span> make {movePhrase(best)} in {full?.plan?.target} · check {conf(best.verdict?.confidence)} ({checkWords(best.verdict?.confidence)}) · {money(best.net_per_year)}/yr
          </>
        ) : full?.plan ? (
          <span className="text-ask">No plan cleared the {BAR} bar</span>
        ) : (
          <span className="text-dim">No plan search yet</span>
        )}
      </span>
    </Link>
  );
}

function YourCases({ cases, onChange }: { cases: CaseSummary[]; onChange: (c: CaseSummary[]) => void }) {
  const [renaming, setRenaming] = useState<string | null>(null);
  const [confirming, setConfirming] = useState<string | null>(null);
  const [rowError, setRowError] = useState<{ id: string; error: unknown } | null>(null);

  const rename = async (id: string, name: string) => {
    const old = cases.find((c) => c.id === id)?.display_name;
    setRenaming(null);
    if (!name.trim() || name.trim() === old) return;
    onChange(cases.map((c) => (c.id === id ? { ...c, display_name: name.trim() } : c)));
    try {
      await api.rename(id, name.trim());
    } catch (e) {
      onChange(cases.map((c) => (c.id === id ? { ...c, display_name: old ?? c.display_name } : c)));
      setRowError({ id, error: e });
    }
  };
  const remove = async (id: string) => {
    try {
      await api.deleteCase(id);
      setConfirming(null);
      onChange(cases.filter((c) => c.id !== id));
    } catch (e) {
      setRowError({ id, error: e });
    }
  };

  return (
    <ScrollX label="Your cases">
      <table className="w-full min-w-[760px] border-collapse border border-edge bg-sheet text-[13px] shadow-[4px_4px_0_var(--shadow)]">
        <thead>
          <tr>
            {[...copy.cases.columns, ""].map((h, i) => (
              <th key={i} scope="col" className="border-b border-line bg-paper px-3.5 py-2.5 text-left text-[10.5px] font-medium uppercase tracking-[0.08em] text-dim">
                {h || <span className="sr-only">Actions</span>}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {cases.map((c) => {
            const n = stepIndex[c.step] ?? 1;
            if (confirming === c.id)
              return (
                <tr key={c.id} className="border-b border-line bg-fail-wash">
                  <td colSpan={7} className="px-3.5 py-3">
                    <span className="duo text-fail">
                      Delete &ldquo;{c.display_name}&rdquo;? Its prediction, plans and drafts are removed for everyone.
                    </span>
                    <span className="ml-3 inline-flex gap-2">
                      <Button className="min-h-8 border-fail bg-fail px-3 py-1 text-[12.5px] text-white shadow-none" onClick={() => remove(c.id)}>
                        Delete case
                      </Button>
                      <Button variant="ghost" className="min-h-8 px-3 py-1 text-[12.5px]" onClick={() => setConfirming(null)}>
                        Cancel
                      </Button>
                    </span>
                  </td>
                </tr>
              );
            return (
              <tr key={c.id} className="border-b border-line last:border-0 hover:bg-paper">
                <td className="px-3.5 py-3">
                  {renaming === c.id ? (
                    <RenameField value={c.display_name} onDone={(v) => (v === null ? setRenaming(null) : rename(c.id, v))} />
                  ) : (
                    <Link href={caseHref(c)} className="block font-medium hover:underline">
                      {c.display_name}
                    </Link>
                  )}
                  <span className="text-xs text-dim">Case date {formatDate(c.date)}</span>
                  {rowError?.id === c.id && (
                    <span role="alert" className="block text-xs text-fail">
                      {(rowError.error as Error).message}
                    </span>
                  )}
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
                <td className="px-2 py-3 text-right">
                  <RowMenu c={c} onRename={() => setRenaming(c.id)} onDelete={() => setConfirming(c.id)} />
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </ScrollX>
  );
}

function RenameField({ value, onDone }: { value: string; onDone: (v: string | null) => void }) {
  const [v, setV] = useState(value);
  const ref = useRef<HTMLInputElement>(null);
  useEffect(() => {
    ref.current?.select();
  }, []);
  return (
    <input
      ref={ref}
      aria-label="Case name (Enter saves, Esc cancels)"
      maxLength={200}
      className="w-full rounded-[2px] border border-ok bg-white px-2 py-1 text-[13px]"
      value={v}
      onChange={(e) => setV(e.target.value)}
      onKeyDown={(e) => {
        if (e.key === "Enter") onDone(v);
        if (e.key === "Escape") onDone(null);
      }}
      onBlur={() => onDone(v)}
    />
  );
}

function RowMenu({ c, onRename, onDelete }: { c: CaseSummary; onRename: () => void; onDelete: () => void }) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const close = (e: MouseEvent | KeyboardEvent) => {
      if (e instanceof KeyboardEvent ? e.key === "Escape" : !ref.current?.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", close);
    document.addEventListener("keydown", close);
    return () => {
      document.removeEventListener("mousedown", close);
      document.removeEventListener("keydown", close);
    };
  }, [open]);
  return (
    <div className="relative inline-block" ref={ref}>
      <button type="button" aria-label={`Actions for ${c.display_name}`} aria-expanded={open} className="px-2 py-1 text-base text-dim hover:text-ink" onClick={() => setOpen((o) => !o)}>
        ⋯
      </button>
      {open && (
        <div role="menu" className="absolute right-0 z-20 mt-1 w-40 border border-edge bg-sheet py-1 text-left text-[13px] shadow-[4px_4px_0_var(--shadow)]">
          <Link role="menuitem" href={caseHref(c)} className="block px-3 py-1.5 hover:bg-wash">
            Open
          </Link>
          <button role="menuitem" type="button" className="block w-full px-3 py-1.5 text-left hover:bg-wash" onClick={() => (setOpen(false), onRename())}>
            Rename
          </button>
          <button role="menuitem" type="button" className="block w-full px-3 py-1.5 text-left text-fail hover:bg-fail-wash" onClick={() => (setOpen(false), onDelete())}>
            Delete case…
          </button>
        </div>
      )}
    </div>
  );
}
