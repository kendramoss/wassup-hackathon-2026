"use client";

import Link from "next/link";
import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from "react";
import { api, ApiError, type CaseSummary } from "@/lib/api/client";
import { copy } from "@/lib/copy";
import { stepIndex } from "@/lib/format";
import { ACTIONS_ON_TOP, ActionSlot, Portal, useActionSlot, useNextSlot } from "./action-bar";
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
      {/* Case screens portal their case menu here, as a breadcrumb (17 Oct). */}
      <div data-case-crumb="" className="flex min-w-0 items-center border-line empty:hidden sm:border-l sm:pl-5" />
      <ButtonLink href="/cases/new" className="ml-auto min-h-9 py-1.5">
        {copy.cases.newCase}
      </ButtonLink>
    </header>
  );
}

export type CaseTab = "product" | "origin" | "scenarios" | "plan" | "draft";

// The four steps in order, then the optional scenario sandbox (#82): it sat
// between Origin and Plan and read as a required step.
const tabs: { key: CaseTab; label: string; step: number; path: string; optional?: boolean }[] = [
  { key: "product", label: "Product", step: 1, path: "product" },
  { key: "origin", label: "Origin", step: 2, path: "origin" },
  { key: "plan", label: "Plan", step: 3, path: "plan" },
  { key: "draft", label: "Draft for broker", step: 4, path: "draft" },
  { key: "scenarios", label: "Try scenarios", step: 2, path: "scenarios", optional: true },
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
  // One sticky row, read left to right: the steps, then the case menu and
  // Ask, then the step's main button (17 Oct).
  return (
    <div className="no-print z-20 flex min-h-[60px] flex-wrap items-center gap-x-4 gap-y-2 border-b border-line bg-paper px-4 py-2 text-[13px] sm:sticky sm:top-0 sm:px-10">
    <nav aria-label="Case">
      <ul className="flex flex-wrap items-center gap-1.5">
        {tabs.map((t) => {
          const on = t.key === current;
          const open = Boolean(caseId) && (t.step <= Math.max(reached, 2) || t.key === "plan" || t.key === "draft");
          const done = Boolean(caseId) && !on && t.step < 4 && t.step <= reached && t.key !== "scenarios";
          const cls = cx(
            "inline-flex min-h-8 items-center gap-1.5 rounded-[2px] border px-3 py-1",
            on ? "border-ink bg-white font-medium text-ink" : open ? "border-line bg-sheet text-dim hover:border-edge hover:text-ink" : "border-transparent text-dim",
            t.optional && !on && open && "border-dashed bg-transparent",
          );
          const inner = (
            <>
              {done && (
                <span aria-hidden="true" className="text-ok">
                  ✓
                </span>
              )}
              {!t.optional && <span className="font-mono text-[11px] text-dim">{t.step}</span>}
              {t.label}
              {t.optional && <span className="text-[11px] text-dim">· optional</span>}
              {done && <span className="sr-only">(done)</span>}
            </>
          );
          return (
            <li key={t.key} className={cx(t.optional && "sm:ml-3 sm:border-l sm:border-line sm:pl-4")}>
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
    </nav>
      <Crumb>
        <CaseSwitcher caseId={caseId} title={title} />
      </Crumb>
      <div className="ml-auto flex flex-wrap items-center gap-3">
        {onAsk && (
          <button type="button" onClick={onAsk} className="inline-flex min-h-8 items-center rounded-[2px] border border-line bg-sheet px-3 py-1 text-dim hover:border-edge hover:text-ink">
            Ask about this case
          </button>
        )}
        <ActionSlot />
      </div>
    </div>
  );
}

// "Step 2 of 4 · Origin" above a screen's heading; the scenario sandbox
// says it is optional instead.
export function StepEyebrow({ tab, children }: { tab: CaseTab; children?: ReactNode }) {
  const t = tabs.find((x) => x.key === tab)!;
  return (
    <p className="eyebrow">
      {t.optional ? "Optional" : `Step ${t.step} of 4`} · {children ?? t.label}
    </p>
  );
}

type Item = { label: ReactNode; href?: string; onClick?: () => void };

// The right-hand column of a case screen: what to do next (the screen's
// <Next> sentence lands here, then links), then optional suggestions.
export function NextSteps({ steps = [], suggestions = [], inline }: { steps?: Item[]; suggestions?: Item[]; inline?: boolean }) {
  const { setNext } = useNextSlot();
  return (
    <aside aria-label="Next steps" className={cx("no-print flex min-w-0 flex-col gap-5", !inline && "lg:sticky lg:top-36 lg:self-start lg:border-l lg:border-line lg:pl-8")}>
      <section>
        <h2 className="eyebrow mb-2 text-ok">Next steps</h2>
        <div ref={setNext} className="duo text-[13px] text-body empty:hidden" />
        {steps.length > 0 && <Items items={steps} arrow />}
      </section>
      {suggestions.length > 0 && (
        <section>
          <h2 className="eyebrow mb-1">Suggestions</h2>
          <Items items={suggestions} />
        </section>
      )}
    </aside>
  );
}

function Items({ items, arrow }: { items: Item[]; arrow?: boolean }) {
  const cls = "flex min-h-9 w-full items-center gap-2 border-b border-line py-1.5 text-left text-[13px] text-ok hover:underline";
  return (
    <ul className="mt-1.5">
      {items.map((it, i) => {
        const inner = (
          <>
            <span aria-hidden="true" className="w-3 text-dim">
              {arrow ? "→" : "·"}
            </span>
            <span className="min-w-0">{it.label}</span>
          </>
        );
        return (
          <li key={i}>
            {it.href ? (
              <Link href={it.href} className={cls}>
                {inner}
              </Link>
            ) : (
              <button type="button" onClick={it.onClick} className={cls}>
                {inner}
              </button>
            )}
          </li>
        );
      })}
    </ul>
  );
}

// Renders the case menu into this page's app bar. Next keeps the previous
// page mounted but hidden after a client navigation, so there is one slot
// per page: the nearest one before these tabs is this page's.
function Crumb({ children }: { children: ReactNode }) {
  const [el, setEl] = useState<HTMLElement | null>(null);
  const find = useCallback((marker: HTMLElement | null) => {
    if (!marker) return;
    const before = [...document.querySelectorAll<HTMLElement>("[data-case-crumb]")].filter(
      (s) => s.compareDocumentPosition(marker) & Node.DOCUMENT_POSITION_FOLLOWING,
    );
    setEl(before.at(-1) ?? null);
  }, []);
  return (
    <>
      <span ref={find} hidden />
      {el ? <Portal to={el}>{children}</Portal> : null}
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
      if (e instanceof KeyboardEvent ? e.key === "Escape" : !ref.current?.contains(e.target as Node)) {
        setOpen(false);
        // Esc returns keyboard focus to the button that opened the menu.
        if (e instanceof KeyboardEvent) ref.current?.querySelector<HTMLButtonElement>("button[aria-expanded]")?.focus();
      }
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
      <button type="button" className="flex items-center gap-1 text-right text-[13px] text-ink hover:underline" aria-expanded={open} onClick={() => setOpen((o) => !o)}>
        <span className="max-w-[200px] truncate">{title}</span> <span aria-hidden="true">▾</span>
      </button>
      {open && (
        <div className="absolute left-0 z-40 mt-2 w-80 max-w-[calc(100vw-32px)] rounded-[2px] border border-edge bg-sheet p-2 text-[13px] shadow-[4px_4px_0_var(--shadow)]">
          <ul>
            {(cases ?? []).filter((c) => c.id !== caseId).slice(0, 6).map((c) => (
              <li key={c.id}>
                <Link href={stepHref(c.id, stepIndex[c.step] ?? 2)} className="block px-2 py-1.5 hover:bg-wash">
                  {c.display_name || c.product_name}
                  {c.pinned ? " · Demo" : ""}
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

// Set inside the tabs row, where a <Next> sentence has no room.
const InBar = createContext(false);

// The screen's buttons: in the tabs row (ACTIONS_ON_TOP) or a fixed footer.
export function Footer({ children }: { children: ReactNode }) {
  const slot = useActionSlot();
  if (ACTIONS_ON_TOP && slot)
    return (
      <Portal to={slot}>
        <InBar.Provider value={true}>{children}</InBar.Provider>
      </Portal>
    );
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

// Pinned demos are read-only for visitors (backend D57). Says so where an
// edit would have been, with the way to a case of one's own.
export function DemoNote({ title, message }: { title?: string | null; message?: string }) {
  const [demoId, setDemoId] = useState<string | null>(null);
  useEffect(() => {
    if (!title) return;
    api.demos().then((r) => setDemoId(r.demos?.find((d) => d.title === title)?.id ?? null), () => {});
  }, [title]);
  return (
    <div role="status" className="flex flex-wrap items-center gap-x-3 gap-y-1 border border-dashed border-guide bg-sheet px-4 py-3 text-[13px]">
      <span className="duo">{message ?? "This is a pinned demo: you can explore it, but changes here aren't saved."}</span>
      <Link href={demoId ? `/cases/new?demo=${encodeURIComponent(demoId)}` : "/cases/new"} className="text-ok underline underline-offset-2">
        Start a new case from this demo →
      </Link>
    </div>
  );
}

export function ErrorBox({ error, onRetry, demoTitle }: { error: unknown; onRetry?: () => void; demoTitle?: string | null }) {
  const e = error instanceof ApiError ? error : null;
  // 403 "forbidden" (D57): an edit to a pinned demo.
  if (e?.status === 403 && (e.code as string) === "forbidden") return <DemoNote title={demoTitle} message={e.message ? e.message[0].toUpperCase() + e.message.slice(1) + "." : undefined} />;
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

// The next course of action in words: at the top of the Next steps column
// when the screen has one, otherwise inline where it is rendered (Step 1).
export function Next({ children }: { children: ReactNode }) {
  const { next } = useNextSlot();
  const inBar = useContext(InBar);
  if (next)
    return (
      <Portal to={next}>
        <p className="mb-1">{children}</p>
      </Portal>
    );
  // The tabs row holds buttons only; without a column the screen's own text
  // already says what the button does.
  if (inBar) return null;
  return (
    <p className="duo min-w-0 text-[13px] text-body">
      <span className="mr-2 text-[11px] font-medium uppercase tracking-[0.08em] text-ok">Next</span>
      {children}
    </p>
  );
}
