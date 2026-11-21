import { notFound } from "next/navigation";

// Tokens are never accepted in a path (D200): a path segment reaches request
// logs. /review/<anything> is a plain 404, with no redirect that could carry
// the segment on, and the segment is never read.

// The page only ever throws notFound(), so dev-mode instant-navigation
// validation can never render it and logged a console error on every visit.
// There is nothing to validate here (instant-navigation guide, "Opting out").
export const instant = false;

export default function Page(): never {
  notFound();
}
