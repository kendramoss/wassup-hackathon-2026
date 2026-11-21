# Builds the post-cutoff held-out splits of the other three regimes (D210):
#   eval/splits/splits_textile_final2.json   (textile, 19 CFR 102.21)
#   eval/splits/splits_pharma_final2.json    (pharmaceutical, active ingredient)
#   eval/splits/splits_usmca_final2.json     (Part 102 marking origin, the usmca_part102 engine)
#   eval/data/labels_usmca_final2.jsonl      (the post-cutoff Part 102 rulings, the extra precedent pool)
#
# Usage: python3 -I eval/splits/build_regimes_final2_split.py . --frozen-at 2026-11-21T…
#   (from the repo root; needs data/corpus/raw with the 163 post-cutoff rulings,
#    which `go run ./cmd/corpus probe` saved, and eval/data/facts_final2)
#
# Candidates are the 163 rulings of eval/data/labels_final2.jsonl (28 Sep to
# 13 Nov 2026, D150). Each regime keeps the rule of its first split builder,
# applied unchanged to these rulings:
#   textile  (build_textile_split.py):  regime part102_tariff_shift, the raw
#            CROSS file contains "102.21", a primary origin, a facts file.
#   pharma   (build_pharma_split.py):   regime pharma_api, a primary origin, a
#            facts file.
#   usmca    (build_usmca_split.py and corpus usmca-label): the raw text cites
#            102.20 or 102.11 and applies the Part 102 rules to a non-textile
#            good (corpus.USMCARegime, ported below), a primary origin, a facts
#            file; rulings whose holding answers only a substantial-
#            transformation, Section 301 or procurement question are precedents
#            but not scored.
# One correction, decided from the ruling text before any scoring and listed in
# REGIME_CORRECTIONS: two rulings the Go labeller put in pharma_api are steel
# tubes made to "API 5CT", the American Petroleum Institute standard. They are
# not pharmaceutical and are left out of the pharmaceutical split.
#
# The textile rule is kept as it was so the result compares with the first
# window. It matches "102.21" anywhere in the raw file, and Part 102 rulings on
# non-textile goods mention 102.21 in their boilerplate ("textile and apparel
# goods ... are subject to ... 102.21"). So most textile-rule rulings are also
# Part 102 rulings. The split records which ones apply 102.21 itself
# ("applies_102_21") and which are in both splits; the pooled tables count a
# ruling once (D210).

import collections
import json
import os
import re
import sys

root = sys.argv[1] if len(sys.argv) > 1 else "."
frozen_at = None
if "--frozen-at" in sys.argv:
    frozen_at = sys.argv[sys.argv.index("--frozen-at") + 1]

REGIME_CORRECTIONS = {
    "N365097": "steel oil country tubular goods made to API 5CT (American Petroleum Institute), not a pharmaceutical; labelled pharma_api by corpus.ClassifyRegime",
    "N365098": "steel oil country tubular goods made to API 5CT (American Petroleum Institute), not a pharmaceutical; labelled pharma_api by corpus.ClassifyRegime",
}

# corpus.USMCARegime (backend/internal/corpus/holding.go), same patterns.
PART102_CITE = re.compile(r"102\.(?:20|11)\b")
PART102_APPLIED = re.compile(r"(?i)102\.11\s*\(\s*[a-d]\s*\)|tariff shift|change in tariff classification")
TEXTILE_APPLIED = re.compile(r"102\.21\s*\(")
# build_usmca_split.py, same patterns.
OTHER = re.compile(r"substantial(ly)? transform|Section 301|trade remed|procurement|Trade Agreements Act", re.I)
PART102 = re.compile(r"marking|102\.|tariff shift|USMCA|NAFTA", re.I)


def rows(p):
    with open(f"{root}/{p}") as f:
        return [json.loads(l) for l in f if l.strip()]


def raw_file(rid):
    with open(f"{root}/data/corpus/raw/{rid}.json", errors="ignore") as f:
        return f.read()


def raw_text(rid):
    return json.loads(raw_file(rid))["text"]


def has_facts(rid):
    return os.path.exists(f"{root}/eval/data/facts_final2/{rid}.txt")


def other_question(r):
    ev = " ".join(r["evidence"])
    return bool(OTHER.search(ev)) and not PART102.search(ev)


def usmca_regime(text):
    return bool(PART102_CITE.search(text)) and bool(PART102_APPLIED.search(text)) and not TEXTILE_APPLIED.search(text)


labels = rows("eval/data/labels_final2.jsonl")
assert len(labels) == 163, len(labels)
assert all(r["date"] > "2026-09-25" for r in labels)
old_textile = json.load(open(f"{root}/eval/splits/splits_textile.json"))
old_pharma = json.load(open(f"{root}/eval/splits/splits_pharma.json"))
old_usmca = {r["ruling"] for r in rows("eval/data/labels_usmca.jsonl")}
assert not any(r["ruling"] in old_usmca for r in labels)
key = lambda r: (r["date"], r["ruling"])

