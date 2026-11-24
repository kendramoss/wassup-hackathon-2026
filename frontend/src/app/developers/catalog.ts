export const toolGroups = {
  origin: "Origin prediction",
  planning: "Production planning",
  tariff: "19 CFR 102.20 tariff-shift",
  broker: "Broker drafting",
} as const;

export type ToolGroup = keyof typeof toolGroups;

export const rates = {
  basic: "120 req/min",
  light: "20 req/min",
  heavy: "4 req/min",
  lookup: "30 req/min",
} as const;

type Tool = [name: string, group: ToolGroup, limit: keyof typeof rates, input: string, output: string];

// Keep names and rate classes in sync with the MCP guide and server registry.
export const tools = [
  ["predict_origin", "origin", "light", "Product facts + date", "Origin, citations and case ID"],
  ["extract_product", "origin", "light", "Product description", "Draft parts and plants"],
  ["extract_product_from_media", "origin", "light", "Photo or scanned parts list", "Draft parts and plants"],
  ["plan_production_moves", "planning", "heavy", "Case + target country", "Moves with supporting rulings"],
  ["compare_target_countries", "planning", "heavy", "Case + target countries", "Ranked move options"],
  ["search_rulings", "origin", "lookup", "Query + optional cutoff", "Matching CBP rulings"],
  ["get_ruling", "origin", "basic", "Ruling ID", "CBP ruling text"],
  ["get_duty_rates", "tariff", "basic", "Tariff code", "U.S. duty rates"],
  ["get_case", "origin", "basic", "Case ID", "Saved case and evidence"],
  ["list_cases", "origin", "basic", "API or case access key", "Saved cases and demos"],
  ["export_ruling_request_draft", "broker", "basic", "Case + optional plan", "Ruling request preview"],
  ["create_broker_review_link", "broker", "basic", "Case + draft choice", "Private review link"],
  ["get_broker_review_status", "broker", "basic", "Case ID", "Review decisions and comments"],
  ["suggest_part_hts", "tariff", "light", "Product + parts", "Candidate tariff codes"],
  ["check_rcep_preference", "tariff", "basic", "Codes, values + origins", "RCEP eligibility"],
  ["check_canada_marking", "tariff", "basic", "Good + parts codes", "Canadian marking rules check"],
  ["check_usmca_origin_rule", "tariff", "basic", "Tariff code, value method + materials", "USMCA origin-rule check"],
  ["check_eu_origin", "tariff", "basic", "Good + parts codes", "EU origin rules check"],
  ["check_uk_origin", "tariff", "basic", "Good + parts codes", "UK origin rules check"],
  ["get_evidence_report", "origin", "basic", "Case ID", "Every check, each separate"],
  ["save_case_answers", "origin", "basic", "Case ID + answers", "Saved answers, check re-run"],
] as const satisfies readonly Tool[];

export type ToolName = (typeof tools)[number][0];

export function filterTools(query: string, group: ToolGroup | "") {
  const terms = query.toLowerCase().trim().split(/\s+/).filter(Boolean);
  return tools.filter(([name, category, , input, output]) => {
    if (group && group !== category) return false;
    const searchable = [name.replaceAll("_", " "), name, input, output].join(" ").toLowerCase();
    return terms.every((term) => searchable.includes(term));
  });
}
