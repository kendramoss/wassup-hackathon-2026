// Formatting shared by every screen.

// The auto-accept bar for a prediction (D32) and the plan bar for a precedent
// check (D73) are different numbers measured on different tests.
export const BAR = 0.92;
export const PLAN_BAR = 0.85;

const months = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];

// "2026-01-14" → "14 Jan 2026". Empty or invalid input gives "".
export function formatDate(iso: string | null | undefined): string {
  if (!iso) return "";
  const m = /^(\d{4})-(\d{2})-(\d{2})/.exec(iso);
  if (!m) return iso;
  return `${Number(m[3])} ${months[Number(m[2]) - 1]} ${m[1]}`;
}

// "2025-08-27" → "Aug 2025".
export function formatMonth(iso: string | null | undefined): string {
  const m = iso ? /^(\d{4})-(\d{2})/.exec(iso) : null;
  return m ? `${months[Number(m[2]) - 1]} ${m[1]}` : "";
}

// 455000 → "$455k"; 1250000 → "$1.25M"; negatives keep their sign.
export function money(n: number | null | undefined): string {
  if (n === null || n === undefined || Number.isNaN(n)) return "—";
  const sign = n < 0 ? "−" : "";
  const a = Math.abs(n);
  if (a >= 1_000_000) return `${sign}$${trim((a / 1_000_000).toFixed(2))}M`;
  if (a >= 1_000) return `${sign}$${Math.round(a / 1_000)}k`;
  return `${sign}$${Math.round(a)}`;
}

function trim(s: string) {
  return s.replace(/\.?0+$/, "");
}

export function pct(rate: number | null | undefined): string {
  if (rate === null || rate === undefined) return "—";
  return `${trim((rate * 100).toFixed(1))}%`;
}

export function conf(c: number | null | undefined): string {
  return c === null || c === undefined ? "—" : c.toFixed(2);
}

export const routeLabels: Record<string, string> = {
  auto: "Auto-accepted",
  needs_review: "Needs expert review",
  needs_info: "Needs more facts",
};

export function statusLabel(route: string | null | undefined): string {
  return route ? (routeLabels[route] ?? route) : "Draft";
}

export const stepNames = ["Product", "Origin", "Plan", "Draft for broker"] as const;
export const stepIndex: Record<string, number> = { product: 1, origin: 2, plan: 3, export: 4 };

// Short subject from CBP's full subject line (DESIGN.md step 4): the part
// before the first ";", the text after "origin [determination] of|for|on"
// (else after the last "for|on|of a/an/the"), without a leading article or
// count word or a trailing "from {Country}", capitalised, cut at 48
// characters on a word boundary.
export function shortSubject(subject: string | null | undefined): string {
  if (!subject) return "";
  let s = subject.split(";")[0].trim();
  const origin = /origin(?:\s+determination)?\s+(?:of|for|on)\s+(.+)$/i.exec(s);
  if (origin) {
    s = origin[1];
  } else {
    const all = [...s.matchAll(/\b(?:for|on|of)\s+(?:a|an|the)\s+/gi)];
    if (all.length) {
      const last = all[all.length - 1];
      s = s.slice(last.index! + last[0].length);
    }
  }
  s = s.replace(/^(?:a|an|the|certain|one|two|three|four|five|six|seven|eight|nine|ten)\s+/i, "");
  s = s.replace(/\s+from\s+[A-Z][\w .'-]*$/, "").replace(/\.$/, "").trim();
  if (!s) return "";
  s = s[0].toUpperCase() + s.slice(1);
  if (s.length > 48) {
    const cut = s.slice(0, 48);
    s = cut.slice(0, cut.lastIndexOf(" ") > 20 ? cut.lastIndexOf(" ") : 48).replace(/[,\s]+$/, "") + "…";
  }
  return s;
}

// Elapsed seconds as "17 s".
export function seconds(ms: number): string {
  return `${Math.max(0, Math.round(ms / 1000))} s`;
}

export function isProxy(provider: string | null | undefined): boolean {
  return provider === "litellm" || provider === "fake";
}
