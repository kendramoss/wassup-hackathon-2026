// Minimal ESM loader for the TypeScript helpers the script tests import.
// Node 24 needed --experimental-transform-types for parameter properties;
// Node 26 removed that flag. Using the repo's own `typescript` dependency
// works on both and needs no experimental Node flag (#437).
import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import ts from "typescript";

export async function load(url, context, nextLoad) {
  if (!url.startsWith("file:") || !url.endsWith(".ts")) return nextLoad(url, context);
  const source = await readFile(fileURLToPath(url), "utf8");
  const { outputText } = ts.transpileModule(source, {
    fileName: url,
    compilerOptions: {
      module: ts.ModuleKind.ESNext,
      target: ts.ScriptTarget.ES2022,
      // Keep only runtime imports; the helpers use `import type` for schemas.
      verbatimModuleSyntax: false,
    },
  });
  return { format: "module", source: outputText, shortCircuit: true };
}
