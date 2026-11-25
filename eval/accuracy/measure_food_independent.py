"""#821 / #823: paired report for an independent food-v1 test set.

No model calls. Reads two committed result JSONLs and a frozen split manifest,
then writes a deterministic JSON report. The split must have been frozen before
the two runs; this script only pairs the rows it is given.

Usage (from the repository root):

    python3 -I -B eval/accuracy/measure_food_independent.py . \
      --control eval/results/<control>.jsonl \
      --variant eval/results/<variant>.jsonl \
      --split eval/splits/<frozen>.json \
      --out eval/accuracy/<name>-measure.json

    python3 -I -B eval/accuracy/measure_food_independent.py . --self-test
    python3 -I -B eval/accuracy/measure_food_independent.py . \
      --control ... --variant ... --split ... --out ... --check
"""
import argparse
import json
import math
import os
import sys
import tempfile


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
    k = min(b, c)
    p = sum(math.comb(n, i) for i in range(k + 1)) / 2 ** n
    return round(min(1.0, 2 * p), 4)


def load(path):
    rows = {}
    with open(path, encoding="utf-8") as f:
        for line in f:
            if line.strip():
                r = json.loads(line)
                rows[r["ruling"]] = r
    return rows


def split_entries(path):
    with open(path, encoding="utf-8") as f:
        d = json.load(f)
    if isinstance(d, list):
        raw = d
    elif isinstance(d, dict):
        raw = d.get("rulings", d.get("ids"))
    else:
        raise ValueError("split manifest must be a list or an object with rulings/ids")
    if not isinstance(raw, list):
        raise ValueError("split manifest has no rulings/ids list")
    out = []
    for e in raw:
        if isinstance(e, str):
            out.append({"ruling": e})
        elif isinstance(e, dict):
            rid = e.get("ruling", e.get("id"))
            if not rid:
                raise ValueError("split entry has no ruling/id")
            out.append(e)
        else:
            raise ValueError("split entry must be a string or object")
    return out


def pair(keys, before, after):
    k0 = sum(bool(before[k]["correct"]) for k in keys)
    k1 = sum(bool(after[k]["correct"]) for k in keys)
    gained = sorted(k for k in keys if after[k]["correct"] and not before[k]["correct"])
    lost = sorted(k for k in keys if before[k]["correct"] and not after[k]["correct"])
    n = len(keys)
    return {"n": n, "before": [k0] + wilson(k0, n)[1:], "after": [k1] + wilson(k1, n)[1:],
            "gained": len(gained), "lost": len(lost), "net": len(gained) - len(lost),
            "mcnemar_p": mcnemar(len(gained), len(lost)),
            "gained_ids": gained, "lost_ids": lost}


def report(control, variant, entries):
    ids = sorted({e["ruling"] if "ruling" in e else e["id"] for e in entries})
    keys = sorted(set(ids) & set(control) & set(variant))
    gated = sorted(k for k in keys if str(variant[k].get("prompt_version", "")).endswith("+food-v1"))
    return {
        "note": "Paired, independent food test. Both arms used the same frozen split, facts, retrieval, model and fresh gateway cache. Wilson 95% intervals; exact two-sided McNemar. Directional only when n is small.",
        "split_n": len(ids),
        "rows": {"control": len(control), "variant": len(variant)},
        "paired_n": len(keys),
        "missing_in_control": sorted(set(ids) - set(control)),
        "missing_in_variant": sorted(set(ids) - set(variant)),
        "gated_ids": gated,
        "control_vs_variant": pair(keys, control, variant),
    }


def write(parser_args):
    os.chdir(parser_args.root)
    ctl, var = load(parser_args.control), load(parser_args.variant)
    out = report(ctl, var, split_entries(parser_args.split))
    text = json.dumps(out, indent=1, ensure_ascii=False) + "\n"
    if parser_args.check:
        with open(parser_args.out, encoding="utf-8") as f:
            if f.read() != text:
                sys.exit(f"{parser_args.out} is out of date")
        print("ok")
        return
    with open(parser_args.out, "w", encoding="utf-8") as f:
        f.write(text)
    v = out["control_vs_variant"]
    print(f"n={v['n']} {v['before'][0]} -> {v['after'][0]} "
          f"+{v['gained']} -{v['lost']} net {v['net']:+d} p={v['mcnemar_p']}")


def self_test():
    with tempfile.TemporaryDirectory() as d:
        ctl = os.path.join(d, "control.jsonl")
        var = os.path.join(d, "variant.jsonl")
        split = os.path.join(d, "split.json")
        with open(ctl, "w", encoding="utf-8") as f:
            f.write(json.dumps({"ruling": "N1", "correct": False}) + "\n")
            f.write(json.dumps({"ruling": "N2", "correct": True}) + "\n")
        with open(var, "w", encoding="utf-8") as f:
            f.write(json.dumps({"ruling": "N1", "correct": True, "prompt_version": "engine-v1+food-v1"}) + "\n")
            f.write(json.dumps({"ruling": "N2", "correct": True, "prompt_version": "engine-v1+food-v1"}) + "\n")
        with open(split, "w", encoding="utf-8") as f:
            json.dump({"rulings": ["N1", "N2", "N3"]}, f)
        r = report(load(ctl), load(var), split_entries(split))
    assert r["paired_n"] == 2, r
    assert r["missing_in_variant"] == ["N3"], r
    assert r["gated_ids"] == ["N1", "N2"], r
    v = r["control_vs_variant"]
    assert (v["before"][0], v["after"][0], v["gained"], v["lost"], v["mcnemar_p"]) == (1, 2, 1, 0, 1.0), v
    print("ok")


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("root", nargs="?", default=".")
    p.add_argument("--control")
    p.add_argument("--variant")
    p.add_argument("--split")
    p.add_argument("--out")
    p.add_argument("--check", action="store_true")
    p.add_argument("--self-test", action="store_true")
    a = p.parse_args()
    if a.self_test:
        self_test()
        return
    for name in ("control", "variant", "split", "out"):
        if not getattr(a, name):
            p.error(f"--{name} is required unless --self-test is used")
    write(a)


if __name__ == "__main__":
    main()
