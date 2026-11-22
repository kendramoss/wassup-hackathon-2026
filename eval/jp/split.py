"""Frozen dev / held-out split for the Japan Customs origin rulings (D710).

Usage: python3 -I -B eval/jp/split.py <repo> [--check]

Uses only each ruling's id, issue date and agreement. Never reads a label.
"""
import json, os, random, sys

CUTOFF = "2026-01-01"
DEV_EVAL_N = 400
DEV_EVAL_YEAR = "2025"
SEED = 20261122


def build(rulings):
    held = sorted(r["id"] for r in rulings if r["date"] >= CUTOFF)
    dev = sorted(r["id"] for r in rulings if r["date"] < CUTOFF)
    pool = {}
    for r in rulings:
        if r["date"] < CUTOFF and r["date"].startswith(DEV_EVAL_YEAR):
            pool.setdefault(r["agreement"], []).append(r["id"])
    total = sum(len(v) for v in pool.values())
    # Largest-remainder proportional allocation, at least 1 per agreement.
    quota = {a: max(1, DEV_EVAL_N * len(v) // total) for a, v in pool.items()}
    rem = sorted(pool, key=lambda a: (-(DEV_EVAL_N * len(pool[a]) % total), a))
    i = 0
    while sum(quota.values()) < DEV_EVAL_N:
        a = rem[i % len(rem)]
        if quota[a] < len(pool[a]):
            quota[a] += 1
        i += 1
    while sum(quota.values()) > DEV_EVAL_N:  # only if the minimum of 1 overshot
        a = max(quota, key=lambda k: (quota[k], k))
        quota[a] -= 1
    rng = random.Random(SEED)
    dev_eval = []
    for a in sorted(pool):
        dev_eval += rng.sample(sorted(pool[a]), quota[a])
    return {
        "rule": "D710: held_out = issued on or after %s; dev = before; dev_eval = %d of the %s dev rulings, "
                "stratified by agreement (largest remainder, min 1), random.Random(%d) per agreement in sorted order"
                % (CUTOFF, DEV_EVAL_N, DEV_EVAL_YEAR, SEED),
        "frozen": "2026-11-22",
        "counts": {"dev": len(dev), "dev_eval": len(dev_eval), "held_out": len(held)},
        "dev_eval_by_agreement": {a: quota[a] for a in sorted(quota)},
        "dev": dev, "dev_eval": sorted(dev_eval), "held_out": held,
    }


def main():
    repo = sys.argv[1]
    rulings = [json.loads(l) for l in open(os.path.join(repo, "data/jp/rulings.jsonl"), encoding="utf-8")]
    out = json.dumps(build(rulings), indent=1, ensure_ascii=False) + "\n"
    p = os.path.join(repo, "data/jp/splits.json")
    if "--check" in sys.argv:
        if open(p, encoding="utf-8").read() != out:
            sys.exit("data/jp/splits.json differs from the D710 rule")
        print("jp splits ok")
        return
    open(p, "w", encoding="utf-8").write(out)
    print(json.dumps(json.loads(out)["counts"]))


if __name__ == "__main__":
    main()
