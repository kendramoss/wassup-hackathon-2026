// Markets a case can be for (D680): the backend's jurisdiction codes, in its
// order. `flag` is the name Flag() knows; `short` is how a result names it.
// Which are supported comes from GET /v1/jurisdictions; until it answers,
// only the United States is.
import type { components } from "./api/schema";

export type MarketCode = components["schemas"]["JurisdictionCode"];

// `phrase` is the name inside a sentence ("Origin rules for the European Union").
export const markets: { code: MarketCode; name: string; short: string; phrase: string; flag?: string }[] = [
  { code: "us", name: "United States", phrase: "the United States", short: "U.S.", flag: "United States" },
  { code: "eu", name: "European Union", phrase: "the European Union", short: "EU", flag: "European Union" },
  { code: "uk", name: "United Kingdom", phrase: "the United Kingdom", short: "UK", flag: "United Kingdom" },
  { code: "ca", name: "Canada", phrase: "Canada", short: "Canada", flag: "Canada" },
  { code: "jp", name: "Japan", phrase: "Japan", short: "Japan", flag: "Japan" },
  { code: "kr", name: "South Korea", phrase: "South Korea", short: "South Korea", flag: "South Korea" },
  { code: "au", name: "Australia", phrase: "Australia", short: "Australia", flag: "Australia" },
  { code: "mx", name: "Mexico", phrase: "Mexico", short: "Mexico", flag: "Mexico" },
  { code: "other", name: "Other market", phrase: "this market", short: "Other market" },
];

export function marketOf(code: string | null | undefined) {
  return markets.find((m) => m.code === code) ?? markets[0];
}

export const defaultSupported: MarketCode[] = ["us"];

// Whether a case has the steps after the origin answer (Plan, Draft): the
// API's jurisdiction_detail.follow_ups, true for U.S. cases only (#678). A
// case read before the field existed falls back to whether it is supported.
export function hasFollowUps(c: { jurisdiction_detail?: { supported?: boolean; follow_ups?: boolean } }) {
  const d = c.jurisdiction_detail;
  return d?.follow_ups ?? d?.supported !== false;
}
