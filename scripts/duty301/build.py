#!/usr/bin/env python3
"""Build backend/internal/duty/additional.json (D90) from the Section 301
extractions of HTS Chapter 99 (HTS 2026 Rev 21).

Inputs (made by parsing the HTS Chapter 99 PDF, each list extracted twice by
independent parsers and reconciled; see D90):
  /tmp/s301/list{1,2,3,3h,4a}.final.json   China Section 301, U.S. note 20
  /tmp/s301/exclusions.json                 product exclusions, 9903.88.69/.70
  /tmp/s301/forced_labor_headings.json      2026 country-wide 301 headings
  /tmp/s301/forced_labor_exempt.json        U.S. note 52 exemptions

    python3 scripts/duty301/build.py [/tmp/s301]
"""
import json, os, sys

src = sys.argv[1] if len(sys.argv) > 1 else "/tmp/s301"
load = lambda f: json.load(open(os.path.join(src, f)))

NAMES = {
    "9903.88.01": "Section 301 China, List 1",
    "9903.88.02": "Section 301 China, List 2",
    "9903.88.03": "Section 301 China, List 3",
    "9903.88.04": "Section 301 China, List 3 (named products)",
    "9903.88.15": "Section 301 China, List 4A",
}
lists, seen = [], {}
for key in ["list1", "list2", "list3", "list3h", "list4a"]:
    d = load(f"{key}.final.json")
    assert d.get("verified"), key
    subs = sorted(set(d["subheadings"]))
    for s in subs:
        assert s not in seen, f"{s} on {seen[s]} and {d['heading']}"
        seen[s] = d["heading"]
    lists.append({"heading": d["heading"], "name": NAMES[d["heading"]], "origin": "China", "rate": d["rate"],
                  "note": f"U.S. note {d['note']} to subchapter III of chapter 99", "subheadings": subs})

exclusions = []
for x in load("exclusions.json"):
    for frm in x.get("excludes_from") or [x["heading"]]:
        for h in sorted(set(x.get("hts_all") or [x["hts"]])):
            exclusions.append({"heading": frm, "hts": h, "whole_subheading": False,
                               "description": x["description"][:200], "end_date": x.get("end_date", "")})

country = []
heads = load("forced_labor_headings.json")
exempt = load("forced_labor_exempt.json")
# Note 52 exemptions applied automatically: lists that cover a whole
# subheading for every use. Lists tied to a use or an entry claim (civil
# aircraft 52(d), pharmaceutical use 52(e), CAFTA-DR claims (j)(6)(iii) and
# (j)(7)(iii)) and named particular articles ((c), (j)(n)(ii)) are reported
# as conditional instead (D90).
CONDITIONAL = {
    "52(d)": "civil aircraft only (general note 6)",
    "52(e)": "for use in pharmaceutical applications only",
    "52(j)(6)(iii)": "only when entry is claimed under CAFTA-DR",
    "52(j)(7)(iii)": "only when entry is claimed under CAFTA-DR",
}
def covers(e, h):
    """Whether note-52 entry e exempts country heading h."""
    rd = e.get("removes_duty_of") or ""
    if "9903.05.20" in rd and "9903.05.84" in rd:
        return True
    lo_hi = [x.strip() for x in rd.replace("and", ",").split(",") if x.strip()]
    for x in lo_hi:
        if "–" in x:
            lo, hi = [y.strip() for y in x.split("–")]
            if lo <= h["heading"] <= hi:
                return True
        elif x == h["heading"]:
            return True
    return False
for h in heads:
    if h["mode"] == "no_additional":
        continue  # the floor_total row of the same country carries the rule
    ex, cond = set(), []
    for e in exempt:
        if not e.get("hts") or not covers(e, h):
            continue
        if e["note"] in CONDITIONAL:
            cond.append({"note": e["note"], "condition": CONDITIONAL[e["note"]], "hts": sorted(set(e["hts"]))})
        elif e.get("kind") == "particular_articles" or e["note"] == "52(c)":
            cond.append({"note": e["note"], "condition": "named articles only: " + e["summary"][:160], "hts": sorted(set(e["hts"]))})
        else:
            ex.update(e["hts"])
    origin = {"Hong Kong, China": "Hong Kong", "Türkiye": "Turkey"}.get(h["country"], h["country"])
    country.append({"heading": h["heading"], "name": "Section 301 (2026, 91 FR 47318)", "origin": origin,
                    "rate": h["rate"], "mode": h["mode"], "source": "HTS heading " + h["heading"] + "; exemptions U.S. note 52",
                    "exempt": sorted(ex), "conditional": cond})

out = {"source": "HTS Chapter 99 (U.S. notes 20 and 52, headings 9903.88 and 9903.05)",
       "hts_revision": "HTS 2026 Revision 21", "checked": "2026-10-18",
       "lists": lists, "country": country, "exclusions": exclusions}
path = os.path.join(os.path.dirname(__file__), "../../backend/internal/duty/additional.json")
json.dump(out, open(path, "w"), indent=1)
print(f"{sum(len(l['subheadings']) for l in lists)} list subheadings, {len(country)} country duties, "
      f"{len(exclusions)} exclusion entries -> {os.path.normpath(path)}")
