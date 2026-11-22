"""Which product types lack a plan template across both regimes, and which get one (D660).

Reads only development and pool rulings: substantial-transformation development
(469, splits_final.json `dev`) and the D660 pool (eval/families/r3-st-pool.json),
Part 102 development (72, splits_usmca.json `dev`) and the D580 pool
(eval/families/part102-pool.json). The production family check with the 20
templates on each set (eval/results/families-coverage-*-f20.jsonl) says which
have no template; eval/results/families-r3-labels.jsonl (eval/families/label_r3.py)
gives each of those a product category, a product noun and the good's HTS code.
No model calls; held-out rulings are never read (the script stops if a set
holds one).

Rule, in two steps. Step 1 was written before the counts were seen: a
ruling in two sets is counted once, and has a template if any of its checks
found one (family not "none" at confidence 0.6 or more); count the rulings with
no template by category (eval/families/label102.py's list), leaving out
categories a template already covers, chemicals_pharma and apparel_textiles
(regimes with their own engines, as D138 and D580) and "other"; keep
categories with at least 10 rulings with no template, at least 6 of them in a
pool, at most 8. Step 1 kept 5 categories, two of them unrelated goods under one
label (household goods; toys and sporting goods), and missed product types
labelled under a category that has a template (integrated circuits and
transistors under PCBA devices). Step 2, written after step 1's counts and
nouns were seen and used for the pick: group by product type, a fixed pattern
over each ruling's product noun and subject (TYPES below; the first pattern
that matches; chemicals_pharma and apparel_textiles are never typed). Keep
types with at least 10 selection rulings with no template and at least 5 pool
rulings of the type, with or without a template, in the regime most of its pool
rulings were decided under (ties to substantial transformation): a template is
written from, and cites, one regime's pool only, and development rulings are
never cited. By count, at most 8, ties to the type with more pool rulings.

Run from the repository root:
    python3 -I -B eval/families/select_r3.py .          # write
    python3 -I -B eval/families/select_r3.py . --check  # verify
"""
import collections
import re
import json
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import pool_r3  # noqa: E402

OUT = "eval/results/families-r3-selection.json"
LABELS = "eval/results/families-r3-labels.jsonl"
SETS = {
    "st_dev": ("eval/splits/splits_final.json", "dev", "eval/results/families-coverage-dev-all-litellm-f20.jsonl"),
    "st_pool": ("eval/families/r3-st-pool.json", "train", "eval/results/families-coverage-train-all-litellm-r3stpool-f20.jsonl"),
    "p102_dev": ("eval/splits/splits_usmca.json", "dev", "eval/results/families-coverage-dev-all-litellm-p102-f20.jsonl"),
    "p102_pool": ("eval/families/part102-pool.json", "train", "eval/results/families-coverage-train-all-litellm-p102pool-f20.jsonl"),
}
POOLS = {"st_pool": "substantial_transformation", "p102_pool": "part102"}
# Categories (eval/families/label102.py) a template already covers: D138, D212 and D580's.
HAVE_TEMPLATE = {"vehicle_parts", "furniture", "pcba_electronic_devices", "electric_motors_pumps_compressors",
                 "cables_harnesses_connectors", "hand_tools_cutlery", "medical_devices", "optical_cameras",
                 "power_supplies_chargers", "filters", "led_lights", "motorised_tools_small_appliances",
                 "food_agriculture", "metal_hardware_fittings", "plastic_articles", "wood_articles",
                 "machinery_parts", "electrical_apparatus"}
