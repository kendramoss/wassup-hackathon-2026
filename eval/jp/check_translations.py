"""Mechanical spot-check of the English translations, development rulings only (D711).

Usage: python3 -I -B eval/jp/check_translations.py <repo> [--check]

For every development ruling: every tariff number in the Japanese (NNNN.NN,
NN.NN, chapter numbers) appears in the English; every quoted rule code (CC, CTH,
CTSH, RVC) in the reasoning appears in the English reasoning; the country named
in the answer appears in the English answer. Writes eval/jp/translation-check.json.
Held-out rulings are not read.
"""
import json, os, re, sys, unicodedata

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from labels import stated_origin  # noqa: E402

N = lambda s: unicodedata.normalize("NFKC", s)


def nums(s):
    s = N(s)
    return set(re.findall(r"\d{4}\.\d{2}|\d{2}\.\d{2}", s)) | {str(int(c)) for c in re.findall(r"第\s*(\d{1,2})\s*類", s)}


def main():
    repo = sys.argv[1]
    rulings = {json.loads(l)["id"]: json.loads(l) for l in open(os.path.join(repo, "data/jp/rulings.jsonl"), encoding="utf-8")}
    tr = {json.loads(l)["id"]: json.loads(l) for l in open(os.path.join(repo, "data/jp/translations-en.jsonl"), encoding="utf-8")}
    dev = json.load(open(os.path.join(repo, "data/jp/splits.json")))["dev"]
    res = {"rulings": 0, "numbers_kept": 0, "codes_kept": 0, "origin_kept": 0, "origin_named": 0, "failures": []}
    for i in dev:
        r, e = rulings[i], tr[i]["en"]
        res["rulings"] += 1
        ja = " ".join([r["product_ja"], r["summary_ja"], r["reasoning_ja"]])
        en = N(" ".join([e["product"], e["inputs"], e["process"], e["reasoning"]]))
        en_nums = set(re.findall(r"\d{4}\.\d{2}|\d{2}\.\d{2}", en)) | {str(int(x)) for x in re.findall(r"\b\d{1,2}\b", en)}
        missing = sorted(n for n in nums(ja) if n not in en_nums)
        codes = set(re.findall(r"「(CTSH|CTH|CC|RVC)", N(r["reasoning_ja"])))
        lost = sorted(c for c in codes if not re.search(r"\b" + c + r"(?![A-Z])", N(e["reasoning"])))
        o = stated_origin(r["answer_ja"])
        o_ok = True
        if o and o != "EU":
            res["origin_named"] += 1
            squash = lambda x: re.sub(r"[^a-z]", "", x.lower())
            alias = {"Vietnam": "vietnam", "United States": "unitedstates|us", "South Korea": "korea"}.get(o, squash(o))
            o_ok = any(a in squash(N(e["answer"])) for a in alias.split("|"))
            res["origin_kept"] += o_ok
        res["numbers_kept"] += not missing
        res["codes_kept"] += not lost
        if missing or lost or not o_ok:
            res["failures"].append({"id": i, "numbers_missing": missing, "codes_lost": lost, "origin_kept": o_ok})
    out = json.dumps(res, indent=1, ensure_ascii=False) + "\n"
    p = os.path.join(repo, "eval/jp/translation-check.json")
    if "--check" in sys.argv:
        if open(p, encoding="utf-8").read() != out:
            sys.exit(p + " differs")
        print("jp translation check ok")
        return
    open(p, "w", encoding="utf-8").write(out)
    print({k: v for k, v in res.items() if k != "failures"}, "failures", len(res["failures"]))


if __name__ == "__main__":
    main()
