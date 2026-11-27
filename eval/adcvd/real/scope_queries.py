"""Turns a set of Commerce scope rulings (D940 to D944) into the files the realistic-test tools read (D945), offline.

Usage: python3 -I -B eval/adcvd/real/scope_queries.py SET_DIR OUTDIR
SET_DIR holds rulings.jsonl, labels.jsonl and descriptions.jsonl (or pass --desc). A ruling found within or outside its order's scope is one query: the
importer description, the order's country as the origin, and the model's HTS code. Truth: positive when found within, with the ruling's own case numbers
as the orders known to be within.
"""
import argparse
import json
import os

ap = argparse.ArgumentParser()
ap.add_argument("set_dir")
ap.add_argument("out")
ap.add_argument("--desc")
a = ap.parse_args()
ld = lambda p: [json.loads(l) for l in open(p) if l.strip()]
rul = {r["id"]: r for r in ld(os.path.join(a.set_dir, "rulings.jsonl"))}
lab = {r["id"]: r for r in ld(os.path.join(a.set_dir, "labels.jsonl"))}
des = {r["id"]: r for r in ld(a.desc or os.path.join(a.set_dir, "descriptions.jsonl"))}
os.makedirs(a.out, exist_ok=True)
n = 0
with open(os.path.join(a.out, "queries.jsonl"), "w") as fq, open(os.path.join(a.out, "truth.jsonl"), "w") as ft, open(os.path.join(a.out, "facts.jsonl"), "w") as ff:
    for i in sorted(rul):
        l = lab.get(i)
        d = des.get(i)
        if not l or l["outcome"] not in ("within", "outside") or not d or not d["description"].strip():
            continue
        r = rul[i]
        n += 1
        fq.write(json.dumps({"id": i, "hts": l.get("hts", ""), "origin": r["order_country"], "description": d["description"], "in_sample": True}, ensure_ascii=False) + "\n")
        ft.write(json.dumps({"id": i, "positive": l["outcome"] == "within", "truth_groups": [" ".join(r["cases"])] if l["outcome"] == "within" else [], "source": "commerce_ruling"}) + "\n")
        ff.write(json.dumps({"id": i, "facts": ""}) + "\n")
print("queries", n)
