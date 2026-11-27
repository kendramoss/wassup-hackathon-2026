"""Lists the (key, id) pairs the panel reads in full: those either screener marked possible, and every pair the plain HTS-and-country check matches (D945).

Usage: python3 -I -B eval/adcvd/real/stage2.py SPLIT_DIR   (reads pairs.jsonl and screen.jsonl, writes stage2.jsonl)
"""
import json
import os
import sys

d = sys.argv[1]
pairs = [json.loads(l) for l in open(os.path.join(d, "pairs.jsonl"))]
poss = {}
for l in open(os.path.join(d, "screen.jsonl")):
    r = json.loads(l)
    poss.setdefault(r["id"], set()).update(r["possible"])
n = 0
seen = set()
with open(os.path.join(d, "stage2.jsonl"), "w") as f:
    for p in pairs:
        k = (p["key"], p["id"])
        if k in seen:
            continue
        seen.add(k)
        if p["hts_match"] or p["key"] in poss.get(p["id"], ()):
            f.write(json.dumps({"key": p["key"], "id": p["id"]}) + "\n")
            n += 1
print("pairs", len(seen), "read in full", n)
