"""D858 (#821): paired measurement of food-v1 on the food-fresh split. Written before either run.
No model calls. Reads committed result files, writes eval/accuracy/858-measure.json.

  control : foodfresh-...-litellm-858-control          (-food-rules off, gateway)
  variant : foodfresh-...-litellm-food-v1-858-food-v1  (-food-rules on, gateway)
Groups: all; training (in splits_tune.json); new (labels_foodfresh.jsonl).
Wilson 95% intervals; exact two-sided McNemar on gained and lost.

Run from the repository root:
    python3 -I -B eval/accuracy/measure_858.py .          # write
    python3 -I -B eval/accuracy/measure_858.py . --check  # verify
"""
import json
import math
import os
import sys

R = "eval/results/"
CONTROL = R + "foodfresh-gemini-4-argon-prec-engine-v1-litellm-858-control.jsonl"
VARIANT = R + "foodfresh-gemini-4-argon-prec-engine-v1-litellm-food-v1-858-food-v1.jsonl"
OUT = "eval/accuracy/858-measure.json"


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


def pair(keys, before, after):
    keys = sorted(keys)
    gained = [k for k in keys if not before[k]["correct"] and after[k]["correct"]]
    lost = [k for k in keys if before[k]["correct"] and not after[k]["correct"]]
    k0 = sum(bool(before[k]["correct"]) for k in keys)
    k1 = sum(bool(after[k]["correct"]) for k in keys)
    return {"n": len(keys), "control": k0, "variant": k1,
            "control_wilson": wilson(k0, len(keys)), "variant_wilson": wilson(k1, len(keys)),
            "gained": gained, "lost": lost, "net": len(gained) - len(lost),
            "mcnemar_p": mcnemar(len(gained), len(lost))}


def build(root):
    sp = json.load(open(os.path.join(root, "eval/splits/splits_foodfresh.json")))
    tune = json.load(open(os.path.join(root, "eval/splits/splits_tune.json")))
    training = set(tune["train"]) | set(tune["tune_check"])
    c, v = load(os.path.join(root, CONTROL)), load(os.path.join(root, VARIANT))
    ids = sp["foodfresh"]
    missing = [k for k in ids if k not in c or k not in v]
    if missing:
        raise SystemExit(f"rows missing: {missing}")
    gated = [k for k in ids if v[k].get("prompt_version", "").endswith("+food-v1")]
    return {
        "control": CONTROL, "variant": VARIANT,
        "all": pair(ids, c, v),
        "training": pair([k for k in ids if k in training], c, v),
        "new": pair([k for k in ids if k not in training], c, v),
        "gated": len(gated),
        "errors": {"control": sorted(k for k in ids if not c[k].get("pred")),
                   "variant": sorted(k for k in ids if not v[k].get("pred"))},
    }


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
