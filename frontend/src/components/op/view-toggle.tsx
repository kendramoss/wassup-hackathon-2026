import Link from "next/link";
import { cx } from "./ui";

// Business / technical switch, right of the New case button. The technical
// view is the site as built (/); the business view is the story at /business.
export function ViewToggle({ view }: { view: "business" | "technical" }) {
  const item = (on: boolean) =>
    cx(
      "flex h-8 w-10 items-center justify-center rounded-[6px] transition-colors max-sm:h-10 max-sm:w-11",
      on ? "bg-sheet text-ink shadow-[0_1px_2px_rgba(19,33,58,0.08)]" : "text-dim hover:text-ink",
    );
  return (
    <div role="group" aria-label="View" className="flex shrink-0 gap-1 rounded-[10px] bg-[rgba(19,33,58,0.05)] p-1">
      <Link href="/business" aria-label="Business view" title="Business view" aria-current={view === "business" ? "page" : undefined} className={item(view === "business")}>
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
          <path d="M16 20V4a2 2 0 0 0-2-2h-4a2 2 0 0 0-2 2v16" />
          <rect width="20" height="14" x="2" y="6" rx="2" />
        </svg>
      </Link>
      <Link href="/" aria-label="Technical view" title="Technical view" aria-current={view === "technical" ? "page" : undefined} className={item(view === "technical")}>
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
          <path d="m18 16 4-4-4-4" />
          <path d="m6 8-4 4 4 4" />
          <path d="m14.5 4-5 16" />
        </svg>
      </Link>
    </div>
  );
}
