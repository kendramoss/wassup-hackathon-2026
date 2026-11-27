"""Parse the ASEAN-Korea (AKFTA) Appendix 2 product-specific rules (HS 2022).

Usage:
    python3 -B eval/akfta/parse_psr.py build <psr.pdf> <out.json>   (needs pdfplumber)
    python3 -I -B eval/akfta/parse_psr.py check <out.json>          (standard library)

"build" reads the AKFTA Implementing Committee decision that endorses the
transposed Product Specific Rules in HS 2022 (Appendix 2 of Annex 3, in
force 1 May 2026), keeps every row verbatim and adds the rule read from the
text. "check" re-reads every rule from the stored text and fails if it
differs from the stored one, so the table can be verified without the PDF.
The PDF is not committed; the source block records its URL and SHA-256.
"""
import hashlib
import json
import re
import sys

URL = "https://ftadjbc.info/download/akfta-product-specific-rules-in-hs-2022-2/"
TITLE = "AKFTA PSR in HS2022 (FINAL), endorsed by the AKFTA Implementing Committee, to be implemented by 1 May 2026"
VERSION = "akfta-appendix2-psr-hs2022-akic"

WO_EXPORTING = "Wholly-Obtained or Produced in the territory of the exporting Party"
WO_ANY = "Wholly-Obtained or Produced in the territory of any Party"

PROCESS = {
    "cut_and_sewn": "the good is both cut and sewn in the territory of a Party",
    "printing_dyeing": "printing or dyeing is accompanied by at least two preparatory or finishing operations",
    "milk_limit": "the good does not contain milk (or material from milk) imported from a non-Party above the stated share by weight of the total raw milk material",
    "glazed": "the good is glazed from goods that are not glazed",
    "mco": "the good is a multi-component integrated circuit made from goods that are not multi-component integrated circuits",
}


def clean(t):
    t = t.replace("’", "'")
    t = re.sub(r"\s+", " ", t).strip()
    # letter-spaced words such as "C h a n g e t o H e a d i ng"
    def squash(m):
        return m.group(0).replace(" ", "")
    t = re.sub(r"(?:\b\w ){5,}\w+", squash, t)
    t = t.replace("Wholly- Obtained", "Wholly-Obtained").replace("Wholly- Obtained", "Wholly-Obtained")
    t = re.sub(r"[Ss]ub-\s?heading", lambda m: "Subheading" if m.group(0)[0] == "S" else "subheading", t)
    t = re.sub(r"ChangetoHeading", "Change to Heading ", t)
    t = re.sub(r"ChangetoSubheading", "Change to Subheading ", t)
    t = re.sub(r"Change toHeading", "Change to Heading ", t)
    t = re.sub(r"\s+", " ", t).strip()
    t = re.sub(r"Wholly-\s*O\s*btained", "Wholly-Obtained", t)
    t = re.sub(r"W\s?h\s?o\s?l\s?l\s?y\s?-\s?O\s?b\s?t\s?a\s?i\s?n\s?e\s?d", "Wholly-Obtained", t)
    return t


def dotted(d):
    if len(d) == 4:
        return d[:2] + "." + d[2:]
    if len(d) == 6:
        return d[:4] + "." + d[4:]
    return d


def level_of(word):
    w = word.lower()
    if w.startswith("chapter"):
        return "chapter"
    if w.startswith("heading"):
        return "heading"
    return "subheading"


def code_digits(s):
    return re.sub(r"\D", "", s)


def ranges(text, level):
    """'7208 and 7209', '72.08 through 72.10', '0802.31, 0802.32 and 0802.99'."""
    text = text.replace("Sub-headings", "Subheadings")
    out = []
    parts = re.split(r",\s*(?:and\s+)?|\s+and\s+", text)
    for p in parts:
        p = p.strip()
        if not p:
            continue
        m = re.fullmatch(r"([\d.]+)\s+through\s+([\d.]+)", p)
        if m:
            out.append({"level": level, "from": code_digits(m.group(1)), "to": code_digits(m.group(2))})
        else:
            d = code_digits(p)
            if not d:
                return None
            out.append({"level": level, "from": d, "to": d})
    return out


SCOPE_ITEM = r"(?:Chapters?|Headings?|Subheadings?)"


def scope(text):
    """'Chapters 1, 2 and 5', 'Headings 10.03 and 10.06', 'Headings 50.07, 51.11 through 51.13, and Chapters 52 and 60'."""
    text = text.strip()
    segs = re.split(r"(?=\b(?:Chapters?|Headings?|Subheadings?)\b)", text, flags=re.I)
    out = []
    for s in segs:
        s = s.strip().strip(",").strip()
        if not s:
            continue
        s = re.sub(r"\s+and$", "", s).strip(",").strip()
        m = re.match(r"(Chapters?|Headings?|Subheadings?)\s+(.*)", s, flags=re.I)
        if not m:
            return None
        r = ranges(m.group(2), level_of(m.group(1)))
        if r is None:
            return None
        out.extend(r)
    return out or None


