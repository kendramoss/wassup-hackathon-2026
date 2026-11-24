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
    python3 -I -B eval/families/depth_b3.py . amend    # write b3-amend-1.json and b3-dev-splits-a1.json (D826)
    python3 -I -B eval/families/depth_b3.py . check    # verify all and the citations (b3-citations.json)
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


# The template each scope's development cases are expected to match (D820,
# D821): a new template where the bar is met, otherwise the existing one.
EXPECT = {"battery_packs": "power_supplies_chargers", "lifting_machines": "lifting_handling_machines",
          "furniture": "furniture", "optical_precision": "optical_precision_instruments"}
# Scopes a dedicated template already serves: the bar does not apply.
SERVED = {"furniture": "furniture"}
BAR = 10
MIN_DEV = 5


def template_citations(root):
    out = set()
    d = os.path.join(root, "backend/internal/planner/templates")
    for n in sorted(os.listdir(d)):
        with open(os.path.join(d, n)) as f:
            t = json.load(f)
        for g in t["groups"]:
            p = g.get("precedent") or {}
            out |= set(p.get("confers_origin") or []) | set(p.get("not_enough") or [])
    return out


def freeze(root):
    """D820's frozen lists. Development cases: (a) dev-split rulings kept;
    (b) kept pool rulings with no held-origin sentence; (c) where (a) and (b)
    give fewer than MIN_DEV, kept pool rulings no template cites, latest date
    first, until MIN_DEV, never in a scope whose new template's bar depends
    on them."""
    uni, _ = universe(root)
    lab = {l["ruling"]: l for l in _b2(root).jsonl(os.path.join(root, "eval/data/labels.jsonl"))}
    with open(os.path.join(root, SCREEN)) as f:
        scr = json.load(f)
    with open(os.path.join(root, CITES)) as f:
        frozen_cited = set(json.load(f)["cited_before_freeze"])
    out = {"rule": scr["rule"], "frozen_at": scr["frozen_at"], "bar": BAR, "families": {}}
    for fam, rows in scr["families"].items():
        for row in rows:
            if row["ruling"] not in uni:
                sys.exit(f"{fam}: {row['ruling']} is outside the universe")
        keep = [r for r in rows if r["keep"]]
        pool = sorted(r["ruling"] for r in keep if uni[r["ruling"]]["pool"] != "dev")
        dev = sorted(r["ruling"] for r in keep if uni[r["ruling"]]["pool"] == "dev")
        uncitable = sorted(r for r in pool if not lab[r].get("evidence"))
        hold = []
        if len(dev) + len(uncitable) < MIN_DEV and fam in SERVED:
            spare = [r for r in pool if r not in uncitable and r not in frozen_cited]
            spare.sort(key=lambda r: (lab[r]["date"], r), reverse=True)
            hold = sorted(spare[:MIN_DEV - len(dev) - len(uncitable)])
        out["families"][fam] = {
            "expected_template": EXPECT[fam],
            "kept_pool": len(pool),
            "bar": ("served by the %s template" % SERVED[fam]) if fam in SERVED else
                   ("met: %d of %d" % (len(pool), BAR) if len(pool) >= BAR else "not met: %d of %d, %d short" % (len(pool), BAR, BAR - len(pool))),
            "regression_dev": dev,
            "dev_uncitable_pool": uncitable,
            "dev_held_back": hold,
            "citable_pool": [r for r in pool if r not in uncitable and r not in hold],
            "screened_out": sorted(r["ruling"] for r in rows if not r["keep"]),
            "ambiguous": sorted(r["ruling"] for r in rows if not r["keep"] and r.get("ambiguous")),
            "tags": {r["ruling"]: r["tags"] for r in rows if r.get("tags")},
        }
    return out


