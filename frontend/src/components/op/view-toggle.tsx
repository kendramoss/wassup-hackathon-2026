import Link from "next/link";
import { cx } from "./ui";

// Business / technical switch, right of the New case button. It is one
// toggle: a click anywhere on it, either icon, flips to the other view. The
// filled icon shows the view you're on. The technical view is the site as
// built (/); the business view is the story at /business.
export function ViewToggle({ view }: { view: "business" | "technical" }) {
  const business = view === "business";
  const half = (on: boolean) =>
    cx(
      "flex h-8 w-10 items-center justify-center rounded-[6px] transition-colors max-sm:h-10 max-sm:w-11",
      on ? "bg-sheet text-ink shadow-[0_1px_2px_rgba(19,33,58,0.08)]" : "text-dim group-hover:text-ink",
    );
  return (
    <Link
      href={business ? "/" : "/business"}
      aria-label={business ? "Business view on. Switch to technical view" : "Technical view on. Switch to business view"}
      title={business ? "Switch to technical view" : "Switch to business view"}
      className="group flex shrink-0 gap-1 rounded-[10px] bg-[rgba(19,33,58,0.05)] p-1 transition-colors hover:bg-[rgba(19,33,58,0.09)]"
    >
      <span className={half(business)} aria-hidden="true">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
          <path d="M16 20V4a2 2 0 0 0-2-2h-4a2 2 0 0 0-2 2v16" />
          <rect width="20" height="14" x="2" y="6" rx="2" />
        </svg>
      </span>
      <span className={half(!business)} aria-hidden="true">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
          <path d="m18 16 4-4-4-4" />
          <path d="m6 8-4 4 4 4" />
          <path d="m14.5 4-5 16" />
        </svg>
      </span>
    </Link>
  );
}
