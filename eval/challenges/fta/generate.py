#!/usr/bin/env python3
"""Builds the FTA challenge datasets. Standard library only.

Expected labels come from oracle.py, which reads the primary rule rows and the
treaty clauses; nothing here calls the Go engines.
"""
import hashlib
import json
import os
import random
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from oracle import OUTSIDE, PARTIES, digits, evaluate, is_textile, party  # noqa: E402
from rules import Tables, dotted  # noqa: E402
import sources  # noqa: E402

HERE = os.path.dirname(os.path.abspath(__file__))
FEATURES = ["atiga", "akfta", "acfta", "rcep"]
PRODUCERS = {
    "atiga": ["Malaysia", "Thailand", "Vietnam", "Singapore", "Indonesia", "Philippines", "Cambodia", "Laos"],
    "akfta": ["Malaysia", "Thailand", "Vietnam", "Singapore", "Indonesia", "South Korea", "Philippines"],
    "acfta": ["Malaysia", "Thailand", "Vietnam", "China", "Indonesia", "Singapore", "Philippines"],
    "rcep": ["Malaysia", "Thailand", "Vietnam", "Japan", "China", "South Korea", "Australia", "New Zealand", "Indonesia"],
}
FOBS = [100, 120, 200, 250, 400, 500, 800, 1000, 1500, 2000, 4000, 5000]
AUTHORITY_DEV = {
    "atiga": ["miti-2-rvc", "miti-3-cc", "sgc-b-rvc"],
    "akfta": ["ak-wm-build-down", "ak-non-qualifying", "ak-wo-tuna"],
    "acfta": ["ac-mould-rvc", "ac-watch-cth"],
    "rcep": ["p26-wheat-flour", "p27-coffee", "p29-juice"],
}
AUTHORITY_TEST = {
    "atiga": ["miti-4-cth", "miti-5-ctsh", "sgc-f-accumulation", "miti-1-wo"],
    "akfta": ["ak-wm-build-up", "ak-tv-accumulation", "ak-de-minimis-value", "ak-de-minimis-textile", "ak-wo-crude-oil"],
    "acfta": ["ac-accumulation-with", "ac-accumulation-without", "ac-handbag-de-minimis"],
    "rcep": ["p26-maize-flour", "p32-build-up", "p32-build-down", "p33-benzene", "p37-leather-jacket", "p41-spice-cinnamon", "p42-spice-malaysia", "p46-de-minimis"],
}


def clean(o):
    if isinstance(o, dict):
        return {k: clean(v) for k, v in o.items() if v is not None}
    if isinstance(o, list):
        return [clean(v) for v in o]
    if isinstance(o, float) and o == int(o) and abs(o) < 1e12:
        return int(o)
    if isinstance(o, float):
        return round(o, 6)
    return o


class Ctx:
    def __init__(self, tables, feature, split, pool):
        self.t = tables
        self.feature = feature
        self.split = split
        seed = int(hashlib.sha256(("fta-challenge-v1/%s/%s" % (feature, split)).encode()).hexdigest()[:12], 16)
        self.rng = random.Random(seed)
        self.pool = pool
        self.cases = []
        self.seq = {}
        self.used_rows = set()
        self.keys = set()
        self.codes = tables.vintage[feature]
        self.by_ch = {}
        for c in self.codes:
            self.by_ch.setdefault(c[:2], []).append(c)

    def parity_ok(self, row):
        odd = row.chapter % 2 == 1
        return odd if self.split == "dev" else not odd

    def rows(self, shapes, listed=True, textile=False, exclude_chapters=(), family=None):
        out = []
        for r in self.pool:
            if r.shape not in shapes or r.listed != listed or not self.parity_ok(r):
                continue
            if (textile is not None and is_textile(r.code6) != textile) or (family, r.code6) in self.used_rows:
                continue
            if r.chapter in exclude_chapters:
                continue
            out.append(r)
        return out

    def pick_row(self, shapes, listed=True, textile=False, family="x", pred=None):
        rows = [r for r in self.rows(shapes, listed, textile, family=family) if (pred is None or pred(r))]
        if not rows and listed and family not in ("except", "process"):
            rows = [r for r in self.rows(shapes, False, textile, family=family) if (pred is None or pred(r))]
        if not rows and family == "conditions":
            self.used_rows = {u for u in self.used_rows if u[0] != "conditions"}
            rows = [r for r in self.rows(shapes, listed, textile, family=family) if (pred is None or pred(r))]
        if not rows:
            raise RuntimeError("no rows for %s %s %s" % (self.feature, shapes, family))
        r = self.rng.choice(rows)
        self.used_rows.add((family, r.code6))
        return r

    # ---- materials

    def other_code(self, good, rel):
        rng = self.rng
        g = good
        for _ in range(500):
            ch = rng.choice(sorted(self.by_ch))
            c = rng.choice(self.by_ch[ch])
            if rel == "chapter" and c[:2] != g[:2]:
                return c
            if rel == "heading" and c[:2] == g[:2] and c[:4] != g[:4]:
                return c
            if rel == "sub" and c[:4] == g[:4] and c[:6] != g[:6]:
                return c
        if rel == "sub":
            sibs = [c for c in self.by_ch.get(g[:2], []) if c[:4] == g[:4] and c != g]
            if sibs:
                return rng.choice(sibs)
        return None

    def code_for(self, good, rel):
        if rel == "same":
            return good
        return self.other_code(good, rel)

    def outside(self):
        return self.rng.choice(OUTSIDE[self.feature])

    def member(self, exclude=None):
        c = [p for p in PARTIES[self.feature] if p != exclude]
        return self.rng.choice(c)

    def name(self, hs, kind="input"):
        return "%s: %s (HS %s)" % (kind.capitalize(), self.t.chapter_title(hs), dotted(hs))

    def mat(self, hs, value=None, country=None, status="non_originating", weight=None, kind="input", **kw):
        m = {"name": self.name(hs, kind), "hs": dotted(hs) if len(hs) >= 6 else hs, "value": value, "country": country, "status": status, "weight": weight}
        m.update(kw)
        return m

    def fob(self):
        return self.rng.choice(FOBS)

    def producer(self):
        return self.rng.choice(PRODUCERS[self.feature])

    def distractors(self, good, fob, n=None):
        if self.split != "test":
            return []
        rng = self.rng
        n = rng.choice([1, 2]) if n is None else n
        out = []
        for _ in range(n):
            c = rng.choice(self.codes)
            out.append(self.mat(c, round(fob * rng.choice([0.01, 0.02, 0.03]), 2), self.member(), "originating", kind="originating component"))
        return out

    # ---- cases

    def add(self, family, inp, tags, rationale, refs, row=None, parent=None, src_family=None, keys=None, numbers=False, rule=None, mark_formatting=False):
        inp = clean(inp)
        rule = rule or (row.rule() if row is not None else {"alts": []})
        exp = evaluate(self.feature, inp, rule)
        e = {"outcome": exp["outcome"]}
        if row is not None and exp["outcome"] != "no" or exp.get("alternative"):
            pass
        want = keys or {"alternatives", "applied", "accumulation"}
        if "alternatives" in want and exp.get("alternative"):
            plain = {k: v for k, v in exp["alternative"].items() if k in ("RVC40", "RVC45", "RVC35", "RVC60", "RVC70", "CC", "CTH", "CTSH", "WO") and v != "undetermined"}
            if plain:
                e["alternative"] = plain
        if "applied" in want and exp.get("applied"):
            e["applied"] = exp["applied"]
        if "accumulation" in want and "accumulation" in exp:
            e["accumulation"] = exp["accumulation"]
        if "de_minimis_used" in want and "de_minimis_used" in exp:
            e["de_minimis_used"] = exp["de_minimis_used"]
        if numbers:
            if exp.get("rvc_down") is not None:
                e["rvc_down_min"] = round(exp["rvc_down"] - 0.05, 3)
                e["rvc_down_max"] = round(exp["rvc_down"] + 0.05, 3)
            if exp.get("rvc_up") is not None:
                e["rvc_up_min"] = round(exp["rvc_up"] - 0.05, 3)
                e["rvc_up_max"] = round(exp["rvc_up"] + 0.05, 3)
        n = self.seq.get(family, 0) + 1
        self.seq[family] = n
        cid = "%s-%s-%s-%02d" % (self.feature, self.split, family, n)
        norm = json.dumps(normalize(self.feature, inp), sort_keys=True)
        if norm in self.keys:
            return None
        self.keys.add(norm)
        case = {
            "id": cid,
            "split": self.split,
            "feature": self.feature,
            "source_family_id": src_family or family,
            "scenario_family": family,
            "variation_tags": sorted(set(tags)),
            "sources": refs,
            "rule": ({"row": row.rowcode, "text": row.concise, "listed": row.listed} if row is not None else None),
            "rationale": rationale,
            "input": inp,
            "expect": clean(e),
            "expected_abstention": exp["outcome"] == "undetermined",
        }
        if parent:
            case["parent_id"] = parent
        if mark_formatting:
            case["formatting_only"] = True
        self.cases.append(case)
        return case


def normalize(feature, inp):
    n = json.loads(json.dumps(inp))
    for m in n.get("materials", []):
        m.pop("name", None)
        m["hs"] = digits(m.get("hs", ""))
        for k in ("country", "status"):
            if k in m:
                m[k] = m[k].strip().lower()
    n["hs"] = digits(n["hs"])
    n["produced_in"] = n.get("produced_in", "").strip().lower()
    n["materials"] = sorted(n.get("materials", []), key=lambda m: json.dumps(m, sort_keys=True))
    return {"feature": feature, "input": n}


# ----------------------------------------------------------------- references