def devsplit(root):
    """The development set for #781 in the splits format familycov reads
    (-splits ../eval/families/b3-dev-splits.json -split dev): D781's frozen
    development cases and D820's, with the template each is expected to
    match, and the development rulings screened as look-alikes. No held-out
    ID is listed here; the held-out comparison is splits_final.json "final"."""
    b3 = freeze(root)
    with open(os.path.join(root, "eval/families/b2-cases.json")) as f:
        b2 = json.load(f)["families"]
    with open(os.path.join(root, "eval/splits/splits_final.json")) as f:
        devsplit_ids = set(json.load(f)["dev"])
    b2exp = {"batteries": "power_supplies_chargers", "machinery": "motors_pumps_compressors",
             "furniture": "furniture", "medical_optical": "medical_devices"}
    expected, source = {}, {}
    for fam, v in b2.items():
        for r in v["regression_dev"]:
            expected[r] = b2exp[fam]
            source[r] = "D781 " + fam
    for fam, v in b3["families"].items():
        for r in v["regression_dev"] + v["dev_uncitable_pool"] + v["dev_held_back"]:
            if r in expected and expected[r] != v["expected_template"]:
                source[r] += "; D820 " + fam + " (expected template changed)"
            elif r not in expected:
                source[r] = "D820 " + fam
            expected[r] = v["expected_template"]
    look = {}
    for fam, v in list(b2.items()) + list(b3["families"].items()):
        for r in v["ambiguous"]:
            if r in devsplit_ids and r not in expected:
                look.setdefault(r, set()).add(fam)
    return {"rule": "D820: development cases for family matching, frozen 2026-11-24 before any template was written from the "
                    "D820 screen. dev = cases expected to match a template (expected); lookalikes = development rulings "
                    "screened as sharing a cue with a scope, right when not matched to it. No held-out ID is listed.",
            "frozen_at": b3["frozen_at"],
            "dev": sorted(expected), "expected": dict(sorted(expected.items())), "source": dict(sorted(source.items())),
            "lookalikes": {r: sorted(f) for r, f in sorted(look.items())}}


# D826: the template whose new group/outcome pairings each scope's frozen
# citable pool may supply (lifting machines deepen the off-road and lifting
# machinery template; optical instruments have their own template).
NEW = {"offroad_vehicles_machinery": "lifting_machines", "optical_precision_instruments": "optical_precision"}
BEFORE = "eval/families/b3-before-citations.json"
AMEND = "eval/families/b3-amend-1.json"
DEVSPLIT_A1 = "eval/families/b3-dev-splits-a1.json"
# D826, from the review of #782: rulings a template dedicated to the scope's
# goods already cites do not count toward the bar (D820). The off-road and
# mobile machinery template (D660) was written from the scissor lifts and
# the guided vehicles. The display and PCBA templates are broad, as D820's
# own example of the display template's lens rulings says, so their
# citations count.
DEDICATED = {"lifting_machines": ["offroad_vehicles_machinery"], "furniture": ["furniture"]}
EXPECT_A1 = {"lifting_machines": "offroad_vehicles_machinery"}


def amend(root):
    """D826, amending D821 (dated 2026-11-24, after the review of #782):
    the bar recounted strictly, and the expected templates that change.
    D821's files are left as frozen."""
    b3 = freeze(root)["families"]
    with open(os.path.join(root, BEFORE)) as f:
        before = json.load(f)["templates"]
    lab = {l["ruling"]: l for l in _b2(root).jsonl(os.path.join(root, "eval/data/labels.jsonl"))}
    fams = {}
    for fam, v in b3.items():
        pool = sorted(v["citable_pool"] + v["dev_uncitable_pool"] + v["dev_held_back"])
        served = {r: [t for t in DEDICATED.get(fam, []) if r in before[t]["cited"]] for r in pool}
        served = {r: t for r, t in served.items() if t}
        counted = [r for r in pool if r not in served]
        citable = [r for r in counted if lab[r].get("evidence")]
        if fam in SERVED:
            bar = "served by the %s template" % SERVED[fam]
        elif len(counted) >= BAR:
            bar = "met: %d kept of %d, %d citable" % (len(counted), BAR, len(citable))
        else:
            bar = "not met: %d of %d, %d short" % (len(counted), BAR, BAR - len(counted))
        fams[fam] = {"expected_template": EXPECT_A1.get(fam, v["expected_template"]),
                     "kept_pool": len(pool), "served_by_dedicated_template": served,
                     "counted": len(counted), "citable_counted": len(citable), "bar": bar}
    return {"rule": "D826 amends D821: the D820 bar applied strictly (rulings a template dedicated to the scope already "
                    "cites are not counted); expected templates follow the templates actually written.",
            "amended_at": "2026-11-24", "families": fams}


