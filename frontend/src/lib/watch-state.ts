// How a ruling-watch result reads on screen (#294). status is only the origin
// comparison; basis says whether a difference is the rulings. Only basis
// "precedent" may say the rulings moved. A watch saved before basis existed
// (no basis) keeps its old reading.

export type WatchBasis = "precedent" | "regime" | "method" | "specialist_unavailable" | "unverified";
type Route = "auto" | "needs_review" | "needs_info" | null | undefined;

export type WatchView = {
  // changed: the rulings moved (or a legacy changed). unchanged: they did not.
  // review: the result is not comparable or needs a person; never a ruling claim.
  kind: "none" | "changed" | "unchanged" | "review";
  label: string; // short, for the cases list
  note: string; // why, for the detail; "" unless review
};

const WHY: Record<Exclude<WatchBasis, "precedent">, string> = {
  regime: "The re-check was answered under a different legal regime than the original, so a different origin is not a ruling change.",
  method: "The re-check used a different engine version than the original, so a different origin is not a ruling change.",
  specialist_unavailable: "The specialist engine for this case failed, so the re-check is a provisional general answer, not a ruling change.",
  unverified: "The legal regime this case was made under is not confirmed, so the re-check cannot be compared with it.",
};

export function watchView(
  status: string | null | undefined,
  basis: string | null | undefined,
  route: Route,
  reason?: string,
): WatchView {
  if (!status) return { kind: "none", label: "—", note: "" };
  const known = basis && basis in WHY ? (basis as Exclude<WatchBasis, "precedent">) : null;
  if (known) {
    return { kind: "review", label: "Needs review", note: [WHY[known], reason].filter(Boolean).join(" ") };
  }
  if (status === "changed") return { kind: "changed", label: "Changed", note: "" };
  // Unchanged origin, but the watch result itself is flagged for a person.
  if (basis === "precedent" && (route === "needs_review" || route === "needs_info")) {
    return { kind: "review", label: "Needs review", note: ["The origin is the same with today's rulings, but the re-check needs a person to look.", reason].filter(Boolean).join(" ") };
  }
  return { kind: "unchanged", label: "Unchanged", note: "" };
}
