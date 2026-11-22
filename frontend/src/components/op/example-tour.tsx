"use client";

// The tour's links on a recorded example page (judge panel round 8, N11 and
// round 10, item 2): opened from the heat-gun tour (?tour=1&from=<case>), the
// page links back to the tour's last step and on to the next recorded
// example. The search params are read only here, inside Suspense, so the
// rest of the page prerenders.
import Link from "next/link";
import { Suspense } from "react";
import { useSearchParams } from "next/navigation";
import { copy } from "@/lib/copy";

const T = copy.tour;

export function ExampleTour({ at }: { at: (typeof T.examples)[number]["href"] }) {
  return (
    <Suspense fallback={null}>
      <Links at={at} />
    </Suspense>
  );
}

function Links({ at }: { at: string }) {
  const q = useSearchParams();
  const from = q.get("tour") === "1" ? (q.get("from") ?? "") : "";
  if (!/^[A-Za-z0-9_-]{1,64}$/.test(from)) return null;
  const i = T.examples.findIndex((e) => e.href === at);
  const next = i >= 0 ? T.examples[i + 1] : undefined;
  return (
    <p className="duo mt-2 flex flex-wrap gap-x-4 gap-y-1 text-[13.5px]">
      <Link href={`/cases/${from}/draft?tour=1`} className="inline-flex min-h-6 items-center text-ok underline underline-offset-2">
        {copy.ruleCheckExample.tourBack}
      </Link>
      {next ? (
        <Link href={`${next.href}?tour=1&from=${encodeURIComponent(from)}`} className="inline-flex min-h-6 items-center text-ok underline underline-offset-2">
          {T.nextExample(next.label)}
        </Link>
      ) : null}
    </p>
  );
}
