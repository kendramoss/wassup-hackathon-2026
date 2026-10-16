// Copies the flag SVGs (and the PDF.js worker) for every country in src/lib/countries.ts from
// flag-icons (MIT) into public/flags, so flags are bundled and never loaded
// from a CDN. Runs before dev and build.
import { copyFile, mkdir, readFile } from "node:fs/promises";

const src = await readFile(new URL("../src/lib/countries.ts", import.meta.url), "utf8");
const codes = new Set([...src.matchAll(/:\s*"([a-z]{2})",/g)].map((m) => m[1]));
await mkdir(new URL("../public/flags/", import.meta.url), { recursive: true });
for (const code of codes) {
  await copyFile(
    new URL(`../node_modules/flag-icons/flags/4x3/${code}.svg`, import.meta.url),
    new URL(`../public/flags/${code}.svg`, import.meta.url),
  );
}
// The PDF.js worker, for reading BOM tables out of PDFs (src/lib/pdf-bom.ts).
await copyFile(new URL("../node_modules/pdfjs-dist/build/pdf.worker.min.mjs", import.meta.url), new URL("../public/pdf.worker.min.mjs", import.meta.url));
console.log(`copied ${codes.size} flags and the PDF.js worker`);
