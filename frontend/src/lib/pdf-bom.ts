// Reads a bill of materials table out of a PDF in the browser (PDF.js) and
// returns it as CSV for POST /v1/products/import, which takes CSV or Excel.
// The table is found by its header row (a "name" and a "country" column, with
// the importer's aliases); each text line is split into cells by the header's
// x positions, and a line whose first cell is empty continues the row above
// (wrapped text).

// Same names as the backend importer (backend/internal/bom/bom.go).
const aliases: Record<string, string[]> = {
  name: ["name", "component", "component name", "part", "part name", "item", "operation", "operation name", "description"],
  country: ["country", "made in", "made_in", "origin", "country of origin", "coo"],
  plant: ["plant", "factory", "site", "supplier", "plant name"],
  group: ["group", "operation group", "operation_group", "process group", "process"],
  type: ["type", "row type", "kind"],
  plant_capabilities: ["plant capabilities", "plant_capabilities", "capabilities", "can also make"],
};

function canonical(text: string): string | null {
  const t = text.trim().toLowerCase().replace(/\s+/g, " ");
  for (const [key, names] of Object.entries(aliases)) if (names.includes(t)) return key;
  return null;
}

type Item = { str: string; x: number; y: number };

export class PdfTableError extends Error {
  constructor(
    message: string,
    public text: string,
  ) {
    super(message);
  }
}

async function readItems(file: File): Promise<{ pages: Item[][]; text: string }> {
  const pdfjs = await import("pdfjs-dist");
  // Copied into public/ by scripts/copy-flags.mjs (no CDN at runtime).
  pdfjs.GlobalWorkerOptions.workerSrc = "/pdf.worker.min.mjs";
  const doc = await pdfjs.getDocument({ data: new Uint8Array(await file.arrayBuffer()) }).promise;
  const pages: Item[][] = [];
  const lines: string[] = [];
  for (let n = 1; n <= doc.numPages; n++) {
    const page = await doc.getPage(n);
    const content = await page.getTextContent();
    const items = content.items
      .filter((i): i is typeof i & { str: string; transform: number[] } => "str" in i && i.str.trim() !== "")
      .map((i) => ({ str: i.str, x: i.transform[4], y: Math.round(i.transform[5]) }));
    pages.push(items);
    lines.push(...groupLines(items).map((l) => l.map((i) => i.str).join(" ")));
  }
  return { pages, text: lines.join("\n") };
}

// Items on the same baseline (within 2 pt), top to bottom, left to right.
function groupLines(items: Item[]): Item[][] {
  const sorted = [...items].sort((a, b) => b.y - a.y || a.x - b.x);
  const lines: Item[][] = [];
  for (const it of sorted) {
    const line = lines.find((l) => Math.abs(l[0].y - it.y) <= 2);
    if (line) line.push(it);
    else lines.push([it]);
  }
  return lines.map((l) => l.sort((a, b) => a.x - b.x));
}

function csvCell(v: string) {
  return /[",\n]/.test(v) ? `"${v.replace(/"/g, '""')}"` : v;
}

export async function pdfToBomCsv(file: File): Promise<{ csv: string; rows: number }> {
  const { pages, text } = await readItems(file);
  let columns: { key: string; x: number }[] | null = null;
  const rows: string[][] = [];
  for (const items of pages) {
    for (const line of groupLines(items)) {
      const keys = line.map((i) => ({ key: canonical(i.str), x: i.x }));
      const header = keys.filter((k): k is { key: string; x: number } => k.key !== null);
      if (header.length >= 2 && header.some((h) => h.key === "name") && header.some((h) => h.key === "country")) {
        columns = header; // a repeated header on the next page resets nothing
        continue;
      }
      if (!columns) continue;
      // Each item belongs to the last column that starts at or left of it.
      const cells = columns.map(() => "");
      for (const it of line) {
        let c = 0;
        for (let k = 0; k < columns.length; k++) if (it.x >= columns[k].x - 4) c = k;
        cells[c] = cells[c] ? `${cells[c]} ${it.str.trim()}` : it.str.trim();
      }
      if (cells.every((c) => c === "")) continue;
      if (cells[0] === "" && rows.length) {
        const prev = rows[rows.length - 1];
        cells.forEach((v, k) => v && (prev[k] = prev[k] ? `${prev[k]} ${v}` : v));
      } else {
        rows.push(cells);
      }
    }
  }
  if (!columns || rows.length === 0) {
    throw new PdfTableError("This PDF has no table with name and country columns.", text);
  }
  const csv = [columns.map((c) => c.key).join(","), ...rows.map((r) => r.map(csvCell).join(","))].join("\n");
  return { csv, rows: rows.length };
}
