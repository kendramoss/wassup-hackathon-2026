"""Runs one Unde case with the ADK agent and prints a redacted transcript.

    python run_case.py                      # the heat-gun case below
    python run_case.py "your product and target country"

Each line is prefixed with the seconds since start, so the output doubles as
a text replay. The case_access_key and any API key are replaced with
placeholders; tool results are cut to their text content. With a LiteLLM
gateway it also prints the model cost the gateway reports.
"""
import asyncio
import json
import os
import re
import sys
import time

from google.adk.runners import Runner
from google.adk.sessions import InMemorySessionService
from google.genai import types

from unde_agent import agent

DEFAULT_CASE = (
    "Product: a 1,800 W handheld electric heat gun for paint stripping. "
    "Parts: universal motor and fan made in China; control PCBA made in China; "
    "heating element (resistance wire on a mica former) made in China; "
    "plastic housing and nozzle moulded in Vietnam; power cord made in Vietnam. "
    "The motor-fan unit, PCBA and heating element are assembled into the housing "
    "and tested in Vietnam. We also run a plant in Thailand that can do motor-fan assembly, "
    "PCBA assembly, heating-element production and final assembly. "
    "Family: motorised_tools_small_appliances. "
    "Target country for production moves: Thailand. "
    "Predict the U.S. origin, plan the moves to make it Thai, and draft the broker request."
)

START = time.monotonic()
COST = [0.0, 0]
SECRET = re.compile(r"\bmcp_[A-Za-z0-9_-]{8,}|\bop_[A-Za-z0-9_-]{8,}")
MAX_RESULT = int(os.environ.get("MAX_RESULT_CHARS", "2500"))


def out(title, body=""):
    print(f"[+{time.monotonic() - START:5.1f}s] {title}", flush=True)
    if body:
        print(SECRET.sub("<redacted key>", body).rstrip() + "\n", flush=True)


def result_text(resp):
    """The text blocks of an MCP tool result; structuredContent is left out."""
    if isinstance(resp, dict) and isinstance(resp.get("content"), list):
        text = "\n".join(c.get("text", "") for c in resp["content"] if isinstance(c, dict))
        flag = " (tool error)" if resp.get("isError") else ""
    else:
        text, flag = json.dumps(resp, ensure_ascii=False), ""
    if len(text) > MAX_RESULT:
        text = text[:MAX_RESULT] + f"\n[... {len(text) - MAX_RESULT} more characters cut from this transcript]"
    return flag, text


def track_gateway_cost():
    if not os.environ.get("LITELLM_BASE_URL"):
        return
    import litellm
    from litellm.integrations.custom_logger import CustomLogger

    class Tally(CustomLogger):
        def _add(self, response):
            h = (getattr(response, "_hidden_params", None) or {}).get("additional_headers") or {}
            v = h.get("llm_provider-x-litellm-response-cost")
            if v is not None:
                COST[0] += float(v)
                COST[1] += 1

        def log_success_event(self, kwargs, response_obj, start_time, end_time):
            self._add(response_obj)

        async def async_log_success_event(self, kwargs, response_obj, start_time, end_time):
            self._add(response_obj)

    litellm.callbacks = [Tally()]


async def main(question):
    track_gateway_cost()
    runner = Runner(app_name="unde_adk", agent=agent.root_agent, session_service=InMemorySessionService())
    session = await runner.session_service.create_session(app_name="unde_adk", user_id="demo")
    tools = await agent.root_agent.tools[0].get_tools()
    out("MCP server " + agent.MCP_URL + "; tools offered to the model: " + ", ".join(sorted(t.name for t in tools)))
    out("User", question)
    msg = types.Content(role="user", parts=[types.Part(text=question)])
    async for ev in runner.run_async(user_id="demo", session_id=session.id, new_message=msg):
        for p in (ev.content.parts if ev.content else None) or []:
            if p.function_call:
                out("Tool call: " + p.function_call.name, json.dumps(p.function_call.args, ensure_ascii=False))
            elif p.function_response:
                flag, text = result_text(p.function_response.response)
                out("Tool result: " + p.function_response.name + flag, text)
            elif p.text and ev.is_final_response():
                out("Agent", p.text.strip())
    await agent.root_agent.tools[0].close()
    if COST[1]:
        out(f"Gateway-reported model cost: ${COST[0]:.4f} over {COST[1]} model calls")


if __name__ == "__main__":
    asyncio.run(main(sys.argv[1] if len(sys.argv) > 1 else DEFAULT_CASE))
