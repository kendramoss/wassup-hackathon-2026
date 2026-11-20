// #281 test fixtures. SYNTHETIC DTOs built from the sample JSON in
// ledger/reports/281-step3-dto-checkpoint.md (sections 2 to 6, v4.1), which is
// the wired contract the backend owner reported. The real backend wiring was
// not on this branch's base when these were written: nothing here claims a
// real backend response. Field presence follows the checkpoint: the always
// present members of `operations` are present (components never omitted, arrays
// never null), optional ones appear only where the checkpoint says they do.
import "../render-support.mjs";
import { createElement as h } from "react";
import { deepMount, textOf } from "./deep-mount.mjs";
import { json } from "./hook-runtime.mjs";

export { json, textOf };

export const NOT_MODELLED = ["capacity_utilisation", "discounting_time_value", "lead_time_constraint", "moq_constraint:motor_fan_assembly", "ramp_up_yield", "subsidies_incentives", "taxes_residual_value"];
export const ASSUMPTIONS = [
  "lead times of moved groups run in parallel: the move's lead time is the longest of them (no scheduling proof)",
  "one-time costs are amortized straight-line over horizon_years: no discounting, residual value or tax",
];

// Checkpoint section 2: priced, horizon 4, additional labour/logistics, capex 900k + retooling 100k at th.
export const opsPriced = (extra = {}) => ({
  components: {
    duty_effect_per_year: 500000, group_cost_delta_per_year: 120000, labour_delta_per_year: -40000, logistics_delta_per_year: 35000,
    operating_delta_per_year: 115000, one_time_total_usd: 1000000, amortized_one_time_per_year: 250000,
  },
  net_modelled_per_year: 135000, priced: true, horizon_years: 4, unknown: [], not_modelled: NOT_MODELLED, sunk_ignored: [], stale_refs: [],
  feasibility_assessed: false, checks: [], assumptions: ASSUMPTIONS,
  baseline: { duty_usd: 834000, operating_usd: 410000, total_usd: 1244000 },
  proposed: { duty_usd: 334000, operating_usd: 525000, amortized_one_time_usd: 250000, total_usd: 1109000 },
  ...extra,
});

// Checkpoint "Unknown example": labour missing at th, lead limit set, th lead time missing.
export const opsUnknown = () =>
  opsPriced({
    components: {
      duty_effect_per_year: 500000, group_cost_delta_per_year: 120000, labour_delta_per_year: null, logistics_delta_per_year: 35000,
      operating_delta_per_year: null, one_time_total_usd: 1000000, amortized_one_time_per_year: 250000,
    },
    net_modelled_per_year: null, priced: false, feasibility_assessed: true, feasibility: "unknown",
    unknown: [
      { category: "feasibility", factor: "lead_time", group: "motor_fan_assembly", reason: "missing_lead_time" },
      { category: "financial", factor: "labour", group: "motor_fan_assembly", reason: "missing_target" },
    ],
    checks: [{ kind: "lead_time", group: "motor_fan_assembly", status: "unknown", reason: "missing_lead_time" }],
    baseline: { duty_usd: 834000, operating_usd: null, total_usd: null },
    proposed: { duty_usd: 334000, operating_usd: null, amortized_one_time_usd: 250000, total_usd: null },
  });

export const opsInfeasible = () =>
  opsPriced({
    feasibility_assessed: true, feasibility: "infeasible", decision: "operationally_infeasible",
    checks: [{ kind: "lead_time", group: "motor_fan_assembly", status: "infeasible", value: 120, limit: 90, unit: "days", reason: "lead_time_exceeds_max" }],
  });

export const opsCostlier = () =>
  opsPriced({
    components: {
      duty_effect_per_year: 500000, group_cost_delta_per_year: 120000, labour_delta_per_year: -40000, logistics_delta_per_year: 35000,
      operating_delta_per_year: 115000, one_time_total_usd: 2000000, amortized_one_time_per_year: 500000,
    },
    net_modelled_per_year: -115000, decision: "net_costlier",
  });

export const verdict = (c = 0.96) => ({ confidence: c, agreement: "3/3", supporting_ruling: "N325810", counter_ruling: "N111111", reasoning: "Because." });

