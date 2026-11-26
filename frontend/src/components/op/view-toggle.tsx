"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { cx } from "./ui";

// Business / technical switch, right of the New case button. It is one
// toggle: a click anywhere on it, either icon, flips to the other view. A
// thumb sits under the icon of the view you're on; on a click it slides to
// the other icon (220 ms) and then the page changes, so the new page opens
// with the thumb already on its side. With reduced motion it changes at once.
// The technical bar's toggle has the bar's sharp corners and offset shadow;
// the business bar's is the soft, rounded one.
const SLIDE_MS = 220;

export function ViewToggle({ view }: { view: "business" | "technical" }) {
  const router = useRouter();
  const business = view === "business";
  const href = business ? "/" : "/business";
  const [flipped, setFlipped] = useState(false);
  useEffect(() => {
    router.prefetch(href);
  }, [router, href]);

  const onBusiness = business !== flipped;
  const sharp = !business;
  const icon = (on: boolean) =>
    cx(
      "relative z-[1] flex h-8 w-10 items-center justify-center transition-colors duration-200 ease-[cubic-bezier(0.22,1,0.36,1)] max-sm:h-10 max-sm:w-11",
      sharp ? (on ? "text-paper" : "text-ink") : on ? "text-ink" : "text-dim group-hover:text-ink",
    );

  return (
    <Link
      href={href}
      onClick={(e) => {
        if (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey || e.button !== 0) return;
        if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) return;
        e.preventDefault();
        if (flipped) return;
        setFlipped(true);
        setTimeout(() => router.push(href), SLIDE_MS);
      }}
      aria-label={business ? "Business view on. Switch to technical view" : "Technical view on. Switch to business view"}
      title={business ? "Switch to technical view" : "Switch to business view"}
      className={cx(
        "group relative flex shrink-0 gap-1 p-1 transition-colors",
        sharp
          ? "rounded-[2px] border border-edge bg-sheet shadow-btn hover:border-ink"
          : "rounded-[10px] bg-[rgba(19,33,58,0.05)] hover:bg-[rgba(19,33,58,0.09)]",
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
  );
}
