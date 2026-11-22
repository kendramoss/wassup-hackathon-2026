"""Runs the ADK agent on one product of the agent evaluation set (D450) and
appends one JSON row to a results file. Scoring is separate:
eval/agent/score.py reads the rows and needs no network.

    python run_eval.py --id a01 --out ../../eval/agent/results/<run>.jsonl
    python run_eval.py --id a01 --scripted --out /tmp/plumbing.jsonl

--scripted swaps the model for a fixed script that calls the five tools in
the expected order (no model calls, no cost). It tests the harness and the
scorer against a local server (UNDE_MCP_URL=http://localhost:<port>/mcp,
LLM_PROVIDER=fake); it says nothing about the agent.

One product per process, so the gateway-reported cost is per run. What is
kept per tool call: the arguments (case_access_key replaced, product cut to
a short summary), the result's text cut to 1,500 characters, whether it was
an error, a few fields the scorer needs, and every CBP ruling number found
anywhere in the result (text and structuredContent). The final answer is
kept whole. No key, gateway host or header is written.
"""
import argparse
import asyncio
import json
import os
import re
import sys
import time
from typing import AsyncGenerator

from google.adk.agents import LlmAgent
from google.adk.models.base_llm import BaseLlm
from google.adk.models.llm_response import LlmResponse
from google.adk.runners import Runner
from google.adk.sessions import InMemorySessionService
from google.genai import types

from unde_agent import agent as unde

RULING = re.compile(r"\b(?:[HN]\d{6}|NY [A-Z]?\d{5,6}|HQ \d{6})\b")
SECRET = re.compile(r"\bmcp_[A-Za-z0-9_-]{8,}|\bop_[A-Za-z0-9_-]{8,}")
TEXT_CUT = 1500
FIXTURE = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..", "backend", "testdata", "heatgun_2022.json")


def rulings_in(obj):
    s = json.dumps(obj, ensure_ascii=False)
    return sorted({m.replace("NY ", "N").replace("HQ ", "H") for m in RULING.findall(s)})


def scrub(s, secrets):
    for v in secrets:
        if v:
            s = s.replace(v, "<redacted>")
    return SECRET.sub("<redacted key>", s)


def product_summary(p):
    if not isinstance(p, dict):
        return None
    plants = p.get("plants") or []
    return {
        "components": len(p.get("components") or []),
        "operations": len(p.get("operations") or []),
        "plants": [{"id": x.get("id"), "country": x.get("country")} for x in plants if isinstance(x, dict)],
    }


def structured(resp):
    if isinstance(resp, dict):
        sc = resp.get("structuredContent")
        if isinstance(sc, dict):
            return sc
    return {}


def text_of(resp):
    if isinstance(resp, dict) and isinstance(resp.get("content"), list):
        return "\n".join(c.get("text", "") for c in resp["content"] if isinstance(c, dict))
    return json.dumps(resp, ensure_ascii=False)


def predict_fields(sc):
    pred = sc.get("prediction") if isinstance(sc.get("prediction"), dict) else {}
    cited = [c.get("ruling") if isinstance(c, dict) else c for c in pred.get("cited_rulings") or []]
    return {
        "case_id": sc.get("id"),
        "origin": pred.get("origin_country"),
        "precedents": [p for p in pred.get("precedents") or [] if isinstance(p, str)],
        "triage": (sc.get("triage") or {}).get("route"),
        "confidence": pred.get("confidence"),
        "calibrated_confidence": pred.get("calibrated_confidence"),
        "cited": [c for c in cited if c],
        "llm_provider": sc.get("llm_provider"),
    }


def plan_fields(sc):
    plan = sc.get("plan") if isinstance(sc.get("plan"), dict) else {}
    plans = [p for p in plan.get("plans") or [] if isinstance(p, dict)]
    return {
        "plans": len(plans),
        "plan_origins": [(p.get("prediction") or {}).get("origin_country") for p in plans],
        "rejected": len(plan.get("rejected") or []),
    }


# --- scripted model: plumbing only -----------------------------------------

