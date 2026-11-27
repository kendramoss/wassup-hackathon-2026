"""Encode HTSUS General Note 11(o) (USMCA product-specific rules) as rule rows.

Usage: python3 -I -B eval/usmca/build_gn11o.py <repo> [--check]

Reads eval/usmca/gn11o.txt (extract_gn11o.py) and writes
backend/internal/usmca/data/usmca_rules_ext.json in the rule-data schema of
backend/internal/usmca/rules.go. --check rebuilds in memory and fails if the
committed file differs.

Deterministic. Every rule keeps its verbatim text as the row excerpt. An
alternative is encoded only when its whole text matches one of the forms
below; anything else (a product description, "provided that" other than a
regional value content, a reference to a chapter or subheading rule, an
end-use condition) becomes an "unmodeled" condition, which the engine
evaluates as unknown. A numbered rule that opens with "For ..." (end use or
a vehicle type) is kept as an unsupported row so that a broader row never
answers for its goods.

Encoded forms (the tail "; or" / "." is dropped first):
  A change to <targets> from any other <level>[, including another <level> within that group][, except from <codes>]
  A change to <targets> from any <level> outside that group[, except from <codes>]
  A change to <targets> from <codes>[, whether or not there is also a change from any other <level>]
  A change to <targets> from <codes> or any other <level>
  No change in tariff classification [required | to a good of <targets>]
each optionally followed by ", provided there is a regional value content of
not less than: (1) N percent where the transaction value method is used; or
(2) M percent where the net cost method is used" (or one method only).

"from any other <level>" for a group of targets at that same level, without
"including another ... within that group", excludes the group (USMCA Annex
4-B, Section A, interpretive note on groups).
"""
import json
import re
import sys

LEVEL_DIGITS = {"chapter": 2, "heading": 4, "subheading": 6, "tariff item": 8}
KIND = {2: "change_from_chapter", 4: "change_from_heading", 6: "change_from_subheading", 8: "change_from_tariff_item"}
LEVEL_NAME = {2: "chapter", 4: "heading", 6: "subheading", 8: "tariff_item"}

CODE = r"\d{2}(?:\d{2})?(?:\.\d{2}){0,2}"
LEVELW = r"(?:chapters?|headings?|subheadings?|tariff items?)"


def digits(code):
    return code.replace(".", "")


def norm(s):
    s = s.replace("—", "-").replace("–", "-").replace("“", '"').replace("”", '"')
    s = re.sub(r"\s+", " ", s).strip()
    s = re.sub(r";or\b", "; or", s)
    # chapters are written without a leading zero ("chapter 4"); codes elsewhere never are
    s = re.sub(r"(chapters? (?:\d{1,2}(?:,| or| and| through|,? or| ) ?)+)", lambda m: re.sub(r"\b(\d)\b", r"0\1", m.group(1)), s)
    return s


class Unmodeled(Exception):
    pass


def parse_codes(text, default_level=None):
    """'headings 8501 through 8503, 8504 or subheading 8505.11' -> [(lo, hi)]."""
    text = text.strip().strip(",")
    if not text:
        raise Unmodeled("empty code list")
    out = []
    level = default_level
    # tokens: level words, codes, 'through', separators
    rest = text
    pat = re.compile(r"\s*(?:(?P<lvl>" + LEVELW + r")\s+)?(?P<lo>" + CODE + r")(?:\s+through\s+(?:(?:" + LEVELW + r")\s+)?(?P<hi>" + CODE + r"))?\s*(?:,\s*(?:or\s+|and\s+)?|\s+or\s+|\s+and\s+|$)")
    pos = 0
    while pos < len(rest):
        m = pat.match(rest, pos)
        if not m or m.end() == pos:
            raise Unmodeled("code list: " + rest[pos:])
        lvl = m.group("lvl")
        lo = digits(m.group("lo"))
        hi = digits(m.group("hi") or m.group("lo"))
        if lvl:
            level = lvl.rstrip("s") if not lvl.startswith("tariff") else "tariff item"
        if len(lo) != len(hi):
            raise Unmodeled("range of mixed length")
        if level and LEVEL_DIGITS[level] != len(lo):
            raise Unmodeled("code length does not match its level")
        out.append((lo, hi))
        pos = m.end()
    return out


def parse_thresholds(text):
    t = re.fullmatch(
        r"(?:\(\w+\)\s*)?(\d+) percent where the transaction value method is used;? (?:or|and)? ?(?:\(\w+\)\s*)?(\d+) percent where the net cost method is used",
        text)
    if t:
        return {"transaction_value": t.group(1), "net_cost": t.group(2)}
    t = re.fullmatch(r"(\d+) percent (?:under|where) the net cost method(?: is used)?", text)
    if t:
        return {"net_cost": t.group(1)}
    t = re.fullmatch(r"(\d+) percent (?:under|where) the transaction value method(?: is used)?", text)
    if t:
        return {"transaction_value": t.group(1)}
    raise Unmodeled("rvc wording: " + text)


