# Builds eval/splits/splits_textile.json and backend/data/textile_rulings.json (D51).
# Usage: python3 eval/splits/build_textile_split.py .   (from the repo root; needs data/corpus/raw)

import json,os,sys,collections
root=sys.argv[1]
L={}
for l in open(f'{root}/eval/data/labels.jsonl'):
    r=json.loads(l); L[r['ruling']]=r
tex=[]
for f in sorted(os.listdir(f'{root}/data/corpus/raw')):
    rid=f[:-5]; r=L.get(rid)
    if not r or r['regime']!='part102_tariff_shift' or not r.get('primary_origin'): continue
    if '102.21' not in open(f'{root}/data/corpus/raw/{f}',errors='ignore').read(): continue
    if not os.path.exists(f'{root}/data/corpus/facts/{rid}.txt'): continue
    tex.append(r)
tex.sort(key=lambda r:(r['date'],r['ruling']))
dev=[r['ruling'] for r in tex if '2025-07-01'<=r['date']<'2026-01-01']
final=[r['ruling'] for r in tex if r['date']>='2026-01-01']
out={"source":"labels.jsonl rulings with regime part102_tariff_shift whose raw text cites 19 CFR 102.21, with a primary origin and a facts file (textile regime, D51)",
     "frozen_at":"2026-10-16","rule":"dated: dev = Jul-Dec 2025, final = 2026 (held out; run once with the frozen textile prompt). Precedents for every case are textile rulings dated before it.",
     "textile": [r['ruling'] for r in tex], "dev":dev, "final":final}
json.dump(out, open(f"{root}/eval/splits/splits_textile.json","w"), indent=1)
json.dump({"source":out["source"],"frozen_at":out["frozen_at"],"rulings":out["textile"]}, open(f"{root}/backend/data/textile_rulings.json","w"), indent=0)
print(len(tex), 'dev', len(dev), 'final', len(final), collections.Counter(L[i]['primary_origin'] for i in final).most_common(5))