def parse_cond(c):
    """One 'provided that' condition. Returns a list of requirements, or None."""
    c = c.strip().rstrip(".").strip()
    m = re.fullmatch(r"(?:the )?materials?(?: of| from)? (.+?) (?:is|are) (Wholly-Obtained or Produced|originating) in the territory of (any Party|the exporting Party)", c)
    if m:
        sc = scope(m.group(1))
        if sc is None:
            return None
        origin = ("wo_" if m.group(2).startswith("Wholly") else "orig_") + ("any" if m.group(3) == "any Party" else "exporting")
        return [{"kind": "materials", "scope": sc, "origin": origin}]
    m = re.fullmatch(r"(?:the )?fabrics of (.+?) are originating(?: in the territory of (any Party))? and the good is both cut and sewn in the territory of (any Party|the exporting Party)", c)
    if m:
        sc = scope(m.group(1))
        if sc is None:
            return None
        o = "orig_exporting" if (m.group(2) is None and m.group(3) == "the exporting Party") else "orig_any"
        return [{"kind": "materials", "scope": sc, "origin": o}, {"kind": "process", "process": "cut_and_sewn"}]
    if re.fullmatch(r"the good is both cut and sewn in the territory of (?:any Party|the exporting Party)", c):
        return [{"kind": "process", "process": "cut_and_sewn"}]
    m = re.fullmatch(r"it has a regional value content of not less than (\d+) percent of the FOB value of the good", c)
    if m:
        return [{"kind": "rvc", "threshold": int(m.group(1))}]
    m = re.fullmatch(r"the value of non-originating materials of (Heading|Headings) ([\d.]+) does not exceed (\d+)% of the FOB value of the good", c)
    if m:
        return [{"kind": "value_cap", "scope": scope(m.group(1) + " " + m.group(2)), "percent": int(m.group(3))}]
    m = re.fullmatch(r"products of [\d.]+ do not contain materials of / from milk imported from a non-Party over (\d+)% by weight of the total raw material of / from milk", c)
    if m:
        return [{"kind": "process", "process": "milk_limit", "percent": int(m.group(1))}]
    return None


def parse_provisos(t):
    """Split ', provided that X and Y' into conditions."""
    whole = parse_cond(t)
    if whole is not None:
        return whole
    reqs = []
    for chunk in re.split(r"\s+and\s+(?=(?:the good is |it has ))", t):
        r = parse_cond(chunk)
        if r is None:
            return None
        reqs.extend(r)
    return reqs


