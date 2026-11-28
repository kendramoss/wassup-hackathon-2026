import json
import os
import re

from oracle import digits

ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "..", ".."))
BK = os.path.join(ROOT, "backend", "internal")


def _load(rel):
    with open(os.path.join(BK, rel)) as f:
        return json.load(f)


def _codes(rel):
    with open(os.path.join(BK, rel)) as f:
        return [l.strip() for l in f if l.strip()]


def dotted(d):
    return d[:4] + "." + d[4:] if len(d) >= 6 else (d[:2] + "." + d[2:] if len(d) == 4 else d)


def _plain_alts(row):
    alts = []
    for a in row.get("alternatives", []):
        reqs = []
        for q in a["requires"]:
            k = q["kind"]
            if k == "rvc" and set(q) <= {"kind", "threshold"}:
                reqs.append(("rvc", q["threshold"]))
            elif k == "ctc" and set(q) <= {"kind", "level", "except"} and all("partial" not in e for e in q.get("except", [])):
                reqs.append(("ctc", q["level"], q.get("except", [])))
            elif k == "wo":
                reqs.append(("wo",))
            elif k == "process":
                reqs.append(("process", q["process"]))
            elif k == "wo_any":
                reqs.append(("wo_any",))
            elif k == "materials" and q.get("origin") == "wo_any":
                reqs.append(("materials", q["scope"]))
            elif k == "value_cap":
                reqs.append(("value_cap", q["scope"], q["percent"]))
            else:
                return None
        alts.append((a["name"], reqs))
    return alts or None


def _rcep_alts(row):
    alts = []
    for a in row.get("alternatives", []):
        if a.get("condition"):
            return None
        c = a["criterion"]
        if c == "RVC":
            alts.append(("RVC%d" % a["threshold"], [("rvc", a["threshold"])]))
        elif c in ("CC", "CTH", "CTSH"):
            lvl = {"CC": "chapter", "CTH": "heading", "CTSH": "subheading"}[c]
            ex = a.get("except", [])
            name = c
            if ex:
                parts = []
                for x in ex:
                    parts.append(x["from"] if x["from"] == x["to"] else x["from"] + " to " + x["to"])
                name = c + " except from " + ex[0]["level"] + " " + ", ".join(parts)
            alts.append((name, [("ctc", lvl, ex)]))
        elif c == "WO":
            alts.append(("WO", [("wo",)]))
        elif c == "CR":
            alts.append(("CR", [("chem",)]))
        else:
            return None
    return alts or None


def shape(alts):
    if alts is None:
        return "other"
    kinds = [tuple(q[0] for q in reqs) for _, reqs in alts]
    if all(k == ("wo",) for k in kinds):
        return "wo"
    if kinds == [("rvc",)]:
        return "rvc"
    if len(kinds) == 1 and kinds[0] == ("ctc",):
        ex = alts[0][1][0][2]
        return "ctc_except" if ex else "ctc"
    if len(kinds) == 2 and sorted(kinds) == [("ctc",), ("rvc",)]:
        ex = [q[2] for _, r in alts for q in r if q[0] == "ctc"][0]
        return "ctc_rvc_except" if ex else "ctc_rvc"
    return "other"


class Row:
    def __init__(self, code6, rowcode, concise, alts, desc, listed, level):
        self.code6 = code6
        self.rowcode = rowcode
        self.concise = concise
        self.alts = alts
        self.desc = desc
        self.listed = listed
        self.level = level
        self.shape = shape(alts)
        self.chapter = int(code6[:2])

    def rule(self):
        return {"alts": self.alts}


GENERAL = {
    "atiga": ("Art. 28.1", "RVC40 or CTH", [("RVC40", [("rvc", 40)]), ("CTH", [("ctc", "heading", [])])]),
    "akfta": ("Rule 4.1", "RVC40 or CTH", [("RVC40", [("rvc", 40)]), ("CTH", [("ctc", "heading", [])])]),
}
ACFTA_CTH_CHAPTERS = set()
for lo, hi in [(25, 26), (28, 29), (31, 31), (39, 39), (42, 49), (57, 59), (61, 62), (64, 64), (66, 71), (73, 83), (86, 86), (88, 88), (91, 97)]:
    ACFTA_CTH_CHAPTERS.update(range(lo, hi + 1))