def split_rvc(text):
    m = re.search(r",? provided there is a regional value content of not less than:? ?", text)
    if not m:
        return text, None
    return text[:m.start()], parse_thresholds(text[m.end():].strip())


def target_of(text):
    """The code(s) a rule alternative changes to; a product description before
    "of <level> <codes>" is allowed (the alternative itself then stays
    unmodeled)."""
    m = re.match(r"A change to (?:a good of |goods of |any other good of )?(" + LEVELW + r") (.+?),? from ", text)
    if m:
        return parse_codes(m.group(1) + " " + m.group(2))
    m = re.match(r"A change to .*? of (" + LEVELW + r") (" + CODE + r"(?:(?: through |, | or | and )(?:" + LEVELW + r" )?" + CODE + r")*)", text)
    if m:
        return parse_codes(m.group(1) + " " + m.group(2))
    m = re.match(r"No change in tariff classification (?:is required )?to (?:a good|goods) of (" + LEVELW + r") (.+?)(?:,|$)", text)
    if m:
        return parse_codes(m.group(1) + " " + m.group(2))
    return None


def exclude_list(text):
    """', except from X' -> list of 'lo' or 'lo-hi' strings."""
    return [lo if lo == hi else lo + "-" + hi for lo, hi in parse_codes(text)]


def parse_alternative(text, targets):
    """-> list of condition dicts. Raises Unmodeled."""
    shift, rvc = split_rvc(text)
    if re.search(r"provided that|\brule\b|\bNote\b|\bnote\b|for use", shift):
        raise Unmodeled("product or process condition")
    conds = []
    if shift.startswith("No change in tariff classification"):
        if not re.fullmatch(r"No change in tariff classification (?:is required|(?:is required )?to (?:a good|goods) of " + LEVELW + r" [\d\.,\s]+(?:through [\d\.]+|or [\d\.]+|and [\d\.]+)*)", shift):
            raise Unmodeled("no-change wording")
        if rvc is None:
            raise Unmodeled("no change without rvc")
        conds.append({"kind": "no_shift_required"})
    else:
        m = re.match(r"A change to (?:a good of |goods of )?" + LEVELW + r" .+? from (.+)$", shift)
        if not m:
            raise Unmodeled("not a change rule")
        src = m.group(1)
        tdig = len(targets[0][0])
        group = any(lo != hi for lo, hi in targets) or len(targets) > 1
        m1 = re.fullmatch(r"any other (chapter|heading|subheading|tariff item)(, including another (?:chapter|heading|subheading|tariff item) within that group)?(?:,? except from (.+))?", src)
        m2 = re.fullmatch(r"any (heading|subheading|tariff item) outside that group(?:,? except from (.+))?", src)
        m3 = re.fullmatch(r"(.+?), whether or not there is also a change from any other (chapter|heading|subheading|tariff item)", src)
        m4 = re.fullmatch(r"(.+?),? or any other (chapter|heading|subheading|tariff item)", src)
        if m1:
            n = LEVEL_DIGITS[m1.group(1)]
            excl = exclude_list(m1.group(3)) if m1.group(3) else []
            if group and n == tdig and not m1.group(2):
                excl += [lo if lo == hi else lo + "-" + hi for lo, hi in targets]
            c = {"kind": KIND[n]}
            if excl:
                c["exclude_ranges"] = excl
            conds.append(c)
        elif m2:
            n = LEVEL_DIGITS[m2.group(1)]
            excl = [lo if lo == hi else lo + "-" + hi for lo, hi in targets]
            if m2.group(2):
                excl += exclude_list(m2.group(2))
            conds.append({"kind": KIND[n], "exclude_ranges": excl})
        elif m3 or m4:
            mm = m3 or m4
            if "any other" in mm.group(1) or "within" in mm.group(1):
                raise Unmodeled("compound source")
            frm = exclude_list(mm.group(1))
            n = LEVEL_DIGITS[mm.group(2)]
            conds.append({"kind": "all_inputs_from", "others_from": frm, "others_outside_level": LEVEL_NAME[n]})
        else:
            # "from <codes>" alone: every non-originating material is under <codes>
            frm = exclude_list(src)
            conds.append({"kind": "all_inputs_from", "others_from": frm})
    if rvc:
        conds.append({"kind": "rvc", "thresholds": rvc})
    return conds


