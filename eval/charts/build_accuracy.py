# Builds the model sweep, calibration and second held-out numbers (D150-D155)
# from committed result files only. No model calls, no network, standard
# library only. Output is deterministic.
#
# Usage (from the repo root):
#   python3 -I eval/charts/build_accuracy.py .          # write the outputs
#   python3 -I eval/charts/build_accuracy.py . --check  # fail if the outputs are stale
#
# Reads:  eval/data/labels.jsonl, eval/data/labels_final2.jsonl,
#         eval/data/scoring_adjustments.json, eval/splits/splits_final{,2}.json,
#         eval/results/dev-*.jsonl, final-*.jsonl, final2-*.jsonl (listed in RUNS),
#         eval/results/scope-{dev,final}-router-v2-vertex.jsonl,
#         textile/pharma/usmca held-out result files (all-regimes report)
# Writes: eval/results/accuracy-v2-metrics.json
#         eval/results/calibration-map.json (isotonic map, fitted on dev only)
#         docs/img/accuracy-v2-selective.svg

import json
import math
import os
import random
import sys

ROOT = sys.argv[1] if len(sys.argv) > 1 else "."
CHECK = "--check" in sys.argv[2:]
SEED = 20261017
BOOTSTRAP = 2000
BAR = 0.92

# Palette tokens from frontend/src/app/globals.css.
SHEET, GRID, LINE = "#f9fbfe", "#d3dff0", "#c4d0e2"
INK, DIM, FAINT = "#13213a", "#47566f", "#6e7c94"
HELD, DEV, ALT = "#1d4fc4", "#8fa7d6", "#13213a"
FONT = "'Paper Mono', ui-monospace, SFMono-Regular, Menlo, monospace"

R = "eval/results/"
# Development runs in the sweep (D151, D152). Each is the same engine-v1
# prompt, BM25, K = 5; only the model or the path to it differs.
DEV_RUNS = {
    "argon_vertex": R + "dev-gemini-4-argon-prec-engine-v1-vertex.jsonl",
    "argon_gateway": R + "dev-gemini-4-argon-prec-engine-v1-litellm-gw.jsonl",
    "carbon_gateway": R + "dev-gemini-4-carbon-prec-engine-v1-litellm.jsonl",
    "pro31_gateway": R + "dev-gemini-3.1-pro-preview-prec-engine-v1-litellm.jsonl",
    "flash38_vertex": R + "dev-gemini-3.8-flash-prec-engine-v1-vertex.jsonl",
}
BASELINE = "argon_vertex"
# The candidate pre-registered in D153 and its held-out runs (D154).
CANDIDATE = None  # set in D153 before any held-out file exists
HELDOUT_RUNS = {}


def path(p):
    return f"{ROOT}/{p}"


def exists(p):
    return os.path.exists(path(p))


def jsonl(p):
    with open(path(p)) as f:
        return [json.loads(line) for line in f if line.strip()]


def wilson(k, n):
    if n == 0:
        return 0.0, 0.0, 0.0
    z = 1.96
    p = k / n
    d = 1 + z * z / n
    c = (p + z * z / (2 * n)) / d
    h = z * math.sqrt(p * (1 - p) / n + z * z / (4 * n * n)) / d
    return p, c - h, c + h


def pct(x, digits=1):
    return round(100 * x, digits)


def r4(x):
    return round(x, 4)


def countries(pred):
    return [p.strip() for p in pred.replace("|", "/").split("/") if p.strip()]


ADJ = None


def scored(p, scope=None):
    """Rows of one run with the scoring adjustments applied (D23)."""
    global ADJ
    if ADJ is None:
        ADJ = json.load(open(path("eval/data/scoring_adjustments.json")))
    out = {}
    for r in jsonl(p):
        rid = r["ruling"]
        if rid in ADJ["exclude"]:
            continue
        err = bool(r.get("error"))
        ok = bool(r["correct"]) and not err
        alts = ADJ["alternatives"].get(rid)
        if alts is not None and not err:
            parts = countries(r["pred"])
            ok = len(parts) > 0 and all(x in alts for x in parts)
        out[rid] = {
            "id": rid, "ok": ok, "err": err, "conf": 0.0 if err else float(r["confidence"]),
            "pred": r.get("pred", ""), "gold": r.get("gold", ""),
            "in_scope": True if scope is None else scope.get(rid, True),
            "cost": float(r.get("cost_usd_gateway") or r.get("cost_usd") or 0.0),
            "gateway_cost": float(r.get("cost_usd_gateway") or 0.0),
            "lat": int(r.get("latency_ms") or 0),
        }
    return out


def percentile(v, q):
    if not v:
        return 0
    v = sorted(v)
    i = max(0, min(len(v) - 1, int(math.ceil(q / 100 * len(v))) - 1))
    return v[i]


def binom_two_sided(k, n):
    """Exact two-sided McNemar p-value on n discordant pairs, k of one kind."""
    if n == 0:
        return 1.0
    k = min(k, n - k)
    tail = sum(math.comb(n, i) for i in range(k + 1)) / 2 ** n
    return min(1.0, 2 * tail)


