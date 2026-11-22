"""Parse the raw Japan Customs advance-ruling snapshot into data/jp/.

Usage: python3 -I -B eval/jp/parse.py <raw_dir> <repo>

Reads <raw_dir>/detail/<id>.html and <raw_dir>/links.json (from crawl.py).
Writes:
  data/jp/rulings.jsonl     one parsed ruling per line, Japanese text as published
  data/jp/snapshot.json     retrieval dates, source URLs, per-page SHA-256 of the raw HTML
No model calls and no labels: eval/jp/labels.py derives the criterion and the
stated origin from the Japanese fields, development rulings only (D710).
"""
import hashlib, html, json, os, re, sys, time, unicodedata

FIELDS = {
    "登録番号": "id", "税関": "office", "交付年月日": "date", "一般的品名": "product_ja", "税番": "hs_raw",
    "回答": "answer_ja", "特恵種別": "agreement_ja", "貨物概要": "summary_ja", "認定理由": "reasoning_ja",
    "法令": "articles_ja", "その他": "other_ja",
}

# Published 特恵種別 values (all 18 seen on 22 Nov 2026) -> agreement code. Unknown values stop the parse.
AGREEMENTS = {
    "日EU協定": "JP-EU EPA", "TPP協定": "CPTPP", "日アセアン協定": "AJCEP", "RCEP協定": "RCEP",
    "日米貿易協定": "JP-US TA", "一般特恵": "GSP", "日インド協定": "JP-India CEPA", "日タイ協定": "JP-Thailand EPA",
    "日インドネシア協定": "JP-Indonesia EPA", "日オーストラリア協定": "JP-Australia EPA", "日英協定": "JP-UK CEPA",
    "日ベトナム協定": "JP-Vietnam EPA", "日マレーシア協定": "JP-Malaysia EPA", "非特恵（国定・WTO協定）": "Non-preferential",
    "日メキシコ協定": "JP-Mexico EPA", "日フィリピン協定": "JP-Philippines EPA", "日スイス協定": "JP-Switzerland EPA",
    "日チリ協定": "JP-Chile EPA",
}