CITES = {
    "wo": {"atiga": ("atiga-text", "Art. 26(a), Art. 27"), "akfta": ("akfta-annex3", "Rule 2.1(a), Rule 3"), "acfta": ("acfta-annex1", "Art. 2(a), Art. 3"), "rcep": ("rcep-ch3", "Art. 3.2(a), Art. 3.3")},
    "pe": {"atiga": ("atiga-text", "Art. 26(b), Art. 30.1"), "akfta": ("akfta-annex3", "Rule 2, Rule 7"), "acfta": ("acfta-annex1", "Art. 2(b)"), "rcep": ("rcep-ch3", "Art. 3.2(b)")},
    "party": {"atiga": ("atiga-text", "Art. 26 (good of the exporting Member State)"), "akfta": ("akfta-annex3", "Rule 2"), "acfta": ("acfta-annex1", "Art. 2"), "rcep": ("rcep-ch3", "Art. 3.2")},
    "rvc": {"atiga": ("atiga-text", "Art. 28.2(b), Art. 29 (build-down)"), "akfta": ("akfta-annex3", "Rule 4.2, Rule 5"), "acfta": ("acfta-annex1", "Art. 5 (build-down)"), "rcep": ("rcep-ch3", "Art. 3.5")},
    "ctc": {"atiga": ("atiga-text", "Art. 28.2(d): tariff-shift rules apply to non-originating materials only"), "akfta": ("akfta-annex3", "Rule 5, Rule 7"), "acfta": ("acfta-annex1", "Art. 4"), "rcep": ("rcep-ch3", "Art. 3.2(c), Annex 3A")},
    "dm": {"atiga": ("atiga-text", "Art. 33 (10 % of FOB value)"), "akfta": ("akfta-annex3", "Rule 10 (10 % of FOB value; weight for chapters 50 to 63)"), "acfta": ("acfta-annex1", "Art. 9 (10 % of FOB value; weight or value for chapters 50 to 63)"), "rcep": ("rcep-ch3", "Art. 3.7")},
    "cum": {"atiga": ("atiga-text", "Art. 30.1"), "akfta": ("akfta-annex3", "Rule 7"), "acfta": ("acfta-annex1", "Art. 6"), "rcep": ("rcep-ch3", "Art. 3.4")},
    "minimal": {"atiga": ("atiga-text", "Art. 31"), "akfta": ("akfta-annex3", "Rule 8"), "acfta": ("acfta-annex1", "Art. 7 (wholly obtained goods only)"), "rcep": ("rcep-ch3", "Art. 3.6")},
    "unknown": {"atiga": ("atiga-handbook", "p. 6: materials of undetermined origin count as non-originating; Art. 29.2(b)"), "akfta": ("akfta-annex3", "Rule 4: only originating materials count"), "acfta": ("acfta-annex1", "Art. 5"), "rcep": ("rcep-ch3", "Art. 3.5.4")},
    "general": {"atiga": ("atiga-text", "Art. 28.1"), "akfta": ("akfta-annex3", "Rule 4.1"), "acfta": ("acfta-annex1", "Art. 4(1)"), "rcep": ("rcep-ch3", "Art. 3.2(c)")},
}
PSR = {"atiga": "atiga-annex3", "akfta": "akfta-appendix2", "acfta": "acfta-attb", "rcep": "rcep-annex3a"}


def refs(feature, topic, row=None, extra=None):
    out = []
    s, loc = CITES[topic][feature]
    out.append({"source": s, "locator": loc})
    if row is not None:
        out.append({"source": PSR[feature], "locator": ("row %s: %s" % (row.rowcode, row.concise)) if row.listed else "no row for %s; general rule" % dotted(row.code6)})
    for e in extra or []:
        out.append(e)
    return out


def money(x):
    return ("%g" % round(x, 2))


# ----------------------------------------------------------------- families


def fam_wo(c, n_wo, n_nonwo, n_pe, n_none):
    f = c.feature
    for i in range(n_wo):
        row = c.pick_row(["wo"], family="wo")
        fob = c.fob()
        mats = [c.mat(row.code6, round(fob * 0.3, 2), c.outside(), "non_originating", kind="imported input")] if i % 2 == 0 else []
        inp = {"hs": dotted(row.code6), "produced_in": c.producer(), "fob": fob, "wholly_obtained": True, "materials": mats}
        c.add("wo-statement", inp, ["wholly-obtained", "stated", "wo-rule-row"], "Annex rule for %s is WO and the producer states the good is wholly obtained in the exporting Party, so it is originating on that basis." % dotted(row.code6), refs(f, "wo", row), row=row)
    for i in range(n_nonwo):
        row = c.pick_row(["wo"], family="wo")
        fob = c.fob()
        hs = c.other_code(row.code6, "chapter")
        mats = [c.mat(hs, round(fob * 0.35, 2), c.outside(), "non_originating", kind="imported input")]
        inp = {"hs": dotted(row.code6), "produced_in": c.producer(), "fob": fob, "materials": mats}
        c.add("wo-not-met", inp, ["wo-rule-row", "non-originating-material", "negative"], "Row %s requires the good to be wholly obtained; a non-originating imported material is used and no wholly obtained statement is made, so the rule is not met." % dotted(row.code6), refs(f, "wo", row), row=row)
    if not FEATURE_PE[f]:
        for i in range(n_pe):
            row = c.pick_row(["ctc", "ctc_rvc"], family="pe", pred=lambda r: all(not q[2] for _, rq in r.alts for q in rq if q[0] == "ctc"))
            fob = c.fob()
            mats = [c.mat(c.rng.choice(c.codes), round(fob * 0.2, 2), c.member(), "originating", kind="originating input") for _ in range(c.rng.choice([1, 2]))]
            inp = {"hs": dotted(row.code6), "produced_in": c.producer(), "fob": fob, "materials": mats}
            c.add("all-originating", inp, ["produced-from-originating-materials", "cumulation-possible", "positive", "no-separate-pe-basis"], "Every material is originating, so no non-originating material can fail the tariff shift or lower the regional content; the product rule is met on the facts given.", refs(f, "pe", row), row=row, keys={"accumulation"})
    if FEATURE_PE[f]:
        for i in range(n_pe):
            row = c.pick_row(["wo", "ctc", "ctc_rvc"], family="pe")
            fob = c.fob()
            mats = []
            producer = c.producer()
            for _ in range(c.rng.choice([1, 2])):
                hs = c.rng.choice(c.codes)
                mats.append(c.mat(hs, round(fob * 0.2, 2), c.member(), "originating", kind="originating input"))
            inp = {"hs": dotted(row.code6), "produced_in": producer, "fob": fob, "materials": mats}
            c.add("all-originating", inp, ["produced-from-originating-materials", "cumulation-possible", "positive"], "Every material is declared originating in a Party, so the good is produced only from originating materials and qualifies whatever the product rule says.", refs(f, "pe", row), row=row, keys={"accumulation", "applied"})
    for i in range(n_none):
        row = c.pick_row(["ctc", "ctc_rvc", "rvc"], family="wo")
        inp = {"hs": dotted(row.code6), "produced_in": c.producer(), "fob": c.fob(), "materials": []}
        c.add("no-materials", inp, ["missing-facts", "abstention-expected"], "No materials listed and no wholly obtained statement: the bill of materials is missing, so the origin cannot be decided on the facts given.", refs(f, "unknown", row), row=row)


FEATURE_PE = {"atiga": True, "acfta": True, "rcep": True, "akfta": False}


def fam_party(c, n_out, n_missing, n_alias):
    f = c.feature
    for i in range(n_out):
        row = c.pick_row(["ctc_rvc", "ctc", "rvc"], family="party")
        fob = c.fob()
        inp = {"hs": dotted(row.code6), "produced_in": c.outside(), "fob": fob, "wholly_obtained": i % 2 == 0, "materials": [c.mat(c.other_code(row.code6, "chapter"), round(fob * 0.1, 2), c.outside())]}
        c.add("production-outside-agreement", inp, ["non-member-producer", "membership", "negative"], "The good is made in a country that is not a Party to this agreement, so it cannot be an originating good of a Party, even if it states wholly obtained.", refs(f, "party", row), row=row, keys={"applied"})
    for i in range(n_missing):
        row = c.pick_row(["ctc_rvc", "ctc", "rvc"], family="party")
        fob = c.fob()
        inp = {"hs": dotted(row.code6), "produced_in": "", "fob": fob, "materials": [c.mat(c.other_code(row.code6, "chapter"), round(fob * 0.1, 2), c.outside())]}
        c.add("production-country-missing", inp, ["missing-facts", "abstention-expected"], "No country of production is given, so whether the good is made in a Party cannot be decided.", refs(f, "party", row), row=row)
    aliases = {"Vietnam": "Viet Nam", "Laos": "Lao PDR", "Brunei": "Brunei Darussalam", "South Korea": "Republic of Korea"}
    for i in range(n_alias):
        p = [x for x in PRODUCERS[f] if x in aliases]
        if not p:
            continue
        row = c.pick_row(["ctc", "ctc_rvc"], family="party", pred=lambda r: r.shape in ("ctc", "ctc_rvc"))
        fob = c.fob()
        hs_m = c.other_code(row.code6, "chapter")
        base = {"hs": dotted(row.code6), "produced_in": aliases[c.rng.choice(p)], "fob": fob, "materials": [c.mat(hs_m, round(fob * 0.2, 2), c.outside())]}
        c.add("country-name-alias", base, ["formatting-only", "member-alias", "positive"], "Formatting-only robustness check: the treaty name of a Party is used for the producing country; the good changes tariff classification for every material, so it qualifies when the Party is recognised.", refs(f, "party", row), row=row, keys={"applied"}, mark_formatting=True)


def rvc_thresholds(row):
    return [q[1] for _, reqs in row.alts for q in reqs if q[0] == "rvc"]