def split_rules(chapter_text):
    """Numbered rules of one chapter, skipping chapter/heading/subheading notes."""
    lines = chapter_text.split("\n")
    rules = []
    cur = None
    for ln in lines:
        m = re.match(r"^(\d+)[.,]?\s*(?=\(A\)|A change|No change|No required|For |A good|Any |The )", ln)
        note = re.match(r"^(Chapter|Heading|Subheading|Section|Tariff item) rule", ln)
        if m:
            cur = [m.group(1), ln[m.end():]]
            rules.append(cur)
        elif note:
            cur = None
        elif cur is not None:
            cur[1] += "\n" + ln
    return [(n, t) for n, t in rules]


PHASED = re.compile(r"Beginning on July 1, 20\d\d,? and thereafter, the following rules? of origin shall apply to [^:]+:\s*")
PHASED_END = re.compile(r"\n(?:Subheading rule|Heading rule|Chapter rule|\d+[.,]?\s*(?=\(A\)|A change|No change))")


def phased_rules(chapter_text):
    """The rules in force from the last stated date ("Beginning on July 1,
    2022, and thereafter, ..."), which GN 11(o) gives as subheading or heading
    rules rather than numbered rules. Their (a)/(b)/(c) alternatives are
    renumbered (A)/(B)/(C)."""
    out = []
    for i, m in enumerate(PHASED.finditer(chapter_text)):
        rest = chapter_text[m.end():]
        e = PHASED_END.search(rest)
        body = rest[:e.start()] if e else rest
        body = re.sub(r"(?:^|(?<=\s))\(([a-c])\)\s*(?=A change|No change)", lambda x: "(" + x.group(1).upper() + ") ", body)
        out.append((f"p{i + 1}", body))
    return out


def split_alternatives(text):
    parts = re.split(r"(?:^|\n|\s)\(([A-Z])\)\s*(?=A change|No change|A good|Any |For )", text)
    if len(parts) == 1:
        return [("A", text)]
    alts = []
    head = parts[0].strip()
    for i in range(1, len(parts), 2):
        alts.append((parts[i], parts[i + 1]))
    return alts if not head else [("_head", head)] + alts


def clean_alt(t):
    t = norm(t)
    t = re.sub(r"\s*;?\s*(?:or|and)?\s*$", "", t)
    t = re.sub(r"[.;]\s*(?:or)?$", "", t).strip()
    return t


