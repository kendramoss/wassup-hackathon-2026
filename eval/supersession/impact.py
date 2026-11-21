"""Offline effect of the supersession guard (D320) on committed results.

For every committed result row that shows or cites earlier CBP rulings, count
the rulings that CBP had revoked or modified before that row's case date
(backend/internal/domain/supersession.json). A change dated on the case date
itself does not count (D370): when the case is the revoking ruling, the
ruling it revokes was still CBP's position until this case was decided. Those are the citations the guard
would now refuse. No model calls; nothing is re-scored.

Run from the repository root:
    python3 -I -B eval/supersession/impact.py .          # write
    python3 -I -B eval/supersession/impact.py . --check  # verify
"""
import glob
import json
import os
import sys

OUT = "eval/results/supersession-impact.json"
MAP = "backend/internal/domain/supersession.json"
# The heat-gun demo's evidence bound: rulings dated before 14 Jan 2026, the
# request date of N357771 (backend/internal/demos/fixtures/heatgun_2022.json
# "notes"). Its plan, ask and seed files carry no per-row date.
HEATGUN_BOUND = "2026-01-14"
FLIP_DATA = ["eval/data/flips.jsonl", "eval/data/flips_post.jsonl", "eval/data/flips_post_addendum.jsonl"]


def load_map(root):
    with open(os.path.join(root, MAP)) as f:
        m = json.load(f)
    first = {}
    for e in m["edges"]:
        cur = first.get(e["ruling"])
        if cur is None or e["date"] < cur["date"]:
            first[e["ruling"]] = e
    return m["version"], first


def flip_bounds(root):
    """The verifier's date bound per flip pair: the earlier of the pair date
    and both rulings' dates (cmd/eval flips)."""
    out = {}
    for p in FLIP_DATA:
        with open(os.path.join(root, p)) as f:
            for line in f:
                r = json.loads(line)
                ds = [r.get("date")] + [r.get(v, {}).get("date") for v in ("variant_a", "variant_b")]
                out[r["id"]] = min(d for d in ds if d)
    return out


def rows(path):
    with open(path) as f:
        if path.endswith(".jsonl"):
            for line in f:
                line = line.strip()
                if line:
                    yield json.loads(line)
            return
        doc = json.load(f)
    stack = [doc]
    while stack:
        x = stack.pop()
        if isinstance(x, dict):
            yield x
            stack.extend(x.values())
        elif isinstance(x, list):
            stack.extend(x)


def as_list(v):
    if isinstance(v, str):
        return [v] if v else []
    if isinstance(v, list):
        return [s for s in v if isinstance(s, str)]
    return []


