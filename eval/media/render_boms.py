"""Render committed demo BOMs to PNG images for the image-import check.

    python3 eval/media/render_boms.py     # from the repo root; needs Pillow

Writes eval/media/images/<name>.png and eval/media/images/manifest.json.
Each image is a plain table (type, name, country, plant) drawn from a BOM CSV
already in the repo, so the expected parts and countries are known exactly.
"heatgun-photo" is the same table blurred, shrunk and turned 2 degrees, a
rough stand-in for a phone photo. All five are synthetic renderings of
illustrative BOMs, not customer documents.
"""
import csv
import json
import os

from PIL import Image, ImageDraw, ImageFilter, ImageFont

ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
OUT = os.path.join(ROOT, "eval", "media", "images")
SOURCES = [
    ("heatgun", "backend/testdata/heatgun_bom.csv"),
    ("lidar", "backend/testdata/lidar_bom.csv"),
    ("template", "frontend/public/samples/bom-template.csv"),
    ("heatgun-supplier", "frontend/public/samples/heatgun-bom.csv"),
]
COLS = ["type", "name", "country", "plant"]


def rows_of(path):
    with open(os.path.join(ROOT, path), newline="") as f:
        out = []
        for r in csv.DictReader(f):
            r = {k.strip().lower(): (v or "").strip() for k, v in r.items()}
            out.append({c: r.get(c, "") for c in COLS})
        return out


def render(title, rows):
    font = ImageFont.load_default(size=18)
    bold = ImageFont.load_default(size=22)
    widths = [110, 640, 140, 220]
    line = 30
    w, h = sum(widths) + 40, 90 + line * (len(rows) + 1) + 30
    img = Image.new("RGB", (w, h), "white")
    d = ImageDraw.Draw(img)
    d.text((20, 20), f"Bill of materials: {title} (illustrative)", fill="black", font=bold)
    y = 70
    x = 20
    for c, cw in zip(COLS, widths):
        d.text((x, y), c.capitalize(), fill="black", font=font)
        x += cw
    d.line((20, y + line - 4, w - 20, y + line - 4), fill="black", width=1)
    for r in rows:
        y += line
        x = 20
        for c, cw in zip(COLS, widths):
            text = r[c]
            while font.getlength(text) > cw - 10 and len(text) > 4:
                text = text[:-2]
            d.text((x, y), text if text == r[c] else text + "…", fill="black", font=font)
            x += cw
    return img


def main():
    os.makedirs(OUT, exist_ok=True)
    manifest = []
    for name, path in SOURCES:
        rows = rows_of(path)
        img = render(name, rows)
        img.save(os.path.join(OUT, f"{name}.png"), optimize=True)
        manifest.append({"image": f"{name}.png", "source": path, "components": [r for r in rows if r["type"] != "operation"]})
        if name == "heatgun":
            photo = img.rotate(2, expand=True, fillcolor="white").filter(ImageFilter.GaussianBlur(0.8))
            photo = photo.resize((photo.width * 2 // 3, photo.height * 2 // 3))
            photo.save(os.path.join(OUT, "heatgun-photo.png"), optimize=True)
            manifest.append({"image": "heatgun-photo.png", "source": path, "components": manifest[-1]["components"]})
    for m in manifest:
        m["components"] = [{"name": r["name"], "country": r["country"]} for r in m["components"]]
    with open(os.path.join(OUT, "manifest.json"), "w") as f:
        json.dump(manifest, f, indent=1)
        f.write("\n")


if __name__ == "__main__":
    main()
