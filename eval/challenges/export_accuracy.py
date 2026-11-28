"""Copy explicitly selected completed studies into the client evidence bundle."""
import argparse
import hashlib
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


def build():
    index = json.loads((ROOT / 'eval/challenges/accuracy-sources.json').read_text())
    if index.get('schema_version') != 1 or not isinstance(index.get('sources'), list):
        raise ValueError('invalid challenge source index')
    selected, ids, paths = [], set(), set()
    for name in index['sources']:
        if not isinstance(name, str) or not name.startswith('eval/challenges/') or '..' in name or not name.endswith('.json') or name in paths:
            raise ValueError('invalid or repeated challenge source path')
        paths.add(name)
        path = (ROOT / name).resolve()
        if not path.is_relative_to((ROOT / 'eval/challenges').resolve()):
            raise ValueError('challenge source escapes its data directory')
        raw = path.read_bytes()
        doc = json.loads(raw)
        if doc.get('schema_version') != 1 or doc.get('evidence_kind') != 'source_grounded_synthetic' or not isinstance(doc.get('features'), list):
            raise ValueError('source is not a measured synthetic challenge summary')
        for feature in doc['features']:
            if feature['id'] in ids:
                raise ValueError('repeated feature across selected studies')
            ids.add(feature['id'])
            test = feature['test']
            if test['cases'] < 100 or test['matched'] + test['mismatched'] + test['errors'] != test['cases']:
                raise ValueError('undersized or incomplete challenge source')
            if feature.get('simulated') or feature.get('llm_provider') in ['fake', 'vertex']:
                raise ValueError('unapproved execution mode')
        selected.append({'sourcePath': name, 'sourceSHA256': hashlib.sha256(raw).hexdigest(), 'summary': doc})
    return {'schema_version': 1, 'sources': selected}


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--check', action='store_true')
    args = parser.parse_args()
    text = json.dumps(build(), indent=2, ensure_ascii=False) + '\n'
    out = ROOT / 'frontend/src/lib/challenge-data.json'
    if args.check:
        if out.read_text() != text:
            raise SystemExit('Accuracy challenge bundle differs from selected source studies')
        print('Accuracy challenge bundle matches')
    else:
        out.write_text(text)
        print('wrote Accuracy challenge bundle')
