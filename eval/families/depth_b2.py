"""Tracks 3-6 (#766-#769): pools, keyword hits and frozen case lists for the
battery, machinery, furniture and medical/optical family work (D780).

No model calls. Reads only the ruling IDs of held-out lists (to exclude
them), never their labels, facts or results.

Universe: labelled substantial-transformation rulings (eval/data/labels.jsonl)
with a facts file, dated before 2026, in no final, final2, pilot or
tune_check list of any split file (eval/splits/*.json, eval/splits/routed/*.json)
and not among the IDs of eval/data/labels_final2.jsonl,
eval/data/labels_usmca_final2.jsonl or eval/data/facts_final2. Pools:
  dev   = universe ∩ splits_final.json "dev"
  train = universe ∩ splits_tune.json "train"
  other = universe − dev − train
Templates cite train and other only; dev rulings are regression cases.

Run from the repository root:
    python3 -I -B eval/families/depth_b2.py . hits     # write eval/families/b2-hits.json
    python3 -I -B eval/families/depth_b2.py . freeze   # write eval/families/b2-cases.json from b2-screen.json
    python3 -I -B eval/families/depth_b2.py . check    # verify both are current and the v2 citations (D782)
"""
import glob
import json
import os
import re
import sys

CUTOFF = "2026"
HELD = ("final", "final2", "pilot", "tune_check")
HITS = "eval/families/b2-hits.json"
SCREEN = "eval/families/b2-screen.json"
CASES = "eval/families/b2-cases.json"
HEAD = 600  # characters of facts read with the subject by the keyword pass

# D780: fixed before any count was taken with them.
PATTERNS = {
    "batteries": r"batter(y|ies)|lithium[- ]ion|\bli-ion\b|power bank|energy storage|\bbms\b|battery management|\bcells?\b[^.]{0,40}\b(lithium|battery|batteries|ion)\b",
    "machinery": r"\bmachin(e|es|ery)\b|\bpumps?\b|compressor|\bmotors?\b|gear ?box|reducer|conveyor|\brobot|\bcnc\b|\blathe|\bgenerators?\b|\bengines?\b|actuator|hydraulic|pneumatic|spindle|welder|\bpress(es)?\b|industrial|\bvalves?\b|\bblowers?\b|\bfans?\b|\bwinch",
    "furniture": r"furniture|\bchairs?\b|\btables?\b|\bdesks?\b|\bsofas?\b|\bcouch|\bcabinets?\b|shel(f|ves|ving)\b|\bbeds?\b|bed frame|mattress|\bstools?\b|\bbench(es)?\b|ottoman|wardrobe|dresser|recliner|\bvanit(y|ies)\b|bookcase|nightstand|futon|headboard|seating",
    "medical_optical": r"medical|surgical|syringe|needle|catheter|cannula|\bstent|implant|orthop(a)?edic|dental|diagnostic|glucose|thermometer|oximeter|stethoscope|wound|infusion|endoscop|\blens(es)?\b|optical|microscope|telescope|binocular|riflescope|rifle scope|spectacle|eyeglass|eyewear|sunglass|contact lens|\bprism|rangefinder|precision instrument|measuring instrument|caliper|hearing aid|blood pressure|sphygmomanometer|nebuli[sz]er|ventilator|\bx-ray\b|ultrasound",
}


def jsonl(path):
    with open(path) as f:
        return [json.loads(line) for line in f if line.strip()]


def held_ids(root):
    ids = set()
    for p in sorted(glob.glob(os.path.join(root, "eval/splits/*.json")) + glob.glob(os.path.join(root, "eval/splits/routed/*.json"))):
        with open(p) as f:
            d = json.load(f)
        for k in HELD:
            if isinstance(d.get(k), list):
                ids |= set(d[k])
    # Held-out label files: read the ruling ID of each line and nothing else.
    for p in ("eval/data/labels_final2.jsonl", "eval/data/labels_usmca_final2.jsonl"):
        with open(os.path.join(root, p)) as f:
            for line in f:
                m = re.search(r'"ruling"\s*:\s*"([HN]\d{6})"', line)
                if m:
                    ids.add(m.group(1))
    d2 = os.path.join(root, "eval/data/facts_final2")
    if os.path.isdir(d2):
        ids |= {n.split(".")[0] for n in os.listdir(d2)}
    return ids


def universe(root):
    lab = jsonl(os.path.join(root, "eval/data/labels.jsonl"))
    facts = {n[:-4] for n in os.listdir(os.path.join(root, "backend/data/facts")) if n.endswith(".txt")}
    held = held_ids(root)
    with open(os.path.join(root, "eval/splits/splits_final.json")) as f:
        dev = set(json.load(f)["dev"])
    with open(os.path.join(root, "eval/splits/splits_tune.json")) as f:
        train = set(json.load(f)["train"])
    out = {}
    for l in lab:
        r = l["ruling"]
        if r in held or l["regime"] != "substantial_transformation" or r not in facts or l["date"] >= CUTOFF:
            continue
        out[r] = dict(l, pool="dev" if r in dev else "train" if r in train else "other")
    if set(out) & held:
        sys.exit("held-out ruling in the universe")
    return out, len(held)


def facts_head(root, r, n=HEAD):
    with open(os.path.join(root, "backend/data/facts", r + ".txt")) as f:
        return f.read(n)