def parse_alt(a):
    """One alternative (text between '; or '). Returns (name, requires) or None."""
    a = a.strip().rstrip(".").strip()
    if a == WO_EXPORTING:
        return "WO", [{"kind": "wo"}]
    if a == WO_ANY:
        return "WO-AK", [{"kind": "wo_any"}]
    if a == "Printing or dyeing accompanied by at least two preparatory or finishing operations":
        return "printing or dyeing", [{"kind": "process", "process": "printing_dyeing"}]
    m = re.fullmatch(r"A regional value content of not less than (\d+) percent of the FOB value of the good(?:, provided that (.+))?", a)
    if m:
        reqs = [{"kind": "rvc", "threshold": int(m.group(1))}]
        name = "RVC%s" % m.group(1)
        if m.group(2):
            pr = parse_provisos(m.group(2))
            if pr is None:
                return None
            reqs += pr
            name += " with conditions"
        return name, reqs
    m = re.fullmatch(r"Change to glazed goods of Heading ([\d.]+) from goods that are not glazed of Heading ([\d.]+)", a)
    if m:
        return "glazed goods", [{"kind": "process", "process": "glazed"}]
    m = re.fullmatch(r"Change to multi-component integrated circuits \(MCOs\) from goods that are not multi-component integrated circuits \(MCOs\) of Heading ([\d.]+)", a)
    if m:
        return "MCO", [{"kind": "process", "process": "mco"}]
    m = re.fullmatch(r"Change to parts, provided that a regional value content of not less than (\d+) percent of the FOB value of the good", a)
    if m:
        return "RVC%s (parts)" % m.group(1), [{"kind": "rvc", "threshold": int(m.group(1))}]
    m = re.fullmatch(
        r"Change to (?P<what>.+?) from (?P<src>any other Chapter|any other Heading|any other Subheading|within this Subheading or any other Subheading"
        r"|transmission apparatus of Subheading [\d.]+ or any other Subheading)"
        r"(?:, except from (?:(?P<exk>Headings?|Subheadings?) (?P<ex>.+?)|(?P<expart>triethanolamine's salt of Subheading [\d.]+)))?"
        r"(?:, (?:in conditions that (?P<nodm>the de minimis rule shall not be applied to a non-originating material imported from a non-Party used in the production of the good unless the non-originating material is provided for in a different subheading from that of the good)"
        r"|provided that (?P<prov>.+)))?", a)
    if not m:
        return None
    what = m.group("what")
    if " from " in what:
        return None
    src = m.group("src")
    if src.startswith("transmission"):
        lvl, within = "subheading", True
    else:
        lvl = {"any other Chapter": "chapter", "any other Heading": "heading", "any other Subheading": "subheading", "within this Subheading or any other Subheading": "subheading"}[src]
        within = src.startswith("within")
    ctc = {"kind": "ctc", "level": lvl}
    if within:
        ctc["within"] = True
    if m.group("exk"):
        ex = ranges(m.group("ex"), level_of(m.group("exk")))
        if ex is None:
            return None
        ctc["except"] = ex
    if m.group("expart"):
        d = code_digits(re.search(r"Subheading ([\d.]+)", m.group("expart")).group(1))
        ctc["except"] = [{"level": "subheading", "from": d, "to": d, "partial": "triethanolamine's salt only"}]
    name = {"chapter": "CC", "heading": "CTH", "subheading": "CTSH"}[lvl]
    if within:
        name = "CTSH (within)"
    reqs = [ctc]
    if m.group("nodm"):
        ctc["no_de_minimis_same_subheading"] = True
    if m.group("prov"):
        pr = parse_provisos(m.group("prov"))
        if pr is None:
            return None
        reqs += pr
        name += " with conditions"
    return name, reqs


def parse_variant_free(t):
    alts = re.split(r";\s+or\s+(?=[A-Z])", t)
    out = []
    for a in alts:
        r = parse_alt(a)
        if r is None:
            return None
        out.append({"name": r[0], "requires": r[1]})
    return out


def split_variants(t):
    """A./B. branches and 'For Korea's ... ; For others:' branches."""
    m = re.match(r"A\. ", t)
    if m:
        parts = re.split(r"\s+(?=[B-D]\. )", t)
        res = []
        for p in parts:
            m = re.match(r"([A-D])\. (.*)", p)
            if not m:
                return None
            res.append((m.group(1), m.group(2), None))
        return res
    if t.startswith("For Korea"):
        m = re.match(r"(For Korea.s [^:]*?):\s*(.*?);\s*For others:\s*(.*)", t)
        if not m:
            return None
        return [("korea", m.group(2), m.group(1)), ("others", m.group(3), "Other goods")]
    return None


def label_variant(vid, body):
    m = re.match(r"([^:]{3,80}?)\s*:\s*(.*)", body)
    if m and not body.startswith(("Change", "A regional", "Wholly")):
        return m.group(1).strip(), m.group(2).strip()
    m = re.match(r"Change to (?!Subheading|Heading|Chapter|a Subheading)(.+?) from (?:any other|transmission|goods)", body)
    if m:
        return m.group(1).strip(), body
    m = re.match(r"Change to (other good(?: of Subheading [\d.]+)?) from", body)
    if m:
        return m.group(1), body
    return vid, body


def parse_rule(text):
    """Returns (alternatives, variants) or None."""
    t = clean(text)
    sv = split_variants(t)
    if sv:
        vs = []
        for vid, body, head in sv:
            lab, b = label_variant(vid, body)
            if head:
                lab = head
            alts = parse_variant_free(b)
            if alts is None:
                return None
            vs.append({"id": vid.lower() if vid in ("korea", "others") else vid, "label": lab, "alternatives": alts})
        return [], vs
    alts = parse_variant_free(t)
    if alts is None:
        return None
    return alts, []


def concise(alts, variants):
    def names(al):
        return " or ".join(a["name"] for a in al)
    if variants:
        return "; ".join("%s: %s" % (v["id"], names(v["alternatives"])) for v in variants)
    return names(alts)


