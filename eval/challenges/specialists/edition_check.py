"""Japan cases whose tariff codes fall outside the HS edition of the schedule in force.

Writes edition-defects.json (ids per split). The good's code and every material
code are checked against the HS 2017 list for schedules of HS 2017 or older
(older editions only approximately) and against the HS 2022 list for HS 2022
schedules. Preflight cases (code outside the edition on purpose) are excluded.
Deterministic; `--check` compares with the committed file.
"""
import json, os, re, sys
HERE = os.path.dirname(os.path.abspath(__file__))
J = os.path.join(HERE, "..", "..", "..", "backend", "internal", "jprules", "data")
D = "2026-11-28"
ed = {}
for s in json.load(open(os.path.join(J, "schedules.json"))):
    if s["from"] <= D and (not s["until"] or D < s["until"]):
        ed[s["agreement"]] = s["stated_edition"]
h17 = {l.strip() for l in open(os.path.join(J, "hs2017_codes.txt"))}
h22 = {l.strip() for l in open(os.path.join(J, "hs2022_codes.txt"))}
out = {}
for sp in ("dev", "test"):
    ids = []
    for l in open(os.path.join(HERE, "japan-%s.jsonl" % sp)):
        c = json.loads(l)
        if c["expected"].get("preflight"):
            continue
        j = c["input"]["japan"]
        ref = h22 if ed[j["agreement"]] == 2022 else h17
        codes = [j["hs"].replace(".", "")] + [x.replace(".", "") for x in re.findall(r"HS (\d{4}\.\d{2})", c["input"]["facts"])]
        if any(x not in ref for x in codes):
            ids.append(c["id"])
    out[sp] = sorted(ids)
text = json.dumps(out, indent=1) + "\n"
p = os.path.join(HERE, "edition-defects.json")
if "--check" in sys.argv:
    sys.exit(0 if open(p).read() == text else "edition-defects.json differs")
open(p, "w").write(text)
print({k: len(v) for k, v in out.items()})
