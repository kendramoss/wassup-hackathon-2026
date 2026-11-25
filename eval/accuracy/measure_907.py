"""D907: a 2019-2022 pool of older New York rulings as retrieval precedents (#839). Written before the runs.
No model calls. Pairs the variant (production pool + labels_older.jsonl) against a same-day control (pool as
in production) on all 469 development cases, and splits them by what eval retrdiff says changes:
cases that retrieve an older ruling, cases whose precedents only reorder, and unchanged cases.

    python3 -I -B eval/accuracy/measure_907.py .          # write
    python3 -I -B eval/accuracy/measure_907.py . --check  # verify
"""
import json
import math
import os
import sys

R = "eval/results/"
CONTROL = R + "dev-gemini-4-argon-prec-engine-v1-litellm-907-control.jsonl"
VARIANT = R + "dev-gemini-4-argon-prec-engine-v1-litellm-907-older.jsonl"
DIFF = "eval/accuracy/907-retrdiff.json"
OUT = "eval/accuracy/907-measure.json"


def mcnemar(b, c):
    n = b + c
    if n == 0:
        return 1.0
    return round(min(1.0, 2 * sum(math.comb(n, i) for i in range(min(b, c) + 1)) / 2 ** n), 4)


def wilson(k, n, z=1.96):
    if n == 0:
        return [0.0, 0.0]
    p = k / n
    d = 1 + z * z / n
    c = (p + z * z / (2 * n)) / d
    h = z * math.sqrt(p * (1 - p) / n + z * z / (4 * n * n)) / d
    return [round(c - h, 4), round(c + h, 4)]


def load(p):
    with open(p) as f:
        return {r["ruling"]: r for r in (json.loads(l) for l in f if l.strip())}


def pair(ids, a, b):
    g = sorted(k for k in ids if not a[k]["correct"] and b[k]["correct"])
    l = sorted(k for k in ids if a[k]["correct"] and not b[k]["correct"])
    before = sum(bool(a[k]["correct"]) for k in ids)
    after = sum(bool(b[k]["correct"]) for k in ids)
    return {"n": len(ids), "before": before, "after": after,
            "before_wilson": wilson(before, len(ids)), "after_wilson": wilson(after, len(ids)),
            "gained": g, "lost": l, "net": len(g) - len(l), "mcnemar_p": mcnemar(len(g), len(l))}


def build(root):
    with open(os.path.join(root, DIFF)) as f:
        diff = json.load(f)
    c, v = (load(os.path.join(root, p)) for p in (CONTROL, VARIANT))
    ids = sorted(c)
    changed = {r["ruling"]: r for r in diff["rows"]}
    added = [i for i in ids if i in changed and changed[i]["added_from_b"]]
    reorder = [i for i in ids if i in changed and not changed[i]["added_from_b"]]
    same = [i for i in ids if i not in changed]
    return {"control": CONTROL, "variant": VARIANT,
            "all": pair(ids, c, v), "with_an_older_ruling": pair(added, c, v),
            "reordered_only": pair(reorder, c, v), "unchanged": pair(same, c, v),
            "failed": {"control": sorted(k for k in ids if not c[k].get("pred")),
                       "variant": sorted(k for k in ids if not v[k].get("pred"))}}


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
