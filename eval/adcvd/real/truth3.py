"""Truth of the realistic AD/CVD test by three independent labellers, with "determinable" and "contested" cases (D959), offline, no model call.

Usage:
    python3 -I -B eval/adcvd/real/truth3.py SPLIT_DIR --pairs PAIRS --stage2 STAGE2.jsonl --labels FILE:VERSION[:MODEL+MODEL] [--labels ...] --models M1,M2,M3

Per pair read in full, the three labellers each answer in, out or unclear. A pair is `in` when at least two say in and none says out, `out` when at least two say out and none says in (rule `nodissent`; rule `all`:
all three agree), otherwise contested (a pair no screener listed was not read and is `out`). A ruling is
  positive   when at least one pair is unanimous in, or CBP's own ruling names an order by case number that is in the pool;
  negative   when no pair has a vote other than out and CBP names no order (every read pair unanimous out);
  contested  otherwise: no pair is unanimous in, but at least one pair drew an in or unclear vote from a labeller.
`truth_groups` are the orders known to be within (unanimous in, or cited by CBP). `contested_groups` are those with some in vote but not unanimous.
Writes truth3.jsonl and truth3-summary.json in SPLIT_DIR.
"""
import argparse
import json
import os
from collections import Counter


def load(p):
    return [json.loads(l) for l in open(p) if l.strip()]


def pair_verdict(v, rule):
    """in, out or contested. Rule `all`: all three labellers agree. Rule `nodissent` (D959): at least two agree and none says the opposite (unclear is an abstention, not a dissent)."""
    n_in, n_out = v.count("in"), v.count("out")
    if rule == "all":
        return "in" if n_in == len(v) else "out" if n_out == len(v) else "contested"
    if n_in >= 2 and n_out == 0:
        return "in"
    if n_out >= 2 and n_in == 0:
        return "out"
    return "contested"


def build(split, pairs, stage2, label_specs, models, rule="nodissent", suffix=""):
    qs = load(os.path.join(split, "queries.jsonl"))
    read = {(r["key"], r["id"]) for r in load(stage2)}
    ans = {}
    for spec in label_specs:
        f, ver, only = (spec.split(":") + ["", ""])[:3]
        for r in load(f):
            if ver and r["prompt_version"] != ver:
                continue
            if only and r["model"] not in only.split("+"):
                continue
            ans.setdefault((r["key"], r["id"]), {})[r["model"]] = r["verdict"]
    by = {}
    for p in load(pairs):
        by.setdefault(p["id"], []).append(p)
    rows, st = [], Counter()
    for q in qs:
        un_in, con_in, odd = [], [], 0
        seen = set()
        for p in by.get(q["id"], []):
            k = (p["key"], p["id"])
            if k in seen or k not in read:
                continue
            seen.add(k)
            v = [ans.get(k, {}).get(m, "missing") for m in models]
            pv = pair_verdict(v, rule)
            if pv == "in":
                un_in.append(" ".join(p["cases"]))
                st["pairs_unanimous_in"] += 1
            elif pv == "out":
                st["pairs_unanimous_out"] += 1
            else:
                odd += 1
                st["pairs_contested"] += 1
                if any(x == "in" for x in v):
                    con_in.append(" ".join(p["cases"]))
        cited = sorted({" ".join(p["cases"]) for p in by.get(q["id"], []) if any(c in q.get("note_cases", []) for c in p["cases"])})
        if un_in or cited:
            status = "positive"
        elif odd:
            status = "contested"
        else:
            status = "negative"
        st["rulings_" + status] += 1
        rows.append({"id": q["id"], "status": status, "positive": status == "positive", "truth_groups": sorted(set(un_in) | set(cited)),
                     "contested_groups": sorted(set(con_in) - set(un_in)), "contested_pairs": odd})
    with open(os.path.join(split, "truth3%s.jsonl" % suffix), "w") as f:
        for r in rows:
            f.write(json.dumps(r) + "\n")
    summ = {"rulings": len(rows), **dict(st)}
    json.dump(summ, open(os.path.join(split, "truth3%s-summary.json" % suffix), "w"), indent=1)
    print(json.dumps(summ, indent=1))


if __name__ == "__main__":
    ap = argparse.ArgumentParser()
    ap.add_argument("split")
    ap.add_argument("--pairs", required=True)
    ap.add_argument("--stage2", required=True)
    ap.add_argument("--labels", action="append", required=True)
    ap.add_argument("--models", required=True)
    ap.add_argument("--rule", default="nodissent", choices=["nodissent", "all"])
    ap.add_argument("--suffix", default="")
    a = ap.parse_args()
    build(a.split, a.pairs, a.stage2, a.labels, a.models.split(","), a.rule, a.suffix)
