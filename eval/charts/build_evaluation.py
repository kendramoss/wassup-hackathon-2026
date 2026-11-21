# Builds the figures and numbers in docs/EVALUATION.md from committed files only.
# No model calls, no network, standard library only. Output is deterministic.
#
# Usage (from the repo root):
#   python3 -I eval/charts/build_evaluation.py .          # write the outputs
#   python3 -I eval/charts/build_evaluation.py . --check  # fail if the outputs are stale
#
# Reads:  eval/data/labels.jsonl, eval/data/scoring_adjustments.json,
#         eval/splits/splits_final.json, eval/splits/splits_tune.json,
#         eval/results/{final,dev}-gemini-4-argon-prec-engine-v1-vertex.jsonl,
#         eval/results/scope-{final,dev}-router-v2-vertex.jsonl
# Writes: eval/results/evaluation-metrics.json
#         docs/img/selective-accuracy.svg
#         docs/img/reliability-final.svg

import json
import math
import random
import sys

ROOT = sys.argv[1] if len(sys.argv) > 1 else "."
CHECK = "--check" in sys.argv[2:]

SEED = 20261017  # the same seed the model calls use (backend/internal/llm, D76)
BOOTSTRAP = 2000
BAR = 0.92
BASELINE = 60.5  # most-mentioned country, all 377; reported, unverified (docs/NUMBERS.md)

# Palette tokens from frontend/src/app/globals.css.
SHEET, GRID, LINE, WASH = "#f9fbfe", "#d3dff0", "#c4d0e2", "#e2e9f3"
INK, DIM, FAINT = "#13213a", "#47566f", "#6e7c94"
HELD, DEV = "#1d4fc4", "#8fa7d6"  # --ok, --guide
FONT = "'Paper Mono', ui-monospace, SFMono-Regular, Menlo, monospace"


def path(p):
    return f"{ROOT}/{p}"


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


# ---------------------------------------------------------------- funnel


def funnel():
    labels = jsonl("eval/data/labels.jsonl")
    final = json.load(open(path("eval/splits/splits_final.json")))
    tune = json.load(open(path("eval/splits/splits_tune.json")))
    used = set(final["final"]) | set(final["dev"]) | set(final["pilot"])
    tune_ids = set(tune["train"]) | set(tune["tune_check"])

    st = [r for r in labels if r["regime"] == "substantial_transformation"]
    st_multi = [r for r in st if r["multi_country"]]
    held = [r for r in st_multi if r["primary_origin"]]

    def window(rows, lo, hi):
        return [r for r in rows if lo <= r["date"] <= hi]

    windows = {
        "before_dev": ("0000-00-00", "2025-06-30"),
        "dev": ("2025-07-01", "2025-12-31"),
        "final": ("2026-01-01", "2026-12-31"),
    }
    out = {
        "labelled_rulings": len(labels),
        "labelled_date_min": min(r["date"] for r in labels),
        "labelled_date_max": max(r["date"] for r in labels),
        "with_any_held_origin": sum(1 for r in labels if r["primary_origin"]),
        "substantial_transformation": len(st),
        "substantial_transformation_multi_country": len(st_multi),
        "with_held_origin": len(held),
        "windows": {},
    }
    for name, (lo, hi) in windows.items():
        w = window(held, lo, hi)
        multi = [r for r in w if "|" in r["primary_origin"]]
        single = [r for r in w if "|" not in r["primary_origin"]]
        row = {
            "from": lo if name != "before_dev" else min(r["date"] for r in w),
            "to": hi,
            "eligible": len(w),
            "excluded_two_or_more_countries_held": len(multi),
            "single_origin": len(single),
        }
        if name == "before_dev":
            row["in_splits_tune"] = sum(1 for r in w if r["ruling"] in tune_ids)
            row["in_splits_tune_with_two_or_more_countries"] = sum(
                1 for r in multi if r["ruling"] in tune_ids
            )
        else:
            row["single_origin_not_in_splits"] = sum(
                1 for r in single if r["ruling"] not in used
            )
            row["excluded_but_in_splits"] = sum(1 for r in multi if r["ruling"] in used)
        out["windows"][name] = row
    by_id = {r["ruling"]: r for r in labels}
    for name in ("dev", "final", "pilot"):
        ids = final[name]
        rows = [by_id[i] for i in ids]
        out[f"split_{name}"] = {
            "n": len(ids),
            "date_min": min(r["date"] for r in rows),
            "date_max": max(r["date"] for r in rows),
            "ny": sum(1 for r in rows if r["collection"] == "NY"),
            "hq": sum(1 for r in rows if r["collection"] == "HQ"),
            "label_regex": sum(1 for r in rows if r["method"] == "regex"),
            "label_manual": sum(1 for r in rows if r["method"] == "manual"),
        }
    out["split_tune"] = {"train": len(tune["train"]), "tune_check": len(tune["tune_check"])}
    return out


