// How an operations-mode result reads (#281), as plain data the screens
// render. Legacy results (no `operations` block) are never touched here: the
// callers keep their existing formatting for them.
//
// Rules this file holds:
//  - null is never shown as $0: it is "unknown" (a financial input is
//    missing) or "not modelled" (the factor was never entered).
//  - net_priced is read before net_per_year; when it is false the legacy
//    figure is a ranking aid, not an amount to show.
//  - a net of exactly 0 reads "no modelled saving".
import type { Comparison, Plan } from "@/lib/api/client";
import type { components } from "@/lib/api/schema";
import { money } from "@/lib/format";
import { groupName } from "./plan-bits";

export type PlanOperations = components["schemas"]["PlanOperations"];
type Unknown = PlanOperations["unknown"][number];
type Check = PlanOperations["checks"][number];

export const NO_SAVING = "No modelled saving";
export const UNKNOWN = "Unknown";
export const NOT_MODELLED = "Not modelled";

// Operations-mode money only (the shared formatter is unchanged): a real,
// non-zero amount under 50 cents is shown in cents, never as $0 or -$0.
export function opsMoney(n: number | null | undefined): string {
  if (typeof n === "number" && n !== 0 && Math.abs(n) < 0.5) return `${n < 0 ? "−" : ""}$${Math.abs(n).toFixed(2)}`;
  return money(n);
}

const human = (s: string) => s.replace(/_/g, " ");
const g = (id: string) => groupName(id);

const FACTORS: Record<string, string> = {
  duty: "duty",
  group_cost: "group cost",
  labour: "labour",
  logistics: "logistics",
  capex: "CapEx",
  retooling: "retooling",
  operating: "operating cost",
  horizon: "horizon",
  one_time: "one-time costs",
  amortized_one_time: "amortized one-time costs",
  net: "net",
  lead_time: "lead time",
  moq: "minimum order quantity",
};

const REASONS: Record<string, string> = {
  missing_baseline: "not entered for the current plant",
  missing_target: "not entered for the target plant",
  no_duty_rate: "no duty rate for an origin",
  no_import_value: "no annual import value",
  ambiguous_baseline: "the group is made at more than one plant today",
  missing_horizon: "no horizon entered",
  missing_group_cost: "no group cost entered",
  cost_basis_missing: "cost basis not set",
  arithmetic_range: "the number is outside the range that can be added exactly",
  missing_lead_time: "no lead time for the target plant",
  missing_moq: "no minimum order quantity for the target plant",
  unit_mismatch: "units differ between demand and the minimum order quantity",
  missing_demand: "no annual demand entered",
  missing_orders_per_year: "no orders per year entered",
  lead_time_exceeds_max: "lead time is longer than your limit",
  order_below_moq: "an order is below the minimum order quantity",
};

const NOT_MODELLED_WORDS: Record<string, string> = {
  capacity_utilisation: "capacity and utilisation",
  discounting_time_value: "discounting and the time value of money",
  lead_time_constraint: "a lead-time limit",
  ramp_up_yield: "ramp-up and yield",
  subsidies_incentives: "subsidies and incentives",
  taxes_residual_value: "taxes and residual value",
};

export function factorLabel(f: string): string {
  return FACTORS[f] ?? human(f);
}

export function reasonLabel(r: string): string {
  return REASONS[r] ?? human(r);
}

// "moq_constraint:motor_fan_assembly" -> "a minimum order quantity (motor-fan assembly)"
export function notModelledLabel(token: string): string {
  const known = NOT_MODELLED_WORDS[token];
  if (known) return known;
  const [head, ...rest] = token.split(":");
  const tail = rest.join(":");
  const base = head === "moq_constraint" ? "a minimum order quantity" : head === "group_cost" ? "group cost" : human(head);
  return tail ? `${base} (${g(tail)})` : base;
}

export function unknownLine(u: Unknown): string {
  return `${factorLabel(u.factor)}${u.group ? ` (${g(u.group)})` : ""}: ${reasonLabel(u.reason)}`;
}

export const hasFinancialUnknown = (o: PlanOperations) => o.unknown.some((u) => u.category === "financial");

// The authoritative net of an operations result, or null when it is not
// known. net_priced false (or priced false) means a financial input is
// missing: the legacy net_per_year is not read.
export function opsNet(p: { operations?: PlanOperations; net_priced?: boolean }): number | null {
  const o = p.operations;
  if (!o) return null;
  if (p.net_priced === false || !o.priced) return null;
  return o.net_modelled_per_year;
}

