"""Scores the scope-first method against truth3, offline (D959): the figures of the development set and of the held-out set.

Usage: python3 -I -B eval/adcvd/real/run3.py SPLIT_DIR --screen method-screen.jsonl --reader reader.jsonl --truth truth3-hs.jsonl [--truth truth3-hs-all.jsonl ...] [--out summary3.json]

The method is D959's: a pair is asked when it is in the retrieval's top 12 and the method's name screen (gemini-4-argon) listed it; the reader
(gemini-4-argon, adcvd-scope-v4) flags it on `in` or `unclear` with a lean of `in`. Beside it: the plain HTS-and-country check, the rulings the
method could not answer for (a missing reader answer is not a flag), and the share of rulings flagged at all. No model is called.
"""
import argparse
import json
import os
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import score3 as T  # noqa: E402

S = T.S


def method_flags(d, screen, reader):
    scr = {}
    for r in S.load(os.path.join(d, screen)):
        if r["model"] == "gemini-4-argon":
            scr.setdefault(r["id"], set()).update(r["possible"])
    asked, rd, missing = set(), {}, 0
    for r in S.load(os.path.join(d, reader)):
        if r["model"] == "gemini-4-argon" and r["prompt_version"] == "adcvd-scope-v4":
            rd[r["key"]] = r
    flags = {}
    for p in S.load(os.path.join(d, "pairs.jsonl")):
        if 1 <= p["rank"] <= 12 and p["key"] in scr.get(p["id"], ()) and p["key"] not in asked:
            asked.add(p["key"])
            r = rd.get(p["key"])
            if r is None:
                missing += 1
                continue
            flags[p["key"]] = r["verdict"] == "in" or (r["verdict"] == "unclear" and r.get("lean") == "in")
    return flags, len(asked), missing


def plain_flags(d):
    return {p["key"]: True for p in S.load(os.path.join(d, "pairs.jsonl")) if p["hts_match"]}


def every_flags(d):
    return {p["key"]: True for p in S.load(os.path.join(d, "pairs.jsonl")) if 1 <= p["rank"] <= 12}


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("split")
    ap.add_argument("--screen", default="method-screen.jsonl")
    ap.add_argument("--reader", default="reader.jsonl")
    ap.add_argument("--truth", action="append", required=True)
    ap.add_argument("--out")
    a = ap.parse_args()
    pairs = os.path.join(a.split, "pairs.jsonl")
    mf, asked, missing = method_flags(a.split, a.screen, a.reader)
    out = {"pairs_asked": asked, "reader_answers_missing": missing, "truths": {}}
    for tf in a.truth:
        t = {s["id"]: s for s in S.load(os.path.join(a.split, tf))}
        n = {k: sum(1 for x in t.values() if x["status"] == k) for k in ("positive", "negative", "contested")}
        o = {"rulings": len(t), **n, "method": T.score3(a.split, pairs, mf, tf), "plain_hts_check": T.score3(a.split, pairs, plain_flags(a.split), tf), "flag_every_ruling_top12": T.score3(a.split, pairs, every_flags(a.split), tf)}
        out["truths"][tf] = o
        print(tf, {k: o[k] for k in ("rulings", "positive", "negative", "contested")})
        for k in ("method", "plain_hts_check", "flag_every_ruling_top12"):
            print("  " + T.fmt(k, o[k]))
    print("pairs asked", asked, "reader answers missing", missing)
    if a.out:
        json.dump(out, open(a.out, "w"), indent=1)


if __name__ == "__main__":
    main()
