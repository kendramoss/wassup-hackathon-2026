#!/usr/bin/env python3
"""Deterministic generator for the legal-rules challenge datasets.

    python3 eval/challenges/legal-rules/build_cases.py

Writes sources.json and <feature>-<split>.jsonl next to this file. Expected
labels come from the oracle_* modules and cases_tariff.py, which share no
code with the engines under test. Nothing here calls the candidate.
"""
import hashlib
import json
import os
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
REPO = os.path.abspath(os.path.join(HERE, "..", "..", ".."))

import cases_revise as RV  # noqa: E402
import cases_tariff as TW  # noqa: E402
import cases_textile as TX  # noqa: E402
import cases_usmca as US  # noqa: E402

SOURCES = [
    ("gn11o", "HTSUS Revision 23 (2026) General Note 11(o), product-specific rules (USITC, public domain)", "eval/usmca/gn11o.txt", "primary_text_extract"),
    ("gn11-general", "HTSUS Revision 23 (2026) General Note 11(c), (e), (n), (p) pages (USITC, public domain)", "eval/challenges/legal-rules/sources/gn11-general-excerpts.txt", "primary_text_extract"),
    ("usmca-doc", "Scope statement of the candidate: what the USMCA check models and declines (documents the contract cases test; it is not law)", "backend/internal/usmca/doc.go", "candidate_documentation"),
    ("ecfr-102-21", "19 CFR 102.21, eCFR point-in-time 18 Nov 2026 (public domain)", "backend/internal/textilerules/source/ecfr-102.21.xml", "primary_text"),
    ("fr-87-68356", "87 FR 68356 (CBP Dec. 22-25), 6201-6208 entry", "backend/internal/textilerules/source/fr-87-68356-6201-6208.txt", "primary_text"),
    ("textile-doc", "Scope statement of the candidate (documents abstentions; not law)", "backend/internal/textilerules/doc.go", "candidate_documentation"),
    ("additional-duty", "Embedded additional-duty data derived from HTS Chapter 99 (Section 301 lists and exclusions)", "backend/internal/duty/additional.json", "embedded_derivative"),
    ("ch99-rev23", "USITC HTS 2026 Revision 23 Chapter 99 excerpt (9903.88.69/70, 9903.91.12-.16)", "backend/internal/tariffwatch/testdata/ch99_rev23_excerpt.txt", "primary_text_extract"),
    ("ch99-rev22", "USITC HTS 2026 Revision 22 Chapter 99 excerpt", "backend/internal/tariffwatch/testdata/ch99_rev22_excerpt.txt", "primary_text_extract"),
    ("ch99-rev21", "USITC HTS 2026 Revision 21 Chapter 99 excerpt", "backend/internal/tariffwatch/testdata/ch99_rev21_excerpt.txt", "primary_text_extract"),
    ("fr-90-48320", "90 FR 48320 Annex V.A (maritime action lines, 9903.91.12 and 9903.91.14)", "eval/tariffwatch/fr-90FR48320-annexVA.txt", "primary_text_extract"),
    ("fr-89-76581", "89 FR 76581 Annex B (solar exclusions)", "eval/tariffwatch/fr-89FR76581-annexB.txt", "primary_text_extract"),
    ("tariffwatch-doc", "Scope statement of the candidate (what a re-check reports; not law)", "backend/internal/tariffwatch/doc.go", "candidate_documentation"),
]


def sha(path):
    return hashlib.sha256(open(os.path.join(REPO, path), "rb").read()).hexdigest()


def write_sources():
    out = []
    for i, t, p, k in SOURCES:
        out.append({"id": i, "title": t, "path": p, "sha256": sha(p), "kind": k})
    json.dump({"note": "No external source operation was used by this lane (0 of 2 allowed); every source is a local archive already in the repository.",
               "sources": out}, open(os.path.join(HERE, "sources.json"), "w"), indent=1)
    return {s[0] for s in SOURCES}


# --- locators ---------------------------------------------------------------