# ---------------------------------------------------------------- scoring


def countries(pred):
    return [p.strip() for p in pred.replace("|", "/").split("/") if p.strip()]


def scored_rows(split):
    """Per-case rows with scoring adjustments applied (D23), as evalkit.Apply does."""
    adj = json.load(open(path("eval/data/scoring_adjustments.json")))
    rows = jsonl(f"eval/results/{split}-gemini-4-argon-prec-engine-v1-vertex.jsonl")
    scope = {r["ruling"]: r["in_scope"] for r in jsonl(f"eval/results/scope-{split}-router-v2-vertex.jsonl")}
    out = []
    for r in rows:
        if r["ruling"] in adj["exclude"]:
            continue
        ok = bool(r["correct"]) and not r.get("error")
        alts = adj["alternatives"].get(r["ruling"])
        if alts is not None and not r.get("error"):
            parts = countries(r["pred"])
            ok = len(parts) > 0 and all(p in alts for p in parts)
        out.append({"conf": float(r["confidence"]), "ok": ok, "in_scope": scope[r["ruling"]]})
    return out, len(rows)


def selective(rows):
    """Accept when confidence >= t, for every confidence the model emitted."""
    n = len(rows)
    pts = []
    for t in sorted({r["conf"] for r in rows}, reverse=True):
        acc = [r for r in rows if r["conf"] >= t]
        k = sum(r["ok"] for r in acc)
        p, lo, hi = wilson(k, len(acc))
        pts.append({
            "threshold": t, "accepted": len(acc), "correct": k,
            "coverage_pct": pct(len(acc) / n), "accuracy_pct": pct(p),
            "ci_lo_pct": pct(lo), "ci_hi_pct": pct(hi),
        })
    return pts


def calibration(rows):
    n = len(rows)
    levels = []
    for c in sorted({r["conf"] for r in rows}):
        b = [r for r in rows if r["conf"] == c]
        k = sum(r["ok"] for r in b)
        p, lo, hi = wilson(k, len(b))
        levels.append({"confidence": c, "n": len(b), "correct": k,
                       "accuracy_pct": pct(p), "ci_lo_pct": pct(lo), "ci_hi_pct": pct(hi)})

    def ece_levels(rs):
        tot = 0.0
        for c in {r["conf"] for r in rs}:
            b = [r for r in rs if r["conf"] == c]
            tot += len(b) * abs(sum(r["ok"] for r in b) / len(b) - c)
        return tot / len(rs)

    def ece_bins(rs, m=10):
        bins = [[] for _ in range(m)]
        for r in rs:
            bins[min(int(r["conf"] * m), m - 1)].append(r)
        tot = 0.0
        for b in bins:
            if b:
                tot += len(b) * abs(sum(r["ok"] for r in b) / len(b) - sum(r["conf"] for r in b) / len(b))
        return tot / len(rs)

    def brier(rs):
        return sum((r["conf"] - (1.0 if r["ok"] else 0.0)) ** 2 for r in rs) / len(rs)

    rng = random.Random(SEED)
    boot = {"ece_levels": [], "ece_10_bins": [], "brier": []}
    for _ in range(BOOTSTRAP):
        s = [rows[rng.randrange(n)] for _ in range(n)]
        boot["ece_levels"].append(ece_levels(s))
        boot["ece_10_bins"].append(ece_bins(s))
        boot["brier"].append(brier(s))

    def ci(v):
        v = sorted(v)
        return [round(v[int(0.025 * len(v))], 4), round(v[int(0.975 * len(v)) - 1], 4)]

    acc = sum(r["ok"] for r in rows) / n
    return {
        "n": n,
        "accuracy_pct": pct(acc),
        "mean_confidence_pct": pct(sum(r["conf"] for r in rows) / n),
        "ece_levels": round(ece_levels(rows), 4),
        "ece_levels_ci95": ci(boot["ece_levels"]),
        "ece_10_bins": round(ece_bins(rows), 4),
        "ece_10_bins_ci95": ci(boot["ece_10_bins"]),
        "brier": round(brier(rows), 4),
        "brier_ci95": ci(boot["brier"]),
        "brier_constant_forecast": round(acc * (1 - acc), 4),
        "levels": levels,
    }


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


