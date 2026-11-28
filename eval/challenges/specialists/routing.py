"""Legal-routing cases: the specialist cases whose regime the router labels,
plus general-origin negative controls.

Appropriate cases reuse the facts of the textile, pharmaceutical and Part 102
specialist cases (Japan cases are not routed: the router has no Japan regime).
Their expected regime is the specialist's own regime and the case keeps its
specialist case as parent. Negative controls are real CBP rulings of the corpus
whose recorded regime is substantial_transformation, dated from 2026-01, split
by date (dev: Jan-Apr; test: May-Sep) so no router tuning window contains them.
The expected regime of a control is the corpus's recorded regime for the ruling
(built by the corpus labeller, not by the router).
"""
import os, sys
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from common import *

REGIME = {"textile_specialist": "textile_102_21", "pharma_specialist": "pharma_api", "part102_specialist": "usmca_part102"}
GROUP = {"textile_specialist": "textile", "pharma_specialist": "pharma", "part102_specialist": "part102"}
NEG = {"dev": 45, "test": 110}


def negatives(split):
    rows = [json.loads(l) for l in open(os.path.join(DATA, "labels.jsonl"))]
    lo, hi = ("2026-01-01", "2026-05-01") if split == "dev" else ("2026-05-01", "2026-10-01")
    pool = [r for r in rows if r["regime"] == "substantial_transformation" and lo <= r["date"] < hi and os.path.exists(os.path.join(DATA, "facts", r["ruling"] + ".txt"))]
    pool.sort(key=lambda r: hashlib.sha256(("neg|" + r["ruling"]).encode()).hexdigest())
    out = []
    for r in pool[:NEG[split]]:
        facts = open(os.path.join(DATA, "facts", r["ruling"] + ".txt"), encoding="utf-8").read().strip()
        out.append((r, facts))
    return out


def build(split, spec):
    out = []
    for feat, cases in spec.items():
        for c in cases:
            out.append(make_case("legal_routing", split, "route|" + c["id"], "route-" + GROUP[feat], GROUP[feat], "routing-of-specialist-case",
                                 ["appropriate-specialist-case", c["expected"]["kind"]], c["sources"],
                                 "the router should label this %s case with the %s regime; its facts are those of the specialist case" % (GROUP[feat], REGIME[feat]),
                                 c["input"]["facts"], {"kind": "decided", "origin": REGIME[feat]}, parents=[c["id"]]))
    for r, facts in negatives(split):
        out.append(make_case("legal_routing", split, "neg|" + r["ruling"], "negctl-" + r["date"][:7], "negative-control", "general-origin-ruling",
                             ["negative-control", "real-ruling-facts"],
                             [{"id": "cbp-" + r["ruling"].lower(), "locator": "CBP ruling %s (%s): recorded regime substantial_transformation in the corpus labels" % (r["ruling"], r["date"])}],
                             "a general-origin ruling of 2026 whose recorded regime is substantial transformation: the router must not send it to a specialist regime",
                             facts, {"kind": "decided", "origin": "substantial_transformation"}, parents=[r["ruling"]]))
    return out
