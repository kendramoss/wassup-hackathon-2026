"""D868: the older rulings of the cited-only pool as retrieval precedents. Written before the runs. No model calls.
Pairs the variant (pool + labels_cited.jsonl) against a same-day control (pool as in production) on the 16
development cases whose precedents change (splits_retrdiff.json); every other development case would get a
byte-identical prompt. Also against D860's development control on the same 16, as a second before.

    python3 -I -B eval/accuracy/measure_868.py .          # write
    python3 -I -B eval/accuracy/measure_868.py . --check  # verify
"""
import json
import math
import os
import sys

R = "eval/results/"
CONTROL = R + "subset-gemini-4-argon-prec-engine-v1-litellm-868-control.jsonl"
VARIANT = R + "subset-gemini-4-argon-prec-engine-v1-litellm-868-cited.jsonl"
D860 = R + "dev-gemini-4-argon-prec-engine-v1-litellm-860-control.jsonl"
OUT = "eval/accuracy/868-measure.json"


def mcnemar(b, c):
    n = b + c
    if n == 0:
        return 1.0
    return round(min(1.0, 2 * sum(math.comb(n, i) for i in range(min(b, c) + 1)) / 2 ** n), 4)


def load(p):
    with open(p) as f:
        return {r["ruling"]: r for r in (json.loads(l) for l in f if l.strip())}


def pair(ids, a, b):
    g = sorted(k for k in ids if not a[k]["correct"] and b[k]["correct"])
    l = sorted(k for k in ids if a[k]["correct"] and not b[k]["correct"])
    return {"n": len(ids), "before": sum(bool(a[k]["correct"]) for k in ids), "after": sum(bool(b[k]["correct"]) for k in ids),
            "gained": g, "lost": l, "net": len(g) - len(l), "mcnemar_p": mcnemar(len(g), len(l))}


def build(root):
    with open(os.path.join(root, "eval/accuracy/868-retrdiff.json")) as f:
        diff = json.load(f)
    ids = [r["ruling"] for r in diff["rows"]]
    added = [r["ruling"] for r in diff["rows"] if r["added_from_b"]]
    c, v, d = (load(os.path.join(root, p)) for p in (CONTROL, VARIANT, D860))
    return {"control": CONTROL, "variant": VARIANT,
            "all": pair(ids, c, v), "with_an_older_ruling": pair(added, c, v),
            "reordered_only": pair([i for i in ids if i not in added], c, v),
            "vs_d860_control": pair(ids, d, v),
            "control_vs_d860_control": pair(ids, d, c),
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
