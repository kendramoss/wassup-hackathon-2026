"""Answers to the Japan question card, taken from each ruling (D921).

Usage: python3 -I -B eval/jp/answer_oracle.py <repo> [--check]

When the Japan engine cannot decide a case with no parts list, the product
asks one question (jpengine.Questions): which materials have no proof of
origin under the agreement, and their tariff codes. A requester knows that
from their suppliers' paperwork. Japan Customs records it in the first
sentences of each ruling's reasoning (認定理由), before the rule is applied:
"上記原材料のうち①については…原産材料と認められる。その他の原材料については
全て非原産材料として扱う。" This script reads only those sentences, and from
them only each numbered material's status: originating, wholly obtained,
neutral element or non-originating. It never reads the rule, the criterion,
the answer or anything after the first sentence that applies the rule. A
ruling whose status sentences cannot be read gets no answer ("Not sure"),
which adds no fact.

Writes eval/jp/answers-d921.jsonl: one line per ruling in the corpus and the
fresh set, and per dev_neg case, with the answer text in English (material
names from the published English translation).
"""
import json, os, re, sys

CIRCLED = "①②③④⑤⑥⑦⑧⑨⑩⑪⑫⑬⑭⑮⑯⑰⑱⑲⑳"
OUT = "eval/jp/answers-d921.jsonl"

NON, ORIG, WO, NEUTRAL = "non-originating", "originating", "wholly obtained", "neutral element"


def lead(reasoning):
    """The sentences before the first one that applies the rule."""
    out = []
    for s in re.split(r"(?<=。)", reasoning):
        if ("認められるためには" in s or "と認められるには" in s or s.startswith("非原産材料を使用して")
                or "生産される関税率表" in s or "得られ、又は生産される" in s):
            break
        out.append(s)
    return "".join(out)


def indices(spec):
    """'①から③まで及び⑤' -> [1, 2, 3, 5]."""
    out = []
    for m in re.finditer(r"([%s])(?:(?:から|～|~)([%s])(?:まで)?)?" % (CIRCLED, CIRCLED), spec):
        a = CIRCLED.index(m.group(1)) + 1
        b = CIRCLED.index(m.group(2)) + 1 if m.group(2) else a
        out.extend(range(a, b + 1))
    return out


def status_of(text):
    if "非原産" in text:
        return NON
    if "中立的な要素" in text:
        return NEUTRAL
    if "完全生産品" in text or "完全に得られ" in text:
        return WO
    if "原産材料" in text or "原産品" in text:
        return ORIG
    return None


IDX = r"[%s](?:(?:から|～|~|まで|、|及び|並びに|又は)*[%s])*(?:まで)?" % (CIRCLED, CIRCLED)


def parse(reasoning, n):
    """Status of materials 1..n, or None when the sentences cannot be read."""
    text = lead(reasoning)
    text = re.sub(r"[（(]以下、?「[^」]*」という。?[）)]", "", text)
    text = re.sub(r"照会[者書](?:から|より)提出された(?:情報|資料)により、?", "", text)
    text = text.replace("、その他", "。その他")
    if not text.strip():
        return None
    st = {}
    rest = None
    for sent in [s for s in re.split(r"(?<=。)", text) if s.strip()]:
        if sent.startswith("その他"):
            rest = status_of(sent)
            if rest is None:
                return None
            continue
        clauses = list(re.finditer(r"(%s)(?:について(?:は|、)|は)" % IDX, sent))
        head = sent[:clauses[0].start()] if clauses else sent
        if re.match(r"上記(?:原材料|原産材料)(?:は|については|、)", head) and "のうち" not in head:
            s = status_of(sent if not clauses else head)
            if s is None:
                return None
            for i in range(1, n + 1):
                st[i] = s
        elif not clauses:
            return None
        for k, m in enumerate(clauses):
            end = clauses[k + 1].start() if k + 1 < len(clauses) else len(sent)
            s = status_of(sent[m.end():end])
            if s is None:
                return None
            for i in indices(m.group(1)):
                if i > n:
                    return None
                st[i] = s
    if rest:
        for i in range(1, n + 1):
            st.setdefault(i, rest)
    if not st or any(i not in st for i in range(1, n + 1)):
        return None
    return st