USMCA_LOC = {
    "gn11o-8202.31": "GN 11(o) Chapter 82 rule 3 (GN p.93)",
    "gn11o-8207.13": "GN 11(o) Chapter 82 rule 6 (GN p.93)",
    "gn11o-9503.00": "GN 11(o) Chapter 95 rule 1 (GN p.160)",
    "gn11o-3004": "GN 11(o) Chapter 30 rule 2",
    "gn11o-8472": "GN 11(o) Chapter 84 rule 209",
    "gn11o-1501-1518": "GN 11(o) Chapter 15 rule 1",
    "gn11o-271119": "GN 11(o) Chapter 27 rule 10",
    "gn11o-7009.10-7009.91": "GN 11(o) Chapter 70 rule 7",
    "gn11o-4010-4011": "GN 11(o) Chapter 40 rule 13",
    "gn11o-2009.11-2009.39": "GN 11(o) Chapter 20 rule 4",
    "gn11o-7614": "GN 11(o) Chapter 76 rule 10",
    "gn11o-8407.10-8407.29": "GN 11(o) Chapter 84 rule 16",
    "gn11o-8478.10": "GN 11(o) Chapter 84 rule 233",
    "gn11o-8471.70": "GN 11(o) Chapter 84 rule 204",
    "gn11o-8471.60": "GN 11(o) Chapter 84 rule 203",
    "gn11o-8471.50": "GN 11(o) Chapter 84 rule 202",
    "gn11o-8471.30": "GN 11(o) Chapter 84 rule 200",
}


def usmca_sources(c):
    sf, fam = c["source_family"], c["scenario_family"]
    s = []
    base = sf
    for k in USMCA_LOC:
        if sf.startswith(k):
            base = k
    if base in USMCA_LOC:
        s.append({"id": "gn11o", "locator": USMCA_LOC[base]})
    if fam.startswith("rvc"):
        s.append({"id": "gn11-general", "locator": "GN 11(c)(ii)-(iii) RVC formulas (GN p.29)"})
    if fam == "de-minimis":
        s.append({"id": "gn11-general", "locator": "GN 11(e)(i)-(ii) de minimis (GN p.31-32)"})
    if fam == "parts-route":
        s.append({"id": "gn11-general", "locator": "GN 11(n)(i) parts route (GN p.28-29, 48-49)"})
    if fam == "deemed":
        s.append({"id": "gn11-general", "locator": "GN 11(p) deemed originating goods (GN p.163)"})
    if fam in ("profile", "invalid-input", "missing-fact", "claims", "completeness", "good-code", "input-code"):
        s.append({"id": "usmca-doc", "locator": "package doc: tri-state inputs, caller-supplied claims, scope limits"})
    if not s:
        s.append({"id": "usmca-doc", "locator": "package doc"})
    return s


def textile_sources(c):
    fam = c["scenario_family"]
    s = [{"id": "ecfr-102-21", "locator": "19 CFR 102.21(c)(1)-(5) and Table 1 to paragraph (e)(1), " + c["source_family"].replace("ecfr-", "row ")}]
    if fam == "e2":
        s.append({"id": "ecfr-102-21", "locator": "19 CFR 102.21(e)(2)"})
    if fam in ("tariff-shift-abstention", "scope"):
        s.append({"id": "textile-doc", "locator": "package doc: abstains where a rule turns on a judgment or a shift"})
    if "6201" in c["source_family"]:
        s.append({"id": "fr-87-68356", "locator": "entry for 6201-6208"})
    return s


def tariff_sources(c):
    sf = c["source_family"]
    s = []
    if sf.startswith("list-") or sf.startswith("window-") or sf.startswith("excl-"):
        s.append({"id": "additional-duty", "locator": "list " + sf.split("-", 1)[1].split("-")[0] + " lines and rate (embedded HTS Chapter 99 data)"})
    if sf.startswith("maritime-"):
        s += [{"id": "fr-90-48320", "locator": "Annex V.A, headings 9903.91.12 / 9903.91.14"}, {"id": "ch99-rev23", "locator": "9903.91.12 to 9903.91.16"}]
    if sf.startswith("ch99-revision-reader"):
        s += [{"id": "ch99-rev23", "locator": "heading layout and wording"}, {"id": "ch99-rev22", "locator": "9903.88.69 and 9903.88.70"}]
    if sf.startswith("excl-"):
        s.append({"id": "fr-89-76581", "locator": "exclusion lines (heading layout)"})
    s.append({"id": "tariffwatch-doc", "locator": "package doc: what a re-check reports; the revised data set is SYNTHETIC"})
    return s


