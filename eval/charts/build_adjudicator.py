# Scores the second-stage adjudicator runs against their stage-1 rows (D163).
#
# Usage:  python3 -I eval/charts/build_adjudicator.py .          # write
#         python3 -I eval/charts/build_adjudicator.py . --check  # fail if stale
# Reads:  eval/results/*+adjudicate-v1-*.jsonl (and their stage-1 files)
# Writes: eval/results/adjudicator-metrics.json
#
# No model calls. Scoring adjustments (D23) and the exact McNemar test are
# the ones build_accuracy.py uses. Cost per case is stage 1 (token-priced,
# as the stage-1 file records it) plus the adjudicator's gateway cost, or its
# token-priced cost for a call the gateway answered from its cache.
import glob
import json
import os
import sys

sys.dont_write_bytecode = True
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import build_accuracy as ba  # noqa: E402

PASS_NET, PASS_P, PASS_COST = 5, 0.1, 1.5  # D163


def jsonl_last(p):
    out = {}
    for r in ba.jsonl(p):
        out[r["ruling"]] = r
    return out


def ok_of(r):
    adj = ba.ADJ
    if r.get("error"):
        return False
    alts = adj["alternatives"].get(r["ruling"])
    if alts is not None:
        parts = ba.countries(r["pred"])
        return len(parts) > 0 and all(x in alts for x in parts)
    return r["pred"] == r["gold"]


SCOPE = {"dev": "eval/results/scope-dev-router-v2-vertex.jsonl", "final": "eval/results/scope-final-router-v2-vertex.jsonl"}


def from_china_only(rows):
    """Variant 3 (D164), computed from variant 2's rows with no new calls:
    keep the adjudicator's answer only when it moves stage 1 away from China."""
    out = {}
    for i, r in rows.items():
        r = dict(r)
        if r.get("adj_changed") and r["base_pred"] != "China":
            r["pred"], r["confidence"], r["correct"], r["adj_changed"] = r["base_pred"], r["base_confidence"], r["base_correct"], False
        out[i] = r
    return out


def one(p, rows=None):
    rows = rows or jsonl_last(p)
    stem = os.path.basename(p)[:-len(".jsonl")]
    base_name = stem.split("+")[0] + ".jsonl"
    base = jsonl_last(ba.path("eval/results/" + base_name))
    ba.scored(ba.path("eval/results/" + base_name))  # loads ADJ
    ids = [i for i in base if i not in ba.ADJ["exclude"] and i in rows]
    if len(ids) != len([i for i in base if i not in ba.ADJ["exclude"]]):
        return None  # incomplete run
    split = rows[ids[0]].get("split")
    if split in SCOPE:
        sc = {r["ruling"]: r["in_scope"] for r in ba.jsonl(ba.path(SCOPE[split]))}
        for i in ids:
            base[i] = dict(base[i], in_scope=sc.get(i, True))
            rows[i] = dict(rows[i], in_scope=sc.get(i, True))
    a = {i: {"ok": ok_of(base[i])} for i in ids}
    b = {i: {"ok": ok_of(rows[i])} for i in ids}
    mc = ba.mcnemar(a, b)
    k0, k1, n = sum(v["ok"] for v in a.values()), sum(v["ok"] for v in b.values()), len(ids)
    trig = [i for i in ids if rows[i].get("adj_triggered")]
    changed = [i for i in ids if rows[i].get("adj_changed")]
    s1 = sum(float(base[i].get("cost_usd_gateway") or base[i].get("cost_usd") or 0) for i in ids)
    # The gateway reports no cost for a request it answered from its cache
    # (D151); those calls are priced from their tokens instead.
    s2 = sum(float(rows[i].get("adj_cost_usd_gateway") or rows[i].get("adj_cost_usd") or 0) for i in ids if rows[i].get("adj_triggered"))
    lo0 = ba.wilson(k0, n)
    lo1 = ba.wilson(k1, n)

    def fast(rs):
        sel = [i for i in ids if not rs[i].get("error") and rs[i].get("in_scope", True) and float(rs[i]["confidence"]) >= 0.92]
        k = sum(ok_of(rs[i]) for i in sel)
        return {"n": len(sel), "correct": k, "coverage_pct": ba.pct(len(sel) / n), "accuracy_pct": ba.pct(k / len(sel)) if sel else None}

    def case(i):
        r = rows[i]
        return {"ruling": i, "gold": r["gold"], "stage1": r["base_pred"], "stage1_conf": r["base_confidence"],
                "adjudicator": r.get("adj_origin"), "adj_conf": r.get("adj_confidence"), "final": r["pred"]}

    net = mc["fixed"] - mc["broke"]
    ratio = (s1 + s2) / s1 if s1 else None
    return {
        "file": "eval/results/" + os.path.basename(p), "stage1": "eval/results/" + base_name,
        "split": split, "n": n,
        "stage1_accuracy": {"correct": k0, "pct": ba.pct(k0 / n), "ci95": [ba.pct(lo0[1]), ba.pct(lo0[2])]},
        "accuracy": {"correct": k1, "pct": ba.pct(k1 / n), "ci95": [ba.pct(lo1[1]), ba.pct(lo1[2])]},
        "triggered": len(trig), "changed": len(changed),
        "adj_errors": sum(1 for i in ids if rows[i].get("adj_error")),
        "mcnemar_vs_stage1": mc, "net": net,
        "fixed": [case(i) for i in ids if not a[i]["ok"] and b[i]["ok"]],
        "broke": [case(i) for i in ids if a[i]["ok"] and not b[i]["ok"]],
        "cost_per_case_usd": {"stage1": round(s1 / n, 5), "adjudicator": round(s2 / n, 5), "total": round((s1 + s2) / n, 5),
                              "ratio": round(ratio, 3) if ratio else None},
        "fast_path_092": {"stage1": fast(base), "with_adjudicator": fast(rows)},
        "passes_d163": bool(net >= PASS_NET and mc["p"] < PASS_P and ratio is not None and ratio <= PASS_COST),
    }


