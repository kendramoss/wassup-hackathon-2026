"""Build the china-v1 unseen check (D860): training and tune_check rulings whose
facts name China and another country (engine.ChinaCase). No model calls, no
labels or results read.

Usage: python3 -I eval/accuracy/build_chinacheck.py <repo> --frozen-at <time>
Writes eval/splits/splits_chinacheck.json (key "train"). A Go test checks the
list against engine.ChinaCase.
"""
import argparse, json, os, re


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("repo")
    ap.add_argument("--frozen-at", required=True)
    a = ap.parse_args()
    R = lambda *p: os.path.join(a.repo, *p)
    src = open(R("backend", "internal", "domain", "countries.go")).read()
    i = src.find("countryNames = []string{")
    names = re.findall(r'"([^"]+)"', src[i:src.find("}", i)])
    other = sorted((n for n in names if n not in ("China", "United States")), key=len, reverse=True)
    cn = re.compile(r"\b(?:China|Chinese)\b")
    ot = re.compile(r"\b(?:" + "|".join(re.escape(n) for n in other) + r")\b")
    tune = json.load(open(R("eval", "splits", "splits_tune.json")))
    ids = []
    for r in tune["train"] + tune["tune_check"]:
        p = R("backend", "data", "facts", r + ".txt")
        t = open(p).read() if os.path.exists(p) else ""
        if cn.search(t) and ot.search(t):
            ids.append(r)
    out = {"frozen_at": a.frozen_at,
           "rule": "training and tune_check rulings (splits_tune.json) whose facts name China and another country of the picker table other than the United States (engine.ChinaCase); the china-v1 writer read none of them (D860)",
           "train": sorted(ids)}
    with open(R("eval", "splits", "splits_chinacheck.json"), "w") as f:
        json.dump(out, f, indent=2)
        f.write("\n")
    print(len(ids))


if __name__ == "__main__":
    main()