def hits(root):
    uni, nheld = universe(root)
    pats = {k: re.compile(v, re.I) for k, v in PATTERNS.items()}
    fams = {k: [] for k in PATTERNS}
    for r in sorted(uni):
        l = uni[r]
        text = (l.get("subject") or "") + " | " + facts_head(root, r)
        for k, p in pats.items():
            if p.search(text):
                fams[k].append({"ruling": r, "pool": l["pool"], "date": l["date"], "origin": l.get("primary_origin"),
                                "multi_country": l.get("multi_country"), "subject": l.get("subject")})
    counts = {k: {p: sum(1 for h in v if h["pool"] == p) for p in ("dev", "train", "other")} for k, v in fams.items()}
    return {"rule": "D780 keyword pass: pattern over subject + first %d characters of facts; universe and pools as in this "
                    "script's docstring. A hit is a candidate only; the hand screen (b2-screen.json) decides." % HEAD,
            "universe": len(uni), "excluded_held_out_ids": nheld,
            "pools": {p: sum(1 for l in uni.values() if l["pool"] == p) for p in ("dev", "train", "other")},
            "patterns": PATTERNS, "counts": counts, "hits": fams}


def freeze(root):
    uni, _ = universe(root)
    with open(os.path.join(root, SCREEN)) as f:
        scr = json.load(f)
    out = {"rule": scr["rule"], "frozen_at": scr["frozen_at"], "families": {}}
    for fam, rows in scr["families"].items():
        for row in rows:
            if row["ruling"] not in uni:
                sys.exit(f"{fam}: {row['ruling']} is outside the universe")
        keep = [r for r in rows if r["keep"]]
        out["families"][fam] = {
            "regression_dev": sorted(r["ruling"] for r in keep if uni[r["ruling"]]["pool"] == "dev"),
            "citable_pool": sorted(r["ruling"] for r in keep if uni[r["ruling"]]["pool"] != "dev"),
            "screened_out": sorted(r["ruling"] for r in rows if not r["keep"]),
            "ambiguous": sorted(r["ruling"] for r in rows if not r["keep"] and r.get("ambiguous")),
        }
    return out


def later_pairs(root):
    """Pairings a later round records with its own check (D863, depth_b4.py)."""
    p = os.path.join(root, "eval/families/b4-citations.json")
    if not os.path.exists(p):
        return set()
    with open(p) as f:
        return {(c["template"], w + ":" + c["ruling"]) for c in json.load(f)["citations"] for w in c["cited_in"]}


def check_citations(root):
    """Every group/outcome pairing a v2 template has that v1 did not (D786),
    whether the ruling is new or reused, is a ruling of a frozen citable pool
    with its held-origin sentence in the bundled labels, and is recorded in
    b2-citations.json ("citations" for new rulings, "reused" for rulings v1
    already cited) with exactly where it is cited (D782)."""
    with open(os.path.join(root, CASES)) as f:
        pool = {r for v in json.load(f)["families"].values() for r in v["citable_pool"]}
    with open(os.path.join(root, "eval/families/b2-v1-citations.json")) as f:
        v1 = json.load(f)["templates"]
    with open(os.path.join(root, "eval/families/b2-citations.json")) as f:
        rec = json.load(f)
    lab = {l["ruling"]: l for l in jsonl(os.path.join(root, "eval/data/labels.jsonl"))}
    want = {}
    later = later_pairs(root)
    for fam, old in v1.items():
        with open(os.path.join(root, "backend/internal/planner/templates", fam + ".json")) as f:
            t = json.load(f)
        for g in t["groups"]:
            p = g.get("precedent") or {}
            for k in ("confers_origin", "not_enough"):
                for r in p.get(k) or []:
                    if g["id"] + ":" + k + ":" + r not in old["pairs"] and (fam, g["id"] + ":" + k + ":" + r) not in later:
                        kind = "reused" if r in old["cited"] else "citations"
                        want.setdefault((kind, r), (fam, []))[1].append(g["id"] + ":" + k)
    have = {(kind, c["ruling"]): c for kind in ("citations", "reused") for c in rec.get(kind, [])}
    if set(want) != set(have):
        sys.exit("b2-citations.json records %s, the templates add %s" % (sorted(have), sorted(want)))
    for key, (fam, where) in want.items():
        r, c = key[1], have[key]
        if r not in pool or not lab.get(r, {}).get("evidence") or c["template"] != fam or sorted(c["cited_in"]) != sorted(where):
            sys.exit(f"{r}: not in a frozen pool, no held-origin sentence, or its record is stale")
    print(len(want), "new pairings checked")


def dump(o):
    return json.dumps(o, indent=1, ensure_ascii=False) + "\n"


if __name__ == "__main__":
    root, cmd = sys.argv[1], sys.argv[2]
    if cmd == "check":
        check_citations(root)
    jobs = {"hits": [(HITS, hits)], "freeze": [(CASES, freeze)], "check": [(HITS, hits), (CASES, freeze)]}[cmd]
    for path, fn in jobs:
        want = dump(fn(root))
        full = os.path.join(root, path)
        if cmd == "check":
            with open(full) as f:
                if f.read() != want:
                    sys.exit(f"{path} is stale; re-run")
            print(path, "current")
        else:
            with open(full, "w") as f:
                f.write(want)
            print("wrote", path)
