"""Builds the truth of the realistic AD/CVD test (D945) from the panel's answers and CBP's own warnings, offline.

Usage:
    python3 -I -B eval/adcvd/real/truth.py SPLIT_DIR --pairs PAIRS --labels LABELS.jsonl --a MODEL_A --b MODEL_B --adj MODEL_ADJ

Per pair of the pool (a product and an order group): two labellers answer in, out or unclear; when they agree on in or out that is the label,
otherwise the adjudicator's answer (in or out; unclear counts as out). Per ruling: it is a positive when at least one pair is labelled in, or when CBP's own ruling names an order by case number as one the product may be
subject to and that order is in the pool (queries.jsonl `note_cases`). CBP's other warnings (`note`) are not labels: they are counted against the panel as a check. `truth_groups` are the order
groups known to be within: the labelled-in pairs, and the groups holding a case number CBP's warning names. Writes truth.jsonl and
truth-summary.json in SPLIT_DIR.
"""
import argparse
import json
import os
from collections import Counter


def load(p):
    return [json.loads(l) for l in open(p) if l.strip()]


def final_label(a, b, adj):
    if a in ("in", "out") and a == b:
        return a, "agree"
    return ("in" if adj == "in" else "out"), "adjudicated"


def build(split, pairs, labels, ma, mb, madj):
    qs = load(os.path.join(split, "queries.jsonl"))
    read = {(r["key"], r["id"]) for r in load(os.path.join(split, "stage2.jsonl"))}
    ans = {}
    for r in load(labels):
        ans.setdefault((r["key"], r["id"]), {})[r["model"]] = r["verdict"]
    by = {}
    for p in load(pairs):
        by.setdefault(p["id"], []).append(p)
    rows, stats = [], Counter()
    for q in qs:
        ins, via = [], Counter()
        for p in by.get(q["id"], []):
            a = ans.get((p["key"], p["id"]), {})
            if (p["key"], p["id"]) not in read:
                via["screened_out"] += 1
                stats["pairs_screened_out"] += 1
                continue
            lab, how = final_label(a.get(ma, ""), a.get(mb, ""), a.get(madj, ""))
            via[how] += 1
            stats["pairs_" + how] += 1
            if lab == "in":
                ins.append(" ".join(p["cases"]))
        note_groups = []
        for p in by.get(q["id"], []):
            if any(c in q.get("note_cases", []) for c in p["cases"]):
                note_groups.append(" ".join(p["cases"]))
        note = bool(q.get("note"))
        cited = bool(note_groups)  # CBP names an order by case number and it is in the pool: that is a label of its own
        pos = bool(ins) or cited
        src = "panel+cbp_case" if (ins and cited) else "cbp_case" if cited else "panel" if ins else "none"
        stats["cbp_warned_" + ("panel_in" if ins else "panel_none")] += 1 if note else 0
        stats["rulings_" + src] += 1
        if ins and not note:
            stats["panel_in_without_cbp_warning"] += 1
        rows.append({"id": q["id"], "positive": pos, "source": src, "truth_groups": sorted(set(ins) | set(note_groups)), "pairs": sum(via.values()), "in_sample": q.get("in_sample", True)})
    with open(os.path.join(split, "truth.jsonl"), "w") as f:
        for r in rows:
            f.write(json.dumps(r) + "\n")
    summ = {"rulings": len(rows), "positives": sum(1 for r in rows if r["positive"]), **dict(stats)}
    json.dump(summ, open(os.path.join(split, "truth-summary.json"), "w"), indent=1)
    print(json.dumps(summ, indent=1))


if __name__ == "__main__":
    ap = argparse.ArgumentParser()
    ap.add_argument("split")
    ap.add_argument("--pairs", required=True)
    ap.add_argument("--labels", required=True)
    ap.add_argument("--a", required=True)
    ap.add_argument("--b", required=True)
    ap.add_argument("--adj", required=True)
    a = ap.parse_args()
    build(a.split, a.pairs, a.labels, a.a, a.b, a.adj)