def mcnemar(a, b):
    ids = sorted(set(a) & set(b))
    fixed = sum(1 for i in ids if not a[i]["ok"] and b[i]["ok"])
    broke = sum(1 for i in ids if a[i]["ok"] and not b[i]["ok"])
    return {"n": len(ids), "fixed": fixed, "broke": broke, "p": round(binom_two_sided(fixed, fixed + broke), 3)}


def ece_levels(rs, key="conf"):
    tot = 0.0
    for c in {r[key] for r in rs}:
        b = [r for r in rs if r[key] == c]
        tot += len(b) * abs(sum(r["ok"] for r in b) / len(b) - c)
    return tot / len(rs)


def ece_bins(rs, key="conf", m=10):
    bins = [[] for _ in range(m)]
    for r in rs:
        bins[min(int(r[key] * m), m - 1)].append(r)
    tot = 0.0
    for b in bins:
        if b:
            tot += len(b) * abs(sum(r["ok"] for r in b) / len(b) - sum(r[key] for r in b) / len(b))
    return tot / len(rs)


def brier(rs, key="conf"):
    return sum((r[key] - (1.0 if r["ok"] else 0.0)) ** 2 for r in rs) / len(rs)


def calib(rs, key="conf"):
    rng = random.Random(SEED)
    n = len(rs)
    boot = {"ece_levels": [], "brier": []}
    for _ in range(BOOTSTRAP):
        s = [rs[rng.randrange(n)] for _ in range(n)]
        boot["ece_levels"].append(ece_levels(s, key))
        boot["brier"].append(brier(s, key))

    def ci(v):
        v = sorted(v)
        return [r4(v[int(0.025 * len(v))]), r4(v[int(0.975 * len(v)) - 1])]

    acc = sum(r["ok"] for r in rs) / n
    return {
        "mean_confidence_pct": pct(sum(r[key] for r in rs) / n),
        "ece_levels": r4(ece_levels(rs, key)), "ece_levels_ci95": ci(boot["ece_levels"]),
        "ece_10_bins": r4(ece_bins(rs, key)),
        "brier": r4(brier(rs, key)), "brier_ci95": ci(boot["brier"]),
        "brier_constant_forecast": r4(acc * (1 - acc)),
    }


def summary(rows, with_calibration=True):
    rs = list(rows.values())
    n = len(rs)
    k = sum(r["ok"] for r in rs)
    p, lo, hi = wilson(k, n)
    fast = [r for r in rs if not r["err"] and r["conf"] >= BAR and r["in_scope"]]
    fk = sum(r["ok"] for r in fast)
    fp, flo, fhi = wilson(fk, len(fast))
    ok_rows = [r for r in rs if not r["err"]]
    out = {
        "n": n, "correct": k, "errors": sum(r["err"] for r in rs),
        "accuracy_pct": pct(p), "ci95_pct": [pct(lo), pct(hi)],
        "fast_path": {"accepted": len(fast), "coverage_pct": pct(len(fast) / n) if n else 0,
                      "correct": fk, "wrong": len(fast) - fk, "accuracy_pct": pct(fp), "ci95_pct": [pct(flo), pct(fhi)]},
        "cost_per_case_usd": r4(sum(r["cost"] for r in ok_rows) / max(1, len(ok_rows))),
        "latency_p50_s": round(percentile([r["lat"] for r in ok_rows], 50) / 1000, 1),
        "latency_p95_s": round(percentile([r["lat"] for r in ok_rows], 95) / 1000, 1),
    }
    if with_calibration and n:
        out["calibration"] = calib(rs)
    return out


def selective(rows, key="conf"):
    rs = list(rows.values()) if isinstance(rows, dict) else rows
    n = len(rs)
    pts = []
    for t in sorted({r[key] for r in rs}, reverse=True):
        acc = [r for r in rs if r[key] >= t]
        k = sum(r["ok"] for r in acc)
        p, lo, hi = wilson(k, len(acc))
        pts.append({"threshold": t, "accepted": len(acc), "correct": k, "coverage_pct": pct(len(acc) / n),
                    "accuracy_pct": pct(p), "ci_lo_pct": pct(lo), "ci_hi_pct": pct(hi)})
    return pts


# ---------------------------------------------------------------- isotonic


def isotonic_fit(rs):
    """Pool-adjacent-violators on (confidence, correct), one block per emitted
    confidence value. Returns a step map: list of {"from": c, "to": calibrated}."""
    levels = sorted({r["conf"] for r in rs})
    blocks = []
    for c in levels:
        b = [r for r in rs if r["conf"] == c]
        blocks.append({"lo": c, "hi": c, "n": len(b), "k": sum(r["ok"] for r in b)})
    merged = []
    for b in blocks:
        merged.append(dict(b))
        while len(merged) > 1 and merged[-2]["k"] / merged[-2]["n"] > merged[-1]["k"] / merged[-1]["n"]:
            x = merged.pop()
            y = merged.pop()
            merged.append({"lo": y["lo"], "hi": x["hi"], "n": x["n"] + y["n"], "k": x["k"] + y["k"]})
    return [{"from": b["lo"], "to_incl": b["hi"], "n": b["n"], "correct": b["k"],
             "calibrated": r4(b["k"] / b["n"])} for b in merged]