def selective_svg(final_pts, dev_pts, n_final, n_dev):
    W, H = 760, 500
    L, R, T, B = 64, 24, 104, 64
    pw, ph = W - L - R, H - T - B
    y0, y1 = 55.0, 100.0

    def X(c):
        return L + pw * c / 100

    def Y(a):
        return T + ph * (y1 - a) / (y1 - y0)

    body = [text(L, 26, "Accuracy of accepted cases as the confidence bar moves", 15, INK, weight="600"),
            text(L, 46, "Accept a case when its stated confidence is at least t, for each t the model emitted.", 11, DIM)]
    # Legend (two series), above the plot.
    lx, ly = L, 70
    body.append(f'<line x1="{lx}" x2="{lx + 26}" y1="{ly}" y2="{ly}" stroke="{HELD}" stroke-width="2"/>')
    body.append(text(lx + 34, ly + 4, f"Held-out 2026, n = {n_final}; shaded: 95% Wilson interval", 11, INK))
    body.append(f'<line x1="{lx}" x2="{lx + 26}" y1="{ly + 18}" y2="{ly + 18}" stroke="{DEV}" stroke-width="2" stroke-dasharray="6 4"/>')
    body.append(text(lx + 34, ly + 22, f"Development set Jul–Dec 2025, n = {n_dev} after scoring adjustments; the bar was chosen here", 11, INK))

    for a in range(60, 101, 10):
        body.append(f'<line x1="{L}" x2="{W - R}" y1="{f(Y(a))}" y2="{f(Y(a))}" stroke="{GRID}" stroke-width="1"/>')
        body.append(text(L - 8, Y(a) + 4, f"{a}%", 11, FAINT, "end"))
    for c in range(0, 101, 20):
        body.append(text(X(c), T + ph + 18, f"{c}%", 11, FAINT, "middle"))
    body.append(f'<line x1="{L}" x2="{W - R}" y1="{T + ph}" y2="{T + ph}" stroke="{LINE}" stroke-width="1"/>')
    body.append(text(L + pw / 2, H - 18, "Share of cases accepted without review (coverage)", 12, DIM, "middle"))
    body.append(text(16, T + ph / 2, "Accuracy of accepted cases", 12, DIM, "middle").replace(
        "<text ", f'<text transform="rotate(-90 16 {f(T + ph / 2)})" '))

    # Held-out Wilson band.
    up = " ".join(f"{f(X(p['coverage_pct']))},{f(Y(min(p['ci_hi_pct'], y1)))}" for p in final_pts)
    dn = " ".join(f"{f(X(p['coverage_pct']))},{f(Y(max(p['ci_lo_pct'], y0)))}" for p in reversed(final_pts))
    body.append(f'<polygon points="{up} {dn}" fill="{WASH}" stroke="none"/>')

    # Baseline: one figure on all 377 cases.
    body.append(f'<line x1="{L}" x2="{W - R}" y1="{f(Y(BASELINE))}" y2="{f(Y(BASELINE))}" stroke="{FAINT}" stroke-width="1.5" stroke-dasharray="2 4"/>')
    body.append(text(W - R, Y(BASELINE) - 8, f"Most-mentioned-country rule on all 377: {BASELINE}% (reported, unverified)", 11, DIM, "end"))

    def line(pts, color, dash):
        d = " ".join(f"{f(X(p['coverage_pct']))},{f(Y(p['accuracy_pct']))}" for p in pts)
        extra = f' stroke-dasharray="{dash}"' if dash else ""
        return f'<polyline points="{d}" fill="none" stroke="{color}" stroke-width="2" stroke-linejoin="round"{extra}/>'

    body.append(line(dev_pts, DEV, "6 4"))
    body.append(line(final_pts, HELD, ""))

    def dot(p, color):
        return (f'<circle cx="{f(X(p["coverage_pct"]))}" cy="{f(Y(p["accuracy_pct"]))}" r="4.5" '
                f'fill="{color}" stroke="{SHEET}" stroke-width="2"/>')

    def leader(p, tx, ty):
        return (f'<line x1="{f(X(p["coverage_pct"]))}" y1="{f(Y(p["accuracy_pct"]) + 6)}" '
                f'x2="{f(tx)}" y2="{f(ty)}" stroke="{FAINT}" stroke-width="1"/>')

    fb = next(p for p in final_pts if p["threshold"] == BAR)
    db = next(p for p in dev_pts if p["threshold"] == BAR)
    f95 = next(p for p in final_pts if p["threshold"] == 0.95)
    fa, da = final_pts[-1], dev_pts[-1]

    # Callouts in the empty area below the curves, joined to their points.
    bx, by = X(fb["coverage_pct"]) - 6, Y(80)
    body.append(leader(fb, bx + 6, by - 14))
    body.append(text(bx, by, "0.92 bar (in use)", 11, INK, "end", "600"))
    body.append(text(bx, by + 16, f"held-out: {fb['accepted']} of {n_final} ({fb['coverage_pct']}%) at {fb['accuracy_pct']}%", 11, INK, "end"))
    body.append(text(bx, by + 32, f"development: {db['coverage_pct']}% at {db['accuracy_pct']}%", 11, DIM, "end"))
    sx, sy = X(f95["coverage_pct"]) - 6, Y(88)
    body.append(leader(f95, sx + 6, sy - 14))
    body.append(text(sx, sy, f"0.95: {f95['accepted']} of {n_final} ({f95['coverage_pct']}%) at {f95['accuracy_pct']}%", 11, INK, "end"))
    body.append(text(X(100) - 4, Y(fa["accuracy_pct"]) + 20, f"all held-out: {fa['accuracy_pct']}%", 11, INK, "end"))
    body.append(text(X(100) - 4, Y(da["accuracy_pct"]) - 22, f"all development: {da['accuracy_pct']}%", 11, DIM, "end"))

    for p in (db, da):
        body.append(dot(p, DEV))
    for p in (fb, f95, fa):
        body.append(dot(p, HELD))

    return svg_wrap(W, H, "Selective accuracy: held-out and development",
                    "Accuracy of accepted cases against coverage as the confidence bar moves. "
                    f"At the 0.92 bar the held-out split accepts {fb['coverage_pct']}% of cases at {fb['accuracy_pct']}%.",
                    body)


