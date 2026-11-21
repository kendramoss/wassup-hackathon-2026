"""Convert fetched Customs Bulletin PDFs to text for `corpus bulletins extract` (D400).

Needs pypdf (pip install pypdf); it is not used by `make reproduce`, which
reads only the committed notice excerpts. Each page is written as a form feed,
"[[page N]]", a newline and the page text, which is the format
backend/internal/bulletin.Normalize reads.

    python3 -I eval/bulletins/pdftext.py data/corpus/bulletins
"""
import os
import sys

import pypdf


def main(root):
    src, dst = os.path.join(root, "pdf"), os.path.join(root, "text")
    os.makedirs(dst, exist_ok=True)
    for name in sorted(os.listdir(src)):
        if not name.endswith(".pdf"):
            continue
        reader = pypdf.PdfReader(os.path.join(src, name))
        parts = []
        for i, page in enumerate(reader.pages):
            parts.append("\f[[page %d]]\n" % (i + 1) + (page.extract_text() or ""))
        with open(os.path.join(dst, name[:-4] + ".txt"), "w", encoding="utf-8") as fh:
            fh.write("".join(parts))
        print(name, len(reader.pages), "pages")


if __name__ == "__main__":
    main(sys.argv[1] if len(sys.argv) > 1 else "data/corpus/bulletins")
