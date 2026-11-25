"""Build the food-fresh split (D858): food rulings the food-v1 text was not written from.

Usage: python3 -I eval/accuracy/build_foodfresh.py <repo> --frozen-at <time>
Reads labels.jsonl, splits_tune.json, labels_foodfresh.jsonl and the facts
directories; writes eval/splits/splits_foodfresh.json. No model calls.
"""
import argparse, json, os, re, sys

# D856 names these as the rulings food-v1 was written from.
SOURCE = {"N350517", "N352231", "N352389", "N354481", "H326919", "N330420", "N331014",
          "N335413", "N337385", "N338956", "N339749", "N333864", "N334669", "N345699"}
# Training rulings whose facts and CBP holding the writer of food-v1 read (#821).
SEEN = {"N333285", "N334421", "H331430", "H333793", "N338945", "N340558", "N341162",
        "N342920", "N343631", "N344668", "N344994", "N344995", "N344996", "N345710",
        "N346762", "N347400", "N347924"}
# Left out by hand, with the reason (D858).
EXCLUDED = {"H317062": "a trade-agreement claim on tahini; the label is the seeds' origin, not the tahini's, and it shares its importer and question with source ruling H326919"}


def gate(food_go):
    src = open(food_go).read()
    strong = re.compile(re.search(r"foodStrong = regexp.MustCompile\(`(.*?)`\)", src).group(1))
    weak = re.compile(re.search(r"foodWeak = regexp.MustCompile\(`(.*?)`\)", src).group(1))
    return lambda t: bool(strong.search(t)) or len({m.lower() for m in weak.findall(t)}) >= 2


def load(path):
    return [json.loads(l) for l in open(path) if l.strip()]


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("repo")
    ap.add_argument("--frozen-at", required=True)
    a = ap.parse_args()
    R = lambda *p: os.path.join(a.repo, *p)
    food = gate(R("backend", "internal", "engine", "food.go"))

    def facts(rid, dirs):
        for d in dirs:
            p = R(*d, rid + ".txt")
            if os.path.exists(p):
                return open(p).read()
        return ""

    def keep(l, dirs):
        o = l.get("primary_origin")
        return (l.get("regime") == "substantial_transformation" and o and "|" not in o
                and food(facts(l["ruling"], dirs)))

    labels = {l["ruling"]: l for l in load(R("eval", "data", "labels.jsonl"))}
    tune = json.load(open(R("eval", "splits", "splits_tune.json")))
    train = [r for k in ("train", "tune_check") for r in tune[k]
             if r not in SOURCE and r not in SEEN and keep(labels[r], [("backend", "data", "facts")])]
    fresh = [l for l in load(R("eval", "data", "labels_foodfresh.jsonl"))
             if l["ruling"] not in EXCLUDED and keep(l, [("eval", "data", "facts_foodfresh")])]
    ids = sorted(set(train) | {l["ruling"] for l in fresh})
    out = {
        "frozen_at": a.frozen_at,
        "rule": "food rulings food-v1 was not written from (D858): (a) training and tune_check rulings that pass engine.FoodCase with one held origin, minus the D856 source rulings and the 17 the writer read; (b) rulings in labels_foodfresh.jsonl (cited by the corpus's food rulings, or published after 13 Nov 2026) that are substantial transformation, have one held origin and pass engine.FoodCase; H317062 left out by hand",
        "excluded": EXCLUDED,
        "foodfresh": ids,
    }
    with open(R("eval", "splits", "splits_foodfresh.json"), "w") as f:
        json.dump(out, f, indent=2)
        f.write("\n")
    print(f"foodfresh: {len(ids)} rulings ({len(train)} training, {len(ids) - len(train)} new)", file=sys.stderr)


if __name__ == "__main__":
    main()
