"""Scope-first reader for the AD/CVD exposure check (D944): a model reads an order's written scope against an importer's product description.

Usage (standard library only; gateway, LITELLM_* in the environment):
    python3 -I -B eval/adcvd/reader.py run PAIRS.jsonl OUT.jsonl [--model M] [--workers N]

PAIRS.jsonl is written by `go run ./cmd/adcvdeval -pairs`: one row per (ruling, order the check flagged) with
the key, the order's product and country, its written scope and the neutral product description. Each row is
asked once, at temperature 0, with the prompts of one version in scope-reader-prompts.json; rows already in OUT.jsonl (same key,
model and prompt version) are kept and not asked again. OUT.jsonl holds the verdict, the reason and the token
counts, and `adcvdeval -reader OUT.jsonl` reads it with no model call, so a score can be recomputed offline.
"""
import argparse
import concurrent.futures as cf
import json
import os
import sys
import time
import urllib.error
import urllib.request

HERE = os.path.dirname(os.path.abspath(__file__))


def chat(model, system, user, base, key, retries=3):
    body = json.dumps({"model": model, "temperature": 0, "response_format": {"type": "json_object"},
                       "messages": [{"role": "system", "content": system}, {"role": "user", "content": user}]}).encode()
    for i in range(retries):
        try:
            req = urllib.request.Request(base + "/v1/chat/completions", data=body,
                                         headers={"Content-Type": "application/json", "Authorization": "Bearer " + key})
            d = json.load(urllib.request.urlopen(req, timeout=75))
            return json.loads(d["choices"][0]["message"]["content"]), d.get("usage", {})
        except urllib.error.HTTPError as e:
            if e.code in (400, 401, 403, 404):
                raise RuntimeError(f"gateway refused the call ({e.code})")
            time.sleep(3 * (i + 1))
        except Exception:
            time.sleep(3 * (i + 1))
    raise RuntimeError("gateway call failed")


def run(pairs, out, model, workers, version=None):
    base, key = os.environ["LITELLM_BASE_URL"].rstrip("/"), os.environ["LITELLM_API_KEY"]
    allp = json.load(open(os.path.join(HERE, "scope-reader-prompts.json")))
    version = version or allp["current"]
    pr = dict(allp["versions"][version], version=version)
    model = model or "gemini-4-argon"
    rows = [json.loads(l) for l in open(pairs) if l.strip()]
    have = {}
    if os.path.exists(out):
        for l in open(out):
            r = json.loads(l)
            have[(r["key"], r["model"], r["prompt_version"])] = r
    todo = [r for r in rows if (r["key"], model, pr["version"]) not in have]
    tok = {"prompt": 0, "completion": 0, "calls": 0}

    failed = []

    def one(r):
        try:
            return one_(r)
        except Exception as e:  # a failed question is left unanswered and asked again by the next run
            failed.append(r["key"])
            return None

    def one_(r):
        user = pr["user"].format(product=r["product"], country=r["country"], scope=r["scope_text"], description=r["description"])
        a, u = chat(model, pr["system"], user, base, key)
        v = str(a.get("verdict", "")).strip().lower()
        if "p_within" in a:
            try:
                pw = max(0, min(100, int(float(a["p_within"]))))
            except (TypeError, ValueError):
                raise RuntimeError("no probability")
            return {"key": r["key"], "model": model, "prompt_version": pr["version"], "verdict": "in" if pw >= 50 else "out", "lean": "", "p_within": pw, "reason": a.get("reason", ""), "quote": a.get("quote", ""),
                    "prompt_tokens": u.get("prompt_tokens", 0), "completion_tokens": u.get("completion_tokens", 0)}
        if v not in ("in", "out", "unclear"):
            v = "unclear"
        lean = str(a.get("lean", "")).strip().lower()
        return {"key": r["key"], "model": model, "prompt_version": pr["version"], "verdict": v, "lean": lean if lean in ("in", "out") else "", "reason": a.get("reason", ""), "quote": a.get("quote", ""),
                "prompt_tokens": u.get("prompt_tokens", 0), "completion_tokens": u.get("completion_tokens", 0)}

    with open(out, "a") as f, cf.ThreadPoolExecutor(workers) as ex:
        for res in (fu.result() for fu in cf.as_completed([ex.submit(one, r) for r in todo])):
            if res is None:
                continue
            f.write(json.dumps(res, ensure_ascii=False) + "\n")
            f.flush()
            tok["prompt"] += res["prompt_tokens"]
            tok["completion"] += res["completion_tokens"]
            tok["calls"] += 1
    print(json.dumps({"failed": len(failed), "asked": len(todo), "cached": len(rows) - len(todo), "model": model, **tok}))


if __name__ == "__main__":
    ap = argparse.ArgumentParser()
    sub = ap.add_subparsers(dest="cmd", required=True)
    r = sub.add_parser("run")
    r.add_argument("pairs")
    r.add_argument("out")
    r.add_argument("--model", default=None)
    r.add_argument("--version", default=None, help="prompt version in scope-reader-prompts.json (default: current)")
    r.add_argument("--workers", type=int, default=6)
    a = ap.parse_args()
    run(a.pairs, a.out, a.model, a.workers, a.version)
