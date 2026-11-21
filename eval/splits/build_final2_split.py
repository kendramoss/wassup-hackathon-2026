# Builds eval/splits/splits_final2.json, the second held-out split (D150).
# Usage: python3 -I eval/splits/build_final2_split.py . [--frozen-at 2026-11-21T15:00+08:00]
#   (from the repo root; needs eval/data/labels_final2.jsonl from
#    `go run ./cmd/corpus final2-label` and the facts files it wrote)
#
# Rule, the same one that reproduces the first held-out split (docs/EVALUATION.md 2.2):
#   substantial-transformation regime, facts naming two or more countries,
#   a held origin, and that origin a single country ("A | B" holdings are
#   excluded), with a non-empty facts section. Every ruling is dated after
#   2026-09-25, the last date of the first held-out split.

import collections
import json
import os
import sys

root = sys.argv[1] if len(sys.argv) > 1 else "."
frozen_at = None
if "--frozen-at" in sys.argv:
    frozen_at = sys.argv[sys.argv.index("--frozen-at") + 1]


def rows(p):
    with open(f"{root}/{p}") as f:
        return [json.loads(l) for l in f if l.strip()]


labels = rows("eval/data/labels_final2.jsonl")
probe = rows("eval/data/final2_probe.jsonl")
old = {r["ruling"] for r in rows("eval/data/labels.jsonl")}

assert not any(r["ruling"] in old for r in labels), "a final2 ruling is already in labels.jsonl"
assert all(r["date"] > "2026-09-25" for r in labels), "a final2 ruling is dated inside the first held-out window"

steps = collections.OrderedDict()
steps["labelled_rulings"] = len(labels)
st = [r for r in labels if r["regime"] == "substantial_transformation"]
steps["substantial_transformation"] = len(st)
multi = [r for r in st if r["multi_country"]]
steps["multi_country"] = len(multi)
held = [r for r in multi if r["primary_origin"]]
steps["with_held_origin"] = len(held)
single = [r for r in held if "|" not in r["primary_origin"]]
steps["excluded_two_or_more_countries_held"] = len(held) - len(single)
keep = [r for r in single if os.path.exists(f"{root}/eval/data/facts_final2/{r['ruling']}.txt")]
steps["without_facts_file"] = len(single) - len(keep)
keep.sort(key=lambda r: (r["date"], r["ruling"]))
steps["in_split"] = len(keep)

regimes = collections.Counter(r["regime"] for r in labels)
regimes_with_origin = collections.Counter(r["regime"] for r in labels if r["primary_origin"] and "|" not in r["primary_origin"])
probed = collections.defaultdict(lambda: {"from": None, "to": None, "numbers": 0, "found": 0})
seen = set()
for p in probe:
    if p["ruling"] in seen:
        continue
    seen.add(p["ruling"])
    k = p["ruling"][0]
    n = int(p["ruling"][1:])
    b = probed[k]
    b["from"] = n if b["from"] is None else min(b["from"], n)
    b["to"] = n if b["to"] is None else max(b["to"], n)
    b["numbers"] += 1
    b["found"] += p["found"]

out = {
    "source": "CBP rulings published on rulings.cbp.gov after the first held-out window, found by ruling number "
              "(go run ./cmd/corpus probe; eval/data/final2_probe.jsonl lists every number tried) and labelled by "
              "go run ./cmd/corpus final2-label (eval/data/labels_final2.jsonl; manual corrections in eval/data/final2_manual.json)",
    "frozen_at": frozen_at,
    "rule": "second held-out split (D150): rulings dated 2026-09-26 or later; substantial-transformation regime, facts naming "
            "two or more countries, a single held origin ('A | B' holdings excluded), a facts section. Frozen before any engine "
            "or model was scored on it; every candidate is scored on it at most once (-confirm-final).",
    "date_min": keep[0]["date"] if keep else None,
    "date_max": keep[-1]["date"] if keep else None,
    "funnel": steps,
    "regimes_labelled": dict(sorted(regimes.items())),
    "regimes_with_single_held_origin": dict(sorted(regimes_with_origin.items())),
    "probe_ranges": {k: dict(v) for k, v in sorted(probed.items())},
    "collections": dict(collections.Counter(r["collection"] for r in keep)),
    "label_method": dict(collections.Counter(r["method"] for r in keep)),
    "origins": dict(collections.Counter(r["primary_origin"] for r in keep).most_common()),
    "final2": [r["ruling"] for r in keep],
}
with open(f"{root}/eval/splits/splits_final2.json", "w") as f:
    json.dump(out, f, indent=1, ensure_ascii=False)
    f.write("\n")
print(json.dumps({k: v for k, v in out.items() if k not in ("final2", "origins", "source", "rule")}, indent=1))