def fam_rvc(c, n, authority_note=""):
    f = c.feature
    for i in range(n):
        shapes = ["rvc", "ctc_rvc"]
        row = c.pick_row(shapes, family="rvc", pred=lambda r: all(not q[2] for _, rq in r.alts for q in rq if q[0] == "ctc"))
        t = rvc_thresholds(row)[0]
        fob = c.rng.choice([100, 200, 250, 500, 1000, 2000])
        kind = ["above", "exact", "just-below", "far-below", "above", "just-below"][i % 6]
        pct_nm = {"above": 100 - t - 3, "exact": 100 - t, "just-below": 100 - t + 0.5, "far-below": 100 - t + 18}[kind]
        vnm = round(fob * pct_nm / 100, 2)
        hs_same = c.code_for(row.code6, "same")
        parts = c.rng.choice([1, 2])
        mats = []
        remaining = vnm
        for p in range(parts):
            v = round(remaining / (parts - p), 2) if p < parts - 1 else round(remaining, 2)
            remaining -= v
            code = hs_same if (p == 0) else c.code_for(row.code6, "same")
            mats.append(c.mat(code, v, c.outside(), "non_originating", kind="imported input"))
        orig_v = round(fob * 0.2, 2)
        mats.append(c.mat(c.rng.choice(c.codes), orig_v, c.member(), "originating", kind="originating input"))
        mats.extend(c.distractors(row.code6, fob))
        inp = {"hs": dotted(row.code6), "produced_in": c.producer(), "fob": fob, "materials": mats}
        if f in ("atiga", "akfta"):
            inp["method"] = "indirect"
        if f == "rcep" and i % 3 == 2:
            vom = sum(m["value"] for m in mats if m["status"] == "originating")
            inp["costs"] = {"labour": round(fob * 0.02, 2), "overhead": round(fob * 0.01, 2), "profit": round(fob * 0.01, 2), "other": 0}
        tags = ["rvc-threshold", kind, "threshold-%d" % t, "tariff-shift-blocked"]
        if row.shape == "rvc":
            tags.append("rvc-only-rule")
        rationale = "Row %s needs RVC %d. Non-originating value %s of FOB %s leaves build-down content %s %%; the non-originating materials are in the good's own subheading, so no tariff-shift alternative can help." % (dotted(row.code6), t, money(vnm), money(fob), money((fob - vnm) / fob * 100))
        if "costs" in inp:
            rationale += " Costs are given, so the build-up figure is also below the threshold."
        c.add("rvc-boundary", inp, tags, rationale, refs(f, "rvc", row), row=row, numbers=True)


def shift_levels(row):
    return {"chapter": 2, "heading": 4, "subheading": 6}[[q[1] for _, reqs in row.alts for q in reqs if q[0] == "ctc"][0]]


def fam_shift(c, n):
    f = c.feature
    rels = ["chapter", "heading", "sub", "same"]
    for i in range(n):
        row = c.pick_row(["ctc", "ctc_rvc"], family="shift", pred=lambda r: all(not q[2] for _, rq in r.alts for q in rq if q[0] == "ctc"))
        level = [q[1] for _, reqs in row.alts for q in reqs if q[0] == "ctc"][0]
        rel = rels[i % 4]
        fob = c.rng.choice([100, 200, 400, 1000])
        code = c.code_for(row.code6, rel)
        if code is None:
            code = c.code_for(row.code6, "chapter")
            rel = "chapter"
        v_np = round(fob * (0.7 if row.shape == "ctc_rvc" else 0.25), 2)
        mats = [c.mat(code, v_np, c.outside(), "non_originating", kind="imported input")]
        if i % 2 == 1:
            mats.append(c.mat(c.other_code(row.code6, "chapter"), round(fob * 0.05, 2), c.outside(), "non_originating"))
            mats[-1]["name"] = mats[-1]["name"].replace("input", "input B")
            mats[0]["name"] = mats[0]["name"].replace("input", "input A")
        mats.append(c.mat(row.code6, round(fob * 0.1, 2), c.member(), "originating", kind="originating input in the good's own subheading"))
        mats.extend(c.distractors(row.code6, fob))
        inp = {"hs": dotted(row.code6), "produced_in": c.producer(), "fob": fob, "materials": mats}
        if row.shape == "ctc_rvc" and f in ("atiga", "akfta"):
            inp["method"] = "indirect"
        n_ = {"chapter": 2, "heading": 4, "subheading": 6}[level]
        passes = code[:n_] != row.code6[:n_]
        rationale = "Row %s is decided by a change of %s. The non-originating material is in %s (%s); %s. The originating material in the good's own subheading is disregarded because tariff-shift rules apply to non-originating materials only." % (dotted(row.code6), level, {"chapter": "a different chapter", "heading": "the same chapter but a different heading", "sub": "the same heading but a different subheading", "same": "the same subheading"}[rel], dotted(code), "the shift is met" if passes else "no shift at this level, and the %s %% value share keeps the RVC alternative and de minimis from rescuing it" % money(100 * v_np / fob))
        tags = ["tariff-shift-%s" % level, "material-relation-%s" % rel, "positive" if passes else "negative"]
        if rel in ("heading", "sub") and (level == "chapter" and rel == "heading" or level == "heading" and rel == "sub"):
            tags.append("level-sensitive")
        if i % 2 == 1:
            tags.append("two-failing-candidates")
        c.add("tariff-shift-grid", inp, tags, rationale, refs(f, "ctc", row), row=row)


def fam_short(c, n):
    f = c.feature
    for i in range(n):
        row = c.pick_row(["ctc", "ctc_rvc"], family="short", pred=lambda r: all(not q[2] for _, rq in r.alts for q in rq if q[0] == "ctc") and [q[1] for _, rq in r.alts for q in rq if q[0] == "ctc"][0] != "chapter")
        level = [q[1] for _, reqs in row.alts for q in reqs if q[0] == "ctc"][0]
        n_ = {"heading": 4, "subheading": 6}[level]
        short = c.other_code(row.code6, "chapter")[: n_ - 2]
        mats = [{"name": "Imported input with an incomplete code", "hs": short, "country": c.outside(), "status": "non_originating"}]
        inp = {"hs": dotted(row.code6), "produced_in": c.producer(), "materials": mats}
        if f in ("atiga", "akfta"):
            inp["method"] = "indirect"
        c.add("incomplete-hs", inp, ["missing-facts", "short-material-code", "abstention-expected"], "The material's code has only %d digits but the rule tests a change of %s; with no values given neither the shift nor de minimis nor RVC can be tested, so the answer is undetermined." % (len(short), level), refs(f, "ctc", row), row=row)


def fam_dm(c, n, textile=False):
    f = c.feature
    cases = [(5, True), (9.9, True), (10, True), (10.1, False), (15, False), (8, True), (12, False), (10, True)]
    k = 0
    while k < n:
        pct, ok = cases[k % len(cases)]
        row = c.pick_row(["ctc", "ctc_rvc"], family="dm", pred=lambda r: all(not q[2] for _, rq in r.alts for q in rq if q[0] == "ctc") and is_textile(r.code6) is False)
        level = [q[1] for _, reqs in row.alts for q in reqs if q[0] == "ctc"][0]
        fob = c.rng.choice([100, 200, 400, 1000])
        two = k % 4 == 3
        total = round(fob * pct / 100, 2)
        bad_code = c.code_for(row.code6, "same" if level != "chapter" else "same")
        mats = []
        if two:
            a = round(total / 2, 2)
            mats += [c.mat(bad_code, a, c.outside(), "non_originating", kind="imported input A"), c.mat(bad_code, round(total - a, 2), c.outside(), "non_originating", kind="imported input B")]
        else:
            mats.append(c.mat(bad_code, total, c.outside(), "non_originating", kind="imported input"))
        big = round(fob * 0.7, 2)
        mats.append(c.mat(c.other_code(row.code6, "chapter"), big, c.outside(), "non_originating", kind="imported input that changes classification"))
        mats.extend(c.distractors(row.code6, fob, 1))
        inp = {"hs": dotted(row.code6), "produced_in": c.producer(), "fob": fob, "materials": mats}
        if f in ("atiga", "akfta"):
            inp["method"] = "indirect"
        tags = ["de-minimis", "share-%s" % money(pct), "positive" if ok else "negative", "failing-material-share-boundary" if pct in (9.9, 10, 10.1) else "failing-material-share"]
        if two:
            tags.append("two-failing-materials-summed")
        rationale = "Row %s is a tariff-shift rule. The material(s) that fail the shift total %s of FOB %s (%s %%), %s the 10 %% limit; the other imported material changes classification. %s" % (dotted(row.code6), money(total), money(fob), money(pct), "within" if ok else "over", "The RVC alternative fails because non-originating content is above 70 %% of FOB." if row.shape == "ctc_rvc" else "The rule has no RVC alternative.")
        if c.add("de-minimis-value", inp, tags, rationale, refs(f, "dm", row), row=row, keys={"alternatives", "applied", "de_minimis_used"}) is not None or True:
            k += 1


