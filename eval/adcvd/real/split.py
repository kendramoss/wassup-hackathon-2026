"""Chooses the rulings of one split of the realistic AD/CVD test (D945), offline.

Usage:
    python3 -I -B eval/adcvd/real/split.py EXTRACT.jsonl ORIGINS_OK.txt OUTDIR --from 2022-01-01 --to 2023-12-31 --n 1000 --seed 945

The universe is every product ruling in the date range whose origin is a country with an active order and that has a description and an HTS
code. A uniform random sample of n (seeded, ids sorted first) is the test; every other ruling in the universe that carries a CBP warning is
added and marked in_sample false, for the secondary figure on CBP's own warnings. Writes queries.jsonl and facts.jsonl.
"""
import argparse
import json
import os
import random


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("extract")
    ap.add_argument("origins")
    ap.add_argument("out")
    ap.add_argument("--from", dest="lo", required=True)
    ap.add_argument("--to", dest="hi", required=True)
    ap.add_argument("--n", type=int, required=True)
    ap.add_argument("--seed", type=int, required=True)
    a = ap.parse_args()
    ok = {l for l in open(a.origins).read().split("\n") if l}
    rows = {}
    for l in open(a.extract):
        r = json.loads(l)
        rows[r["id"]] = r
    uni = sorted(i for i, r in rows.items() if a.lo <= r["date"] <= a.hi and r["product_ruling"] and r["origin"] in ok and r["description"].strip() and r["hts"])
    pick = set(random.Random(a.seed).sample(uni, min(a.n, len(uni))))
    os.makedirs(a.out, exist_ok=True)
    nq = 0
    with open(os.path.join(a.out, "queries.jsonl"), "w") as fq, open(os.path.join(a.out, "facts.jsonl"), "w") as ff:
        for i in uni:
            r = rows[i]
            if i not in pick and not r["note"]:
                continue
            nq += 1
            fq.write(json.dumps({"id": i, "date": r["date"], "hts": r["hts"][0], "origin": r["origin"], "description": r["description"], "note": r["note"], "note_cases": r["note_cases"],
                                 "in_sample": i in pick}, ensure_ascii=False) + "\n")
            ff.write(json.dumps({"id": i, "facts": r["facts"]}, ensure_ascii=False) + "\n")
    print(json.dumps({"universe": len(uni), "sample": len(pick), "queries": nq, "notes_in_universe": sum(1 for i in uni if rows[i]["note"]), "notes_in_sample": sum(1 for i in pick if rows[i]["note"])}))


if __name__ == "__main__":
    main()