ACFTA_RVC_ONLY_HEADINGS = {"2901", "2902", "3105", "3901", "3902", "3903", "3907", "3908"}


class Tables:
    def __init__(self):
        self.hs2022 = _codes("jprules/data/hs2022_codes.txt")
        self.hs2017 = _codes("jprules/data/hs2017_codes.txt")
        self.chapters = {}
        rc = _load("rcep/data/psr_hs2022.json")["rules"]
        for r in rc:
            if r["level"] == "chapter":
                t = r["description"].split(":", 1)[1].strip().lower().split(";")[0].split(", not elsewhere")[0]
                if len(t) > 42:
                    t = t[:42].rsplit(" ", 1)[0].rstrip(" ,")
                self.chapters[int(r["code"])] = t
        at = _load("atiga/data/psr_hs2022.json")["rules"]
        ak = _load("aseanfta/data/akfta_psr_hs2022.json")["rules"]
        ac = _load("aseanfta/data/acfta_psr_hs2017.json")["rules"]
        self.rows = {}
        self.sub = {"atiga": {}, "akfta": {}, "acfta": {}, "rcep": {}}
        self.head = {"akfta": {}, "rcep": {}}
        self.chap = {"akfta": {}, "rcep": {}}
        for r in at:
            self.sub["atiga"][digits(r["code"])] = r
        for r in ac:
            self.sub["acfta"][digits(r["code"])] = r
        for r in ak:
            d = digits(r["code"])
            if len(d) == 6:
                self.sub["akfta"][d] = r
            elif len(d) == 4:
                self.head["akfta"][d] = r
            else:
                self.chap["akfta"][d] = r
        for r in rc:
            d = digits(r["code"])
            if r.get("status") == "split":
                continue
            if r["level"] == "subheading":
                self.sub["rcep"][d] = r
            elif r["level"] == "heading":
                self.head["rcep"][d] = r
            else:
                self.chap["rcep"][d] = r
        self.vintage = {"atiga": self.hs2022, "akfta": self.hs2022, "acfta": self.hs2017, "rcep": self.hs2022}

    def resolve(self, feature, hs6):
        r = self.sub[feature].get(hs6)
        level = "subheading"
        if r is None and feature in self.head:
            r = self.head[feature].get(hs6[:4])
            level = "heading"
        if r is None and feature in self.chap:
            r = self.chap[feature].get(hs6[:2])
            level = "chapter"
        if r is None:
            if feature in ("atiga", "akfta"):
                code, concise, alts = GENERAL[feature]
                return Row(hs6, code, concise, alts, "goods not listed", False, "general")
            if feature == "acfta":
                ch = int(hs6[:2])
                alts = [("RVC40", [("rvc", 40)])]
                concise = "RVC40"
                if ch in ACFTA_CTH_CHAPTERS and hs6[:4] not in ACFTA_RVC_ONLY_HEADINGS:
                    alts.append(("CTH", [("ctc", "heading", [])]))
                    concise = "RVC40 or CTH"
                return Row(hs6, "Art. 4(1)", concise, alts, "goods not listed", False, "general")
            return None
        alts = _rcep_alts(r) if feature == "rcep" else _plain_alts(r)
        if r.get("variants") or r.get("note"):
            alts = None
        return Row(hs6, r["code"], r.get("concise") or r.get("text"), alts, r.get("description", ""), True, level)

    def pool(self, feature):
        out = []
        for c in self.vintage[feature]:
            row = self.resolve(feature, c)
            if row is not None:
                out.append(row)
        return out

    def chapter_title(self, hs):
        return self.chapters.get(int(hs[:2]), "goods of chapter " + hs[:2])
