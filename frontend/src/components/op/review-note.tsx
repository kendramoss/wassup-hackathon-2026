import type { CaseResponse } from "@/lib/api/client";
import { copy } from "@/lib/copy";
import { regimeOf } from "@/lib/regime";

// The broker-review and scope note (#278, #282): a few plain lines beside a
// result, once per screen. It is screen-only (no-print) and lives outside the
// broker letter; the letter, its download and its print never include it
// (#265, see screens/draft-step.tsx).
//
// It reads only what the case already shows (the triage reason and the regime
// of the prediction) and nothing from a plan result, so the same words hold
// for any plan, whatever fields the result carries.
export type ReviewPlace = "origin" | "plan" | "compare" | "scenarios" | "draft";

function lines(place: ReviewPlace, c: CaseResponse | undefined, best: boolean): string[] {
  const R = copy.review;
  const J = copy.jurisdiction;
  switch (place) {
    case "origin":
      return [
        R.origin,
        J.originScope,
        ...(/^out of scope/i.test(c?.triage?.reason ?? "") ? [J.outOfScope] : []),
        ...(regimeOf(c)?.key === "usmca" ? [J.part102] : []),
      ];
    case "plan":
      return [R.plan, ...(best ? [R.bestBounded] : []), J.planScope];
    case "compare":
      return [R.compare, R.compareBounded, J.compareScope];
    case "scenarios":
      return [R.scenarios, R.scenariosBounded, J.compareScope];
    case "draft":
      return [J.draftNote];
  }
}

export function ReviewNote({ place, c, best = false }: { place: ReviewPlace; c?: CaseResponse; best?: boolean }) {
  return (
    <div
      role="note"
      aria-label={copy.review.label}
      data-review-note={place}
      className={`no-print duo flex flex-col gap-1.5 border border-line bg-sheet px-4 py-2.5 text-[12.5px] text-dim ${place === "draft" ? "mx-auto mb-4 w-full max-w-[760px]" : place === "origin" ? "mt-5 max-w-[80ch]" : "max-w-[80ch]"}`}
    >
      {lines(place, c, best).map((l) => (
        <p key={l}>{l}</p>
      ))}
    </div>
  );
}
