"""Parse the ATIGA Annex 3 product-specific rules (HS 2022) into JSON.

Usage (standard library only):
    python3 -I -B eval/atiga/parse_psr.py build <psr.xlsx> <out.json>
    python3 -I -B eval/atiga/parse_psr.py check <out.json>

"build" reads the workbook the ASEAN Secretariat publishes as Annex 3
(ATIGA-PSR-HS-2022-endorsed-AFTAC36.xlsx), keeps every row verbatim (code,
description, concise and full rule text) and adds the rule read from the
concise column. "check" re-reads the rule from the embedded concise text and
fails if it differs from the stored one, so the table can be verified
without the workbook. The workbook is not committed; the source block
records its URL and SHA-256.
"""
import hashlib
import json
import re
import sys
import xml.etree.ElementTree as ET
import zipfile

NS = {"m": "http://schemas.openxmlformats.org/spreadsheetml/2006/main"}
URL = "https://asean.org/wp-content/uploads/2025/09/ATIGA-PSR-HS-2022-endorsed-AFTAC36.xlsx"
VERSION = "atiga-annex3-psr-hs2022-aftac36"

# Process conditions a caller states; the engine cannot test them from codes.
PROCESS = {
    "textile process rule": "textile_process",
    "cut and sewn in any AMS": "cut_and_sewn",
    "no CTC provided that the good is produced by refining": "refining",
    "no CTC provided that there is a change from wet to dry": "wet_to_dry",
    "Recording sound or other similarly recorded phenomena onto blank or unrecorded media shall confer origin whether or not there has been a CTC": "recording",
    "no CTSH is required provided the machines, instruments and apparatus are manufactured from parts produced solely for the machines, instruments or apparatus of the same subheading": "parts_same_subheading",
}
LEVEL = {"CC": "chapter", "CTH": "heading", "CTSH": "subheading"}


def col(ref):
    letters = re.match(r"[A-Z]+", ref).group(0)
    n = 0
    for ch in letters:
        n = n * 26 + ord(ch) - 64
    return n - 1


def read_xlsx(path):
    z = zipfile.ZipFile(path)
    shared = []
    if "xl/sharedStrings.xml" in z.namelist():
        for si in ET.fromstring(z.read("xl/sharedStrings.xml")).findall("m:si", NS):
            shared.append("".join(t.text or "" for t in si.iter("{%s}t" % NS["m"])))
    sheet = ET.fromstring(z.read("xl/worksheets/sheet1.xml"))
    rows = []
    for row in sheet.find("m:sheetData", NS).findall("m:row", NS):
        cells = {}
        for c in row.findall("m:c", NS):
            v = c.find("m:v", NS)
            t = c.get("t")
            if t == "inlineStr":
                val = "".join(x.text or "" for x in c.iter("{%s}t" % NS["m"]))
            elif v is None:
                val = ""
            elif t == "s":
                val = shared[int(v.text)]
            else:
                val = v.text or ""
            cells[col(c.get("r"))] = val
        rows.append([cells.get(i, "") for i in range(5)])
    return rows


def norm(s):
    return re.sub(r"\s+", " ", (s or "").replace("’", "'")).strip()


def ranges(text):
    """'7213-7217', '7208, 7209 and 7211', '3501.90 or 3503' -> ranges."""
    out = []
    for part in re.split(r",\s*|\s+and\s+|\s+or\s+", text.strip()):
        part = part.strip()
        if not part:
            continue
        lo, _, hi = part.partition("-")
        hi = hi or lo
        for x in (lo, hi):
            if not re.fullmatch(r"\d{4}(\.\d\d)?", x):
                raise ValueError("bad range %r in %r" % (part, text))
        lev = "subheading" if "." in lo else "heading"
        out.append({"level": lev, "from": lo, "to": hi})
    return out


def requirement(tok):
    tok = tok.strip()
    if tok in PROCESS:
        return {"kind": "process", "process": PROCESS[tok]}
    m = re.fullmatch(r"RVC(\d+)", tok)
    if m:
        return {"kind": "rvc", "threshold": int(m.group(1))}
    m = re.fullmatch(r"(CC|CTH|CTSH)(?: except from (.+))?", tok)
    if m:
        r = {"kind": "ctc", "level": LEVEL[m.group(1)]}
        if m.group(2):
            r["except"] = ranges(m.group(2))
        return r
    if tok == "WO":
        return {"kind": "wo"}
    raise ValueError("unread requirement %r" % tok)


