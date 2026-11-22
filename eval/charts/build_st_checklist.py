# Scores the component-checklist variants of the substantial-transformation
# engine against engine-v1 (D650).
#
# Usage:  python3 -I eval/charts/build_st_checklist.py .          # write
#         python3 -I eval/charts/build_st_checklist.py . --check  # fail if stale
# Reads:  eval/results/dev-gemini-4-argon-prec-st-checklist-v*-litellm-d650.jsonl,
#         the held-out rows of the same name if they exist, and the engine-v1
#         rows named below
# Writes: eval/results/st-checklist-metrics.json
#
# No model calls. Scoring adjustments (D23), Wilson intervals, the exact
# McNemar test, ECE and Brier are build_accuracy.py's. "China-held" is a
# ruling whose gold origin is China.
import json
import os
import sys

sys.dont_write_bytecode = True
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import build_accuracy as ba  # noqa: E402

R = "eval/results/"
VARIANTS = {"Va": "st-checklist-v1", "Vb": "st-checklist-v2"}
DEV_BASE = R + "dev-gemini-4-argon-prec-engine-v1-litellm-gw.jsonl"  # D650 baseline (D190 run 1)
DEV_REPEATS = [R + "dev-gemini-4-argon-prec-engine-v1-litellm-router-rep2.jsonl",
               R + "dev-gemini-4-argon-prec-engine-v1-litellm-router-rep3.jsonl",
               R + "dev-gemini-4-argon-prec-engine-v1-vertex.jsonl"]
HELD = {  # split: (official engine-v1 rows, scope file or None for the rows' own router labels, variant file pattern)
    "final": (R + "final-gemini-4-argon-prec-engine-v1-vertex.jsonl", R + "scope-final-router-v2-vertex.jsonl",
              R + "final-gemini-4-argon-prec-{v}-litellm-d650.jsonl"),
    "final2": (R + "final2-gemini-4-argon-prec-engine-v1-litellm-router.jsonl", None,
               R + "final2-gemini-4-argon-prec-{v}-litellm-router-d650.jsonl"),
}
SCOPE_DEV = R + "scope-dev-router-v2-vertex.jsonl"
CALMAP = "backend/internal/calibrate/map.json"
NET_MIN, BASE_ACC, CHINA_BASE, OTHER_BASE, OTHER_LOSS_MAX = 5, 425, 66, 359, 2  # D650


def rows(p, scope=None):
    out = ba.scored(p, scope)
    raw = {r["ruling"]: r for r in ba.jsonl(p)}
    for i, r in out.items():
        r["out_tokens"] = int(raw[i].get("output_tokens") or 0)
        if scope is None and "in_scope" in raw[i] and raw[i].get("router_route") is not None:
            r["in_scope"] = bool(raw[i]["in_scope"])
    return out


def subset(rs, china):
    s = [r for r in rs.values() if (r["gold"] == "China") == china]
    k = sum(r["ok"] for r in s)
    p, lo, hi = ba.wilson(k, len(s))
    return {"n": len(s), "correct": k, "accuracy_pct": ba.pct(p), "ci95_pct": [ba.pct(lo), ba.pct(hi)]}


def block(rs, base, calmap):
    s = ba.summary(rs)
    ok = [r for r in rs.values() if not r["err"]]
    s["output_tokens_mean"] = round(sum(r["out_tokens"] for r in ok) / max(1, len(ok)), 1)
    s["gateway_cost_total_usd"] = ba.r4(sum(r["gateway_cost"] for r in rs.values()))
    s["china_held"] = subset(rs, True)
    s["not_china_held"] = subset(rs, False)
    if base is not None:
        s["mcnemar_vs_engine_v1"] = ba.mcnemar(base, rs)
        bc = {i: r for i, r in base.items() if r["gold"] == "China"}
        bn = {i: r for i, r in base.items() if r["gold"] != "China"}
        s["mcnemar_china_held"] = ba.mcnemar(bc, {i: rs[i] for i in bc if i in rs})
        s["mcnemar_not_china_held"] = ba.mcnemar(bn, {i: rs[i] for i in bn if i in rs})
    cal = ba.with_calibrated(rs, calmap["steps"])
    s["cal_v2_bar_reading"] = ba.at_bar(cal, calmap["bar"])
    return s


