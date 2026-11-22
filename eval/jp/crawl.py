"""Polite snapshot of Japan Customs advance rulings on origin (事前教示回答事例・原産地関係).

Usage: python3 -I -B eval/jp/crawl.py <raw_dir>

Writes raw HTML to <raw_dir> (not committed: about 55 MB). parse.py turns it
into data/jp/. Etiquette: robots.txt is checked first (it allowed all paths on
22 Nov 2026); at most three connections, request starts at least 1.1 s apart; a plain client name;
back off on any error. Resumable: pages already on disk are not fetched again.

Source content is under the Public Data License 1.0 (PDL1.0); see
data/jp/LICENSE-SOURCE.md for attribution.
"""
import concurrent.futures as cf, html, http.cookiejar, threading, json, os, re, sys, time, urllib.parse, urllib.request, urllib.robotparser

UA = "rulings-research-crawler/1.0"
BASE = "https://www.customs.go.jp"
FORM = BASE + "/searchao/jaosv001.jsp"
RESULT = BASE + "/searchao/Result"
PDL = "https://www.digital.go.jp/resources/open_data/public_data_license_v1.0"
RULES = BASE + "/kyotsu/rules.htm"
DELAY = 1.1

opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
_last = [0.0]
_lock = threading.Lock()


def get(url, data=None, referer=None):
    for attempt in range(5):
        with _lock:  # request starts at least DELAY apart, across both workers
            wait = _last[0] + DELAY - time.time()
            if wait > 0:
                time.sleep(wait)
            _last[0] = time.time()
        h = {"User-Agent": UA}
        if referer:
            h["Referer"] = referer
        try:
            with opener.open(urllib.request.Request(url, data=data, headers=h), timeout=60) as r:
                return r.read()
        except Exception as e:  # back off and retry
            print("retry", attempt, url, e, file=sys.stderr)
            time.sleep(10 * (attempt + 1))
    raise SystemExit("giving up on " + url)


def form_fields(page):
    out = []
    for m in re.finditer(r"<(input|select)([^>]*)>", page):
        a = m.group(2)
        n = re.search(r'name="([^"]*)"', a)
        if not n:
            continue
        n = n.group(1)
        t = re.search(r'type="([^"]*)"', a)
        t = t.group(1) if t else ""
        if t in ("button", "image") or (t == "radio" and "checked" not in a):
            continue
        v = re.search(r'value=(?:"([^"]*)"|([^\s"/>]+))', a)
        v = html.unescape(v.group(1) if v.group(1) is not None else v.group(2)) if v else ""
        if m.group(1) == "select":
            rest = page[m.end():page.find("</select>", m.end())]
            o = re.search(r'<option value="([^"]*)" selected', rest) or re.search(r'<option value="([^"]*)"', rest)
            v = o.group(1)
        out.append((n, v))
    return out


def main():
    raw = sys.argv[1]
    os.makedirs(os.path.join(raw, "list"), exist_ok=True)
    os.makedirs(os.path.join(raw, "detail"), exist_ok=True)
    rp = urllib.robotparser.RobotFileParser()
    rp.parse(get(BASE + "/robots.txt").decode("utf-8", "replace").splitlines())
    for u in (FORM, RESULT, BASE + "/tetsuzuki_search/origin/J1/24/J124120.htm"):
        if not rp.can_fetch(UA, u):
            raise SystemExit("robots.txt disallows " + u)
    meta = {"retrieved_start_utc": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()), "user_agent": UA}
    for name, url in (("pdl1.0.html", PDL), ("customs-rules.html", RULES)):
        p = os.path.join(raw, name)
        if not os.path.exists(p):
            open(p, "wb").write(get(url))
    form = get(FORM).decode("utf-8")
    fields = form_fields(form)
    d = dict(fields)
    # Every tariff number 01-97, sorted by registration number ascending, 100 a page.
    d.update({"$Search_Radio_Target$": "20", "$Search_Text_Item_CstnoF$": "01", "$Search_Text_Item_CstnoT$": "97",
              "$Search_Internal_CstnoF$": "010000000", "$Search_Internal_CstnoT$": "979999999",
              "$Search_Text_Item_SortVal$": "20", "$Search_Combo_Refine_SortOpt$": "0",
              "$Search_Select_Item_Viewcnt$": "100"})
    links, total, start = {}, None, 1
    body = urllib.parse.urlencode([(k, d[k]) for k, _ in fields]).encode()
    while True:
        page = get(RESULT, body, FORM).decode("utf-8")
        m = re.search(r"検索結果：\s*(\d+)\s*件", page)
        r = re.search(r"件中の\s*(\d+)&#xFF5E;(\d+)", page)
        if not m or not r or int(r.group(1)) != start:
            open(os.path.join(raw, "error.html"), "w", encoding="utf-8").write(page)
            raise SystemExit("unexpected result page at start %d" % start)
        total = int(m.group(1))
        open(os.path.join(raw, "list", "start%05d.html" % start), "w", encoding="utf-8").write(page)
        rows = re.findall(r'<a href="(https://www\.customs\.go\.jp/tetsuzuki_search/origin/[^"]+\.htm)">(\d+)</a>', page)
        for url, rid in rows:
            links[rid] = url
        print("list", start, len(rows), "of", total, file=sys.stderr)
        # The page's own "start～end" label is wrong on the last page (22 Nov 2026 it read
        # "3601～1000"), so the end is counted from the rows.
        end = start + len(rows) - 1
        if not rows or end >= total:
            break
        # Next page: re-post the result form (jao001form2) as its "next" button does.
        f2 = page[page.index('name="jao001form2"'):]
        f2 = f2[:f2.index("</form>")]
        f2fields = form_fields(f2)
        d2 = dict(f2fields)
        start = end + 1
        d2["$Search_StartRange$"] = str(start)
        body = urllib.parse.urlencode([(k, d2[k]) for k, _ in f2fields] + [("AFTER.x", "10"), ("AFTER.y", "10")]).encode()
    meta["list_total"] = total
    meta["list_unique"] = len(links)
    json.dump(links, open(os.path.join(raw, "links.json"), "w"), indent=0, sort_keys=True)
    todo = [(rid, url) for rid, url in sorted(links.items()) if not os.path.exists(os.path.join(raw, "detail", rid + ".html"))]

    def fetch(item):
        rid, url = item
        b = get(url, referer=RESULT)
        tmp = os.path.join(raw, "detail", rid + ".tmp")
        open(tmp, "wb").write(b)
        os.replace(tmp, os.path.join(raw, "detail", rid + ".html"))

    # Three connections at most; request starts stay at least DELAY apart (under 1 a second).
    with cf.ThreadPoolExecutor(3) as ex:
        for i, _ in enumerate(ex.map(fetch, todo)):
            if i % 100 == 0:
                print("detail", i, len(todo), file=sys.stderr)
    meta["retrieved_end_utc"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    json.dump(meta, open(os.path.join(raw, "crawl-meta.json"), "w"), indent=1)


if __name__ == "__main__":
    main()
