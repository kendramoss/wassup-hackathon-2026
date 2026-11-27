"""Panel labels for the realistic AD/CVD test (D945): models read one order's whole written scope against a product and say in, out or unclear.

Usage (standard library only; gateway, LITELLM_* in the environment):
    python3 -I -B eval/adcvd/real/label.py run PAIRS.jsonl FACTS.jsonl OUT.jsonl --model M [--workers N] [--only KEYS.jsonl]

PAIRS.jsonl is written by `go run ./cmd/adcvdreal`; FACTS.jsonl holds, for each ruling id, the facts of the ruling the labellers may use (the
ruling's own words with every antidumping and countervailing duty paragraph removed). A labeller sees the importer description AND the
facts, which is more than the method under test is given. One call per (pair key, model, prompt version) at temperature 0; answers
already in OUT.jsonl are kept. Disagreements are sent to the adjudicator with `--adjudicate`, which also shows both labellers' reasons.
"""
import argparse
import concurrent.futures as cf
import json
import os
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.dirname(HERE))
import re  # noqa: E402
import time  # noqa: E402
import urllib.error  # noqa: E402
import urllib.request  # noqa: E402


def parse_json(text):
    """The first JSON object in a model's reply, with or without a code fence around it."""
    text = (text or "").strip()
    try:
        return json.loads(text)
    except ValueError:
        pass
    m = re.search(r"\{.*\}", text, re.S)
    if m:
        return json.loads(m.group(0))
    raise ValueError("no JSON")


def chat(model, system, user, base, key, retries=4):
    body = json.dumps({"model": model, "temperature": 0, "response_format": {"type": "json_object"},
                       "messages": [{"role": "system", "content": system}, {"role": "user", "content": user}]}).encode()
    for i in range(retries):
        try:
            req = urllib.request.Request(base + "/v1/chat/completions", data=body, headers={"Content-Type": "application/json", "Authorization": "Bearer " + key})
            d = json.load(urllib.request.urlopen(req, timeout=120))
            return parse_json(d["choices"][0]["message"]["content"]), d.get("usage", {})
        except urllib.error.HTTPError as e:
            if e.code in (400, 401, 403, 404):
                raise RuntimeError(f"gateway refused the call ({e.code})")
            time.sleep(3 * (i + 1))
        except Exception:
            time.sleep(3 * (i + 1))
    raise RuntimeError("gateway call failed")

VERSION = "adcvd-real-label-v1"
SYSTEM = ("You decide whether one product is within the written scope of one U.S. antidumping (AD) or countervailing duty (CVD) order, the way the U.S. Department of Commerce reads a scope: by the words of the scope, read as a whole, including every exclusion, qualification and definition in it. The HTS numbers a scope lists are for convenience and do not decide scope. "
          "You are given the order's name and country, its whole written scope, a short description of the product and fuller facts about it from a customs ruling. Use any of them for the product's facts.\n"
          "Work in this order: (1) what merchandise the scope describes; (2) whether the product is that kind of merchandise, and made in the order's country; (3) whether any exclusion, limit or qualification in the scope takes the product out.\n"
          "Answer with a JSON object with exactly these keys: \"verdict\", \"quote\" and \"reason\".\n"
          "\"verdict\": \"in\" if the scope's words cover the product and no exclusion removes it; \"out\" if the scope describes a different kind of merchandise or an exclusion covers the product; \"unclear\" only if the facts given really do not settle it. Do not use \"unclear\" for a close case: decide it on the words.\n"
          "\"quote\": the exact sentence or clause of the scope you relied on, copied word for word.\n"
          "\"reason\": one sentence.\nAnswer with the JSON object only.")
USER = "Order: {product} from {country}\n\nWritten scope of the order:\n{scope}\n\nProduct (made in {country}): {description}\n\nFuller facts about the product, from a customs ruling:\n{facts}"
ADJ_SYSTEM = SYSTEM + "\nTwo earlier readers disagreed or were unsure about this question; their answers and reasons follow. Decide for yourself from the scope's words; you must answer \"in\" or \"out\", not \"unclear\"."
ADJ_USER = USER + "\n\nEarlier reader 1: {a_verdict}. {a_reason}\nEarlier reader 2: {b_verdict}. {b_reason}"


