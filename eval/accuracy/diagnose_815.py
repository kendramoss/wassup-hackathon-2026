"""#815: miss table for the U.S. engine on the development split, food and no-family slices.
No model calls. Reads committed result files, writes 815-misses.json. Development rulings only.

  Engine rows : eval/results/dev-gemini-4-argon-prec-engine-v1-vertex.jsonl (the official development run of the
                production configuration: gemini-4-argon, engine-v1, BM25 k 5, no router; 422 of 469).
  Family slice: eval/results/families-coverage-dev-all-litellm-f26-b2-g3.jsonl, the last family check that covers
                all 469 development rulings (26 templates, guard v3); a ruling is in its matched template at
                confidence >= 0.6, else "none" (the rule D840/D842 use). Food = food_preparations.
  Unstable    : the miss was answered correctly by at least one of three other runs of the same configuration
                (gateway: -litellm, -litellm-gw, -seed-litellm).

Run from the repository root:
    python3 -I -B eval/accuracy/diagnose_815.py .          # write
    python3 -I -B eval/accuracy/diagnose_815.py . --check  # verify
"""
import json
import os
import sys

R = "eval/results/"
ENGINE = R + "dev-gemini-4-argon-prec-engine-v1-vertex.jsonl"
SAME = [R + "dev-gemini-4-argon-prec-engine-v1-litellm.jsonl",
        R + "dev-gemini-4-argon-prec-engine-v1-litellm-gw.jsonl",
        R + "dev-gemini-4-argon-prec-engine-v1-seed-litellm.jsonl"]
FAMILY = R + "families-coverage-dev-all-litellm-f26-b2-g3.jsonl"
OUT = "eval/accuracy/815-misses.json"
BAR = 0.6

# Cause of each miss, assigned by reading the ruling facts, CBP's held origin and the engine's reasoning and
# retrieved precedents (development rulings only). retrieval = the retrieved precedents misled or lacked the
# analogue; rule = the engine applied the wrong side of a CBP rule; fact = a stated fact was missed;
# label = the label or the case is ambiguous (two scenarios, or the engine flips between identical runs).
CAUSE = {
    "N350517": ("rule", "Pureeing and blending fruit, then pasteurising, was read as simple processing; CBP held France (a different food: puree). No fruit-puree precedent exists before this date, so retrieval is not the cause."),
    "N352231": ("label", "Cooking and canning razor clams was read as a transformation; CBP held the harvest country. The gateway run (-litellm-gw) was right: the call is close."),
    "N352389": ("rule", "Esterification and distillation of crude fish oil was read as a chemical transformation; CBP held Peru (refining an oil). Label is manual."),
    "N354481": ("rule", "Cooking skewered chicken was read as a transformation (N345699, fully cooked, battered chicken: China); CBP held the United States, as for chargrilled chicken (N339749). Retrieved precedents point both ways."),
    "H331375": ("rule", "House kit made in China, assembled in Italy, used, disassembled: the engine kept China; HQ held Italy, where the kit last became the house."),
    "H354210": ("rule", "Retanning and dyeing U.S. wet blue hides in Brazil was read as a transformation to crust leather; HQ held the United States."),
    "N348376": ("rule", "Sintered spinel from China, then crushed, milled and classified in Germany: engine kept China (crushing is not a transformation); CBP held Germany. Label is manual."),
    "N349972": ("label", "Helmet shell moulded in Taiwan, liner and assembly in China. Essential character is close; the engine flips between identical runs (right in -litellm-gw)."),
    "N350091": ("rule", "Pen with an Indian refill and Chinese barrel parts assembled in China: the engine named N345964 (refill gives character) and still chose China, at 0.65."),
    "N351081": ("rule", "Air filter: CBP held China, where the filter paper is made; the engine took Thailand, where it was pleated and assembled."),
    "N351073": ("label", "Oven lights with two scenarios; the label is the second scenario's result and the engine answered the first. The ruling has two answers."),
    "N351174": ("label", "Jack stands: Thai ratchet bar, Chinese base parts, assembly in Thailand. Close; right in two other identical runs."),
    "N351368": ("rule", "Door lock: Chinese knob assemblies, Vietnamese cylinder and latch. CBP held China; the engine held Vietnam. Retrieved precedents are ovens and ranges, not locks."),
    "N352485": ("retrieval", "Four retrieved sibling air filters all held China (Chinese media); here the media is composited and pleated in Thailand and CBP held Thailand. The near-duplicate precedents anchored the answer."),
    "N353232": ("label", "Soap: UK soap base, mixed and moulded in Canada. Close; right in one other identical run."),
    "N353581": ("rule", "Borax pentahydrate (U.S.) made into decahydrate in France: the engine held that a hydration change is not a transformation; CBP held France (new chemical name and use)."),
    "N353597": ("rule", "Metal tiara body from China, Cambodian headband and combs, gluing, screwing and ribbon in Cambodia: engine kept China; CBP held Cambodia."),
    "N353879": ("rule", "Showerhead: about 40 parts from China, assembled and tested in Thailand. The engine called it simple assembly; CBP held Thailand."),
    "N356520": ("label", "Deodorising filter: Korean core and mesh, Chinese carbon, filled and heat-pressed in Korea. The five retrieved water filters all held Korea; right in one other identical run."),
}


