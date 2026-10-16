// fig.1: the Kobalt heat gun drawn as a 3D blueprint (three.js). Parts are
// grouped the way the demo case groups them; scroll explodes them, then the
// motor-fan assembly and PCBA move to Thailand. Lines, not shading: fat
// outlines, dashed hidden edges, live silhouettes on the shell, hatched
// cores. Colours come from the CSS tokens.
import * as THREE from "three";
import { OrbitControls } from "three/addons/controls/OrbitControls.js";
import { LineSegments2 } from "three/addons/lines/LineSegments2.js";
import { LineSegmentsGeometry } from "three/addons/lines/LineSegmentsGeometry.js";
import { LineMaterial } from "three/addons/lines/LineMaterial.js";
import { mergeVertices } from "three/addons/utils/BufferGeometryUtils.js";

export type Country = "China" | "Thailand";
type GroupKey = "housing" | "heater" | "motor" | "pcba" | "minor";

type Group = {
  key: GroupKey;
  label: string;
  country: Country;
  n: number;
  node: THREE.Group;
  pivot: THREE.Vector3;
  explode: THREE.Vector3;
  labelOffset: [number, number];
  mats: { outline: LineMaterial; detail: LineMaterial; hidden: LineMaterial };
  fillMat: THREE.MeshBasicMaterial;
  hatchMat: THREE.MeshBasicMaterial;
  toneMat: THREE.MeshBasicMaterial;
  hit: THREE.Mesh[];
  el?: HTMLDivElement;
  ln?: SVGPathElement;
  dot?: SVGCircleElement;
};

export type HeatGun = {
  // ex: 0 assembled … 1 exploded; mv: 0 … 1 motor-fan + PCBA moved to Thailand.
  frame: (ex: number, mv: number, dt: number, elapsed: number) => void;
  resize: () => void;
  dispose: () => void;
};

const MOVING: GroupKey[] = ["motor", "pcba"];
const clamp01 = (v: number) => Math.min(1, Math.max(0, v));