# --- normalisation (the runner applies the same rules) -----------------------


def norm_amount(x):
    if x is None:
        return None
    s = str(x)
    if "." in s:
        s = s.rstrip("0").rstrip(".")
    return s


def normalize(feature, c):
    inp = json.loads(json.dumps(c["input"]))
    if feature == "usmca_rules":
        body = inp.get("input")
        if body is None:
            return json.dumps({"raw": inp.get("raw")}, sort_keys=True)
        for m in body.get("materials", []) or []:
            m.pop("id", None)
            if "value" in m:
                m["value"] = norm_amount(m["value"])
        body["materials"] = sorted(body.get("materials", []) or [], key=lambda m: json.dumps(m, sort_keys=True))
        for k in ("transaction_value", "net_cost", "vnm", "total_cost"):
            if k in body:
                body[k] = norm_amount(body[k])
        inp["input"] = body
    elif feature == "textile_rules":
        f = inp["facts"]
        f["materials"] = sorted(f.get("materials", []) or [], key=lambda m: json.dumps(m, sort_keys=True))
    return json.dumps(inp, sort_keys=True, separators=(",", ":"))


# --- assembly ----------------------------------------------------------------


def usmca_sets():
    dev, test = [], []
    S = US
    # development
    dev += S.two_alt("gn11o-8202.31", "pilot", "820231", "820239", "820231", "820220", "390110", "732010", "82", "chapter", "8202", "82023130", "820299", "8202.31 saw blades")
    dev += S.two_alt("gn11o-8472", "extended", "847210", "847330", "847290", None, "390110", "732010", "84", "heading", None, "84721030", None, "heading 8472")
    dev += S.noshift_alt("gn11o-3004", "extended", "300490", "300410", "300510", "390110", "300320", None, "heading 3004")
    dev += S.shift_row("gn11o-1501-1518", "extended", "150710", "120100", "151190", "382300", "38", None, "headings 1501-1518", "chapter")
    dev += S.shift_row("gn11o-271119", "extended", "271119", "270900", "271119", "271129", "27", "2711", "subheading 2711.19", "subheading")
    dev += S.shift_row("gn11o-7009.10-7009.91", "extended", "700910", "390110", "700990", "700510", "70", "7009", "subheadings 7009.10-7009.91", "heading")
    dev += S.de_minimis("gn11o-7009.10-7009.91", "700910", "700990", "390110", "700510", "7009 de minimis", False)
    dev += S.de_minimis_low("gn11o-271119", "271119", "271119", "2711.19")
    dev += S.de_minimis_low("gn11o-1501-1518", "150710", "151190", "1507")
    dev += S.parts_route("gn11o-7009.10-7009.91", "700910", "700910", "700510", "7009 parts route")
    dev += S.deemed("gn11o-8471.30", "extended", "847130", "847149", "8471.30 deemed")
    dev += S.contract("usmca-doc-820231", "820231", "390110", "820239", "pilot contract")
    dev += S.invalid("usmca-doc-input-validation", "820231", "dev")
    # test
    test += S.two_alt("gn11o-8207.13", "pilot", "820713", "820719", "820713", "820730", "848210", "732690", "82", "chapter", "8207", "82071330", "820750", "8207.13 rock drilling tools")
    test += S.two_alt("gn11o-8478.10", "extended", "847810", "847890", "847810", None, "390110", "732010", "84", "heading", "8478", "84781030", None, "subheading 8478.10")
    test += S.noshift_alt("gn11o-9503.00", "pilot", "950300", "950300", "950440", "392690", None, "9503", "subheading 9503.00")
    test += S.shift_row("gn11o-4010-4011", "extended", "401110", "390110", "401120", "401210", "40", None, "headings 4010-4011", "heading")
    test += S.shift_row("gn11o-2009.11-2009.39", "extended", "200912", "170199", "200890", "080510", "08", "2009", "subheadings 2009.11-2009.39", "chapter")
    test += S.shift_row("gn11o-7614", "extended", "761410", "740311", "761490", "760410", "76", None, "heading 7614", "heading")
    test += S.and_rvc("gn11o-8407.10-8407.29", "840721", "840790", "390110", "subheadings 8407.10-8407.29")
    dm = S.de_minimis("gn11o-7614", "761410", "761490", "740311", "760410", "7614 de minimis", False)
    test += dm
    keep = {"threshold_boundary", "total_cost_base", "aggregate", "base_not_elected"}
    test += [s for s in S.de_minimis("gn11o-4010-4011", "401110", "401120", "390110", "401210", "4011 de minimis", False) if keep & set(s.tags)]
    test += S.de_minimis_low("gn11o-2009.11-2009.39", "200912", "200890", "2009.12")
    test += S.de_minimis_rvc("gn11o-8407.10-8407.29", "840721", "840790", "390110", "8407.21")
    test += S.parts_route("gn11o-7614", "761410", "761410", "760410", "7614 parts route")
    test += S.deemed("gn11o-8471.70", "extended", "847170", "847149", "8471.70 deemed", "390110")
    test += S.deemed("gn11o-8471.60", "extended", "84716010", "847149", "8471.60.10 deemed", "390110")
    test += S.deemed("gn11o-8471.50", "extended", "847150", "847149", "8471.50 deemed")
    test.append(US.Sc("deemed", "deemed-originating", "gn11o-8471.60", ["negative", "not_listed_tariff_item"],
                      "8471.60.30 is not one of the tariff items GN 11(p) lists, so imported-from-USMCA does not deem it originating",
                      US.mk("84716030", [US.M("847149")], prof={"imported_from_usmca_country_confirmed": True}), ruleset="extended"))
    test += S.contract("usmca-doc-820713", "820713", "848210", "820719", "pilot contract")
    test += S.invalid("usmca-doc-input-validation", "820713", "test")
    return US.finish(dev), US.finish(test)


