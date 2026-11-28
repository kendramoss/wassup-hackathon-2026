"""Japan preference engine challenge cases, with a reference oracle.

Rows are read from backend/internal/jprules/data/schedules.json (the product-
specific-rule extract of each agreement's published annex, with page and source
hash recorded there) and only rows whose entire rule text is CC, CTH or CTSH, in
agreements and chapters without a chapter or section note, are used. The oracle
applies the change-of-classification test to the materials' stated codes and
origin status with its own code. It shares no code with internal/jpengine or
internal/jprules. Failing materials always carry at least 25 percent of the
ex-works price so no tolerance or de minimis provision of any agreement can
apply. The label is outcome only; the criterion the model names is graded by
the set of criteria that are legally valid on the stated facts (any of CC, CTH,
CTSH or PE that the facts satisfy and the row accepts), never by a single one.
"""
import os, sys
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from common import *

SCHED = os.path.join(DATA, "..", "internal", "jprules", "data", "schedules.json")
HS22 = [l.strip() for l in open(os.path.join(DATA, "..", "internal", "jprules", "data", "hs2022_codes.txt")) if len(l.strip()) == 6]
HS17 = set(l.strip() for l in open(os.path.join(DATA, "..", "internal", "jprules", "data", "hs2017_codes.txt")))
CH = {"82", "83", "84", "85", "90", "91", "92", "94", "95", "96"}
PARTY = {"JP-EU EPA": ["Poland", "Germany", "Italy", "France", "Spain", "Netherlands", "Czechia", "Sweden"],
         "AJCEP": ["Vietnam", "Thailand", "Malaysia", "Indonesia", "Philippines", "Singapore", "Cambodia"],
         "JP-Thailand EPA": ["Thailand"], "JP-Indonesia EPA": ["Indonesia"], "JP-Australia EPA": ["Australia"],
         "JP-UK CEPA": ["United Kingdom"], "JP-Vietnam EPA": ["Vietnam"], "JP-Switzerland EPA": ["Switzerland"]}
NONPARTY = ["China", "Brazil", "Turkey", "India", "Taiwan", "United States", "South Africa", "Argentina", "Mexico", "Egypt"]
SHORT = {"JP-EU EPA": "the Japan-EU EPA", "AJCEP": "AJCEP", "JP-Thailand EPA": "the Japan-Thailand EPA", "JP-Indonesia EPA": "the Japan-Indonesia EPA",
         "JP-Australia EPA": "the Japan-Australia EPA", "JP-UK CEPA": "the Japan-UK CEPA", "JP-Vietnam EPA": "the Japan-Vietnam EPA",
         "JP-Switzerland EPA": "the Japan-Switzerland EPA"}
PARTS = ["housing", "drive unit", "control board", "valve set", "frame", "cover", "bracket set", "fastener kit", "wiring set", "rotor", "casing",
         "seal kit", "handle assembly", "blade set", "filter element", "gear set", "bearing set", "mounting plate", "sensor module", "coupling"]
CIRC = "①②③④⑤⑥⑦⑧⑨⑩"
LEVEL = {"CC": 2, "CTH": 4, "CTSH": 6}
STRICT = {"CC": ["CC", "CTH", "CTSH"], "CTH": ["CTH", "CTSH"], "CTSH": ["CTSH"]}


def active(s):
    return s["from"] <= DATE and (not s["until"] or DATE < s["until"])


def load_rows():
    out = []
    for s in json.load(open(SCHED)):
        if s["agreement"] not in PARTY or not active(s): continue
        cover = [(r["start"].ljust(6, "0"), r["end"].ljust(6, "9")) for r in s["rows"]]
        noted = {(n["scope"], n["key"]) for n in s["notes"]}
        for r in s["rows"]:
            if r["text"].strip() not in LEVEL or r["start"] != r["end"] or len(r["start"]) not in (4, 6) or r["start"][:2] not in CH: continue
            if ("chapter", r["start"][:2]) in noted: continue
            lo, hi = r["start"].ljust(6, "0"), r["end"].ljust(6, "9")
            if sum(1 for a, b in cover if not (b < lo or a > hi)) != 1: continue
            codes = [c for c in HS22 if lo <= c <= hi and (s["agreement"] != "AJCEP" or c in HS17)]
            if codes: out.append({"agreement": s["agreement"], "citation": s["citation"], "sha": s["sha256"], "url": s["source_url"], "start": r["start"],
                                  "page": r["page"], "rule": r["text"].strip(), "codes": codes})
    return out