def main():
    ba.ROOT = sys.argv[1] if len(sys.argv) > 1 else "."
    calmap = json.load(open(ba.path(CALMAP)))
    scope_dev = {r["ruling"]: r["in_scope"] for r in ba.jsonl(SCOPE_DEV)}
    base = rows(DEV_BASE, scope_dev)
    out = {
        "_doc": "Component-checklist variants of the substantial-transformation engine against engine-v1 (D650). Built by python3 -I eval/charts/build_st_checklist.py . from committed rows; no model calls. China-held: gold origin China. cal_v2_bar_reading applies engine-v1's cal-v2 map to the variant's stated confidence as a reading only; the map was not fitted for it.",
        "pass_rule_d650": {"accuracy_at_least": BASE_ACC, "net_at_least": NET_MIN, "china_held_more_than": CHINA_BASE,
                           "not_china_held_at_least": OTHER_BASE - OTHER_LOSS_MAX},
        "dev": {"engine-v1 (gateway, D190 run 1)": dict(block(base, None, calmap), file=DEV_BASE)},
        "held_out": {},
    }
    for name, v in VARIANTS.items():
        p = R + f"dev-gemini-4-argon-prec-{v}-litellm-d650.jsonl"
        if not ba.exists(p):
            continue
        rs = rows(p, scope_dev)
        if len(rs) != len(base):
            continue  # incomplete run
        b = block(rs, base, calmap)
        b["file"] = p
        b["mcnemar_vs_other_engine_v1_runs"] = {os.path.basename(q): ba.mcnemar(rows(q, scope_dev), rs) for q in DEV_REPEATS}
        mc = b["mcnemar_vs_engine_v1"]
        b["net"] = mc["fixed"] - mc["broke"]
        b["cost_ratio_to_engine_v1"] = round(b["cost_per_case_usd"] / out["dev"]["engine-v1 (gateway, D190 run 1)"]["cost_per_case_usd"], 2)
        b["passes_d650"] = bool(b["correct"] >= BASE_ACC and b["net"] >= NET_MIN and b["china_held"]["correct"] > CHINA_BASE
                                and b["not_china_held"]["correct"] >= OTHER_BASE - OTHER_LOSS_MAX)
        out["dev"][f"{name}: {v}"] = b
    for split, (bp, scope_file, pat) in HELD.items():
        scope = {r["ruling"]: r["in_scope"] for r in ba.jsonl(scope_file)} if scope_file else None
        hb = rows(bp, scope)
        entry = {"engine-v1 (official)": dict(block(hb, None, calmap), file=bp)}
        for name, v in VARIANTS.items():
            p = pat.format(v=v)
            if ba.exists(p):
                rs = rows(p, scope)
                entry[f"{name}: {v}"] = dict(block(rs, hb, calmap), file=p)
        if len(entry) > 1:
            out["held_out"][split] = entry
    body = json.dumps(out, indent=1) + "\n"
    target = ba.path(R + "st-checklist-metrics.json")
    if "--check" in sys.argv[2:]:
        if not os.path.exists(target) or open(target).read() != body:
            print("stale: " + R + "st-checklist-metrics.json")
            sys.exit(1)
        print("checked " + R + "st-checklist-metrics.json")
        return
    with open(target, "w") as f:
        f.write(body)
    for sec in ("dev",):
        for k, v in out[sec].items():
            print(sec, k, v["correct"], v["n"], v["accuracy_pct"], "china", v["china_held"]["correct"], "/", v["china_held"]["n"],
                  "other", v["not_china_held"]["correct"], "/", v["not_china_held"]["n"], "cost", v["cost_per_case_usd"],
                  "p50/p95", v["latency_p50_s"], v["latency_p95_s"], "mc", v.get("mcnemar_vs_engine_v1"), "pass", v.get("passes_d650"))
    for split, e in out["held_out"].items():
        for k, v in e.items():
            print(split, k, v["correct"], v["n"], v["accuracy_pct"], v["ci95_pct"], "china", v["china_held"]["correct"], "/", v["china_held"]["n"],
                  "mc", v.get("mcnemar_vs_engine_v1"), "ece", v["calibration"]["ece_levels"], "bar", v["cal_v2_bar_reading"])


if __name__ == "__main__":
    main()