def textile_sets():
    T = TX
    Y, N = "yes", "no"
    dev, test = [], []
    dev += T.process_row("ecfr-5204-5207", "5205110000", "spinning", "cotton yarn 5204-5207", "5206.11", "5201.00", precursor="dyeing")
    dev += T.dyeprint("ecfr-5208-5212", "5208120000", "woven cotton fabric 5208-5212", "5208")
    dev += T.assembly_row("ecfr-6201-6208", "6203424011", "men's trousers 6201-6208", "6203", "5208.12", "5205.11")
    dev += T.process_row("ecfr-6307.20", "6307200000", "assembly", "life jackets and belts 6307.20", None, "5503.20", precursor="fabric_making", c_fibre="India", comps=Y, tmpl="single-process-rule")
    dev += T.packaging_row("ecfr-6309-6310", "6309000000", "worn clothing 6309")
    dev += T.e2_rows("ecfr-6301-6306", "6302221020", "bed linen 6302.22", "6302.22")
    dev += T.shift_abstain("ecfr-5001-5002", "5001000000", "silk cocoons 5001-5002")
    dev += T.scope_cases("textile-doc", [
        ("3926201000", "plastic apparel (3926.20) is outside 19 CFR 102.21(b)(5)", ["out_of_scope"], TX.F("3926201000", [TX.S("assembly", "China")])),
        ("6403996030", "footwear of chapter 64 is not a textile product under (b)(5)", ["out_of_scope"], TX.F("6403996030", [TX.S("assembly", "China"), TX.S("assembly", "Vietnam")])),
        ("", "no good code at all", ["no_code"], TX.F("", [TX.S("assembly", "China")])),
        ("62", "a two-digit code cannot select a rule", ["no_code"], TX.F("62", [TX.S("assembly", "China")])),
        ("4202", "4202 overlaps the (b)(5) list only in part, so scope cannot be told from four digits", ["code_too_short"], TX.F("4202", [TX.S("assembly", "China")])),
        ("9619", "9619 overlaps the (b)(5) list in part", ["code_too_short"], TX.F("9619", [TX.S("assembly", "China")])),
    ])
    # test
    test += T.process_row("ecfr-5508-5511", "5509220000", "spinning", "staple yarn 5508-5511", "5510.11", "5503.20")
    test += T.dyeprint("ecfr-5407-5408", "5407520000", "woven synthetic filament fabric 5407-5408", "5407")
    d6001 = T.dyeprint("ecfr-6001-6006", "6001920000", "knit pile fabric 6001-6006", "6001", wool="no")
    test += d6001
    test += [T.Sc("wool-condition", "dye-print-or-fabric-rule", "ecfr-6001-6006", ["positive", "wool_fabric"], "6001-6006: fabric of wool skips alternative 1 and fabric-making decides",
                  T.F("6001920000", [T.S("fabric_making", "India"), T.S("dyeing", "China"), T.S("printing", "China")] + T.FIN2, fabric_of_wool="yes")),
             T.Sc("wool-condition", "dye-print-or-fabric-rule", "ecfr-6001-6006", ["undetermined", "wool_not_stated"], "6001-6006: whether the fabric is of wool is not stated",
                  T.F("6001920000", [T.S("fabric_making", "India"), T.S("dyeing", "China"), T.S("printing", "China")] + T.FIN2))]
    test += T.process_row("ecfr-5401-5406", "5402333000", "extrusion", "filament yarn 5401-5406", "5402.20", "3901.10", precursor="spinning", c_fibre="Indonesia")
    test += T.assembly_row("ecfr-6210-6211", "6210109010", "coated fabric garments 6210-6211", "6210", "6001.92", "5503.20")
    test += T.assembly_row("ecfr-6212", "6212109010", "brassieres 6212", "6212", "5208.12", "5503.20", knit_aware=True)
    test += T.knit_row("ecfr-6212", "6212109010", "brassieres 6212")
    test += T.assembly_row("ecfr-6101-6117", "6109100012", "knit T-shirts 6101-6117", "6109", "5208.12", "5503.20", knit_aware=True, c_fab="Pakistan", c_asm="Honduras", c_other="Mexico")
    test += T.knit_row("ecfr-6101-6117", "6110202079", "knit sweaters 6101-6117", c_knit="Sri Lanka", c_other="Pakistan")
    test += T.e2_rows("ecfr-6213-6214", "6213201000", "handkerchiefs 6213", "6213", c_fab="Turkey", c_fin="Italy")
    test += T.e2_rows("ecfr-6101-6117-e2", "6117102000", "knit accessories 6117.10", "6117.10", c_fab="India", c_fin="China", tmpl="e2-dye-print-or-fabric", skip=("ordinary_row", "fabric_fallback"))
    test += [T.Sc("e2", "e2-dye-print-or-fabric", "ecfr-6101-6117-e2", ["positive", "knit_to_shape", "e2_iii_a"], "6117.10 not of cotton, knit to shape, dyed only: (e)(2)(iii)(A) names the knit-to-shape country",
                  T.F("6117102000", [T.S("spinning", "India"), T.S("knit_to_shape", "Bangladesh"), T.S("dyeing", "China")], complete=True, cotton_wool_16=N, knit_to_shape=Y, two_or_more_components=Y)),
             T.Sc("e2", "e2-dye-print-or-fabric", "ecfr-6101-6117-e2", ["positive", "assembled", "e2_iii_b"], "6117.10 not of cotton, not knit to shape, two or more components: (e)(2)(iii)(B) names the assembly country",
                  T.F("6117102000", [T.S("fabric_making", "India"), T.S("assembly", "Vietnam")], complete=True, cotton_wool_16=N, knit_to_shape=N, two_or_more_components=Y)),
             T.Sc("e2", "e2-dye-print-or-fabric", "ecfr-6101-6117-e2", ["positive", "cotton", "ordinary_row"], "6117.10 of cotton: the table row 6101-6117 applies (assembled good)",
                  T.F("6117102000", [T.S("fabric_making", "India"), T.S("assembly", "Vietnam")], complete=True, cotton_wool_16=Y, knit_to_shape=N, two_or_more_components=Y)),
             T.Sc("e2", "e2-dye-print-or-fabric", "ecfr-6101-6117-e2", ["undetermined", "knit_to_shape_not_stated"], "6117.10 not of cotton, knit-to-shape not stated: (e)(2)(ii)/(iii) cannot be chosen",
                  T.F("6117102000", [T.S("fabric_making", "India"), T.S("assembly", "Vietnam")], cotton_wool_16=N))]
    test += T.fibre_row("ecfr-5609", "5609003000", "articles of yarn 5609")
    test += T.last_row("ecfr-5604", "5604900000", "rubber thread and covered yarn 5604")
    test += T.process_row("ecfr-5807", "5807100510", "fabric_making", "labels and badges 5807", None, "5503.20", precursor="spinning", c_fibre="India", c_proc="Turkey", c_other="Italy")
    test += T.process_row("ecfr-5810.10", "5810102000", "embroidery", "embroidery without visible ground 5810.10", None, "5503.20", precursor="fabric_making", c_fibre="India", c_proc="Pakistan", c_other="Turkey")
    test += T.shift_abstain("ecfr-5201", "5201001800", "raw cotton 5201")
    test += T.scope_cases("textile-doc", [
        ("8708290000", "motor vehicle parts of 8708.29 are not in the (b)(5) list", ["out_of_scope"], TX.F("8708290000", [TX.S("assembly", "Mexico"), TX.S("assembly", "Canada")])),
        ("9503000000", "toys of chapter 95 are not textile products", ["out_of_scope"], TX.F("9503000000", [TX.S("assembly", "China")])),
        ("6", "a one-digit code", ["no_code"], TX.F("6", [TX.S("assembly", "China")])),
        ("620", "a three-digit code cannot select a rule", ["no_code"], TX.F("620", [TX.S("assembly", "China")])),
        ("6302", "6302 overlaps the (e)(2) subheadings but sits in the 6301-6306 row: (e)(2) cannot be told apart without more digits", ["code_too_short"], TX.F("6302", [TX.S("fabric_making", "India"), TX.S("assembly", "China")])),
        ("63", "chapter 63 alone is too short", ["no_code"], TX.F("63", [TX.S("assembly", "China")])),
    ])
    return TX.finish(dev), TX.finish(test)


