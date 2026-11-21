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
#         textile/pharma/usmca held-out result files, both windows, and their
#         post-cutoff splits (all-regimes report, D210)
# Writes: eval/results/accuracy-v2-metrics.json
#         backend/internal/calibrate/map.json (cal-v2 map and bar, D160; embedded in the server)
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
CAP = 0.99  # highest calibrated probability the layer will state

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
    "argon_rules_v2_gateway": R + "dev-gemini-4-argon-prec-engine-v1-litellm-rules-v2.jsonl",
}
BASELINE = "argon_vertex"
# The candidate pre-registered in D153 and its held-out runs (D154).
CANDIDATE = {"decision": "D153", "engine": "engine-v1", "model": "gemini-4-argon", "retrieval": "bm25", "k": 5,
             "calibration": "isotonic map fitted on " + "eval/results/dev-gemini-4-argon-prec-engine-v1-vertex.jsonl"}
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


def ece_on_partition(rs, value_key, partition_key):
    """ECE with value_key as the probability, grouped by partition_key.

    The before/after ECE in calib() uses one bin per emitted value, so the two
    sides do not share a partition when the map pools levels. Scoring the
    stated values on the calibrated partition separates probability quality
    from the number of bins (issue #397)."""
    tot = 0.0
    for level in {r[partition_key] for r in rs}:
        b = [r for r in rs if r[partition_key] == level]
        pred = sum(r[value_key] for r in b) / len(b)
        obs = sum(r["ok"] for r in b) / len(b)
        tot += len(b) * abs(obs - pred)
    return tot / len(rs)


def level_ties(rs, value_key="conf"):
    """How many stated values each calibrated level merges, and how many rows."""
    out = []
    for level in sorted({r["cal"] for r in rs}):
        b = [r for r in rs if r["cal"] == level]
        out.append({"calibrated": level, "n": len(b),
                    "stated": sorted({r[value_key] for r in b})})
    return out


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
    # Capped at 0.99: no errors in a block of a hundred cases is not certainty.
    return [{"from": b["lo"], "to_incl": b["hi"], "n": b["n"], "correct": b["k"],
             "calibrated": r4(min(CAP, b["k"] / b["n"]))} for b in merged]


MIN_BLOCK = 20  # cal-v2 (D160): blocks smaller than this are pooled before the fit
CAL_TARGET, CAL_MIN_AUTO = 0.96, 100  # calibrated auto-accept bar rule (D160)
CAL_VERSION = "cal-v2"


def isotonic_fit_min(rs, min_n=MIN_BLOCK):
    """cal-v2: as isotonic_fit, but adjacent confidence values are pooled
    (lowest first) until each block holds at least min_n cases; a short last
    block joins the one below. Then pool-adjacent-violators."""
    levels = sorted({r["conf"] for r in rs})
    blocks, cur = [], None
    for c in levels:
        b = [r for r in rs if r["conf"] == c]
        if cur is None:
            cur = {"lo": c, "hi": c, "n": 0, "k": 0}
        cur["hi"] = c
        cur["n"] += len(b)
        cur["k"] += sum(r["ok"] for r in b)
        if cur["n"] >= min_n:
            blocks.append(cur)
            cur = None
    if cur is not None:
        if blocks:
            last = blocks[-1]
            blocks[-1] = {"lo": last["lo"], "hi": cur["hi"], "n": last["n"] + cur["n"], "k": last["k"] + cur["k"]}
        else:
            blocks.append(cur)
    merged = []
    for b in blocks:
        merged.append(dict(b))
        while len(merged) > 1 and merged[-2]["k"] / merged[-2]["n"] > merged[-1]["k"] / merged[-1]["n"]:
            x = merged.pop()
            y = merged.pop()
            merged.append({"lo": y["lo"], "hi": x["hi"], "n": x["n"] + y["n"], "k": x["k"] + y["k"]})
    return [{"from": b["lo"], "to_incl": b["hi"], "n": b["n"], "correct": b["k"],
             "calibrated": r4(min(CAP, b["k"] / b["n"]))} for b in merged]


