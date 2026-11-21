// Which label the "earlier rulings disagree" panel gives a ruling (D180).
// "follows" only when the ruling the model leans on held the origin it
// predicts; a lean to a ruling that held another country is shown neutrally
// as the "closest" earlier ruling, never as the one this case follows.
export function conflictLean(leans: boolean, held: string | null | undefined, predicted: string | null | undefined): "follows" | "closest" | null {
  if (!leans) return null;
  const norm = (x?: string | null) => (x ?? "").trim().toLowerCase();
  return norm(held) !== "" && norm(held) === norm(predicted) ? "follows" : "closest";
}
