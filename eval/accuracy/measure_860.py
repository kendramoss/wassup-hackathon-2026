"""D860 (#831): paired measurement of china-v1. Written before any run. No model calls.
Reads committed result files, writes eval/accuracy/860-measure.json.

  dev   : dev-...-litellm-860-control  vs  dev-...-litellm-china-v1-860-china-v1   (469)
  check : train-...-litellm-860-control vs train-...-litellm-china-v1-860-china-v1 (splits_chinacheck.json, unseen)
"gated" = rulings whose variant prompt version ends in +china-v1. Also: China answers and how many are right.
Wilson 95% intervals; exact two-sided McNemar on gained and lost.

    python3 -I -B eval/accuracy/measure_860.py .          # write
    python3 -I -B eval/accuracy/measure_860.py . --check  # verify
"""
import json
import math
import os
import sys

R = "eval/results/"
RUNS = {
    "dev": (R + "dev-gemini-4-argon-prec-engine-v1-litellm-860-control.jsonl",
            R + "dev-gemini-4-argon-prec-engine-v1-litellm-china-v1-860-china-v1.jsonl"),
    "check": (R + "train-gemini-4-argon-prec-engine-v1-litellm-860-control.jsonl",
              R + "train-gemini-4-argon-prec-engine-v1-litellm-china-v1-860-china-v1.jsonl"),
}
OUT = "eval/accuracy/860-measure.json"


def wilson(k, n, z=1.959963984540054):
    if n == 0:
        return [None, None, None]
    p = k / n
    d = 1 + z * z / n
    c = (p + z * z / (2 * n)) / d
    h = z * math.sqrt(p * (1 - p) / n + z * z / (4 * n * n)) / d
    return [round(p, 4), round(max(0, c - h), 4), round(min(1, c + h), 4)]


def mcnemar(b, c):
    n = b + c
    if n == 0:
        return 1.0
    p = sum(math.comb(n, i) for i in range(min(b, c) + 1)) / 2 ** n
    return round(min(1.0, 2 * p), 4)


def load(p):
    d = {}
    with open(p) as f:
        for l in f:
            if l.strip():
                r = json.loads(l)
                d[r["ruling"]] = r
    return d


def pair(keys, a, b):
    keys = sorted(keys)
    g = [k for k in keys if not a[k]["correct"] and b[k]["correct"]]
    l = [k for k in keys if a[k]["correct"] and not b[k]["correct"]]
    k0 = sum(bool(a[k]["correct"]) for k in keys)
    k1 = sum(bool(b[k]["correct"]) for k in keys)
    return {"n": len(keys), "control": k0, "variant": k1, "control_wilson": wilson(k0, len(keys)),
            "variant_wilson": wilson(k1, len(keys)), "gained": len(g), "lost": len(l), "net": len(g) - len(l),
            "mcnemar_p": mcnemar(len(g), len(l)), "gained_ids": g, "lost_ids": l}


def china(rows):
    cn = [r for r in rows.values() if r.get("pred") == "China"]
    return {"answers": len(cn), "right": sum(bool(r["correct"]) for r in cn)}


def build(root):
    out = {}
    for name, (cp, vp) in RUNS.items():
        c, v = load(os.path.join(root, cp)), load(os.path.join(root, vp))
        ids = sorted(set(c) & set(v))
        gated = [k for k in ids if v[k].get("prompt_version", "").endswith("+china-v1")]
        rest = [k for k in ids if k not in set(gated)]
        out[name] = {"control": cp, "variant": vp, "rows": [len(c), len(v)], "all": pair(ids, c, v),
                     "gated": pair(gated, c, v), "ungated": pair(rest, c, v),
                     "china_answers": {"control": china(c), "variant": china(v)},
                     "failed": {"control": sorted(k for k in ids if not c[k].get("pred")),
                                "variant": sorted(k for k in ids if not v[k].get("pred"))}}
    return out


def main():
    root = sys.argv[1]
    got = json.dumps(build(root), indent=2, sort_keys=True) + "\n"
    p = os.path.join(root, OUT)
    if "--check" in sys.argv:
        if open(p).read() != got:
            raise SystemExit(f"{OUT} is stale")
        return
    open(p, "w").write(got)
    print(got)


if __name__ == "__main__":
    main()
