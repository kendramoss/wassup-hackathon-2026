"""Build the active U.S. antidumping and countervailing duty orders file.

Usage (standard library only; needs the network for "build"):
    python3 -I -B eval/adcvd/build_orders.py build --as-of 2026-11-27 --out backend/internal/adcvd/data/orders.json [--cache DIR]
    python3 -I -B eval/adcvd/build_orders.py check backend/internal/adcvd/data/orders.json

Sources (all public, Federal Register API, no key):
  1. Commerce (ITA) monthly "Opportunity To Request Administrative Review"
     notices: each lists every order whose anniversary falls in that month,
     with country, product and case number. The twelve notices up to the
     as-of date cover every order at least a year old.
  2. Commerce order notices ("... Antidumping Duty Order", "... Countervailing
     Duty Order") in the 13 months before the as-of date: orders too new to be
     in a monthly list yet. Their date is also the order date.
  3. Commerce revocation notices in the same window remove an order.
  4. The newest Commerce notice for each case that carries the scope text
     ("Scope of the Order") and names HTS numbers: continuation, order,
     amended order or review results. The HTS numbers are read from that text.

Only notices published on or before --as-of are used, so a data file built
for an earlier date contains nothing the later notices added.
"build" keeps the verbatim scope paragraphs naming HTS numbers; "check" reads
the HTS numbers again from those paragraphs and fails if they differ, so the
file can be verified without the network.
"""
import argparse
import concurrent.futures as cf
import datetime as dt
import hashlib
import html
import json
import os
import re
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

API = "https://www.federalregister.gov/api/v1/documents.json?"
UA = {"User-Agent": "Mozilla/5.0 (origin-proof adcvd build)"}
VERSION = "adcvd-orders-v1"
CASE = re.compile(r"\b([AC])-(\d{3})-(\d{3})\b")
# a table cell can run the product into the case number ("Yarn" "A-549-843")
CASE_CELL = re.compile(r"(?<![\dA-Z-])([AC])-(\d{3})-(\d{3})\b")
OPP_TITLE = "Opportunity To Request Administrative Review and Join Annual Inquiry Service List"

# ---------------------------------------------------------------- network


class Net:
    def __init__(self, cache):
        self.cache = cache
        os.makedirs(cache, exist_ok=True)

    def _get(self, url, retries=5):
        for i in range(retries):
            try:
                req = urllib.request.Request(url, headers=UA)
                with urllib.request.urlopen(req, timeout=90) as r:
                    return r.read()
            except urllib.error.HTTPError as e:
                if e.code == 404:
                    raise
                time.sleep(2 * (i + 1))
            except Exception:
                time.sleep(2 * (i + 1))
        raise RuntimeError("fetch failed: " + url)

    def cached(self, key, url):
        p = os.path.join(self.cache, key)
        if os.path.exists(p):
            return open(p, "rb").read()
        b = self._get(url)
        open(p, "wb").write(b)
        return b

    def search(self, key, **params):
        """All pages of a Federal Register search, newest first."""
        out, page = [], 1
        while True:
            p = dict(params)
            p.update({"per_page": 100, "page": page, "order": "newest",
                      "fields[]": ["title", "document_number", "publication_date", "type"]})
            url = API + urllib.parse.urlencode(p, doseq=True, safe="[]")
            d = json.loads(self.cached(f"{key}-p{page}.json", url))
            out += d.get("results", [])
            if page >= min(d.get("total_pages", 1), 5):
                return out
            page += 1

    def text(self, doc):
        # govinfo serves the same text as the Federal Register's own .txt
        # route, which takes about twenty seconds a notice
        url = f"https://www.govinfo.gov/content/pkg/FR-{doc['publication_date']}/html/{doc['document_number']}.htm"
        raw = self.cached(f"doc-{doc['document_number']}.txt", url)
        return clean(raw.decode("utf8", "replace")), hashlib.sha256(raw).hexdigest()


def clean(t):
    t = re.sub(r"<[^>]+>", "", t)
    return html.unescape(t)


# ------------------------------------------------------------ opp notices