def materials(inputs_en):
    """'① A (Chapter 1), ② B' -> {1: '① A (Chapter 1)', 2: '② B'}."""
    parts = re.split(r"(?=[%s])" % CIRCLED, inputs_en or "")
    out, last = {}, None
    for p in parts:
        if p and p[0] in CIRCLED and p[1:2] not in (":", "：", ")", "）") and CIRCLED.index(p[0]) + 1 not in out:
            last = CIRCLED.index(p[0]) + 1
            out[last] = p
        elif last is not None:
            out[last] += p  # a reference inside an item ("raw materials of ①: ...")
    return {k: v.strip().rstrip(",、").strip() for k, v in out.items()}


def answer(st, names, extra_non=None):
    groups = {NON: [], ORIG: [], WO: [], NEUTRAL: []}
    for i in sorted(st):
        groups[st[i]].append(names[i])
    if extra_non:
        groups[NON].append(extra_non)
    lines = []
    if groups[NON]:
        lines.append("These materials have no proof of origin: %s." % "; ".join(groups[NON]))
    else:
        lines.append("Every material has proof of origin.")
    if groups[ORIG]:
        lines.append("These materials have proof of origin under the agreement: %s." % "; ".join(groups[ORIG]))
    if groups[WO]:
        lines.append("These materials are wholly obtained, with proof of origin under the agreement: %s." % "; ".join(groups[WO]))
    if groups[NEUTRAL]:
        lines.append("These materials are neutral elements, so their origin need not be determined: %s." % "; ".join(groups[NEUTRAL]))
    return " ".join(lines)


def build(repo):
    def jl(p):
        return [json.loads(l) for l in open(os.path.join(repo, p), encoding="utf-8")]
    rulings = jl("data/jp/rulings.jsonl") + jl("data/jp/fresh-1126/rulings.jsonl")
    en = {r["id"]: r["en"] for r in jl("data/jp/translations-en.jsonl") + jl("data/jp/fresh-1126/translations-en.jsonl")}
    by_id = {r["id"]: r for r in rulings}
    out = []

    def one(rid, case_id, extra_non=None):
        r, e = by_id[rid], en.get(rid, {})
        names = materials(e.get("inputs", ""))
        if not names and e.get("inputs", "").strip():
            names = {1: e["inputs"].strip()}  # one unnumbered material
        n = max(names) if names else 0
        st = parse(r["reasoning_ja"], n) if n and sorted(names) == list(range(1, n + 1)) else None
        row = {"id": case_id, "agreement": r["agreement"], "answer": ""}
        if st:
            row["status"] = {str(i): st[i] for i in sorted(st)}
            row["answer"] = answer(st, names, extra_non)
        out.append(row)

    for r in rulings:
        one(r["id"], r["id"])
    for n in jl("eval/jp/dev-negatives.jsonl"):
        m = re.search(r"semi-finished article of HS (\S+), imported from Brazil", n["facts"])
        one(n["source"], n["id"], "the semi-finished article of HS %s imported from Brazil" % m.group(1).rstrip(","))
    return out


def main():
    if len(sys.argv) < 2 or len(sys.argv) > 3 or (len(sys.argv) == 3 and sys.argv[2] != "--check"):
        sys.exit(__doc__.split("\n\n")[1])
    repo = sys.argv[1]
    rows = build(repo)
    text = "".join(json.dumps(r, ensure_ascii=False, sort_keys=True) + "\n" for r in rows)
    path = os.path.join(repo, OUT)
    if len(sys.argv) == 3:
        if open(path, encoding="utf-8").read() != text:
            sys.exit("answer_oracle: %s is out of date" % OUT)
        print("answer oracle ok", len(rows))
        return
    open(path, "w", encoding="utf-8").write(text)
    print("answers", len(rows), "with an answer", sum(1 for r in rows if r["answer"]))


if __name__ == "__main__":
    main()
