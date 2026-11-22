"""Which Part 102 product types lack a plan template, and which get one (D580).

Reads only the Part 102 selection pool (eval/families/part102-pool.json, the D560
pool: labelled Part 102 rulings dated before 2024 in no development or held-out
split): the family check with the 14 templates on it
(eval/results/families-coverage-train-all-litellm-p102pool-f14.jsonl) and one
product category per ruling (eval/results/families102-pool-labels.jsonl,
eval/families/label102.py). No model calls.

Rule (written after the pool counts were seen; pool data only): count the pool
rulings the family check gives no template (answer "none", confidence below 0.6,
or a failed check) by category. Leave out categories with a template already
(their misses are check misses, not a missing family), chemicals_pharma and
apparel_textiles (regimes with their own engines, as D138) and "other". Keep
categories with at least 7 such rulings, by count, at most 6; ties go to the
category with more assembled goods.

Run from the repository root:
    python3 -I -B eval/families/select102.py .          # write
    python3 -I -B eval/families/select102.py . --check  # verify
"""
import collections
import json
import os
import sys

OUT = "eval/results/families102-selection.json"
COV = "eval/results/families-coverage-train-all-litellm-p102pool-f14.jsonl"
LABELS = "eval/results/families102-pool-labels.jsonl"
POOL = "eval/families/part102-pool.json"
HAVE_TEMPLATE = {"vehicle_parts", "furniture", "pcba_electronic_devices", "electric_motors_pumps_compressors",
                 "cables_harnesses_connectors", "hand_tools_cutlery", "medical_devices", "optical_cameras",
                 "power_supplies_chargers", "filters", "led_lights", "motorised_tools_small_appliances"}
OWN_ENGINE = {"chemicals_pharma", "apparel_textiles"}
MIN, CAP = 7, 6
# The template each picked category becomes.
FAMILY = {"food_agriculture": "food_preparations", "metal_hardware_fittings": "metal_products",
          "plastic_articles": "plastic_articles", "wood_articles": "wood_panels_flooring",
          "machinery_parts": "machinery_parts", "electrical_apparatus": "electrical_control_apparatus"}


def jsonl(path):
    with open(path) as f:
        return [json.loads(line) for line in f if line.strip()]


def build(root):
    pool = set(json.load(open(os.path.join(root, POOL)))["train"])
    cov = {r["ruling"]: r for r in jsonl(os.path.join(root, COV))}
    lab = {r["ruling"]: r for r in jsonl(os.path.join(root, LABELS))}
    if set(cov) != pool or set(lab) != pool:
        sys.exit("coverage or labels do not match the pool")
    unc = [r for r in sorted(pool) if not cov[r]["covered"]]
    n = collections.Counter(lab[r]["category"] for r in unc)
    asm = collections.Counter(lab[r]["category"] for r in unc if lab[r]["assembled"])
    rows = [{"category": c, "uncovered": n[c], "assembled": asm[c],
             "left_out": ("has a template" if c in HAVE_TEMPLATE else "own engine" if c in OWN_ENGINE else "other" if c == "other" else None)}
            for c in sorted(n, key=lambda c: (-n[c], -asm[c], c))]
    eligible = [r for r in rows if r["left_out"] is None and r["uncovered"] >= MIN]
    picked = eligible[:CAP]
    for r in picked:
        r["family"] = FAMILY[r["category"]]
    return {
        "rule": __doc__.split("Rule ")[1].split("\n\nRun")[0].replace("\n", " "),
        "pool": len(pool), "covered_f14": sum(cov[r]["covered"] for r in pool),
        "failed_checks": sum(1 for r in pool if cov[r].get("error")), "uncovered": len(unc),
        "categories": rows,
        "not_picked_at_or_over_min": [r["category"] for r in eligible[CAP:]],
        "picked": [{"category": r["category"], "family": r["family"], "uncovered": r["uncovered"],
                    "members": sorted(x for x in pool if lab[x]["category"] == r["category"])} for r in picked],
    }


if __name__ == "__main__":
    root = sys.argv[1]
    want = json.dumps(build(root), indent=1) + "\n"
    path = os.path.join(root, OUT)
    if "--check" in sys.argv:
        with open(path) as f:
            if f.read() != want:
                sys.exit(OUT + " is stale: run eval/families/select102.py")
        print(OUT, "ok")
    else:
        with open(path, "w") as f:
            f.write(want)
        print("wrote", OUT)