def calibrated_bar(rs):
    """D160: the lowest calibrated step at which the in-scope cases at or
    above it are at least CAL_TARGET right, with at least CAL_MIN_AUTO cases.
    Returns the bar and the dev curve over every step."""
    curve, bar = [], None
    for b in sorted({r["cal"] for r in rs}, reverse=True):
        acc = [r for r in rs if not r["err"] and r["in_scope"] and r["cal"] >= b]
        k = sum(r["ok"] for r in acc)
        p, lo, hi = wilson(k, len(acc))
        curve.append({"bar": b, "auto": len(acc), "correct": k, "coverage_pct": pct(len(acc) / len(rs)),
                      "accuracy_pct": pct(p), "ci95_pct": [pct(lo), pct(hi)]})
        if len(acc) >= CAL_MIN_AUTO and p >= CAL_TARGET:
            bar = b
    return bar, curve


def at_bar(rows, bar, key="cal"):
    rs = list(rows.values())
    acc = [r for r in rs if not r["err"] and r["in_scope"] and r[key] >= bar]
    k = sum(r["ok"] for r in acc)
    p, lo, hi = wilson(k, len(acc))
    return {"n": len(rs), "auto": len(acc), "correct": k, "wrong": len(acc) - k, "coverage_pct": pct(len(acc) / len(rs)),
            "accuracy_pct": pct(p), "ci95_pct": [pct(lo), pct(hi)]}


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


# ---------------------------------------------------------------- held-out helpers

FINAL2_RUN = R + "final2-gemini-4-argon-prec-engine-v1-litellm-router.jsonl"
ALL_REGIMES = {
    "substantial_transformation": (R + "final-gemini-4-argon-prec-engine-v1-vertex.jsonl", "Vertex AI"),
    "textile_102_21": (R + "textile-final-gemini-4-argon-prec-textile-v1-vertex.jsonl", "Vertex AI"),
    "pharmaceutical": (R + "pharma-final-gemini-4-argon-prec-pharma-v1-litellm.jsonl", "proxy"),
    "part102_marking": (R + "usmca-final-gemini-4-argon-prec-usmca-v1-litellm.jsonl", "proxy"),
}
# The second window of each regime (D210): frozen post-cutoff splits, each
# scored once through the gateway with the production engine of its regime.
ALL_REGIMES_FINAL2 = {
    "substantial_transformation": (R + "final2-gemini-4-argon-prec-engine-v1-litellm-router.jsonl", "gateway, engine-v1", "eval/splits/splits_final2.json"),
    "textile_102_21": (R + "textile-final2-gemini-4-argon-prec-textile-v2-litellm-reasoning-v1-router.jsonl", "gateway, textile-v2", "eval/splits/splits_textile_final2.json"),
    "pharmaceutical": (R + "pharma-final2-gemini-4-argon-prec-pharma-v1-litellm-router.jsonl", "gateway, pharma-v1", "eval/splits/splits_pharma_final2.json"),
    "part102_marking": (R + "usmca-final2-gemini-4-argon-prec-usmca-v1-litellm-router.jsonl", "gateway, usmca-v1", "eval/splits/splits_usmca_final2.json"),
}


def scored_router(p):
    """A run made with -router: the scope label is on the row itself."""
    scope = {r["ruling"]: r.get("in_scope", True) for r in jsonl(p)}
    return scored(p, scope)


def last_rows(p):
    """One row per ruling, the last one (a resumed run can repeat a ruling)."""
    out = {}
    for r in jsonl(p):
        out[r["ruling"]] = r
    return out


def regime_block(rows, k=None):
    n = len(rows)
    k = sum(1 for r in rows.values() if r["correct"] and not r.get("error")) if k is None else k
    a, lo, hi = wilson(k, n)
    return {"n": n, "correct": k, "accuracy_pct": pct(a), "ci95_pct": [pct(lo), pct(hi)]}


