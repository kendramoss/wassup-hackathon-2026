// Register the TypeScript loader before the test's `-e` module imports run.
// Spawn with: node --import <this file> --no-warnings --input-type=module -e ...
import { register } from "node:module";

register("./ts-loader.mjs", import.meta.url);
