"""Shared helpers for the specialist challenge generators (D970).

Every generator is deterministic: the same seed and parameters write the same
bytes. Expected labels come from the reference oracle in each generator module
(a hand-written reading of the cited law), never from a model or from the code
under test. Model-visible fields are input.facts and input.date only; the case
id is an opaque hash.
"""
import hashlib, json, os, random, re

HERE = os.path.dirname(os.path.abspath(__file__))
DATE = "2026-11-28"  # after the whole retrieval corpus (rulings to 2026-09), so no ruling is filtered by date


def oid(feature, split, key):
    return "c" + hashlib.sha256(("%s|%s|%s" % (feature, split, key)).encode()).hexdigest()[:12]


def norm(text):
    return re.sub(r"[^a-z0-9]+", " ", text.lower()).strip()


def rng_for(*parts):
    return random.Random(int(hashlib.sha256("|".join(map(str, parts)).encode()).hexdigest()[:12], 16))


def pick(r, seq, avoid=()):
    pool = [x for x in seq if x not in avoid]
    return r.choice(pool)


def make_case(feature, split, key, family, scenario, template, tags, sources, rationale, facts, expected,
              exclude=(), parents=(), extra_input=None):
    inp = {"facts": facts, "date": DATE}
    if extra_input:
        inp.update(extra_input)
    return {
        "id": oid(feature, split, key), "feature": feature, "split": split,
        "source_family": family, "scenario_family": scenario, "template": template,
        "variation_tags": sorted(tags), "sources": sources, "parents": list(parents),
        "exclude_rulings": sorted(exclude), "rationale": rationale, "synthetic": True,
        "input": inp, "expected": expected,
    }


def write_jsonl(path, cases):
    seen = set()
    with open(path, "w", encoding="utf-8", newline="\n") as f:
        for c in cases:
            n = norm(c["input"]["facts"])
            assert n not in seen, "duplicate normalised input in %s" % path
            seen.add(n)
            f.write(json.dumps(c, sort_keys=True, ensure_ascii=False, separators=(",", ":")) + "\n")


def sha256_file(path):
    return hashlib.sha256(open(path, "rb").read()).hexdigest()


DATA = os.path.join(HERE, "..", "..", "..", "backend", "data")


def exemplars(ids, pattern, n=3):
    """Corpus rulings (from the ids a pool holds) whose facts match pattern, lowest ids first.

    These are the rulings a scenario family is modelled on; they are excluded
    from retrieval for the family's cases."""
    rx = re.compile(pattern)
    out = []
    for i in sorted(ids):
        p = os.path.join(DATA, "facts", i + ".txt")
        if os.path.exists(p) and rx.search(open(p, encoding="utf-8").read()):
            out.append(i)
            if len(out) == n:
                break
    return out


def pool_ids(name):
    if name == "textile":
        return json.load(open(os.path.join(DATA, "textile_rulings.json")))["rulings"]
    if name == "pharma":
        return json.load(open(os.path.join(DATA, "pharma_rulings.json")))["rulings"]
    if name == "usmca":
        return [json.loads(l)["ruling"] for l in open(os.path.join(DATA, "labels_usmca.jsonl"))]
    raise KeyError(name)