COUNTRIES = {
    "ベトナム": "Vietnam", "中国": "China", "中華人民共和国": "China", "タイ": "Thailand", "インドネシア": "Indonesia",
    "マレーシア": "Malaysia", "フィリピン": "Philippines", "カンボジア": "Cambodia", "ミャンマー": "Myanmar", "ラオス": "Laos",
    "シンガポール": "Singapore", "ブルネイ": "Brunei", "インド": "India", "韓国": "South Korea", "大韓民国": "South Korea",
    "オーストラリア": "Australia", "豪州": "Australia", "ニュージーランド": "New Zealand", "カナダ": "Canada", "メキシコ": "Mexico",
    "米国": "United States", "アメリカ合衆国": "United States", "アメリカ": "United States", "チリ": "Chile", "ペルー": "Peru",
    "英国": "United Kingdom", "イギリス": "United Kingdom", "フランス": "France", "イタリア": "Italy", "ドイツ": "Germany",
    "スペイン": "Spain", "ポルトガル": "Portugal", "オランダ": "Netherlands", "ベルギー": "Belgium", "オーストリア": "Austria",
    "デンマーク": "Denmark", "スウェーデン": "Sweden", "フィンランド": "Finland", "アイルランド": "Ireland", "ポーランド": "Poland",
    "チェコ": "Czechia", "ハンガリー": "Hungary", "ルーマニア": "Romania", "ブルガリア": "Bulgaria", "ギリシャ": "Greece",
    "スロバキア": "Slovakia", "スロベニア": "Slovenia", "クロアチア": "Croatia", "エストニア": "Estonia", "ラトビア": "Latvia",
    "リトアニア": "Lithuania", "ルクセンブルク": "Luxembourg", "マルタ": "Malta", "キプロス": "Cyprus", "スイス": "Switzerland",
    "バングラデシュ": "Bangladesh", "スリランカ": "Sri Lanka", "パキスタン": "Pakistan", "ネパール": "Nepal", "モンゴル": "Mongolia",
    "トルコ": "Turkey", "ブラジル": "Brazil", "アルゼンチン": "Argentina", "南アフリカ": "South Africa", "エジプト": "Egypt",
    "ケニア": "Kenya", "エチオピア": "Ethiopia", "モロッコ": "Morocco", "チュニジア": "Tunisia", "ウクライナ": "Ukraine",
    "台湾": "Taiwan", "NZ": "New Zealand", "ＮＺ": "New Zealand", "香港": "Hong Kong", "日本": "Japan", "ノルウェー": "Norway", "コロンビア": "Colombia", "エクアドル": "Ecuador",
    "グアテマラ": "Guatemala", "ホンジュラス": "Honduras", "コスタリカ": "Costa Rica", "ウルグアイ": "Uruguay", "パラグアイ": "Paraguay",
    "ボリビア": "Bolivia", "ニカラグア": "Nicaragua", "エルサルバドル": "El Salvador", "ドミニカ": "Dominican Republic",
    "マダガスカル": "Madagascar", "モーリシャス": "Mauritius", "ガーナ": "Ghana", "コートジボワール": "Cote d'Ivoire",
    "タンザニア": "Tanzania", "ウガンダ": "Uganda", "ナイジェリア": "Nigeria", "セネガル": "Senegal", "ジョージア": "Georgia",
    "アルメニア": "Armenia", "ウズベキスタン": "Uzbekistan", "カザフスタン": "Kazakhstan", "キルギス": "Kyrgyzstan",
    "タジキスタン": "Tajikistan", "フィジー": "Fiji", "パプアニューギニア": "Papua New Guinea", "東ティモール": "Timor-Leste",
    "ブータン": "Bhutan", "モルディブ": "Maldives", "アフガニスタン": "Afghanistan", "イラン": "Iran", "ヨルダン": "Jordan",
    "レバノン": "Lebanon", "ペルー": "Peru", "イスラエル": "Israel", "サウジアラビア": "Saudi Arabia", "アラブ首長国連邦": "United Arab Emirates",
}
_CTRY = sorted(COUNTRIES, key=len, reverse=True)
_CTRY_RE = re.compile("|".join(map(re.escape, _CTRY)))

def text(s):
    s = re.sub(r"(?s)<br\s*/?>", "\n", s)
    s = re.sub(r"<[^>]+>", "", s)
    s = html.unescape(s).replace("\r", "")
    return "\n".join(l.strip() for l in s.split("\n") if l.strip())


def decode(b):
    for enc in ("utf-8", "cp932"):
        try:
            return b.decode(enc)
        except UnicodeDecodeError:
            pass
    return b.decode("cp932", "replace")


def agreement_code(a):
    return AGREEMENTS.get(a.strip())


def split_summary(s):
    n = s
    m_in = re.search(r"原材料[：:]", n)
    m_pr = re.search(r"(?:製造|生産|加工)工程[：:]", n)
    inputs = process = ""
    if m_in and m_pr and m_pr.start() > m_in.start():
        inputs, process = n[m_in.end():m_pr.start()], n[m_pr.end():]
    elif m_in:
        inputs = n[m_in.end():]
    elif m_pr:
        process = n[m_pr.end():]
    return inputs.strip(), process.strip()


_ITEM = re.compile(r"[①-⑳㉑-㉟㊱-㊿]")


def parse_inputs(inp):
    """Inputs as published: circled-number items with chapter/heading references and any country named."""
    if not inp:
        return []
    parts = [p for p in _ITEM.split(inp) if p.strip("、，, 。")] if _ITEM.search(inp) else [inp]
    out = []
    for p in parts:
        p = p.strip("、，, 。\n")
        n = unicodedata.normalize("NFKC", p)
        chapters = sorted({int(x) for x in re.findall(r"第\s*(\d{1,2})\s*類", n)})
        headings = sorted(set(re.findall(r"第?\s*(\d{2}\.\d{2})\s*項", n)))
        subheadings = sorted(set(re.findall(r"(\d{4}\.\d{2})\s*号", n)))
        name = re.split(r"[（(]", p, maxsplit=1)[0].strip()
        out.append({"text_ja": p, "name_ja": name, "chapters": chapters, "headings": headings,
                    "subheadings": subheadings, "countries": sorted({COUNTRIES[c] for c in _CTRY_RE.findall(p)})})
    return out


