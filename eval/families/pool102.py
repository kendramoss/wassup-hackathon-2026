"""The Part 102 selection pool for the Part 102 plan templates (D580).

The D560 rule (#619): labelled Part 102 rulings dated before 2024 that are in
no development or held-out split (splits_usmca.json dev and final,
splits_usmca_final2.json final2). Families are chosen and written only from
this pool; development rulings measure coverage; held-out rulings are never
read here. Written under the "train" key so `cmd/eval familycov -split train`
reads it. No model calls.

Run from the repository root:
    python3 -I -B eval/families/pool102.py .          # write
    python3 -I -B eval/families/pool102.py . --check  # verify
"""
import json
import os
import sys

OUT = "eval/families/part102-pool.json"
CUTOFF = "2024"


def jsonl(path):
    with open(path) as f:
        return [json.loads(line) for line in f if line.strip()]


def build(root):
    with open(os.path.join(root, "eval/splits/splits_usmca.json")) as f:
        splits = json.load(f)
    with open(os.path.join(root, "eval/splits/splits_usmca_final2.json")) as f:
        final2 = json.load(f)["final2"]
    label = {r["ruling"]: r for r in jsonl(os.path.join(root, "eval/data/labels_usmca.jsonl"))}
    held = set(splits["dev"]) | set(splits["final"]) | set(final2)
    pool = sorted(r for r in splits["usmca"] if label[r]["date"] < CUTOFF and r not in held)
    if set(pool) & held:
        sys.exit("development or held-out rulings in the pool")
    return {
        "rule": "D580, the D560 pool (#619): labelled Part 102 rulings in splits_usmca.json 'usmca' dated before 2024, "
        "none in dev, final or final2. Families are chosen and written from these only.",
        "excluded": {"dev": len(splits["dev"]), "final": len(splits["final"]), "final2": len(final2),
                     "dated_2024_on_not_in_a_split": sum(1 for r in splits["usmca"] if label[r]["date"] >= CUTOFF and r not in held)},
        "train": pool,
    }


if __name__ == "__main__":
    root = sys.argv[1]
    want = json.dumps(build(root), indent=1) + "\n"
    path = os.path.join(root, OUT)
    if "--check" in sys.argv:
        with open(path) as f:
            if f.read() != want:
                sys.exit(OUT + " is stale: run eval/families/pool102.py")
        print(OUT, "ok")
    else:
        with open(path, "w") as f:
            f.write(want)
        print("wrote", OUT)
