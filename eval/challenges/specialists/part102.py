"""Part 102 marking specialist (usmca-v5) challenge cases, with a reference oracle.

NOT the USMCA preference calculator. The candidate is the production Part 102
engine (19 CFR 102.11, 102.13, 102.18, 102.20). The oracle below reads the
102.20 row text in backend/internal/tariffshift/rules.json (the eCFR 102.20
text as of 2026-11-18) with its own parser and applies 102.11(a)(2)-(3), (b),
102.13 (de minimis: 7 percent or less of the adjusted value) and 102.18(b)(1).
It shares no code with the candidate or with internal/tariffshift. Rows used
are only those whose whole text is "A change to [sub]heading X [through Y] from
any other chapter/heading/subheading." so a shift can be decided from codes
alone; rows with exceptions, alternatives or value tests are not used.
"""
import os, sys
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from common import *

RULES = os.path.join(DATA, "..", "internal", "tariffshift", "rules.json")
HS = [l.strip() for l in open(os.path.join(DATA, "..", "internal", "jprules", "data", "hs2022_codes.txt")) if len(l.strip()) == 6]
PROD = ["Canada", "Mexico"]
MATS = [c for c in HS if c[:2] in ("39", "40", "72", "73", "74", "76", "82", "83", "84", "85", "87", "90", "94")]
FOREIGN = ["China", "Germany", "Japan", "South Korea", "India", "Taiwan", "Vietnam", "Italy", "Brazil", "Thailand", "United States", "Malaysia", "Turkey", "Poland"]
PARTS = ["housing", "drive unit", "control board", "valve set", "frame", "cover", "bracket set", "fastener kit", "wiring set", "rotor", "casing",
         "seal kit", "handle assembly", "blade set", "filter element", "gear set", "bearing set", "mounting plate", "sensor module", "coupling"]
PAT = re.compile(r"A change to (sub)?heading ([0-9.]+)(?: through ([0-9.]+))? from any other (chapter|heading|subheading)(?:, including another subheading within that group)?\.")


def d(code): return "".join(ch for ch in code if ch.isdigit())


def rows():
    out = []
    for x in json.load(open(RULES))["rules"]:
        if len(x["alternatives"]) != 1: continue
        m = PAT.fullmatch(x["text"])
        if not m: continue
        lo = d(m.group(2)); hi = d(m.group(3)) if m.group(3) else lo
        if not re.search(r"Chapters (72 through 83|84 through 85|86 through 89|90 through 92|94 through 96)", x["section"]): continue
        out.append({"id": x["id"], "text": x["text"], "lo": lo, "hi": hi, "level": m.group(4)})
    return out


def goods_in(row):
    n = len(row["lo"])
    k = {"chapter": 2, "heading": 4, "subheading": 6}[row["level"]]
    return [c for c in HS if row["lo"] <= c[:n] <= row["hi"] and (k == 6 or sum(1 for o in HS if o[:k] == c[:k]) > 1)]


def shifts(row, good, mat):
    """Does a material of code mat make the change the row requires for good?"""
    lvl = row["level"]
    k = {"chapter": 2, "heading": 4, "subheading": 6}[lvl]
    return d(mat)[:k] != d(good)[:k]


def dot(c): return c[:4] + "." + c[4:]


def pick_code(r, row, good, want_shift):
    """A 6-digit material code that does (want_shift) or does not make the change for the row."""
    k = {"chapter": 2, "heading": 4, "subheading": 6}[row["level"]]
    for _ in range(400):
        c = r.choice(MATS)
        if shifts(row, good, c) == want_shift: return c
    raise RuntimeError("no code")


def fail_code(r, row, good):
    k = {"chapter": 2, "heading": 4, "subheading": 6}[row["level"]]
    same = [c for c in HS if c[:k] == good[:k] and c != good] if k < 6 else [good]
    return r.choice(same)


def render(r, style, good, gname, prod, mats, adjective, closing):
    lead = ["A producer in %s asks for the marking country of origin of %s (HTSUS %s) under the USMCA Marking Rules, 19 CFR Part 102." % (prod, gname, dot(good)),
            "Request for a country of origin ruling for %s classified in HTSUS %s, produced in %s, for marking under the Part 102 USMCA marking rules." % (gname, dot(good), prod),
            "%s (HTSUS %s) is produced in %s and exported to the United States. The producer asks which country of origin applies for marking under the USMCA Marking Rules (Part 102)." % (gname[0].upper() + gname[1:], dot(good), prod)][style % 3]
    sents = []
    for m in mats:
        s = "The %s is made in %s" % (m["name"], m["origin"])
        s += (" and is classified in HTSUS %s" % dot(m["code"])) if m.get("code") else " and its tariff classification has not been determined"
        if m.get("share") is not None: s += "; it accounts for %s percent of the adjusted value of the good" % m["share"]
        sents.append(s)
    sents.append("All of these materials are assembled and the finished good is produced in %s" % prod)
    if style % 3 == 1:
        body = "Materials: " + "; ".join("(%d) %s" % (i + 1, s[0].lower() + s[1:]) for i, s in enumerate(sents)) + "."
    else:
        body = ". ".join(sents) + "."
    return lead + " " + body + (" " + closing if closing else "")


