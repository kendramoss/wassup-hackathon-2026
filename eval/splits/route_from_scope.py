# Seeds eval/results/route-final-router-v2-gemini-3.8-flash-litellm.jsonl
# (D232) with router-v2's labels on the 377 general-engine rulings of the
# first held-out split, as the official Vertex AI scope run recorded them
# (eval/results/scope-final-router-v2-vertex.jsonl), so that router-v2 is not
# scored on them a second time. `eval route -split final -prompt router-v2
# -confirm-final` then routes only the rulings missing from the file (resume).
#
# Usage: python3 -I eval/splits/route_from_scope.py .

import json
import os
import sys

root = sys.argv[1] if len(sys.argv) > 1 else "."
out = f"{root}/eval/results/route-final-router-v2-gemini-3.8-flash-litellm.jsonl"
assert not os.path.exists(out), "already seeded"
ENGINE = {"textile_102_21", "usmca_part102", "pharma_api"}
with open(f"{root}/eval/splits/splits_router.json") as f:
    sp = json.load(f)
rows = {}
with open(f"{root}/eval/results/scope-final-router-v2-vertex.jsonl") as f:
    for line in f:
        r = json.loads(line)
        rows[r["ruling"]] = r
with open(out, "w") as f:
    for rid in sp["final"]:
        if rid not in rows:
            continue
        lab = rows[rid]["router"]
        gold = sp["gold"][rid]
        eng = lab["regime"] if lab["regime"] in ENGINE else "general"
        f.write(json.dumps({"ruling": rid, "split": "final", "gold_engine": gold, "engine": eng,
                            "correct": gold != "none" and eng == gold, "router": lab}) + "\n")
print(sum(1 for r in sp["final"] if r in rows), "rows seeded")
