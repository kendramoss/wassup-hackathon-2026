"""Check the AJCEP general-rule source PDFs against the pinned digests.

Usage: python3 -I -B eval/jp/verify_ajcep_general.py <pdf-directory>
The directory must hold asean1.pdf, asean2-1.pdf and asean3.pdf downloaded
from https://www.customs.go.jp/roo/english/text/. The digests are read from
backend/internal/jprules/data/ajcep_general.json. Nothing is executed.
"""
import hashlib
import json
import sys
from pathlib import Path

DATA = Path(__file__).resolve().parents[2] / "backend/internal/jprules/data/ajcep_general.json"


def main(directory):
    bundle = json.loads(DATA.read_text(encoding="utf-8"))
    ok = True
    for src in bundle["sources"]:
        path = Path(directory) / src["name"]
        digest = hashlib.sha256(path.read_bytes()).hexdigest() if path.is_file() else "missing"
        match = digest == src["sha256"]
        ok &= match
        print(f"{'ok' if match else 'MISMATCH'}  {src['name']}  {digest}")
    return 0 if ok else 1


if __name__ == "__main__":
    if len(sys.argv) != 2:
        sys.exit(__doc__)
    sys.exit(main(sys.argv[1]))