export const move = (extra = {}) => ({
  key: "motor_fan_assembly@th", description: "Make the motor-fan assembly in Thailand", move: [{ group: "motor_fan_assembly", to_plant: "th" }], shown: true,
  prediction: { origin_country: "Thailand", confidence: 0.95 }, verdict: verdict(),
  duty_effect_per_year: 500000, added_cost_per_year: 115000, net_per_year: 135000, net_priced: true, operations: opsPriced(), ...extra,
});

export const OBJECTIVE = {
  mode: "operations_v1", scope: "searched_moves_only",
  statement: "Ranked by modelled yearly duty saved minus modelled yearly operating cost difference minus one-time costs amortized over horizon_years, for the moves searched and the inputs you entered. Not a global optimum: terms listed in not_modelled are in neither side.",
};

export const planResult = (extra = {}) => ({
  target: "Thailand", plans: [move()], rejected: [], stats: { moves_searched: 3, engine_calls: 4, wall_seconds: 12 },
  objective: OBJECTIVE, economics_basis: "a".repeat(64), ...extra,
});

export const product = {
  name: "Heat gun",
  plants: [{ id: "cn", name: "Dongguan factory", country: "China" }, { id: "th", name: "Chonburi works", country: "Thailand" }],
  components: [{ name: "motor", made_in: "China", plant: "cn", group: "motor_fan_assembly" }, { name: "pcb", made_in: "China", plant: "cn", group: "pcba" }],
  operations: [{ name: "final assembly", plant: "cn", group: "final_assembly" }],
};

export const caseJson = (extra = {}) => ({
  id: "c1", facts: "f", step: "plan", date: "2026-01-01", display_name: "Heat gun",
  prediction: { origin_country: "China", confidence: 0.97 }, triage: { route: "auto", reason: "confidence 0.97 >= 0.92 and in scope" },
  product, economics: { illustrative: true, annual_import_value_usd: 2000000, duty_rates: { China: 0.417, Thailand: 0.167 } }, ...extra,
});

// Mounts a step against a fake API. `serve(url, method, body)` may answer a
// call; otherwise the case is served and everything else answers {}. Every
// request is recorded.
export function makeMount(live) {
  return async function mount(Step, c, serve = () => null) {
    const state = { current: c };
    const calls = [];
    globalThis.fetch = async (url, init = {}) => {
      const u = String(url);
      const m = init.method ?? "GET";
      let body;
      try {
        body = typeof init.body === "string" ? JSON.parse(init.body) : undefined;
      } catch {}
      calls.push({ m, u, body, raw: init.body });
      const r = serve(u, m, body, state);
      if (r) return r;
      if (/\/cases\/c1$/.test(u) && m === "GET") return json(state.current);
      if (u.endsWith("/plan/estimate")) return json({});
      if (u.includes("/demos")) return json({ demos: [] });
      if (u.endsWith("/api/session")) return json({ email: null });
      return json({});
    };
    const el = Step();
    const app = deepMount(h(el.type, el.props));
    live.push(app);
    app.refresh();
    await app.flush();
    return Object.assign(app, { calls, state });
  };
}

export const pageText = (app) => app.all().map((e) => textOf(e.props?.children)).join(" ");
// The text of the page once, from the roots (no repeated descendants).
export const flatText = (app) => {
  const seen = new Set();
  const out = [];
  for (const e of app.all()) {
    if (typeof e.type === "string" && e.props?.children !== undefined) {
      const t = textOf(e.props.children);
      if (t && !seen.has(t)) {
        seen.add(t);
        out.push(t);
      }
    }
  }
  return out.join(" ¦ ");
};

// The operating-cost fields whose label starts with `start`, in page order:
// the input element and the error list the field carries. (Found by structure,
// not by id: the test hook runtime numbers ids per component, so two fields can
// share one.)
export const fields = (app, start) =>
  app
    .find((e) => e.type === "div" && Array.isArray(e.props.children) && e.props.children[0]?.type === "label" && textOf(e.props.children[0].props.children).startsWith(start))
    .map((d) => ({ div: d, input: d.props.children[1], errors: d.props.children[2]?.props?.list }));
