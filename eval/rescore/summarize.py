"""Record one month's re-scoring result (D931, #884).

Usage: python3 -I -B eval/rescore/summarize.py <repo> --month YYYY-MM [--check]

Reads eval/rescore/<month>/splits.json, the month's refresh reports, and the
RUNLOG.jsonl entry of the CBP run tagged rescore-<month> (and, when present,
the Japan metrics of the run tagged rescore-<month>). Writes
eval/rescore/<month>/result.json: accuracy with a 95% Wilson interval next to
the earlier held-out figures. With --check, recomputes and compares instead.
A month with no cases records cases 0 and no accuracy.
"""
import glob, json, math, os, sys

# Earlier held-out figures, from RUNLOG.jsonl (D154) and D920.
EARLIER = {
    "cbp_final": {"correct": 340, "cases": 377, "source": "D154 (first held-out split, Vertex AI)"},
    "cbp_final2": {"correct": 77, "cases": 83, "source": "D154 (second held-out split)"},
    "jp_fresh_1126": {"correct": 37, "cases": 44, "source": "D920"},
}


def wilson(k, n, z=1.96):
    if n == 0:
        return None
    p = k / n
    d = 1 + z * z / n
    c = (p + z * z / (2 * n)) / d
    h = z * math.sqrt(p * (1 - p) / n + z * z / (4 * n * n)) / d
    return [round(c - h, 4), round(c + h, 4)]


def figure(k, n):
    return {"correct": k, "cases": n, "accuracy": round(k / n, 4) if n else None, "ci95": wilson(k, n)}


def build(repo, month):
    d = os.path.join(repo, "eval/rescore", month)
    split = json.load(open(os.path.join(d, "splits.json"), encoding="utf-8"))
    tag = "rescore-" + month
    runs = []
    p = os.path.join(repo, "eval/results/RUNLOG.jsonl")
    if os.path.exists(p):
        for line in open(p, encoding="utf-8"):
            r = json.loads(line)
            if r.get("split") == "monthly" and str(r.get("run", "")).endswith(tag):
                runs.append(r)
    out = {"month": month, "cbp": {"set": len(split["monthly"]), "date_min": split["date_min"], "date_max": split["date_max"]}}
    if split["monthly"]:
        if len(runs) != 1:
            raise SystemExit(f"want exactly one RUNLOG entry for {tag} on split monthly, found {len(runs)}")
        r = runs[0]
        out["cbp"].update(figure(r["correct"], r["cases"]))
        out["cbp"].update({k: r.get(k) for k in ("run", "git_revision", "engine_model", "prompt_version", "cost_per_case_usd",
                                                    "fast_path_coverage", "fast_path_correct", "latency_p50_ms", "latency_p95_ms")})
    else:
        out["cbp"].update(figure(0, 0))
        out["cbp"]["note"] = "no new rulings passed the D150 rule; nothing scored"
    jp = sorted(glob.glob(os.path.join(repo, "eval/jp", f"jp-fresh_*{tag}-metrics.json")))
    if jp:
        m = json.load(open(jp[0], encoding="utf-8"))
        cm = m.get("criterion_match", {})
        out["japan"] = figure(cm.get("k", 0), cm.get("n", 0)) | {"metrics": os.path.relpath(jp[0], repo)}
    else:
        out["japan"] = figure(0, 0) | {"note": "no new Japan Customs rulings; nothing scored"}
    out["earlier"] = {k: figure(v["correct"], v["cases"]) | {"source": v["source"]} for k, v in EARLIER.items()}
    reports = sorted(glob.glob(os.path.join(repo, "eval/refresh/*/cbp_report.json")))
    out["refresh_reports"] = [os.path.relpath(p, repo) for p in reports if os.path.basename(os.path.dirname(p)).startswith(month)]
    return out


def main():
    if "--month" not in sys.argv:
        sys.exit(__doc__)
    repo, month = sys.argv[1], sys.argv[sys.argv.index("--month") + 1]
    out = build(repo, month)
    path = os.path.join(repo, "eval/rescore", month, "result.json")
    text = json.dumps(out, indent=1, ensure_ascii=False) + "\n"
    if "--check" in sys.argv:
        if open(path, encoding="utf-8").read() != text:
            raise SystemExit(f"{path} differs from a recomputation")
        print("rescore result ok", month)
        return
    with open(path, "w", encoding="utf-8") as f:
        f.write(text)
    print(json.dumps({k: out[k] for k in ("month", "cbp", "japan")}, indent=1))


if __name__ == "__main__":
    main()
