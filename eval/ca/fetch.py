"""Polite snapshot of the CBSA's published origin and marking rulings.

Usage: python3 -I -B eval/ca/fetch.py <raw_dir>

The CBSA publishes, with each applicant's consent, the rulings it issues
("Rulings issued by the CBSA", CARM). This keeps the Origin Advance Rulings
and the Marking National Customs Rulings of the public list, as JSON from the
page's own read-only endpoints. Raw files are not committed. Etiquette:
robots.txt checked (26 Nov 2026: it disallows one onboarding PDF only), a
plain client name, requests at least 1.2 s apart, no sign-in.
"""
import json, os, sys, time, urllib.request

UA = "rulings-research-crawler/1.0"
API = "https://ccp-pcc.cbsa-asfc.cloud-nuage.canada.ca/carmwebservices/v2/carm/publicrulings"
KEEP = ("Origin Advance Ruling", "Marking National Customs Ruling")


def get(url):
    req = urllib.request.Request(url, headers={"User-Agent": UA})
    with urllib.request.urlopen(req, timeout=60) as r:
        return r.read()


def main(raw):
    os.makedirs(os.path.join(raw, "detail"), exist_ok=True)
    body = get(API + "/list?lang=en&curr=CAD")
    open(os.path.join(raw, "list.json"), "wb").write(body)
    for r in json.loads(body)["rulings"]:
        if r["type"] not in KEEP:
            continue
        p = os.path.join(raw, "detail", r["rulingsId"] + ".json")
        if os.path.exists(p):
            continue
        time.sleep(1.2)
        open(p, "wb").write(get(API + "/rulingsdetails/" + r["rulingsId"] + "?lang=en&curr=CAD"))
        print("detail", r["rulingsId"], file=sys.stderr)


if __name__ == "__main__":
    if len(sys.argv) != 2:
        sys.exit(__doc__)
    main(sys.argv[1])