def main(root, check):
    version, first = load_map(root)
    bounds = flip_bounds(root)
    files = sorted(glob.glob(os.path.join(root, "eval/results/*.json")) + glob.glob(os.path.join(root, "eval/results/*.jsonl")))
    per_file = {}
    hits = []
    totals = {"rows_with_rulings": 0, "rows_showing_superseded": 0, "rows_citing_superseded": 0,
              "supported_verdicts_on_superseded": 0, "rows_undated": 0,
              "undated_rows_naming_any_superseded": 0,
              # D400: edges read from Customs Bulletin notices, any date and
              # before the case date.
              "rows_naming_bulletin_ruling_any_date": 0,
              "rows_showing_or_citing_bulletin_superseded": 0}
    bulletin_named = {}
    for path in files:
        # JSON keys must match the Linux-generated artifact on every platform.
        rel = os.path.relpath(path, root).replace(os.sep, "/")
        if rel == OUT:
            continue
        f = {"rows_with_rulings": 0, "rows_showing_superseded": 0, "rows_citing_superseded": 0, "supported_verdicts_on_superseded": 0, "rows_undated": 0}
        for r in rows(path):
            shown = as_list(r.get("precedents"))
            cited = as_list(r.get("cited")) + as_list(r.get("cited_rulings"))
            support = as_list(r.get("supporting_ruling"))
            counter = as_list(r.get("counter_ruling"))
            if not (shown or cited or support or counter):
                continue
            named_b = sorted({i for i in shown + cited + support + counter if i in first and first[i].get("source") == "customs_bulletin"})
            if named_b:
                totals["rows_naming_bulletin_ruling_any_date"] += 1
                for i in named_b:
                    bulletin_named[i] = bulletin_named.get(i, 0) + 1
            date = r.get("date") or r.get("precedents_before") or bounds.get(r.get("id", ""))
            if not date and "heatgun" in os.path.basename(path):
                date = HEATGUN_BOUND
            f["rows_with_rulings"] += 1
            if not date:
                # Plans and demo seeds carry no per-row date: count any
                # superseded ruling at all (an upper bound).
                f["rows_undated"] += 1
                if any(i in first for i in shown + cited + support + counter):
                    totals["undated_rows_naming_any_superseded"] += 1
                continue
            def sup(ids):
                return [i for i in ids if i in first and first[i]["date"] < date]
            s_shown, s_cited = sup(shown), sup(cited + support + counter)
            if s_shown:
                f["rows_showing_superseded"] += 1
            if s_cited:
                f["rows_citing_superseded"] += 1
            s_support = sup(support)
            if s_support and r.get("precedent_supported"):
                f["supported_verdicts_on_superseded"] += 1
            if any(first[i].get("source") == "customs_bulletin" for i in s_shown + s_cited):
                totals["rows_showing_or_citing_bulletin_superseded"] += 1
            if s_shown or s_cited:
                hits.append({"file": rel, "row": r.get("ruling") or r.get("id"), "case_date": date,
                             "shown_superseded": s_shown, "cited_superseded": s_cited,
                             "supporting_superseded": s_support,
                             "precedent_supported": r.get("precedent_supported"), "correct": r.get("correct"),
                             "superseded_by": {i: first[i]["superseded_by"] + " " + first[i]["kind"] + " " + first[i]["date"] for i in sorted(set(s_shown + s_cited))}})
        if f["rows_with_rulings"]:
            per_file[rel] = f
            for k in f:
                totals[k] += f[k]
    out = {
        "note": "Offline count (D320, D370): committed result rows that showed or cited a CBP ruling revoked or modified before the row's case date (a change dated on the case date does not count). "
                "No model calls; nothing re-scored. The engine rows would now be predicted without those precedents; the effect on accuracy is not measured. "
                "Edges read from Customs Bulletin notices (D400) are dated by their effective date; bulletin_rulings_named counts rows that name such a ruling at any date.",
        "map_version": version,
        "files_with_rulings": len(per_file),
        "totals": totals,
        "files": {k: v for k, v in per_file.items() if v["rows_showing_superseded"] or v["rows_citing_superseded"]},
        "undated_files": {k: v["rows_undated"] for k, v in per_file.items() if v["rows_undated"]},
        "rows": sorted(hits, key=lambda h: (h["file"], str(h["row"]))),
        "bulletin_rulings_named": {i: {"rows": n, "superseded_by": first[i]["superseded_by"], "effective": first[i]["date"]} for i, n in sorted(bulletin_named.items())},
    }
    text = json.dumps(out, indent=1, sort_keys=True) + "\n"
    dest = os.path.join(root, OUT)
    if check:
        with open(dest) as fh:
            if fh.read() != text:
                sys.exit(f"{OUT} is stale: run python3 -I -B eval/supersession/impact.py .")
        print(f"{OUT} matches ({totals['rows_showing_superseded']} rows showed, {totals['rows_citing_superseded']} cited a superseded ruling)")
        return
    with open(dest, "w") as fh:
        fh.write(text)
    print(json.dumps(totals))


if __name__ == "__main__":
    main(sys.argv[1] if len(sys.argv) > 1 else ".", "--check" in sys.argv)
