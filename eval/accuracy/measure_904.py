"""D904: recency re-rank of BM25 precedents (recency-v1). Written before the runs. No model calls.
Paired on all 469 development cases: variant (-retrieval recency) against a same-day control; split into the cases
whose precedents changed (904-retrdiff.json: new ruling, or order only) and the rest.

    python3 -I -B eval/accuracy/measure_904.py .          # write eval/accuracy/904-measure.json
    python3 -I -B eval/accuracy/measure_904.py . --check
"""
import json
import math
import os
import sys

R = "eval/results/"
CONTROL = R + "dev-gemini-4-argon-prec-engine-v1-litellm-904-control.jsonl"
VARIANT = R + "dev-gemini-4-argon-recency30-engine-v1-litellm-904-recency.jsonl"
OUT = "eval/accuracy/904-measure.json"


def mcnemar(b, c):
    n = b + c
    if n == 0:
        return 1.0
    return round(min(1.0, 2 * sum(math.comb(n, i) for i in range(min(b, c) + 1)) / 2 ** n), 4)


def wilson(k, n, z=1.959963984540054):
    if n == 0:
        return [None, None, None]
    p = k / n
    d = 1 + z * z / n
    c = (p + z * z / (2 * n)) / d
    h = z * math.sqrt(p * (1 - p) / n + z * z / (4 * n * n)) / d
    return [round(p, 4), round(c - h, 4), round(c + h, 4)]


def load(p):
    with open(p) as f:
        return {r["ruling"]: r for r in (json.loads(l) for l in f if l.strip())}


def pair(ids, a, b):
    g = sorted(k for k in ids if not a[k]["correct"] and b[k]["correct"])
    l = sorted(k for k in ids if a[k]["correct"] and not b[k]["correct"])
    k0, k1 = sum(bool(a[k]["correct"]) for k in ids), sum(bool(b[k]["correct"]) for k in ids)
    return {"n": len(ids), "control": k0, "variant": k1, "control_wilson": wilson(k0, len(ids)),
            "variant_wilson": wilson(k1, len(ids)), "gained": g, "lost": l, "net": len(g) - len(l),
            "mcnemar_p": mcnemar(len(g), len(l))}


def build(root):
    c, v = load(os.path.join(root, CONTROL)), load(os.path.join(root, VARIANT))
    with open(os.path.join(root, "eval/accuracy/904-retrdiff.json")) as f:
        rows = json.load(f)["rows"]
    new = {r["ruling"] for r in rows if set(r["b"]) - set(r["a"])}
    order = {r["ruling"] for r in rows} - new
    ids = sorted(set(c) & set(v))
    return {"control": CONTROL, "variant": VARIANT, "all": pair(ids, c, v),
            "new_precedent": pair([i for i in ids if i in new], c, v),
            "order_only": pair([i for i in ids if i in order], c, v),
            "unchanged": pair([i for i in ids if i not in new and i not in order], c, v),
            "failed": {"control": sorted(k for k in ids if not c[k].get("pred")), "variant": sorted(k for k in ids if not v[k].get("pred"))}}


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