OWN_ENGINE = {"chemicals_pharma", "apparel_textiles"}
MIN, MIN_POOL, CAP = 10, 6, 8
TYPE_MIN, TYPE_MIN_POOL = 10, 5
# Step 2 product types: pattern over "<product noun> | <subject>", first match wins.
TYPES = [
    ("display_modules", r"liquid crystal|\blcd\b|display (module|panel)"),
    ("semiconductors", r"integrated circuit|transistor|diode|mosfet|semiconductor|\bwafers?\b|front.end.module|microcontroller"),
    ("automotive_filters", r"(air|oil|fuel|cabin)[^.|]{0,25}filter"),
    ("paper_tissue", r"paper|tissue|napkin|towel|paperboard|folders?\b|notebook|envelope|greeting card|lottery ticket|labelstock|seat cover"),
    ("offroad_machines", r"excavator|grader|road roller|telehandler|scissor lift|guided vehicle|\bagvs?\b|utility (task|terrain) vehicle|\butvs?\b|\batvs?\b|all.terrain|off.road|dirt bike|lawn ?mower|forklift|wheel loader|tractor"),
    ("hvac_heating", r"heat pump|furnace|air.condition|\bhvac\b|gas heater|outdoor units?\b|boiler|water heater"),
    ("bicycles", r"bicycle|e-?bikes?\b|scooter"),
    ("golf_sporting", r"golf|helmet|fishing|\bskis?\b|skate|racket|climbing skin"),
    ("toys_kits", r"\btoys?\b|plush|doll|hack pack|build box|launcher kit"),
    ("lifting_jacks", r"\bjacks?\b"),
    ("writing_instruments", r"\bpens?\b|pencil|marker"),
    ("batteries", r"batter|lithium.ion|\bcells?\b"),
    ("solar", r"photovoltaic|solar"),
]
# Product nouns of pool rulings with a template, from the earlier labels (D138 and D580).
NOUNS = ["eval/results/families-selection-pool-labels.jsonl", "eval/results/families102-pool-labels.jsonl"]


def jsonl(path):
    with open(path) as f:
        return [json.loads(line) for line in f if line.strip()]