SCREEN_VERSION = "adcvd-real-screen-v1"
SCREEN_SYSTEM = ("You screen which U.S. antidumping (AD) or countervailing duty (CVD) orders could plausibly cover one product, before anyone reads the full scopes. "
                 "You are given a short description of a product made in one country and a numbered list of orders on that country with each order's name and the start of its written scope. "
                 "Answer with a JSON object with exactly one key, \"possible\": the list of numbers of every order whose merchandise could plausibly include the product, or be close enough that someone must read its whole scope to be sure. "
                 "Be generous: include an order when in doubt, leave out an order only when the product is plainly a different kind of merchandise. An empty list is a fine answer.")
SCREEN_USER = "Product (made in {country}): {description}\n{facts}\nOrders:\n{orders}"


def screen(pairs, facts, out, model, workers, no_facts=False):
    base, key = os.environ["LITELLM_BASE_URL"].rstrip("/"), os.environ["LITELLM_API_KEY"]
    fx = {json.loads(l)["id"]: ("" if no_facts else json.loads(l)["facts"]) for l in open(facts)}
    by = {}
    for l in open(pairs):
        p = json.loads(l)
        by.setdefault(p["id"], []).append(p)
    have = set()
    if os.path.exists(out):
        for l in open(out):
            r = json.loads(l)
            have.add((r["id"], r["model"], r["prompt_version"]))
    todo = [i for i in sorted(by) if (i, model, SCREEN_VERSION) not in have]
    tok = {"prompt": 0, "completion": 0, "calls": 0}

    def one(i):
        ps = by[i]
        lines = []
        for n, p in enumerate(ps, 1):
            lines.append("%d. %s (%s)%s\n   %s" % (n, p["product"], " / ".join(p["cases"]), " [its listed HTS numbers include the product's code]" if p["hts_match"] else "", " ".join(p["scope_text"].split())[:400]))
        user = SCREEN_USER.format(country=ps[0]["country"], description=ps[0]["description"], facts=("\nFuller facts from a customs ruling:\n" + fx.get(i, "")[:1500] + "\n") if fx.get(i) else "", orders="\n".join(lines))
        try:
            a, u = chat(model, SCREEN_SYSTEM, user, base, key)
        except Exception:
            return None, {}
        idx = a.get("possible", [])
        keys = []
        for x in idx if isinstance(idx, list) else []:
            try:
                n = int(x)
            except (TypeError, ValueError):
                continue
            if 1 <= n <= len(ps):
                keys.append(ps[n - 1]["key"])
        return {"id": i, "model": model, "prompt_version": SCREEN_VERSION, "possible": sorted(set(keys)), "asked": len(ps)}, u

    failed = 0
    with open(out, "a") as f, cf.ThreadPoolExecutor(workers) as ex:
        for r, u in (fu.result() for fu in cf.as_completed([ex.submit(one, x) for x in todo])):
            if r is None:
                failed += 1
                continue
            f.write(json.dumps(r) + "\n")
            f.flush()
            tok["prompt"] += u.get("prompt_tokens", 0)
            tok["completion"] += u.get("completion_tokens", 0)
            tok["calls"] += 1
    print(json.dumps({"asked": len(todo), "failed": failed, "model": model, **tok}))


def clip_scope(s, n=24000):
    return s if len(s) <= n else s[:n] + "\n[scope text continues]"