def fam_dm_incomplete(c, n):
    f = c.feature
    for i in range(n):
        row = c.pick_row(["ctc"] if f in ("akfta",) and False else ["ctc", "ctc_rvc"], family="dmi", pred=lambda r: all(not q[2] for _, rq in r.alts for q in rq if q[0] == "ctc") and not is_textile(r.code6))
        fob = c.rng.choice([100, 400, 1000])
        bad_code = row.code6
        mode = i % 3
        mats = [c.mat(bad_code, None if mode != 2 else round(fob * 0.05, 2), c.outside(), "non_originating"),
                c.mat(c.other_code(row.code6, "chapter"), round(fob * 0.7, 2), c.outside(), "non_originating")]
        inp = {"hs": dotted(row.code6), "produced_in": c.producer(), "materials": mats}
        if mode != 1:
            inp["fob"] = fob
        else:
            mats[0]["value"] = round(fob * 0.05, 2)
        if f in ("atiga", "akfta"):
            inp["method"] = "indirect"
        why = {0: "the failing material has no value", 1: "no FOB value is given", 2: "no good-level weight or value was needed but the material is within the RVC window only if values were complete"}[mode]
        if mode == 2:
            continue
        c.add("de-minimis-incomplete", inp, ["de-minimis", "missing-facts", "abstention-expected"], "The material that fails the tariff shift cannot be measured against the 10 %% limit because %s, so the answer is undetermined rather than no." % why, refs(f, "dm", row), row=row, keys={"alternatives"})


def textile_rows(c, shapes):
    return c.rows(shapes, listed=True, textile=True)


def fam_textile(c, n):
    f = c.feature
    k = 0
    tries = 0
    while k < n and tries < 40:
        tries += 1
        shapes = ["ctc", "ctc_rvc"]
        rows = [r for r in c.rows(shapes, True, True, family='textile') if all(not q[2] for _, rq in r.alts for q in rq if q[0] == "ctc")]
        if not rows:
            raise RuntimeError("no textile rows %s %s" % (f, c.split))
        row = c.rng.choice(rows)
        c.used_rows.add(('textile', row.code6))
        fob = 100
        weight = 100
        pat = ["weight-ok-value-over", "value-ok-weight-over", "both-ok", "both-over"][k % 4]
        w_pct = {"weight-ok-value-over": 8, "value-ok-weight-over": 14, "both-ok": 6, "both-over": 15}[pat]
        v_pct = {"weight-ok-value-over": 14, "value-ok-weight-over": 8, "both-ok": 5, "both-over": 16}[pat]
        bad_code = row.code6
        good_code = c.other_code(row.code6, "chapter")
        mats = [c.mat(bad_code, v_pct, c.outside(), "non_originating", weight=w_pct, kind="imported input"),
                c.mat(good_code, 65, c.outside(), "non_originating", weight=60, kind="imported input that changes classification")]
        inp = {"hs": dotted(row.code6), "produced_in": c.producer(), "fob": fob, "weight": weight, "materials": mats}
        if f in ("atiga", "akfta"):
            inp["method"] = "indirect"
        if f == "atiga":
            inp["processes"] = {"textile_process": False}
        if f == "akfta" and any(p for _, rq in row.alts for q in rq for p in [q] if q[0] == "process"):
            inp["processes"] = {p[1]: False for _, rq in row.alts for p in rq if p[0] == "process"}
        if f == "acfta":
            names = [q[1] for _, rq in row.alts for q in rq if q[0] == "process"]
            if names:
                inp["processes"] = {nm: False for nm in names}
        c.add("textile-de-minimis", inp, ["de-minimis", "textile-chapter", "weight-vs-value", pat], "Chapter %s good. The material that fails the tariff shift is %s %% of the weight and %s %% of the value of the good. %s" % (row.code6[:2], w_pct, v_pct, {"atiga": "ATIGA Art. 33 tests value only.", "akfta": "AKFTA Rule 10(1)(b) tests weight only for chapters 50 to 63.", "acfta": "ACFTA Art. 9(b) accepts weight or value for chapters 50 to 63.", "rcep": "RCEP Art. 3.7 accepts weight or value for chapters 50 to 63."}[f]), refs(f, "dm", row), row=row, keys={"alternatives", "applied", "de_minimis_used"})
        k += 1


def fam_cum(c, n):
    f = c.feature
    for i in range(n):
        kind = ["member-originating", "non-member-declared-originating", "member-nonoriginating-status", "member-originating-rvc", "unknown-status-from-member", "member-originating-chapter-rule"][i % 6]
        row = c.pick_row(["ctc", "ctc_rvc", "rvc"], family="cum", pred=lambda r: all(not q[2] for _, rq in r.alts for q in rq if q[0] == "ctc"))
        fob = c.rng.choice([100, 200, 500, 1000])
        producer = c.producer()
        other_member = c.member(exclude=producer)
        outsider = c.outside()
        t = (rvc_thresholds(row) or [40])[0]
        hs_bad = row.code6
        diff = c.other_code(row.code6, "chapter")
        mats = []
        if kind == "member-originating":
            mats = [c.mat(hs_bad, round(fob * 0.5, 2), other_member, "originating", kind="originating input from another Party"), c.mat(diff, round(fob * 0.15, 2), c.outside(), "non_originating")]
            why = "A material in the good's own subheading comes from another Party and is declared originating, so it counts as originating (cumulation) and does not block the shift or add to non-originating value."
            tag = ["cumulation", "positive", "member-origin"]
        elif kind == "non-member-declared-originating":
            mats = [c.mat(hs_bad, round(fob * 0.5, 2), outsider, "originating", kind="input declared originating but made outside the agreement"), c.mat(diff, round(fob * 0.15, 2), c.outside(), "non_originating")]
            why = "The material is declared originating but is made in %s, which is not a Party; it counts as non-originating, so it fails the shift and its 50 %% value share keeps RVC below the threshold." % outsider
            tag = ["cumulation", "negative", "non-member-origin", "conflicting-evidence"]
        elif kind == "member-nonoriginating-status":
            mats = [c.mat(hs_bad, round(fob * 0.5, 2), other_member, "non_originating", kind="input from another Party that does not qualify"), c.mat(diff, round(fob * 0.15, 2), c.outside(), "non_originating")]
            why = "The material is made in another Party but is not itself originating, so cumulation does not apply and it counts as non-originating."
            tag = ["cumulation", "negative", "member-origin", "conflicting-evidence"]
        elif kind == "member-originating-rvc":
            mats = [c.mat(hs_bad, round(fob * 0.2, 2), outsider, "non_originating"), c.mat(hs_bad, round(fob * 0.45, 2), other_member, "originating", kind="originating input from another Party")]
            why = "Only %s %% of FOB is non-originating once the 45 %% originating input from another Party is counted as originating, which keeps RVC above 40 %%." % money(20)
            tag = ["cumulation", "positive", "member-origin", "rvc-with-cumulation"]
        elif kind == "unknown-status-from-member":
            mats = [c.mat(hs_bad, round(fob * 0.5, 2), other_member, "unknown", kind="input of unknown status from another Party"), c.mat(diff, round(fob * 0.15, 2), c.outside(), "non_originating")]
            why = "Origin of the material from another Party is not established, so it counts as non-originating."
            tag = ["cumulation", "negative", "member-origin", "unknown-status", "conflicting-evidence"]
        else:
            mats = [c.mat(hs_bad, round(fob * 0.4, 2), other_member, "originating", kind="originating input from another Party"), c.mat(diff, round(fob * 0.2, 2), c.outside(), "non_originating")]
            why = "Originating input from another Party counts as originating."
            tag = ["cumulation", "positive", "member-origin"]
        mats.extend(c.distractors(row.code6, fob, 1))
        inp = {"hs": dotted(row.code6), "produced_in": producer, "fob": fob, "materials": mats}
        if f in ("atiga", "akfta"):
            inp["method"] = "indirect"
        c.add("cumulation-membership", inp, tag, "Row %s. %s" % (dotted(row.code6), why), refs(f, "cum", row), row=row, numbers=False)
    # membership edge: partner agreement parties that are not parties here
    for i in range(2):
        other = {"atiga": ["South Korea", "China"], "akfta": ["China", "Japan"], "acfta": ["South Korea", "Japan"], "rcep": ["India", "Taiwan"]}[f][i % 2]
        if other == "Taiwan":
            other = "India"
        row = c.pick_row(["ctc", "ctc_rvc"], family="cum", pred=lambda r: all(not q[2] for _, rq in r.alts for q in rq if q[0] == "ctc"))
        fob = c.rng.choice([100, 400])
        mats = [c.mat(row.code6, round(fob * 0.5, 2), other, "originating", kind="input declared originating in a Party of a different agreement"), c.mat(c.other_code(row.code6, "chapter"), round(fob * 0.1, 2), c.outside(), "non_originating")]
        inp = {"hs": dotted(row.code6), "produced_in": c.producer(), "fob": fob, "materials": mats}
        if f in ("atiga", "akfta"):
            inp["method"] = "indirect"
        c.add("cumulation-membership", inp, ["cumulation", "negative", "non-member-origin", "other-agreement-party"], "Row %s. %s is not a Party to this agreement, so its originating status under another agreement does not count here." % (dotted(row.code6), other), refs(f, "cum", row), row=row)


def fam_minimal(c, n):
    f = c.feature
    from oracle import MINIMAL
    real = ["machining", "weaving", "moulding", "fermenting", "smelting", "distilling"]
    for i in range(n):
        row = c.pick_row(["ctc", "ctc_rvc"], family="minimal", pred=lambda r: all(not q[2] for _, rq in r.alts for q in rq if q[0] == "ctc"))
        fob = c.rng.choice([100, 400, 1000])
        shift_ok = i % 2 == 0
        code = c.other_code(row.code6, "chapter") if shift_ok else row.code6
        mats = [c.mat(code, round(fob * 0.4, 2), c.outside(), "non_originating", kind="imported input")]
        mats.extend(c.distractors(row.code6, fob, 1))
        mode = ["only-minimal", "minimal-plus-real", "real-only", "assembly-vocabulary"][i % 4]
        mins = sorted(MINIMAL[f])
        if mode == "only-minimal":
            ops = [c.rng.choice(mins)]
        elif mode == "minimal-plus-real":
            ops = [c.rng.choice(mins), c.rng.choice(real)]
        elif mode == "real-only":
            ops = [c.rng.choice(real)]
        else:
            ops = ["assembly"]
        inp = {"hs": dotted(row.code6), "produced_in": c.producer(), "fob": fob, "materials": mats, "operations": ops}
        if f in ("atiga", "akfta"):
            inp["method"] = "indirect"
        tags = ["minimal-operations", mode, "shift-met" if shift_ok else "shift-not-met"]
        what = {"only-minimal": "only operations of the minimal kind are stated, which cannot confer origin where the agreement applies the list to origin claims", "minimal-plus-real": "a minimal operation is stated together with real production, so the product rule decides", "real-only": "real production is stated, so the product rule decides", "assembly-vocabulary": "the word assembly is stated; it is a listed minimal operation only where the agreement lists simple assembly"}[mode]
        c.add("minimal-operations", inp, tags, "Operations %s: %s (%s). The tariff shift is %s." % (", ".join(ops), what, CITES["minimal"][f][1], "met" if shift_ok else "not met"), refs(f, "minimal", row), row=row)


