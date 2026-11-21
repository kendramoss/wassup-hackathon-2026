"use client";

import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { useEffect, useState } from "react";
import { copy } from "@/lib/copy";
import { cx } from "./ui";
import type { CaseTab } from "./chrome";

const T = copy.tour;

// Whether the demo tour is on: the landing's demo button opens the case with
// ?tour=1, and every tour link keeps it.
export function useTour(): boolean {
  return useSearchParams().get("tour") === "1";
}

// Adds ?tour=1 to a case link while the tour is on, so the tour survives a
// click (judge panel round 5).
export function tourHref(href: string, on: boolean): string {
  if (!on) return href;
  const [path, hash] = href.split("#");
  return `${path}${path.includes("?") ? "&" : "?"}tour=1${hash ? `#${hash}` : ""}`;
}

// The page's step in words. Outside the tour: "Step 3 of 4", the case's own
// count. In the tour the case count is hidden, so there is one counter only:
// the plan is tour step 1, the backtest 2, the draft 3.
export function useStepLabel(tab: CaseTab, fallback: string): string {
  const on = useTour();
  if (!on) return fallback;
  if (tab === "plan") return T.stepOf(1);
  if (tab === "draft") return T.stepOf(3);
  return T.outside;
}

// Follows #backtest on the plan page: step 2 is the backtest section of the
// same page, so the tour tracks the hash, and the section coming into view.
function useAtBacktest(tab: CaseTab, on: boolean): boolean {
  const [at, setAt] = useState(false);
  useEffect(() => {
    if (!on || tab !== "plan" || typeof window.location === "undefined") return;
    const read = () => setAt(window.location.hash === "#backtest");
    read();
    window.addEventListener("hashchange", read);
    // Scrolling to the backtest by hand counts too: the section is step 2
    // while it fills the middle of the screen.
    let obs: IntersectionObserver | undefined;
    let tries = 0;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const watch = () => {
      const el = document.getElementById("backtest");
      if (!el || typeof IntersectionObserver === "undefined") {
        if (!el && tries++ < 40) timer = setTimeout(watch, 250); // the plan loads after the shell
        return;
      }
      obs = new IntersectionObserver((es) => {
        for (const e of es) setAt(e.isIntersecting);
      }, { rootMargin: "-35% 0px -35% 0px" });
      obs.observe(el);
    };
    watch();
    return () => {
      window.removeEventListener("hashchange", read);
      obs?.disconnect();
      if (timer) clearTimeout(timer);
    };
  }, [tab, on]);
  return at;
}

// The landing's demo button opens a short tour (judge panel round 4): the
// plan, the backtest against CBP's later ruling, then the broker draft. It
// starts where the product is strongest; the origin step, which opens on an
// honest "needs more facts", stays one click away. Shown only with ?tour=1,
// so a case opened any other way looks as it always did. Round 5: the bar
// follows #backtest, stays on screen while the page scrolls (from 640 px up;
// on a phone it would cover half the screen), and every link
// in it keeps ?tour=1.
export function DemoTour({ id, tab }: { id: string; tab: CaseTab }) {
  const on = useTour();
  const atBacktest = useAtBacktest(tab, on);
  if (!on) return null;
  const base = `/cases/${encodeURIComponent(id)}`;
  const steps = [
    { key: "plan", label: T.steps[0], href: `${base}/plan?tour=1`, on: tab === "plan" && !atBacktest },
    { key: "backtest", label: T.steps[1], href: `${base}/plan?tour=1#backtest`, on: tab === "plan" && atBacktest },
    { key: "draft", label: T.steps[2], href: `${base}/draft?tour=1`, on: tab === "draft" },
  ];
  const at = steps.findIndex((s) => s.on);
  const next = at === -1 ? steps[0] : steps[at + 1];
  return (
    <nav aria-label={T.label} className="no-print z-10 mx-auto w-full max-w-[1240px] px-4 pt-3 sm:sticky sm:top-[60px] sm:px-10">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2 border border-edge bg-sheet px-3 py-2 text-[12.5px]">
        <span className="eyebrow">{T.label}</span>
        <ol className="flex flex-wrap items-center gap-x-3 gap-y-1">
          {steps.map((s, i) => (
            <li key={s.key} className="flex items-center gap-1.5">
              <span aria-hidden="true" className={cx("inline-grid size-4 place-items-center border text-[10px]", s.on ? "border-ink bg-ink text-paper" : "border-line text-dim")}>
                {i + 1}
              </span>
              <Link href={s.href} aria-current={s.on ? "step" : undefined} className={cx("inline-flex min-h-6 items-center underline-offset-2 hover:underline", s.on ? "text-ink" : "text-ok")}>
                {s.label}
              </Link>
            </li>
          ))}
        </ol>
        <span className="flex flex-wrap items-center gap-x-3 sm:ml-auto">
          {next ? (
            <Link href={next.href} className="inline-flex min-h-6 items-center text-ok underline underline-offset-2">
              {T.next(next.label)}
            </Link>
          ) : null}
          {tab !== "origin" && (
            <Link href={`${base}/origin?tour=1`} className="inline-flex min-h-6 items-center text-dim underline underline-offset-2">
              {T.origin}
            </Link>
          )}
        </span>
      </div>
    </nav>
  );
}