class ScriptedModel(BaseLlm):
    """Calls extract -> predict -> plan -> export, then answers. No model."""

    model: str = "scripted"
    target: str = ""
    family: str = ""

    async def generate_content_async(self, llm_request, stream: bool = False) -> AsyncGenerator[LlmResponse, None]:
        user, done = "", {}
        for c in llm_request.contents or []:
            for p in c.parts or []:
                if p.text and c.role == "user" and not user:
                    user = p.text
                if p.function_response:
                    done[p.function_response.name] = p.function_response.response
        pred = structured(done.get("predict_origin", {}))
        case_id = pred.get("id")
        key = pred.get("case_access_key")
        if "extract_product" not in done:
            call = ("extract_product", {"facts": user, "family": self.family})
        elif "predict_origin" not in done:
            prod = structured(done["extract_product"]).get("product")
            family = self.family
            if not (isinstance(prod, dict) and prod.get("components")):
                # The fake server's extract returns an empty draft; plumbing
                # then uses a committed fixture product so plan and export run.
                fx = json.load(open(FIXTURE))
                prod, family = fx["product"], fx["family"]
            call = ("predict_origin", {"facts": user, "product": prod, "family": family})
        elif "plan_production_moves" not in done:
            call = ("plan_production_moves", {"case_id": case_id, "case_access_key": key, "target_country": self.target,
                                               "idempotency_key": f"{case_id}-plan-{self.target}"})
        elif "export_ruling_request_draft" not in done:
            call = ("export_ruling_request_draft", {"case_id": case_id, "case_access_key": key, "plan_index": 0, "target": self.target})
        else:
            pf = predict_fields(pred)
            text = (f"Predicted origin: {pf['origin']}. Cited rulings: {', '.join(pf['cited']) or 'none'}.\n"
                    f"{unde.DISCLAIMER}")
            yield LlmResponse(content=types.Content(role="model", parts=[types.Part(text=text)]))
            return
        name, args = call
        args = {k: v for k, v in args.items() if v not in (None, "")}
        yield LlmResponse(content=types.Content(role="model", parts=[types.Part(function_call=types.FunctionCall(name=name, args=args))]))


# --- gateway cost ----------------------------------------------------------

COST = {"usd": 0.0, "calls": 0, "priced": 0, "prompt_tokens": 0, "completion_tokens": 0}


def track_gateway_cost():
    import litellm
    from litellm.integrations.custom_logger import CustomLogger

    class Tally(CustomLogger):
        def _add(self, response):
            COST["calls"] += 1
            h = (getattr(response, "_hidden_params", None) or {}).get("additional_headers") or {}
            v = h.get("llm_provider-x-litellm-response-cost")
            if v is not None:
                COST["usd"] += float(v)
                COST["priced"] += 1
            u = getattr(response, "usage", None)
            if u is not None:
                COST["prompt_tokens"] += int(getattr(u, "prompt_tokens", 0) or 0)
                COST["completion_tokens"] += int(getattr(u, "completion_tokens", 0) or 0)

        def log_success_event(self, kwargs, response_obj, start_time, end_time):
            self._add(response_obj)

        async def async_log_success_event(self, kwargs, response_obj, start_time, end_time):
            self._add(response_obj)

    litellm.callbacks = [Tally()]


