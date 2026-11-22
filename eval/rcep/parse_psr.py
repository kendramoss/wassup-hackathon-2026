"""Parse the RCEP Annex 3A product-specific rules (HS 2022) PDF into JSON.

Usage (needs pdfplumber, which is not a repo dependency):
    python3 -I eval/rcep/parse_psr.py rows   <psr.pdf> <rows.json>
    python3 -I eval/rcep/parse_psr.py build  <rows.json> <out.json> <source.json>
    python3 -I eval/rcep/parse_psr.py diff   <rowsA.json> <rowsB.json>

"rows" reads the PDF tables cell by cell. "build" turns rows into the rule
table the Go engine embeds (backend/internal/rcep/data/psr_hs2022.json).
"diff" compares two copies of the PDF row by row. The PDF itself is not
committed; source.json records where it came from and its SHA-256.
"""
import json
import re
import sys

CODE_CH = re.compile(r"^\d\d$")
CODE_HD = re.compile(r"^\d\d\.\d\d$")
CODE_SUB = re.compile(r"^\d{4}\.\d\d$")


def norm(s):
    return re.sub(r"\s+", " ", (s or "").replace("’", "'")).strip()


def cmd_rows(pdf_path, out_path):
    import pdfplumber  # noqa: deferred so build/diff run without it

    out = []
    with pdfplumber.open(pdf_path) as pdf:
        for pi, page in enumerate(pdf.pages):
            for table in page.extract_tables():
                for r in table:
                    r = [(c or "").strip() for c in r]
                    if len(r) != 5:
                        out.append({"page": pi + 1, "bad": r})
                        continue
                    if r[0].startswith("HS Code") or r[0] == "Chapter":
                        continue
                    out.append({"page": pi + 1, "ch": r[0], "hd": r[1], "sub": r[2],
                                "desc": r[3], "rule": r[4]})
    with open(out_path, "w") as f:
        json.dump(out, f, ensure_ascii=False, indent=0)
    print(f"{len(out)} rows")


def merge(rows):
    """Join rule cells split across pages; drop section banners."""
    merged = []
    for r in rows:
        if "bad" in r:
            merged.append(r)
            continue
        if r["ch"].startswith("SECTION"):
            continue
        has_code = r["ch"] or r["hd"] or r["sub"]
        if not has_code and not r["desc"] and r["rule"] and merged:
            merged[-1]["rule"] = merged[-1]["rule"] + " " + r["rule"]
            merged[-1]["pages"].append(r["page"])
            continue
        if not has_code:
            continue  # descriptive dash rows such as "- Coconuts"
        r = dict(r)
        r["pages"] = [r.pop("page")]
        merged.append(r)
    return merged


EXCEPT = re.compile(
    r"^(CC|CTH|CTSH) except from (chapters?|headings?|subheadings?) (.+)$", re.I)


def parse_codes(level, text):
    """'72.08 or 72.09' / '55.09 to 55.11' / '8504.31, 8504.33 or 8504.34'."""
    text = text.strip()
    m = re.fullmatch(r"([\d.]+) (?:to|through) ([\d.]+)", text)
    if m:
        return [{"level": level, "from": m.group(1), "to": m.group(2)}]
    parts = re.split(r",\s*|\s+or\s+|\s+and\s+", text)
    out = []
    for p in parts:
        p = p.strip()
        if not re.fullmatch(r"[\d.]+", p):
            return None
        out.append({"level": level, "from": p, "to": p})
    return out


def parse_alt(a):
    a = a.strip()
    if a in ("WO", "CC", "CTH", "CTSH", "CR"):
        return {"criterion": a}
    m = re.fullmatch(r"RVC ?(\d+)", a)
    if m:
        return {"criterion": "RVC", "threshold": int(m.group(1))}
    m = EXCEPT.match(a)
    if m:
        lvl = m.group(2).lower().rstrip("s")
        codes = parse_codes(lvl, m.group(3))
        if codes is None:
            return None
        return {"criterion": m.group(1).upper(), "except": codes}
    return None


