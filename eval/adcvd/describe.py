"""Importer-style product descriptions for scope rulings (D944): the product as an importer states it, without the ruling's conclusion.

Usage (standard library only; gateway, LITELLM_* in the environment):
    python3 -I -B eval/adcvd/describe.py run RULINGS.jsonl OUT.jsonl [--workers N]

One call per ruling reads the whole entry and writes the description (prompt in describe-prompts.json), a second call audits the
description for leakage of the ruling's conclusion. A flagged description is written again once with a note naming the leak; the row
keeps both attempts. Rows already in OUT.jsonl are kept. Mechanical checks (outcome words, order words, copied scope runs) are
done by `check`, with no model.
"""
import argparse
import concurrent.futures as cf
import json
import os
import re
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from rulings import chat  # noqa: E402

HERE = os.path.dirname(os.path.abspath(__file__))
WORDS = re.compile(r"\b(within the scope|outside the scope|out of scope|in scope|scope|covered|subject merchandise|subject to the order|excluded|exclusion|antidumping|countervailing|the order|commerce)\b", re.I)


def run(rulings, out, workers):
    base, key = os.environ["LITELLM_BASE_URL"].rstrip("/"), os.environ["LITELLM_API_KEY"]
    pr = json.load(open(os.path.join(HERE, "describe-prompts.json")))
    rows = [json.loads(l) for l in open(rulings) if l.strip()]
    have = {json.loads(l)["id"] for l in open(out)} if os.path.exists(out) else set()
    tok = {"prompt": 0, "completion": 0, "calls": 0}

    def one(r):
        t = {"prompt": 0, "completion": 0, "calls": 0}
        attempts = []
        extra = ""
        for _ in range(2):
            a, u = chat(pr["model"], pr["system"] + extra, pr["user"].format(text=r["text"]), base, key)
            d = str(a.get("description", "")).strip()
            b, u2 = chat(pr["model"], pr["audit_system"], pr["audit_user"].format(description=d, text=r["text"]), base, key)
            for x in (u, u2):
                t["prompt"] += x.get("prompt_tokens", 0)
                t["completion"] += x.get("completion_tokens", 0)
            t["calls"] += 2
            leak = bool(b.get("leak")) or bool(WORDS.search(d)) or not d
            attempts.append({"description": d, "model_leak": bool(b.get("leak")), "why": b.get("why", ""), "word_leak": bool(WORDS.search(d))})
            if not leak:
                break
            extra = "\nYour previous attempt was rejected because it leaked: " + str(b.get("why", "")) + ". Previous attempt: " + d + ". Write it again without that."
        return {"id": r["id"], "description": attempts[-1]["description"], "attempts": attempts, "prompt_version": pr["version"], "model": pr["model"]}, t

    todo = [r for r in rows if r["id"] not in have]
    with open(out, "a") as f, cf.ThreadPoolExecutor(workers) as ex:
        for res, t in ex.map(one, todo):
            f.write(json.dumps(res, ensure_ascii=False) + "\n")
            f.flush()
            for k in tok:
                tok[k] += t[k]
    print(json.dumps({"asked": len(todo), "cached": len(rows) - len(todo), **tok}))


if __name__ == "__main__":
    ap = argparse.ArgumentParser()
    sub = ap.add_subparsers(dest="cmd", required=True)
    r = sub.add_parser("run")
    r.add_argument("rulings")
    r.add_argument("out")
    r.add_argument("--workers", type=int, default=6)
    a = ap.parse_args()
    run(a.rulings, a.out, a.workers)
