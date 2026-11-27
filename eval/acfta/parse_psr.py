"""Parse the ASEAN-China (ACFTA) Attachment B product-specific rules (HS 2017).

Usage:
    python3 -B eval/acfta/parse_psr.py build <primary.pdf> <cross-check.pdf> <out.json>   (needs pdfplumber)
    python3 -I -B eval/acfta/parse_psr.py check <out.json>                       (standard library)

Attachment B, as amended by the ACFTA Upgrading Protocol (Annex 1 Rules of
Origin, effective 15 August 2019), is written in HS 2017. "build" reads two
independent copies of it (Enterprise Singapore, the primary, and Cambodia's
Ministry of Commerce). Every row the two both give must match (a rule that
runs on across a page and is cut short in one copy counts as a match), and
every code in the primary must appear in the cross-check's text. It keeps
every row verbatim and adds the rule read from the text. "check" re-reads every rule from the stored
text and fails if it differs from the stored one. The PDFs are not
committed; the source block records each URL and SHA-256.
"""
import hashlib
import json
import re
import sys

VERSION = "acfta-attachment-b-psr-hs2017-upgrading-protocol"
SOURCES = [
    {"name": "Enterprise Singapore (primary)", "url": "https://www.enterprisesg.gov.sg/-/media/esg/files/non-financial-assistance/for-companies/free-trade-agreements/ASEAN-China-FTA/upgrade-protocol/Attachment-B-PSR.pdf"},
    {"name": "Cambodia, Ministry of Commerce (cross-check)", "url": "https://co.moc.gov.kh/Content/resources/FormE_PSR.pdf"},
]

PROCESS = {
    "process_rule_1": "Process Rule 1: manufacturing through fibre-making (polymerisation, polycondensation and extrusion), spinning, twisting, texturising or braiding from a blend or any of silk, wool, fine or coarse animal hair, cotton fibres, vegetable textile fibres, synthetic or artificial filaments or staple fibres",
    "process_rule_2": "Process Rule 2: manufacturing from polymer or fibres (non-woven), yarns (fabrics) or raw or unbleached fabrics (finished fabrics) through needle punching, spin bonding or chemical bonding, weaving or knitting, crocheting, wadding or tufting, or dyeing or printing and finishing, impregnation, coating, covering or lamination",
    "process_rule_3": "Process Rule 3: manufacturing through cutting and assembly of parts into a complete article (apparel and tents), incorporating embroidery, embellishment or printing (made-up articles), from raw or unbleached fabric or finished fabric",
    "process_rule": "a Process Rule of Attachment B (the table does not say which of Process Rules 1 to 3)",
    "animals_raised": "the goods are obtained from sheep, lambs or other animals raised in either Party",
    "fats_wo": "the goods are manufactured from fats or oil wholly obtained in either Party",
}


def clean(t):
    t = t.replace("’", "'")
    t = re.sub(r"\s+", " ", t).strip()
    t = re.sub(r"\bexccept\b", "except", t)
    t = re.sub(r"Chapter 7and Chapter 10", "Chapters 7 and 10", t)
    t = re.sub(r"Sub- heading", "Subheading", t)
    t = re.sub(r"(\d)- (\d)", r"\1-\2", t)
    t = re.sub(r"RVC ?(\d+)", r"RVC \1", t)
    t = re.sub(r",\s*except", " except", t)
    t = re.sub(r"\)except", ") except", t)
    t = t.replace("CC,except", "CC except").replace("or CC except", "or CC except")
    return t


def exc_ranges(s):
    s = s.strip().rstrip(".").strip()
    s = re.sub(r"^(?:from\s+)?", "", s)
    out = []
    chapter = False
    for p in re.split(r",\s*|\s+or\s+|\s+and\s+", s):
        p = p.strip()
        if not p:
            continue
        m = re.match(r"(Chapters?|[Hh]eadings?|Subheadings?)\s+(.*)", p)
        if m:
            chapter = m.group(1).lower().startswith("chapter")
            p = m.group(2).strip()
        m = re.fullmatch(r"(\d[\d.]*)-(\d[\d.]*)", p)
        a, b = (m.group(1), m.group(2)) if m else (p, p)
        a, b = re.sub(r"\D", "", a), re.sub(r"\D", "", b)
        if chapter:
            a, b = a.zfill(2), b.zfill(2)
        if not a or not b or len(a) != len(b) or len(a) not in (2, 4, 6):
            return None
        out.append({"level": {2: "chapter", 4: "heading", 6: "subheading"}[len(a)], "from": a, "to": b})
    return out or None