def all_regimes_by_window():
    """D210: every regime's held-out run in both windows. "sum" adds the
    splits as they are; "dedup" counts each ruling once, a ruling in both the
    textile and the Part 102 split under Part 102 (fixed before the runs)."""
    windows = {
        "first_window_2026_jan_sep": {k: (p, via, None) for k, (p, via) in ALL_REGIMES.items()},
        "second_window_2026_sep_nov": ALL_REGIMES_FINAL2,
    }
    out = {"dedup_rule": "a ruling in both the textile and the Part 102 split counts once, under Part 102 (D210)"}
    tot = {"sum": [0, 0], "dedup": [0, 0]}
    for w, runs in windows.items():
        rs = {k: last_rows(p) for k, (p, _, _) in runs.items()}
        block = {}
        for k, (p, via, sp) in runs.items():
            b = {"file": p, "via": via, **regime_block(rs[k])}
            if sp is not None:
                want = set(json.load(open(path(sp)))["final2"])
                assert set(rs[k]) == want, f"{p}: rows differ from {sp}"
                b["split"] = sp
            block[k] = b
        both = sorted(set(rs["textile_102_21"]) & set(rs["part102_marking"]))
        tex_only = {i: r for i, r in rs["textile_102_21"].items() if i not in both}
        block["textile_not_in_part102"] = regime_block(tex_only)
        block["textile_and_part102_overlap"] = {
            "n": len(both),
            "textile_engine_correct": sum(1 for i in both if rs["textile_102_21"][i]["correct"]),
            "part102_engine_correct": sum(1 for i in both if rs["part102_marking"][i]["correct"]),
        }
        sn = sum(len(v) for v in rs.values())
        sk = sum(b["correct"] for k, b in block.items() if k in runs)
        dn = sn - len(both)
        dk = sk - block["textile_and_part102_overlap"]["textile_engine_correct"]
        block["pooled_sum"] = regime_block({i: None for i in range(sn)}, sk)
        block["pooled_dedup"] = regime_block({i: None for i in range(dn)}, dk)
        for key, (n, k) in (("sum", (sn, sk)), ("dedup", (dn, dk))):
            tot[key][0] += n
            tot[key][1] += k
        out[w] = block
    for key in ("sum", "dedup"):
        n, k = tot[key]
        out["all_windows_" + key] = regime_block({i: None for i in range(n)}, k)
    return out


def final2_composition():
    sp = json.load(open(path("eval/splits/splits_final2.json")))
    return {k: sp[k] for k in ("frozen_at", "date_min", "date_max", "funnel", "collections", "label_method")}


# ---------------------------------------------------------------- svg


def f(x):
    return f"{x:.1f}"


def text(x, y, s, size=12, fill=DIM, anchor="start", weight="400"):
    return (f'<text x="{f(x)}" y="{f(y)}" font-size="{size}" fill="{fill}" '
            f'text-anchor="{anchor}" font-weight="{weight}">{s}</text>')


def svg_wrap(w, h, title, desc, body):
    return (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {w} {h}" width="{w}" height="{h}" '
            f'role="img" aria-labelledby="t d" font-family="{FONT}">\n'
            f'<title id="t">{title}</title>\n<desc id="d">{desc}</desc>\n'
            f'<rect width="{w}" height="{h}" fill="{SHEET}"/>\n' + "\n".join(body) + "\n</svg>\n")


SWEEP_LABELS = [
    ("argon_vertex", "Gemini 4 Argon, Vertex AI (in use)"),
    ("argon_gateway", "Gemini 4 Argon, gateway"),
    ("flash38_vertex", "Gemini 3.8 Flash, Vertex AI"),
    ("pro31_gateway", "Gemini 3.1 Pro preview, gateway"),
    ("carbon_gateway", "Gemini 4 Carbon, gateway"),
    ("argon_rules_v2_gateway", "Argon + rules-v2 prompt, gateway"),
]