ALT_START = r"(?=RVC\d|CC\b|CTH\b|CTSH\b|WO\b|no CT|textile process rule|Recording sound)"


def alternatives(text):
    text = norm(text)
    alts = []
    for alt in re.split(r"\s+or\s+" + ALT_START, text):
        reqs = []
        for tok in re.split(r"\s*\+\s*|\s+and\s+(?=cut and sewn)", alt):
            reqs.append(requirement(tok))
        alts.append({"name": alt, "requires": reqs})
    return alts


def variant_id(label):
    return re.sub(r"[^a-z0-9]+", "_", label.lower()).strip("_")


def read_rule(concise):
    c = norm(concise)
    # "1. RULE for LABEL 2. RULE for other goods"
    m = re.fullmatch(r"1\. (.+?) for (.+?) 2\. (.+?) for (other goods)", c)
    if m:
        return {"variants": [
            {"id": variant_id(m.group(2)), "label": m.group(2), "alternatives": alternatives(m.group(1))},
            {"id": "other", "label": m.group(4), "alternatives": alternatives(m.group(3))}]}
    # "Of steel: RULE; Of stainless steel: RULE" / "For products from 7208: RULE; ..."
    if re.match(r"(Of [a-z ]+|For products from \d{4}): ", c):
        vs = []
        for part in c.split(";"):
            label, _, rule = part.strip().partition(": ")
            vs.append({"id": variant_id(label), "label": label, "alternatives": alternatives(rule)})
        return {"variants": vs}
    return {"alternatives": alternatives(c)}


def build_rules(rows):
    rules = []
    for r in rows:
        no, code, desc, concise, full = [str(x) for x in r]
        if not re.fullmatch(r"\d{4}\.\d\d", code.strip()):
            continue
        rule = {"row": int(float(no)), "code": code.strip(), "description": norm(desc),
                "concise": concise.strip(), "full": full.strip()}
        rule.update(read_rule(concise))
        rules.append(rule)
    return rules


def cmd_build(xlsx, out):
    raw = open(xlsx, "rb").read()
    rules = build_rules(read_xlsx(xlsx))
    doc = {
        "version": VERSION,
        "source": {
            "title": "ATIGA Annex 3, Product Specific Rules, HS 2022 (endorsed by the 36th AFTA Council; in force 1 April 2023)",
            "publisher": "ASEAN Secretariat",
            "url": URL,
            "index_page": "https://asean.org/asean-trade-in-goods-agreement-atiga/",
            "retrieved": "2026-11-27",
            "sha256": hashlib.sha256(raw).hexdigest(),
            "rows": len(rules),
        },
        "rules": rules,
    }
    with open(out, "w") as f:
        head = {k: v for k, v in doc.items() if k != "rules"}
        f.write(json.dumps(head, ensure_ascii=False, indent=1)[:-2] + ',\n "rules": [\n')
        f.write(",\n".join(json.dumps(r, ensure_ascii=False, separators=(",", ":")) for r in rules))
        f.write("\n ]\n}\n")
    print("rules", len(rules))


def cmd_check(path):
    doc = json.load(open(path))
    bad = 0
    for r in doc["rules"]:
        want = read_rule(r["concise"])
        got = {k: r[k] for k in ("alternatives", "variants") if k in r}
        if want != got:
            bad += 1
            print("differs:", r["code"])
    if bad or len(doc["rules"]) != doc["source"]["rows"]:
        sys.exit("atiga psr check failed")
    print("atiga psr ok", len(doc["rules"]))


if __name__ == "__main__":
    if len(sys.argv) == 4 and sys.argv[1] == "build":
        cmd_build(sys.argv[2], sys.argv[3])
    elif len(sys.argv) == 3 and sys.argv[1] == "check":
        cmd_check(sys.argv[2])
    else:
        sys.exit(__doc__)