def tariff_sets():
    Tw = TW
    dev, test = [], []

    def tag(scs, src):
        for s in scs:
            s.source = src
        return scs

    dev += tag(Tw.list_family("", "9903.88.01", "Section 301 China, List 1", "0.25", "List 1 line"), "list-9903.88.01")
    dev += tag(Tw.list_family("", "9903.88.15", "Section 301 China, List 4A", "0.075", "List 4A line", subs_ix=(2, 9, 20), mfn_old="0.05"), "list-9903.88.15")
    dev += tag(Tw.window_family("", "9903.88.01", "Section 301 China, List 1 (dated, synthetic)", "0.25", "dated List 1 action"), "window-9903.88.01")
    dev += tag(Tw.exclusion_family("", "9903.88.01", "Section 301 China, List 1", "0.25", "List 1 exclusion line 8483.50.9040", "8483.50.9040"), "excl-9903.88.01")
    dev += tag(Tw.maritime_family("", "9903.91.12", "Section 301 China maritime, chassis", ["8716.39.00", "8716.90.30", "8716.90.50"], "chassis action 9903.91.12", "1.5"), "maritime-9903.91.12")
    dev += tag(Tw.invalid_family("", "dev data set", "dev"), "dataset-validation-dev")
    dev += RV.scenarios("dev")

    def only(scs, tags):
        return [s for s in scs if set(s.tags) & tags]

    test += tag(Tw.list_family("", "9903.88.03", "Section 301 China, List 3", "0.25", "List 3 line", subs_ix=(1, 40, 500), mfn_old="0.0"), "list-9903.88.03")
    test += tag(only(Tw.list_family("", "9903.88.02", "Section 301 China, List 2", "0.25", "List 2 line", subs_ix=(4, 11, 30), mfn_old="0.045"),
                     {"rate_cut", "line_removed", "mfn_and_list", "plan_origin_gains", "other_origin_unaffected", "user_entered_rate", "line_added", "rate_to_small_change", "small_import_value"}), "list-9903.88.02")
    test += tag(only(Tw.list_family("", "9903.88.04", "Section 301 China, List 3 (named products)", "0.25", "List 3 named line", subs_ix=(0, 5, 9), mfn_old="0.0"),
                     {"rate_rise", "list_withdrawn", "mfn_cut_to_free", "mfn_rise", "not_modelled_covered", "modelled_added", "origin_specific_232", "no_import_value", "mfn_not_in_dataset"}), "list-9903.88.04")
    test += tag(Tw.window_family("", "9903.88.02", "Section 301 China, List 2 (dated, synthetic)", "0.25", "dated List 2 action", subs_ix=(2, 7)), "window-9903.88.02")
    test += tag(Tw.exclusion_family("", "9903.88.03", "Section 301 China, List 3", "0.25", "List 3 exclusion line 0304.72.5000", "0304.72.5000"), "excl-9903.88.03")
    test += tag(only(Tw.exclusion_family("", "9903.88.02", "Section 301 China, List 2", "0.25", "List 2 exclusion line 9025.19.8010", "9025.19.8010"),
                     {"window_extended", "whole_subheading_valid", "whole_subheading_lapsed", "exclusion_removed"}), "excl-9903.88.02")
    mar = Tw.maritime_family("", "9903.91.14", "Section 301 China maritime, ship-to-shore cranes", ["8426.19.00"], "crane action 9903.91.14", "1.2")
    test += tag([s for s in mar if "second_line" not in s.tags], "maritime-9903.91.14")
    test += tag(Tw.invalid_family("", "test data set", "test"), "dataset-validation-test")
    test += RV.scenarios("test")
    return Tw.finish(dev), Tw.finish(test)


