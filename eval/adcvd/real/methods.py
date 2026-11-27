"""The methods tried on development data (D945), scored offline against the truth, no model call.

Usage:
    python3 -I -B eval/adcvd/real/methods.py --reader READER.jsonl [--resolve RESOLVE.jsonl] [--reader-model M --reader-version V] [--facts-reader FILE] [--out FILE] DIR [DIR ...]

Each DIR holds queries.jsonl, truth.jsonl, pairs.jsonl (from `go run ./cmd/adcvdreal`), method-screen.jsonl. Counts are pooled over the DIRs and
Wilson 95% intervals recomputed. A ruling is flagged when at least one order is; a flag is right when the ruling is a positive and the flagged
orders include one the ruling is known to be within (or none is known).
"""
import argparse
import importlib.util
import json
import os

HERE = os.path.dirname(os.path.abspath(__file__))
spec = importlib.util.spec_from_file_location("score", os.path.join(HERE, "score.py"))
S = importlib.util.module_from_spec(spec)
spec.loader.exec_module(S)


def answers(path, model, version, suffix=""):
    out = {}
    if not path or not os.path.exists(path):
        return out
    for r in S.load(path):
        if r["model"] == model and r["prompt_version"] == version:
            out[r["key"].removesuffix(suffix) if suffix else r["key"]] = r
    return out


def pooled(dirs, flagfn_for, k=12, screened=True, hts=False, smodel="gemini-4-argon"):
    tot = {"TP": 0, "FP": 0, "FN": 0, "TN": 0, "missing": 0}
    for d in dirs:
        scr = None
        if screened:
            scr = {}
            for r in S.load(os.path.join(d, "method-screen.jsonl")):
                if r["model"] == smodel:
                    scr.setdefault(r["id"], set()).update(r["possible"])
        o, _ = S.score(d, os.path.join(d, "pairs.jsonl"), flagfn_for(d), k, hts, scr)
        for x in ("TP", "FP", "FN", "TN"):
            tot[x] += o[x]
        tot["missing"] += o["answers_missing"]
    tot["precision"] = S.wilson(tot["TP"], tot["TP"] + tot["FP"])
    tot["recall"] = S.wilson(tot["TP"], tot["TP"] + tot["FN"])
    tot["specificity"] = S.wilson(tot["TN"], tot["TN"] + tot["FP"])
    return tot


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("dirs", nargs="+")
    ap.add_argument("--reader", required=True)
    ap.add_argument("--resolve")
    ap.add_argument("--reader-model", default="gemini-4-argon")
    ap.add_argument("--reader-version", default="adcvd-scope-v4")
    ap.add_argument("--facts-reader")
    ap.add_argument("--out")
    a = ap.parse_args()
    rd = answers(a.reader, a.reader_model, a.reader_version)
    res = {}
    if a.resolve:
        for r in S.load(a.resolve):
            res.setdefault(r["model"], {})[r["key"]] = r["p_within"]
    fr = answers(a.facts_reader, a.reader_model, a.reader_version, "#facts") if a.facts_reader else {}
    table = {}

    def reader_flag(name, f):
        table[name] = pooled(a.dirs, lambda d: f)

    table["plain HTS and country check (no model)"] = pooled(a.dirs, lambda d: (lambda p: True), k=0, screened=False, hts=True)
    table["retrieval top 12 only (no model)"] = pooled(a.dirs, lambda d: (lambda p: True), screened=False)
    table["name screen only"] = pooled(a.dirs, lambda d: (lambda p: True))
    pol = {"in": lambda v: v == "in", "in+lean_in": lambda v: v in ("in", "unclear_in"), "in+unclear": lambda v: v != "out"}

    def tag(r):
        return "unclear_" + r["lean"] if r["verdict"] == "unclear" and r.get("lean") else r["verdict"]

    for name, f in pol.items():
        reader_flag("screen + reader: " + name, (lambda f: lambda p: (f(tag(rd[p["key"]])) if p["key"] in rd else None))(f))
    for m in sorted(res):
        for t in (30, 40, 50, 60):
            def fl(p, m=m, t=t):
                r = rd.get(p["key"])
                if r is None:
                    return None
                if r["verdict"] != "unclear":
                    return r["verdict"] == "in"
                x = res[m].get(p["key"])
                return None if x is None else x >= t
            reader_flag("screen + reader; unclear resolved by %s p>=%d" % (m, t), fl)
    if len(res) > 1:
        for t in (30, 40, 50):
            def fl(p, t=t):
                r = rd.get(p["key"])
                if r is None:
                    return None
                if r["verdict"] != "unclear":
                    return r["verdict"] == "in"
                ps = [res[m].get(p["key"]) for m in res]
                ps = [x for x in ps if x is not None]
                return None if not ps else sum(ps) / len(ps) >= t
            reader_flag("screen + reader; unclear resolved by the mean of %d models p>=%d" % (len(res), t), fl)
    if fr:
        for name, f in pol.items():
            reader_flag("[information ceiling] reader given the ruling's facts: " + name, (lambda f: lambda p: (f(tag(fr[p["key"]])) if p["key"] in fr else None))(f))
    for name, t in table.items():
        print("%-86s P %5.1f%% (%d/%d)  R %5.1f%% (%d/%d)" % (name, 100 * (t["precision"]["p"] or 0), t["precision"]["k"], t["precision"]["n"], 100 * (t["recall"]["p"] or 0), t["recall"]["k"], t["recall"]["n"]))
    if a.out:
        json.dump(table, open(a.out, "w"), indent=1)


if __name__ == "__main__":
    main()
