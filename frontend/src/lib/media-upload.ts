// Import from an image or scanned document: which path a picked file takes,
// the limits the API enforces (POST /v1/products/extract-media), and how far
// a large photo is scaled down before it is sent. Pure, so node:test can
// check it; the canvas work lives in the component.

// The API's limits (backend/internal/media). The server checks them again
// from the bytes; these only let the page say so before uploading.
export const MEDIA_MAX_FILES = 4;
export const MEDIA_MAX_BYTES = 8 << 20;

// A photo larger than this, in bytes or on its longest side, is re-encoded
// as a JPEG with its longest side at SCALE_TO before upload. A phone photo
// is ~12 MP and several MB; 3,072 px keeps a printed BOM legible.
export const SCALE_ABOVE_BYTES = 3 << 20;
export const SCALE_ABOVE_SIDE = 4096;
export const SCALE_TO = 3072;

export type Route = "bom" | "pdf" | "media";

const imageTypes = new Set(["image/png", "image/jpeg", "image/webp"]);
const imageExt = /\.(png|jpe?g|webp)$/i;

export function isImage(f: { name: string; type: string }): boolean {
  return imageTypes.has(f.type) || imageExt.test(f.name);
}

export function isPdf(f: { name: string; type: string }): boolean {
  return f.type === "application/pdf" || /\.pdf$/i.test(f.name);
}

// routeFor decides what to do with the picked files:
// - one CSV or Excel file: the BOM import, as before ("bom");
// - one PDF: try reading its table in the browser first, and fall back to
//   the media endpoint when it has none, i.e. a scan ("pdf");
// - images, or several images and PDFs: the media endpoint ("media").
// An error message is returned for a mix the import cannot take.
export function routeFor(files: { name: string; type: string; size: number }[]): { route: Route } | { error: string } {
  if (files.length === 0) return { error: "Pick a file." };
  if (files.some((x) => x.type.startsWith("image/") && !isImage(x))) return { error: "Images can be PNG, JPEG or WebP. Save or screenshot it as one of those." };
  if (files.length === 1 && !isImage(files[0]) && !isPdf(files[0])) return { route: "bom" };
  if (files.some((f) => !isImage(f) && !isPdf(f))) {
    return { error: "Import a CSV or Excel file on its own. Several files can only be images (PNG, JPEG, WebP) or PDFs." };
  }
  if (files.length > MEDIA_MAX_FILES) return { error: `Pick at most ${MEDIA_MAX_FILES} images or PDFs at once.` };
  if (files.length === 1 && isPdf(files[0])) return { route: "pdf" };
  return { route: "media" };
}

// tooLarge says whether files together pass the API's size limit.
export function tooLarge(files: { size: number }[]): string | null {
  const total = files.reduce((n, f) => n + f.size, 0);
  return total > MEDIA_MAX_BYTES ? `These files are ${(total / (1 << 20)).toFixed(1)} MB together; the limit is ${MEDIA_MAX_BYTES >> 20} MB. Send fewer pages or smaller images.` : null;
}

// scaledSize is the size to re-encode an image at, or null to send it as is.
export function scaledSize(width: number, height: number, bytes: number): { width: number; height: number } | null {
  const longest = Math.max(width, height);
  if (longest <= 0) return null;
  if (bytes <= SCALE_ABOVE_BYTES && longest <= SCALE_ABOVE_SIDE) return null;
  if (longest <= SCALE_TO) return null; // big in bytes only: re-encoding at the same size would not help much
  const k = SCALE_TO / longest;
  return { width: Math.max(1, Math.round(width * k)), height: Math.max(1, Math.round(height * k)) };
}

// mediaBanner is the note above the parts editor after a media draft.
export function mediaBanner(simulated: boolean | undefined): string {
  return simulated
    ? "Simulated draft from the local fake model. It did not read your file; the rows are a fixed example."
    : "Read from your image or scan by AI. Check every part, country and group against the document before you continue.";
}
