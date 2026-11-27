"""Scores a method against truth3 (determinable and contested cases, D959), offline, no model call.

A ruling is flagged when the method flags at least one order of its pool. On determinable rulings (status positive or negative):
a flag is right when the ruling is positive and a flagged order is one the ruling is known to be within (unanimous or cited);
precision = right / flagged, recall = right / positives. Contested rulings are counted apart: how many the method flags, and
how many of them have an order some labeller found within (the flag is "possible exposure, a person must decide").
`flags` maps a pair key to True or False (None: no answer, counted as missing and not flagged).
"""
import importlib.util
import json
import os

HERE = os.path.dirname(os.path.abspath(__file__))
spec = importlib.util.spec_from_file_location("score", os.path.join(HERE, "score.py"))
S = importlib.util.module_from_spec(spec)
spec.loader.exec_module(S)


def score3(split, pairs_path, flags, truth_file="truth3.jsonl"):
    """flags: dict key -> bool for the pairs the method read. Pairs not in `flags` were not asked and are not flagged."""
    truth = {t["id"]: t for t in S.load(os.path.join(split, truth_file))}
    by = {}
    for p in S.load(pairs_path):
        if p["id"] in truth and flags.get(p["key"]):
            by.setdefault(p["id"], set()).update(p["cases"])
    TP = FP = FN = TN = 0
    c = {"contested": 0, "contested_flagged": 0, "contested_flagged_with_in_vote": 0, "contested_pos_flagged_unanimous": 0}
    wrong_group = 0
    for i, t in truth.items():
        fl = by.get(i, set())
        if t["status"] == "contested":
            c["contested"] += 1
            if fl:
                c["contested_flagged"] += 1
                if fl & {x for g in t["contested_groups"] for x in g.split()}:
                    c["contested_flagged_with_in_vote"] += 1
            continue
        known = {x for g in t["truth_groups"] for x in g.split()}
        pos = t["status"] == "positive"
        right = bool(fl) and pos and bool(fl & known)
        if right:
            TP += 1
        elif fl:
            FP += 1
            if pos:
                FN += 1
                wrong_group += 1
        elif pos:
            FN += 1
        else:
            TN += 1
    out = {"TP": TP, "FP": FP, "FN": FN, "TN": TN, "precision": S.wilson(TP, TP + FP), "recall": S.wilson(TP, TP + FN), "flagged_wrong_group": wrong_group, **c}
    return out


def fmt(name, o):
    return "%-60s P %5.1f%% (%d/%d)  R %5.1f%% (%d/%d)  contested %d flagged %d" % (
        name, 100 * (o["precision"]["p"] or 0), o["precision"]["k"], o["precision"]["n"], 100 * (o["recall"]["p"] or 0), o["recall"]["k"], o["recall"]["n"], o["contested"], o["contested_flagged"])