def parse_opp(text):
    """Orders listed in one monthly notice: [(case, country, product)].

    The layout changes from month to month (country headings in capitals or
    not, entries wrapped over two or three lines, a case number split at its
    hyphen), so entries are rebuilt from lines: a line at the margin or four
    spaces in starts an entry, one space or five or more continues it.
    """
    lines = text.split("\n")
    start = next((i for i, l in enumerate(lines) if l.strip().startswith("Antidumping Duty Proceedings")), None)
    if start is None:
        return []
    entries, country, buf, skip = [], None, "", False
    for l in lines[start + 1:]:
        s = l.strip()
        if s.startswith("Suspension Agreements") or s.startswith("Suspended Investigations"):
            skip, buf = True, ""
            continue
        if not s or set(s) <= set("-") or s.startswith("[[Page") or s == "Period" or s.startswith("Federal Register"):
            continue
        if s.startswith("Antidumping Duty Proceedings") or s.startswith("Countervailing Duty Proceedings"):
            skip, buf = False, ""
            continue
        if skip:
            continue
        indent = len(l) - len(l.lstrip(" "))
        cont = bool(buf) and (indent == 1 or indent >= 5)
        l = re.sub(r"\s+\d{1,2}/\d{1,2}/\d{2}-\d{1,2}/\d{1,2}/\d{2}\s*$", "", l).replace("[uuml]", "\u00fc").replace("[Uuml]", "\u00dc")
        l = re.sub(r"\.{3,}\s*$", "", l)
        s = l.strip()
        head = re.match(r"^([A-Za-z][^:\n]{1,60}):\s*(.*)$", l) if indent == 0 else None
        if head and not CASE_CELL.search(head.group(1)):
            country = head.group(1).strip()
            s = head.group(2).strip()
            buf = ""
            if not s:
                continue
        elif not cont:
            buf = ""
        buf = (buf + " " + s).strip()
        joined = re.sub(r"(-)\s+(\d)", r"\1\2", buf)
        m = CASE_CELL.search(joined)
        if m:
            entries.append((m.group(1) + "-" + m.group(2) + "-" + m.group(3), country, joined[:m.start()].rstrip(", ").strip()))
            buf = ""
    return entries


# --------------------------------------------------------------- HTS parse

DOTTED = re.compile(r"(?<![\d.])(\d{4})\.(\d{2})(?:\.(\d{4})|\.(\d{2})(?:\.(\d{2}))?)?(?![\d])")
HEADING_LIST = re.compile(r"\b(?:sub)?headings?\s+((?:\d{4}(?:\.\d{2}(?:\.\d{2}(?:\.\d{2})?)?)?(?:\s*(?:,|and|or|to|through|\()\s*)*)+)", re.I)
EXCLUDE = re.compile(r"\b(exclu|not covered|outside the scope|not within|not included|not subject|do not fall|separately classified|retail sale)", re.I)
KEEP = re.compile(r"(currently classif|typically|may (also )?enter|are imported|is imported|entered under|enter under|classified|provided for|reference file|imported under)", re.I)


def fmt(d):
    if len(d) == 4:
        return d
    if len(d) == 6:
        return d[:4] + "." + d[4:]
    if len(d) == 8:
        return d[:4] + "." + d[4:6] + "." + d[6:]
    return d[:4] + "." + d[4:6] + "." + d[6:]


def numbers(sentence):
    """HTS numbers in a sentence as digit strings (4, 6, 8 or 10 digits)."""
    out = []
    for m in DOTTED.finditer(sentence):
        g = m.groups()
        d = g[0] + g[1] + (g[2] or ((g[3] or "") + (g[4] or "")))
        out.append(d)
    # bare four-digit headings only after "heading(s)" and never a year
    for hm in HEADING_LIST.finditer(sentence):
        span = hm.group(1)
        seq = re.findall(r"(?<![\d.])\d{4}(?![\d]|\.\d)|\bto\b|\bthrough\b", span)
        i = 0
        while i < len(seq):
            if re.fullmatch(r"\d{4}", seq[i]):
                a = seq[i]
                if i + 2 < len(seq) and seq[i + 1] in ("to", "through") and re.fullmatch(r"\d{4}", seq[i + 2]):
                    b = seq[i + 2]
                    if int(b) - int(a) <= 12 and int(b) >= int(a):
                        out += [str(x) for x in range(int(a), int(b) + 1)]
                        i += 3
                        continue
                out.append(a)
            i += 1
    out = [d for d in out if 1 <= int(d[:2]) <= 99]
    seen, res = set(), []
    for d in out:
        if d not in seen and len(d) in (4, 6, 8, 10):
            seen.add(d)
            res.append(d)
    return res


def sentences(par):
    par = re.sub(r"\s+", " ", par)
    # split on a period that ends a sentence, not on "8202.39" or "No."
    return re.split(r"(?<=[a-z\)\]\d])\.\s+(?=[A-Z\(])", par)


SEPARATE = re.compile(r"(?:that is |which is )?separately classified (?:under|in) [^,;]*?(?:HTSUS|HTS)[^,;]*,", re.I)


