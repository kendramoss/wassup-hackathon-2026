"""Writes golden-screen.jsonl (D959): the name-screen prompt label.py builds for 40 development queries, for the Go test that checks the production check builds the same text.

Usage: python3 -I -B eval/adcvd/real/screen_golden.py   (reads dev/pairs.jsonl and dev/queries.jsonl; the 40 queries with the most pool orders)
"""
import json
import os
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
sys.path.insert(0, os.path.dirname(HERE))
import label  # noqa: E402

by = {}
for l in open(os.path.join(HERE, "dev/pairs.jsonl")):
    p = json.loads(l)
    by.setdefault(p["id"], []).append(p)
qs = {json.loads(l)["id"]: json.loads(l) for l in open(os.path.join(HERE, "dev/queries.jsonl"))}
ids = sorted(by, key=lambda i: (-len(by[i]), i))[:40]
with open(os.path.join(HERE, "golden-screen.jsonl"), "w") as f:
    for i in sorted(ids):
        ps = by[i]
        lines = []
        for n, p in enumerate(ps, 1):
            lines.append("%d. %s (%s)%s\n   %s" % (n, p["product"], " / ".join(p["cases"]), " [its listed HTS numbers include the product's code]" if p["hts_match"] else "", " ".join(p["scope_text"].split())[:400]))
        user = label.SCREEN_USER.format(country=ps[0]["country"], description=ps[0]["description"], facts="", orders="\n".join(lines))
        q = qs[i]
        f.write(json.dumps({"id": i, "description": q["description"], "origin": q["origin"], "hts": q["hts"], "user": user, "reader_key_first": ps[0]["key"]}, ensure_ascii=False) + "\n")
print("golden", len(ids))