def load(p):
    with open(p) as f:
        return {r["ruling"]: r for r in (json.loads(l) for l in f if l.strip())}


def build():
    eng = load(ENGINE)
    same = [load(p) for p in SAME]
    fam = load(FAMILY)

    def slice_of(k):
        r = fam.get(k)
        ok = bool(r) and r.get("covered") and not r.get("error") and (r.get("confidence") or 0) >= BAR
        f = r["family"] if ok else "none"
        return "food" if f == "food_preparations" else ("none" if f == "none" else "other")

    counts = {}
    for k, r in eng.items():
        c = counts.setdefault(slice_of(k), {"n": 0, "correct": 0})
        c["n"] += 1
        c["correct"] += bool(r["correct"])
    misses = []
    for k, r in sorted(eng.items()):
        s = slice_of(k)
        if r["correct"] or s == "other":
            continue
        cause, why = CAUSE[k]
        misses.append({
            "ruling": k, "slice": s, "family_run": fam[k]["family"] if s != "none" else "none",
            "gold": r["gold"], "engine": r["pred"], "confidence": r["confidence"],
            "label_method": r["label_method"], "cause": cause, "why": why,
            "unstable": any(d[k]["correct"] for d in same),
            "correct_in_other_runs": sum(bool(d[k]["correct"]) for d in same),
        })
    assert set(m["ruling"] for m in misses) == set(CAUSE), "cause table out of step with the misses"
    by = {}
    for m in misses:
        by.setdefault(m["slice"], {}).setdefault(m["cause"], 0)
        by[m["slice"]][m["cause"]] += 1
    allc = {}
    for m in misses:
        allc[m["cause"]] = allc.get(m["cause"], 0) + 1
    china = sum(1 for k, r in eng.items() if r["pred"] == "China")
    china_ok = sum(1 for k, r in eng.items() if r["pred"] == "China" and r["correct"])
    return {
        "note": "Development split only (469). No held-out ruling is read. Causes assigned by hand (see diagnose_815.py).",
        "engine_run": os.path.basename(ENGINE), "family_run": os.path.basename(FAMILY),
        "slice_counts": counts, "causes_by_slice": by, "causes_all": allc,
        "china_predictions": {"predicted": china, "correct": china_ok},
        "misses": misses,
    }


def main():
    root = sys.argv[1]
    os.chdir(root)
    doc = build()
    text = json.dumps(doc, indent=1, ensure_ascii=False) + "\n"
    if "--check" in sys.argv:
        with open(OUT) as f:
            if f.read() != text:
                sys.exit("815-misses.json is out of date")
        print("ok")
        return
    with open(OUT, "w") as f:
        f.write(text)
    print(json.dumps({k: doc[k] for k in ("slice_counts", "causes_by_slice", "causes_all", "china_predictions")}))


if __name__ == "__main__":
    main()