def parse_hts(paragraphs):
    """(included, excluded) digit strings from the paragraphs naming HTS numbers."""
    inc, exc = [], []
    for p in paragraphs:
        for s in sentences(p):
            s = SEPARATE.sub("", s)
            nums = numbers(s)
            if not nums:
                continue
            if EXCLUDE.search(s) and not KEEP.search(s):
                exc += nums
            else:
                inc += nums
    def uniq(x):
        seen, out = set(), []
        for d in x:
            if d not in seen:
                seen.add(d)
                out.append(d)
        return out
    inc = uniq(inc)
    exc = [d for d in uniq(exc) if d not in inc]
    return inc, exc


SCOPE_HEAD = re.compile(r"\n\s*(?:Appendix[-\u2013\u2014 ]*)?(?:[IVX]+\.\s*)?Scope of the (?:Antidumping Duty and Countervailing Duty |Antidumping Duty |Countervailing Duty )?Orders?(?:\(s\))?\s*\d*\s*\n", re.I)


def scope_paragraphs(text):
    """Paragraphs naming HTS numbers, from the first scope heading on."""
    m = SCOPE_HEAD.search(text)
    if not m:
        return []
    rest = re.sub(r"\n\[\[Page \d+\]\]\n", "\n", text[m.end():])
    out = []
    for p in re.split(r"\n\s*\n|\n {4}(?=\S)", rest):
        q = re.sub(r"\s+", " ", p).strip()
        q = re.sub(r"\s*-{20,}.*$", "", q).strip()
        if q.startswith("\\") or not q:
            continue
        if re.search(r"HTS|Harmonized Tariff", q) and numbers(q):
            out.append(q)
    return out


# ------------------------------------------------------------------- build

COUNTRY_TITLE = re.compile(r"\bFrom (?:the )?(.+?):")


def header_cases(text):
    head = text[:3000]
    return sorted({m.group(0) for m in CASE.finditer(head)})


def order_kind(title):
    """"order" for a notice that issues an AD or CVD order, else None."""
    if not re.search(r"(Antidumping|Countervailing) Duty Orders?\b", title, re.I):
        return None
    if re.search(r"\b(Continuation|Revocation|Initiation|Review|Reviews|Rescission|Extension|Postponement|Termination|Scope|Circumvention|Changed|Opportunity|Preliminary|Intent|Correction|Suspension)\b", title, re.I):
        return None
    return "order"


def in_effect_kind(title):
    """A sunset notice that leaves an order in effect: a continuation, or sunset results that do not revoke."""
    if re.search(r"Suspension|Suspended|Revocation|Revoke|Initiation|Preliminary|Advance Notification|Institution|Intent|in Part|Scope|Circumvention|Rescission", title, re.I):
        return None
    if re.search(r"Continuation of", title, re.I) and re.search(r"Orders?\b", title):
        return "continuation"
    if re.search(r"Final Results of (?:the )?(?:Expedited |Full )?(?:\w+ )?Sunset Reviews?", title, re.I):
        return "sunset"
    return None


def months_before_add(d, n):
    y, m = d.year, d.month + n
    while m > 12:
        y += 1
        m -= 12
    return dt.date(y, m, 1)


def months_before(d, n):
    y, m = d.year, d.month - n
    while m <= 0:
        y -= 1
        m += 12
    return dt.date(y, m, min(d.day, 28))


