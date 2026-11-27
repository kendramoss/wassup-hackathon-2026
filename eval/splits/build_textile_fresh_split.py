# Builds eval/splits/splits_textile_fresh.json (D955): two textile 19 CFR 102.21 sets that the
# rule layer (D951) was neither written nor tuned from and that D952 did not score. No model call.
# Usage: python3 -I eval/splits/build_textile_fresh_split.py . OLDER_RAW_DIR CORPUS_RAW_DIR --frozen-at <iso>
#   OLDER_RAW_DIR: raw CROSS JSON of the 2019-2022 New York rulings fetched by number (D907, not committed)
#   CORPUS_RAW_DIR: raw CROSS JSON of the 2023-2026 corpus (not committed)
import json, os, re, subprocess, sys, collections
root, older_raw, corpus_raw = sys.argv[1], sys.argv[2], sys.argv[3]
frozen = sys.argv[sys.argv.index('--frozen-at') + 1]
def jl(p): return {json.loads(l)['ruling']: json.loads(l) for l in open(p) if l.strip()}
older_l = jl(f'{root}/backend/data/labels_older.jsonl')
main_l = jl(f'{root}/eval/data/labels.jsonl')
sp = json.load(open(f'{root}/eval/splits/splits_textile.json'))
f2 = json.load(open(f'{root}/eval/splits/splits_textile_final2.json'))['final2']
used = set(sp['dev']) | set(sp['final']) | set(f2)  # development and the D952 held-out
# A ruling number written anywhere in the rule code, its tests or the earlier entries is left out
# (the writer may have read it), and so is every ruling the earlier textile steps listed.
mention_dirs = ['backend/internal/textilerules', 'backend/cmd/textilerules', 'docs/DECISIONS.md', 'eval/textile102', 'frontend/src', 'frontend/scripts']
mentioned = set(subprocess.run(['grep', '-rhoE', '[NH][0-9]{6}'] + [f'{root}/{d}' for d in mention_dirs],
                               capture_output=True, text=True).stdout.split())
left = collections.OrderedDict()
def cites(path): return '102.21' in open(path, errors='ignore').read()
def ok(r, rid, facts_dir):
    if not r: return 'no label row'
    if r['regime'] != 'part102_tariff_shift': return 'regime ' + r['regime']
    if not r.get('primary_origin'): return 'no single held origin'
    if not os.path.exists(f'{root}/{facts_dir}/{rid}.txt'): return 'no facts file'
    if rid in used: return 'development or D952 held-out'
    if rid in mentioned: return 'named in the rule code, tests or earlier entries'
    return ''
older, pool = [], []
for f in sorted(os.listdir(older_raw)):
    rid = f[:-5]
    if not cites(f'{older_raw}/{f}'): continue
    r = older_l.get(rid)
    why = ok(r, rid, 'backend/data/facts_older') if r else 'in another labels file (not a textile pool row)'
    if why: left[rid] = why
    else: older.append(r)
for rid in sp['textile']:  # 2023-01 to 2025-06 corpus rulings citing 102.21 with an origin and facts
    r = main_l[rid]
    why = ok(r, rid, 'backend/data/facts')
    if not os.path.exists(f'{corpus_raw}/{rid}.json'): why = why or 'no raw text'
    if why:
        if why != 'development or D952 held-out': left[rid] = why
    else: pool.append(r)
older.sort(key=lambda r: (r['date'], r['ruling'])); pool.sort(key=lambda r: (r['date'], r['ruling']))
out = {"source": "rulings citing 19 CFR 102.21, regime part102_tariff_shift, one held origin, a facts file; not in the development or D952 held-out splits and not named in the rule code, its tests or earlier entries (D955)",
       "frozen_at": frozen,
       "older": [r['ruling'] for r in older], "pool": [r['ruling'] for r in pool],
       "older_range": [older[0]['date'], older[-1]['date']], "pool_range": [pool[0]['date'], pool[-1]['date']],
       "left_out": left}
json.dump(out, open(f'{root}/eval/splits/splits_textile_fresh.json', 'w'), indent=1)
print(len(older), len(pool), out['older_range'], out['pool_range'], collections.Counter(left.values()))