def main():
    ba.ROOT = sys.argv[1] if len(sys.argv) > 1 else "."
    out = {"_doc": "Second-stage adjudicator (D163) against its stage-1 rows. Built by python3 -I eval/charts/build_adjudicator.py . from committed rows; no model calls.",
           "pass_rule_d163": {"net_at_least": PASS_NET, "p_below": PASS_P, "cost_ratio_at_most": PASS_COST}, "runs": {}}
    for p in sorted(glob.glob(ba.path("eval/results/*+adjudicate-v1-*.jsonl"))):
        if p.endswith("-probe.jsonl"):
            continue
        r = one(p)
        if r is None:
            continue
        name = os.path.basename(p)[:-len(".jsonl")]
        out["runs"][name] = r
        if "-symmetric-" in name:
            v3 = one(p, from_china_only(jsonl_last(p)))
            v3["file"] = "computed from " + r["file"] + ": only changes away from China kept (variant 3, chosen after variants 1 and 2)"
            v3["cost_per_case_usd"] = r["cost_per_case_usd"]  # it needs the same calls
            out["runs"][name + "-from-china-only"] = v3
    body = json.dumps(out, indent=1) + "\n"
    target = ba.path("eval/results/adjudicator-metrics.json")
    if "--check" in sys.argv[2:]:
        if not os.path.exists(target) or open(target).read() != body:
            print("stale: eval/results/adjudicator-metrics.json")
            sys.exit(1)
        print("checked eval/results/adjudicator-metrics.json")
        return
    with open(target, "w") as f:
        f.write(body)
    for k, v in out["runs"].items():
        print(k, v["split"], v["n"], "stage1", v["stage1_accuracy"]["pct"], "->", v["accuracy"]["pct"],
              "trig", v["triggered"], "changed", v["changed"], "fixed/broke", v["mcnemar_vs_stage1"]["fixed"], v["mcnemar_vs_stage1"]["broke"],
              "p", v["mcnemar_vs_stage1"]["p"], "cost x", v["cost_per_case_usd"]["ratio"], "pass", v["passes_d163"])


if __name__ == "__main__":
    main()