def build(as_of, out, cache):
    net = Net(cache)
    cut = dt.date.fromisoformat(as_of)
    iso = cut.isoformat()
    # 1. monthly notices
    opp = net.search("opp-" + iso, **{"conditions[term]": f'"{OPP_TITLE}"', "conditions[agencies][]": "international-trade-administration",
                               "conditions[type][]": "NOTICE", "conditions[publication_date][gte]": months_before(cut, 16).isoformat(),
                               "conditions[publication_date][lte]": iso})
    opp = [d for d in opp if "Opportunity To Request Administrative Review" in d["title"] and d["title"].startswith("Antidumping or Countervailing")]
    opp = sorted(opp, key=lambda d: d["publication_date"], reverse=True)
    # one notice per calendar month, the twelve newest months
    seen_m, picked = set(), []
    for d in opp:
        ym = d["publication_date"][:7]
        if ym not in seen_m:
            seen_m.add(ym)
            picked.append(d)
    picked = picked[:12]
    orders = {}
    for d in picked:
        text, sha = net.text(d)
        for case, country, prod in parse_opp(text):
            o = orders.setdefault(case, {"case": case, "type": "AD" if case[0] == "A" else "CVD", "country": country, "product": prod,
                                         "listed_in": []})
            o["listed_in"].append({"document_number": d["document_number"], "date": d["publication_date"]})
    n_listed = len(orders)
    # ITA numbers each country with the middle three digits of its case numbers
    code_country = {}
    for o in orders.values():
        if o["country"]:
            code_country.setdefault(o["case"][2:5], o["country"])
    # 2. new order notices
    # every Commerce notice of the window, in ten-day slices (the search caps a
    # query at 500 results), kept when its title issues an order
    newo, lo = [], months_before(cut, 13)
    while lo <= cut:
        hi = min(lo + dt.timedelta(days=9), cut)
        newo += net.search("allita-" + lo.isoformat(), **{"conditions[agencies][]": "international-trade-administration", "conditions[type][]": "NOTICE",
                                                          "conditions[publication_date][gte]": lo.isoformat(), "conditions[publication_date][lte]": hi.isoformat()})
        lo = hi + dt.timedelta(days=1)
    new_docs = list({d["document_number"]: d for d in newo if order_kind(d["title"]) or in_effect_kind(d["title"])}.values())
    for d in new_docs:
        text, sha = net.text(d)
        cs = header_cases(text)
        mc = COUNTRY_TITLE.search(d["title"])
        title_country = mc.group(1) if mc and not re.search(r",| and ", mc.group(1)) else None
        for case in cs:
            o = orders.setdefault(case, {"case": case, "type": "AD" if case[0] == "A" else "CVD", "country": code_country.get(case[2:5]) or title_country,
                                         "product": d["title"].split(" From ")[0], "listed_in": []})
            o["order_notice"] = {"document_number": d["document_number"], "date": d["publication_date"]}
    n_new = len(orders) - n_listed
    # 3. revocations
    rev = net.search("revoke-" + iso, **{"conditions[term]": "revocation",
                                  "conditions[agencies][]": "international-trade-administration", "conditions[type][]": "NOTICE",
                                  "conditions[publication_date][gte]": months_before(cut, 13).isoformat(),
                                  "conditions[publication_date][lte]": iso})
    revoked = {}
    for d in rev:
        t = d["title"]
        # a whole order revoked: not "in part", not the start of a review, not a proposal
        if re.search(r"\bRevocation\b|\bRevoked\b", t) and re.search(r"Antidumping|Countervailing|Sunset", t) and not re.search(r"in Part|Initiation|Intent|Preliminary|Consideration|Rescission|Continuation|Opportunity|Advance Notification", t, re.I):
            text, sha = net.text(d)
            for case in header_cases(text):
                if case in orders:
                    revoked[case] = {"document_number": d["document_number"], "date": d["publication_date"], "title": t}
    for case in revoked:
        orders.pop(case, None)
    # 4. per-case scope notice
    def work(case):
        docs = net.search("case-" + case, **{"conditions[term]": f'"{case}"', "conditions[agencies][]": "international-trade-administration",
                                              "conditions[type][]": "NOTICE"})
        docs = [d for d in docs if d["publication_date"] <= iso]
        # order date: earliest order notice naming the case in its header
        first = None
        tries = 0
        res = {"order_date": None, "scope": None, "tried": 0}
        order_docs = sorted([d for d in docs if order_kind(d["title"])], key=lambda d: d["publication_date"])
        for d in order_docs[:3]:
            text, sha = net.text(d)
            if case in header_cases(text):
                res["order_date"] = d["publication_date"]
                break
        if not orders[case]["country"]:
            # a new order with no country yet (its notice names several): the
            # country of an earlier notice about this case alone
            for d in docs:
                mc = COUNTRY_TITLE.search(d["title"])
                if not mc or re.search(r",| and ", mc.group(1)) or re.search(r"Opportunity|Initiation|Rescission|Scope", d["title"]):
                    continue
                text, sha = net.text(d)
                hc = header_cases(text)
                if hc and all(c[2:5] == case[2:5] for c in hc):
                    res["country"] = mc.group(1)
                    break
        cand = [d for d in docs if re.search(r"Continuation|Antidumping Duty Order|Countervailing Duty Order|Final Results|Amended Final|Final Determination|Preliminary Results|Preliminary Determination|Final Affirmative|Final Negative", d["title"])
                and not re.search(r"\b(Rescission|Recission|Initiation|Opportunity|Advance Notification|Notice of Scope|Extension|Postponement|Intent To Rescind|Termination|Revocation)\b", d["title"], re.I)]
        # newest continuation or order notice first (they carry the whole scope),
        # then review results, newest first
        rank = lambda d: 0 if re.search(r"Continuation|Duty Orders?\b", d["title"]) else 1
        cand = sorted(cand, key=lambda d: (rank(d), -int(d["publication_date"].replace("-", ""))))
        for d in cand[:14]:
            res["tried"] += 1
            text, sha = net.text(d)
            if case not in header_cases(text):
                continue
            pars = scope_paragraphs(text)
            inc, exc = parse_hts(pars)
            if inc:
                res["scope"] = {"document_number": d["document_number"], "date": d["publication_date"], "title": d["title"], "sha256": sha,
                                "url": f"https://www.federalregister.gov/d/{d['document_number']}", "paragraphs": pars, "hts": inc, "hts_excluded": exc}
                break
        return case, res
    with cf.ThreadPoolExecutor(10) as ex:
        for case, res in ex.map(work, sorted(orders)):
            o = orders[case]
            if res["order_date"] is None and o.get("order_notice"):
                res["order_date"] = o["order_notice"]["date"]
            if res.get("country") and not o["country"]:
                o["country"] = res["country"]
            o["order_date"] = res["order_date"]
            o["scope_notice"] = {k: res["scope"][k] for k in ("document_number", "date", "title", "sha256", "url")} if res["scope"] else None
            o["hts_text"] = res["scope"]["paragraphs"] if res["scope"] else []
            o["hts"] = [fmt(d) for d in res["scope"]["hts"]] if res["scope"] else []
            o["hts_excluded"] = [fmt(d) for d in res["scope"]["hts_excluded"]] if res["scope"] else []
            if res["scope"]:
                o["product"] = res["scope"]["title"].split(" From ")[0].strip() or o["product"]
            o["status"] = "active"
            o.pop("order_notice", None)
            o["listed_in"] = o["listed_in"][-1] if o["listed_in"] else None
    # companion orders (A- and C- on one product and country) share scope text
    ordered = [orders[c] for c in sorted(orders)]
    data = {
        "version": VERSION,
        "snapshot": iso,
        "fetched": dt.date.today().isoformat(),
        "sources": [
            {"name": "Commerce (ITA) monthly notices: Opportunity To Request Administrative Review and Join Annual Inquiry Service List",
             "url": "https://www.federalregister.gov/api/v1/documents.json", "documents": [{"document_number": d["document_number"], "date": d["publication_date"]} for d in picked]},
            {"name": "Commerce order notices, 13 months to the as-of date", "documents": [{"document_number": d["document_number"], "date": d["publication_date"]} for d in new_docs]},
            {"name": "Commerce revocation notices applied (a whole order revoked)", "documents": [dict(x, case=c) for c, x in sorted(revoked.items())]},
            {"name": "Scope text and HTS numbers: newest Commerce notice carrying the scope of each order (scope_notice)"},
        ],
        "counts": {"listed_in_monthly_notices": n_listed, "added_from_new_order_notices": n_new, "removed_as_revoked": len(revoked), "orders": len(ordered),
                   "with_hts": sum(1 for o in ordered if o["hts"]), "without_hts": sum(1 for o in ordered if not o["hts"])},
        "notice": "HTS numbers in a scope are for convenience and customs purposes; the written scope of the order governs.",
        "orders": ordered,
    }
    os.makedirs(os.path.dirname(out) or ".", exist_ok=True)
    with open(out, "w") as f:
        json.dump(data, f, indent=1, ensure_ascii=False)
        f.write("\n")
    print(json.dumps(data["counts"]))


def check(path):
    d = json.load(open(path))
    bad = 0
    for o in d["orders"]:
        inc, exc = parse_hts(o["hts_text"])
        if [fmt(x) for x in inc] != o["hts"] or [fmt(x) for x in exc] != o["hts_excluded"]:
            print("MISMATCH", o["case"], o["hts"], [fmt(x) for x in inc])
            bad += 1
        if o["hts"] and not o["scope_notice"]:
            print("NO SOURCE", o["case"])
            bad += 1
    print("orders", len(d["orders"]), "mismatches", bad)
    sys.exit(1 if bad else 0)


if __name__ == "__main__":
    ap = argparse.ArgumentParser()
    sub = ap.add_subparsers(dest="cmd", required=True)
    b = sub.add_parser("build")
    b.add_argument("--as-of", required=True)
    b.add_argument("--out", required=True)
    b.add_argument("--cache", default=os.path.expanduser("~/.cache/origin-proof-adcvd"))
    c = sub.add_parser("check")
    c.add_argument("path")
    a = ap.parse_args()
    if a.cmd == "build":
        build(a.as_of, a.out, a.cache)
    else:
        check(a.path)
