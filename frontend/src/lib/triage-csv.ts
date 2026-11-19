// Pure CSV helpers for portfolio triage, kept free of React/Next imports so
// they can be exercised with `node --test` without a browser or dev server.

export type Preview = { name: string; facts: string; date: string }[];

// Enough CSV to preview the file before sending it: quoted fields may hold
// commas, quotes ("") and line breaks. The backend parses it for real.
export function parseCsv(text: string): string[][] {
  const rows: string[][] = [];
  let row: string[] = [];
  let cell = "";
  let quoted = false;
  for (let i = 0; i < text.length; i++) {
    const ch = text[i];
    if (quoted) {
      if (ch === '"' && text[i + 1] === '"') {
        cell += '"';
        i++;
      } else if (ch === '"') quoted = false;
      else cell += ch;
    } else if (ch === '"') quoted = true;
    else if (ch === ",") {
      row.push(cell);
      cell = "";
    } else if (ch === "\n" || ch === "\r") {
      if (ch === "\r" && text[i + 1] === "\n") i++;
      row.push(cell);
      if (row.some((c) => c.trim())) rows.push(row);
      row = [];
      cell = "";
    } else cell += ch;
  }
  row.push(cell);
  if (row.some((c) => c.trim())) rows.push(row);
  return rows;
}

export function preview(text: string): Preview {
  const [head, ...body] = parseCsv(text);
  if (!head) return [];
  const h = head.map((c) => c.trim().toLowerCase());
  // Keep these in step with the authoritative parser in httpapi/batch.go so a
  // supported CSV isn't rejected by the preview (#310).
  const nameAliases = ["name", "product", "product name", "sku"];
  const factsAliases = ["facts", "description", "product description"];
  const dateAliases = ["date", "case date"];
  const ni = h.findIndex((c) => nameAliases.includes(c));
  const fi = h.findIndex((c) => factsAliases.includes(c));
  const di = h.findIndex((c) => dateAliases.includes(c));
  if (fi < 0) return [];
  return body.map((r) => ({
    name: (ni >= 0 ? r[ni] : "")?.trim() ?? "",
    facts: r[fi]?.trim() ?? "",
    date: (di >= 0 ? r[di] : "")?.trim() ?? "",
  }));
}

// Uploaded names and reasons are untrusted: a value starting with a formula
// prefix would execute when the downloaded CSV is opened in a spreadsheet.
// A leading apostrophe makes it literal text; numeric columns stay numeric.
// Control characters (including spaces) before the prefix are covered too.
const FORMULA_START = /^[\u0000-\u0020]*[=+\-@]/;

export function csvText(v: unknown): string {
  const raw = v == null ? "" : String(v);
  const s = FORMULA_START.test(raw) ? `'${raw}` : raw;
  return /[",\r\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s;
}

export function csvNumber(v: unknown): string {
  return typeof v === "number" && Number.isFinite(v) ? String(v) : csvText(v);
}
