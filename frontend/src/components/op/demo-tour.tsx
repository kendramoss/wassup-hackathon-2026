"use client";

import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { copy } from "@/lib/copy";
import { cx } from "./ui";
import type { CaseTab } from "./chrome";

const T = copy.tour;

// The landing's demo button opens a short tour (judge panel round 4): the
// plan, the backtest against CBP's later ruling, then the broker draft. It
// starts where the product is strongest; the origin step, which opens on an
// honest "needs more facts", stays one click away. Shown only with ?tour=1,
// so a case opened any other way looks as it always did.
export function DemoTour({ id, tab }: { id: string; tab: CaseTab }) {
  const params = useSearchParams();
  if (params.get("tour") !== "1") return null;
  const base = `/cases/${encodeURIComponent(id)}`;
  const steps = [
    { key: "plan", label: T.steps[0], href: `${base}/plan?tour=1`, on: tab === "plan" },
    { key: "backtest", label: T.steps[1], href: `${base}/plan?tour=1#backtest`, on: false },
    { key: "draft", label: T.steps[2], href: `${base}/draft?tour=1`, on: tab === "draft" },
  ];
  const next = tab === "plan" ? steps[1] : tab === "draft" ? null : steps[0];
  return (
    <nav aria-label={T.label} className="mx-auto w-full max-w-[1240px] px-4 pt-3 sm:px-10">
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
          {next && next.key !== steps.find((s) => s.on)?.key ? (
            <Link href={next.href} className="inline-flex min-h-6 items-center text-ok underline underline-offset-2">
              {T.next(next.label)}
            </Link>
          ) : null}
          {tab !== "origin" && (
            <Link href={`${base}/origin`} className="inline-flex min-h-6 items-center text-dim underline underline-offset-2">
              {T.origin}
            </Link>
          )}
        </span>
      </div>
    </nav>
  );
}