def devsplit_a1(root):
    """D821's development set with D826's expected templates. Same cases."""
    d = devsplit(root)
    a = amend(root)["families"]
    b3 = freeze(root)["families"]
    old = d["expected"]
    for fam, v in b3.items():
        for r in v["regression_dev"] + v["dev_uncitable_pool"] + v["dev_held_back"]:
            if old[r] != a[fam]["expected_template"]:
                d["source"][r] += "; D826: expected %s (was %s)" % (a[fam]["expected_template"], old[r])
                d["expected"][r] = a[fam]["expected_template"]
    d["rule"] = "D826 amendment of b3-dev-splits.json (D821, left as frozen): same cases and look-alikes; expected templates " \
                "follow the templates written (lifting cases expect offroad_vehicles_machinery). " + d["rule"]
    d["amended_at"] = "2026-11-24"
    return d


def check_citations(root):
    """Every group/outcome pairing a template has that main's did not
    (b3-before-citations.json) is in a template of NEW, is a ruling of that
    scope's frozen citable pool with its held-origin sentence in the bundled
    labels, and is recorded in b3-citations.json with exactly where it is
    cited; no template cites a frozen development case (D820, D826)."""
    cases = freeze(root)["families"]
    lab = {l["ruling"]: l for l in _b2(root).jsonl(os.path.join(root, "backend/data/labels.jsonl"))}
    with open(os.path.join(root, BEFORE)) as f:
        before = json.load(f)["templates"]
    with open(os.path.join(root, CITES)) as f:
        rec = {(c["template"], c["ruling"]): c for c in json.load(f)["citations"]}
    want = {}
    d = os.path.join(root, "backend/internal/planner/templates")
    for n in sorted(os.listdir(d)):
        with open(os.path.join(d, n)) as f:
            t = json.load(f)
        old = set(before.get(t["family"], {}).get("pairs", []))
        for g in t["groups"]:
            p = g.get("precedent") or {}
            for k in ("confers_origin", "not_enough"):
                for r in p.get(k) or []:
                    if g["id"] + ":" + k + ":" + r not in old:
                        if t["family"] not in NEW:
                            sys.exit(f"{t['family']}: new pairing {g['id']}:{k}:{r} in a template D826 does not change")
                        want.setdefault((t["family"], r), []).append(g["id"] + ":" + k)
    if set(want) != set(rec):
        sys.exit("b3-citations.json records %s, the templates add %s" % (sorted(rec), sorted(want)))
    for (fam, r), where in want.items():
        c = rec[(fam, r)]
        if r not in cases[NEW[fam]]["citable_pool"] or not lab.get(r, {}).get("evidence") or sorted(c["cited_in"]) != sorted(where) \
                or c["held_sentence"] != lab[r]["evidence"]:
            sys.exit(f"{fam} {r}: not in the frozen citable pool, no held-origin sentence, or its record is stale")
    dev = {r for v in cases.values() for r in v["regression_dev"] + v["dev_uncitable_pool"] + v["dev_held_back"]}
    cited = template_citations(root)
    if dev & cited:
        sys.exit("a template cites a frozen development case: %s" % sorted(dev & cited))
    print(len(want), "new pairings checked")


def dump(o):
    return json.dumps(o, indent=1, ensure_ascii=False) + "\n"


if __name__ == "__main__":
    root, cmd = sys.argv[1], sys.argv[2]
    if cmd == "check":
        check_citations(root)
    jobs = {"hits": [(HITS, hits)], "freeze": [(CASES, freeze), (DEVSPLIT, devsplit)],
            "amend": [(AMEND, amend), (DEVSPLIT_A1, devsplit_a1)],
            "check": [(HITS, hits), (CASES, freeze), (DEVSPLIT, devsplit), (AMEND, amend), (DEVSPLIT_A1, devsplit_a1)]}[cmd]
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