def reliability_svg(cal):
    W, H = 760, 470
    L, R, T, B = 64, 24, 84, 64
    pw, ph = W - L - R, H - T - B
    x0, x1 = 0.70, 1.00
    y0, y1 = 0.0, 100.0

    def X(c):
        return L + pw * (c - x0) / (x1 - x0)

    def Y(a):
        return T + ph * (y1 - a) / (y1 - y0)

    body = [text(L, 26, "Stated confidence against observed accuracy, held-out", 15, INK, weight="600"),
            text(L, 46, f"One point per confidence value the model emitted, n = {cal['n']}. Bars: 95% Wilson interval.", 11, DIM),
            text(L, 62, f"Mean stated confidence {cal['mean_confidence_pct']}%, accuracy {cal['accuracy_pct']}%. "
                        f"ECE {cal['ece_levels']:.3f}, Brier {cal['brier']:.3f}.", 11, DIM)]
    for a in range(0, 101, 20):
        body.append(f'<line x1="{L}" x2="{W - R}" y1="{f(Y(a))}" y2="{f(Y(a))}" stroke="{GRID}" stroke-width="1"/>')
        body.append(text(L - 8, Y(a) + 4, f"{a}%", 11, FAINT, "end"))
    for c in (0.70, 0.75, 0.80, 0.85, 0.90, 0.95, 1.00):
        body.append(text(X(c), T + ph + 18, f"{c:.2f}", 11, FAINT, "middle"))
    body.append(f'<line x1="{L}" x2="{W - R}" y1="{T + ph}" y2="{T + ph}" stroke="{LINE}" stroke-width="1"/>')
    body.append(text(L + pw / 2, H - 18, "Stated confidence", 12, DIM, "middle"))
    body.append(text(16, T + ph / 2, "Observed accuracy", 12, DIM, "middle").replace(
        "<text ", f'<text transform="rotate(-90 16 {f(T + ph / 2)})" '))
    # Perfect calibration diagonal.
    body.append(f'<line x1="{f(X(x0))}" y1="{f(Y(x0 * 100))}" x2="{f(X(x1))}" y2="{f(Y(100))}" stroke="{FAINT}" stroke-width="1.5" stroke-dasharray="2 4"/>')
    body.append(text(X(0.805), Y(80.5) - 10, "stated = observed", 11, FAINT, "middle"))
    # 0.92 bar.
    body.append(f'<line x1="{f(X(BAR))}" x2="{f(X(BAR))}" y1="{T}" y2="{T + ph}" stroke="{LINE}" stroke-width="1"/>')
    body.append(text(X(BAR) - 6, Y(30), "0.92 bar", 11, INK, "end"))
    body.append(text(L + 4, Y(13), "correct / n", 10, FAINT))
    for i, lv in enumerate(cal["levels"]):
        cx = X(lv["confidence"])
        body.append(f'<line x1="{f(cx)}" x2="{f(cx)}" y1="{f(Y(lv["ci_lo_pct"]))}" y2="{f(Y(lv["ci_hi_pct"]))}" stroke="{DEV}" stroke-width="2"/>')
        body.append(f'<circle cx="{f(cx)}" cy="{f(Y(lv["accuracy_pct"]))}" r="5" fill="{HELD}" stroke="{SHEET}" stroke-width="2"/>')
        label_y = Y(7) if i % 2 == 0 else Y(2)
        body.append(text(cx, label_y, f"{lv['correct']}/{lv['n']}", 10, DIM, "middle"))
    return svg_wrap(W, H, "Reliability diagram, held-out",
                    "Observed accuracy at each stated confidence on the 377 held-out cases, with the diagonal for perfect calibration.",
                    body)


