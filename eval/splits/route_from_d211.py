# Writes eval/results/route-final2-router-v2-d211.jsonl (D230): router-v2's
# labels on the 107 post-cutoff rulings as the D211 runs recorded them
# (router_regime, router_route, router_subfamily on each row; the textile and
# Part 102 runs agree on every ruling they share), in the `eval route` row
# format. No model call: this is the router the product ran, on the rulings
# D211 scored, so the routed path of router-v2 is the product today.
#
# Usage: python3 -I eval/splits/route_from_d211.py .

import json
import sys

root = sys.argv[1] if len(sys.argv) > 1 else "."
FILES = [
    "eval/results/final2-gemini-4-argon-prec-engine-v1-litellm-router.jsonl",
    "eval/results/textile-final2-gemini-4-argon-prec-textile-v2-litellm-reasoning-v1-router.jsonl",
    "eval/results/pharma-final2-gemini-4-argon-prec-pharma-v1-litellm-router.jsonl",
    "eval/results/usmca-final2-gemini-4-argon-prec-usmca-v1-litellm-router.jsonl",
]
ENGINE = {"textile_102_21": "textile_102_21", "usmca_part102": "usmca_part102", "pharma_api": "pharma_api"}
with open(f"{root}/eval/splits/splits_router.json") as f:
    sp = json.load(f)
labels = {}
for p in FILES:
    with open(f"{root}/{p}") as f:
        for line in f:
            r = json.loads(line)
            lab = (r["router_regime"], r["router_route"], r["router_subfamily"])
            assert labels.setdefault(r["ruling"], lab) == lab, r["ruling"]
assert set(labels) == set(sp["final2"]), "the D211 rulings are the final2 router split"
with open(f"{root}/eval/results/route-final2-router-v2-d211.jsonl", "w") as f:
    for rid in sp["final2"]:
        regime, route, sub = labels[rid]
        gold = sp["gold"][rid]
        eng = ENGINE.get(regime, "general")
        row = {"ruling": rid, "split": "final2", "gold_engine": gold, "engine": eng, "correct": gold != "none" and eng == gold,
               "router": {"route": route, "subfamily": sub, "regime": regime, "reason": "", "model_id": "gemini-3.8-flash",
                          "llm_provider": "litellm", "prompt_version": "router-v2"}}
        f.write(json.dumps(row) + "\n")
print(len(sp["final2"]), "rows")