def parse(page):
    i = page.find("事前教示回答事例（原産地関係）詳細")
    if i < 0:
        return None
    body = page[i:page.find("</table>", i)]
    rec = {}
    for th, td in re.findall(r"(?s)<th[^>]*>(.*?)</th>\s*<td[^>]*>(.*?)</td>", body):
        k = FIELDS.get(text(th))
        if k:
            rec[k] = text(td)
    return rec


def main():
    raw, repo = sys.argv[1], sys.argv[2]
    links = json.load(open(os.path.join(raw, "links.json")))
    meta = json.load(open(os.path.join(raw, "crawl-meta.json")))
    out, snap = [], []
    mtimes = []
    unknown_ag, unparsed = set(), []
    for rid in sorted(links):
        p = os.path.join(raw, "detail", rid + ".html")
        b = open(p, "rb").read()
        mtimes.append(os.path.getmtime(p))
        rec = parse(decode(b))
        snap.append({"id": rid, "url": links[rid], "sha256": hashlib.sha256(b).hexdigest(), "bytes": len(b)})
        if not rec or rec.get("id") != rid:
            unparsed.append(rid)
            continue
        hs = unicodedata.normalize("NFKC", rec.get("hs_raw", "")).strip().rstrip("-").strip()
        digits = re.sub(r"\D", "", hs)
        ag = agreement_code(rec.get("agreement_ja", ""))
        if ag is None:
            unknown_ag.add(rec.get("agreement_ja", ""))
        inputs_ja, process_ja = split_summary(rec.get("summary_ja", ""))
        d = rec.get("date", "")
        out.append({
            "id": rid, "url": links[rid], "office_ja": rec.get("office", ""),
            "date": f"{d[:4]}-{d[4:6]}-{d[6:8]}" if re.fullmatch(r"\d{8}", d) else d,
            "product_ja": rec.get("product_ja", ""), "hs": hs, "hs_chapter": int(digits[:2]) if len(digits) >= 2 else None,
            "hs_heading": digits[:4] if len(digits) >= 4 else None,
            "agreement_ja": rec.get("agreement_ja", ""), "agreement": ag,
            "summary_ja": rec.get("summary_ja", ""), "inputs_ja": inputs_ja, "process_ja": process_ja,
            "inputs": parse_inputs(inputs_ja),
            "reasoning_ja": rec.get("reasoning_ja", ""), "articles_ja": rec.get("articles_ja", ""),
            "other_ja": rec.get("other_ja", ""), "answer_ja": rec.get("answer_ja", ""),
            "raw_sha256": snap[-1]["sha256"],
        })
    if unparsed:
        sys.exit("unparsed pages: %s" % unparsed[:20])
    if unknown_ag:
        sys.exit("unknown agreements: %s" % sorted(unknown_ag))
    os.makedirs(os.path.join(repo, "data/jp"), exist_ok=True)
    with open(os.path.join(repo, "data/jp/rulings.jsonl"), "w", encoding="utf-8") as f:
        for r in out:
            f.write(json.dumps(r, ensure_ascii=False, sort_keys=True) + "\n")
    json.dump({"source": "Japan Customs, 事前教示回答事例（原産地関係）",
               "search_url": "https://www.customs.go.jp/searchao/jaosv001.jsp",
               "index_url": "https://www.customs.go.jp/zeikan/seido/origin/kaitoujirei.htm",
               "licence": "PDL1.0 (https://www.digital.go.jp/resources/open_data/public_data_license_v1.0)",
               "list_retrieved_start_utc": meta["retrieved_start_utc"],
               "detail_pages_fetched_utc": [time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(min(mtimes))),
                                            time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(max(mtimes)))],
               "user_agent": meta["user_agent"], "search_result_count": meta["list_total"], "pages": snap},
              open(os.path.join(repo, "data/jp/snapshot.json"), "w"), indent=1, ensure_ascii=False)
    print("rulings", len(out))


if __name__ == "__main__":
    main()