def parse_rule(text):
    """Returns {"alternatives", "variants", "note"} or None. The first two are
    lists, the note is a string; empty ones are left out of the result."""
    t = clean(text)
    m = re.fullmatch(r"(RVC 40 or CTH),? and for waste and scrap: WO", t)
    if m:
        base = parse_rule(m.group(1))
        return {"variants": [
            {"id": "goods", "label": "goods other than waste and scrap", "alternatives": base["alternatives"]},
            {"id": "waste_scrap", "label": "waste and scrap", "alternatives": [alt("WO")]},
        ]}
    m = re.fullmatch(r"CC or RVC 40\. (For goods obtained by simple mixing , the individual inputs shall retain their origin prior to such mixing\.)", t)
    if m:
        return {"alternatives": [alt("CC"), alt("RVC 40")], "note": m.group(1)}
    if t == "Obtained from sheep, lambs or other animals raised in either Party":
        return {"alternatives": [{"name": "animals raised in either Party", "requires": [{"kind": "process", "process": "animals_raised"}]}]}
    if t == "Manufactured from fats or oil wholly obtained in either Party":
        return {"alternatives": [{"name": "fats or oil wholly obtained in either Party", "requires": [{"kind": "process", "process": "fats_wo"}]}]}
    exc = None
    packaging = False
    m = re.search(r" except when change to this heading is only through packaging or presenting goods for (?:retail )?sale$", t)
    if m:
        packaging = True
        t = t[: m.start()]
    else:
        m = re.search(r" except (?!when)(.+)$", t)
        if m:
            exc = exc_ranges(m.group(1))
            if exc is None:
                return None
            t = t[: m.start()]
    t = t.replace("9619.00* ", "")
    toks = [x.strip() for x in re.split(r"\s+or\s+|,\s*", t) if x.strip()]
    alts = []
    for tok in toks:
        a = alt(tok)
        if a is None:
            return None
        alts.append(a)
    if not alts:
        return None
    if exc or packaging:
        ctcs = [a for a in alts if a["requires"][0]["kind"] == "ctc"]
        if len(ctcs) != 1:
            return None
        if exc:
            ctcs[0]["requires"][0]["except"] = exc
        if packaging:
            ctcs[0]["requires"][0]["not_only_packaging"] = True
    return {"alternatives": alts}


def alt(tok):
    if tok == "WO":
        return {"name": "WO", "requires": [{"kind": "wo"}]}
    m = re.fullmatch(r"RVC (\d+)", tok)
    if m:
        return {"name": "RVC%s" % m.group(1), "requires": [{"kind": "rvc", "threshold": int(m.group(1))}]}
    if tok in ("CC", "CTH", "CTSH"):
        return {"name": tok, "requires": [{"kind": "ctc", "level": {"CC": "chapter", "CTH": "heading", "CTSH": "subheading"}[tok]}]}
    m = re.fullmatch(r"Process Rule(?: (\d))?", tok)
    if m:
        key = "process_rule_" + m.group(1) if m.group(1) else "process_rule"
        return {"name": "Process Rule" + (" " + m.group(1) if m.group(1) else ""), "requires": [{"kind": "process", "process": key}]}
    return None


def concise(p):
    def names(al):
        return " or ".join(a["name"] for a in al)
    if p.get("variants"):
        return "; ".join("%s: %s" % (v["id"], names(v["alternatives"])) for v in p["variants"])
    return names(p["alternatives"])


def read_rows(pdf_path):
    import pdfplumber

    rows = []
    with pdfplumber.open(pdf_path) as p:
        for pi, pg in enumerate(p.pages[2:], 3):
            for t in pg.extract_tables():
                for r in t:
                    r = [(c or "").strip() for c in r]
                    if len(r) != 4 or r[3] == "Product\nSpecific Rules" or r[1].startswith("Tariff"):
                        continue
                    if r[1] and r[3]:
                        rows.append((r[1].replace("*", ""), clean(r[2]), clean(r[3])))
                    elif r[3] and not r[0].startswith("CHAPTER") and rows:
                        # a rule that runs on from the previous page
                        c, ds, ru = rows[-1]
                        rows[-1] = (c, (ds + " " + clean(r[2])).strip(), clean(ru + " " + r[3]))
                    elif r[1] or r[3]:
                        raise SystemExit("page %d: row with a code and no rule, or a rule and no code: %r" % (pi, r))
    return rows


