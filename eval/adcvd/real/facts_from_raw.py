"""Rebuilds the ruling facts the labellers see (D959), offline: for each id of QUERIES.jsonl the part of the CROSS ruling's text before its classification analysis, with every antidumping / countervailing duty paragraph removed (extract.py facts_of).

The committed copy for the held-out set is heldout/facts.jsonl.gz (gunzip -k it to read it with label.py); the raw rulings are not in the repository.

Usage: python3 -I -B eval/adcvd/real/facts_from_raw.py RAW.jsonl QUERIES.jsonl OUT.jsonl
"""
import json
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import extract  # noqa: E402

raw, qs, out = sys.argv[1:4]
want = {json.loads(l)["id"] for l in open(qs)}
n = 0
with open(out, "w") as f:
    for l in open(raw):
        r = json.loads(l)
        if r.get("n") in want and r.get("text"):
            f.write(json.dumps({"id": r["n"], "facts": extract.parse(r)["facts"]}, ensure_ascii=False) + "\n")
            n += 1
print("facts", n, "of", len(want))
