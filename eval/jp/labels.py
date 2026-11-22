"""Labels for the Japan Customs origin rulings, from the published Japanese fields (D710).

Usage: python3 -I -B eval/jp/labels.py <repo> [--check]
       python3 -I -B eval/jp/labels.py <repo> --held-out --confirm-phase2   (phase 2 only)

Writes data/jp/labels-dev.jsonl for the development split. Held-out labels are
written only with --held-out --confirm-phase2, and nothing about them is printed.

criterion: the rule the reasoning (認定理由) says the good meets.
  WO   wholly obtained / produced (完全に得られ…, 完全生産品)
  PE   produced exclusively from originating materials (原産材料のみから生産)
  CC   change of chapter      CTH  change of heading      CTSH change of subheading
  RVC  value content (RVC, VA, QVC, MaxNOM, 付加価値, 原産割合)
  SP   a specific process (knitting, cutting and sewing, a chemical reaction, blending, ...):
       any quoted product-specific rule that is not a change of classification, value test or WO
The rule text quoted before を満たす / が行われている is classified first; the
whole reasoning is the fallback. A ruling may meet several (an alternative
rule, or different rules for different inputs); `criteria` keeps them all and
`criterion` is the first quoted one.
stated_origin: the country the answer (回答) names, if it names one.
"""
import json, os, re, sys, unicodedata

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from parse import COUNTRIES, _CTRY_RE  # noqa: E402

N = lambda s: unicodedata.normalize("NFKC", s)

# Classifiers for a quoted rule text, NFKC-normalised. First match wins inside one quote.
QUOTE_RULES = [
    ("RVC", r"RVC|VA\s*\d|QVC|MaxNOM|価額|付加価値|原産割合|域内原産|域内価値"),
    ("CTSH", r"CTSH|号の変更|他の号の材料|異なる[^」]{0,30}号"),
    ("CTH", r"CTH|項の変更|他の項の材料|異なる[^」]{0,40}項"),
    ("CC", r"\bCC\b|^CC|類の変更|他の類の材料|異なる[^」]{0,30}類"),
    ("WO", r"完全に得られ|完全に生産"),
    ("SP", r"編み|編物|裁断|縫製|縫い合わせ|製品にすること|化学反応|精製|混合|工程|紡績|製織|なせん|浸染|生産"),
]
WO = r"完全に得られ|完全生産品|完全に生産され|完全に取得"
PE = r"原産材料のみから(?:完全に)?生産|原産材料のみを(?:使用|用いて)"
SATISFIED = re.compile(r"「([^「」]{1,400})」(?:[（(][^）)]{0,60}[）)])?(?:の規定)?\s*(?:を満たす|を満たし|が行われ|に該当する加工)")
ORDER = ["WO", "PE", "CC", "CTH", "CTSH", "RVC", "SP"]


CTC_UNIT = re.compile(r"(類|項|号)(?:[(（][^)）]{0,40}[)）])?(?:の材料|に属する物品|に該当する物品以外の物品|に該当する材料以外の材料)")
UNIT = {"類": "CC", "項": "CTH", "号": "CTSH"}


def classify(q):
    """Classify one quoted rule. A rule that opens with a code (「CTH。ただし…」) is that code;
    conditions after it (weight limits, excluded headings) do not change it."""
    q = N(q)
    m = re.match(r"\s*(CTSH|CTH|CC)\b", q)
    if m:
        return m.group(1)
    if re.search(QUOTE_RULES[0][1], q):
        return "RVC"
    units = CTC_UNIT.findall(q)
    if units and re.search(r"からの(?:変更|製造|生産)|以外", q):
        return UNIT[units[-1]] if not re.search(r"への他の(類|項|号)の材料", q) else UNIT[re.search(r"への他の(類|項|号)の材料", q).group(1)]
    m = re.search(r"\b(CTSH|CTH|CC)\b", q)
    if m:
        return m.group(1)
    for code, pat in QUOTE_RULES[1:]:
        if re.search(pat, q):
            return code
    # Any other product-specific rule text is a process rule ("配合", "人造繊維の押出し").
    return "SP"


def criterion(reasoning):
    r = N(reasoning)
    found = []
    for m in SATISFIED.finditer(r):
        c = classify(m.group(1))
        if c and c not in found:
            found.append(c)
    if not found:  # a rule quoted after 品目別(原産地)規則, met through de minimis or for some inputs
        for m in re.finditer(r"品目別(?:原産地)?規則(?:のうち)?「([^「」]{1,400})」", r):
            c = classify(m.group(1))
            if c and c not in found:
                found.append(c)
    if not found:  # GSP general rule, quoted with a nested 「原産品」: "...の項と異なることとなる加工又は製造"
        m = re.search(r"(類|項|号)と異なることとなる加工|すなわち、(?:類|項|号)の変更", r)
        m = m and re.search(r"(類|項|号)(?:と異なることとなる加工|の変更)", m.group(0))
        if m:
            found.append(UNIT[m.group(1)])
    pe = bool(re.search(PE, r))
    if re.search(WO, re.sub(PE, "", r)) and not found and not pe:  # 原産材料のみから完全に生産 is PE, not WO
        found.append("WO")
    if pe and "PE" not in found:
        found.append("PE")
    return found


def stated_origin(answer):
    a = N(answer)
    for pat in (r"RCEP原産国は(.+?)[)）]", r"原産地は(.+?)と認め", r"上の(.+?)原産品"):
        m = re.search(pat, a)
        if m:
            c = _CTRY_RE.findall(m.group(1))
            if c:
                return COUNTRIES[c[-1]]
            if "EU" in m.group(1):
                return "EU"
    return None


def outcome(answer):
    a = N(answer)
    if re.search(r"認められない|該当しない", a):
        return "negative"
    if re.search(r"認められる|と認める", a):
        return "positive"
    return "unclear"


def label(r):
    crit = criterion(r["reasoning_ja"])
    return {"id": r["id"], "agreement": r["agreement"], "outcome": outcome(r["answer_ja"]),
            "stated_origin": stated_origin(r["answer_ja"]), "criteria": crit, "criterion": crit[0] if crit else None}


def main():
    repo = sys.argv[1]
    rulings = {json.loads(l)["id"]: json.loads(l) for l in open(os.path.join(repo, "data/jp/rulings.jsonl"), encoding="utf-8")}
    splits = json.load(open(os.path.join(repo, "data/jp/splits.json")))
    held = "--held-out" in sys.argv
    if held and "--confirm-phase2" not in sys.argv:
        sys.exit("held-out labels are written only in phase 2 (--confirm-phase2)")
    ids = splits["held_out"] if held else splits["dev"]
    out = "".join(json.dumps(label(rulings[i]), ensure_ascii=False, sort_keys=True) + "\n" for i in ids)
    p = os.path.join(repo, "data/jp", "labels-heldout.jsonl" if held else "labels-dev.jsonl")
    if "--check" in sys.argv:
        if open(p, encoding="utf-8").read() != out:
            sys.exit(p + " differs from labels.py")
        print("jp labels ok")
        return
    open(p, "w", encoding="utf-8").write(out)
    if held:
        print("held-out labels written:", len(ids))
        return
    from collections import Counter
    rows = [json.loads(l) for l in out.splitlines()]
    print("dev", len(rows), "criterion", Counter(r["criterion"] for r in rows).most_common(),
          "outcome", Counter(r["outcome"] for r in rows).most_common(),
          "stated_origin named", sum(1 for r in rows if r["stated_origin"]))


if __name__ == "__main__":
    main()
