"use client";

import Link from "next/link";
import { useStepLabel } from "./demo-tour";
import { ViewTransition, createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from "react";
import { api, ApiError, errorText, isDailyLimit, type CaseSummary } from "@/lib/api/client";
import { copy } from "@/lib/copy";
import { stepIndex } from "@/lib/format";
import { ACTIONS_ON_TOP, ActionSlot, Portal, useActionSlot, useNextSlot } from "./action-bar";
import { LogoMark } from "./logo";
import { isDemoRefusal, isForbidden, isSigninRequired } from "@/lib/roles";
import { SignIn, SignInRequired } from "./sign-in";
import { Button, ButtonLink, Spinner, cx } from "./ui";
import { ViewToggle } from "./view-toggle";

// The active section is passed in rather than read from the URL, so the bar
// prerenders in the static shell of dynamic routes. Cases lives at /cases;
// the landing page (/) passes "none".
export function AppBar({ active = "cases", sticky = false }: { active?: "cases" | "accuracy" | "method" | "api" | "none"; sticky?: boolean }) {
  // The sections read as buttons, not loose text (18 Oct): one segmented
  // group, the current one filled.
  const item = (on: boolean) =>
    // At 320 px the four links plus their padding overflow the viewport, so
    // tighten only that narrowest band (#509). Phones (below 640 px) get
    // 44 px tap heights (#685); wider screens keep 40 px.
    cx("flex min-h-10 items-center rounded-[2px] px-3 transition-colors max-sm:min-h-11 max-[359px]:px-2 lg:px-5", on ? "bg-ink text-paper" : "text-ink hover:bg-wash");
  return (
    // From md up the bar is three columns, so the section buttons sit in the
    // true middle on one row: logo and case breadcrumb left, actions right.
    <header
      className={cx(
        "no-print flex min-h-14 flex-wrap items-center gap-x-5 gap-y-2 max-sm:gap-x-4 border-b border-line bg-paper px-4 py-2 sm:px-10 md:grid md:grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)] md:gap-x-4 lg:gap-x-6",
        sticky && "sticky top-0 z-30",
      )}
    >
      {/* Below sm the logo takes what the actions leave and its name may wrap,
          so the actions never drop to a row of their own and the view toggle
          and account tile sit where the business bar has them (#864). */}
      <div className="flex min-w-0 items-center gap-5 max-sm:flex-1 max-sm:basis-0">
        {/* On the landing page a link to "/" doesn't navigate, so it scrolls
            back to the top instead; elsewhere it goes home as usual. The path
            is read on click, so the bar still prerenders. */}
        <Link
          href="/"
          className="flex shrink-0 items-center gap-2.5 text-[19px] font-semibold tracking-[-0.03em] max-sm:min-h-11 max-sm:shrink max-sm:leading-tight"
          onClick={(e) => {
            if (window.location.pathname !== "/") return;
            e.preventDefault();
            const reduce = matchMedia("(prefers-reduced-motion: reduce)").matches;
            window.scrollTo({ top: 0, behavior: reduce ? "auto" : "smooth" });
          }}
        >
          <LogoMark size={24} />
          <span>
            Unde<i className="not-italic text-brand">Labs</i>
          </span>
        </Link>
        {/* Case screens portal their case menu here, as a breadcrumb (17 Oct). */}
        {/* Hidden below lg, where the page heading names the case (#95). */}
        <div data-case-crumb="" className="hidden min-w-0 items-center border-line empty:hidden lg:flex lg:border-l lg:pl-5" />
      </div>
      {/* Below md (phones) the buttons take their own centred row under the logo. */}
      <nav
        aria-label="Main"
        className="order-3 flex w-full justify-center whitespace-nowrap text-[13px] max-[359px]:text-[12px] md:order-none md:w-auto lg:text-[15px]"
      >
        <div className="flex gap-1 rounded-[2px] border border-edge bg-sheet p-1 shadow-btn max-[359px]:gap-0.5 max-[359px]:p-0.5 sm:gap-1.5">
          <Link href="/cases" aria-current={active === "cases" ? "page" : undefined} className={item(active === "cases")}>
            {copy.nav.cases}
          </Link>
          <Link href="/accuracy" aria-current={active === "accuracy" ? "page" : undefined} className={item(active === "accuracy")}>
            {/* Below lg the short label, so four sections fit beside the logo (390 to 1023 px). */}
            <span className="lg:hidden">{copy.nav.accuracyShort}</span>
            <span className="hidden lg:inline">{copy.nav.accuracy}</span>
          </Link>
          <Link href="/method" aria-current={active === "method" ? "page" : undefined} className={item(active === "method")}>
            {copy.nav.method}
          </Link>
          <Link href="/developers" aria-current={active === "api" ? "page" : undefined} className={item(active === "api")}>
            {copy.nav.api}
          </Link>
        </div>
      </nav>
      {/* "Not a ruling" left the bar on 18 Oct (owner's call); it stays in
          the landing footer, on the accuracy page and on the broker draft. */}
      <div className="ml-auto flex min-w-0 items-center justify-end gap-4 md:ml-0">
        <span className="flex shrink-0 items-center gap-4">
          <ButtonLink href="/cases/new" className="min-h-9 py-1.5 max-sm:min-h-11" aria-label={copy.cases.newCase}>
            <span className="lg:hidden" aria-hidden="true">+ New</span>
            <span className="hidden lg:inline" aria-hidden="true">{copy.cases.newCase}</span>
          </ButtonLink>
          <ViewToggle view="technical" />
          {/* The account square ends the bar: Google's G signed out, your initial signed in. */}
          {/* Named in both bars so it holds still while the page crossfades. */}
          <ViewTransition name="account-tile" share="bar-keep" default="none">
            <SignIn />
          </ViewTransition>
        </span>
      </div>
    </header>
  );
}

export type CaseTab = "product" | "origin" | "scenarios" | "plan" | "draft" | "checks";

// The four steps in order, then the optional scenario sandbox (#82): it sat
// between Origin and Plan and read as a required step.
const tabs: { key: CaseTab; label: string; step: number; path: string; optional?: boolean }[] = [
  { key: "product", label: "Product", step: 1, path: "product" },
  { key: "origin", label: "Origin", step: 2, path: "origin" },
  { key: "plan", label: "Plan", step: 3, path: "plan" },
  { key: "draft", label: "Draft", step: 4, path: "draft" },
  { key: "scenarios", label: "Scenarios", step: 2, path: "scenarios", optional: true },
  // Every check on the same product, each on its own card (#771, D803).
  // Rules checks only, so every market has it.
  { key: "checks", label: "All checks", step: 2, path: "checks", optional: true },
];

export const tabStep: Record<CaseTab, number> = { product: 1, origin: 2, scenarios: 2, plan: 3, draft: 4, checks: 2 };

// The tabs a case shows. Plan, Draft and Scenarios are U.S. steps (CBP
// searches, letters, re-runs), so a case whose market has no follow-ups
// (jurisdiction_detail.follow_ups, #678) hides them; the one being viewed
// stays so a direct link still has its tab.
export function caseTabs(followUps: boolean, current: CaseTab) {
  return tabs.filter((t) => followUps || t.key === "product" || t.key === "origin" || t.key === "checks" || t.key === current);
}

// Case tabs: every case action one click away, each a separate quiet button
// (bordered, never filled) so "+ New case" stays the loudest control. A tick
// marks steps the case has done; tabs a new case can't open yet are inert.
export function CaseTabs({
  caseId,
  current,
  reached,
  title,
  onAsk,
  followUps = true,
}: {
  caseId?: string;
  current: CaseTab;
  reached: number;
  title: ReactNode;
  onAsk?: () => void;
  // false: the case's market has no plan or draft (#678).
  followUps?: boolean;
}) {
  // One sticky row, read left to right: the steps, then the case menu and
  // Ask, then the step's main button (17 Oct).
  return (
    <>
    <div className="no-print z-20 flex min-h-[60px] flex-wrap items-center gap-x-4 gap-y-2 border-b border-line bg-paper px-4 py-2 text-[13px] sm:sticky sm:top-0 sm:px-10">
    {/* The steps wrap onto a second row on a phone instead of scrolling
        sideways, where "4 Draft for broker" was cut off (judge panel round 5). */}
    <nav aria-label="Case" className="w-full sm:w-auto">
      <ul className="flex flex-wrap items-center gap-1.5 whitespace-nowrap">
        {caseTabs(followUps, current).map((t) => {
          const on = t.key === current;
          const open = Boolean(caseId) && (t.step <= Math.max(reached, 2) || t.key === "plan" || t.key === "draft");
          const done = Boolean(caseId) && !on && t.step < 4 && t.step <= reached && !t.optional;
          const cls = cx(
            "inline-flex min-h-8 items-center gap-1.5 rounded-[2px] border px-3 py-1",
            on ? "border-ink bg-white font-medium text-ink" : open ? "border-line bg-sheet text-dim hover:border-edge hover:text-ink" : "cursor-not-allowed border-dashed border-line text-faint",
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
              {!on && !open && <span className="sr-only">(after step 1)</span>}
            </>
          );
          return (
            <li key={t.key} className={cx(t.optional && "sm:ml-3 sm:border-l sm:border-line sm:pl-4")}>
              {on || !open ? (
                <span className={cls} aria-current={on ? "page" : undefined} aria-disabled={!on && !open ? true : undefined} title={!on && !open ? "Opens after step 1" : undefined}>
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
        {!followUps && <li className="px-1 text-dim">{copy.market.followUpsUSOnly}</li>}
      </ul>
    </nav>
      {/* A new case has nothing to switch from yet (#94, #97). */}
      {caseId && (
        <Crumb>
          <CaseSwitcher caseId={caseId} title={title} />
        </Crumb>
      )}
      <div className="ml-auto flex flex-wrap items-center gap-3">
        {onAsk && (
          <button type="button" onClick={onAsk} aria-label="Ask about this case" className="inline-flex min-h-8 items-center rounded-[2px] border border-line bg-sheet px-3 py-1 text-dim hover:border-edge hover:text-ink">
            Ask
          </button>
        )}
        <ActionSlot />
      </div>
    </div>
    {/* A clear gap below the bar, then the top edge of the content column:
        the same dashed blueprint line as its sides (18 Oct). */}
    <div aria-hidden="true" className="no-print column mx-auto mt-6 w-full max-w-[1240px] border-t border-dashed border-guide" />
    </>
  );
}

// "Step 2 of 4 · Origin" above a screen's heading; the scenario sandbox
// says it is optional instead.
export function StepEyebrow({ tab, children, followUps = true }: { tab: CaseTab; children?: ReactNode; followUps?: boolean }) {
  const t = tabs.find((x) => x.key === tab)!;
  const total = followUps ? 4 : 2;
  const label = useStepLabel(tab, t.optional ? "Optional" : `Step ${t.step} of ${total}`);
  return (
    <p className="eyebrow">
      {label} · {children ?? t.label}
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
  // The name is cut short in the bar (#232): the full one is its tooltip and
  // the button's accessible name. title is a node, so read its text.
  const btn = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    const b = btn.current;
    const name = b?.firstElementChild?.textContent?.trim();
    if (!b || !name) return;
    b.title = name;
    b.setAttribute("aria-label", `${name}. Switch case`);
  });
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
    <div
      className="relative min-w-0"
      ref={ref}
      // #539: a menu-button popup must dismiss when focus leaves both the
      // button and its menu, not just on Escape or an outside mouse click.
      onBlur={(e) => {
        if (!e.currentTarget.contains(e.relatedTarget as Node | null)) setOpen(false);
      }}
    >
      <button
        ref={btn}
        type="button"
        className="flex min-h-8 min-w-0 max-w-full items-center gap-1 text-right text-[13px] text-ink hover:underline"
        aria-expanded={open}
        onClick={() => setOpen((o) => !o)}
      >
        <span className="min-w-0 max-w-[200px] truncate xl:max-w-[340px] 2xl:max-w-[460px]">{title}</span> <span aria-hidden="true">▾</span>
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
            <Link href="/cases" className="text-ok">All cases</Link>
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
    // pb-20: the last content sits well clear of the window's bottom edge (18 Oct).
    <main id="main" className={cx("column mx-auto w-full max-w-[1240px] flex-1 px-4 pb-20 pt-8 sm:px-10", className)}>
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
            Too many requests. Try again in{" "}
            {/* #564: role=status is a live region; hide the ticking seconds
                so the message is announced once, not once a second (#548). */}
            <span aria-hidden="true" className="font-mono">
              {left} s
            </span>
            <span className="sr-only">a moment</span>. Nothing is lost.
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
      <span className="duo">{message ?? "Pinned demo: changes here aren't saved."}</span>
      <Link href={demoId ? `/cases/new?demo=${encodeURIComponent(demoId)}` : "/cases/new"} className="text-ok underline underline-offset-2">
        Start a new case from this demo →
      </Link>
    </div>
  );
}

const sentence = (m: string) => (/[.!?]$/.test(m) ? m : m + ".");

// This client's share of the day's model budget, or an API key's daily quota
// (429 until 00:00 UTC, D137): not a rate limit, so no countdown of many
// hours. The backend's own message says what still works.
function DailyLimitNote({ e }: { e: ApiError }) {
  const key = e.code === "quota_exceeded";
  return (
    <div role="status" className="border border-dashed border-guide bg-sheet px-4 py-3 text-[13px]">
      <p className="duo font-medium text-ink">{key ? "Today's request quota for this API key is used up." : "Today's model budget for this network is used up."}</p>
      {e.message ? <p className="duo mt-1">{sentence(errorText(e.message))}</p> : null}
      <p className="duo mt-1 text-dim">It resets after 00:00 UTC. Saved cases and demo cases still open.</p>
      <Link href="/cases" className="mt-1 inline-block text-ok underline underline-offset-2">
        Open a demo case →
      </Link>
    </div>
  );
}

// A long call cut by the hosting's request timeout (a non-JSON 504): the run
// may still finish on the server and be saved, so point at Saved cases.
function TimeoutNote({ message }: { message: string }) {
  return (
    <div role="status" className="border border-dashed border-guide bg-sheet px-4 py-3 text-[13px]">
      <p className="duo">{message}</p>
      <Link href="/cases" className="mt-1 inline-block text-ok underline underline-offset-2">
        Open Saved cases →
      </Link>
    </div>
  );
}

// lead names the action that failed when a screen has more than one (step 1's
// "Turn into parts and plants" beside Find origin).
export function ErrorBox({ error, onRetry, demoTitle, lead }: { error: unknown; onRetry?: () => void; demoTitle?: string | null; lead?: string }) {
  const e = error instanceof ApiError ? error : null;
  // 403 "forbidden": an edit to a pinned demo (D57), or a workspace viewer's
  // write (#283). The server's wording tells them apart; a role refusal is not
  // a demo, so it gets the role message and a re-check (the role may change).
  if (e && isForbidden(e) && !demoTitle && !isDemoRefusal(e))
    return (
      <div role="status" className="border border-ask-line bg-ask-wash px-4 py-3 text-[13px] text-ask">
        <p className="duo">{copy.access.viewerRefused}</p>
        {onRetry && (
          <Button variant="ghost" className="mt-2" onClick={onRetry}>
            {copy.access.recheck}
          </Button>
        )}
      </div>
    );
  if (e && isForbidden(e)) return <DemoNote title={demoTitle} message={e.message ? e.message[0].toUpperCase() + e.message.slice(1) + "." : undefined} />;
  if (e && isDailyLimit(e)) return <DailyLimitNote e={e} />;
  if (e?.code === "timeout") return <TimeoutNote message={e.message} />;
  if (e?.code === "rate_limited") return <RateLimitNote key={String(e.retryAfter)} seconds={Math.ceil(e.retryAfter ?? 30)} onRetry={onRetry} />;
  // 503 "budget_reached" (D59): a calm note, not an error; the demo cases still work.
  if (e?.code === "budget_reached")
    return (
      <div role="status" className="border border-dashed border-guide bg-sheet px-4 py-3 text-[13px]">
        <p className="duo">{e.message ? sentence(errorText(e.message)) : "Today's demo budget for model calls is used up."}</p>
        <Link href="/cases" className="mt-1 inline-block text-ok underline underline-offset-2">
          Open a demo case →
        </Link>
      </div>
    );
  // 401 "unauthenticated": signed out and the server requires sign-in
  // (STRICT_AUTH, #283), or the Google sign-in expired (D68; the proxy cleared
  // it). SignInRequired tells which and never retries on its own.
  if (isSigninRequired(e)) return <SignInRequired onRetry={onRetry} />;
  const outOfScope = e?.code === "out_of_scope";
  const prefix = outOfScope ? "Out of scope: " : e?.code?.startsWith("model") ? "The model didn't answer: " : "";
  const message = errorText(e?.message ?? (error instanceof Error ? error.message : "Something went wrong."), !prefix);
  return (
    <div role="alert" className={cx("rounded-[2px] border px-4 py-3 text-sm", outOfScope ? "border-ask-line bg-ask-wash text-ask" : "border-fail bg-fail-wash text-fail")}>
      <p className="duo">
        {lead ? `${lead} ` : null}
        {prefix}
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

// Shown while a page's client part loads (the Suspense fallback on case
// pages), so a slow first load is never a blank grid (judge journey).
export function PageLoading({ label = "Loading…" }: { label?: string }) {
  return (
    <Main>
      <Spinner label={label} />
    </Main>
  );
}