// One cell/figure of text for a plan row, target or scenario. With no
// `operations` this is the legacy figure, formatted as before.
export function netText(p: { operations?: PlanOperations; net_priced?: boolean; net_per_year?: number }): string {
  if (!p.operations) return money(p.net_per_year);
  const n = opsNet(p);
  if (n === null) return hasFinancialUnknown(p.operations) || p.net_priced === false ? UNKNOWN : NOT_MODELLED;
  if (n === 0) return NO_SAVING;
  return opsMoney(n);
}

// Scenario comparisons: net_usd already is the operations net (null when a
// financial input is unknown).
export function comparisonNetText(c: Comparison | undefined): string {
  if (!c) return money(undefined);
  if (!c.operations) return money(c.net_usd);
  if (c.net_usd === null || c.net_usd === undefined) return hasFinancialUnknown(c.operations) ? UNKNOWN : NOT_MODELLED;
  if (c.net_usd === 0) return NO_SAVING;
  return opsMoney(c.net_usd);
}

// A figure that may be null: a number, "unknown" or "not modelled", never $0.
export function figureText(v: number | null | undefined, ops: PlanOperations, factors: string[]): string {
  if (v !== null && v !== undefined) return opsMoney(v);
  return ops.unknown.some((u) => u.category === "financial" && factors.includes(u.factor)) ? UNKNOWN : NOT_MODELLED;
}

export type ComponentRow = { key: keyof PlanOperations["components"]; label: string; value: number | null; text: string };

const COMPONENTS: { key: keyof PlanOperations["components"]; label: string; factors: string[] }[] = [
  { key: "duty_effect_per_year", label: "Duty effect a year", factors: ["duty"] },
  { key: "group_cost_delta_per_year", label: "Group-cost change a year", factors: ["group_cost", "operating"] },
  { key: "labour_delta_per_year", label: "Labour change a year", factors: ["labour", "operating"] },
  { key: "logistics_delta_per_year", label: "Logistics change a year", factors: ["logistics", "operating"] },
  { key: "operating_delta_per_year", label: "Operating change a year", factors: ["operating", "group_cost", "labour", "logistics"] },
  { key: "one_time_total_usd", label: "One-time costs (USD, once)", factors: ["capex", "retooling", "one_time", "horizon"] },
  { key: "amortized_one_time_per_year", label: "One-time costs a year (amortized)", factors: ["amortized_one_time", "one_time", "capex", "retooling", "horizon"] },
];

export function componentRows(o: PlanOperations): ComponentRow[] {
  return COMPONENTS.map((c) => ({ key: c.key, label: c.label, value: o.components[c.key], text: figureText(o.components[c.key], o, c.factors) }));
}

export function feasibilityText(o: PlanOperations): string {
  if (!o.feasibility_assessed || !o.feasibility) return "Feasibility not assessed: no lead-time limit or minimum order quantity was entered.";
  if (o.feasibility === "feasible") return "Feasible on the limits you entered.";
  if (o.feasibility === "infeasible") return "Not feasible on the limits you entered.";
  return "Feasibility unknown: an input the limits need is missing. This is not a pass.";
}

export function checkText(c: Check): string {
  const unit = c.unit ? ` ${c.unit}` : "";
  const what = c.kind === "lead_time" ? "Lead time" : "Order quantity vs minimum";
  const nums = c.value !== undefined || c.limit !== undefined ? `: ${c.value ?? "?"}${unit} against ${c.limit ?? "?"}${unit}` : "";
  const status = c.status === "feasible" ? "within the limit" : c.status === "infeasible" ? "outside the limit" : "unknown";
  return `${what}${c.group ? ` (${g(c.group)})` : ""}${nums} · ${status}${c.reason && c.status !== "feasible" ? ` (${reasonLabel(c.reason)})` : ""}`;
}

export function notModelledLine(o: PlanOperations): string {
  return o.not_modelled.length ? `Not modelled: ${o.not_modelled.map(notModelledLabel).join("; ")}.` : "";
}

export const isCostRejection = (p: Pick<Plan, "rejection_kind">) => p.rejection_kind === "operationally_infeasible" || p.rejection_kind === "net_costlier";
