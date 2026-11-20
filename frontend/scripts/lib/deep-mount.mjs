// Mounts a React element tree without a DOM: every function component gets its
// own hook instance (keyed by its position), is rendered, and its output is
// expanded, so a test can reach a control nested several components deep and
// call its real props (onClick, onChange). fetch is whatever the test installs.
import { React, instance, tick } from "./hook-runtime.mjs";

export function deepMount(rootEl) {
  const store = new Map();
  const flat = [];
  let sched = false;
  const app = { renders: 0 };
  const schedule = () => {
    if (sched) return;
    sched = true;
    queueMicrotask(() => {
      sched = false;
      app.refresh();
    });
  };
  const visit = (node, path) => {
    if (node == null || typeof node !== "object") return;
    if (Array.isArray(node)) return node.forEach((x, i) => visit(x, `${path}.${i}`));
    if (!React.isValidElement(node)) return;
    flat.push(node);
    const { type, props } = node;
    if (typeof type === "function") {
      const key = `${path}:${type.name}`;
      let inst = store.get(key);
      if (!inst) {
        store.set(key, (inst = instance(type, schedule)));
        // Context: the default value (providers are not modelled).
        inst.dispatcher.useContext = (c) => c._currentValue;
        const d = inst.dispatcher;
        d.useLayoutEffect = d.useInsertionEffect = d.useEffect;
        d.useImperativeHandle = () => {};
        d.useDeferredValue = (v) => v;
        d.useTransition = () => [false, (f) => f()];
        d.useReducer = (red, init, fn) => {
          const [st, set] = d.useState(() => (fn ? fn(init) : init));
          return [st, (a) => set((x) => red(x, a))];
        };
      }
      inst.Component = type;
      inst.touched = true;
      inst.render(props);
      return visit(inst.tree, key);
    }
    const k = props?.children;
    visit(typeof k === "function" ? k(":field") : k, path);
  };
  app.refresh = () => {
    for (let n = 0; n < 40; n++) {
      flat.length = 0;
      for (const i of store.values()) i.touched = false;
      visit(rootEl, "root");
      app.renders++;
      if (![...store.values()].some((i) => i.dirty)) break;
    }
  };
  app.flush = async () => {
    for (let n = 0; n < 8; n++) {
      await tick();
      app.refresh();
    }
  };
  app.all = () => flat.slice();
  app.find = (pred) => flat.filter(pred);
  app.text = (n) => textOf(n.props?.children);
  app.btn = (label) => flat.find((e) => (e.type === "button" || e.type?.name === "Button") && textOf(e.props?.children).includes(label));
  app.unmount = () => {
    for (const i of store.values()) {
      i.mounted = false;
      for (const h of i.hooks) h?.cleanup?.();
    }
  };
  return app;
}

export const textOf = (n) => (n == null || typeof n === "boolean" ? "" : typeof n !== "object" ? String(n) : Array.isArray(n) ? n.map(textOf).join("") : textOf(n.props?.children));
