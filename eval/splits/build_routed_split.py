# Builds the split files for the routed end-to-end path (D230): for one
# post-cutoff route file, the (ruling, engine) pairs the D211 runs do not
# already hold, one split file per engine, so each pair is run once with
# `eval run -regime <engine> -split final2 -confirm-final -tag routed-<tag>`.
#
# Usage: python3 -I eval/splits/build_routed_split.py . <route file> <tag>
#   e.g. eval/results/route-final2-router-v2-gemini-3.8-flash-litellm.jsonl v2
#
# A pair is already held when the D211 run of that engine scored the ruling:
#   general        eval/results/final2-gemini-4-argon-prec-engine-v1-litellm-router.jsonl
#   textile_102_21 eval/results/textile-final2-gemini-4-argon-prec-textile-v2-litellm-reasoning-v1-router.jsonl
#   pharma_api     eval/results/pharma-final2-gemini-4-argon-prec-pharma-v1-litellm-router.jsonl
#   usmca_part102  eval/results/usmca-final2-gemini-4-argon-prec-usmca-v1-litellm-router.jsonl
# Each split file keeps the precedent pool of its engine's D211 split file
# (textile and pharma pools), with final2 = the rulings to run.

import json
import os
import sys

root, route_file, tag = sys.argv[1], sys.argv[2], sys.argv[3]
D211 = {
    "general": ("eval/results/final2-gemini-4-argon-prec-engine-v1-litellm-router.jsonl", "eval/splits/splits_final2.json"),
    "textile_102_21": ("eval/results/textile-final2-gemini-4-argon-prec-textile-v2-litellm-reasoning-v1-router.jsonl", "eval/splits/splits_textile_final2.json"),
    "pharma_api": ("eval/results/pharma-final2-gemini-4-argon-prec-pharma-v1-litellm-router.jsonl", "eval/splits/splits_pharma_final2.json"),
    "usmca_part102": ("eval/results/usmca-final2-gemini-4-argon-prec-usmca-v1-litellm-router.jsonl", "eval/splits/splits_usmca_final2.json"),
}


def last_rows(p):
    out = {}
    with open(f"{root}/{p}") as f:
        for line in f:
            if line.strip():
                r = json.loads(line)
                out[r["ruling"]] = r
    return out


held = {e: set(last_rows(f)) for e, (f, _) in D211.items()}
routes = last_rows(route_file)
need = {e: [] for e in D211}
for rid, r in sorted(routes.items()):
    eng = r.get("engine")
    if eng and rid not in held[eng]:
        need[eng].append(rid)
os.makedirs(f"{root}/eval/splits/routed", exist_ok=True)
for eng, ids in need.items():
    if not ids:
        continue
    with open(f"{root}/{D211[eng][1]}") as f:
        base = json.load(f)
    out = {k: v for k, v in base.items() if k in ("textile", "pharma")}
    out.update({
        "frozen_at": None,
        "rule": f"D230 routed path: post-cutoff rulings the router ({route_file}) sends to the {eng} engine that the D211 run of that engine did not score; each pair is run once.",
        "final2": ids,
    })
    p = f"eval/splits/routed/splits_routed_{tag}_{eng}.json"
    with open(f"{root}/{p}", "w") as f:
        json.dump(out, f, indent=1)
        f.write("\n")
    print(p, len(ids), ids)