export function createHeatGun({
  canvas,
  labels,
  leaders,
  flag,
  reduce,
}: {
  canvas: HTMLCanvasElement;
  labels: HTMLElement;
  leaders: SVGSVGElement;
  flag: (c: Country) => string;
  reduce: boolean;
}): HeatGun {
  const host = canvas.parentElement as HTMLElement;
  const token = (n: string) => getComputedStyle(document.documentElement).getPropertyValue(n).trim();
  const C = {
    ink: new THREE.Color(token("--ink")),
    ok: new THREE.Color(token("--ok")),
    sheet: new THREE.Color(token("--sheet")),
    wash: new THREE.Color(token("--ok-wash")),
    edge: new THREE.Color(token("--edge")),
    dimension: new THREE.Color(token("--dimension")),
  };

  const renderer = new THREE.WebGLRenderer({ canvas, antialias: true, alpha: true });
  renderer.setPixelRatio(Math.min(devicePixelRatio, 2));
  const scene = new THREE.Scene();
  const camera = new THREE.PerspectiveCamera(26, 1, 0.1, 100);
  const controls = new OrbitControls(camera, canvas);
  controls.enableZoom = false;
  controls.enablePan = false;
  controls.enableDamping = !reduce;
  controls.minPolarAngle = Math.PI * 0.28;
  controls.maxPolarAngle = Math.PI * 0.62;
  controls.minAzimuthAngle = -Math.PI * 0.45;
  controls.maxAzimuthAngle = Math.PI * 0.45;

  const model = new THREE.Group();
  scene.add(model);
  const disposables: { dispose: () => void }[] = [];

  // Fat lines so outlines read at 2 px and detail at 1 px. Every edge is
  // drawn again, dashed and faint, where it sits behind a filled part: the
  // blueprint convention for hidden lines.
  const lineMats: LineMaterial[] = [];
  const lm = (o: ConstructorParameters<typeof LineMaterial>[0]) => {
    const m = new LineMaterial({ linewidth: 1, ...o });
    lineMats.push(m);
    disposables.push(m);
    return m;
  };
  const fat = (geom: THREE.BufferGeometry, mat: LineMaterial) => {
    const g = new LineSegmentsGeometry().setPositions(geom.attributes.position.array as Float32Array);
    disposables.push(g);
    const l = new LineSegments2(g, mat);
    l.computeLineDistances();
    return l;
  };

  // Section hatching on the solid cores (motor, heater core, PCBA).
  const hatch = (() => {
    const cv = document.createElement("canvas");
    cv.width = cv.height = 32;
    const x = cv.getContext("2d")!;
    x.fillStyle = token("--sheet");
    x.fillRect(0, 0, 32, 32);
    x.strokeStyle = token("--edge");
    x.lineWidth = 1.2;
    for (let i = -32; i < 64; i += 8) {
      x.beginPath();
      x.moveTo(i, 32);
      x.lineTo(i + 32, 0);
      x.stroke();
    }
    const t = new THREE.CanvasTexture(cv);
    t.wrapS = t.wrapT = THREE.RepeatWrapping;
    t.repeat.set(3, 3);
    t.colorSpace = THREE.SRGBColorSpace;
    disposables.push(t);
    return t;
  })();

  // Silhouette edges: an edge whose two faces point opposite ways relative
  // to the camera. Adjacency is precomputed; the test runs every frame.
  type Sil = { p: THREE.BufferAttribute; n: Float32Array; o: Float32Array; edges: { u: number; v: number; f1: number; f2: number }[]; owner: THREE.Object3D; line: LineSegments2 };
  const sils: Sil[] = [];
  function silhouette(geom: THREE.BufferGeometry, owner: THREE.Object3D, mat: LineMaterial) {
    const src = geom.clone();
    src.deleteAttribute("normal");
    src.deleteAttribute("uv");
    const g = mergeVertices(src, 1e-4);
    const p = g.attributes.position as THREE.BufferAttribute;
    const ix = g.index!.array;
    const nF = ix.length / 3;
    const n = new Float32Array(nF * 3);
    const o = new Float32Array(nF * 3);
    const a = new THREE.Vector3(), b = new THREE.Vector3(), c = new THREE.Vector3();
    const map = new Map<string, { u: number; v: number; f1: number; f2: number }>();
    const edges: { u: number; v: number; f1: number; f2: number }[] = [];
    for (let f = 0; f < nF; f++) {
      const i = ix[f * 3], j = ix[f * 3 + 1], k = ix[f * 3 + 2];
      a.fromBufferAttribute(p, i);
      b.fromBufferAttribute(p, j);
      c.fromBufferAttribute(p, k);
      const nn = new THREE.Vector3().subVectors(c, b).cross(new THREE.Vector3().subVectors(a, b)).normalize();
      n.set([nn.x, nn.y, nn.z], f * 3);
      o.set([a.x, a.y, a.z], f * 3);
      for (const [u, v] of [[i, j], [j, k], [k, i]]) {
        const key = u < v ? `${u}_${v}` : `${v}_${u}`;
        const e = map.get(key);
        if (e) e.f2 = f;
        else {
          const ne = { u, v, f1: f, f2: -1 };
          map.set(key, ne);
          edges.push(ne);
        }
      }
    }
    const lg = new LineSegmentsGeometry();
    disposables.push(lg, g, src);
    const line = new LineSegments2(lg, mat);
    owner.add(line);
    sils.push({ p, n, o, edges: edges.filter((e) => e.f2 >= 0), owner, line });
  }
  const camLocal = new THREE.Vector3();
  function updateSilhouettes() {
    for (const s of sils) {
      s.owner.updateWorldMatrix(true, false);
      camLocal.copy(camera.position);
      s.owner.worldToLocal(camLocal);
      const facing = (f: number) =>
        s.n[f * 3] * (camLocal.x - s.o[f * 3]) + s.n[f * 3 + 1] * (camLocal.y - s.o[f * 3 + 1]) + s.n[f * 3 + 2] * (camLocal.z - s.o[f * 3 + 2]) > 0;
      const out: number[] = [];
      for (const e of s.edges) {
        if (facing(e.f1) === facing(e.f2)) continue;
        out.push(s.p.getX(e.u), s.p.getY(e.u), s.p.getZ(e.u), s.p.getX(e.v), s.p.getY(e.v), s.p.getZ(e.v));
      }
      if (out.length) s.line.geometry.setPositions(out);
      s.line.visible = out.length > 0;
    }
  }

  // One part: outline (or detail) lines, dashed hidden lines, and an opaque
  // fill so parts in front hide what is behind. Shells (housing, covers) are
  // lines only, an x-ray, so the parts inside stay visible.
  type PartOpts = { x?: number; y?: number; z?: number; rx?: number; ry?: number; rz?: number; shell?: boolean; detail?: boolean; hatched?: boolean; extra?: THREE.BufferGeometry[]; bold?: THREE.BufferGeometry[]; sil?: boolean; crease?: number };
  function part(group: Group, geom: THREE.BufferGeometry, { x = 0, y = 0, z = 0, rx = 0, ry = 0, rz = 0, shell = false, detail = false, hatched = false, extra = [], bold = [], sil = false, crease = 20 }: PartOpts = {}) {
    disposables.push(geom);
    const g = new THREE.Group();
    g.position.set(x, y, z);
    g.rotation.set(rx, ry, rz);
    const edges = new THREE.EdgesGeometry(geom, crease);
    disposables.push(edges);
    if (sil) {
      silhouette(geom, g, group.mats.outline);
      const tone = new THREE.Mesh(geom, group.toneMat);
      tone.renderOrder = -1;
      g.add(tone);
    }
    g.add(fat(edges, detail ? group.mats.detail : group.mats.outline));
    g.add(fat(edges, group.mats.hidden));
    for (const e of extra) {
      disposables.push(e);
      g.add(fat(e, group.mats.detail), fat(e, group.mats.hidden));
    }
    for (const e of bold) {
      disposables.push(e);
      g.add(fat(e, group.mats.outline), fat(e, group.mats.hidden));
    }
    if (!shell) {
      const fill = new THREE.Mesh(geom, hatched ? group.hatchMat : group.fillMat);
      fill.userData.group = group;
      group.hit.push(fill);
      g.add(fill);
    }
    group.node.add(g);
    return g;
  }

  const groups: Partial<Record<GroupKey, Group>> = {};
  function mkGroup(key: GroupKey, label: string, country: Country, n: number, pivot: THREE.Vector3, explode: THREE.Vector3, labelOffset: [number, number]) {
    const col = country === "China" ? C.ok : C.ink;
    const mats = {
      outline: lm({ color: col, linewidth: 2 }),
      detail: lm({ color: col, linewidth: 1 }),
      hidden: lm({ color: col, linewidth: 1, dashed: true, dashSize: 0.05, gapSize: 0.05, transparent: true, opacity: 0.3, depthFunc: THREE.GreaterDepth, depthWrite: false }),
    };
    const fillCol = country === "China" ? C.wash : C.sheet;
    const fillOpts = { polygonOffset: true, polygonOffsetFactor: 1, polygonOffsetUnits: 1 };
    const fillMat = new THREE.MeshBasicMaterial({ color: fillCol, ...fillOpts });
    const hatchMat = new THREE.MeshBasicMaterial({ color: fillCol, map: hatch, ...fillOpts });
    const toneMat = new THREE.MeshBasicMaterial({ color: C.ok, transparent: true, opacity: 0.045, depthWrite: false, side: THREE.DoubleSide });
    disposables.push(fillMat, hatchMat, toneMat);
    const node = new THREE.Group();
    node.position.copy(pivot);
    model.add(node);
    const g: Group = { key, label, country, n, node, pivot: pivot.clone(), explode, labelOffset, mats, fillMat, hatchMat, toneMat, hit: [] };
    groups[key] = g;
    return g;
  }

  const V = (x: number, y: number, z = 0) => new THREE.Vector3(x, y, z);
  const along = <T extends THREE.BufferGeometry>(geom: T) => geom.rotateZ(-Math.PI / 2); // runs along +X, nozzle right
  const lathe = (pts: [number, number][], seg = 72) => along(new THREE.LatheGeometry(pts.map(([r, x]) => new THREE.Vector2(r, x)), seg));
  const seg = (a: THREE.Vector3, b: THREE.Vector3) => new THREE.BufferGeometry().setFromPoints([a, b]);
  const polyline = (pts: THREE.Vector3[]) => new THREE.BufferGeometry().setFromPoints(pts.flatMap((p, k) => (k ? [pts[k - 1], p] : [])));
  const at = (a: number, r: number, x: number) => V(x, Math.cos(a) * r, Math.sin(a) * r);

  // ---- Housing and front cover (Thailand): the chassis --------------------
  const housing = mkGroup("housing", "Main housing + front cover", "Thailand", 1, V(0, 0), V(0, 0), [140, -120]);
  {
    const prof = new THREE.SplineCurve(
      ([[0.34, -1.5], [0.55, -1.47], [0.64, -1.36], [0.655, -1.0], [0.64, -0.6], [0.575, -0.3], [0.54, 0.0], [0.535, 0.7], [0.545, 1.25], [0.52, 1.36]] as [number, number][]).map(([r, x]) => new THREE.Vector2(r, x)),
    ).getPoints(48);
    const barrel = along(new THREE.LatheGeometry(prof, 96));
    const rings = [0, 6, 18, 30, 45, 48].map((i) => {
      const pts: THREE.Vector3[] = [];
      for (let k = 0; k <= 64; k++) pts.push(at((k / 64) * Math.PI * 2, prof[i].x, prof[i].y));
      return polyline(pts);
    });
    // Parting line of the two clamshell halves, and the side vents.
    const seam = [0, Math.PI].map((a) => polyline(prof.map((p) => at(a, p.x + 0.002, p.y))));
    const vents: THREE.BufferGeometry[] = [];
    for (let i = 0; i < 8; i++) {
      const x = -1.2 + i * 0.085;
      vents.push(seg(at(0.95, 0.66, x), at(2.05, 0.66, x)));
    }
    part(housing, barrel, { shell: true, sil: true, crease: 50, extra: [...rings, ...seam, ...vents] });

    // Grip: a rounded, raked outline with a soft bevel, running up into the
    // barrel so the two read as one moulding.
    const s = new THREE.Shape();
    s.moveTo(-0.18, -0.3);
    s.bezierCurveTo(-0.24, -0.75, -0.36, -1.25, -0.38, -1.62);
    s.quadraticCurveTo(-0.4, -1.95, -0.22, -2.06);
    s.quadraticCurveTo(-0.2, -2.16, -0.36, -2.18);
    s.lineTo(-1.2, -2.18);
    s.quadraticCurveTo(-1.4, -2.15, -1.3, -1.95);
    s.bezierCurveTo(-1.2, -1.5, -1.08, -0.85, -1.06, -0.3);
    s.lineTo(-0.18, -0.3);
    const grip = new THREE.ExtrudeGeometry(s, { depth: 0.34, bevelEnabled: true, bevelThickness: 0.08, bevelSize: 0.07, bevelSegments: 4, curveSegments: 16 }).translate(0, 0, -0.17);
    const gripSeam = polyline(s.getPoints(24).map((p) => V(p.x, p.y, 0.252)));
    part(housing, grip, { shell: true, sil: true, crease: 60, extra: [gripSeam] });

    const nprof = ([[0.53, 0], [0.56, 0.03], [0.56, 0.12], [0.5, 0.3], [0.42, 0.52], [0.41, 0.6], [0.32, 0.62]] as [number, number][]).map(([r, x]) => new THREE.Vector2(r, x));
    part(housing, along(new THREE.LatheGeometry(nprof, 96)), { x: 1.36, shell: true, sil: true, crease: 25 });
  }

  // ---- Heating element (Thailand) ------------------------------------------
  const heater = mkGroup("heater", "Heating element", "Thailand", 2, V(0.75, 0), V(0.75, 1.05), [40, 70]);
  {
    const pts: THREE.Vector3[] = [];
    for (let t = 0; t <= 1; t += 1 / 500) {
      const a = t * Math.PI * 2 * 12;
      pts.push(V(-0.55 + t * 1.1, Math.cos(a) * 0.3, Math.sin(a) * 0.3));
    }
    part(heater, new THREE.TubeGeometry(new THREE.CatmullRomCurve3(pts), 700, 0.02, 6), { detail: true });
    part(heater, new THREE.BoxGeometry(1.2, 0.56, 0.03), { hatched: true });
    part(heater, new THREE.BoxGeometry(1.2, 0.03, 0.56), { hatched: true });
  }

  // ---- Motor-fan assembly (China) ------------------------------------------
  const motor = mkGroup("motor", "Motor-fan assembly", "China", 3, V(-0.7, 0), V(-0.55, 1.15), [-90, -60]);
  {
    part(motor, lathe([[0.001, -0.36], [0.22, -0.36], [0.28, -0.3], [0.28, 0.24], [0.2, 0.3], [0.001, 0.3]], 48), { x: -0.05, hatched: true });
    part(motor, along(new THREE.CylinderGeometry(0.05, 0.05, 0.42, 16)), { x: 0.42, detail: true });
    part(motor, along(new THREE.CylinderGeometry(0.12, 0.12, 0.1, 24)), { x: 0.6 });
    for (let i = 0; i < 9; i++) part(motor, new THREE.BoxGeometry(0.025, 0.32, 0.12).translate(0, 0.27, 0).rotateY(0.5).rotateX((i / 9) * Math.PI * 2), { x: 0.6, detail: true });
    part(motor, new THREE.TorusGeometry(0.5, 0.035, 8, 64).rotateY(Math.PI / 2), { x: -0.4, detail: true });
  }

  // ---- PCBA (China) ----------------------------------------------------------
  const pcba = mkGroup("pcba", "PCBA", "China", 4, V(-0.72, -1.25), V(-1.45, -0.2), [-110, 30]);
  {
    part(pcba, new THREE.BoxGeometry(0.05, 0.95, 0.32), { rz: -0.15, hatched: true });
    for (const [y, h] of [[0.28, 0.12], [0.05, 0.2], [-0.22, 0.1]]) part(pcba, new THREE.BoxGeometry(0.08, h, 0.14), { x: 0.06 + y * 0.15, y, rz: -0.15, detail: true });
  }

  // ---- Switch, cord and minor parts (China) --------------------------------
  const minor = mkGroup("minor", "Switch, cord + minor parts", "China", 5, V(-0.25, -0.8), V(1.05, -0.3), [80, 60]);
  {
    const t = new THREE.Shape();
    t.moveTo(0, 0.2);
    t.lineTo(0.13, 0.14);
    t.quadraticCurveTo(0.16, -0.05, 0.1, -0.2);
    t.lineTo(-0.02, -0.15);
    t.lineTo(0, 0.2);
    part(minor, new THREE.ExtrudeGeometry(t, { depth: 0.16, bevelEnabled: false }).translate(0, 0, -0.08));
    part(minor, lathe([[0.001, -0.03], [0.6, -0.03], [0.6, 0.03], [0.001, 0.03]], 64), { x: -1.17, y: 0.8 });
    const grille: THREE.BufferGeometry[] = [];
    for (let i = -4; i <= 4; i++) {
      const h = Math.sqrt(Math.max(0, 0.25 - (i * 0.1) ** 2));
      grille.push(seg(V(-1.21, 0.8 + i * 0.1, -h), V(-1.21, 0.8 + i * 0.1, h)));
    }
    part(minor, new THREE.BoxGeometry(0.001, 0.001, 0.001), { shell: true, extra: grille });
    part(minor, new THREE.CylinderGeometry(0.1, 0.1, 0.12, 24), { x: -0.7, y: 1.44 });
    part(minor, along(new THREE.CylinderGeometry(0.09, 0.07, 0.22, 16)).rotateZ(-Math.PI / 2), { x: -0.56, y: -1.4 });
    const cord = new THREE.CatmullRomCurve3([V(-0.56, -1.5), V(-0.6, -1.7), V(-0.9, -1.9), V(-1.5, -1.98), V(-2.1, -1.84)]);
    part(minor, new THREE.TubeGeometry(cord, 90, 0.045, 8), { detail: true });
  }

  // Centre line through the barrel.
  const centre = lm({ color: C.dimension, linewidth: 1, dashed: true, dashSize: 0.22, gapSize: 0.07, transparent: true, opacity: 0.85 });
  model.add(fat(seg(V(-2.5, 0), V(2.7, 0)), centre));
  model.position.set(0.15, 0.1, 0);

  // ---- Callouts: HTML labels with SVG leaders, projected each frame --------
  const SVGNS = "http://www.w3.org/2000/svg";
  const org = (c: Country) => {
    const span = document.createElement("span");
    span.className = "org";
    const img = document.createElement("img");
    img.className = "flag";
    img.alt = "";
    img.src = flag(c);
    span.append(img, c);
    return span;
  };
  const all = Object.values(groups) as Group[];
  for (const g of all) {
    const el = document.createElement("div");
    el.className = "callout";
    const ball = document.createElement("span");
    ball.className = "ball";
    ball.textContent = String(g.n);
    const name = document.createElement("span");
    name.textContent = g.label;
    const o = document.createElement("span");
    o.className = "o";
    o.append(org(g.country));
    el.append(ball, name, "·", o);
    el.dataset.c = g.country;
    labels.appendChild(el);
    g.el = el;
    g.ln = document.createElementNS(SVGNS, "path");
    g.ln.setAttribute("fill", "none");
    g.ln.setAttribute("stroke-width", "1");
    g.dot = document.createElementNS(SVGNS, "circle");
    g.dot.setAttribute("r", "2.5");
    leaders.append(g.ln, g.dot);
  }

  // ---- Camera, hover, sizing ------------------------------------------------
  const base = { az: 0.42, pol: 1.32, r: 11.5 };
  const placeCamera = () => {
    camera.position.setFromSphericalCoords(base.r, base.pol, base.az);
    controls.target.set(0, 0, 0);
    controls.update();
  };
  const ray = new THREE.Raycaster();
  const mouse = new THREE.Vector2(9, 9);
  const onMove = (e: PointerEvent) => {
    const r = canvas.getBoundingClientRect();
    mouse.set(((e.clientX - r.left) / r.width) * 2 - 1, -((e.clientY - r.top) / r.height) * 2 + 1);
  };
  const onLeave = () => mouse.set(9, 9);
  canvas.addEventListener("pointermove", onMove);
  canvas.addEventListener("pointerleave", onLeave);

  function resize() {
    const w = host.clientWidth, h = host.clientHeight;
    if (!w || !h) return;
    renderer.setSize(w, h, false);
    for (const m of lineMats) m.resolution.set(w, h);
    camera.aspect = w / h;
    base.r = w / h < 0.7 ? 24 : w / h < 1 ? 18 : w / h < 1.4 ? 13 : 11.5;
    camera.updateProjectionMatrix();
    leaders.setAttribute("viewBox", `0 0 ${w} ${h}`);
    placeCamera();
  }
  resize();

  const tmp = new THREE.Vector3();
  const lineCol = new THREE.Color(), fillCol = new THREE.Color();
  const inkHex = token("--ink"), okHex = token("--ok");

  function frame(ex: number, mv: number, _dt: number, elapsed: number) {
    if (!reduce) model.rotation.y = Math.sin(elapsed * 0.35) * 0.08; // gentle idle turn

    ray.setFromCamera(mouse, camera);
    const hit = ray.intersectObjects(all.flatMap((g) => g.hit))[0];
    const hot = (hit?.object.userData.group as Group | undefined)?.key;

    const w = host.clientWidth, h = host.clientHeight;
    for (const g of all) {
      const moving = MOVING.includes(g.key);
      const t = moving ? mv : 0;
      g.node.position.copy(g.pivot).addScaledVector(g.explode, ex);
      if (moving && !reduce) {
        g.node.position.y += Math.sin(Math.PI * t) * 0.4; // lifts across to Thailand
        g.node.rotation.x = t * Math.PI * 2;
      }
      const cn = g.country === "China" && !(moving && t > 0.5);
      lineCol.copy(g.country === "China" ? C.ok : C.ink).lerp(C.ink, t);
      for (const m of Object.values(g.mats)) m.color.copy(lineCol);
      fillCol.copy(g.country === "China" ? C.wash : C.sheet).lerp(C.sheet, t);
      g.fillMat.color.copy(fillCol);
      g.hatchMat.color.copy(fillCol);
      if (g.key === hot) g.mats.outline.color.copy(C.ok).multiplyScalar(0.6);

      // Callout anchored at the group's centre, offset on screen, kept inside the sheet.
      g.node.getWorldPosition(tmp);
      tmp.project(camera);
      const ax = (tmp.x * 0.5 + 0.5) * w, ay = (-tmp.y * 0.5 + 0.5) * h;
      const sc = w < 600 ? 0.55 : 1;
      const lx = ax + g.labelOffset[0] * sc, ly = ay + g.labelOffset[1] * sc;
      const vis = clamp01((ex - 0.25) / 0.4);
      const el = g.el!;
      el.style.opacity = String(vis);
      el.classList.toggle("cn", cn);
      el.classList.toggle("hot", g.key === hot);
      const want: Country = cn ? "China" : "Thailand";
      if (el.dataset.c !== want) {
        el.dataset.c = want;
        el.querySelector(".o")!.replaceChildren(org(want));
      }
      const right = g.labelOffset[0] < 0;
      const ew = el.offsetWidth;
      const left = Math.min(w - ew - 10, Math.max(10, right ? lx - ew : lx));
      const top = Math.min(h - 70, Math.max(70, ly));
      el.style.left = `${left}px`;
      el.style.top = `${top}px`;
      const col = cn ? okHex : inkHex;
      g.ln!.setAttribute("d", `M${ax},${ay} L${right ? left + ew : left},${top}`);
      g.ln!.setAttribute("stroke", col);
      g.ln!.style.opacity = String(vis);
      g.dot!.setAttribute("cx", String(ax));
      g.dot!.setAttribute("cy", String(ay));
      g.dot!.setAttribute("fill", col);
      g.dot!.style.opacity = String(vis);
    }
    canvas.style.cursor = hot ? "pointer" : "";
    controls.update();
    updateSilhouettes();
    renderer.render(scene, camera);
  }

  return {
    frame,
    resize,
    dispose() {
      canvas.removeEventListener("pointermove", onMove);
      canvas.removeEventListener("pointerleave", onLeave);
      controls.dispose();
      for (const d of disposables) d.dispose();
      renderer.dispose();
      labels.replaceChildren();
      leaders.replaceChildren();
    },
  };
}
