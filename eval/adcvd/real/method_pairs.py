"""The questions the method asks its reader (D945): the pairs of a split that the method's name screen lets through, unique by key.

Usage: python3 -I -B eval/adcvd/real/method_pairs.py SPLIT_DIR OUT.jsonl --screen FILE --screen-model M [--k 12] [--hts]
The reader (eval/adcvd/scope_reader.py) is then run on OUT.jsonl; `score.py` reads its answers offline.
"""
import argparse
import json
import os

ap = argparse.ArgumentParser()
ap.add_argument("split")
ap.add_argument("out")
ap.add_argument("--screen", required=True)
ap.add_argument("--screen-model", required=True)
ap.add_argument("--k", type=int, default=12)
ap.add_argument("--hts", action="store_true")
a = ap.parse_args()
scr = {}
for l in open(a.screen):
    r = json.loads(l)
    if r["model"] == a.screen_model:
        scr.setdefault(r["id"], set()).update(r["possible"])
seen, n = set(), 0
with open(a.out, "w") as f:
    for l in open(os.path.join(a.split, "pairs.jsonl")):
        p = json.loads(l)
        ok = (1 <= p["rank"] <= a.k and p["key"] in scr.get(p["id"], ())) or (a.hts and p["hts_match"])
        if ok and p["key"] not in seen:
            seen.add(p["key"])
            f.write(json.dumps({"key": p["key"], "id": p["id"], "cases": " ".join(p["cases"]), "product": p["product"], "country": p["country"], "description": p["description"], "scope_text": p["scope_text"]}) + "\n")
            n += 1
print("questions", n)
