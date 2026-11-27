"""Lists the (key, id) pairs on which the two labellers do not agree on in or out (D945), for the adjudicator.

Usage: python3 -I -B eval/adcvd/real/disagree.py SPLIT_DIR MODEL_A MODEL_B   (reads labels.jsonl, writes disagree.jsonl)
"""
import json
import os
import sys

d, ma, mb = sys.argv[1:4]
ans = {}
for l in open(os.path.join(d, "labels.jsonl")):
    r = json.loads(l)
    ans.setdefault((r["key"], r["id"]), {})[r["model"]] = r["verdict"]
n = tot = 0
read = [json.loads(l) for l in open(os.path.join(d, "stage2.jsonl"))]
with open(os.path.join(d, "disagree.jsonl"), "w") as f:
    for r in read:
        a = ans.get((r["key"], r["id"]), {})
        tot += 1
        if not (a.get(ma) in ("in", "out") and a.get(ma) == a.get(mb)):
            f.write(json.dumps({"key": r["key"], "id": r["id"]}) + "\n")
            n += 1
print("read in full", tot, "disagree or unclear", n)
