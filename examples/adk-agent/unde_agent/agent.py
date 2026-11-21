"""A Google ADK trade-compliance agent that runs a full Unde case over MCP.

It connects to Unde's MCP server (Streamable HTTP) with ADK's McpToolset,
offers the model only the tools a case needs, and walks the flow:

    extract_product -> predict_origin -> plan_production_moves
        -> export_ruling_request_draft

with suggest_part_hts and get_ruling when the server lists them.

Configuration, all from the environment (nothing secret is in this file):

    UNDE_MCP_URL     MCP endpoint (default https://undelabs.dev/mcp)
    UNDE_API_KEY     optional Unde API key (op_...); without one the agent
                     works anonymously and passes the case_access_key that
                     predict_origin returns to every later call
    ADK_MODEL        model name (default gemini-flash-latest, or
                     gemini-3.8-flash on the gateway path)

Model, pick one:
    Gemini directly (what a real user runs): GOOGLE_API_KEY for AI Studio, or
        GOOGLE_GENAI_USE_VERTEXAI=TRUE with GOOGLE_CLOUD_PROJECT and
        GOOGLE_CLOUD_LOCATION for Vertex AI. ADK talks to Gemini natively.
    An OpenAI-compatible LiteLLM gateway to Gemini: LITELLM_BASE_URL and
        LITELLM_API_KEY. ADK's LiteLlm wrapper is used. This is what the
        committed transcript ran.
"""
import os

from google.adk.agents import LlmAgent
from google.adk.tools.mcp_tool import McpToolset
from google.adk.tools.mcp_tool.mcp_session_manager import StreamableHTTPConnectionParams

MCP_URL = os.environ.get("UNDE_MCP_URL", "https://undelabs.dev/mcp")

# Only what a case needs. ADK drops names the server does not list, so
# suggest_part_hts is simply absent on a server that does not have it yet.
TOOLS = [
    "extract_product",
    "predict_origin",
    "plan_production_moves",
    "export_ruling_request_draft",
    "suggest_part_hts",
    "get_ruling",
]

DISCLAIMER = "Not a ruling — confirm with a licensed customs broker."

INSTRUCTION = f"""You are a trade-compliance assistant for importers into the United States.
You use Unde's tools to predict the country of origin U.S. Customs (CBP) would likely
assign to a product under the substantial-transformation test, and to plan production moves.

Work in this order and call each tool once unless a call fails:
1. extract_product with the user's description as `facts` (and `family` if the user names
   one). It returns a structured `product` with parts, plants and operations.
   Plan search can only move work to a plant in the target country. If the product has no
   plant there, add one to product.plants (id "target_plant", that country, and as
   capabilities every group id the product's components and operations use).
2. predict_origin with `facts`, that `product` and `family`. Report the predicted origin, the
   confidence and the CBP rulings it cites, by ruling number.
3. If suggest_part_hts is available and parts have no HTS codes, call it once with the
   product and report the codes as unverified suggestions. Do not add them to the product.
4. plan_production_moves on the case with the target country. Report the best plan: which
   groups move, the predicted origin after the move, and why earlier rulings support it.
5. export_ruling_request_draft for that plan (pass its move_key and the target, or its
   plan_index). Give the draft's opening lines and say it is a draft for a broker to review.

Rules:
- Without an API key, predict_origin returns a case_access_key. Pass it unchanged to every
  later call on that case. Never print it to the user.
- Use only what the tools return. Do not invent rulings, duties or costs.
- If a tool returns an error, say what failed and stop.
- End every answer with this line, word for word: {DISCLAIMER}
"""


def _headers():
    key = os.environ.get("UNDE_API_KEY")
    return {"Authorization": "Bearer " + key} if key else None


def make_toolset() -> McpToolset:
    return McpToolset(
        # Plan search is the slow tool. In the committed run one call passed
        # ADK's default 300 s read timeout and the agent called it again;
        # 900 s gives it room (D391).
        connection_params=StreamableHTTPConnectionParams(url=MCP_URL, headers=_headers(), sse_read_timeout=900),
        tool_filter=TOOLS,
    )


def make_model():
    """Gemini by name for a Google API key or Vertex AI; LiteLlm for a gateway."""
    base = os.environ.get("LITELLM_BASE_URL")
    if not base:
        return os.environ.get("ADK_MODEL", "gemini-flash-latest")
    from google.adk.models.lite_llm import LiteLlm

    # "openai/<alias>" is LiteLLM's route to an OpenAI-compatible server.
    return LiteLlm(
        model="openai/" + os.environ.get("ADK_MODEL", "gemini-3.8-flash"),
        api_base=base.rstrip("/") + "/v1",
        api_key=os.environ["LITELLM_API_KEY"],
    )


def drop_root_composition(callback_context, llm_request):
    """Removes anyOf/oneOf/allOf/not at the root of each tool's parameters.

    Unde's server no longer puts them there (D390). Before that fix,
    predict_origin's root anyOf made an OpenAI-compatible gateway refuse the
    whole request with a 400. This keeps the agent working against a server
    that still has it, and does nothing otherwise.
    """
    for tool in (llm_request.config.tools if llm_request.config else None) or []:
        for fd in getattr(tool, "function_declarations", None) or []:
            schema = fd.parameters_json_schema
            if isinstance(schema, dict):
                for k in ("anyOf", "oneOf", "allOf", "not"):
                    schema.pop(k, None)
    return None


root_agent = LlmAgent(
    name="unde_trade_compliance",
    model=make_model(),
    description="Predicts U.S. country of origin with cited CBP rulings, plans production moves and drafts a broker request, using Unde over MCP.",
    instruction=INSTRUCTION,
    tools=[make_toolset()],
    before_model_callback=drop_root_composition,
)