def fam_missing(c, n):
    f = c.feature
    for i in range(n):
        kind = ["no-fob", "no-values", "unknown-status-shift", "unknown-status-rvc", "no-hs-on-material", "no-costs-direct"][i % 6]
        if kind in ("no-fob", "no-values", "unknown-status-rvc", "no-costs-direct"):
            shapes = ["rvc", "ctc_rvc"]
        else:
            shapes = ["ctc", "ctc_rvc"]
        if kind == "no-costs-direct" and f not in ("atiga", "akfta"):
            kind = "no-fob"
        row = c.pick_row(shapes, family="missing", pred=lambda r: all(not q[2] for _, rq in r.alts for q in rq if q[0] == "ctc"))
        fob = c.rng.choice([100, 400, 1000])
        if kind == "no-fob":
            mats = [c.mat(row.code6, round(fob * 0.5, 2), c.outside(), "non_originating")]
            inp = {"hs": dotted(row.code6), "produced_in": c.producer(), "materials": mats}
            why = "No FOB value is given, so RVC cannot be computed."
            exp_t = ["missing-facts", "abstention-expected"]
        elif kind == "no-values":
            mats = [c.mat(row.code6, None, c.outside(), "non_originating"), c.mat(c.other_code(row.code6, "chapter"), None, c.outside(), "non_originating")]
            inp = {"hs": dotted(row.code6), "produced_in": c.producer(), "fob": fob, "materials": mats}
            why = "The non-originating materials have no values, so RVC cannot be computed."
            exp_t = ["missing-facts", "abstention-expected"]
        elif kind == "unknown-status-shift":
            mats = [c.mat(c.other_code(row.code6, "chapter"), round(fob * 0.5, 2), c.outside(), "unknown")]
            inp = {"hs": dotted(row.code6), "produced_in": c.producer(), "fob": fob, "materials": mats}
            why = "A material of unknown origin counts as non-originating; its code is in another chapter so the shift is still met."
            exp_t = ["unknown-status", "positive", "conservative-default"]
        elif kind == "unknown-status-rvc":
            mats = [c.mat(row.code6, round(fob * 0.3, 2), c.outside(), "unknown"), c.mat(c.rng.choice(c.codes), round(fob * 0.2, 2), c.member(), "originating")]
            inp = {"hs": dotted(row.code6), "produced_in": c.producer(), "fob": fob, "materials": mats}
            why = "A material of unknown origin counts as non-originating; with 30 % of FOB non-originating the build-down content is 70 %."
            exp_t = ["unknown-status", "positive", "conservative-default"]
        elif kind == "no-hs-on-material":
            mats = [{"name": "Imported input without a classification", "country": c.outside(), "status": "non_originating", "value": round(fob * 0.5, 2)}]
            inp = {"hs": dotted(row.code6), "produced_in": c.producer(), "fob": fob, "materials": mats}
            why = "The material has no HS code, so a tariff shift cannot be tested; its value is over the de minimis limit."
            exp_t = ["missing-facts", "abstention-expected", "missing-material-code"]
        else:
            mats = [c.mat(row.code6, round(fob * 0.7, 2), c.outside(), "non_originating")]
            inp = {"hs": dotted(row.code6), "produced_in": c.producer(), "fob": fob, "materials": mats, "method": "direct"}
            why = "The direct method is chosen but no costs are given, so the build-up figure cannot be computed."
            exp_t = ["missing-facts", "abstention-expected", "direct-method-no-costs"]
        if f in ("atiga", "akfta") and "method" not in inp and kind not in ("no-fob",):
            inp["method"] = "indirect"
        c.add("missing-facts", inp, ["missing-facts"] + exp_t, "Row %s. %s" % (dotted(row.code6), why), refs(f, "unknown", row), row=row)


def fam_except(c, n):
    f = c.feature
    for i in range(n):
        row = c.pick_row(["ctc_except", "ctc_rvc_except"], family="except", pred=lambda r: r.listed)
        q = [q for _, rq in row.alts for q in rq if q[0] == "ctc"][0]
        ex = q[2][0]
        fob = c.rng.choice([100, 200, 500])
        lo, hi = digits(ex["from"]), digits(ex["to"])
        k = {"chapter": 2, "heading": 4, "subheading": 6}[ex["level"]]
        excl = None
        for code in c.codes:
            if lo <= code[:k] <= hi and code[:{"chapter": 2, "heading": 4, "subheading": 6}[q[1]]] != row.code6[:{"chapter": 2, "heading": 4, "subheading": 6}[q[1]]]:
                excl = code
                break
        if excl is None:
            continue
        outside_code = None
        for _ in range(200):
            cand = c.rng.choice(c.codes)
            n_ = {"chapter": 2, "heading": 4, "subheading": 6}[q[1]]
            if cand[:n_] != row.code6[:n_] and not (lo <= cand[:k] <= hi):
                outside_code = cand
                break
        use_excl = i % 2 == 0
        code = excl if use_excl else outside_code
        v = round(fob * (0.7 if len([1 for _, rq in row.alts for x in rq if x[0] == "rvc"]) else 0.3), 2)
        mats = [c.mat(code, v, c.outside(), "non_originating", kind="imported input")]
        inp = {"hs": dotted(row.code6), "produced_in": c.producer(), "fob": fob, "materials": mats}
        if f in ("atiga", "akfta"):
            inp["method"] = "indirect"
        c.add("except-from", inp, ["tariff-shift-exclusion", "excluded-origin-material" if use_excl else "outside-exclusion", "negative" if use_excl else "positive"], "Row %s: the shift rule excludes materials from %s %s. The non-originating material (%s) %s the excluded range, so the shift is %s." % (dotted(row.code6), ex["level"], (ex["from"] if ex["from"] == ex["to"] else ex["from"] + " to " + ex["to"]), dotted(code), "falls in" if use_excl else "is outside", "not met" if use_excl else "met"), refs(f, "ctc", row), row=row, keys={"alternatives", "applied"})


def fam_process(c, n):
    f = c.feature
    if f == "rcep":
        return
    for i in range(n):
        row = c.pick_row(["other"], textile=None, family="process", pred=lambda r: r.alts and any(q[0] == "process" for _, rq in r.alts for q in rq) and all(q[0] in ("rvc", "ctc", "process") for _, rq in r.alts for q in rq) and all(not q[2] for _, rq in r.alts for q in rq if q[0] == "ctc"))
        procs = sorted({q[1] for _, rq in row.alts for q in rq if q[0] == "process"})
        fob = c.rng.choice([100, 200, 400])
        state = ["true", "false", "unstated"][i % 3]
        mats = [c.mat(row.code6, round(fob * 0.8, 2), c.outside(), "non_originating", kind="imported input in the good's own subheading")]
        inp = {"hs": dotted(row.code6), "produced_in": c.producer(), "fob": fob, "materials": mats}
        if f in ("atiga", "akfta"):
            inp["method"] = "indirect"
        if state != "unstated":
            inp["processes"] = {p: state == "true" for p in procs}
        ptxt = {"true": "stated as carried out", "false": "stated as not carried out", "unstated": "not stated"}[state]
        c.add("process-conditions", inp, ["process-rule", "process-" + state, "tariff-shift-blocked", "rvc-fails"], "Row %s has alternatives that depend on a process (%s). RVC (20 %% content) and the tariff shift (material in the good's own subheading) fail, so the answer rests on whether the process is carried out: it is %s." % (dotted(row.code6), ", ".join(procs), ptxt), refs(f, "ctc", row), row=row, keys={"alternatives"})


def fam_direct(c, n):
    f = c.feature
    if f == "acfta":
        return
    for i in range(n):
        row = c.pick_row(["rvc", "ctc_rvc"], family="direct", pred=lambda r: all(not q[2] for _, rq in r.alts for q in rq if q[0] == "ctc"))
        t = rvc_thresholds(row)[0]
        fob = c.rng.choice([100, 200, 500, 1000])
        kind = ["direct-pass-indirect-fail", "both-fail", "direct-just-below", "direct-exact", "direct-fail-indirect-pass"][i % 5]
        orig = round(fob * 0.2, 2)
        vnm = round(fob * (0.1 if kind == "direct-fail-indirect-pass" else 0.65), 2)
        labour = 0
        if kind == "direct-pass-indirect-fail":
            labour = round(fob * (t / 100 - 0.2 + 0.01), 2)
        elif kind in ("both-fail", "direct-fail-indirect-pass"):
            labour = round(fob * 0.05, 2)
        elif kind == "direct-just-below":
            labour = round(fob * (t / 100 - 0.2 - 0.005), 2)
        else:
            labour = round(fob * (t / 100 - 0.2), 2)
        mats = [c.mat(row.code6, vnm, c.outside(), "non_originating", kind="imported input in the good's own subheading"), c.mat(c.rng.choice(c.codes), orig, c.member(), "originating", kind="originating input")]
        inp = {"hs": dotted(row.code6), "produced_in": c.producer(), "fob": fob, "materials": mats, "costs": {"labour": labour, "overhead": 0, "profit": 0, "other": 0}}
        if f in ("atiga", "akfta"):
            inp["method"] = "direct"
        if f == "rcep":
            if kind == "direct-pass-indirect-fail":
                pass
        up = (orig + labour) / fob * 100
        c.add("direct-build-up", inp, ["rvc-direct" if f != "rcep" else "rvc-build-up", kind, "tariff-shift-blocked"], "Row %s needs RVC %d. Originating material %s plus labour %s over FOB %s gives build-up content %s %%; build-down content is %s %%." % (dotted(row.code6), t, money(orig), money(labour), money(fob), money(up), money((fob - vnm) / fob * 100)), refs(f, "rvc", row), row=row, numbers=True)