def page_text(pdf_path):
    import pdfplumber

    with pdfplumber.open(pdf_path) as p:
        return "\n".join((pg.extract_text() or "") for pg in p.pages)


def sha(path):
    return hashlib.sha256(open(path, "rb").read()).hexdigest()


def code6(code):
    """The source prints a few codes with five digits ("5204.2"); each is a
    subheading ending in 0 (5204.20)."""
    d = re.sub(r"\D", "", code)
    return d + "0" if len(d) == 5 else d


def build(pdf1, pdf2, out):
    a, b = read_rows(pdf1), read_rows(pdf2)
    db = {code6(x[0]): x[2] for x in b}
    other_text = page_text(pdf2)
    flat = re.sub(r"\s+", "", other_text)
    mismatch, absent = [], []
    for c, _, rule in a:
        k = code6(c)
        if k in db:
            r2 = db[k]
            if not (rule == r2 or rule.startswith(r2) or r2.startswith(rule)):
                mismatch.append(k)
        elif c.replace("*", "") not in other_text and re.sub(r"\s+", "", c) not in flat:
            absent.append(k)
    if mismatch or absent:
        sys.exit("cross-check failed: %d rows differ %s; %d codes missing from the other copy %s" % (len(mismatch), mismatch[:10], len(absent), absent[:10]))
    seen = set()
    rules = []
    for i, (c, desc, rule) in enumerate(a, 1):
        d = code6(c)
        if len(d) != 6 or d in seen:
            sys.exit("bad or repeated code %r" % c)
        seen.add(d)
        r = {"row": i, "code": d[:4] + "." + d[4:], "description": desc, "full": rule}
        if len(re.sub(r"\D", "", c)) == 5:
            r["source_code"] = c
        parsed = parse_rule(rule)
        if parsed is None:
            r["text_only"] = True
            r["concise"] = "text rule"
        else:
            for k in ("alternatives", "variants", "note"):
                if parsed.get(k):
                    r[k] = parsed[k]
            r["concise"] = concise(parsed)
        rules.append(r)
    srcs = [dict(s, sha256=sha(p)) for s, p in zip(SOURCES, (pdf1, pdf2))]
    doc = {"version": VERSION, "source": {"copies": srcs, "hs": "HS 2017", "rows": len(rules), "cross_checked_rows": len(set(code6(c) for c, _, _ in a) & set(db))},
           "rules": rules}
    with open(out, "w") as f:
        json.dump(doc, f, ensure_ascii=False, indent=1)
        f.write("\n")
    to = [r["row"] for r in rules if r.get("text_only")]
    print("%d rows (%d in both copies' tables), %d text-only %s" % (len(rules), doc["source"]["cross_checked_rows"], len(to), to))


def check(path):
    doc = json.load(open(path))
    bad = 0
    for r in doc["rules"]:
        parsed = parse_rule(r["full"])
        if parsed is None:
            if not r.get("text_only"):
                print("row %d: stored rule does not re-read" % r["row"])
                bad += 1
            continue
        got = {k: r.get(k) for k in ("alternatives", "variants", "note") if r.get(k)}
        want = {k: parsed[k] for k in ("alternatives", "variants", "note") if parsed.get(k)}
        if r.get("text_only") or got != want:
            print("row %d: stored rule differs from the text" % r["row"])
            bad += 1
    print("%d rows checked, %d differences" % (len(doc["rules"]), bad))
    sys.exit(1 if bad else 0)


if __name__ == "__main__":
    if len(sys.argv) == 5 and sys.argv[1] == "build":
        build(sys.argv[2], sys.argv[3], sys.argv[4])
    elif len(sys.argv) == 3 and sys.argv[1] == "check":
        check(sys.argv[2])
    else:
        sys.exit(__doc__)