def shifts(rule, good, mat): return mat[:LEVEL[rule]] != good[:LEVEL[rule]]


def dot(c): return c[:4] + "." + c[4:]


def mat_code(r, rule, good, want_shift):
    k = LEVEL[rule]
    pool = [c for c in HS22 if c[:2] in CH or c[:2] in ("39", "40", "72", "73", "76")]
    for _ in range(500):
        c = r.choice(pool)
        if shifts(rule, good, c) == want_shift: return c
    raise RuntimeError


def fail_code(r, rule, good):
    k = LEVEL[rule]
    same = [c for c in HS22 if c[:k] == good[:k]]
    return r.choice(same)


def build_case(split, k, r, row):
    ag, rule = row["agreement"], row["rule"]
    good = r.choice(row["codes"]); party = r.choice(PARTY[ag]); mode = k % 12
    n = 2 + r.randrange(2); names = r.sample(PARTS, n)
    mats = []
    for i in range(n):
        mats.append({"name": names[i], "code": mat_code(r, rule, good, True), "origin": r.choice(NONPARTY), "status": "non", "share": r.randrange(25, 33)})
    desc = ""
    prefl = False
    if mode in (0, 1, 2):  # all pass
        exp = {"kind": "decided", "outcome": "qualifies", "criteria": STRICT[rule] + ["PE"][:0]}
        tmpl = "all-non-originating-materials-shift"
        why = "every non-originating material is in a different %s from the good, so the %s requirement of the product-specific rule is met for each" % ({"CC": "chapter", "CTH": "heading", "CTSH": "subheading"}[rule], rule)
        if mode == 2:
            mats[0]["status"] = "orig"; mats[0]["origin"] = party; mats[0]["code"] = fail_code(r, rule, good)
            tmpl = "originating-material-in-same-class-is-ignored"
            why = "the %s is originating (made in %s with proof), so its classification does not matter; the non-originating materials all make the %s change" % (mats[0]["name"], party, rule)
    elif mode in (3, 4, 5):  # one fails
        mats[0]["code"] = fail_code(r, rule, good)
        exp = {"kind": "decided", "outcome": "does_not_qualify", "criteria": []}
        tmpl = "one-non-originating-material-fails"
        why = "the %s is non-originating and in the same %s as the good (%s), so the %s requirement fails; it is %d percent of the price, above any tolerance" % (mats[0]["name"], {"CC": "chapter", "CTH": "heading", "CTSH": "subheading"}[rule], dot(mats[0]["code"]), rule, mats[0]["share"])
    elif mode == 6:  # failing material from a Party but without proof
        mats[0]["code"] = fail_code(r, rule, good); mats[0]["origin"] = r.choice(PARTY[ag]); mats[0]["status"] = "noproof"
        exp = {"kind": "decided", "outcome": "does_not_qualify", "criteria": []}
        tmpl = "party-material-without-proof-is-non-originating"
        why = "the %s was made in a Party but has no proof of origin, so it counts as non-originating and, in the same classification as the good, fails the %s rule" % (mats[0]["name"], rule)
    elif mode == 7:  # all originating
        for m in mats: m["status"] = "orig"; m["origin"] = party; m["code"] = fail_code(r, rule, good) if r.random() < .5 else m["code"]
        exp = {"kind": "decided", "outcome": "qualifies", "criteria": STRICT[rule] + ["PE"]}
        tmpl = "all-materials-originating"
        why = "every material is originating with proof, so the good is produced entirely from originating materials and the tariff-change test is not needed"
    elif mode == 8:  # uncertain: status of a would-fail material unknown
        mats[0]["code"] = fail_code(r, rule, good); mats[0]["status"] = "unknown"; mats[0]["origin"] = None
        exp = {"kind": "uncertain", "outcome": "undetermined", "criteria": []}
        tmpl = "uncertain-origin-status-unknown"
        why = "the %s is in the same classification as the good, so the good qualifies only if that material is originating, and the facts do not say whether it is" % mats[0]["name"]
    elif mode == 9:  # uncertain: a material's classification not given
        mats[0]["code"] = None
        exp = {"kind": "uncertain", "outcome": "undetermined", "criteria": []}
        tmpl = "uncertain-material-classification-missing"
        why = "the %s is non-originating and carries no tariff classification, so the %s test cannot be applied" % (mats[0]["name"], rule)
    elif mode == 10:  # uncertain: no materials list
        mats = []
        exp = {"kind": "uncertain", "outcome": "undetermined", "criteria": []}
        tmpl = "uncertain-no-materials-list"
        why = "no materials are listed, so no change of classification can be tested and nothing shows which inputs are originating"
    else:  # code not in the agreement's edition (AJCEP is HS 2017 until 2027-02-15)
        mats = [dict(m) for m in mats]
        new = [c for c in HS22 if c not in HS17 and c[:2] in CH]
        if ag == "AJCEP" and new:
            good = r.choice(new)
            exp = {"kind": "uncertain", "outcome": "undetermined", "criteria": [], "preflight": True}
            tmpl = "uncertain-code-not-in-schedule-edition"
            why = "the code %s is not in the HS 2017 edition the AJCEP schedule in force on the case date uses, so no rule applies until the code is converted" % dot(good)
            prefl = True
        else:
            for m in mats: m["code"] = mat_code(r, rule, good, True)
            exp = {"kind": "decided", "outcome": "qualifies", "criteria": STRICT[rule]}
            tmpl = "all-non-originating-materials-shift"
            why = "every non-originating material is in a different classification from the good, so the %s requirement is met for each" % rule
    if exp["kind"] == "decided" and exp["outcome"] == "qualifies":
        exp["criteria"] = sorted(set(exp["criteria"]))
    lines = []
    for i, m in enumerate(mats):
        c = ("HS %s" % dot(m["code"])) if m["code"] else "tariff code not stated"
        if m["status"] == "orig": st = "originating, made in %s, with proof of origin" % m["origin"]
        elif m["status"] == "noproof": st = "made in %s, no proof of origin" % m["origin"]
        elif m["status"] == "unknown": st = "origin status not known"
        else: st = "non-originating, made in %s" % m["origin"]
        lines.append("%s %s (%s), %s, %d%% of the ex-works price" % (CIRC[i], m["name"], c, st, m["share"]))
    prod = r.choice(["Machine article", "Finished article", "Assembled apparatus", "Manufactured good", "Product"])
    style = r.randrange(3)
    parts = ["Product: %s\nTariff number of the good (Japan): %s\nPreference claimed: %s\n" % (prod, dot(good), SHORT[ag])]
    parts.append("Materials: " + ("; ".join(lines) if lines else "not listed") + "\n")
    parts.append("Production process: the good is assembled and finished in %s from the materials above." % party)
    if style == 1: parts.append(" The importer holds a certificate of origin for the good.")
    if style == 2: parts.append(" The good is shipped directly from %s to Japan." % party)
    facts = "".join(parts)
    tags = [exp["kind"], tmpl, "style-%d" % style, rule.lower() + "-rule", "preflight" if prefl else "model-call"]
    src = [{"id": "jp-psr", "locator": "%s, product-specific rule row %s, page %s, rule text %s (extract sha256 of the annex PDF %s)" % (row["citation"], row["start"], row["page"], rule, row["sha"][:16])}]
    return make_case("japan_engine", split, "jp-%d" % k, "%s-%s" % (ag, row["start"]), "japan-" + ag, tmpl, tags, src, why, facts, exp,
                     extra_input={"japan": {"agreement": ag, "hs": dot(good)}}), prefl


QUOTA = {"dev": 48, "test": 112}


def build(split):
    rows = load_rows()
    rows.sort(key=lambda x: hashlib.sha256((x["agreement"] + x["start"]).encode()).hexdigest())
    mine = [x for i, x in enumerate(rows) if (i % 3 == 0) == (split == "dev")]
    # spread across agreements
    byag = {}
    for x in mine: byag.setdefault(x["agreement"], []).append(x)
    ags = sorted(byag)
    out = []
    for k in range(QUOTA[split]):
        r = rng_for("japan", split, k)
        ag = ags[k % len(ags)] if k % 12 != 11 else "AJCEP"
        pool = byag.get(ag) or byag[ags[0]]
        row = pool[(k // len(ags)) % len(pool)]
        c, _ = build_case(split, k, r, row)
        out.append(c)
    return out


if __name__ == "__main__":
    for s in ("dev", "test"):
        cs_ = build(s)
        print(s, len(cs_), sum(c["expected"]["kind"] == "uncertain" for c in cs_), len({c["source_family"] for c in cs_}), sum("preflight" in c["variation_tags"] for c in cs_))
        for c in cs_[3:20:6]: print(c["input"], c["expected"], c["rationale"]); print()