# ---------------------------------------------------------------- main


def main():
    final_rows, final_raw = scored_rows("final")
    dev_rows, dev_raw = scored_rows("dev")
    final_pts, dev_pts = selective(final_rows), selective(dev_rows)

    def fast(rows):
        acc = [r for r in rows if r["conf"] >= BAR and r["in_scope"]]
        k = sum(r["ok"] for r in acc)
        p, lo, hi = wilson(k, len(acc))
        return {"accepted": len(acc), "correct": k, "coverage_pct": pct(len(acc) / len(rows)),
                "accuracy_pct": pct(p), "ci_lo_pct": pct(lo), "ci_hi_pct": pct(hi)}

    metrics = {
        "_doc": "Generated by eval/charts/build_evaluation.py from committed files; do not edit by hand. "
                "Scoring adjustments (D23) applied. Selective points accept on confidence alone; "
                "fast_path_0_92 also requires router-v2 in scope, as production does.",
        "seed": SEED,
        "bootstrap_resamples": BOOTSTRAP,
        "funnel": funnel(),
        "final": {"rows": final_raw, "scored": len(final_rows),
                  "fast_path_0_92": fast(final_rows),
                  "calibration": calibration(final_rows), "selective": final_pts},
        "dev": {"rows": dev_raw, "scored": len(dev_rows),
                "fast_path_0_92": fast(dev_rows),
                "calibration": calibration(dev_rows), "selective": dev_pts},
    }
    outputs = {
        "eval/results/evaluation-metrics.json": json.dumps(metrics, indent=1, ensure_ascii=False) + "\n",
        "docs/img/selective-accuracy.svg": selective_svg(final_pts, dev_pts, len(final_rows), len(dev_rows)),
        "docs/img/reliability-final.svg": reliability_svg(metrics["final"]["calibration"]),
    }
    stale = []
    for p, content in outputs.items():
        if CHECK:
            try:
                same = open(path(p), encoding="utf-8").read() == content
            except FileNotFoundError:
                same = False
            if not same:
                stale.append(p)
        else:
            with open(path(p), "w", encoding="utf-8") as fh:
                fh.write(content)
    if stale:
        print("stale (re-run without --check):", ", ".join(stale))
        sys.exit(1)
    c = metrics["final"]["calibration"]
    print(f"final: ECE {c['ece_levels']} {c['ece_levels_ci95']} | 10-bin {c['ece_10_bins']} | "
          f"Brier {c['brier']} {c['brier_ci95']} (constant {c['brier_constant_forecast']})")
    c = metrics["dev"]["calibration"]
    print(f"dev:   ECE {c['ece_levels']} {c['ece_levels_ci95']} | 10-bin {c['ece_10_bins']} | "
          f"Brier {c['brier']} {c['brier_ci95']} (constant {c['brier_constant_forecast']})")
    print("ok" if CHECK else "wrote " + ", ".join(outputs))


main()