def scenario(split, k, r, rowset):
    row = r.choice(rowset); good = r.choice(goods_in(row)); prod = r.choice(PROD)
    gname = r.choice(["the assembled good", "the finished article", "the unit", "the product", "the apparatus", "the assembly"])
    nmat = 2 + r.randrange(2); names = r.sample(PARTS, nmat + 1)
    foreign = r.sample(FOREIGN, nmat)
    mats = [{"name": names[i], "origin": foreign[i], "code": pick_code(r, row, good, True), "share": None} for i in range(nmat)]
    rest = 100 - 0
    mode = k % 9
    desc, exp, why, tmpl = None, None, None, None
    shares = [r.randrange(5, 20) for _ in mats]
    for m, s in zip(mats, shares): m["share"] = s
    if mode in (0, 1):  # all foreign materials make the shift
        exp = {"kind": "decided", "origin": prod, "rule": "(a)(3)"}
        tmpl = "all-materials-shift"; why = "each foreign material is in a different %s from the good, so each makes the %s shift and (a)(3) gives the country of production" % (row["level"], row["level"])
    elif mode in (2, 3):  # one fails, above de minimis
        f = mats[0]; f["code"] = fail_code(r, row, good); f["share"] = r.randrange(9, 40)
        exp = {"kind": "decided", "origin": f["origin"], "rule": "(b)"}
        tmpl = "one-material-fails"; why = "the %s (%s) is in the same %s as the good so it does not make the shift; its value is above the 7 percent de minimis of 102.13, so (b) gives its origin as the only failing material (102.18(b)(1))" % (f["name"], dot(f["code"]), row["level"])
    elif mode == 4:  # fails but de minimis
        f = mats[0]; f["code"] = fail_code(r, row, good); f["share"] = r.choice([2, 3, 4, 5, 6, 7])
        exp = {"kind": "decided", "origin": prod, "rule": "(a)(3) via 102.13"}
        tmpl = "failing-material-de-minimis"; why = "the %s fails the shift but is worth %d percent, at most 7 percent of the adjusted value, so 102.13 disregards it and the country of production decides" % (f["name"], f["share"])
    elif mode == 5:  # failing material is domestic: not foreign
        f = mats[0]; f["origin"] = prod; f["code"] = fail_code(r, row, good); f["share"] = r.randrange(15, 45)
        exp = {"kind": "decided", "origin": prod, "rule": "(a)(2)/(a)(3)"}
        tmpl = "failing-material-is-domestic"; why = "the %s fails the shift but is made in %s, the country of production, so it is not a foreign material; the foreign materials all shift" % (f["name"], prod)
    elif mode == 6:  # two failing, different countries, essential character stated
        a, b = mats[0], mats[1]; a["code"] = fail_code(r, row, good); b["code"] = fail_code(r, row, good); a["share"], b["share"] = r.randrange(10, 25), r.randrange(10, 25)
        win = r.choice([a, b])
        exp = {"kind": "decided", "origin": win["origin"], "rule": "(b)"}
        tmpl = "two-failing-essential-character"; why = "both the %s and the %s fail the shift and exceed de minimis; the facts state the %s imparts the essential character, so (b) gives its origin (102.18(b)(1))" % (a["name"], b["name"], win["name"])
        desc = "The %s imparts the essential character of the good." % win["name"]
    elif mode == 7:  # uncertain: failing material value not given and origin differs
        f = mats[0]; f["code"] = fail_code(r, row, good); f["share"] = None
        exp = {"kind": "uncertain", "origin": None, "rule": "102.13"}
        tmpl = "uncertain-failing-material-value-not-given"; why = "the %s fails the shift and its value share is not given: below 7 percent 102.13 disregards it (origin %s), above it (b) gives %s; the answer turns on the missing value" % (f["name"], prod, f["origin"])
    else:  # uncertain: a material's classification not determined
        f = mats[0]; f["code"] = None; f["share"] = r.randrange(15, 40)
        exp = {"kind": "uncertain", "origin": None, "rule": "102.20"}
        tmpl = "uncertain-material-classification-unknown"; why = "the %s's tariff classification is undetermined and its value exceeds 7 percent, so whether it makes the %s shift, and hence the origin, cannot be decided" % (f["name"], row["level"])
    assert sum(m["share"] or 0 for m in mats) < 100
    style = r.randrange(3)
    closing = " ".join(x for x in [desc, r.choice(["", "", "State the country of origin for marking."])] if x)
    facts = render(r, style, good, gname, prod, mats, "", closing)
    return row, good, tmpl, exp, why, style, facts


QUOTA = {"dev": 48, "test": 112}


def build(split):
    allrows = rows()
    allrows = [x for x in allrows if goods_in(x)]
    mine = [x for i, x in enumerate(sorted(allrows, key=lambda x: sha256_str(x["id"]))) if (i % 3 == 0) == (split == "dev")]
    out = []
    ids = pool_ids("usmca")
    for k in range(QUOTA[split]):
        r = rng_for("part102", split, k)
        row, good, tmpl, exp, why, style, facts = scenario(split, k, r, mine)
        ex = exemplars(ids, r"\b%s\b" % re.escape(dot(good)[:4]), 2)
        tags = [exp["kind"], tmpl, "style-%d" % style, "multi-country"]
        out.append(make_case("part102_specialist", split, "p102-%d" % k, "row-" + row["id"].split("102.20")[1].strip(", ()"), "part102-" + row["level"], tmpl, tags,
                             [{"id": "ecfr-102-20", "locator": "19 CFR %s: %s" % (row["id"], row["text"])},
                              {"id": "ecfr-102-11-102-13-102-18", "locator": "19 CFR 102.11(a)(2)-(3), (b); 102.13 (de minimis 7 percent); 102.18(b)(1)"}],
                             why, facts, exp, exclude=ex))
    return out


def sha256_str(s): return hashlib.sha256(s.encode()).hexdigest()


if __name__ == "__main__":
    for s in ("dev", "test"):
        cs_ = build(s)
        print(s, len(cs_), sum(c["expected"]["kind"] == "uncertain" for c in cs_), len({c["source_family"] for c in cs_}))
        for c in cs_[2:20:6]: print(c["input"]["facts"], c["expected"], c["rationale"]); print()
