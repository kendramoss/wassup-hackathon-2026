import type { CaseResponse, Ruling } from "@/lib/api/client";
import type { components } from "@/lib/api/schema";
import type { ToolName } from "./catalog";

type PlanResponse = components["schemas"]["PlanResponse"];

export const mcpEndpoint = "https://undelabs.dev/mcp";
export const keyHeader = "X-API-Key: op_...";
const caseId = "case_0123456789ab";
const accessKey = "mcp_example_not_a_real_key";
const rulingId = "N328954";
const facts = "A heat gun is assembled in Thailand from a motor and control board made in China.";
const disclaimer = "Not a ruling. Confirm with a licensed customs broker or a CBP binding ruling.";

export const setupExamples = {
  gemini: {
    label: "Gemini CLI",
    note: "",
    code: `gemini mcp add --transport http unde ${mcpEndpoint}`,
  },
  claude: {
    label: "Claude Desktop",
    note: "Add the URL as a custom connector, or run the published relay with npx.",
    code: JSON.stringify({
      mcpServers: {
        unde: {
          command: "npx",
          args: ["-y", "@undelabs/mcp"],
        },
      },
    }, null, 2),
  },
  adk: {
    label: "Google ADK, Go",
    note: "Illustrative Go code for Google’s Agent Development Kit (ADK). Add the toolset to your agent.",
    code: `package unde

import (
    "github.com/modelcontextprotocol/go-sdk/mcp"
    "google.golang.org/adk/tool"
    "google.golang.org/adk/tool/mcptoolset"
)

func Toolset() (tool.Toolset, error) {
    return mcptoolset.New(mcptoolset.Config{
        Transport: &mcp.StreamableClientTransport{
            Endpoint: "${mcpEndpoint}",
        },
    })
}`,
  },
} as const;

const caseRequest = { date: "2026-06-01", facts, jurisdiction: "us" };
const casePreview = {
  id: caseId,
  prediction: { origin_country: "China", confidence: 0.96, cited_rulings: [rulingId] },
  disclaimer,
} satisfies Pick<CaseResponse, "id" | "prediction" | "disclaimer">;
const planPreview = {
  case_id: caseId,
  plan: {
    target: "Thailand",
    plans: [{
      description: "Move motor-fan assembly to Thailand",
      prediction: { origin_country: "Thailand", confidence: 0.96, cited_rulings: ["N345938"] },
      verdict: { precedent_supported: true, supporting_ruling: "N345938", confidence: 0.96 },
    }],
  },
  disclaimer,
} satisfies Pick<PlanResponse, "case_id" | "plan" | "disclaimer">;
const rulingPreview = {
  id: rulingId,
  origin: "China",
  url: `https://rulings.cbp.gov/ruling/${rulingId}`,
} satisfies Ruling;
const product = {
  name: "Heat gun",
  components: [{ name: "Motor", made_in: "China" }, { name: "Control board", made_in: "China" }],
};

type Example = {
  input: Record<string, unknown>;
  output: Record<string, unknown>;
  note?: string;
};

