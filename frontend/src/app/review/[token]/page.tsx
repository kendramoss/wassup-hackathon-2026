import { notFound } from "next/navigation";

// Tokens are never accepted in a path (D200): a path segment reaches request
// logs. /review/<anything> is a plain 404, with no redirect that could carry
// the segment on, and the segment is never read.
export default function Page(): never {
  notFound();
}
