"""Scores a method on the realistic AD/CVD test (D945), offline: no model is called.

Usage:
    python3 -I -B eval/adcvd/real/score.py SPLIT_DIR --pairs PAIRS --reader READER.jsonl --model M --version V [--policy P] [--k 12] [--out SUMMARY.json] [--check]

SPLIT_DIR holds queries.jsonl (one row per ruling: id, origin, hts, note, note_cases, ...) and truth.jsonl (one row per ruling: id, positive,
truth_groups, source). READER.jsonl holds the recorded answers of the method's reader (key, model, prompt_version, verdict, lean).
A ruling is a positive when CBP's own ruling warns that the product may be subject to an AD or CVD order, or a panel found the product
within the written scope of at least one active order. The method flags a ruling when it flags at least one order. A flag is right when
the ruling is a positive and, if the orders it is within are known, the flagged orders include one of them.
"""
import argparse
import json
import math
import os
import sys


def wilson(k, n):
    if n == 0:
        return {"k": k, "n": n, "p": None, "lo": None, "hi": None}
    z = 1.959964
    p = k / n
    d = 1 + z * z / n
    c = p + z * z / (2 * n)
    h = z * math.sqrt(p * (1 - p) / n + z * z / (4 * n * n))
    r = lambda x: round(x, 6)
    return {"k": k, "n": n, "p": r(p), "lo": r(max(0, (c - h) / d)), "hi": r(min(1, (c + h) / d))}


def load(path):
    return [json.loads(l) for l in open(path) if l.strip()]


POLICIES = {
    "in": lambda v: v == "in",
    "in+unclear": lambda v: v in ("in", "unclear", "unclear_in", "unclear_out"),
    "in+lean_in": lambda v: v in ("in", "unclear_in"),
}


def reader_answers(path, model, version):
    ans = {}
    for a in load(path):
        if (not model or a["model"] == model) and (not version or a["prompt_version"] == version):
            v = a["verdict"]
            if v == "unclear" and a.get("lean"):
                v = "unclear_" + a["lean"]
            ans[a["key"]] = v
    return ans


def score(split, pairs_path, flagfn, k, include_hts=False, screened=None):
    """flagfn(pair) -> True when the method flags the pair. Only pairs in the retrieval's top k (and, if include_hts, the plain check's) are asked;
    with `screened` (a map of ruling id to the keys a name screen let through) only those keys are, plus the plain check's when include_hts."""
    qs = {q["id"]: q for q in load(os.path.join(split, "queries.jsonl"))}
    truth = {t["id"]: t for t in load(os.path.join(split, "truth.jsonl"))}
    by = {}
    for p in load(pairs_path):
        if p["id"] not in qs:
            continue
        top = 1 <= p["rank"] <= k
        if screened is not None:
            top = top and p["key"] in screened.get(p["id"], ())
        if top or (include_hts and p["hts_match"]):
            by.setdefault(p["id"], []).append(p)
    TP = FP = FN = TN = 0
    detail = []
    miss = 0
    for i, t in truth.items():
        flagged = set()
        for p in by.get(i, []):
            f = flagfn(p)
            if f is None:
                miss += 1
                continue
            if f:
                flagged.update(p["cases"])
        pos = t["positive"]
        known = {c for g in t.get("truth_groups", []) for c in g.split()}
        fl = bool(flagged)
        right = fl and pos and (not known or bool(flagged & known))
        if right:
            TP += 1
        elif fl:
            FP += 1
            if pos:
                FN += 1
        elif pos:
            FN += 1
        else:
            TN += 1
        detail.append({"id": i, "positive": pos, "flagged": sorted(flagged), "right": right})
    out = {"rulings": len(truth), "positives": sum(1 for t in truth.values() if t["positive"]), "TP": TP, "FP": FP, "FN": FN, "TN": TN,
           "precision": wilson(TP, TP + FP), "recall": wilson(TP, TP + FN), "specificity": wilson(TN, TN + FP), "answers_missing": miss}
    return out, detail


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("split")
    ap.add_argument("--pairs", required=True)
    ap.add_argument("--reader")
    ap.add_argument("--model", default="")
    ap.add_argument("--version", default="")
    ap.add_argument("--policy", default="in+lean_in")
    ap.add_argument("--k", type=int, default=12)
    ap.add_argument("--screen", help="screen answers file; only pairs the screen model let through are read")
    ap.add_argument("--screen-model", default="")
    ap.add_argument("--hts", action="store_true", help="also read the pairs the plain HTS-and-country check matches")
    ap.add_argument("--out")
    a = ap.parse_args()
    if a.policy == "retrieve":
        fl = lambda p: True
    else:
        ans = reader_answers(a.reader, a.model, a.version)
        fl = lambda p: (POLICIES[a.policy](ans[p["key"]]) if p["key"] in ans else None)
    screened = None
    if a.screen:
        screened = {}
        for r in load(a.screen):
            if not a.screen_model or r["model"] == a.screen_model:
                screened.setdefault(r["id"], set()).update(r["possible"])
    out, detail = score(a.split, a.pairs, fl, a.k, a.hts, screened)
    print(json.dumps(out, indent=1))
    if a.out:
        json.dump({"summary": out, "detail": detail}, open(a.out, "w"), indent=1)


if __name__ == "__main__":
    main()