def parse_rule(text):
    """Return (alternatives, None) or (None, reason)."""
    t = norm(text)
    if not t:
        return None, "empty"
    if re.match(r"^1\.", t):
        return None, "numbered product-specific alternatives (ex-outs by product description)"
    if "provided that" in t:
        return None, "condition in words (provided that ...)"
    # Split on "or" between alternatives. Commas before "or" are optional in the PDF.
    pieces = re.split(r",?\s+or\s+(?=(?:RVC|CR\b|CTH|CTSH|CC\b))", t)
    pieces = [p.strip(" ,") for p in pieces]
    alts = []
    for p in pieces:
        a = parse_alt(p)
        if a is None:
            return None, f"unrecognised alternative: {p!r}"
        alts.append(a)
    return alts, None


def cmd_build(rows_path, out_path, source_path):
    rows = merge(json.load(open(rows_path)))
    source = json.load(open(source_path))
    rules, unparsed, bad = [], [], []
    for r in rows:
        if "bad" in r:
            bad.append(r)
            continue
        if r["sub"]:
            level, code = "subheading", r["sub"]
            ok = CODE_SUB.match(code)
        elif r["hd"]:
            level, code = "heading", r["hd"]
            ok = CODE_HD.match(code)
        else:
            level, code = "chapter", r["ch"]
            ok = CODE_CH.match(code)
        if not ok:
            bad.append(r)
            continue
        desc = norm(r["desc"])
        text = norm(r["rule"])
        entry = {"code": code, "level": level, "description": desc, "text": text,
                 "pages": r["pages"]}
        if r["hd"] and r["sub"]:
            entry["heading"] = r["hd"]  # e.g. 96.02 9602.00 on one row
        if not text:
            # A chapter or heading row whose rules sit on its subheadings.
            entry["status"] = "split"
            rules.append(entry)
            continue
        alts, why = parse_rule(text)
        if alts is None:
            entry["status"] = "manual"
            entry["reason"] = why
            unparsed.append({"code": code, "level": level, "text": text, "reason": why,
                             "pages": r["pages"]})
        else:
            entry["status"] = "parsed"
            entry["alternatives"] = alts
        rules.append(entry)
    with_rule = [e for e in rules if e["status"] != "split"]
    out = {
        "version": source["version"],
        "source": source,
        "coverage": {
            "rows": len(rules),
            "rows_with_rule": len(with_rule),
            "parsed": sum(1 for e in with_rule if e["status"] == "parsed"),
            "manual": sum(1 for e in with_rule if e["status"] == "manual"),
            "split_rows_without_rule": sum(1 for e in rules if e["status"] == "split"),
            "unreadable_rows": len(bad),
            "chapters": len({e["code"] for e in rules if e["level"] == "chapter"}),
        },
        "unparsed": unparsed,
        "rules": rules,
    }
    with open(out_path, "w") as f:
        json.dump(out, f, ensure_ascii=False, indent=1)
        f.write("\n")
    print(json.dumps(out["coverage"]))
    for u in unparsed:
        print("manual", u["code"], u["reason"], "|", u["text"][:90])
    if bad:
        print("unreadable", bad)
        sys.exit(1)


def cmd_diff(a_path, b_path):
    a = merge(json.load(open(a_path)))
    b = merge(json.load(open(b_path)))
    codes = lambda r: (r.get("ch"), r.get("hd"), r.get("sub"))
    print(f"A {len(a)} rows, B {len(b)} rows")
    rule_diffs = [(i, x, y) for i, (x, y) in enumerate(zip(a, b))
                  if codes(x) != codes(y) or norm(x.get("rule")) != norm(y.get("rule"))]
    desc_diffs = [(i, x, y) for i, (x, y) in enumerate(zip(a, b))
                  if norm(x.get("desc")) != norm(y.get("desc"))]
    print(f"code or rule differences: {len(rule_diffs)}")
    print(f"description differences: {len(desc_diffs)}")
    for i, x, y in (rule_diffs + desc_diffs)[:50]:
        print(i, codes(x), "A:", norm(x.get("desc")), "|", norm(x.get("rule")))
        print(i, codes(y), "B:", norm(y.get("desc")), "|", norm(y.get("rule")))
    sys.exit(1 if rule_diffs or len(a) != len(b) else 0)


if __name__ == "__main__":
    cmd = sys.argv[1]
    if cmd == "rows":
        cmd_rows(sys.argv[2], sys.argv[3])
    elif cmd == "build":
        cmd_build(sys.argv[2], sys.argv[3], sys.argv[4])
    elif cmd == "diff":
        cmd_diff(sys.argv[2], sys.argv[3])
    else:
        sys.exit("usage: rows|build|diff")
