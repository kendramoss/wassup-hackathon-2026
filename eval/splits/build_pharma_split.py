# Builds eval/splits/splits_pharma.json and backend/data/pharma_rulings.json (D62).
# Usage: python3 eval/splits/build_pharma_split.py .   (from the repo root; needs data/corpus/facts)

import json,os,sys,collections
root=sys.argv[1]
L={}
for l in open(f'{root}/eval/data/labels.jsonl'):
    r=json.loads(l); L[r['ruling']]=r
ph=[r for r in L.values() if r['regime']=='pharma_api' and r.get('primary_origin')
    and os.path.exists(f"{root}/data/corpus/facts/{r['ruling']}.txt")]
ph.sort(key=lambda r:(r['date'],r['ruling']))
dev=[r['ruling'] for r in ph if '2025-07-01'<=r['date']<'2026-01-01']
final=[r['ruling'] for r in ph if r['date']>='2026-01-01']
out={"source":"labels.jsonl rulings with regime pharma_api, with a primary origin and a facts file (pharmaceutical regime, D62)",
     "frozen_at":"2026-10-17","rule":"dated: dev = Jul-Dec 2025, final = 2026 (held out; run once with the frozen pharma prompt). Precedents for every case are pharmaceutical rulings dated before it.",
     "pharma": [r['ruling'] for r in ph], "dev":dev, "final":final}
json.dump(out, open(f"{root}/eval/splits/splits_pharma.json","w"), indent=1)
json.dump({"source":out["source"],"frozen_at":out["frozen_at"],"rulings":out["pharma"]}, open(f"{root}/backend/data/pharma_rulings.json","w"), indent=0)
print(len(ph), 'dev', len(dev), 'final', len(final), collections.Counter(L[i]['primary_origin'] for i in dev).most_common(5))
