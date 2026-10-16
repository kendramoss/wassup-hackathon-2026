"use client";

import Link from "next/link";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { api, ApiError, type CaseSummary } from "@/lib/api/client";
import { copy } from "@/lib/copy";
import { stepIndex } from "@/lib/format";
import { ACTIONS_ON_TOP, ActionSlot, Portal, useActionSlot } from "./action-bar";
import { Button, ButtonLink, cx } from "./ui";

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
      <ButtonLink href="/cases/new" className="ml-auto min-h-9 py-1.5">
        {copy.cases.newCase}
      </ButtonLink>
    </header>
  );
}

export type CaseTab = "product" | "origin" | "scenarios" | "plan" | "draft";

const tabs: { key: CaseTab; label: string; step: number; path: string }[] = [
  { key: "product", label: "Product", step: 1, path: "product" },
  { key: "origin", label: "Origin", step: 2, path: "origin" },
  { key: "scenarios", label: "What if", step: 2, path: "scenarios" },
  { key: "plan", label: "Plan", step: 3, path: "plan" },
  { key: "draft", label: "Draft for broker", step: 4, path: "draft" },
];

export const tabStep: Record<CaseTab, number> = { product: 1, origin: 2, scenarios: 2, plan: 3, draft: 4 };

// Case tabs: every case action one click away, each a separate quiet button
// (bordered, never filled) so "+ New case" stays the loudest control. A tick
// marks steps the case has done; tabs a new case can't open yet are inert.
export function CaseTabs({
  caseId,
  current,
  reached,
  title,
  onAsk,
}: {
  caseId?: string;
  current: CaseTab;
  reached: number;
  title: ReactNode;
  onAsk?: () => void;
}) {
  return (
    <>
    <nav aria-label="Case" className="no-print flex min-h-[52px] flex-wrap items-center gap-x-2 gap-y-2 border-b border-line bg-paper px-4 py-2 text-[13px] sm:px-10">
      <ul className="flex flex-wrap items-center gap-1.5">
        {tabs.map((t) => {
          const on = t.key === current;
          const open = Boolean(caseId) && (t.step <= Math.max(reached, 2) || t.key === "plan" || t.key === "draft");
          const done = !on && t.step < 4 && t.step <= reached && t.key !== "scenarios";
          const cls = cx(
            "inline-flex min-h-8 items-center gap-1.5 rounded-[2px] border px-3 py-1",
            on ? "border-ink bg-white font-medium text-ink" : open ? "border-line bg-sheet text-dim hover:border-edge hover:text-ink" : "border-dashed border-line text-faint",
          );
          const inner = (
            <>
              {done && (
                <span aria-hidden="true" className="text-ok">
                  ✓
                </span>
              )}
              {t.label}
              {done && <span className="sr-only">(done)</span>}
            </>
          );
          return (
            <li key={t.key}>
              {on || !open ? (
                <span className={cls} aria-current={on ? "page" : undefined} aria-disabled={!on && !open ? true : undefined}>
                  {inner}
                </span>
              ) : (
                <Link className={cls} href={`/cases/${encodeURIComponent(caseId!)}/${t.path}`}>
                  {inner}
                </Link>
              )}
            </li>
          );
        })}
      </ul>
      <div className="ml-auto flex flex-wrap items-center gap-3">
        {onAsk && (
          <button type="button" onClick={onAsk} className="inline-flex min-h-8 items-center rounded-[2px] border border-line bg-sheet px-3 py-1 text-dim hover:border-edge hover:text-ink">
            Ask about this case
          </button>
        )}
        <CaseSwitcher caseId={caseId} title={title} />
      </div>
    </nav>
    <ActionSlot />
    </>
  );
}

const stepHref = (id: string, n: number) => `/cases/${encodeURIComponent(id)}/${["", "product", "origin", "plan", "draft"][n]}`;

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
                  {c.display_name || c.product_name}
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
  const slot = useActionSlot();
  if (ACTIONS_ON_TOP && slot) return <Portal to={slot}>{children}</Portal>;
  if (ACTIONS_ON_TOP) return null; // the slot mounts with the tabs; actions appear there
  return (
    <>
      <div className="h-24" aria-hidden="true" />
      <footer className="no-print fixed inset-x-0 bottom-0 z-10 flex min-h-[68px] flex-wrap items-center gap-3 border-t border-dashed border-guide bg-paper px-4 py-2 sm:px-10">
        {children}
      </footer>
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

// Rate limits (429) are not failures: a calm note where the action was,
// counting down, with the retry re-enabled at zero (DESIGN.md, D45).
function RateLimitNote({ seconds, onRetry }: { seconds: number; onRetry?: () => void }) {
  const [left, setLeft] = useState(seconds);
  useEffect(() => {
    if (left <= 0) return;
    const t = setTimeout(() => setLeft((l) => l - 1), 1000);
    return () => clearTimeout(t);
  }, [left]);
  return (
    <div role="status" className="flex flex-wrap items-center gap-3 border border-dashed border-guide bg-sheet px-4 py-3 text-sm">
      <span className="duo">
        {left > 0 ? (
          <>
            Too many requests. Try again in <span className="font-mono">{left} s</span>. Nothing is lost.
          </>
        ) : (
          "Ready to try again."
        )}
      </span>
      {onRetry && (
        <Button variant="ghost" className="min-h-8 px-3 py-1 text-[12.5px]" disabled={left > 0} onClick={onRetry}>
          {copy.retry}
        </Button>
      )}
    </div>
  );
}

export function ErrorBox({ error, onRetry }: { error: unknown; onRetry?: () => void }) {
  const e = error instanceof ApiError ? error : null;
  if (e?.code === "rate_limited") return <RateLimitNote key={String(e.retryAfter)} seconds={Math.ceil(e.retryAfter ?? 30)} onRetry={onRetry} />;
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
