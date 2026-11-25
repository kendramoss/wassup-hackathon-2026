"""D900: precedent support as a triage gate. Written before any figure on the fit set was read. No model calls.

Auto-accepted = in scope and stated confidence >= 0.92 (on engine-v1 with Gemini 4 Argon the same cases as
calibrated >= 0.90, D160). Support of a row: agree = how many of its five precedents held the predicted country
(labels.jsonl primary_origin); age = mean months between the case and its precedents.
Gates (fixed here): S1 agree >= 1; S2 agree >= 2; A12 age <= 12; S1A6 agree >= 1 or age <= 6.
Fit: the training rows of D860's check control. Pick: among gates that keep at least 85% of auto-accepted
fit rows, the one that removes the most auto-accepted errors (ties: fewer rows removed).
Test: the official development run (primary) and D815's and D860's development controls.

    python3 -I -B eval/accuracy/measure_900.py .          # write eval/accuracy/900-measure.json
    python3 -I -B eval/accuracy/measure_900.py . --check
"""
import json
import os
import statistics
import sys

R = "eval/results/"
FIT = R + "train-gemini-4-argon-prec-engine-v1-litellm-860-control.jsonl"
PRIMARY = R + "dev-gemini-4-argon-prec-engine-v1-vertex.jsonl"
SECONDARY = [R + "dev-gemini-4-argon-prec-engine-v1-litellm-815-control.jsonl",
             R + "dev-gemini-4-argon-prec-engine-v1-litellm-860-control.jsonl"]
OUT = "eval/accuracy/900-measure.json"
BAR = 0.92


def months(d):
    return int(d[:4]) * 12 + int(d[5:7])


def support(r, lab):
    ps = [p for p in r.get("precedents") or [] if p in lab]
    agree = sum(1 for p in ps if lab[p].get("primary_origin") == r.get("pred"))
    age = statistics.mean(months(r["date"]) - months(lab[p]["date"]) for p in ps) if ps else 999.0
    return agree, age


GATES = {
    "S1": lambda a, g: a >= 1,
    "S2": lambda a, g: a >= 2,
    "A12": lambda a, g: g <= 12,
    "S1A6": lambda a, g: a >= 1 or g <= 6,
}


def stats(rows, lab, gate=None):
    auto = [r for r in rows if r.get("in_scope") and (r.get("confidence") or 0) >= BAR and r.get("pred")]
    kept = auto if gate is None else [r for r in auto if GATES[gate](*support(r, lab))]
    err = sum(1 for r in kept if not r["correct"])
    return {"n": len(rows), "auto": len(kept), "coverage": round(len(kept) / len(rows), 4),
            "auto_errors": err, "auto_accuracy": round(1 - err / len(kept), 4) if kept else None}


def build(root):
    lab = {}
    with open(os.path.join(root, "eval/data/labels.jsonl")) as f:
        for l in f:
            x = json.loads(l)
            lab[x["ruling"]] = x
    load = lambda p: [json.loads(l) for l in open(os.path.join(root, p)) if l.strip()]
    fit = load(FIT)
    base = stats(fit, lab)
    fitted = {g: stats(fit, lab, g) for g in GATES}
    ok = [g for g in GATES if fitted[g]["auto"] >= 0.85 * base["auto"]]
    pick = min(ok, key=lambda g: (fitted[g]["auto_errors"], base["auto"] - fitted[g]["auto"])) if ok else None
    out = {"fit": {"file": FIT, "before": base, "gates": fitted, "eligible": ok, "picked": pick}, "test": {}}
    for p in [PRIMARY] + SECONDARY:
        rows = load(p)
        out["test"][p] = {"before": stats(rows, lab), "after": stats(rows, lab, pick) if pick else None,
                          "all_gates": {g: stats(rows, lab, g) for g in GATES}}
    pb, pa = out["test"][PRIMARY]["before"], out["test"][PRIMARY]["after"]
    sec_up = all(out["test"][p]["after"]["auto_accuracy"] > out["test"][p]["before"]["auto_accuracy"] for p in SECONDARY) if pick else False
    out["criterion"] = {"primary_errors_cut": round(1 - pa["auto_errors"] / pb["auto_errors"], 4) if pick and pb["auto_errors"] else None,
                        "primary_coverage_drop_points": round((pb["coverage"] - pa["coverage"]) * 100, 2) if pick else None,
                        "secondary_accuracy_up_both": sec_up}
    out["criterion"]["passed"] = bool(pick and out["criterion"]["primary_errors_cut"] is not None
                                      and out["criterion"]["primary_errors_cut"] >= 0.30
                                      and out["criterion"]["primary_coverage_drop_points"] <= 10 and sec_up)
    return out


if __name__ == "__main__":
    root = sys.argv[1]
    got = json.dumps(build(root), indent=2, sort_keys=True) + "\n"
    p = os.path.join(root, OUT)
    if "--check" in sys.argv:
        if open(p).read() != got:
            sys.exit(OUT + " is stale")
    else:
        open(p, "w").write(got)
        print(got)
