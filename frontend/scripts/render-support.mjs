// Registers the loader and gives tests a markup renderer. Import this first.
import { register } from "node:module";
register("./tsx-loader.mjs", import.meta.url);
const { createElement } = await import("react");
const { renderToStaticMarkup } = await import("react-dom/server");
export const html = (component, props) => renderToStaticMarkup(createElement(component, props));
const ENTITIES = { "&amp;": "&", "&lt;": "<", "&gt;": ">", "&quot;": '"', "&#x27;": "'", "&#39;": "'" };
export const text = (markup) =>
  markup
    .replace(/<[^>]*>/g, " ")
    .replace(/&(amp|lt|gt|quot|#x27|#39);/g, (e) => ENTITIES[e])
    .replace(/\s+/g, " ")
    .trim();
