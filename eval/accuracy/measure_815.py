"""D856/D857 (#815): paired development measurement of food-v1. Written and pushed before the variant run.
No model calls. Reads committed result files, writes eval/accuracy/815-measure.json.

  control  : dev-...-litellm-815-control         (same code, -food-rules off, gateway)
  variant  : dev-...-litellm-food-v1-815-food-v1 (-food-rules on, gateway)
  official : dev-...-vertex                      (the official development run, a second "before")
Slices (D856): food, none, other by the family check f26-b2-g3 (matched template at confidence >= 0.6, else none);
"gated" = rulings whose variant prompt version ends in +food-v1 (the rulings whose prompt changed).
Wilson 95% intervals; exact two-sided McNemar on gained and lost.

Run from the repository root:
    python3 -I -B eval/accuracy/measure_815.py .          # write
    python3 -I -B eval/accuracy/measure_815.py . --check  # verify
"""
import json
import math
import os
import sys

R = "eval/results/"
CONTROL = R + "dev-gemini-4-argon-prec-engine-v1-litellm-815-control.jsonl"
VARIANT = R + "dev-gemini-4-argon-prec-engine-v1-litellm-food-v1-815-food-v1.jsonl"
OFFICIAL = R + "dev-gemini-4-argon-prec-engine-v1-vertex.jsonl"
FAMILY = R + "families-coverage-dev-all-litellm-f26-b2-g3.jsonl"
OUT = "eval/accuracy/815-measure.json"
BAR = 0.6


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


def load(p):
    with open(p) as f:
        d = {}
        for l in f:
            if l.strip():
                r = json.loads(l)
                d[r["ruling"]] = r
        return d


def pair(keys, before, after):
    k0 = sum(bool(before[k]["correct"]) for k in keys)
    k1 = sum(bool(after[k]["correct"]) for k in keys)
    gained = sorted(k for k in keys if after[k]["correct"] and not before[k]["correct"])
    lost = sorted(k for k in keys if before[k]["correct"] and not after[k]["correct"])
    n = len(keys)
    return {"n": n, "before": [k0] + wilson(k0, n)[1:], "after": [k1] + wilson(k1, n)[1:],
            "gained": len(gained), "lost": len(lost), "net": len(gained) - len(lost),
            "mcnemar_p": mcnemar(len(gained), len(lost)), "gained_ids": gained, "lost_ids": lost}


def main():
    os.chdir(sys.argv[1])
    ctl, var, off, fam = load(CONTROL), load(VARIANT), load(OFFICIAL), load(FAMILY)
    keys_all = sorted(set(ctl) & set(var) & set(off))

    def sl(k):
        r = fam[k]
        ok = r.get("covered") and not r.get("error") and (r.get("confidence") or 0) >= BAR
        f = r["family"] if ok else "none"
        return "food" if f == "food_preparations" else ("none" if f == "none" else "other")

    groups = {"food": [k for k in keys_all if sl(k) == "food"], "none": [k for k in keys_all if sl(k) == "none"],
              "other": [k for k in keys_all if sl(k) == "other"], "all": keys_all,
              "gated": [k for k in keys_all if str(var[k].get("prompt_version", "")).endswith("+food-v1")]}
    out = {"note": "Development only (469). Held-out was spent in D842 and is not read or re-scored.",
           "rows": {"control": len(ctl), "variant": len(var), "official": len(off)},
           "gated_outside_food_slice": sorted(k for k in groups["gated"] if sl(k) != "food"),
           "variant_vs_control": {g: pair(ks, ctl, var) for g, ks in groups.items()},
           "variant_vs_official": {g: pair(ks, off, var) for g, ks in groups.items()},
           "control_vs_official": {g: pair(ks, off, ctl) for g, ks in groups.items()}}
    text = json.dumps(out, indent=1) + "\n"
    if "--check" in sys.argv:
        with open(OUT) as f:
            if f.read() != text:
                sys.exit("815-measure.json is out of date")
        print("ok")
        return
    with open(OUT, "w") as f:
        f.write(text)
    for name in ("variant_vs_control", "variant_vs_official", "control_vs_official"):
        print(name)
        for g, v in out[name].items():
            print(f"  {g:6} n={v['n']:3} {v['before'][0]:3} -> {v['after'][0]:3}  +{v['gained']} -{v['lost']}  net {v['net']:+d}  p={v['mcnemar_p']}")


if __name__ == "__main__":
    main()
