// Test-only module hooks: lets `node --test` import the app's .ts/.tsx files
// and render them with react-dom/server. It resolves the "@/" alias, finds
// extensionless imports, swaps next/link and next/navigation for stubs, and transpiles
// with the TypeScript already in devDependencies. No bundler, no browser.
import { existsSync, statSync } from "node:fs";
import { readFile } from "node:fs/promises";
import { fileURLToPath, pathToFileURL } from "node:url";
import ts from "typescript";

const SRC = new URL("../src/", import.meta.url);
const LINK = new URL("./stubs/next-link.mjs", import.meta.url).href;
const NAV = new URL("./stubs/next-navigation.mjs", import.meta.url).href;
const isFile = (p) => existsSync(p) && statSync(p).isFile();

export async function resolve(spec, ctx, next) {
  if (spec === "next/link") return { url: LINK, shortCircuit: true };
  if (spec === "next/navigation") return { url: NAV, shortCircuit: true };
  let url = null;
  if (spec.startsWith("@/")) url = new URL(spec.slice(2), SRC);
  else if ((spec.startsWith("./") || spec.startsWith("../")) && ctx.parentURL?.startsWith("file:") && !ctx.parentURL.includes("/node_modules/")) url = new URL(spec, ctx.parentURL);
  if (url) {
    const base = fileURLToPath(url);
    for (const ext of ["", ".ts", ".tsx", "/index.ts", "/index.tsx"]) {
      if (isFile(base + ext)) return next(pathToFileURL(base + ext).href, ctx);
    }
  }
  return next(spec, ctx);
}

export async function load(url, ctx, next) {
  // A JSON file the app imports (Next bundles it; Node wants an attribute).
  if (url.startsWith("file:") && url.includes("/frontend/src/") && url.endsWith(".json")) {
    return { format: "module", source: `export default ${await readFile(fileURLToPath(url), "utf8")};`, shortCircuit: true };
  }
  if (url.startsWith("file:") && url.includes("/frontend/src/") && /\.tsx?$/.test(url)) {
    const source = await readFile(fileURLToPath(url), "utf8");
    const out = ts.transpileModule(source, {
      fileName: url,
      compilerOptions: { jsx: ts.JsxEmit.ReactJSX, module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
    });
    return { format: "module", source: out.outputText, shortCircuit: true };
  }
  return next(url, ctx);
}
