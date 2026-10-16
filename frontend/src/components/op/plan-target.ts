import type { CaseResponse } from "@/lib/api/client";

// The default plan target: the latest plan's target, else the first plant
// country that isn't the predicted origin.
export function planTarget(c: CaseResponse): string | undefined {
  if (c.plan?.target) return c.plan.target;
  const origin = c.prediction?.origin_country;
  return c.product?.plants?.map((p) => p.country).find((country) => country && country !== origin);
}