def build(root):
    held = pool_r3.held_out(root)
    member = collections.defaultdict(set)
    covered = collections.defaultdict(bool)
    counts = {}
    for name, (split_file, key, cov_file) in SETS.items():
        with open(os.path.join(root, split_file)) as f:
            ids = json.load(f)[key]
        if set(ids) & held:
            sys.exit(f"{name}: held-out rulings in a selection set")
        cov = {r["ruling"]: r for r in jsonl(os.path.join(root, cov_file))}
        if not set(cov) <= set(ids):
            sys.exit(f"{name}: coverage rows outside the set")
        # familycov reads only rulings with a single held origin; the rest are not in the count.
        ids = [r for r in ids if r in cov]
        for r in ids:
            member[r].add(name)
            covered[r] = covered[r] or bool(cov.get(r, {}).get("covered"))
        counts[name] = {"checked": len(ids), "with_template": sum(1 for r in ids if cov.get(r, {}).get("covered")),
                        "failed_checks": sum(1 for r in ids if cov.get(r, {}).get("error"))}
    lab = {r["ruling"]: r for r in jsonl(os.path.join(root, LABELS)) if "error" not in r}
    unc = sorted(r for r in member if not covered[r])
    missing = [r for r in unc if r not in lab]
    if missing:
        sys.exit(f"{len(missing)} rulings with no template have no label: {missing[:5]}")
    by = collections.defaultdict(list)
    for r in unc:
        by[lab[r]["category"]].append(r)

    def pool_n(rs):
        return sum(1 for r in rs if member[r] & set(POOLS))

    rows = []
    for c, rs in by.items():
        heads = collections.Counter((lab[r].get("hts") or "")[:4] or "none" for r in rs)
        nouns = collections.Counter(lab[r]["product"].lower() for r in rs)
        rows.append({
            "category": c, "no_template": len(rs), "in_pool": pool_n(rs),
            "by_set": {k: sum(1 for r in rs if k in member[r]) for k in SETS},
            "assembled": sum(1 for r in rs if lab[r]["assembled"]),
            "top_headings": heads.most_common(6), "top_nouns": nouns.most_common(8),
            "left_out": ("has a template" if c in HAVE_TEMPLATE else "own engine" if c in OWN_ENGINE else "other" if c == "other" else None),
        })
    rows.sort(key=lambda x: (-x["no_template"], -x["in_pool"], x["category"]))
    eligible = [x for x in rows if x["left_out"] is None and x["no_template"] >= MIN and x["in_pool"] >= MIN_POOL]
    # Step 2: product types.
    subj = {}
    for f in ("eval/data/labels.jsonl", "eval/data/labels_usmca.jsonl"):
        for x in jsonl(os.path.join(root, f)):
            subj[x["ruling"]] = x["subject"]
    noun = {}
    for f in NOUNS:
        for x in jsonl(os.path.join(root, f)):
            if "product" in x:
                noun[x["ruling"]] = x["product"]
    noun.update({r: x["product"] for r, x in lab.items()})
    pats = [(t, re.compile(p, re.I)) for t, p in TYPES]

    def typ(r):
        if r in lab and lab[r]["category"] in OWN_ENGINE:
            return None
        text = noun.get(r, "") + " | " + subj.get(r, "")
        return next((t for t, p in pats if p.search(text)), None)

    pool_ids = {r for r in member if member[r] & set(POOLS)}
    trows = []
    for t, _ in TYPES:
        u = [r for r in unc if typ(r) == t]
        pm = sorted(r for r in pool_ids if typ(r) == t)
        per = {reg: [r for r in pm if pool in member[r]] for pool, reg in POOLS.items()}
        regime = "part102" if len(per["part102"]) > len(per["substantial_transformation"]) else "substantial_transformation"
        trows.append({"regime": regime, "pool_in_regime": len(per[regime]), "type": t, "no_template": len(u), "no_template_in_pool": sum(1 for r in u if r in pool_ids),
                      "by_set": {k: sum(1 for r in u if k in member[r]) for k in SETS},
                      "pool_rulings": len(pm), "pool_with_template": sum(1 for r in pm if covered[r]),
                      "categories": collections.Counter(lab[r]["category"] for r in u).most_common(),
                      "nouns": collections.Counter(lab[r]["product"].lower() for r in u).most_common(10),
                      "_pool": pm, "_unc": u})
    trows.sort(key=lambda x: (-x["no_template"], -x["pool_rulings"], x["type"]))
    teligible = [x for x in trows if x["no_template"] >= TYPE_MIN and x["pool_in_regime"] >= TYPE_MIN_POOL]
    picked = []
    for x in teligible[:CAP]:
        per = {reg: [r for r in x["_pool"] if pool in member[r]] for pool, reg in POOLS.items()}
        regime = "part102" if len(per["part102"]) > len(per["substantial_transformation"]) else "substantial_transformation"
        other = "substantial_transformation" if regime == "part102" else "part102"
        picked.append({"type": x["type"], "no_template": x["no_template"], "regime": regime,
                       "pool_members": per[regime], "other_regime_pool_members": per[other],
                       "no_template_members": x["_unc"]})
    untyped = [r for r in unc if typ(r) is None]
    for x in trows:
        del x["_pool"], x["_unc"]
    return {
        "rule": __doc__.split("Rule, ")[1].split("\n\nRun")[0].replace("\n", " "),
        "sets": counts,
        "selection_rulings": len(member), "with_template": len(member) - len(unc), "no_template": len(unc),
        "step1_categories": rows,
        "step1_kept": [x["category"] for x in eligible[:CAP]],
        "types": trows,
        "untyped_no_template": len(untyped),
        "untyped_by_category": collections.Counter(lab[r]["category"] for r in untyped).most_common(),
        "not_picked_at_or_over_min": [x["type"] for x in teligible[CAP:]],
        "picked": picked,
    }


if __name__ == "__main__":
    root = sys.argv[1]
    want = json.dumps(build(root), indent=1) + "\n"
    path = os.path.join(root, OUT)
    if "--check" in sys.argv:
        with open(path) as f:
            if f.read() != want:
                sys.exit(OUT + " is stale: run eval/families/select_r3.py")
        print(OUT, "ok")
    else:
        with open(path, "w") as f:
            f.write(want)
        print("wrote", OUT)
