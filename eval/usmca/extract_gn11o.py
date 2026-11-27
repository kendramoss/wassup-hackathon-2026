"""Extract the text of HTSUS General Note 11(o) (USMCA product-specific rules).

Usage: <python with pdfplumber 0.11.10> -I eval/usmca/extract_gn11o.py <gn.pdf> eval/usmca/gn11o.txt

Source: USITC HTSUS Revision 23 (2026), General Notes PDF
(https://hts.usitc.gov/reststop/file?release=currentRelease&filename=General%20Notes),
sha256 044efaf934ab97bad75c1d42a1fc08671dee74f2cdf145b57b65615970f82028.
GN 11(o) runs from the "Chapter 1" heading on GN p.52 to the end of chapter 97 on GN p.163.
The repeated page header lines are dropped; nothing else is changed.
"""
import hashlib
import sys

import pdfplumber

WANT = "044efaf934ab97bad75c1d42a1fc08671dee74f2cdf145b57b65615970f82028"
HEADER = ("Harmonized Tariff", "Annotated for Statistical", "GN p.")


def main(pdf_path, out_path):
    if hashlib.sha256(open(pdf_path, "rb").read()).hexdigest() != WANT:
        raise SystemExit("gn.pdf does not match the pinned sha256")
    pages = []
    with pdfplumber.open(pdf_path) as pdf:
        for i in range(51, 163):  # GN p.52 .. p.163
            lines = (pdf.pages[i].extract_text() or "").split("\n")
            while lines and (lines[0].startswith(HEADER) or lines[0] == "USMCA"):
                lines.pop(0)
            pages.append("\n".join(lines))
    open(out_path, "w").write("\n".join(pages))


if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2])
