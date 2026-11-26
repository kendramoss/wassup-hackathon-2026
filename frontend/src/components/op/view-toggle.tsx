"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { ViewTransition, useEffect, useLayoutEffect, useRef, useState } from "react";
import { cx } from "./ui";

// Business / technical switch, right of the New case button. It is one
// toggle: a click anywhere on it, either icon, flips the thumb. Clicks can
// come as fast as you like; each one flips the thumb again, and the page
// only changes once the clicks settle (SETTLE_MS after the last one) and
// the thumb ends on the other view. Two quick clicks land back where you
// started and nothing loads. With reduced motion it changes at once.
// The page change is a view transition: the page crossfades while the
// toggle and the account tile, named in both bars, stay in place.
// The technical bar's toggle has the bar's sharp corners and offset shadow;
// the business bar's is the soft, rounded one.
const SETTLE_MS = 220;
const hrefFor = (v: "business" | "technical") => (v === "business" ? "/business" : "/");

export function ViewToggle({ view }: { view: "business" | "technical" }) {
  const router = useRouter();
  const business = view === "business";
  const other = business ? "technical" : "business";
  const [target, setTarget] = useState(view);
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined);
  // Once a page change has started, a later click must navigate again even
  // back to this view, so the newest click always wins.
  const pushed = useRef(false);

  useEffect(() => {
    router.prefetch(hrefFor(other));
  }, [router, other]);
  // Next keeps a page you leave alive in the background (Activity) and
  // shows it again with its old state when you come back, which left the
  // thumb on the view you had clicked towards, so the next click only put
  // it back (#867). A layout effect's cleanup runs as the page is hidden,
  // so reset the thumb to this page's view there; it is right when shown.
  useLayoutEffect(
    () => () => {
      clearTimeout(timer.current);
      pushed.current = false;
      setTarget(view);
    },
    [view],
  );

  const onBusiness = target === "business";
  const sharp = !business;
  const icon = (on: boolean) =>
    cx(
      "relative z-[1] flex h-8 w-10 items-center justify-center transition-colors duration-200 ease-[cubic-bezier(0.22,1,0.36,1)] max-sm:h-10 max-sm:w-11",
      sharp ? (on ? "text-paper" : "text-ink") : on ? "text-ink" : "text-dim group-hover:text-ink",
    );

  return (
    <ViewTransition name="view-toggle" share="bar-keep" default="none">
      <Link
        href={hrefFor(other)}
        onClick={(e) => {
          if (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey || e.button !== 0) return;
          if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) return;
          e.preventDefault();
          const next = target === "business" ? "technical" : "business";
          setTarget(next);
          clearTimeout(timer.current);
          timer.current = setTimeout(() => {
            if (next === view && !pushed.current) return;
            pushed.current = true;
            router.push(hrefFor(next));
          }, SETTLE_MS);
        }}
        aria-label={business ? "Business view on. Switch to technical view" : "Technical view on. Switch to business view"}
        title={business ? "Switch to technical view" : "Switch to business view"}
        className={cx(
          "group relative flex shrink-0 touch-manipulation select-none gap-1 p-1 transition-colors",
          sharp
            ? "rounded-[2px] border border-edge bg-sheet shadow-btn hover:border-ink"
            : // Transparent border: same outer size as the technical toggle.
              "rounded-[10px] border border-transparent bg-[rgba(19,33,58,0.05)] hover:bg-[rgba(19,33,58,0.09)]",
        )}
      >
        <span
          aria-hidden="true"
          className={cx(
            "absolute left-1 top-1 h-8 w-10 transition-transform duration-[220ms] ease-[cubic-bezier(0.22,1,0.36,1)] motion-reduce:transition-none max-sm:h-10 max-sm:w-11",
            onBusiness ? "translate-x-0" : "translate-x-11 max-sm:translate-x-12",
            sharp ? "rounded-[2px] bg-ink" : "rounded-[6px] bg-sheet shadow-[0_1px_2px_rgba(19,33,58,0.08),0_0_0_1px_rgba(19,33,58,0.04)]",
          )}
        />
        <span className={icon(onBusiness)} aria-hidden="true">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
            <path d="M16 20V4a2 2 0 0 0-2-2h-4a2 2 0 0 0-2 2v16" />
            <rect width="20" height="14" x="2" y="6" rx="2" />
          </svg>
        </span>
        <span className={icon(!onBusiness)} aria-hidden="true">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
            <path d="m18 16 4-4-4-4" />
            <path d="m6 8-4 4 4 4" />
            <path d="m14.5 4-5 16" />
          </svg>
        </span>
      </Link>
    </ViewTransition>
  );
}
