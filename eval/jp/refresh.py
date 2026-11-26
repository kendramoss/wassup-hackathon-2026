"""Scheduled refresh of the Japan Customs origin rulings (D931, #883).

Usage: python3 -I -B eval/jp/refresh.py <repo> <raw_dir> [--seed <earlier_raw_dir>] [--date YYYY-MM-DD]

Re-reads the published list with crawl.py (its etiquette: robots.txt, at most
three connections, requests at least 1.1 s apart), fetching only detail pages
not already in <raw_dir>; with --seed, the earlier snapshot's detail pages are
copied into an empty <raw_dir> first, so only new rulings are downloaded.
parse.py then parses the whole list into a scratch directory, and the result
is compared with every ruling the repo already holds (data/jp/rulings.jsonl
and each data/jp/fresh-*/rulings.jsonl).

Writes eval/refresh/<date>/jp_report.json (counts, new and withdrawn ids, the
checks of each new ruling) and eval/refresh/<date>/jp_new_rulings.jsonl (the
parsed new rulings, Japanese text as published, PDL1.0). Raw HTML stays in
<raw_dir> and is not committed. No model calls and no labels.
"""
import datetime, glob, json, os, shutil, subprocess, sys, tempfile

REQUIRED = ("id", "date", "agreement", "hs", "product_ja", "reasoning_ja", "answer_ja")


def rows(p):
    with open(p, encoding="utf-8") as f:
        return [json.loads(l) for l in f if l.strip()]


def known_ids(repo):
    ids = {r["id"] for r in rows(os.path.join(repo, "data/jp/rulings.jsonl"))}
    for p in sorted(glob.glob(os.path.join(repo, "data/jp/fresh-*/rulings.jsonl"))):
        ids |= {r["id"] for r in rows(p)}
    return ids


def check(r):
    """Reasons a parsed ruling cannot enter a test set; empty when it can."""
    reasons = [f"missing {k}" for k in REQUIRED if not str(r.get(k) or "").strip()]
    try:
        datetime.date.fromisoformat(r.get("date") or "")
    except ValueError:
        reasons.append("date is not YYYY-MM-DD")
    return reasons


def compare(parsed, known):
    """New rulings (with their checks) and known ids absent from the list."""
    ids = {r["id"] for r in parsed}
    new = sorted((r for r in parsed if r["id"] not in known), key=lambda r: r["id"])
    return new, sorted(known - ids)


def report(date, parsed, new, withdrawn, meta):
    items = [{"id": r["id"], "date": r.get("date"), "agreement": r.get("agreement"),
              "valid": not check(r), "reasons": check(r)} for r in new]
    valid = [i for i in items if i["valid"]]
    agreements = {}
    for i in valid:
        agreements[i["agreement"]] = agreements.get(i["agreement"], 0) + 1
    return {
        "date": date,
        "list_total": meta.get("list_total"),
        "retrieved_start_utc": meta.get("retrieved_start_utc"),
        "retrieved_end_utc": meta.get("retrieved_end_utc"),
        "parsed": len(parsed),
        "new": len(new),
        "valid": len(valid),
        "withdrawn_from_list": len(withdrawn),
        "withdrawn_ids": withdrawn,
        "date_min": min((i["date"] for i in valid), default=None),
        "date_max": max((i["date"] for i in valid), default=None),
        "agreements": dict(sorted(agreements.items())),
        "items": items,
    }


def main():
    if len(sys.argv) < 3:
        sys.exit(__doc__)
    repo, raw = sys.argv[1], sys.argv[2]
    date = sys.argv[sys.argv.index("--date") + 1] if "--date" in sys.argv else datetime.datetime.now(datetime.timezone.utc).date().isoformat()
    here = os.path.dirname(os.path.abspath(__file__))
    os.makedirs(os.path.join(raw, "detail"), exist_ok=True)
    if "--seed" in sys.argv and not os.listdir(os.path.join(raw, "detail")):
        seed = os.path.join(sys.argv[sys.argv.index("--seed") + 1], "detail")
        for f in os.listdir(seed):
            if f.endswith(".html"):
                shutil.copy2(os.path.join(seed, f), os.path.join(raw, "detail", f))
    subprocess.run([sys.executable, "-I", "-B", os.path.join(here, "crawl.py"), raw], check=True)
    with tempfile.TemporaryDirectory() as tmp:
        subprocess.run([sys.executable, "-I", "-B", os.path.join(here, "parse.py"), raw, tmp], check=True)
        parsed = rows(os.path.join(tmp, "data/jp/rulings.jsonl"))
    meta = json.load(open(os.path.join(raw, "crawl-meta.json"), encoding="utf-8"))
    new, withdrawn = compare(parsed, known_ids(repo))
    out = os.path.join(repo, "eval/refresh", date)
    os.makedirs(out, exist_ok=True)
    rep = report(date, parsed, new, withdrawn, meta)
    with open(os.path.join(out, "jp_report.json"), "w", encoding="utf-8") as f:
        json.dump(rep, f, ensure_ascii=False, indent=1)
        f.write("\n")
    with open(os.path.join(out, "jp_new_rulings.jsonl"), "w", encoding="utf-8") as f:
        for r in new:
            f.write(json.dumps(r, ensure_ascii=False, sort_keys=True) + "\n")
    print(f"jp refresh: {rep['parsed']} on the list, {rep['new']} new, {rep['valid']} pass the checks, "
          f"{rep['withdrawn_from_list']} withdrawn -> {os.path.join(out, 'jp_report.json')}")


if __name__ == "__main__":
    main()