PARENT_TAGS = {"good_code_long", "ten_digit_code", "code_short_decisive"}


def emit(feature, prefix, dev, test, source_fn, srcs):
    outs = {}
    for split, cases in (("dev", dev), ("test", test)):
        rows = []
        seen = set()
        first_by_src = {}
        for n, c in enumerate(cases, 1):
            c["id"] = f"{prefix}-{split}-{n:03d}"
            c["feature"], c["split"] = feature, split
            c["synthetic"] = True
            c["sources"] = source_fn(c)
            for s in c["sources"]:
                assert s["id"] in srcs, s
            key = normalize(feature, c)
            assert key not in seen, ("duplicate normalized input inside split", c["id"], c["rationale"])
            seen.add(key)
            # parent: robustness variants point to the first plain positive case of the family
            first_by_src.setdefault(c["source_family"], c["id"])
            if PARENT_TAGS & set(c["variation_tags"]):
                c["parent"] = first_by_src[c["source_family"]]
            rows.append(c)
        outs[split] = rows
    dk = {normalize(feature, c) for c in outs["dev"]}
    tk = {normalize(feature, c) for c in outs["test"]}
    assert not (dk & tk), "cross-split duplicate normalized input"
    for split, rows in outs.items():
        path = os.path.join(HERE, f"{prefix}-{split}.jsonl")
        with open(path, "w") as f:
            for c in rows:
                keys = ["id", "feature", "split", "source_family", "scenario_family", "template", "parent", "variation_tags", "sources",
                        "rationale", "synthetic", "expect_invalid", "input", "expected"]
                f.write(json.dumps({k: c[k] for k in keys if k in c}, sort_keys=False, separators=(",", ":")) + "\n")
        print(feature, split, len(rows))


def main():
    srcs = write_sources()
    ud, ut = usmca_sets()
    emit("usmca_rules", "usmca", ud, ut, usmca_sources, srcs)
    xd, xt = textile_sets()
    emit("textile_rules", "textile", xd, xt, textile_sources, srcs)
    td, tt = tariff_sets()
    emit("tariff_watch_rates", "tariffwatch", td, tt, tariff_sources, srcs)


if __name__ == "__main__":
    main()
