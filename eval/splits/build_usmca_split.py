# Builds eval/splits/splits_usmca.json (D61) from eval/data/labels_usmca.jsonl.
# Usage: python3 eval/splits/build_usmca_split.py .   (from the repo root)
# labels_usmca.jsonl and the facts come from (in backend/):
#   go run ./cmd/corpus usmca-fetch && go run ./cmd/corpus usmca-label -facts data/facts

import json,os,re,sys,collections
root=sys.argv[1]
# 2025 alone has about 40 labelled rulings; two years give a usable interval.
DEV_FROM, FINAL_FROM = "2024-01-01", "2026-01-01"
# A ruling whose holding sentences answer only a substantial-transformation,
# Section 301 or procurement question, with no word of marking or Part 102,
# decided another question. It stays a precedent but is not scored. Decided
# from the labels before any model run.
OTHER=re.compile(r'substantial(ly)? transform|Section 301|trade remed|procurement|Trade Agreements Act',re.I)
PART102=re.compile(r'marking|102\.|tariff shift|USMCA|NAFTA',re.I)
def other_question(r):
    ev=' '.join(r['evidence'])
    return bool(OTHER.search(ev)) and not PART102.search(ev)

rows=[json.loads(l) for l in open(f'{root}/eval/data/labels_usmca.jsonl')]
lab=[r for r in rows if r.get('primary_origin') and os.path.exists(f'{root}/backend/data/facts/{r["ruling"]}.txt')]
lab.sort(key=lambda r:(r['date'],r['ruling']))
scored=[r for r in lab if not other_question(r)]
dev=[r['ruling'] for r in scored if DEV_FROM<=r['date']<FINAL_FROM]
final=[r['ruling'] for r in scored if r['date']>=FINAL_FROM]
out={"source":"eval/data/labels_usmca.jsonl: CROSS rulings citing 19 CFR 102.20 or 102.11 (2015 on) that apply the Part 102 general rules to a non-textile good, with a primary origin and a facts file (USMCA regime, D61)",
     "frozen_at":"2026-10-17",
     "rule":f"dated: dev = {DEV_FROM} to {FINAL_FROM} (exclusive), final = {FINAL_FROM} on (held out; run once with a frozen prompt). Rulings whose holding sentences answer only a substantial-transformation, Section 301 or procurement question are precedents but not scored. Precedents for every case are the labelled Part 102 rulings dated before it.",
     "usmca":[r['ruling'] for r in lab], "dev":dev, "final":final}
json.dump(out, open(f"{root}/eval/splits/splits_usmca.json","w"), indent=1)
by_year=collections.Counter(r['date'][:4] for r in lab)
print(len(rows),'rows,',len(lab),'with origin and facts,',len(lab)-len(scored),'not scored (other question); dev',len(dev),'final',len(final),dict(sorted(by_year.items())))
print('dev origins',collections.Counter(r['primary_origin'] for r in scored if r['ruling'] in set(dev)).most_common(6))
