import type { CaseResponse, Economics } from "./api/client";

// Live MFN rates (D65): with DUTY_LIVE_MFN on, an HTS line the researched
// table lacks is priced from the USITC schedule's column 1 general rate.
// MFN alone is never the full duty: such rates carry this label, unless
// the Section 301 duties were added to them (D90, duty_additional).
export const MFN_ONLY = "MFN only — additional duties not included";

export function pricedEconomics(c: CaseResponse): Economics | null | undefined {
  return (c as CaseResponse & { economics_priced?: Economics | null }).economics_priced ?? c.economics;
}

// Live rates that are MFN alone. With the Section 301 table (D90) a live
// rate can include Chapter 99 duties (duty_additional); those aren't MFN
// only, and DutyBreakdown shows what they add.
export function liveMfnOrigins(e: Economics | null | undefined): Set<string> {
  return new Set((e?.duty_rates_live_mfn ?? []).filter((o) => !e?.duty_additional?.[o]?.length));
}

// The backend's note says "… (source, fetched 2026-10-17). …".
export function liveMfnFetched(e: Economics | null | undefined): string | null {
  return /fetched (\d{4}-\d{2}-\d{2})/.exec(e?.duty_rates_note ?? "")?.[1] ?? null;
}
