"""Canada's published CBSA origin and marking rulings, as display-only precedents (D923).

Usage: python3 -I -B eval/ca/parse.py <raw_dir>      rebuild from fetch.py's snapshot
       python3 -I -B eval/ca/parse.py --check        check the committed files

Writes data/ca/rulings.jsonl and its embedded copy
backend/internal/carulings/rulings.json. Every Origin Advance Ruling and
Marking National Customs Ruling on the CBSA's public list is kept; none is
excluded. HOLDING is our one-line summary of each decision; the full text is
the CBSA page at the ruling's url. HS_GOOD gives the good's code where the
list's classification field is blank, taken from the ruling's own text.
"""
import json, os, re, sys

ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
OUT = os.path.join(ROOT, "data", "ca", "rulings.jsonl")
EMBED = os.path.join(ROOT, "backend", "internal", "carulings", "rulings.json")
PAGE = "https://ccp-pcc.cbsa-asfc.cloud-nuage.canada.ca/en/national-rulings/national-rulings-details/"
TYPES = {"Origin Advance Ruling": "origin", "Marking National Customs Ruling": "marking"}

# id -> (agreement or marking regulations, holding)
HOLDING = {
    "8000001179": ("NAFTA marking rules", "No marking required; if marked, Mexico"),
    "8000001289": ("NAFTA", "Originating under NAFTA"),
    "8000001298": ("CUFTA", "Originating under the Canada–Ukraine agreement"),
    "8000001445": ("CPTPP", "Not originating under CPTPP"),
    "8000001456": ("CPTPP", "Originating under CPTPP"),
    "8000001565": ("NAFTA marking rules", "No marking required; if marked, United Kingdom"),
    "8000002155": ("CUSMA", "Originating under CUSMA"),
    "8000002826": ("CUSMA", "Originating under CUSMA"),
    "8000002867": ("CUSMA", "Not originating under CUSMA"),
    "8000003153": ("CPTPP", "Blouse originating under CPTPP; pullover not (thread from Korea)"),
    "8000005476": ("CUSMA", "Not originating under CUSMA"),
    "8000005665": ("CUSMA", "Not originating under CUSMA"),
    "8000005741": ("Non-CUSMA marking rules", "Complies if the retail box shows the origin"),
    "8000006862": ("CETA", "Not originating under CETA"),
}
HS_GOOD = {
    "8000001179": ["6302.51"],
    "8000001298": ["6203.42", "6204.62"],
    "8000001456": ["6201.13"],
    "8000001565": ["1515.90"],
    "8000002155": ["9404.90"],
}
FIELDS = ["id", "date", "type", "product", "hs", "agreement", "holding", "facts", "reasoning", "url"]


def clean(s):
    return re.sub(r"\s+", " ", (s or "").replace("&&", " ")).strip()


def cut(s, n):
    if len(s) <= n:
        return s
    s = s[:n]
    return s[: s.rfind(" ")] + " …"


def hs_codes(field):
    return [c.strip() for c in re.split(r"[/,]", field) if c.strip()]


def build(raw):
    listed = json.load(open(os.path.join(raw, "list.json")))["rulings"]
    out = []
    for r in sorted((r for r in listed if r["type"] in TYPES), key=lambda r: (r["date"], r["rulingsId"])):
        rid = r["rulingsId"]
        d = json.load(open(os.path.join(raw, "detail", rid + ".json")))
        agreement, holding = HOLDING[rid]
        out.append({
            "id": rid,
            "date": d["date"],
            "type": TYPES[d["type"]],
            "product": clean(d["product"]),
            "hs": HS_GOOD.get(rid) or hs_codes(d["classificationNumber"]),
            "agreement": agreement,
            "holding": holding,
            "facts": cut(clean(d["productDescription"]), 1200),
            "reasoning": cut(clean(d["analysis"]), 900),
            "url": PAGE + rid,
        })
    if {r["id"] for r in out} != set(HOLDING):
        sys.exit("parse: the published list and HOLDING disagree; review new or removed rulings")
    return out


def write(rows):
    os.makedirs(os.path.dirname(OUT), exist_ok=True)
    with open(OUT, "w", encoding="utf-8") as f:
        for r in rows:
            f.write(json.dumps(r, ensure_ascii=False) + "\n")
    with open(EMBED, "w", encoding="utf-8") as f:
        json.dump(rows, f, ensure_ascii=False, indent=1)
        f.write("\n")


def check():
    rows = [json.loads(l) for l in open(OUT, encoding="utf-8")]
    if rows != json.load(open(EMBED, encoding="utf-8")):
        sys.exit("check: backend/internal/carulings/rulings.json differs from data/ca/rulings.jsonl")
    if {r["id"] for r in rows} != set(HOLDING):
        sys.exit("check: ids differ from HOLDING")
    for r in rows:
        if list(r) != FIELDS or not re.fullmatch(r"\d{10}", r["id"]) or r["type"] not in TYPES.values():
            sys.exit("check: bad row " + r.get("id", "?"))
        if (r["agreement"], r["holding"]) != HOLDING[r["id"]] or not r["hs"] or not r["url"].endswith(r["id"]):
            sys.exit("check: row " + r["id"] + " out of date with parse.py")
    print("ca rulings ok:", len(rows), "rulings,", sum(r["type"] == "origin" for r in rows), "origin,",
          sum(r["type"] == "marking" for r in rows), "marking")


if __name__ == "__main__":
    if sys.argv[1:] == ["--check"]:
        check()
    elif len(sys.argv) == 2:
        write(build(sys.argv[1]))
        check()
    else:
        sys.exit(__doc__)
