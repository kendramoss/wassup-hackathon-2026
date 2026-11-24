"""Dump every non-empty column 4 cell of the JP-Australia EPA Annex 2 table.

Reads the raw table cells with pdfplumber (no build_psr parsing) and writes
page, row key, column 4 as tab-separated lines. The Go test reads the result as
the PDF truth for the rows that must keep their column 4.

    python3 australia_column4_truth.py australia2.pdf ../../backend/internal/jprules/testdata/australia-column4.tsv
"""
import re
import sys

import pdfplumber

CODE = re.compile(r"^(\d{2})\.(\d{2})$|^(\d{4})\.(\d{2})$")


def clean(text):
    return " ".join((text or "").split())


def key(cell):
    cell = clean(cell)
    if m := re.fullmatch(r"Chapter (\d+)", cell):
        return f"{int(m[1]):02}"
    if m := CODE.match(cell):
        return "".join(g for g in m.groups() if g)
    raise ValueError(f"unexpected code cell {cell!r}")


def main(source, destination):
    lines = []
    with pdfplumber.open(source) as pdf:
        for number, page in enumerate(pdf.pages, 1):
            if number < 11:
                continue
            for table in page.extract_tables():
                for raw in table:
                    if len(raw) != 4 or not clean(raw[3]):
                        continue
                    if clean(raw[0]) == "1":  # the "1 2 3 4" column-number row
                        continue
                    lines.append(f"{number}\t{key(raw[0])}\t{clean(raw[3])}")
    with open(destination, "w", encoding="utf-8") as out:
        out.write("\n".join(lines) + "\n")
    print(len(lines))


if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2])