# Textile.
tex = [r for r in labels if r["regime"] == "part102_tariff_shift" and r.get("primary_origin")
       and "102.21" in raw_file(r["ruling"]) and has_facts(r["ruling"])]
tex.sort(key=key)
# Pharmaceutical.
ph_all = [r for r in labels if r["regime"] == "pharma_api"]
ph = [r for r in ph_all if r.get("primary_origin") and has_facts(r["ruling"]) and r["ruling"] not in REGIME_CORRECTIONS]
ph.sort(key=key)
# Part 102 (usmca_part102 engine).
us_pool = [r for r in labels if usmca_regime(raw_text(r["ruling"])) and has_facts(r["ruling"])]
us_pool.sort(key=key)
us_lab = [r for r in us_pool if r.get("primary_origin")]
us = [r for r in us_lab if not other_question(r)]

tex_ids = [r["ruling"] for r in tex]
us_ids = [r["ruling"] for r in us]
both = sorted(set(tex_ids) & set(us_ids))
applies = [r["ruling"] for r in tex if TEXTILE_APPLIED.search(raw_text(r["ruling"]))]

common = {
    "frozen_at": frozen_at,
    "window": "rulings dated 2026-09-28 to 2026-11-13 in eval/data/labels_final2.jsonl (the second held-out window, D150)",
    "scoring": "held out: scored once with the production engine of the regime, through the gateway (-split final2 -confirm-final, D210)",
}


def dump(p, obj):
    with open(f"{root}/{p}", "w") as f:
        json.dump(obj, f, indent=1, ensure_ascii=False)
        f.write("\n")


dump("eval/splits/splits_textile_final2.json", {
    **common,
    "source": "eval/data/labels_final2.jsonl rulings with regime part102_tariff_shift whose raw CROSS file contains '102.21', "
              "with a primary origin and a facts file (the build_textile_split.py rule, D51)",
    "rule": "the D51 textile rule applied unchanged to the post-cutoff window. Precedents: the first textile pool plus these rulings, each usable only for cases dated after it.",
    "funnel": {"labelled_rulings": len(labels), "part102_tariff_shift": sum(r["regime"] == "part102_tariff_shift" for r in labels), "in_split": len(tex)},
    "applies_102_21": applies,
    "also_in_part102_split": both,
    "origins": dict(collections.Counter(r["primary_origin"] for r in tex).most_common()),
    "textile": old_textile["textile"] + tex_ids,
    "final2": tex_ids,
})
dump("eval/splits/splits_pharma_final2.json", {
    **common,
    "source": "eval/data/labels_final2.jsonl rulings with regime pharma_api, a primary origin and a facts file (the build_pharma_split.py rule, D62)",
    "rule": "the D62 pharmaceutical rule applied unchanged to the post-cutoff window, less the regime corrections listed. Precedents: the first pharmaceutical pool plus these rulings, each usable only for cases dated after it.",
    "funnel": {"labelled_rulings": len(labels), "pharma_api": len(ph_all),
               "regime_corrected_out": len([r for r in ph_all if r["ruling"] in REGIME_CORRECTIONS]),
               "without_primary_origin": len([r for r in ph_all if not r.get("primary_origin") and r["ruling"] not in REGIME_CORRECTIONS]),
               "in_split": len(ph)},
    "regime_corrections": REGIME_CORRECTIONS,
    "origins": dict(collections.Counter(r["primary_origin"] for r in ph).most_common()),
    "pharma": old_pharma["pharma"] + [r["ruling"] for r in ph],
    "final2": [r["ruling"] for r in ph],
})
dump("eval/splits/splits_usmca_final2.json", {
    **common,
    "source": "eval/data/labels_usmca_final2.jsonl: the post-cutoff rulings whose raw text applies 19 CFR 102.11 / 102.20 to a non-textile good (corpus.USMCARegime), "
              "with a primary origin and a facts file (the build_usmca_split.py rule, D61)",
    "rule": "the D61 Part 102 rule applied unchanged to the post-cutoff window. Rulings whose holding answers only a substantial-transformation, "
            "Section 301 or procurement question are precedents but not scored. Precedents: eval/data/labels_usmca.jsonl plus eval/data/labels_usmca_final2.jsonl, each usable only for cases dated after it.",
    "funnel": {"labelled_rulings": len(labels), "part102_by_text": len(us_pool), "with_primary_origin": len(us_lab),
               "other_question_not_scored": len(us_lab) - len(us), "in_split": len(us)},
    "not_scored_other_question": [r["ruling"] for r in us_lab if other_question(r)],
    "also_in_textile_split": both,
    "origins": dict(collections.Counter(r["primary_origin"] for r in us).most_common()),
    "final2": us_ids,
})
with open(f"{root}/eval/data/labels_usmca_final2.jsonl", "w") as f:
    for r in us_pool:
        f.write(json.dumps(r, ensure_ascii=False) + "\n")

print(json.dumps({"textile": len(tex), "applies_102_21": len(applies), "pharma": len(ph), "usmca": len(us),
                  "usmca_pool": len(us_pool), "textile_and_usmca": len(both)}))
