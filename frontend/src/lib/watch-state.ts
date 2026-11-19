// How a ruling-watch result reads on screen (#294). status is only the origin
// comparison; basis says whether a difference is the rulings. Only basis
// "precedent" may say the rulings moved or did not. A watch with no basis was
// saved before the legal regime was recorded, so it cannot be read either
// way: it says "Re-check needed" until it is run again (D100). Nothing is
// deleted; the next re-check replaces it.

export type WatchBasis = "precedent" | "regime" | "method" | "specialist_unavailable" | "unverified";
type Route = "auto" | "needs_review" | "needs_info" | null | undefined;

export type WatchView = {
  // changed: the rulings moved. unchanged: they did not.
  // review: the result is not comparable or needs a person; never a ruling claim.
  // recheck: a legacy result with no basis; it makes no claim until re-run.
  kind: "none" | "changed" | "unchanged" | "review" | "recheck";
  label: string; // short, for the cases list
  note: string; // why, for the detail; "" for changed, unchanged and none
  // A person should look: any kind but none and a plain unchanged, plus a
  // genuine change. Screens style these as attention, never as quiet or fine.
  attention: boolean;
  // For a genuine change only: why a person must also look at it ("" when its
  // route is not a review route). The change itself is not demoted.
  reviewNote: string;
};

// What each non-precedent basis means, written for the person reading it.
const WHY: Record<Exclude<WatchBasis, "precedent">, string> = {
  regime: "The re-check was answered under a different legal regime than the original, so a different origin is not a ruling change.",
  method: "The re-check used a different engine version than the original, so a different origin is not a ruling change.",
  specialist_unavailable: "The specialist engine for this case failed, so the re-check is a provisional general answer, not a ruling change.",
  unverified: "The legal regime this case was made under is not confirmed, so the re-check cannot be compared with it.",
};

const LEGACY =
  "This re-check was saved before its legal regime was recorded, so it cannot be read as a ruling change or as unchanged. Re-check to confirm.";

// The triage reason is shown only when it is why a person has to look.
// An approving reason ("confidence 0.97 >= 0.92 and in scope") beside Needs
// review would read as approval, so an auto route's reason is left out.
const reviewReason = (route: Route, reason?: string) => (route === "needs_review" || route === "needs_info" ? (reason ?? "").trim() : "");

export function watchView(
  status: string | null | undefined,
  basis: string | null | undefined,
  route: Route,
  reason?: string,
): WatchView {
  if (!status) return { kind: "none", label: "—", note: "", attention: false, reviewNote: "" };
  if (basis === "precedent") {
    // A real ruling change stays a change even when the result also needs a
    // person; only an unchanged origin is turned into a review.
    if (status === "changed") {
      const why = reviewReason(route, reason);
      const flagged = route === "needs_review" || route === "needs_info";
      return {
        kind: "changed",
        label: "Changed",
        note: "",
        attention: true,
        reviewNote: flagged ? ["It changed, and the re-check also needs a person to look.", why].filter(Boolean).join(" ") : "",
      };
    }
    if (route === "needs_review" || route === "needs_info") {
      return {
        kind: "review",
        label: "Needs review",
        note: ["The origin is the same with today's rulings, but the re-check needs a person to look.", reviewReason(route, reason)].filter(Boolean).join(" "),
        attention: true,
        reviewNote: "",
      };
    }
    return { kind: "unchanged", label: "Unchanged", note: "", attention: false, reviewNote: "" };
  }
  if (basis && basis in WHY) {
    return { kind: "review", label: "Needs review", note: [WHY[basis as keyof typeof WHY], reviewReason(route, reason)].filter(Boolean).join(" "), attention: true, reviewNote: "" };
  }
  return { kind: "recheck", label: "Re-check needed", note: LEGACY, attention: true, reviewNote: "" };
}