def fam_general(c, n):
    f = c.feature
    if f == "rcep":
        return
    for i in range(n):
        if f == "acfta":
            want_cth = i % 2 == 0
            row = c.pick_row(["rvc", "ctc_rvc"], listed=False, family="general", pred=lambda r, w=want_cth: (r.shape == "ctc_rvc") == w and not is_textile(r.code6))
        else:
            row = c.pick_row(["ctc_rvc"], listed=False, family="general", pred=lambda r: not is_textile(r.code6))
        fob = c.rng.choice([100, 400, 1000])
        shift_ok = i % 3 != 2
        code = c.other_code(row.code6, "chapter") if shift_ok else row.code6
        mats = [c.mat(code, round(fob * 0.65, 2), c.outside(), "non_originating", kind="imported input")]
        inp = {"hs": dotted(row.code6), "produced_in": c.producer(), "fob": fob, "materials": mats}
        if f in ("atiga", "akfta"):
            inp["method"] = "indirect"
        extra = ""
        if f == "acfta":
            extra = " Art. 4(1)(b) allows a change of tariff heading only for the chapters it lists; chapter %s is %s." % (row.code6[:2], "listed" if row.shape == "ctc_rvc" else "not listed")
        c.add("general-rule", inp, ["goods-not-listed", "general-rule", "shift-met" if shift_ok else "shift-not-met", "rvc-fails"], "No product-specific row applies to %s, so the general rule is used: %s. Non-originating content is 65 %% of FOB, so RVC is 35 %%; a change of heading is %s.%s" % (dotted(row.code6), row.concise, ("not an alternative here" if not any(a[0] == "CTH" for a in row.alts) else ("met" if shift_ok else "not met")), extra), refs(f, "general", row), row=row, keys={"alternatives", "applied"})


def fam_partial(c, n):
    if c.feature != "atiga":
        return
    for i in range(n):
        row = c.pick_row(["rvc", "ctc_rvc"], family="partial", pred=lambda r: all(not q[2] for _, rq in r.alts for q in rq if q[0] == "ctc"))
        fob = 100
        pc = [19.9, 20, 30, 39.9, 40, 15][i % 6]
        mats = [c.mat(row.code6, 70, c.member(), "non_originating", rvc_pct=pc, kind="imported input with partial ASEAN content")]
        inp = {"hs": dotted(row.code6), "produced_in": c.producer(), "fob": fob, "method": "indirect", "materials": mats}
        c.add("partial-cumulation", inp, ["partial-cumulation", "annex-6-window", "threshold-pc-%s" % money(pc)], "Annex 6 counts the ASEAN content of a non-originating material only when its own RVC is at least 20 %% and under 40 %%. At %s %% the material contributes %s of its 70 value, so RVC is %s %%." % (money(pc), money(70 * pc / 100 if 20 <= pc < 40 else 0), money(30 + (70 * pc / 100 if 20 <= pc < 40 else 0))), [{"source": "atiga-annex6", "locator": "Art. 30.2 and Annex 6"}, {"source": PSR["atiga"], "locator": "row %s: %s" % (row.rowcode, row.concise)}], row=row, numbers=True)


def fam_refusal(c, n):
    f = c.feature
    kinds = ["negative-fob", "short-hs", "bad-status", "bad-method"]
    done = 0
    for i in range(n):
        row = c.pick_row(["ctc", "ctc_rvc"], family="refusal")
        base = {"hs": dotted(row.code6), "produced_in": c.producer(), "fob": 100, "materials": [c.mat(c.other_code(row.code6, "chapter"), 20, c.outside(), "non_originating")]}
        k = kinds[i % 4]
        if k == "negative-fob":
            base["fob"] = -50
        elif k == "short-hs":
            base["hs"] = "840"
        elif k == "bad-status":
            base["materials"][0]["status"] = "maybe"
        elif f == "rcep":
            base["active_ingredient_originating_pct"] = 120
            k = "bad-active-ingredient-share"
        else:
            base["method"] = "direct" if f == "acfta" else "hybrid"
        case = c.add("malformed-input", base, ["refusal-expected", k], "A malformed request (%s) must be refused as invalid input rather than answered." % k, refs(f, "unknown", row), row=row)
        if case:
            case["expect"] = {"outcome": "refused"}
            case["expected_abstention"] = False


def fam_chem(c, n):
    if c.feature != "rcep":
        return
    for i in range(n):
        row = c.pick_row(["other"], textile=None, family="chem", pred=lambda r: r.alts and any(a[0] == "CR" for a in r.alts))
        fob = c.rng.choice([100, 200, 500])
        state = ["true", "false", "unstated"][i % 3]
        mats = [c.mat(row.code6, round(fob * 0.75, 2), c.outside(), "non_originating", kind="imported feedstock in the good's own heading")]
        inp = {"hs": dotted(row.code6), "produced_in": c.producer(), "fob": fob, "materials": mats}
        if state != "unstated":
            inp["chemical_reaction"] = state == "true"
        c.add("chemical-reaction", inp, ["chemical-reaction", "chem-" + state, "tariff-shift-blocked", "rvc-fails"], "Row %s offers CR as an alternative. The feedstock is in the good's own heading and is 75 %% of FOB, so CTH and RVC40 fail; the answer rests on whether a chemical reaction took place in a Party, which is %s." % (dotted(row.code6), {"true": "stated", "false": "stated not to have", "unstated": "not stated"}[state]), refs("rcep", "ctc", row), row=row, keys={"alternatives"})


SPECIAL_RCEP = {
    "0304.39": {"text": "1. CTH for Carp 2. CC for any other good", "variants": {"carp": [("CTH", [("ctc", "heading", [])])], "other": [("CC", [("ctc", "chapter", [])])]}, "what": "carp of the listed species or other fish"},
    "1601.00": {"text": "1. CC or RVC40 for of insect 2. CC for any other good", "variants": {"insect": [("CC", [("ctc", "chapter", [])]), ("RVC40", [("rvc", 40)])], "other": [("CC", [("ctc", "chapter", [])])]}, "what": "of insects or other sausages"},
    "1602.10": {"text": "1. CC or RVC40 for of insect 2. CC for any other good", "variants": {"insect": [("CC", [("ctc", "chapter", [])]), ("RVC40", [("rvc", 40)])], "other": [("CC", [("ctc", "chapter", [])])]}, "what": "of insects or other homogenised preparations"},
    "1602.90": {"text": "1. CC or RVC40 for of insect 2. CC for any other good", "variants": {"insect": [("CC", [("ctc", "chapter", [])]), ("RVC40", [("rvc", 40)])], "other": [("CC", [("ctc", "chapter", [])])]}, "what": "of insects or other prepared meat"},
}


