"use client";

import Link from "next/link";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { api, ApiError, type CaseSummary } from "@/lib/api/client";
import { copy } from "@/lib/copy";
import { stepIndex, stepNames } from "@/lib/format";
import { Button, cx } from "./ui";

// The active section is passed in rather than read from the URL, so the bar
// prerenders in the static shell of dynamic routes.
export function AppBar({ active = "cases" }: { active?: "cases" | "accuracy" }) {
  const onAccuracy = active === "accuracy";
  return (
    <header className="no-print flex min-h-14 flex-wrap items-center gap-x-7 gap-y-1 border-b border-line bg-paper px-4 py-2 sm:px-10">
      <Link href="/" className="text-[19px] font-semibold tracking-[-0.03em]">
        Origin <i className="not-italic text-ok">Proof</i>
      </Link>
      <nav aria-label="Main" className="flex gap-5 text-sm">
        <Link href="/" aria-current={!onAccuracy ? "page" : undefined} className={cx(!onAccuracy ? "text-ink underline underline-offset-[6px]" : "text-dim hover:text-ink")}>
          {copy.nav.cases}
        </Link>
        <Link href="/accuracy" aria-current={onAccuracy ? "page" : undefined} className={cx(onAccuracy ? "text-ink underline underline-offset-[6px]" : "text-dim hover:text-ink")}>
          {copy.nav.accuracy}
        </Link>
      </nav>
      <span className="ml-auto hidden text-[13px] text-dim md:block">{copy.disclaimer}</span>
    </header>
  );
}

const stepHref = (id: string, n: number) => `/cases/${encodeURIComponent(id)}/${["", "product", "origin", "plan", "draft"][n]}`;

// Product → Origin → Plan → Draft for broker. Steps up to the case's furthest
// step are links; the case name opens the switcher.
export function StepBar({ caseId, current, reached, title }: { caseId?: string; current: number; reached: number; title: ReactNode }) {
  return (
    <div className="no-print flex min-h-[52px] flex-wrap items-center gap-x-3 gap-y-2 border-b border-line bg-paper px-4 py-2 text-sm sm:px-10">
      <ol className="flex flex-wrap items-center gap-x-3 gap-y-1" aria-label="Steps">
        {stepNames.map((name, i) => {
          const n = i + 1;
          const done = n < current || (n !== current && n <= reached);
          const on = n === current;
          const box = (
            <span
              className={cx(
                "grid size-[22px] place-items-center rounded-[2px] border-[1.5px] font-mono text-[11px]",
                on && "border-ink bg-ink text-paper",
                done && !on && "border-ok bg-ok text-white",
                !on && !done && "border-line",
              )}
              aria-hidden="true"
            >
              {done && !on ? "✓" : n}
            </span>
          );
          const label = (
            <span className={cx("flex items-center gap-2", on ? "font-semibold text-ink" : done ? "text-dim" : "text-dim")}>
              {box}
              {name}
            </span>
          );
          return (
            <li key={name} className="flex items-center gap-3" aria-current={on ? "step" : undefined}>
              {i > 0 && <span className="hidden h-px w-10 bg-line sm:block" aria-hidden="true" />}
              {caseId && n <= Math.max(reached, 1) && !on && n !== 1 ? <Link href={stepHref(caseId, n)}>{label}</Link> : label}
            </li>
          );
        })}
      </ol>
      <div className="ml-auto">
        <CaseSwitcher caseId={caseId} title={title} />
      </div>
    </div>
  );
}

function CaseSwitcher({ caseId, title }: { caseId?: string; title: ReactNode }) {
  const [open, setOpen] = useState(false);
  const [cases, setCases] = useState<CaseSummary[] | null>(null);
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open || cases) return;
    api.listCases().then((r) => setCases(r.cases)).catch(() => setCases([]));
  }, [open, cases]);
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
    <div className="relative" ref={ref}>
      <button type="button" className="text-right text-[13px] text-ink hover:underline" aria-expanded={open} onClick={() => setOpen((o) => !o)}>
        {title} <span aria-hidden="true">▾</span>
      </button>
      {open && (
        <div className="absolute right-0 z-20 mt-2 w-80 max-w-[calc(100vw-32px)] rounded-[2px] border border-edge bg-sheet p-2 text-[13px] shadow-[4px_4px_0_var(--shadow)]">
          <ul>
            {(cases ?? []).filter((c) => c.id !== caseId).slice(0, 6).map((c) => (
              <li key={c.id}>
                <Link href={stepHref(c.id, Math.max(stepIndex[c.step] ?? 1, 2))} className="block px-2 py-1.5 hover:bg-wash">
                  {c.product_name}
                </Link>
              </li>
            ))}
            {cases === null && <li className="px-2 py-1.5 text-dim">Loading cases…</li>}
          </ul>
          <div className="mt-1 flex justify-between border-t border-line px-2 pt-2">
            <Link href="/" className="text-ok">All cases</Link>
            <Link href="/cases/new" className="text-ok">{copy.cases.newCase}</Link>
          </div>
        </div>
      )}
    </div>
  );
}

// The next step sits in a fixed footer.
export function Footer({ children }: { children: ReactNode }) {
  return (
    <>
      <div className="h-24" aria-hidden="true" />
      <div className="no-print fixed inset-x-0 bottom-0 z-10 flex min-h-[68px] flex-wrap items-center gap-3 border-t border-dashed border-guide bg-paper px-4 py-2 sm:px-10">
        {children}
      </div>
    </>
  );
}

export function Main({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <main id="main" className={cx("column mx-auto w-full max-w-[1240px] flex-1 px-4 py-8 sm:px-10", className)}>
      {children}
    </main>
  );
}

export function ErrorBox({ error, onRetry }: { error: unknown; onRetry?: () => void }) {
  const e = error instanceof ApiError ? error : null;
  const outOfScope = e?.code === "out_of_scope";
  const message = e?.message ?? (error instanceof Error ? error.message : "Something went wrong.");
  return (
    <div role="alert" className={cx("rounded-[2px] border px-4 py-3 text-sm", outOfScope ? "border-ask-line bg-ask-wash text-ask" : "border-fail bg-fail-wash text-fail")}>
      <p className="duo">
        {outOfScope ? "Out of scope: " : e?.code?.startsWith("model") ? "The model didn't answer: " : ""}
        {message}
      </p>
      {onRetry && (e?.retryable ?? true) && (
        <Button variant="ghost" className="mt-2" onClick={onRetry}>
          {copy.retry}
        </Button>
      )}
    </div>
  );
}

export function Elapsed({ since }: { since: number }) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(t);
  }, []);
  return <span className="font-mono">{Math.max(0, Math.round((now - since) / 1000))} s</span>;
}
