# Builds eval/splits/splits_router.json (D230): the router's own splits, each
# ruling with the engine its legal test belongs to ("gold engine").
#
# Usage: python3 -I eval/splits/build_router_split.py . --frozen-at 2026-11-21T…
#   (from the repo root; needs data/corpus/raw for the 102.21 check)
#
# No model output is read. Every ruling comes from a committed engine split,
# so the router splits are made of rulings already split by date:
#   dev    (dated before 2026): the general engine's dev split (469), the
#          Jul-Dec 2025 scope window's 9802 and single-country rulings (the
#          rulings `eval scope` has always checked), the Part 102 dev split
#          (72), the pharmaceutical dev split (35) and the textile dev rulings
#          that apply 19 CFR 102.21 itself.
#   final  (the first held-out window, Jan-Sep 2026): the general engine's
#          final split (377), the Part 102 final split (32), the
#          pharmaceutical final split (56), the textile final rulings that
#          apply 102.21 itself.
#   final2 (the post-cutoff window, 28 Sep-13 Nov 2026): the same four engine
#          splits of the second window (D150, D210).
# Gold engine, first match wins:
#   pharma_api      in a pharmaceutical split;
#   usmca_part102   in a Part 102 split (non-textile goods; D210 counts a
#                   ruling in both the textile and the Part 102 split here);
#   textile_102_21  in a textile split and the raw text applies 102.21 itself
#                   ("102.21(" with a paragraph). The D51 textile rule matches
#                   "102.21" anywhere, which catches the Part 102 boilerplate
#                   (D210); those rulings are not textile cases;
#   general         the general engine's splits and the scope window's 9802
#                   and single-country rulings (any regime the general
#                   engine answers).
# Left out of routing accuracy and listed (gold engine "none", still routed):
# textile-split rulings that neither apply 102.21 nor are in a Part 102
# split, and rulings in both the general and a Part 102 split.

import json
import re
import sys

root = sys.argv[1] if len(sys.argv) > 1 else "."
frozen_at = sys.argv[sys.argv.index("--frozen-at") + 1] if "--frozen-at" in sys.argv else None

TEXTILE_APPLIED = re.compile(r"102\.21\s*\(")


def load(p):
    with open(f"{root}/{p}") as f:
        return json.load(f)


def rows(p):
    with open(f"{root}/{p}") as f:
        return [json.loads(l) for l in f if l.strip()]


def raw_text(rid):
    with open(f"{root}/data/corpus/raw/{rid}.json", errors="ignore") as f:
        return json.load(f)["text"]


st, st2 = load("eval/splits/splits_final.json"), load("eval/splits/splits_final2.json")
ph, ph2 = load("eval/splits/splits_pharma.json"), load("eval/splits/splits_pharma_final2.json")
us, us2 = load("eval/splits/splits_usmca.json"), load("eval/splits/splits_usmca_final2.json")
tx, tx2 = load("eval/splits/splits_textile.json"), load("eval/splits/splits_textile_final2.json")
labels = {r["ruling"]: r for r in rows("eval/data/labels.jsonl")}

# The Jul-Dec 2025 scope window (`eval scope` defaults): the first 40 rulings
# in file order of each regime the general engine answers besides
# substantial transformation, with a facts file.
window = []
count = {}
for r in rows("eval/data/labels.jsonl"):
    if not ("2025-07-01" <= r["date"] <= "2025-12-31") or r["regime"] not in ("9802_us_components", "single_country"):
        continue
    if count.get(r["regime"], 0) >= 40:
        continue
    try:
        open(f"{root}/data/corpus/facts/{r['ruling']}.txt").close()
    except OSError:
        continue
    count[r["regime"]] = count.get(r["regime"], 0) + 1
    window.append(r["ruling"])


def build(general, pharma, usmca, textile):
    gold, left_out = {}, []
    # A ruling in both the general and the Part 102 split answers two
    # questions (H342474, H345343: substantial transformation for duty, Part
    # 102 for marking); no single engine is right, so it is left out.
    both = set(general) & set(usmca)
    general = [i for i in general if i not in both]
    usmca = [i for i in usmca if i not in both]
    left_out.extend(both)
    for i in pharma:
        gold.setdefault(i, "pharma_api")
    for i in usmca:
        gold.setdefault(i, "usmca_part102")
    for i in textile:
        if i in gold:
            continue
        if TEXTILE_APPLIED.search(raw_text(i)):
            gold[i] = "textile_102_21"
        else:
            left_out.append(i)
    for i in general:
        gold.setdefault(i, "general")
    return gold, sorted(left_out)


splits = {
    "dev": build(st["dev"] + window, ph["dev"], us["dev"], tx["dev"]),
    "final": build(st["final"], ph["final"], us["final"], tx["final"]),
    "final2": build(st2["final2"], ph2["final2"], us2["final2"], tx2["final2"]),
}
# The windows must not share a ruling.
names = list(splits)
for a in range(len(names)):
    for b in range(a + 1, len(names)):
        assert not set(splits[names[a]][0]) & set(splits[names[b]][0]), (names[a], names[b])

out = {
    "frozen_at": frozen_at,
    "source": "the committed engine splits (splits_final, splits_final2, splits_pharma[_final2], splits_usmca[_final2], splits_textile[_final2]) and the Jul-Dec 2025 scope window of eval/data/labels.jsonl",
    "rule": "gold engine: pharma_api if in a pharmaceutical split; usmca_part102 if in a Part 102 split; textile_102_21 if in a textile split and the raw text applies 19 CFR 102.21 itself ('102.21('); general for the general engine's splits and the scope window's 9802 and single-country rulings; none (routed, not scored for routing) for textile-rule rulings that do not apply 102.21 and are in no Part 102 split, and for rulings in both the general and a Part 102 split. dev = rulings dated before 2026 (tune here only); final and final2 are held out: each router candidate is scored on them at most once (-confirm-final). Built without reading any model output (D230).",
    "counts": {},
    "left_out": {},
}
gold_all = {}
for name, (gold, left) in splits.items():
    # Left-out rulings stay in the split with gold engine "none": they are
    # routed (the routed end-to-end path of D230 covers every ruling of the
    # D211 pooled table) but not scored for routing.
    for i in left:
        gold[i] = "none"
    out[name] = sorted(gold)
    out["counts"][name] = {e: sum(1 for v in gold.values() if v == e) for e in ("general", "pharma_api", "usmca_part102", "textile_102_21", "none")}
    out["left_out"][name] = left
    gold_all.update(gold)
out["gold"] = dict(sorted(gold_all.items()))
with open(f"{root}/eval/splits/splits_router.json", "w") as f:
    json.dump(out, f, indent=1)
    f.write("\n")
print(json.dumps({"counts": out["counts"], "left_out": {k: len(v) for k, v in out["left_out"].items()}}))
