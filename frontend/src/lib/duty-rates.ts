import type { Economics } from "./api/client";

// Duty-rate read/edit by country IDENTITY, the same identity the backend prices
// by (Go domain.DutyCountry, #718). The backend fills a derived rate under the
// canonical name ("Taiwan") while the money inputs are keyed by the plant's
// own spelling ("Chinese Taipei"); matching by raw key would show the input as
// blank, and a rate typed there would sit next to the derived one and lose to it.
//
// This is a port of domain.NormalizeCountry + DutyCountry, bounded to the alias
// tables those files hold, not a world country list. backend/internal/domain/
// testdata/duty_country.json is the shared contract: Go and scripts/
// duty-rates.test.mjs both check every case in it, and the Go test fails when
// an alias is added without a fixture case.

const ALIASES: Record<string, string> = {
  usa: "United States", us: "United States", "u.s.": "United States", "united states of america": "United States",
  "us virgin islands": "US Virgin Islands", "united states virgin islands": "US Virgin Islands",
  prc: "China", "people's republic of china": "China", "mainland china": "China",
  "viet nam": "Vietnam", "republic of korea": "South Korea", korea: "South Korea", "s. korea": "South Korea",
  "taiwan (roc)": "Taiwan", uk: "United Kingdom", "great britain": "United Kingdom", england: "United Kingdom",
  "the netherlands": "Netherlands", holland: "Netherlands", czechia: "Czech Republic", turkiye: "Turkey",
  "türkiye": "Turkey", "hong kong sar": "Hong Kong",
  thai: "Thailand", burma: "Myanmar",
};

// Duty-only spellings, keyed by the lower-cased normalised name.
const DUTY_ALIASES: Record<string, string> = {
  dprk: "North Korea",
  "democratic people's republic of korea": "North Korea",
  "korea north": "North Korea",
  "republic of cuba": "Cuba",
  byelorussia: "Belarus",
  uae: "United Arab Emirates",
  "chinese taipei": "Taiwan",
  "taiwan china": "Taiwan",
  "korea south": "South Korea",
};

const ALT_SPLIT = /\s*(?:\/|\||\band\/or\b|\bor\b)\s*/;

function titleCase(s: string): string {
  let prevLetter = false;
  let out = "";
  for (const ch of s) {
    const isLetter = ch >= "a" && ch <= "z";
    out += isLetter && !prevLetter ? ch.toUpperCase() : ch;
    prevLetter = isLetter;
  }
  return out;
}

function normalizeOne(s: string): string {
  let c = s.trim().toLowerCase();
  if (c === "") return "";
  if (c.includes("ü") && ALIASES[c]) return ALIASES[c];
  c = c.replace(/[^a-z .'()]/g, "");
  c = c.replaceAll(".", "").trim();
  if (c.startsWith("the ")) c = c.slice(4);
  if (ALIASES[c]) return ALIASES[c];
  return titleCase(c).replaceAll("Of", "of");
}

function normalizeCountry(s: string): string {
  const parts = s.trim().split(ALT_SPLIT);
  if (parts.length > 1) return parts.map(normalizeOne).filter(Boolean).join("/");
  return normalizeOne(s);
}

/** The country's identity for duty pricing; equal for spellings the backend prices alike. */
export function dutyCountry(s: string): string {
  const n = normalizeCountry(s);
  if (n === "") return "";
  return n
    .split("/")
    .map((p) => DUTY_ALIASES[p.toLowerCase()] ?? p)
    .join("/");
}

type Rates = Record<string, number> | undefined;

function equivalentKeys(rates: Rates, country: string): string[] {
  const want = dutyCountry(country);
  return Object.keys(rates ?? {}).filter((k) => dutyCountry(k) === want);
}

// The key the backend uses when several keys name one country (Economics.DutyRate):
// the exact canonical name, else the first in sorted order. Fallback for
// duplicates that arrive from the API; edits made here never create them.
function usedKey(keys: string[], country: string): string | undefined {
  const canon = dutyCountry(country);
  return keys.includes(canon) ? canon : [...keys].sort()[0];
}

/** The rate shown for a country: the one the backend will use, whatever spelling it is keyed by. */
export function dutyRateFor(rates: Rates, country: string): number | undefined {
  const keys = equivalentKeys(rates, country);
  const k = usedKey(keys, country);
  return k === undefined ? undefined : rates![k];
}

/**
 * Set when the rates hold several spellings of this country with different
 * values: which key is priced and which values it shadows. Same values do not conflict.
 */
export function dutyRateConflict(rates: Rates, country: string): { used: string; usedRate: number; shadowed: { key: string; rate: number }[] } | undefined {
  const keys = equivalentKeys(rates, country);
  if (keys.length < 2) return undefined;
  const used = usedKey(keys, country)!;
  const usedRate = rates![used];
  const shadowed = keys.filter((k) => k !== used && rates![k] !== usedRate).map((key) => ({ key, rate: rates![key] }));
  return shadowed.length ? { used, usedRate, shadowed } : undefined;
}

/** One line for the money forms when dutyRateConflict is set. */
export function dutyConflictNote(c: NonNullable<ReturnType<typeof dutyRateConflict>>): string {
  const other = c.shadowed.map((s) => `${s.key} ${Math.round(s.rate * 1000) / 10}%`).join(", ");
  return `Saved under more than one spelling: ${c.used} ${Math.round(c.usedRate * 1000) / 10}% is used, not ${other}. Editing the rate keeps one.`;
}

const without = <T,>(m: Record<string, T> | undefined, drop: (k: string) => boolean): Record<string, T> | undefined => {
  if (!m) return m;
  const out = Object.fromEntries(Object.entries(m).filter(([k]) => !drop(k)));
  return Object.keys(out).length === Object.keys(m).length ? m : out;
};

/**
 * The economics after the user sets (or clears, with undefined) the rate for a
 * country. Every spelling of that country collapses to one entry under the
 * spelling the user's input is keyed by; the auto-fill provenance for that
 * country (table, live MFN, additional duties) is dropped because the rate is
 * now the user's. Other origins, notes and fields are kept; the input is not mutated.
 */
export function setDutyRate(e: Economics, country: string, rate: number | undefined): Economics {
  const same = (k: string) => dutyCountry(k) === dutyCountry(country);
  const rates = { ...(without(e.duty_rates, same) ?? {}) };
  if (rate !== undefined) rates[country] = rate;
  const out: Economics = { ...e, duty_rates: rates };
  for (const k of ["duty_rates_from_table", "duty_rates_live_mfn"] as const) {
    if (!e[k]) continue;
    const kept = e[k]!.filter((o) => !same(o));
    if (kept.length === e[k]!.length) continue;
    if (kept.length) out[k] = kept;
    else delete out[k];
  }
  if (e.duty_additional) {
    const kept = without(e.duty_additional, same);
    if (kept !== e.duty_additional) {
      if (kept && Object.keys(kept).length) out.duty_additional = kept;
      else delete out.duty_additional;
    }
  }
  return out;
}
