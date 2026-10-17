import type { CaseResponse, Economics } from "./api/client";

// Live MFN rates (D65): with DUTY_LIVE_MFN on, an HTS line the researched
// table lacks is priced from the USITC schedule's column 1 general rate.
// That is never the full duty, so these rates always carry this label.
export const MFN_ONLY = "MFN only — additional duties not included";

export function pricedEconomics(c: CaseResponse): Economics | null | undefined {
  return (c as CaseResponse & { economics_priced?: Economics | null }).economics_priced ?? c.economics;
}

export function liveMfnOrigins(e: Economics | null | undefined): Set<string> {
  return new Set(e?.duty_rates_live_mfn ?? []);
}

// The backend's note says "… (source, fetched 2026-10-17). …".
export function liveMfnFetched(e: Economics | null | undefined): string | null {
  return /fetched (\d{4}-\d{2}-\d{2})/.exec(e?.duty_rates_note ?? "")?.[1] ?? null;
}