def sweep_svg(metrics):
    """Dev accuracy per model with 95% Wilson intervals (a dot and a bar)."""
    rows = [(k, lbl, metrics["dev"][k]) for k, lbl in SWEEP_LABELS if k in metrics["dev"]]
    W, H = 760, 120 + 52 * len(rows)
    L, R2, T = 300, 40, 96
    x0, x1 = 80.0, 96.0

    def X(a):
        return L + (W - L - R2) * (a - x0) / (x1 - x0)

    body = [text(24, 26, "Development set accuracy by model, engine-v1", 15, INK, weight="600"),
            text(24, 46, "Same prompt, retrieval and K = 5; 463 rulings after scoring adjustments. Bars: 95% Wilson interval.", 11, DIM),
            text(24, 62, "A failed call (no answer) counts as wrong.", 11, DIM)]
    for a in range(int(x0), int(x1) + 1, 2):
        body.append(f'<line x1="{f(X(a))}" x2="{f(X(a))}" y1="{T - 12}" y2="{H - 40}" stroke="{GRID}" stroke-width="1"/>')
        body.append(text(X(a), H - 22, f"{a}%", 11, FAINT, "middle"))
    for i, (k, lbl, v) in enumerate(rows):
        y = T + 52 * i + 16
        lo, hi = v["ci95_pct"]
        color = HELD if k == "argon_vertex" else ALT
        body.append(text(24, y + 4, lbl, 12, INK))
        extra = f"{v['errors']} failed calls" if v["errors"] else "no failed calls"
        body.append(text(24, y + 20, f"{v['correct']}/{v['n']} · ${v['cost_per_case_usd']:.4f}/case · {extra}", 10, DIM))
        body.append(f'<line x1="{f(X(max(lo, x0)))}" x2="{f(X(min(hi, x1)))}" y1="{y}" y2="{y}" stroke="{DEV}" stroke-width="3"/>')
        body.append(f'<circle cx="{f(X(v["accuracy_pct"]))}" cy="{y}" r="6" fill="{color}" stroke="{SHEET}" stroke-width="2"/>')
        body.append(text(X(v["accuracy_pct"]), y - 12, f"{v['accuracy_pct']}%", 11, INK, "middle", "600"))
    return svg_wrap(W, H, "Development set accuracy by model",
                    "Accuracy of engine-v1 on the 463-ruling development set for each model, with 95% intervals. "
                    "Gemini 4 Argon is highest; Carbon and 3.1 Pro preview are lower and fail on some calls.", body)


