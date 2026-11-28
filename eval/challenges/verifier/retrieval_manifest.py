"""Hash the exact local retrieval inputs without copying public ruling archives."""
import argparse
import hashlib
import json
from pathlib import Path


def build(labels, facts, raw):
    label_bytes = labels.read_bytes()
    ids = sorted({json.loads(line)['ruling'] for line in label_bytes.splitlines()})
    rows = []
    for id in ids:
        entry = {'ruling': id}
        for key, path in [('facts', facts / (id + '.txt')), ('raw', raw / (id + '.json'))]:
            entry[key] = hashlib.sha256(path.read_bytes()).hexdigest() if path.exists() else None
        rows.append(entry)
    return {'labels_sha256': hashlib.sha256(label_bytes).hexdigest(), 'rulings': len(rows),
            'facts_present': sum(r['facts'] is not None for r in rows),
            'raw_present': sum(r['raw'] is not None for r in rows),
            'content_manifest_sha256': hashlib.sha256(json.dumps(rows, sort_keys=True).encode()).hexdigest()}


if __name__ == '__main__':
    p = argparse.ArgumentParser()
    p.add_argument('--labels', type=Path, required=True)
    p.add_argument('--facts', type=Path, required=True)
    p.add_argument('--raw', type=Path, required=True)
    p.add_argument('--check', type=Path)
    a = p.parse_args()
    result = build(a.labels, a.facts, a.raw)
    if a.check:
        if json.loads(a.check.read_text()) != result:
            raise SystemExit('retrieval archive changed during study')
        print('retrieval manifest matches')
    else:
        print(json.dumps(result, indent=2))
