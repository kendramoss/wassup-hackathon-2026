"use client";

// Grey outlines shaped like the content that is loading (#931; canvas mock
// "#931 · skeletons"), so nothing jumps when it arrives. The labels stay
// real; only the values are grey. They pulse only when reduced motion is
// off, and after 10 s say the service may be busy.
import { useEffect, useState } from "react";
import { cx } from "./ui";

const block = "block bg-[#dfe7f2] motion-safe:animate-pulse";

function useSlow(ms = 10_000) {
  const [slow, setSlow] = useState(false);
  useEffect(() => {
    const t = setTimeout(() => setSlow(true), ms);
    return () => clearTimeout(t);
  }, [ms]);
  return slow;
}

function Status({ label }: { label: string }) {
  const slow = useSlow();
  return (
    <p role="status" className={cx("text-[13px] text-dim", slow ? "mt-4" : "sr-only")}>
      {slow ? "Still loading. The service may be busy." : label}
    </p>
  );
}

// The demo cards on /cases: same grid as the real list.
export function CasesSkeleton() {
  const widths = ["70%", "55%", "80%", "62%", "74%", "50%", "66%", "58%"];
  return (
    <section className="mt-8" aria-busy="true" aria-label="Cases, loading">
      <span className={cx(block, "mb-3 h-3 w-24")} />
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4" aria-hidden="true">
        {widths.map((w, i) => (
          <div key={i} className="flex h-[168px] flex-col gap-3 border border-edge bg-sheet p-4 shadow-btn">
            <span className={cx(block, "h-3.5")} style={{ width: w }} />
            <span className={cx(block, "h-2.5 w-2/5")} />
            <span className="mt-auto flex gap-2">
              <span className={cx(block, "h-[22px] w-[72px]")} />
              <span className={cx(block, "h-[22px] w-[52px]")} />
            </span>
          </div>
        ))}
      </div>
      <Status label="Loading cases…" />
    </section>
  );
}

// A case page (origin and the other tabs) while the case loads.
export function CaseSkeleton() {
  return (
    <div className="flex max-w-[1020px] flex-col gap-5" aria-busy="true" aria-label="The case, loading">
      <div className="flex flex-col gap-4 border border-edge bg-sheet p-6 shadow-btn" aria-hidden="true">
        <span className="text-[12px] text-dim">Likely origin</span>
        <span className="block h-11 w-60 shadow-[inset_0_0_0_2px_#dfe7f2]" />
        <span className="flex items-center gap-3">
          <span className={cx(block, "h-3 w-36")} />
          <span className={cx(block, "h-3 w-20")} />
        </span>
        <span className="flex flex-col gap-2.5 border-t border-line pt-3">
          <span className={cx(block, "h-2.5 w-full")} />
          <span className={cx(block, "h-2.5 w-[92%]")} />
          <span className={cx(block, "h-2.5 w-[70%]")} />
        </span>
      </div>
      <div className="flex flex-col gap-3 border border-edge bg-sheet px-6 py-5 shadow-btn" aria-hidden="true">
        <span className="text-[12px] text-dim">Past decisions used</span>
        {["80%", "64%", "72%"].map((w) => (
          <span key={w} className="grid grid-cols-[90px_1fr_60px] items-center gap-3">
            <span className={cx(block, "h-3")} />
            <span className={cx(block, "h-3")} style={{ width: w }} />
            <span className={cx(block, "h-3")} />
          </span>
        ))}
      </div>
      <Status label="Loading the case…" />
    </div>
  );
}
