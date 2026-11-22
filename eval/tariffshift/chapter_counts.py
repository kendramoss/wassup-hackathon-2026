"""HTS chapters of the goods in the Part 102 development and earlier data (D460).

Counts the chapter of each good's HTS code, to pick which 19 CFR 102.20
chapters the rule check encodes. Development and earlier data only, never
held-out rows:
  - dev: CBP's code for the good in the D361 full-text extraction
    (eval/tariffshift/inputs-dev-full_text.jsonl);
  - earlier: the first "heading"/"subheading" code in the facts of the
    labelled Part 102 rulings dated before 2024 (eval/splits/splits_usmca.json
    "usmca", dates from eval/data/labels_usmca.jsonl, backend/data/facts).
    A ruling in the development or a held-out split (dev, final, final2) is
    never read as earlier; the script stops if one would be (D560, #619);
  - families (informational): the top suggested code for the good of the 23
    per-family fixtures (eval/tariffshift/parts/suggest-dev-v1.jsonl).
No model calls.

D560 (#619): the D460 pick read the whole "usmca" list as the earlier pool,
which held the 72 development and all 32 held-out "final" rulings. Chapters
15, 21, 40, 71 and 95 reached the threshold only through that. They stay
encoded (regulation text, nothing tuned) and are listed apart as
chapters_kept_d460; chapters_added is the pick on development and earlier
data only.

Run from the repository root:
    python3 -I -B eval/tariffshift/chapter_counts.py .          # write
    python3 -I -B eval/tariffshift/chapter_counts.py . --check  # verify
"""
import collections
import json
import os
import re
import sys

OUT = "eval/tariffshift/chapter-counts.json"
# The chapters encoded before D460 (D360).
BEFORE = ["73", "82", "84", "85", "87", "90", "94"]
# Picked in D460 from a pool that read held-out facts; kept encoded (D560).
KEPT_D460 = ["15", "21", "40", "71", "95"]
CUTOFF = "2024"
THRESHOLD = 4
CODE = re.compile(r"(?:subheading|heading)\s+(\d{4}(?:\.\d{2}(?:\.\d{2,4})?)?)", re.I)


def chapter(code):
    d = re.sub(r"\D", "", code or "")
    return d[:2] if len(d) >= 2 else None


def jsonl(path):
    with open(path) as f:
        return [json.loads(line) for line in f if line.strip()]


def earlier_pool(root):
    """Labelled Part 102 rulings dated before 2024, none in dev, final or final2."""
    with open(os.path.join(root, "eval/splits/splits_usmca.json")) as f:
        splits = json.load(f)
    with open(os.path.join(root, "eval/splits/splits_usmca_final2.json")) as f:
        final2 = json.load(f)["final2"]
    date = {r["ruling"]: r["date"] for r in jsonl(os.path.join(root, "eval/data/labels_usmca.jsonl"))}
    held = set(splits["dev"]) | set(splits["final"]) | set(final2)
    pool = [rid for rid in splits["usmca"] if date[rid] < CUTOFF and rid not in held]
    bad = sorted(set(pool) & held)
    if bad:
        sys.exit("development or held-out rulings in the earlier pool: " + ", ".join(bad))
    return pool


def build(root):
    dev = [chapter(r["input"].get("good_hts")) for r in jsonl(os.path.join(root, "eval/tariffshift/inputs-dev-full_text.jsonl"))]
    pool = earlier_pool(root)
    earlier = []
    for rid in pool:
        with open(os.path.join(root, "backend/data/facts", rid + ".txt")) as f:
            m = CODE.search(f.read())
        earlier.append(chapter(m.group(1)) if m else None)
    fam = []
    for r in jsonl(os.path.join(root, "eval/tariffshift/parts/suggest-dev-v1.jsonl")):
        if r["set"] != "family":
            continue
        c = (r.get("good") or {}).get("candidates") or []
        fam.append(chapter(c[0]["hts"]) if c else None)

    def count(xs):
        return dict(sorted(collections.Counter(x for x in xs if x).items()))

    both = collections.Counter(x for x in dev + earlier if x)
    picked = sorted(c for c, n in both.items() if n >= THRESHOLD and c not in BEFORE and not ("50" <= c <= "63"))
    return {
        "note": "Chapter of each good's HTS code in development and earlier Part 102 data (labelled Part 102 rulings dated before 2024, none in dev, final or final2), no held-out rows (D460, D560). Added: every chapter outside the D360 seven with at least %d goods in dev + earlier, textile chapters 50-63 excluded. chapters_kept_d460: picked in D460 from a pool that read the development and held-out rulings too (#619); kept encoded, not picked here. Recompute: python3 -I -B eval/tariffshift/chapter_counts.py . --check" % THRESHOLD,
        "dev": {"n": len(dev), "with_code": sum(1 for x in dev if x), "chapters": count(dev)},
        "earlier": {"n": len(earlier), "with_code": sum(1 for x in earlier if x), "chapters": count(earlier), "rulings": pool},
        "families": {"n": len(fam), "with_code": sum(1 for x in fam if x), "chapters": count(fam)},
        "dev_and_earlier_in_scope_before": sum(n for c, n in both.items() if c in BEFORE),
        "dev_and_earlier_with_code": sum(both.values()),
        "chapters_before": BEFORE,
        "threshold": THRESHOLD,
        "chapters_added": picked,
        "chapters_kept_d460": [c for c in KEPT_D460 if c not in picked],
        "chapters_encoded": sorted(set(BEFORE) | set(picked) | set(KEPT_D460)),
    }


def main():
    root = sys.argv[1]
    out = json.dumps(build(root), indent=1) + "\n"
    path = os.path.join(root, OUT)
    if "--check" in sys.argv:
        with open(path) as f:
            if f.read() != out:
                sys.exit(OUT + " is stale; run python3 -I -B eval/tariffshift/chapter_counts.py .")
        return
    with open(path, "w") as f:
        f.write(out)


if __name__ == "__main__":
    main()