def calibration_svg(fin, m):
    """Held-out reliability before and after the dev-fitted isotonic layer."""
    W, H = 760, 470
    L, R2, T, B = 64, 24, 104, 64
    pw, ph = W - L - R2, H - T - B
    x0, x1 = 0.55, 1.00

    def X(c):
        return L + pw * (c - x0) / (x1 - x0)

    def Y(a):
        return T + ph * (100 - a) / 100

    rs = list(with_calibrated(fin, m).values())
    before, after = calib(rs, "conf"), calib(rs, "cal")
    body = [text(L, 26, "Held-out calibration (377 rulings), before and after the layer", 15, INK, weight="600"),
            text(L, 46, "Calibration map fitted on the development set only (isotonic, one step per emitted confidence).", 11, DIM),
            text(L, 62, f"Before: ECE {before['ece_levels']:.3f}, Brier {before['brier']:.3f}. "
                        f"After: ECE {after['ece_levels']:.3f}, Brier {after['brier']:.3f}.", 11, DIM)]
    for a in range(0, 101, 20):
        body.append(f'<line x1="{L}" x2="{W - R2}" y1="{f(Y(a))}" y2="{f(Y(a))}" stroke="{GRID}" stroke-width="1"/>')
        body.append(text(L - 8, Y(a) + 4, f"{a}%", 11, FAINT, "end"))
    for c in (0.6, 0.7, 0.8, 0.9, 1.0):
        body.append(text(X(c), T + ph + 18, f"{c:.1f}", 11, FAINT, "middle"))
    body.append(text(L + pw / 2, H - 18, "Stated confidence, or calibrated probability", 12, DIM, "middle"))
    body.append(f'<circle cx="{L + 8}" cy="80" r="5" fill="{SHEET}" stroke="{DEV}" stroke-width="2"/>')
    body.append(text(L + 20, 84, "before: stated confidence (engine-v1)", 11, INK))
    body.append(f'<circle cx="{L + 330}" cy="80" r="5" fill="{HELD}" stroke="{SHEET}" stroke-width="2"/>')
    body.append(text(L + 342, 84, "after: calibrated probability", 11, INK))
    body.append(text(W - R2, T + ph - 8, "points with n &lt; 5 are drawn smaller", 10, FAINT, "end"))
    body.append(f'<line x1="{f(X(x0))}" y1="{f(Y(x0 * 100))}" x2="{f(X(x1))}" y2="{f(Y(100))}" stroke="{FAINT}" stroke-width="1.5" stroke-dasharray="2 4"/>')
    body.append(text(X(0.62), Y(62) - 10, "stated = observed", 11, FAINT, "middle"))

    def levels(key):
        out = []
        for c in sorted({r[key] for r in rs}):
            b = [r for r in rs if r[key] == c]
            out.append((c, len(b), sum(r["ok"] for r in b)))
        return out

    for c, n, k in levels("conf"):
        if c < x0:
            continue
        body.append(f'<circle cx="{f(X(c))}" cy="{f(Y(100 * k / n))}" r="{5 if n >= 5 else 3}" fill="{SHEET}" stroke="{DEV}" stroke-width="2"/>')
    for c, n, k in levels("cal"):
        if c < x0:
            continue
        body.append(f'<circle cx="{f(X(c))}" cy="{f(Y(100 * k / n))}" r="{5 if n >= 5 else 3}" fill="{HELD}" stroke="{SHEET}" stroke-width="2"/>')
    return svg_wrap(W, H, "Calibration layer on the held-out split",
                    "Observed accuracy against stated confidence before calibration and against calibrated probability after, "
                    "on the 377 held-out rulings. The map was fitted on the development set.", body)


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
    if "argon_gateway" in dev:
        metrics["dev_vs_argon_gateway"] = {n: mcnemar(dev["argon_gateway"], dev[n]) for n in dev if n != "argon_gateway"}
    # Carbon on the cases it answered (its failures are budget exhaustion).
    if "carbon_gateway" in dev:
        c = dev["carbon_gateway"]
        ans = {i: r for i, r in c.items() if not r["err"]}
        metrics["carbon_answered_vs_argon_gateway"] = mcnemar({i: dev["argon_gateway"][i] for i in ans}, ans)
        metrics["carbon_answered_vs_argon_gateway"]["carbon_correct"] = sum(r["ok"] for r in ans.values())
        metrics["carbon_answered_vs_argon_gateway"]["argon_correct"] = sum(dev["argon_gateway"][i]["ok"] for i in ans)

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

    # The calibration layer for the engine in production (engine-v1, Argon,
    # Vertex): fitted on the development rows only, then applied once to the
    # held-out rows that already exist. No engine or bar changes.
    scope_final = {r["ruling"]: r["in_scope"] for r in jsonl(R + "scope-final-router-v2-vertex.jsonl")}
    fin = scored(R + "final-gemini-4-argon-prec-engine-v1-vertex.jsonl", scope_final)
    m = metrics["calibration_maps"][BASELINE]
    fin_c = list(with_calibrated(fin, m).values())
    dev_c = list(with_calibrated(dev[BASELINE], m).values())
    metrics["calibration_layer_engine_v1"] = {
        "fitted_on": DEV_RUNS[BASELINE], "applied_to": R + "final-gemini-4-argon-prec-engine-v1-vertex.jsonl",
        "method": "isotonic regression (pool-adjacent-violators), one block per emitted confidence value",
        "map": m,
        "heldout_before": calib(fin_c, "conf"), "heldout_after": calib(fin_c, "cal"),
        "heldout_stated_on_calibrated_bins": r4(ece_on_partition(fin_c, "conf", "cal")),
        "heldout_level_counts": {"stated": len({r["conf"] for r in fin_c}),
                                 "calibrated": len({r["cal"] for r in fin_c})},
        "heldout_calibrated_levels": level_ties(fin_c),
        "dev_in_sample_after": calib(dev_c, "cal"),
    }

    # cal-v2 and the calibrated auto-accept bar (D160): fitted on dev only,
    # then applied unchanged to the committed held-out rows.
    m2 = isotonic_fit_min(list(dev[BASELINE].values()))
    dev2 = with_calibrated(dev[BASELINE], m2)
    bar2, curve2 = calibrated_bar(list(dev2.values()))
    v2 = {"version": CAL_VERSION, "fitted_on": DEV_RUNS[BASELINE], "min_block": MIN_BLOCK, "cap": CAP,
          "method": "isotonic regression (pool-adjacent-violators) after pooling adjacent confidence values into blocks of at least "
                    + str(MIN_BLOCK) + " cases",
          "map": m2, "bar_rule": f"lowest step with in-scope dev accuracy >= {CAL_TARGET:.0%} on at least {CAL_MIN_AUTO} cases",
          "bar": bar2, "dev_curve": curve2,
          "dev": {"at_bar": at_bar(dev2, bar2), "raw_092": at_bar(dev[BASELINE], BAR, "conf"),
                  "in_sample_after": calib(list(dev2.values()), "cal")}}
    fin2 = with_calibrated(fin, m2)
    v2["final"] = {"at_bar": at_bar(fin2, bar2), "raw_092": at_bar(fin, BAR, "conf"),
                   "before": calib(list(fin2.values()), "conf"), "after": calib(list(fin2.values()), "cal")}
    if exists(FINAL2_RUN):
        f2r = scored_router(FINAL2_RUN)
        f2v = with_calibrated(f2r, m2)
        v2["final2"] = {"at_bar": at_bar(f2v, bar2), "raw_092": at_bar(f2r, BAR, "conf"),
                        "before": calib(list(f2v.values()), "conf"), "after": calib(list(f2v.values()), "cal")}
        pv = dict(fin2)
        pv.update(f2v)
        pr = dict(fin)
        pr.update(f2r)
        v2["pooled_final_final2"] = {"at_bar": at_bar(pv, bar2), "raw_092": at_bar(pr, BAR, "conf"),
                                     "before": calib(list(pv.values()), "conf"), "after": calib(list(pv.values()), "cal")}
    metrics["calibration_v2"] = v2
    calmap = {"_doc": "Calibration map for engine-v1 on gemini-4-argon (substantial transformation), D160. Generated by "
                      "eval/charts/build_accuracy.py from the development rows only; do not edit by hand.",
              "version": CAL_VERSION, "engine_prompt": "engine-v1", "model": "gemini-4-argon", "fitted_on": DEV_RUNS[BASELINE],
              "rule": v2["method"] + f", capped at {CAP}; a value between steps takes the step at or below it; "
                      "a value under the lowest step takes the lowest step",
              "bar": bar2, "bar_rule": v2["bar_rule"],
              "steps": [{"from": b["from"], "calibrated": b["calibrated"], "n": b["n"], "correct": b["correct"]} for b in m2]}

    # Held-out: the first split (377, Vertex, official), the second split
    # (D150) when its single scored run exists, and both pooled.
    held = {"final": {"file": R + "final-gemini-4-argon-prec-engine-v1-vertex.jsonl", **summary(fin)}}
    f2 = None
    if exists(FINAL2_RUN):
        f2 = scored_router(FINAL2_RUN)
        f2c = list(with_calibrated(f2, m).values())
        held["final2"] = {"file": FINAL2_RUN, **summary(f2)}
        held["final2"]["calibration_layer"] = {"before": calib(f2c, "conf"), "after": calib(f2c, "cal")}
        pooled = dict(fin)
        pooled.update(f2)
        held["pooled_final_final2"] = {"file": "both rows above", **summary(pooled)}
        pc = list(with_calibrated(pooled, m).values())
        held["pooled_final_final2"]["calibration_layer"] = {"before": calib(pc, "conf"), "after": calib(pc, "cal")}
        held["final2"]["selective"] = selective(f2)
        held["final2"]["composition"] = final2_composition()
    metrics["heldout"] = held

    # Every regime's held-out run, first window, and pooled. The pharmaceutical
    # and Part 102 runs went through the proxy (docs/NUMBERS.md).
    regimes = {}
    tot_n = tot_k = 0
    for name, (p, via) in ALL_REGIMES.items():
        rs = jsonl(p)
        k = sum(1 for r in rs if r["correct"] and not r.get("error"))
        a, lo, hi = wilson(k, len(rs))
        regimes[name] = {"file": p, "via": via, "n": len(rs), "correct": k, "accuracy_pct": pct(a), "ci95_pct": [pct(lo), pct(hi)]}
        tot_n += len(rs)
        tot_k += k
    a, lo, hi = wilson(tot_k, tot_n)
    regimes["pooled"] = {"n": tot_n, "correct": tot_k, "accuracy_pct": pct(a), "ci95_pct": [pct(lo), pct(hi)]}
    if f2 is not None:
        k2 = sum(r["ok"] for r in f2.values())
        a, lo, hi = wilson(tot_k + k2, tot_n + len(f2))
        regimes["pooled_with_final2"] = {"n": tot_n + len(f2), "correct": tot_k + k2, "accuracy_pct": pct(a), "ci95_pct": [pct(lo), pct(hi)]}
    metrics["heldout_all_regimes"] = regimes
    metrics["heldout_all_regimes_by_window"] = all_regimes_by_window()

    if CANDIDATE:
        metrics["candidate"] = CANDIDATE
    out = {"eval/results/accuracy-v2-metrics.json": json.dumps(metrics, indent=1, ensure_ascii=False) + "\n",
           "docs/img/model-sweep-dev.svg": sweep_svg(metrics),
           "docs/img/calibration-layer.svg": calibration_svg(fin, m),
           "backend/internal/calibrate/map.json": json.dumps(calmap, indent=1) + "\n"}
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