def isotonic_apply(m, c):
    """Step function: the block whose range holds c, else the nearest block
    below (and the first block for values under every range)."""
    best = m[0]["calibrated"]
    for b in m:
        if c >= b["from"]:
            best = b["calibrated"]
    return best


def with_calibrated(rows, m):
    out = {}
    for i, r in rows.items():
        r = dict(r)
        r["cal"] = isotonic_apply(m, r["conf"])
        out[i] = r
    return out


# ---------------------------------------------------------------- ensemble


def vote(runs, ids, order):
    """Majority over models; a tie keeps the first model in `order`. The
    confidence is the share of models that agree times the mean confidence
    of the agreeing models."""
    out = {}
    for i in ids:
        preds = [(name, runs[name][i]) for name in order]
        counts = {}
        for _, r in preds:
            if not r["err"]:
                counts[r["pred"]] = counts.get(r["pred"], 0) + 1
        if not counts:
            out[i] = dict(preds[0][1])
            continue
        top = max(counts.values())
        winner = next(r["pred"] for _, r in preds if not r["err"] and counts[r["pred"]] == top)
        agree = [r for _, r in preds if not r["err"] and r["pred"] == winner]
        base = next(r for _, r in preds if not r["err"] and r["pred"] == winner)
        out[i] = dict(base)
        out[i]["conf"] = round(len(agree) / len(order) * sum(r["conf"] for r in agree) / len(agree), 4)
        out[i]["cost"] = sum(r["cost"] for _, r in preds)
        out[i]["lat"] = max(r["lat"] for _, r in preds)
        out[i]["unanimous"] = len(agree) == len(order)
    return out


# ---------------------------------------------------------------- main


def main():
    scope_dev = {r["ruling"]: r["in_scope"] for r in jsonl(R + "scope-dev-router-v2-vertex.jsonl")}
    dev = {name: scored(p, scope_dev) for name, p in DEV_RUNS.items() if exists(p)}
    metrics = {"_doc": "Built by eval/charts/build_accuracy.py from committed rows; no model calls. "
                       "Development set = dev split with the D23 scoring adjustments. Cost per case is the "
                       "gateway-reported cost where the row has one, else the token-priced promo cost.",
               "bar": BAR, "dev": {}, "dev_vs_" + BASELINE: {}}
    for name, rows in dev.items():
        metrics["dev"][name] = {"file": DEV_RUNS[name], **summary(rows)}
        if name != BASELINE:
            metrics["dev_vs_" + BASELINE][name] = mcnemar(dev[BASELINE], rows)
    if "argon_gateway" in dev and "carbon_gateway" in dev:
        metrics["dev_vs_argon_gateway"] = {n: mcnemar(dev["argon_gateway"], dev[n]) for n in dev if n != "argon_gateway"}

    # Ensembles of the three newest gateway runs (no extra calls: computed
    # from the rows above).
    trio = [n for n in ("argon_gateway", "carbon_gateway", "pro31_gateway") if n in dev]
    if len(trio) == 3:
        ids = sorted(set.intersection(*(set(dev[n]) for n in trio)))
        for order in (trio, ["carbon_gateway", "argon_gateway", "pro31_gateway"], ["pro31_gateway", "carbon_gateway", "argon_gateway"]):
            ens = vote(dev, ids, order)
            key = "vote3_" + order[0].split("_")[0] + "_first"
            metrics["dev"][key] = {"file": "computed: majority of " + ", ".join(order), **summary(ens)}
            metrics["dev_vs_" + BASELINE][key] = mcnemar(dev[BASELINE], ens)
            un = [r for r in ens.values() if r.get("unanimous")]
            metrics["dev"][key]["unanimous"] = {"n": len(un), "correct": sum(r["ok"] for r in un),
                                                "accuracy_pct": pct(sum(r["ok"] for r in un) / max(1, len(un)))}

    # Isotonic calibration: fitted on each dev run, in-sample numbers here
    # only describe the fit; the held-out check is below.
    metrics["calibration_maps"] = {}
    for name, rows in dev.items():
        if name.startswith("vote3"):
            continue
        rs = list(rows.values())
        m = isotonic_fit(rs)
        metrics["calibration_maps"][name] = m

    if CANDIDATE:
        metrics["candidate"] = CANDIDATE
    out = {"eval/results/accuracy-v2-metrics.json": json.dumps(metrics, indent=1, ensure_ascii=False) + "\n"}
    stale = []
    for p, body in out.items():
        cur = open(path(p)).read() if exists(p) else None
        if cur != body:
            stale.append(p)
            if not CHECK:
                with open(path(p), "w") as f:
                    f.write(body)
    if CHECK and stale:
        print("stale: " + ", ".join(stale))
        sys.exit(1)
    print(("checked " if CHECK else "wrote ") + ", ".join(out))


if __name__ == "__main__":
    main()
