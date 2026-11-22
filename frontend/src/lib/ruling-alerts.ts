// Revocation alerts (D480): CBP changed, or proposed to change, a ruling a
// saved case relies on. The backend works them out at read time
// (ruling_alerts on the case, a count on each cases-list row); this file only
// words them. A saved answer is never changed by an alert.

export type RulingAlertItem = {
  ruling: string;
  roles: string[];
  status: "final" | "proposed";
  kind: "revoked" | "modified";
  by: string;
  date?: string;
  in_effect?: boolean;
  before_case?: boolean;
  bulletin?: string;
  url?: string;
  note: string;
  // D531, D561: whether the change bears on what the case relies on the
  // ruling for (the answer, a shown plan's target, the current setup's CBP
  // holding; relevance_role names which), from the origins the changed and
  // changing rulings hold (shipped labels).
  held_before?: string;
  held_after?: string;
  answer?: string;
  relevance?: "touches_answer" | "not_answer" | "holding_unchanged" | "unknown";
  relevance_note?: string;
  relevance_role?: "cited" | "plan_support" | "current_setup" | "read";
};

export type RulingAlertCounts = { final: number; proposed: number; read_only?: number; touches_answer?: number };

export type RulingAlerts = { counts: RulingAlertCounts; items: RulingAlertItem[]; rechecked_at?: string };

export const relied = (a: RulingAlertItem) => a.roles.some((r) => r !== "read");

// The role words for a sentence: "cited by this answer", "supporting a plan".
export function roleWords(roles: string[]): string {
  const w: string[] = [];
  if (roles.includes("cited")) w.push("cited by this answer");
  if (roles.includes("plan_support")) w.push("supporting a plan");
  if (roles.includes("current_setup")) w.push("given as CBP's holding on the current setup");
  if (!w.length) w.push("read by this answer");
  return w.length === 1 ? w[0] : `${w.slice(0, -1).join(", ")} and ${w[w.length - 1]}`;
}

// How the change reads after the ruling's number: "was revoked by H346161 on
// 5 May 2025", "will be modified by H314991 effective 29 Nov 2026".
export function changeWords(a: RulingAlertItem, fmt: (iso: string) => string): string {
  if (a.status === "proposed") return `has a proposed change: CBP proposed to ${a.kind === "revoked" ? "revoke" : "modify"} it in ${a.by}`;
  const when = a.note.includes(" effective ") ? "effective" : "on";
  const verb = a.in_effect ? `was ${a.kind}` : `will be ${a.kind}`;
  return `${verb} by ${a.by} ${when} ${fmt(a.date ?? "")}`;
}

// The closing words of a final change's line: a re-check is recommended
// either way; "still" when the change does not move the answer's country.
export function recheckWords(a: RulingAlertItem): string {
  return a.relevance === "not_answer" || a.relevance === "holding_unchanged" ? "Re-check still recommended." : "Re-check recommended.";
}

// The case page's tone: attention when a relied-on ruling has a final
// change; calm when there are only proposals or rulings read and not cited.
export function alertTone(ra: RulingAlerts | null | undefined): "attention" | "calm" | null {
  if (!ra?.items?.length) return null;
  return ra.items.some((a) => relied(a) && a.status === "final") ? "attention" : "calm";
}

// The cases-list badge, e.g. "1 cited ruling revoked or modified". touches_answer
// counts changes that bear on what the case relies on the ruling for: the
// answer, a shown plan's target or the current setup's holding (D561).
export function badgeText(n: RulingAlertCounts | null | undefined): { text: string; attention: boolean } | null {
  if (!n || (!n.final && !n.proposed)) return null;
  if (n.final) {
    const base = `${n.final} cited ruling${n.final === 1 ? "" : "s"} revoked or modified`;
    return { text: n.touches_answer ? `${base}, ${n.touches_answer} bearing on the answer or a plan` : base, attention: true };
  }
  return { text: `${n.proposed} cited ruling${n.proposed === 1 ? "" : "s"}: change proposed`, attention: false };
}