def fam_variants(c, n):
    if c.feature != "rcep":
        return
    codes = ["0304.39", "1601.00", "1602.10", "1602.90"]
    for i in range(n):
        code = codes[i % len(codes)]
        sp = SPECIAL_RCEP[code]
        d6 = digits(code)
        fob = c.rng.choice([100, 200, 500])
        kind = ["rvc-only-insect", "chapter-shift", "variant-missing-disagree", "variant-missing-agree"][(i // len(codes)) % 4]
        var_ids = list(sp["variants"])
        chosen = var_ids[i % 2] if kind in ("rvc-only-insect", "chapter-shift") else ""
        if kind == "rvc-only-insect":
            chosen = var_ids[0]
            mats = [c.mat(d6[:2] + "0000" if False else c.other_code(d6, "heading") or d6, round(fob * 0.4, 2), c.outside(), "non_originating")]
        elif kind == "chapter-shift":
            mats = [c.mat(c.other_code(d6, "chapter"), round(fob * 0.5, 2), c.outside(), "non_originating")]
        elif kind == "variant-missing-disagree":
            mats = [c.mat(c.other_code(d6, "heading"), round(fob * 0.4, 2), c.outside(), "non_originating")]
        else:
            mats = [c.mat(c.other_code(d6, "chapter"), round(fob * 0.4, 2), c.outside(), "non_originating")]
        inp = {"hs": code, "produced_in": c.producer(), "fob": fob, "materials": mats}
        if chosen:
            inp["variant"] = chosen
        if kind in ("variant-missing-disagree", "variant-missing-agree"):
            outs = {v: evaluate("rcep", clean(inp), {"alts": a})["outcome"] for v, a in sp["variants"].items()}
            outcome = outs[var_ids[0]] if len(set(outs.values())) == 1 else "undetermined"
            alts = sp["variants"][chosen] if chosen else None
        else:
            outcome = evaluate("rcep", clean(inp), {"alts": sp["variants"][chosen]})["outcome"]
        case = c.add("product-variant", inp, ["product-dependent-rule", "variant-" + (chosen or "unstated"), kind], "Annex 3A row %s: %s. %s" % (code, sp["text"], "The variant is given." if chosen else "No variant is given: when the branches give different answers the outcome is undetermined, and when every branch agrees that answer stands."), refs("rcep", "ctc", None, [{"source": "rcep-annex3a", "locator": "row %s: %s" % (code, sp["text"])}]), rule={"alts": sp["variants"][chosen] if chosen else sp["variants"][var_ids[0]]}, keys=set())
        if case:
            case["expect"] = {"outcome": outcome}
            case["rule"] = {"row": code, "text": sp["text"], "listed": True}
            case["expected_abstention"] = outcome == "undetermined"


def fam_active(c, n):
    if c.feature != "rcep":
        return
    rule = {"alts": [("CTSH", [("ctc", "subheading", []), ("ai",)]), ("RVC40", [("rvc", 40)])]}
    for i in range(n):
        code = c.rng.choice([x for x in c.codes if x.startswith("3808")])
        fob = c.rng.choice([100, 200])
        pct = [60, 50, 49.9, None, 20][i % 5]
        mats = [c.mat(c.other_code(code, "chapter"), round(fob * 0.7, 2), c.outside(), "non_originating", kind="imported active ingredient")]
        inp = {"hs": dotted(code), "produced_in": c.producer(), "fob": fob, "materials": mats}
        if pct is not None:
            inp["active_ingredient_originating_pct"] = pct
        case = c.add("active-ingredient", inp, ["condition-in-words", "heading-3808", "ai-" + ("unstated" if pct is None else money(pct))], "Heading 38.08: CTSH provided at least 50 %% by weight of the active ingredients is originating, or RVC40. The imported ingredient changes subheading but is 70 %% of FOB (RVC 30 %%), so the answer rests on the stated originating share of active ingredients (%s)." % ("not stated" if pct is None else money(pct) + " %"), refs("rcep", "ctc", None, [{"source": "rcep-annex3a", "locator": "heading 38.08: CTSH provided that at least 50 % by weight of the active ingredient or ingredients is originating, or RVC40"}]), rule=rule, keys={"alternatives"})
        if case:
            case["expect"]["alternative"] = {"RVC40": case["expect"]["alternative"]["RVC40"]}
            case["rule"] = {"row": "38.08", "text": "CTSH provided that at least 50 % by weight of the active ingredient or ingredients is originating, or RVC40", "listed": True}


def mutate_copy(inp):
    return json.loads(json.dumps(inp))


def fam_authority(c, parents, parent_inputs, meta):
    f = c.feature
    for pid in parents:
        base = parent_inputs[pid]
        row = c.t.resolve(f, digits(base["hs"])[:6])
        if row is None or row.alts is None:
            continue
        thresholds = rvc_thresholds(row)
        variants = []
        mats = base.get("materials", [])
        nonorig = [i for i, m in enumerate(mats) if m.get("status") != "originating"]
        orig = [i for i, m in enumerate(mats) if m.get("status") == "originating"]
        fob = base.get("fob")
        out = c.rng
        if orig:
            v = mutate_copy(base)
            v["materials"][orig[0]]["status"] = "non_originating"
            variants.append(("originating-material-loses-status", v, ["status-flipped", "conflicting-evidence"]))
            v = mutate_copy(base)
            v["materials"][orig[0]]["country"] = c.outside()
            variants.append(("origin-country-outside-agreement", v, ["origin-changed", "non-member-origin", "conflicting-evidence"]))
        if fob and thresholds and nonorig and all(mats[i].get("value") for i in nonorig):
            vnm = sum(mats[i]["value"] for i in nonorig)
            t = thresholds[0]
            for lbl, tgt in (("just-below-threshold", t - 0.5), ("at-threshold", t)):
                target_vnm = fob * (100 - tgt) / 100
                k = target_vnm / vnm
                v = mutate_copy(base)
                for i in nonorig:
                    v["materials"][i]["value"] = round(mats[i]["value"] * k, 2)
                if abs(sum(v["materials"][i]["value"] for i in nonorig) - target_vnm) > 0.5:
                    continue
                variants.append(("values-rescaled-" + lbl, v, ["values-perturbed", lbl]))
        if fob and nonorig:
            v = mutate_copy(base)
            v["fob"] = round(fob * 0.6, 2)
            variants.append(("fob-lower", v, ["values-perturbed", "fob-changed"]))
        if nonorig:
            v = mutate_copy(base)
            v["materials"][nonorig[0]]["status"] = "unknown"
            v["materials"][nonorig[0]].pop("country", None)
            variants.append(("origin-unknown", v, ["status-unknown"]))
        if base.get("wholly_obtained"):
            v = mutate_copy(base)
            v.pop("wholly_obtained")
            v["materials"] = [{"name": "Imported input", "hs": c.other_code(digits(base["hs"])[:6], "chapter") or "0101.21", "value": 40, "country": c.outside(), "status": "non_originating"}]
            v["fob"] = 100
            variants.append(("wholly-obtained-statement-removed", v, ["status-flipped"]))
        out.shuffle(variants)
        for lbl, inp, tags in variants[: meta.get("per_parent", 3)]:
            if f in ("atiga", "akfta") and "method" not in inp and any(q[0] == "rvc" for _, rq in row.alts for q in rq):
                inp["method"] = "indirect"
            inp.pop("weight", None) if False else None
            ev = evaluate(f, clean(inp), row.rule())
            detail = " Build-down content is %s %%." % money(ev["rvc_down"]) if ev.get("rvc_down") is not None else ""
            c.add("authority-variants", inp, ["parent-variant", lbl] + tags, "Variation of the published worked example %s (%s): %s, under row %s (%s).%s The rule row and arithmetic are those of the cited source; the changed facts decide the label." % (pid, meta["where"][pid], lbl.replace("-", " "), row.rowcode, row.concise, detail), [{"source": meta["source"][pid], "locator": meta["where"][pid]}] + refs(f, "rvc", row)[1:], row=row, parent="eval/%s/reference-cases.json#%s" % (meta["dir"], pid) if meta["dir"] else meta["parent_ref"][pid], src_family="authority-" + pid, numbers=True)


def rcep_parents():
    def m(name, hs, v=None, country="United States", status="non_originating", **kw):
        d = {"name": name, "hs": hs, "country": country, "status": status}
        if v is not None:
            d["value"] = v
        d.update(kw)
        return d
    P = {
        "p26-wheat-flour": {"hs": "1101.00", "produced_in": "Malaysia", "fob": 100, "materials": [m("wheat", "1001.99", 70)]},
        "p27-coffee": {"hs": "0901.21", "produced_in": "Vietnam", "fob": 100, "materials": [m("green coffee", "0901.11", 70)]},
        "p29-juice": {"hs": "2009.90", "produced_in": "Viet Nam", "fob": 100, "materials": [m("oranges", "0805.10", 30), m("limes", "0805.50", 10), m("bananas", "0803", 20, "Australia", "originating"), m("mangoes", "0804", 20, "Australia", "originating")]},
        "p26-maize-flour": {"hs": "1102.20", "produced_in": "Thailand", "fob": 100, "materials": [m("corn", "1005.90", 60)]},
        "p32-build-up": {"hs": "0901.21", "produced_in": "China", "fob": 300, "costs": {"labour": 20, "overhead": 20, "profit": 10, "other": 0}, "materials": [m("originating materials", "0901.11", 85, "China", "originating"), m("other materials", "0901.11", 165)]},
        "p32-build-down": {"hs": "0901.21", "produced_in": "Malaysia", "fob": 100, "materials": [m("non-originating materials", "0901.11", 50)]},
        "p33-benzene": {"hs": "2902.20", "produced_in": "Japan", "fob": 100, "chemical_reaction": False, "materials": [m("toluene", "2902.30", 70), m("hydrogen", "2804.10", 5)]},
        "p37-leather-jacket": {"hs": "4203.10", "produced_in": "Vietnam", "fob": 100, "materials": [m("tanned sheep leather", "4112.00", 50, "New Zealand", "originating"), m("buttons", "9606.21", 5, "China", "originating"), m("zippers", "9607.11", 5, "China", "originating")]},
        "p41-spice-cinnamon": {"hs": "0910.91", "produced_in": "Malaysia", "fob": 18, "costs": {"labour": 3.5, "overhead": 0, "profit": 2.5, "other": 1}, "materials": [m("vanilla pods", "0905.10", 5, "Madagascar"), m("ground cinnamon", "0906.20", 6, "Vietnam")]},
        "p42-spice-malaysia": {"hs": "0910.91", "produced_in": "Malaysia", "fob": 18, "costs": {"labour": 4, "overhead": 0, "profit": 2.7, "other": 1.3}, "materials": [m("vanilla pods", "0905.10", 5, "Madagascar"), m("cinnamon bark", "0906.11", 5, "Sri Lanka")]},
        "p46-de-minimis": {"hs": "8471.30", "produced_in": "Singapore", "fob": 100, "materials": [m("material A", "8542.31", 60), m("material B", "8471.50", 5)]},
    }
    where = {"p26-wheat-flour": "p. 26", "p27-coffee": "p. 27", "p29-juice": "p. 29", "p26-maize-flour": "p. 26", "p32-build-up": "p. 32", "p32-build-down": "p. 32", "p33-benzene": "p. 33", "p37-leather-jacket": "p. 37", "p41-spice-cinnamon": "p. 41", "p42-spice-malaysia": "p. 42", "p46-de-minimis": "p. 46"}
    return P, {"dir": None, "where": where, "source": {k: "abf-rcep-guide" for k in P}, "parent_ref": {k: "backend/internal/rcep/engine_test.go#TestABFWorkedExamples/" + k for k in P}, "per_parent": 2}


def fam_heading(c, n):
    f = c.feature
    heads = sorted({code[:4] for code in c.codes if (int(code[:2]) % 2 == 1) == (c.split == "dev") and not is_textile(code)})
    c.rng.shuffle(heads)
    made = 0
    for h in heads:
        if made >= n:
            break
        subs = [code for code in c.codes if code.startswith(h)]
        rows = [c.t.resolve(f, s) for s in subs]
        if len(subs) < 2 or any(r is None or r.alts is None for r in rows):
            continue
        fob = c.rng.choice([100, 200, 400])
        mats = [c.mat(c.other_code(subs[0], "chapter"), round(fob * 0.3, 2), c.outside(), "non_originating", kind="imported input")]
        inp = {"hs": h, "produced_in": c.producer(), "fob": fob, "materials": mats}
        if f in ("atiga", "akfta"):
            inp["method"] = "indirect"
        outs = {evaluate(f, clean(inp), r.rule())["outcome"] for r in rows}
        sigs = {json.dumps(r.alts, sort_keys=True, default=str) for r in rows}
        agree = len(outs) == 1
        outcome = outs.pop() if agree else "undetermined"
        if len(sigs) == 1:
            kind = "one-rule-for-every-subheading"
        elif agree:
            kind = "rules-differ-answer-agrees"
        else:
            kind = "rules-differ-answer-differs"
        case = c.add("heading-only-code", inp, ["good-code-too-short", "heading-code-only", kind] + (["abstention-expected"] if outcome == "undetermined" else []), "The good is identified only by its four-digit heading %s, which covers %d subheadings under %d distinct product rules. The materials change chapter, so %s." % (dotted(h), len(subs), len(sigs), "every subheading's rule gives the same answer" if agree else "the subheading rules would give different answers and the subheading is needed"), refs(f, "ctc", None, [{"source": PSR[f], "locator": "rows under heading %s" % dotted(h)}]), keys=set(), rule={"alts": []})
        if case:
            case["expect"] = {"outcome": outcome}
            case["expected_abstention"] = outcome == "undetermined"
            case["rule"] = None
            made += 1


def fam_conditions(c, n):
    if c.feature != "akfta":
        return
    for i in range(n):
        kind = ["wo-any-originating-wo", "wo-any-not-wo", "wo-any-nonoriginating", "wo-any-unstated", "value-cap-within", "value-cap-over"][i % 6]
        want_cap = kind.startswith("value-cap")
        row = c.pick_row(["other"], textile=None, family="conditions", pred=lambda r, w=want_cap: r.alts and all(q[0] in ("rvc", "ctc", "materials", "value_cap") for _, rq in r.alts for q in rq) and any(q[0] == ("value_cap" if w else "materials") for _, rq in r.alts for q in rq) and all(not q[2] for _, rq in r.alts for q in rq if q[0] == "ctc") and not is_textile(r.code6))
        fob = c.rng.choice([100, 200, 400])
        scope = [q[1] for _, rq in row.alts for q in rq if q[0] in ("materials", "value_cap")][0]
        cap = [q[2] for _, rq in row.alts for q in rq if q[0] == "value_cap"]
        g0 = scope[0]
        k = {"chapter": 2, "heading": 4, "subheading": 6}[g0["level"]]
        lo = digits(g0["from"])
        in_scope = next((code for code in c.codes if code[:k] == lo.zfill(k) and code[:4] != row.code6[:4] and code[:2] != row.code6[:2]), None) or next((code for code in c.codes if code[:k] == lo.zfill(k) and code[:2] != row.code6[:2]), None)
        if in_scope is None:
            continue
        mats = [c.mat(in_scope, round(fob * 0.2, 2), c.member() if kind in ("wo-any-originating-wo", "wo-any-not-wo", "wo-any-unstated") else c.outside(), "originating" if kind in ("wo-any-originating-wo", "wo-any-not-wo", "wo-any-unstated") else "non_originating", kind="input in the conditioned chapter")]
        if kind == "wo-any-originating-wo":
            mats[0]["wholly_obtained"] = True
        elif kind == "wo-any-not-wo":
            mats[0]["wholly_obtained"] = False
        if want_cap:
            share = 0.5 if kind == "value-cap-within" else 0.7
            mats[0]["value"] = round(fob * share, 2)
        mats.append(c.mat(c.other_code(row.code6, "chapter") if not want_cap else c.other_code(row.code6, "chapter"), round(fob * 0.1, 2), c.outside(), "non_originating", kind="imported input that changes classification"))
        inp = {"hs": dotted(row.code6), "produced_in": c.producer(), "fob": fob, "materials": mats, "method": "indirect"}
        if want_cap:
            note = "non-originating materials of the capped heading are %s %% of FOB against a %s %% cap" % (money(share * 100), money(cap[0]))
        else:
            note = {"wo-any-originating-wo": "the in-scope material is originating and stated wholly obtained", "wo-any-not-wo": "the in-scope material is originating but stated not wholly obtained", "wo-any-nonoriginating": "the in-scope material is non-originating so it cannot be wholly obtained in a Party", "wo-any-unstated": "whether the in-scope material is wholly obtained is not stated"}[kind]
        c.add("material-conditions", inp, ["proviso-on-materials", kind], "Row %s adds a proviso on materials to its rule: %s. The other imported material changes classification." % (dotted(row.code6), note), refs("akfta", "ctc", row), row=row, keys={"alternatives"})


FAMILY_PLAN = {
    "dev": dict(wo=(2, 2, 2, 1), party=(2, 1, 1), rvc=8, shift=12, short=2, dm=6, dmi=2, textile=3, cum=8, minimal=6, missing=6, exc=3, proc=3, direct=3, heading=2, cond=4, general=4, partial=3, refusal=4, chem=3, variants=4, active=3, per_parent=3),
    "test": dict(wo=(4, 4, 4, 2), party=(4, 2, 3), rvc=14, shift=24, short=4, dm=10, dmi=4, textile=8, cum=12, minimal=8, missing=10, exc=6, proc=6, direct=6, heading=5, cond=8, general=6, partial=6, refusal=4, chem=6, variants=8, active=5, per_parent=3),
}


TEST_ONLY = {
    "atiga": ["textile", "partial"],
    "akfta": ["textile", "dmi"],
    "acfta": ["textile", "general"],
    "rcep": ["textile", "chem", "variants", "active"],
}


def parent_inputs(feature):
    if feature == "rcep":
        return rcep_parents()
    d = json.load(open(os.path.join(ROOT_REPO, "eval", feature, "reference-cases.json")))
    inputs = {c["id"]: c["input"] for c in d["cases"]}
    meta = {"dir": feature, "where": {c["id"]: c["where"] for c in d["cases"]}, "source": {c["id"]: sources.PARENT_SOURCE[feature][c["source"]] for c in d["cases"]}, "per_parent": 3}
    return inputs, meta


ROOT_REPO = os.path.abspath(os.path.join(HERE, "..", "..", ".."))


def build_feature(tables, feature, split, pool):
    c = Ctx(tables, feature, split, pool)
    p = dict(FAMILY_PLAN[split])
    if split == "dev":
        for k in TEST_ONLY[feature]:
            p[k] = 0
    fam_wo(c, *p["wo"])
    fam_party(c, *p["party"])
    fam_rvc(c, p["rvc"])
    fam_shift(c, p["shift"])
    fam_short(c, p["short"])
    fam_dm(c, p["dm"])
    fam_dm_incomplete(c, p["dmi"])
    fam_textile(c, p["textile"])
    fam_cum(c, p["cum"])
    fam_minimal(c, p["minimal"])
    fam_missing(c, p["missing"])
    fam_except(c, p["exc"])
    fam_process(c, p["proc"])
    fam_chem(c, p["chem"])
    fam_variants(c, p["variants"])
    fam_active(c, p["active"])
    fam_direct(c, p["direct"])
    fam_heading(c, p["heading"])
    fam_conditions(c, p["cond"])
    fam_general(c, p["general"])
    fam_partial(c, p["partial"])
    inputs, meta = parent_inputs(feature)
    meta = dict(meta)
    meta["per_parent"] = p["per_parent"]
    parents = (AUTHORITY_DEV if split == "dev" else AUTHORITY_TEST)[feature]
    fam_authority(c, parents, inputs, meta)
    fam_refusal(c, p["refusal"])
    return c


ABOUT = ("Source-grounded synthetic challenge cases for the %s check. Expected labels were derived by oracle.py from the cited primary rule rows "
         "and treaty clauses, not by running the candidate engine. Cases were authored by the implementer with knowledge of the engine interface; "
         "this is not blind authoring and not independent real-world customs rulings.")


def write(feature, split, c):
    used = {}
    srcs = sources.SOURCES[feature]
    for cs in c.cases:
        for r in cs["sources"]:
            if r["source"] in srcs:
                used[r["source"]] = srcs[r["source"]]
    doc = {
        "schema_version": 1,
        "dataset_version": "fta-challenge-v1",
        "evidence_kind": "source_grounded_synthetic",
        "feature": feature,
        "split": split,
        "about": ABOUT % feature.upper(),
        "label_method": "oracle.py over the cited rule rows; hand-specified rows are marked in the case rule field",
        "case_count": len(c.cases),
        "sources": used,
        "cases": c.cases,
    }
    path = os.path.join(HERE, "%s-%s.json" % (feature, split))
    with open(path, "w") as fh:
        json.dump(doc, fh, indent=1, ensure_ascii=False)
        fh.write("\n")
    return path


def main():
    tables = Tables()
    for f in FEATURES:
        pool = tables.pool(f)
        for split in ("dev", "test"):
            c = build_feature(tables, f, split, pool)
            path = write(f, split, c)
            print(f, split, len(c.cases), os.path.basename(path))


if __name__ == "__main__":
    main()