// Illustrative excerpts use the field names returned by the MCP server.
// API tool schemas: backend/internal/mcpserver. REST response shapes: OpenAPI.
export const toolExamples: Record<ToolName, Example> = {
  predict_origin: {
    input: caseRequest,
    output: { ...casePreview, case_access_key: accessKey },
    note: "Without an API key, pass the returned case_access_key on later calls. Keep it private.",
  },
  extract_product: { input: { facts }, output: { product, warnings: [] } },
  extract_product_from_media: {
    input: { files: [{ data: "iVBORw0KGgo=", mime_type: "image/png" }] },
    output: { product, warnings: [] },
    note: "Replace the sample data with the complete base64 of your file.",
  },
  plan_production_moves: {
    input: { case_id: caseId, case_access_key: accessKey, target_country: "Thailand" },
    output: planPreview,
    note: "Use a U.S. case with structured parts and a product family.",
  },
  compare_target_countries: {
    input: { case_id: caseId, case_access_key: accessKey, targets: ["Thailand", "Vietnam"] },
    output: { targets: [{ target: "Thailand", rank: 1 }, { target: "Vietnam", rank: 2 }], disclaimer },
  },
  search_rulings: {
    input: { query: "heat gun", before: "2026-06-01", limit: 3 },
    output: { rulings: [{ id: rulingId, origin: "China" }] },
  },
  get_ruling: { input: { ruling_id: rulingId }, output: rulingPreview },
  get_duty_rates: {
    input: { hts: "8516.79.0000", country: "Thailand" },
    output: { hts: "8516.79.0000", rates: [{ origin: "Thailand", unverified: true }] },
  },
  get_case: { input: { case_id: caseId, case_access_key: accessKey }, output: casePreview },
  list_cases: {
    input: { case_access_key: accessKey },
    output: { cases: [{ id: caseId, display_name: "Heat gun", predicted_origin: "China", step: "origin" }] },
  },
  export_ruling_request_draft: {
    input: { case_id: caseId, case_access_key: accessKey, format: "markdown" },
    output: { format: "markdown", content: "Draft request for a country-of-origin ruling.", disclaimer },
  },
  create_broker_review_link: {
    input: { case_id: caseId, case_access_key: accessKey, expires_in_days: 7 },
    output: { review_path: "/review#sample-token", review: { status: "pending" } },
  },
  get_broker_review_status: {
    input: { case_id: caseId, case_access_key: accessKey },
    output: { reviews: [{ status: "pending", decisions: [] }] },
  },
  suggest_part_hts: {
    input: { product },
    output: { parts: [{ name: "Motor", candidates: [{ hts: "8501.10", source: "model" }] }] },
  },
  check_rcep_preference: {
    input: { hs: "4412.31", produced_in: "Vietnam" },
    output: { outcome: "undetermined", regime: "rcep_preferential", reasons: ["Material details are needed."] },
  },
  check_canada_marking: {
    input: { good_hts: "6301.30", country: "Mexico", materials: [{ hts: "5205.11", origin: "United States" }] },
    output: {
      jurisdiction: "ca",
      label: "Rules check only — no accuracy figure yet (no published rulings to test against)",
      result: { status: "undetermined" },
      note: "Canada marking review. Not a ruling.",
    },
  },
  check_usmca_origin_rule: {
    input: { destination: "US", regime: "usmca", good_hs: "8202.31", rvc_method: "transaction_value", transaction_value: "100", vnm: "40" },
    output: { status: "needs_information", regime: "usmca", missing_facts: [{ field: "profile.simple_goods_confirmed" }] },
  },
  check_eu_origin: {
    input: { good_hts: "8527.13", country: "Malaysia", materials: [{ hts: "8534.00", origin: "China" }] },
    output: {
      jurisdiction: "eu",
      label: "Rules check only — no accuracy figure (no published origin rulings to test against)",
      result: { status: "decided", origin: "Malaysia", basis: "Art. 32 DA; Delegated Regulation (EU) 2015/2446, Annex 22-01, 8527" },
    },
  },
  check_uk_origin: {
    input: { good_hts: "8471.30", country: "Vietnam", materials: [{ hts: "8473.30", origin: "China" }] },
    output: {
      jurisdiction: "uk",
      label: "Rules check only — no accuracy figure (no published origin rulings to test against)",
      result: { status: "needs_judgement", basis: "Taxation (Cross-border Trade) Act 2018 s. 17(3)" },
    },
  },
};

const api = "https://undelabs.dev/api";

function curl(method: "GET" | "POST", path: string, body?: Record<string, unknown>, needsKey = true) {
  const args = [`curl -X ${method} "${api}${path}"`];
  if (needsKey) args.push(`  -H "${keyHeader}"`);
  if (body) {
    args.push('  -H "Content-Type: application/json"', `  -d '${JSON.stringify(body)}'`);
  }
  return args.join(" \\\n");
}

export const endpoints = {
  predict: {
    label: "POST /v1/cases/predict",
    status: "201",
    command: curl("POST", "/v1/cases/predict", caseRequest),
    output: casePreview,
    note: "",
  },
  plan: {
    label: "POST /v1/cases/{id}/plan",
    status: "200",
    command: curl("POST", "/v1/cases/$CASE_ID/plan", { target: "Thailand" }),
    output: planPreview,
    note: "Set CASE_ID to your structured U.S. case.",
  },
  ruling: {
    label: "GET /v1/rulings/{id}",
    status: "200",
    command: curl("GET", `/v1/rulings/${rulingId}`, undefined, false),
    output: rulingPreview,
    note: "Public CBP ruling lookup. API key optional.",
  },
} as const;
