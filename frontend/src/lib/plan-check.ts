// One verdict per move (D220). A scenario that moves the same groups to the
// same plants as a plan row is the same move, so Scenarios shows the plan
// search's precedent check with the words Plan uses, and never calls an
// unchecked or below-bar move "auto-accepted" or the best. The backend
// attaches the matching row to each scenario as plan_check.
import type { Scenario } from "./api/client";
import { conf, originConf } from "./format";

export type PlanCheck = NonNullable<Scenario["plan_check"]>;

export function planCheckOf(s: Scenario | null | undefined): PlanCheck | null {
  return s?.plan_check ?? null;
}

// Plan's outcome chip text for a row: "Plan" when shown, else the same
// rejection label the Plan table uses (plan-bits rejectionLabels).
export const outcomeLabels: Record<string, string> = {
  current_setup: "Current setup",
  below_confidence_bar: "Below the bar",
  no_supporting_precedent: "No precedent",
  engine_other_origin: "Other origin",
  check_failed: "Check failed",
  operationally_infeasible: "Not feasible",
  net_costlier: "Net costlier",
};

export type CheckWords = { chip: string; tone: "ok" | "ask" | undefined; line: string; supported: boolean };

// What the precedent check said about a move, in Plan's words.
export function planCheckWords(pc: PlanCheck): CheckWords {
  const c = pc.confidence;
  if (pc.shown) {
    return { chip: "Plan", tone: "ok", supported: true, line: `Precedent-supported: check ${conf(c)}, at or above the ${conf(pc.bar)} precedent-check bar` };
  }
  const kind = pc.rejection_kind ?? "";
  if (kind === "below_confidence_bar") {
    return { chip: `△ ${outcomeLabels[kind]}`, tone: "ask", supported: false, line: `Not precedent-supported (${conf(c)} < ${conf(pc.bar)})` };
  }
  const label = outcomeLabels[kind] ?? (kind || "Not shown");
  const reason = pc.rejection_reason?.trim();
  return { chip: label, tone: undefined, supported: false, line: `Not precedent-supported${reason ? `: ${reason}` : ""}` };
}

// The scenario's own route is the engine's origin answer, not a plan
// approval, so a move never shows "Auto-accepted" (judge panel round 2).
export function engineLabel(s: Scenario): string {
  const p = s.prediction ?? {};
  const o = originConf(p);
  return `Engine: ${p.origin_country ?? "—"}, ${o.calibrated ? "calibrated " : ""}${conf(o.value)}`;
}

// Which scenario earns the "highest net" chip. A move the plan search tried
// and did not show can't be the best, whatever its net; a move it never tried
// can, but it is captioned as not precedent-checked. Ties earn nothing (#89).
export function highestNet(sc: Scenario[]): { index: number; checked: boolean } | null {
  const eligible = sc.map((s, i) => ({ s, i, net: s.comparison?.net_usd })).filter(
    (x) => typeof x.net === "number" && x.net > 0 && (!x.s.plan_check || x.s.plan_check.shown),
  );
  if (eligible.length === 0) return null;
  const best = Math.max(...eligible.map((x) => x.net as number));
  const top = eligible.filter((x) => x.net === best);
  if (top.length !== 1) return null;
  return { index: top[0].i, checked: Boolean(top[0].s.plan_check?.shown) };
}

// Why a what-if on today's setup can differ from the Origin step (D220): the
// Origin step reads the text pasted into the case (here CBP's own facts);
// scenarios and plan moves read the structured parts list. Null when both
// read the same thing or agree.
export function readsDiffer(
  c: { facts_source?: string | null; prediction?: { origin_country?: string | null; confidence?: number | null; calibrated_confidence?: number | null } | null },
  s: Scenario,
): string | null {
  if (c.facts_source !== "pasted" || !s.product) return null;
  const a = c.prediction?.origin_country, b = s.prediction?.origin_country;
  if (!a || !b || a.toLowerCase() === b.toLowerCase()) return null;
  const oa = originConf(c.prediction), ob = originConf(s.prediction);
  const unsure = (oa.value ?? 0) < oa.bar && (ob.value ?? 0) < ob.bar;
  return `Two inputs, two answers: the Origin step reads the text pasted into the case (CBP's own facts) and says ${a}; scenarios and plan moves read the structured parts list for the same setup and say ${b}.${
    unsure ? " Both are under the auto-accept bar, so the model is unsure about this setup either way." : ""
  }`;
}

// The case's "As today" what-if: a structured scenario that moves nothing.
export function asToday(sc: Scenario[] | null | undefined): Scenario | undefined {
  return (sc ?? []).find((s) => s.product && (s.comparison ? (s.comparison.moved?.length ?? 0) === 0 : false));
}
