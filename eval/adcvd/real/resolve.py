"""Second look at the questions the first reader called unclear (D945): a model gives the probability that the product, as described, is within the scope.

Usage (standard library only; gateway, LITELLM_* in the environment):
    python3 -I -B eval/adcvd/real/resolve.py run READS.jsonl FIRST_READER.jsonl OUT.jsonl --model M [--first-model M0 --first-version V0] [--workers N]

Asks only the questions whose first answer (FIRST_READER) is `unclear`; one call per (key, model, prompt version) at temperature 0; answers already
in OUT.jsonl are kept. The description is all the model knows of the product (no ruling facts).
"""
import argparse
import concurrent.futures as cf
import json
import os
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
from label import chat, clip_scope  # noqa: E402

VERSION = "adcvd-real-resolve-v1"
SYSTEM = ("You estimate whether a product that an importer describes is within the written scope of one U.S. antidumping (AD) or countervailing duty (CVD) order, as the U.S. Department of Commerce reads a scope: by the words of the scope read as a whole, including every exclusion, qualification and definition. The HTS numbers a scope lists are for convenience and do not decide scope. "
          "The description is all you know about the product, and it will often not state every attribute the scope mentions. Where a condition of the scope (a size, a grade, a specification, a use) is not stated, judge by what is ordinary for the kind of product described and by how the description names it: a product that the scope's own words name or plainly include as a kind is probably within the scope even when some details are not stated; a product that is a different kind of merchandise, or that an exclusion names, is outside it.\n"
          "Answer with a JSON object with exactly these keys: \"p_within\" (an integer from 0 to 100: your probability that Commerce would find the product within the scope), \"quote\" (the exact sentence or clause of the scope, copied word for word, that matters most) and \"reason\" (one sentence).\nAnswer with the JSON object only.")
USER = "Order: {product} from {country}\n\nWritten scope of the order:\n{scope}\n\nProduct (made in {country}): {description}"


def run(reads, first, out, model, workers, fmodel, fversion):
    base, key = os.environ["LITELLM_BASE_URL"].rstrip("/"), os.environ["LITELLM_API_KEY"]
    fa = {}
    for l in open(first):
        r = json.loads(l)
        if (not fmodel or r["model"] == fmodel) and (not fversion or r["prompt_version"] == fversion):
            fa[r["key"]] = r["verdict"]
    rows = [json.loads(l) for l in open(reads)]
    have = set()
    if os.path.exists(out):
        for l in open(out):
            r = json.loads(l)
            have.add((r["key"], r["model"], r["prompt_version"]))
    todo = [r for r in rows if fa.get(r["key"]) == "unclear" and (r["key"], model, VERSION) not in have]
    tok = {"prompt": 0, "completion": 0, "calls": 0}

    def one(r):
        try:
            a, u = chat(model, SYSTEM, USER.format(product=r["product"], country=r["country"], scope=clip_scope(r["scope_text"]), description=r["description"]), base, key)
            pw = max(0, min(100, int(float(a["p_within"]))))
        except Exception:
            return None, {}
        return {"key": r["key"], "model": model, "prompt_version": VERSION, "p_within": pw, "reason": str(a.get("reason", ""))[:300]}, u

    failed = 0
    with open(out, "a") as f, cf.ThreadPoolExecutor(workers) as ex:
        for res, u in (fu.result() for fu in cf.as_completed([ex.submit(one, r) for r in todo])):
            if res is None:
                failed += 1
                continue
            f.write(json.dumps(res) + "\n")
            f.flush()
            tok["prompt"] += u.get("prompt_tokens", 0)
            tok["completion"] += u.get("completion_tokens", 0)
            tok["calls"] += 1
    print(json.dumps({"asked": len(todo), "failed": failed, "model": model, **tok}))


if __name__ == "__main__":
    ap = argparse.ArgumentParser()
    sub = ap.add_subparsers(dest="cmd", required=True)
    r = sub.add_parser("run")
    r.add_argument("reads")
    r.add_argument("first")
    r.add_argument("out")
    r.add_argument("--model", required=True)
    r.add_argument("--first-model", default="gemini-4-argon")
    r.add_argument("--first-version", default="adcvd-scope-v4")
    r.add_argument("--workers", type=int, default=8)
    a = ap.parse_args()
    run(a.reads, a.first, a.out, a.model, a.workers, a.first_model, a.first_version)
