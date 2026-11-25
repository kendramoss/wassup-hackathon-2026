"""D902: precedent support inside the confidence score (support-aware calibration). Written before any fit.
No model calls.

Score = logistic regression on [stated confidence, agree/5, min(age, 36)/36] with an intercept, fitted on the 623
training rows of D860's check control (target: correct), L2 1e-3, Newton steps, deterministic. agree and age as
D900. Compared on development at matched coverage: in each run, take as many in-scope answers as today's rule
auto-accepts (in scope, stated >= 0.92), the highest-scoring first (ties: stated confidence, then ruling id),
and count the wrong ones.

    python3 -I -B eval/accuracy/measure_902.py .          # write eval/accuracy/902-measure.json
    python3 -I -B eval/accuracy/measure_902.py . --check
"""
import json
import math
import os
import statistics
import sys

R = "eval/results/"
FIT = R + "train-gemini-4-argon-prec-engine-v1-litellm-860-control.jsonl"
PRIMARY = R + "dev-gemini-4-argon-prec-engine-v1-vertex.jsonl"
SECONDARY = [R + "dev-gemini-4-argon-prec-engine-v1-litellm-815-control.jsonl",
             R + "dev-gemini-4-argon-prec-engine-v1-litellm-860-control.jsonl"]
OUT = "eval/accuracy/902-measure.json"
BAR, L2 = 0.92, 1e-3


def months(d):
    return int(d[:4]) * 12 + int(d[5:7])


def feats(r, lab):
    ps = [p for p in r.get("precedents") or [] if p in lab]
    agree = sum(1 for p in ps if lab[p].get("primary_origin") == r.get("pred"))
    age = statistics.mean(months(r["date"]) - months(lab[p]["date"]) for p in ps) if ps else 36.0
    return [1.0, r.get("confidence") or 0.0, agree / 5, min(age, 36) / 36]


def fit(X, y):
    w = [0.0] * len(X[0])
    for _ in range(50):
        g = [L2 * wi for wi in w]
        H = [[L2 if i == j else 0.0 for j in range(len(w))] for i in range(len(w))]
        for x, t in zip(X, y):
            p = 1 / (1 + math.exp(-sum(a * b for a, b in zip(w, x))))
            for i in range(len(w)):
                g[i] += (p - t) * x[i]
                for j in range(len(w)):
                    H[i][j] += p * (1 - p) * x[i] * x[j]
        w = [a - b for a, b in zip(w, solve(H, g))]
    return [round(v, 6) for v in w]


def solve(A, b):
    n = len(b)
    M = [row[:] + [b[i]] for i, row in enumerate(A)]
    for c in range(n):
        p = max(range(c, n), key=lambda r: abs(M[r][c]))
        M[c], M[p] = M[p], M[c]
        for r in range(n):
            if r != c:
                f = M[r][c] / M[c][c]
                M[r] = [a - f * b for a, b in zip(M[r], M[c])]
    return [M[i][n] / M[i][i] for i in range(n)]


def build(root):
    lab = {}
    with open(os.path.join(root, "eval/data/labels.jsonl")) as f:
        for l in f:
            x = json.loads(l)
            lab[x["ruling"]] = x
    load = lambda p: [json.loads(l) for l in open(os.path.join(root, p)) if l.strip()]
    tr = [r for r in load(FIT) if r.get("pred")]
    w = fit([feats(r, lab) for r in tr], [1.0 if r["correct"] else 0.0 for r in tr])
    score = lambda r: sum(a * b for a, b in zip(w, feats(r, lab)))
    out = {"weights": {"intercept": w[0], "confidence": w[1], "agree": w[2], "age": w[3]}, "test": {}}
    for p in [PRIMARY] + SECONDARY:
        rows = [r for r in load(p) if r.get("pred")]
        ins = [r for r in rows if r.get("in_scope")]
        today = [r for r in ins if (r.get("confidence") or 0) >= BAR]
        k = len(today)
        ranked = sorted(ins, key=lambda r: (-score(r), -(r.get("confidence") or 0), r["ruling"]))[:k]
        out["test"][p] = {"auto": k, "errors_today": sum(1 for r in today if not r["correct"]),
                          "errors_support_score": sum(1 for r in ranked if not r["correct"]),
                          "swapped_in": sorted(r["ruling"] for r in ranked if r not in today),
                          "swapped_out": sorted(r["ruling"] for r in today if r not in ranked)}
    pt = out["test"][PRIMARY]
    sec = [out["test"][p] for p in SECONDARY]
    out["passed"] = (pt["errors_today"] - pt["errors_support_score"] >= 3
                     and all(s["errors_support_score"] <= s["errors_today"] for s in sec)
                     and any(s["errors_support_score"] < s["errors_today"] for s in sec))
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
