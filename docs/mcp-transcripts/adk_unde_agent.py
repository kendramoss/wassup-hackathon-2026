"""Minimal Google ADK agent that uses Unde's hosted MCP server.

The model is reached through an OpenAI-compatible LiteLLM gateway (ADK's
LiteLlm wrapper). Set LITELLM_BASE_URL and LITELLM_API_KEY in the
environment; nothing secret is in this file. It prints the MCP tool list and
every tool call and result, so the transcript can be committed.

    python adk_unde_agent.py "your question"
"""
import asyncio
import json
import os
import sys

from google.adk.agents import LlmAgent
from google.adk.models.lite_llm import LiteLlm
from google.adk.runners import Runner
from google.adk.sessions import InMemorySessionService
from google.adk.tools.mcp_tool import McpToolset
from google.adk.tools.mcp_tool.mcp_session_manager import StreamableHTTPConnectionParams
from google.genai import types

import litellm
from litellm.integrations.custom_logger import CustomLogger

MCP_URL = "https://undelabs.dev/mcp"
GATEWAY_COST = [0.0, 0]


class _Tally(CustomLogger):
    """Sums the cost the gateway reports in its response header."""

    def _add(self, response):
        headers = (getattr(response, "_hidden_params", None) or {}).get("additional_headers") or {}
        v = headers.get("llm_provider-x-litellm-response-cost")
        if v is not None:
            GATEWAY_COST[0] += float(v)
            GATEWAY_COST[1] += 1

    def log_success_event(self, kwargs, response_obj, start_time, end_time):
        self._add(response_obj)

    async def async_log_success_event(self, kwargs, response_obj, start_time, end_time):
        self._add(response_obj)


litellm.callbacks = [_Tally()]
MODEL = os.environ.get("ADK_MODEL", "gemini-3.8-flash")


async def main(question: str) -> None:
    listing = McpToolset(connection_params=StreamableHTTPConnectionParams(url=MCP_URL))
    print("## MCP tools listed by", MCP_URL)
    for t in await listing.get_tools():
        print("-", t.name)
    await listing.close()
    # The agent gets the two read-only lookup tools only: no case is created
    # and no model call runs on the Unde side.
    toolset = McpToolset(connection_params=StreamableHTTPConnectionParams(url=MCP_URL), tool_filter=["search_rulings", "get_ruling"])
    agent = LlmAgent(
        # "openai/<alias>": LiteLLM's OpenAI-compatible route to the gateway.
        model=LiteLlm(model="openai/" + MODEL, api_base=os.environ["LITELLM_BASE_URL"].rstrip("/") + "/v1", api_key=os.environ["LITELLM_API_KEY"]),
        name="origin_researcher",
        instruction="Answer with Unde's tools. Use search_rulings to find earlier CBP rulings, then answer in two sentences and name the ruling numbers.",
        tools=[toolset],
    )
    runner = Runner(app_name="unde_adk_check", agent=agent, session_service=InMemorySessionService())
    session = await runner.session_service.create_session(app_name="unde_adk_check", user_id="check")
    print("\n## Question\n" + question)
    msg = types.Content(role="user", parts=[types.Part(text=question)])
    async for ev in runner.run_async(user_id="check", session_id=session.id, new_message=msg):
        for p in (ev.content.parts if ev.content else []) or []:
            if p.function_call:
                print("\n## Tool call\n" + p.function_call.name + " " + json.dumps(p.function_call.args, ensure_ascii=False))
            elif p.function_response:
                body = json.dumps(p.function_response.response, ensure_ascii=False)
                print("\n## Tool result (" + p.function_response.name + ", first 1,500 characters)\n" + body[:1500])
            elif p.text and ev.is_final_response():
                print("\n## Final answer\n" + p.text.strip())
    await toolset.close()
    print(f"\n## Gateway-reported model cost\n${GATEWAY_COST[0]:.4f} over {GATEWAY_COST[1]} model calls")


if __name__ == "__main__":
    asyncio.run(main(sys.argv[1] if len(sys.argv) > 1 else "Which earlier CBP rulings cover a heat gun assembled in Thailand from a Chinese motor and PCBA? Search with the words: heat gun motor assembled Thailand."))