def build(repo):
    full = open(f"{repo}/eval/usmca/gn11o.txt").read()
    cut = full.index("(p) Goods that shall be deemed to be originating goods")
    text, tail = full[:cut], full[cut:full.index("(q) Exceptions to change in tariff classification rules")]
    chapters = list(re.finditer(r"^Chapter (\d+)\s*$", text, re.M))
    rows, stats = [], {"rules": 0, "rows": 0, "unsupported_rows": 0, "alternatives": 0, "unmodeled_alternatives": 0, "skipped_rules": []}
    seen = {}
    for ci, cm in enumerate(chapters):
        ch = cm.group(1).zfill(2)
        body = text[cm.end(): chapters[ci + 1].start() if ci + 1 < len(chapters) else len(text)]
        for num, rtext in phased_rules(body) + split_rules(body):
            stats["rules"] += 1
            excerpt = norm(rtext if num.startswith("p") else num + ". " + rtext)
            raw_alts = split_alternatives(rtext)
            unsupported = None
            if raw_alts[0][0] == "_head":
                head = norm(raw_alts[0][1])
                raw_alts = raw_alts[1:]
                if head.startswith("For "):
                    unsupported = "the rule depends on the good's end use or type: " + head
                else:
                    stats["skipped_rules"].append(f"{ch}-{num}: unparsed head")
                    continue
            alts_text = [(aid, clean_alt(t)) for aid, t in raw_alts]
            targets = None
            for _, t in alts_text:
                try:
                    targets = target_of(t)
                except Unmodeled:
                    targets = None
                if targets:
                    break
            if not targets:
                stats["skipped_rules"].append(f"{ch}-{num}: no target")
                continue
            alternatives, alt_targets = [], []
            for aid, t in alts_text:
                try:
                    alt_targets.append(target_of(t))
                except Unmodeled:
                    alt_targets.append(None)
                stats["alternatives"] += 1
                try:
                    conds = parse_alternative(t, targets)
                except Unmodeled as e:
                    stats["unmodeled_alternatives"] += 1
                    conds = [{"kind": "unmodeled", "text": t}]
                alternatives.append({"id": aid, "label": t, "conditions": conds})
            for lo, hi in targets:
                key = (lo, hi)
                if key in seen:
                    # A later rule for the same code. Rules split by end use or
                    # vehicle type mark the code unsupported; otherwise keep the
                    # first and record the clash.
                    first = seen[key]
                    if unsupported or "unsupported" in first[1]:
                        first[1]["unsupported"] = first[1].get("unsupported") or unsupported
                        stats["skipped_rules"].append(f"{ch}-{num}: end-use variant of {lo}-{hi} (row {first[0]} unsupported)")
                    else:
                        stats["skipped_rules"].append(f"{ch}-{num}: duplicate target {lo}-{hi} (first in {first[0]})")
                    continue
                own = [a for a, t in zip(alternatives, alt_targets) if t is None or any(tl <= lo and hi <= th for tl, th in t)]
                if not own:
                    stats["skipped_rules"].append(f"{ch}-{num}: no alternative covers {lo}-{hi}")
                    continue
                row_unsupported = unsupported
                if "8701" <= lo[:4] <= "8708":
                    row_unsupported = row_unsupported or "motor vehicles and their parts of headings 8701 through 8708 are automotive goods under GN 11(k), which this check does not evaluate"
                row = {
                    "id": f"gn11o-{ch}-{num}-{lo}" + ("" if lo == hi else "-" + hi),
                    "code": lo,
                    "description": f"GN 11(o) chapter {int(ch)}, " + (f"rule {num}" if not num.startswith("p") else "rule in force from the last stated date"),
                    "source": {"citation": f"HTSUS Rev 23 GN 11(o), chapter {int(ch)}, " + (f"rule {num}" if not num.startswith("p") else "subheading or heading rule \"Beginning on July 1, ... and thereafter\""), "excerpt": excerpt},
                    "alternatives": own,
                }
                if hi != lo:
                    row["code_to"] = hi
                if row_unsupported:
                    row["unsupported"] = row_unsupported
                    stats["unsupported_rows"] += 1
                seen[key] = (f"{ch}-{num}", row)
                rows.append(row)
                stats["rows"] += 1
    deemed = []
    pending = ""
    for ln in tail.split("\n")[3:]:
        m = re.match(r"^(.*?)\.{3,}\s*([\d\.,\s]+)$", ln)
        if not m:
            pending += ln + " "
            continue
        name = norm(pending + m.group(1))
        pending = ""
        codes = [digits(c) for c in re.findall(r"\d{4}\.\d{2}(?:\.\d{2})?", m.group(2))]
        deemed.append({"name": name, "codes": codes})
    # wrapped names ("Parts of machines of subheading 8443.31 or 8443.32,\nexcluding ...") join the next line
    deemed_excerpt = norm(tail)
    data = {
        "schema_version": 1,
        "data_version": "gn11o-rev23-2026-11-27-1",
        "note": "Generated by eval/usmca/build_gn11o.py from HTSUS Rev 23 GN 11(o). Unmodeled alternatives evaluate as unknown; rows marked unsupported answer not_supported. Pilot rows (usmca_rules.json) take precedence for their codes.",
        "snapshot": {
            "source": "USITC HTSUS Revision 23 (2026), General Note 11(o), GN p.52-163, text extracted with pdfplumber 0.11.10 (eval/usmca/extract_gn11o.py)",
            "revision": "HTSUS Rev 23 (2026); PDF CreationDate 2026-11-10",
            "retrieved_on": "2026-11-24",
            "source_hash": "gn.pdf sha256 044efaf934ab97bad75c1d42a1fc08671dee74f2cdf145b57b65615970f82028; gn11o.txt sha256 bf044cae6b466ff7b7b457d57541cd8c4faa7137979bbff2cb5b985e2a62b451",
        },
        "rows": rows,
        "deemed_originating": {"citation": "HTSUS Rev 23 GN 11(p)", "excerpt": deemed_excerpt, "goods": deemed},
    }
    return data, stats


def main():
    repo = sys.argv[1]
    data, stats = build(repo)
    out = json.dumps(data, indent=1, ensure_ascii=False) + "\n"
    path = f"{repo}/backend/internal/usmca/data/usmca_rules_ext.json"
    if "--check" in sys.argv:
        if open(path).read() != out:
            raise SystemExit("usmca_rules_ext.json is stale; rerun build_gn11o.py")
        print("gn11o rules ok", stats["rows"], "rows")
        return
    open(path, "w").write(out)
    stats["skipped_rules"] = stats["skipped_rules"][:400]
    json.dump(stats, open(f"{repo}/eval/usmca/gn11o-build-stats.json", "w"), indent=1)
    print({k: (v if not isinstance(v, list) else len(v)) for k, v in stats.items()})


if __name__ == "__main__":
    main()