def run(pairs, facts, out, model, workers, only, adjudicate, version=None):
    version = version or VERSION
    base, key = os.environ["LITELLM_BASE_URL"].rstrip("/"), os.environ["LITELLM_API_KEY"]
    fx = {}
    for l in open(facts):
        r = json.loads(l)
        fx[r["id"]] = r["facts"]
    rows, seen = [], set()
    for l in open(pairs):
        p = json.loads(l)
        k = (p["key"], p["id"])
        if k in seen:
            continue
        seen.add(k)
        rows.append(p)
    # the same question asked for two rulings has two sets of facts: the key is of the description, so keep one per (key, id)
    have = {}
    if os.path.exists(out):
        for l in open(out):
            r = json.loads(l)
            have[(r["key"], r["id"], r["model"], r["prompt_version"])] = r
    keys = None
    if only:
        keys = {(json.loads(l)["key"], json.loads(l)["id"]) for l in open(only)}
    todo = [p for p in rows if (keys is None or (p["key"], p["id"]) in keys) and (p["key"], p["id"], model, version) not in have]
    tok = {"prompt": 0, "completion": 0, "calls": 0}

    def one(p):
        d = dict(product=p["product"], country=p["country"], scope=clip_scope(p["scope_text"]), description=p["description"], facts=fx.get(p["id"], "")[:2500])
        sysm, user = SYSTEM, USER.format(**d)
        if adjudicate:
            sysm, user = ADJ_SYSTEM, ADJ_USER.format(**d, **p["_adj"])
        try:
            a, u = chat(model, sysm, user, base, key)
        except Exception:
            return None, {}
        v = str(a.get("verdict", "")).strip().lower()
        if v not in ("in", "out", "unclear"):
            v = "unclear"
        return {"key": p["key"], "id": p["id"], "model": model, "prompt_version": version, "verdict": v, "reason": str(a.get("reason", ""))[:400], "quote": str(a.get("quote", ""))[:400]}, u

    if adjudicate:
        ans = {}
        for (k, i, m, v), r in have.items():
            ans.setdefault((k, i), {})[m] = r
        pm = adjudicate.split(",")
        for p in todo:
            a = ans.get((p["key"], p["id"]), {})
            p["_adj"] = {"a_verdict": a.get(pm[0], {}).get("verdict", ""), "a_reason": a.get(pm[0], {}).get("reason", ""), "b_verdict": a.get(pm[1], {}).get("verdict", ""), "b_reason": a.get(pm[1], {}).get("reason", "")}
    failed = 0
    with open(out, "a") as f, cf.ThreadPoolExecutor(workers) as ex:
        for r, u in (fu.result() for fu in cf.as_completed([ex.submit(one, x) for x in todo])):
            if r is None:
                failed += 1
                continue
            f.write(json.dumps(r, ensure_ascii=False) + "\n")
            f.flush()
            tok["prompt"] += u.get("prompt_tokens", 0)
            tok["completion"] += u.get("completion_tokens", 0)
            tok["calls"] += 1
    print(json.dumps({"asked": len(todo), "cached": len(rows) - len(todo), "failed": failed, "model": model, **tok}))


if __name__ == "__main__":
    ap = argparse.ArgumentParser()
    sub = ap.add_subparsers(dest="cmd", required=True)
    r = sub.add_parser("run")
    r.add_argument("pairs")
    r.add_argument("facts")
    r.add_argument("out")
    r.add_argument("--model", required=True)
    r.add_argument("--workers", type=int, default=8)
    r.add_argument("--only", default=None)
    r.add_argument("--version", default=None, help="prompt_version recorded and used as the cache key (default adcvd-real-label-v1)")
    r.add_argument("--adjudicate", default=None, help="MODEL_A,MODEL_B: show the two earlier answers; answers are read from OUT")
    sc = sub.add_parser("screen")
    sc.add_argument("pairs")
    sc.add_argument("facts")
    sc.add_argument("out")
    sc.add_argument("--model", required=True)
    sc.add_argument("--workers", type=int, default=8)
    sc.add_argument("--no-facts", action="store_true", help="the method's own screen: the description only, no ruling facts")
    a = ap.parse_args()
    if a.cmd == "screen":
        screen(a.pairs, a.facts, a.out, a.model, a.workers, a.no_facts)
    else:
        run(a.pairs, a.facts, a.out, a.model, a.workers, a.only, a.adjudicate, a.version)