async def run_one(prod, scripted, label):
    model = ScriptedModel(target=prod["target"], family=prod["family"]) if scripted else unde.make_model()
    if not scripted and os.environ.get("LITELLM_BASE_URL"):
        track_gateway_cost()
    toolset = unde.make_toolset()
    ag = LlmAgent(name="unde_trade_compliance", model=model, instruction=unde.INSTRUCTION, tools=[toolset],
                  description="Unde ADK agent (evaluation run)")
    runner = Runner(app_name="unde_adk_eval", agent=ag, session_service=InMemorySessionService())
    session = await runner.session_service.create_session(app_name="unde_adk_eval", user_id="eval")
    offered = sorted(t.name for t in await toolset.get_tools())
    secrets = [os.environ.get(k) for k in ("LITELLM_API_KEY", "LITELLM_BASE_URL", "UNDE_API_KEY")]
    calls, final, error, access_key = [], "", None, None
    pending = {}
    t0 = time.monotonic()
    msg = types.Content(role="user", parts=[types.Part(text=prod["prompt"])])
    try:
        async for ev in runner.run_async(user_id="eval", session_id=session.id, new_message=msg):
            now = round(time.monotonic() - t0, 1)
            for p in (ev.content.parts if ev.content else None) or []:
                if p.function_call:
                    args = dict(p.function_call.args or {})
                    rec = {"tool": p.function_call.name, "t_call": now, "args": args}
                    calls.append(rec)
                    pending.setdefault(p.function_call.name, []).append(rec)
                elif p.function_response:
                    name = p.function_response.name
                    resp = p.function_response.response
                    rec = (pending.get(name) or [None]).pop(0) if pending.get(name) else None
                    if rec is None:
                        rec = {"tool": name, "t_call": None, "args": {}}
                        calls.append(rec)
                    sc = structured(resp)
                    is_err = bool(isinstance(resp, dict) and resp.get("isError")) or (isinstance(resp, dict) and "error" in resp and not sc)
                    rec.update({"t_result": now, "error": is_err, "rulings": rulings_in(resp),
                                "text": text_of(resp)[:TEXT_CUT]})
                    if name == "predict_origin" and not is_err:
                        rec["fields"] = predict_fields(sc)
                        access_key = access_key or sc.get("case_access_key")
                    elif name == "plan_production_moves" and not is_err:
                        rec["fields"] = plan_fields(sc)
                    elif name == "extract_product" and not is_err:
                        rec["fields"] = {"product": product_summary(sc.get("product"))}
                elif p.text and ev.is_final_response():
                    final += p.text
    except Exception as e:  # recorded, scored as a failed run
        error = f"{type(e).__name__}: {e}"
    finally:
        try:
            await toolset.close()
        except Exception:
            pass
    wall = round(time.monotonic() - t0, 1)

    # Arguments: the scorer needs whether the access key was passed and was
    # the one predict_origin returned; the key itself is never written.
    for rec in calls:
        a = rec["args"]
        if "case_access_key" in a:
            rec["access_key_sent"] = True
            rec["access_key_matches"] = bool(access_key) and a["case_access_key"] == access_key
            a["case_access_key"] = "<redacted key>"
        if isinstance(a.get("product"), dict):
            a["product"] = {"summary": product_summary(a["product"])}
        if isinstance(a.get("facts"), str):
            a["facts_chars"] = len(a["facts"])
            a["facts_is_prompt_prefix"] = a["facts"].strip()[:80] in prod["prompt"]
            del a["facts"]
    row = {
        "run": label, "id": prod["id"], "ruling": prod["ruling"], "target": prod["target"],
        "model": "scripted" if scripted else os.environ.get("ADK_MODEL", "gemini-3.8-flash" if os.environ.get("LITELLM_BASE_URL") else "gemini-flash-latest"),
        "mcp": "local" if "localhost" in unde.MCP_URL or "127.0.0.1" in unde.MCP_URL else unde.MCP_URL,
        "tools_offered": offered, "started": time.strftime("%Y-%m-%dT%H:%M:%S%z"),
        "wall_s": wall, "calls": calls, "final": final.strip(), "error": error,
        "cost_usd": round(COST["usd"], 6) if COST["calls"] else 0.0, "model_calls": COST["calls"],
        "priced_calls": COST["priced"], "prompt_tokens": COST["prompt_tokens"], "completion_tokens": COST["completion_tokens"],
    }
    return json.loads(scrub(json.dumps(row, ensure_ascii=False), secrets))


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--id", required=True)
    ap.add_argument("--products", default=os.path.join(os.path.dirname(__file__), "..", "..", "eval", "agent", "products.jsonl"))
    ap.add_argument("--out", required=True)
    ap.add_argument("--label", default="run")
    ap.add_argument("--scripted", action="store_true")
    a = ap.parse_args()
    prods = {json.loads(l)["id"]: json.loads(l) for l in open(a.products) if l.strip()}
    prod = prods[a.id]
    row = asyncio.run(run_one(prod, a.scripted, a.label))
    with open(a.out, "a") as f:
        f.write(json.dumps(row, ensure_ascii=False) + "\n")
    print(f"{a.id}: {row['wall_s']}s, tools {[c['tool'] for c in row['calls']]}, ${row['cost_usd']:.4f}, error={row['error']}", file=sys.stderr)


if __name__ == "__main__":
    main()
