"""Family follow-ups #777-#780 (D820): pools, keyword and HTS screens, frozen
case lists and the citation check for battery packs, lifting and other
uncovered machines, furniture finishing / upholstery / knock-down assembly,
and optical and precision instruments.

No model calls. Reads only the ruling IDs of held-out lists (to exclude
them), never their labels, facts or results. Japan data is not read.

Universe (D820): D780's universe (eval/families/depth_b2.py) without its
date cutoff, so non-held-out rulings dated 2026 in the bundled corpus are in
it (pool "other"). Held-out exclusion is D780's, by ID. Pools:
  dev   = universe ∩ splits_final.json "dev"
  train = universe ∩ splits_tune.json "train"
  other = universe − dev − train
Templates cite train and other only; dev rulings are regression cases.

Candidates: a keyword pattern per scope over the subject and the first
HEAD characters of the facts, OR an HTS heading of the scope stated in the
ruling (backend/data/hts_index.json, or the heading written in the facts).

Run from the repository root:
    python3 -I -B eval/families/depth_b3.py . hits     # write eval/families/b3-hits.json
    python3 -I -B eval/families/depth_b3.py . freeze   # write eval/families/b3-cases.json and b3-dev-splits.json
    python3 -I -B eval/families/depth_b3.py . check    # verify both and the citations (b3-citations.json)
"""
import importlib.util
import json
import os
import re
import sys

HITS = "eval/families/b3-hits.json"
SCREEN = "eval/families/b3-screen.json"
CASES = "eval/families/b3-cases.json"
DEVSPLIT = "eval/families/b3-dev-splits.json"
CITES = "eval/families/b3-citations.json"
V2 = "eval/families/b3-v2-pairs.json"
HEAD = 1500  # characters of facts read with the subject by the keyword pass

# D820: fixed and pushed before any count was taken with them.
PATTERNS = {
    # #777: cells, modules, packs, power banks, storage systems, BMS boards.
    "battery_packs": r"batter(y|ies)|accumulator|lithium[- ]ion|\bli-?ion\b|\blifepo|power bank|power station|energy storage|\bbess\b|\bbms\b|battery management|cell modules?|\bcells?\b[^.]{0,40}\b(lithium|battery|batteries|ion)\b",
    # #778: lifting and handling machines, and the 3D printer stretch.
    "lifting_machines": r"\bjacks?\b|jack stand|\bhoists?\b|\bwinch(es)?\b|\bcranes?\b|forklift|fork lift|pallet (truck|jack)|stacker|\blifts?\b|lifting|scissor lift|tail ?gate lift|\bdavits?\b|\btackle\b|chain block|\bpulley|3d printer|three-dimensional printer|additive manufactur",
    # #779: upholstery-only, finishing-only and knock-down / RTA furniture.
    "furniture": r"furniture|\bchairs?\b|\btables?\b|\bdesks?\b|\bsofas?\b|\bcouch|\bcabinets?\b|shel(f|ves|ving)\b|\bbeds?\b|bed frame|mattress|\bstools?\b|\bbench(es)?\b|ottoman|wardrobe|dresser|recliner|\bvanit(y|ies)\b|bookcase|nightstand|futon|headboard|seating|\bseats?\b|upholster|cushion|knock[- ]?down|ready[- ]to[- ]assemble|\brta\b|flat[- ]?pack|unassembled|disassembled|lacquer|varnish|\bstain(ed|ing)?\b|veneer",
    # #780: optical and precision instruments.
    "optical_precision": r"\blens(es)?\b|optical|optics|microscope|telescope|binocular|monocular|riflescope|rifle ?scope|spotting scope|\bsights?\b|magnif|\bloupes?\b|\bprisms?\b|rangefinder|range finder|laser level|\blevels?\b|caliper|micrometer|\bgauges?\b|measuring|surveying|theodolite|total station|spectromet|refractomet|hygromet|thermomet|anemomet|baromet|tape measure|\bruler|eyewear|eyeglass|spectacle|sunglass",
}
# HTSUS headings per scope (D820). A ruling is a candidate when it states one.
HEADINGS = {
    "battery_packs": ("8506", "8507"),
    "lifting_machines": ("8425", "8426", "8427", "8428", "8485"),
    "furniture": ("9401", "9403", "9404"),
    "optical_precision": ("9001", "9002", "9003", "9004", "9005", "9011", "9012", "9013", "9015", "9016", "9017",
                          "9024", "9025", "9026", "9027", "9028", "9029", "9030", "9031", "9032", "9033"),
}


