"""Freeze one month's re-scoring set (D931, #884).

Usage: python3 -I -B eval/rescore/build_split.py <repo> --month YYYY-MM [--frozen-at ISO]

Needs eval/rescore/<month>/labels.jsonl and facts/ from
`go run ./cmd/corpus final2-label` run on the month's refresh index (see
docs/REFRESH.md). Applies the D150 rule unchanged (the rule that reproduces
both earlier held-out splits): substantial-transformation regime, facts naming
two or more countries, a single held origin ("A | B" excluded), a facts file.
Rulings already in any earlier label file are refused, so a month never
repeats a ruling scored before. Writes eval/rescore/<month>/splits.json with
the split under "monthly", the funnel and frozen_at.

Unlike D150, nobody reads each holding by hand: labels are the regex
labeller's (D150's label check: held origin matched on all 1,620 regex-labelled
rulings). A ruling without a single regex origin is dropped and counted.
"""
import collections, glob, json, os, sys

EARLIER = ("eval/data/labels.jsonl", "eval/data/labels_usmca.jsonl", "eval/data/labels_final2.jsonl",
           "eval/data/labels_usmca_final2.jsonl")


def rows(p):
    with open(p, encoding="utf-8") as f:
        return [json.loads(l) for l in f if l.strip()]


def select(labels, has_facts):
    """The D150 rule. Returns the kept labels (sorted) and the funnel."""
    steps = collections.OrderedDict(labelled_rulings=len(labels))
    st = [r for r in labels if r["regime"] == "substantial_transformation"]
    steps["substantial_transformation"] = len(st)
    multi = [r for r in st if r["multi_country"]]
    steps["multi_country"] = len(multi)
    held = [r for r in multi if r["primary_origin"]]
    steps["with_held_origin"] = len(held)
    single = [r for r in held if "|" not in r["primary_origin"]]
    steps["excluded_two_or_more_countries_held"] = len(held) - len(single)
    keep = sorted((r for r in single if has_facts(r["ruling"])), key=lambda r: (r["date"], r["ruling"]))
    steps["without_facts_file"] = len(single) - len(keep)
    steps["in_split"] = len(keep)
    return keep, steps


def earlier_ids(repo, month):
    ids = set()
    for p in EARLIER:
        if os.path.exists(os.path.join(repo, p)):
            ids |= {r["ruling"] for r in rows(os.path.join(repo, p))}
    for p in glob.glob(os.path.join(repo, "eval/rescore/*/labels.jsonl")):
        if os.path.basename(os.path.dirname(p)) < month:
            ids |= {r["ruling"] for r in rows(p)}
    return ids


def main():
    if "--month" not in sys.argv:
        sys.exit(__doc__)
    repo = sys.argv[1]
    month = sys.argv[sys.argv.index("--month") + 1]
    frozen_at = sys.argv[sys.argv.index("--frozen-at") + 1] if "--frozen-at" in sys.argv else None
    d = os.path.join(repo, "eval/rescore", month)
    labels = rows(os.path.join(d, "labels.jsonl"))
    seen = earlier_ids(repo, month) & {r["ruling"] for r in labels}
    if seen:
        sys.exit(f"rulings already labelled in an earlier set: {sorted(seen)}")

    def has_facts(i):
        p = os.path.join(d, "facts", i + ".txt")
        return os.path.exists(p) and os.path.getsize(p) > 1

    keep, steps = select(labels, has_facts)
    out = {
        "source": f"CBP rulings found by the refresh of {month} (eval/refresh/*/cbp_probe.jsonl), labelled by "
                  "go run ./cmd/corpus final2-label (regex labels, no hand reading)",
        "frozen_at": frozen_at,
        "rule": "D150 rule: substantial-transformation regime, facts naming two or more countries, a single held "
                "origin ('A | B' excluded), a facts section; no ruling from an earlier label file. Frozen before "
                "scoring; production configuration scored once (-confirm-final).",
        "month": month,
        "date_min": keep[0]["date"] if keep else None,
        "date_max": keep[-1]["date"] if keep else None,
        "funnel": steps,
        "label_method": dict(collections.Counter(r["method"] for r in keep)),
        "origins": dict(collections.Counter(r["primary_origin"] for r in keep).most_common()),
        "monthly": [r["ruling"] for r in keep],
    }
    with open(os.path.join(d, "splits.json"), "w", encoding="utf-8") as f:
        json.dump(out, f, indent=1, ensure_ascii=False)
        f.write("\n")
    print(json.dumps({k: out[k] for k in ("month", "date_min", "date_max", "funnel")}, indent=1))


if __name__ == "__main__":
    main()
