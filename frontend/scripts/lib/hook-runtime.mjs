// A tiny hook runtime for tests that mount the real case-shell + screen
// components without a DOM. State setters schedule a re-render, effects run
// after each render, and fetch is whatever the test installs. It is the same
// technique as import-race.test.mjs, shared here for the shell-level tests.
import { register } from "node:module";

const React = await import("react");
export { React };
const internals = React.__CLIENT_INTERNALS_DO_NOT_USE_OR_WARN_USERS_THEY_CANNOT_UPGRADE;

export const tick = () => new Promise((r) => setImmediate(r));
const same = (a, b) => a && b && a.length === b.length && a.every((x, i) => Object.is(x, b[i]));

export function instance(Component, onDirty) {
  const inst = { hooks: [], i: 0, effects: [], mounted: true, tree: null, dirty: false, Component };
  const slot = () => (inst.hooks[inst.i] ??= {});
  inst.dispatcher = {
    useState(init) {
      const h = slot();
      if (!("v" in h)) h.v = typeof init === "function" ? init() : init;
      inst.i++;
      h.set ??= (n) => {
        if (!inst.mounted) return;
        const nv = typeof n === "function" ? n(h.v) : n;
        if (Object.is(nv, h.v)) return;
        h.v = nv;
        inst.dirty = true;
        onDirty();
      };
      return [h.v, h.set];
    },
    useRef(init) {
      const h = slot();
      inst.i++;
      return (h.ref ??= { current: init });
    },
    useId() {
      return `:id${inst.i++}`;
    },
    useCallback(fn, deps) {
      const h = slot();
      inst.i++;
      if (!same(h.deps, deps)) {
        h.deps = deps;
        h.fn = fn;
      }
      return h.fn;
    },
    useMemo(fn, deps) {
      const h = slot();
      inst.i++;
      if (!same(h.deps, deps)) {
        h.deps = deps;
        h.v = fn();
      }
      return h.v;
    },
    useEffect(fn, deps) {
      const h = slot();
      inst.i++;
      const changed = !h.ran || !deps || !same(h.deps, deps);
      if (changed) inst.effects.push({ h, fn, deps });
    },
    useContext() {
      return undefined;
    },
  };
  inst.render = (props) => {
    inst.i = 0;
    inst.dirty = false;
    const prev = internals.H;
    internals.H = inst.dispatcher;
    try {
      inst.tree = inst.Component(props);
    } finally {
      internals.H = prev;
    }
    for (const e of inst.effects.splice(0)) {
      e.h.cleanup?.();
      e.h.ran = true;
      e.h.deps = e.deps;
      e.h.cleanup = e.fn() ?? undefined;
    }
  };
  return inst;
}

export const walk = (n, visit) => {
  if (n == null || typeof n !== "object") return;
  if (Array.isArray(n)) return n.forEach((x) => walk(x, visit));
  if (!React.isValidElement(n)) return;
  visit(n);
  const k = n.props.children;
  walk(typeof k === "function" ? k(":field") : k, visit);
};
export const findAll = (tree, pred) => {
  const out = [];
  walk(tree, (e) => pred(e) && out.push(e));
  return out;
};
export const byName = (name) => (e) => typeof e.type === "function" && e.type.name === name;

// next/navigation for a case page: /cases/<id>?<search>. Registered after
// render-support.mjs so it wins over the default stub.
export function stubNavigation({ id = "c1", search = "" } = {}) {
  register(
    "data:text/javascript," +
      encodeURIComponent(`
export async function resolve(spec, ctx, next) {
  if (spec === "next/navigation") return { url: "data:text/javascript," + encodeURIComponent(${JSON.stringify(`
    export const useParams = () => ({ id: ${JSON.stringify(id)} });
    export const useRouter = () => ({ push() {}, replace() {}, back() {}, refresh() {}, prefetch() {} });
    export const usePathname = () => "/cases/${id}";
    export const useSearchParams = () => new URLSearchParams(${JSON.stringify(search)});
    export const notFound = () => { throw new Error("notFound"); };
    export const redirect = () => { throw new Error("redirect"); };
  `)}), shortCircuit: true };
  return next(spec, ctx);
}`),
  );
}

export const json = (body, status = 200) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });
