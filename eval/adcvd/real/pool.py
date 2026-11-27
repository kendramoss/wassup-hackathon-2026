"""Pools score.py over several split directories (D945): adds the confusion counts and recomputes the Wilson intervals, offline.

Usage: python3 -I -B eval/adcvd/real/pool.py POLICY K SCREEN_MODEL READER_MODEL READER_VERSION DIR [DIR ...]   (each DIR holds queries, truth, pairs, method-screen.jsonl and reader.jsonl)
"""
import importlib.util
import json
import os
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
spec = importlib.util.spec_from_file_location("score", os.path.join(HERE, "score.py"))
S = importlib.util.module_from_spec(spec)
spec.loader.exec_module(S)


def run(policy, k, smodel, rmodel, rversion, dirs, hts=False, screen_name="method-screen.jsonl", reader_name="reader.jsonl", pairs_name="pairs.jsonl", reader_dir=None):
    tot = {"TP": 0, "FP": 0, "FN": 0, "TN": 0, "missing": 0}
    per = {}
    for d in dirs:
        scr = {}
        if smodel:
            for r in S.load(os.path.join(d, screen_name)):
                if r["model"] == smodel:
                    scr.setdefault(r["id"], set()).update(r["possible"])
        ans = S.reader_answers(os.environ.get("READER_FILE") or os.path.join(reader_dir or d, reader_name), rmodel, rversion)
        fl = (lambda p: True) if policy == "retrieve" else (lambda p: (S.POLICIES[policy](ans[p["key"]]) if p["key"] in ans else None))
        o, _ = S.score(d, os.path.join(d, pairs_name), fl, k, hts, scr if smodel else None)
        per[d] = o
        for x in ("TP", "FP", "FN", "TN"):
            tot[x] += o[x]
        tot["missing"] += o["answers_missing"]
    tot["precision"] = S.wilson(tot["TP"], tot["TP"] + tot["FP"])
    tot["recall"] = S.wilson(tot["TP"], tot["TP"] + tot["FN"])
    tot["specificity"] = S.wilson(tot["TN"], tot["TN"] + tot["FP"])
    return tot, per


if __name__ == "__main__":
    policy, k, smodel, rmodel, rversion = sys.argv[1:6]
    tot, per = run(policy, int(k), smodel if smodel != "-" else "", rmodel, rversion, sys.argv[6:])
    print(json.dumps(tot))