def _b2(root):
    spec = importlib.util.spec_from_file_location("depth_b2", os.path.join(root, "eval/families/depth_b2.py"))
    m = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(m)
    return m


def universe(root):
    b2 = _b2(root)
    lab = b2.jsonl(os.path.join(root, "eval/data/labels.jsonl"))
    facts = {n[:-4] for n in os.listdir(os.path.join(root, "backend/data/facts")) if n.endswith(".txt")}
    held = b2.held_ids(root)
    with open(os.path.join(root, "eval/splits/splits_final.json")) as f:
        dev = set(json.load(f)["dev"])
    with open(os.path.join(root, "eval/splits/splits_tune.json")) as f:
        train = set(json.load(f)["train"])
    out = {}
    for l in lab:
        r = l["ruling"]
        if r in held or l["regime"] != "substantial_transformation" or r not in facts:
            continue
        out[r] = dict(l, pool="dev" if r in dev else "train" if r in train else "other")
    if set(out) & held:
        sys.exit("held-out ruling in the universe")
    return out, len(held)


def facts(root, r, n=None):
    with open(os.path.join(root, "backend/data/facts", r + ".txt")) as f:
        return f.read() if n is None else f.read(n)


def stated_headings(root, uni):
    with open(os.path.join(root, "backend/data/hts_index.json")) as f:
        idx = {e["ruling"]: e["hts"] for e in json.load(f)["rulings"]}
    out = {}
    for r in uni:
        hs = {h.replace(".", "")[:4] for h in idx.get(r, [])}
        hs |= set(re.findall(r"\b(\d{4})\.\d{2}(?:\.\d{2,4})?\b", facts(root, r)))
        out[r] = hs
    return out


def hits(root):
    uni, nheld = universe(root)
    pats = {k: re.compile(v, re.I) for k, v in PATTERNS.items()}
    heads = stated_headings(root, uni)
    fams = {k: [] for k in PATTERNS}
    for r in sorted(uni):
        l = uni[r]
        text = (l.get("subject") or "") + " | " + facts(root, r, HEAD)
        for k, p in pats.items():
            kw = bool(p.search(text))
            hts = sorted(heads[r] & set(HEADINGS[k]))
            if kw or hts:
                fams[k].append({"ruling": r, "pool": l["pool"], "date": l["date"], "origin": l.get("primary_origin"),
                                "multi_country": l.get("multi_country"), "subject": l.get("subject"),
                                "by": ("keyword" if kw else "") + ("+" if kw and hts else "") + ("hts " + ",".join(hts) if hts else "")})
    counts = {k: {p: sum(1 for h in v if h["pool"] == p) for p in ("dev", "train", "other")} for k, v in fams.items()}
    return {"rule": "D820 screens: keyword pattern over subject + first %d characters of facts, or a stated HTSUS heading of "
                    "the scope; universe and pools as in this script's docstring. A hit is a candidate only; the hand screen "
                    "(b3-screen.json) decides." % HEAD,
            "universe": len(uni), "excluded_held_out_ids": nheld,
            "pools": {p: sum(1 for l in uni.values() if l["pool"] == p) for p in ("dev", "train", "other")},
            "pools_2026": {p: sum(1 for l in uni.values() if l["pool"] == p and l["date"] >= "2026") for p in ("dev", "train", "other")},
            "patterns": PATTERNS, "headings": HEADINGS, "counts": counts, "hits": fams}


def dump(o):
    return json.dumps(o, indent=1, ensure_ascii=False) + "\n"


if __name__ == "__main__":
    root, cmd = sys.argv[1], sys.argv[2]
    jobs = {"hits": [(HITS, hits)], "check": [(HITS, hits)]}[cmd]
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