def read_rows(pdf_path):
    import pdfplumber

    rows = []
    with pdfplumber.open(pdf_path) as p:
        for pi, pg in enumerate(p.pages[1:], 2):
            for t in pg.extract_tables():
                for r in t:
                    r = [(c or "").strip() for c in r]
                    if len(r) < 8 or r[1] in ("Serial", "No") or r[3] == "Harmonized System Code":
                        continue
                    code = " ".join(x for x in (r[3], r[4], r[5]) if x)
                    rows.append({"page": pi, "serial": r[0], "code": code, "desc": r[6], "rule": r[7]})
    out = []
    for r in rows:
        if r["serial"].isdigit():
            out.append(dict(r))
        elif out and r["rule"] and r["rule"] != "Origin Conferring Criteria":
            # a row split across a page break
            out[-1]["rule"] += " " + r["rule"]
            out[-1]["desc"] += (" " + r["desc"]) if r["desc"] else ""
    return out


def code_norm(code):
    code = clean(code)
    m = re.fullmatch(r"Chapter (\d{1,2})", code)
    if m:
        return "Chapter %02d" % int(m.group(1))
    if re.fullmatch(r"\d\d\.\d\d", code) or re.fullmatch(r"\d{4}\.\d\d", code):
        return code
    return None


def build(pdf, out):
    raw = open(pdf, "rb").read()
    sha = hashlib.sha256(raw).hexdigest()
    rows = read_rows(pdf)
    nums = [int(r["serial"]) for r in rows]
    if nums != list(range(1, len(nums) + 1)):
        sys.exit("serial numbers are not 1..n")
    rules = []
    for r in rows:
        code = code_norm(r["code"])
        if code is None:
            sys.exit("row %s: code %r" % (r["serial"], r["code"]))
        full = clean(r["rule"])
        rule = {"row": int(r["serial"]), "code": code, "description": clean(r["desc"]), "full": full}
        parsed = parse_rule(full)
        if parsed is None:
            rule["text_only"] = True
            rule["concise"] = "text rule"
        else:
            alts, vs = parsed
            if alts:
                rule["alternatives"] = alts
            if vs:
                rule["variants"] = vs
            rule["concise"] = concise(alts, vs)
        rules.append(rule)
    doc = {
        "version": VERSION,
        "source": {"url": URL, "title": TITLE, "sha256": sha, "rows": len(rules)},
        "rules": rules,
    }
    with open(out, "w") as f:
        json.dump(doc, f, ensure_ascii=False, indent=1)
        f.write("\n")
    textonly = [r["row"] for r in rules if r.get("text_only")]
    print("%d rows, %d read, %d text-only %s" % (len(rules), len(rules) - len(textonly), len(textonly), textonly))


def count_kinds(r):
    n = {"wo": 0, "wo_any": 0, "rvc": 0, "ctc": 0, "except": 0, "materials": 0, "process": 0}
    alts = list(r.get("alternatives", []))
    for v in r.get("variants", []):
        alts += v["alternatives"]
    for a in alts:
        for q in a["requires"]:
            n[q["kind"]] = n.get(q["kind"], 0) + 1
            if q.get("except"):
                n["except"] += 1
    return n


def independent(full):
    """Counts read straight from the words, not through the grammar."""
    t = full
    return {
        "wo": len(re.findall(r"Wholly-Obtained or Produced in the territory of the exporting Party", t)),
        "wo_any": len(re.findall(r"^(?:[A-D]\. [^:]*: ?)?Wholly-Obtained or Produced in the territory of any Party|; or Wholly-Obtained or Produced in the territory of any Party", t)),
        "rvc": len(re.findall(r"regional value content of not less than", t, flags=re.I)),
        "except": len(re.findall(r", except from ", t)),
    }


def check(path):
    doc = json.load(open(path))
    bad = 0
    for r in doc["rules"]:
        have, said = count_kinds(r), independent(r["full"])
        # "provided that ... are Wholly-Obtained" conditions are not WO criteria
        for k in ("rvc", "except"):
            if have[k] != said[k]:
                print("row %d: %d %s in the rule, %d in the text" % (r["row"], have[k], k, said[k]))
                bad += 1
        parsed = parse_rule(r["full"])
        if parsed is None:
            if not r.get("text_only"):
                print("row %d: stored rule does not re-read" % r["row"])
                bad += 1
            continue
        alts, vs = parsed
        if r.get("text_only") or r.get("alternatives", []) != alts or r.get("variants", []) != vs:
            print("row %d: stored rule differs from the text" % r["row"])
            bad += 1
    print("%d rows checked, %d differences" % (len(doc["rules"]), bad))
    sys.exit(1 if bad else 0)


if __name__ == "__main__":
    if len(sys.argv) == 4 and sys.argv[1] == "build":
        build(sys.argv[2], sys.argv[3])
    elif len(sys.argv) == 3 and sys.argv[1] == "check":
        check(sys.argv[2])
    else:
        sys.exit(__doc__)
