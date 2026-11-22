"""The substantial-transformation selection pool for the third round of plan templates (D660).

Labelled substantial-transformation rulings with a facts file, dated before
2026 (the first held-out window starts in January 2026), in no held-out,
pilot or tune-check split of any regime (every `final`, `final2`, `pilot` and
`tune_check` list under eval/splits/), and not in the development split
(development rulings are counted from their own coverage file). Written under
the "train" key so `cmd/eval familycov -split train` reads it. The Part 102
pool is D580's (eval/families/part102-pool.json). No model calls.

Run from the repository root:
    python3 -I -B eval/families/pool_r3.py .          # write
    python3 -I -B eval/families/pool_r3.py . --check  # verify
"""
import glob
import json
import os
import sys

OUT = "eval/families/r3-st-pool.json"
CUTOFF = "2026"
HELD = ("final", "final2", "pilot", "tune_check")


def held_out(root):
    ids = set()
    for p in sorted(glob.glob(os.path.join(root, "eval/splits/*.json")) + glob.glob(os.path.join(root, "eval/splits/routed/*.json"))):
        with open(p) as f:
            d = json.load(f)
        for k in HELD:
            if isinstance(d.get(k), list):
                ids |= set(d[k])
    return ids


def build(root):
    with open(os.path.join(root, "eval/data/labels.jsonl")) as f:
        lab = [json.loads(l) for l in f if l.strip()]
    with open(os.path.join(root, "eval/splits/splits_final.json")) as f:
        dev = set(json.load(f)["dev"])
    facts = {n[:-4] for n in os.listdir(os.path.join(root, "backend/data/facts")) if n.endswith(".txt")}
    held = held_out(root)
    pool = sorted(l["ruling"] for l in lab if l["regime"] == "substantial_transformation" and l["ruling"] in facts
                  and l["date"] < CUTOFF and l["ruling"] not in held and l["ruling"] not in dev)
    if set(pool) & (held | dev):
        sys.exit("development or held-out rulings in the pool")
    return {
        "rule": "D660: labelled substantial-transformation rulings with facts, dated before 2026, in no final, final2, pilot "
        "or tune_check list of any split file and not in dev. Families are chosen from these and the development rulings, "
        "and written from these only.",
        "excluded_held_out_ids": len(held),
        "train": pool,
    }


if __name__ == "__main__":
    root = sys.argv[1]
    want = json.dumps(build(root), indent=1) + "\n"
    path = os.path.join(root, OUT)
    if "--check" in sys.argv:
        with open(path) as f:
            if f.read() != want:
                sys.exit(OUT + " is stale: run eval/families/pool_r3.py")
        print(OUT, "ok")
    else:
        with open(path, "w") as f:
            f.write(want)
        print("wrote", OUT)
