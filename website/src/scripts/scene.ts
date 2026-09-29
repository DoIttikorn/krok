import * as THREE from "three";
import { OrbitControls } from "three/examples/jsm/controls/OrbitControls.js";
import {
  databaseColor,
  extras as extraCatalog,
  frameworkColor,
  portLabels,
  portSlots,
  type Config,
} from "./catalog";

type Role = "in" | "repo-mem" | "repo-db";

interface Item {
  key: string;
  label: string;
  short: string;
  kind: "core" | "port" | "adapter";
  el?: HTMLElement;
  obj: THREE.Mesh;
  mat: THREE.MeshStandardMaterial;
  target: number; // 1 = present, 0 = shrinking away
  dest: THREE.Vector3; // where the item eases to
  link?: THREE.Line; // connector from its port to this adapter
  from?: THREE.Vector3;
  role?: Role;
  phase: number;
}

export interface Scene3D {
  setConfig(c: Config): void;
  dispose(): void;
}

const reduced = () => window.matchMedia("(prefers-reduced-motion: reduce)").matches;

const CORE_R = 1.9;
const APOTHEM = CORE_R * Math.cos(Math.PI / 6);
const ADAPTER_DIST = 4.7;
const H = 0.45;

// Hexagonal architecture: the domain sits in the middle hexagon, every face is a
// port, and adapters (HTTP, databases, brokers, ...) plug into the ports from outside.
export function createScene(container: HTMLElement, tooltip: HTMLElement): Scene3D {
  const canvas = document.createElement("canvas");
  container.appendChild(canvas);
  const renderer = new THREE.WebGLRenderer({ canvas, antialias: true, alpha: true });
  renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));

  const scene = new THREE.Scene();
  const camera = new THREE.PerspectiveCamera(38, 1, 0.1, 100);
  camera.position.set(0, 12.5, 14.5);

  scene.add(new THREE.AmbientLight(0xffffff, 0.95));
  const light = new THREE.PointLight(0xffffff, 90, 60);
  light.position.set(4, 10, 6);
  scene.add(light);

  const controls = new OrbitControls(camera, canvas);
  controls.enableDamping = true;
  controls.enablePan = false;
  controls.enableZoom = false;
  controls.minPolarAngle = Math.PI * 0.12;
  controls.maxPolarAngle = Math.PI * 0.48;
  controls.autoRotate = !reduced();
  controls.autoRotateSpeed = 0.7;
  // Stop spinning while the pointer is down so drags feel direct.
  controls.addEventListener("start", () => (controls.autoRotate = false));
  controls.addEventListener("end", () => (controls.autoRotate = !reduced()));

  const root = new THREE.Group();
  scene.add(root);

  // Names are HTML, projected from 3D positions each frame, so the text stays crisp.
  const labelLayer = document.createElement("div");
  labelLayer.className = "labels";
  container.appendChild(labelLayer);
  let stageW = 1;
  let stageH = 1;

  const items = new Map<string, Item>();
  const meshes: THREE.Mesh[] = [];

  function make(
    key: string,
    label: string,
    short: string,
    kind: Item["kind"],
    color: number,
    geo: THREE.BufferGeometry,
    opacity = 0.24,
  ): Item {
    const mat = new THREE.MeshStandardMaterial({
      color,
      transparent: true,
      opacity,
      roughness: 0.4,
      emissive: color,
      emissiveIntensity: 0.18,
    });
    const obj = new THREE.Mesh(geo, mat);
    const edge = new THREE.LineBasicMaterial({ color, transparent: true, opacity: 0.95 });
    obj.add(new THREE.LineSegments(new THREE.EdgesGeometry(geo), edge));
    obj.userData.opacity = opacity;
    return { key, label, short, kind, obj, mat, target: 1, dest: new THREE.Vector3(), phase: Math.random() * 6.28 };
  }

  const hex = (r: number, h = H) => new THREE.CylinderGeometry(r, r, h, 6);

  // Points along the direction of a slot, on the ground plane.
  const dir = (deg: number) => new THREE.Vector3(Math.cos((deg * Math.PI) / 180), 0, Math.sin((deg * Math.PI) / 180));

  function place(spec: Item, dest: THREE.Vector3, from?: THREE.Vector3) {
    let it = items.get(spec.key);
    if (!it) {
      it = spec;
      it.obj.scale.setScalar(0.001);
      it.obj.position.copy(dest);
      root.add(it.obj);
      meshes.push(it.obj);
      it.el = document.createElement("span");
      it.el.className = `lbl ${it.kind}`;
      it.el.textContent = it.short;
      labelLayer.appendChild(it.el);
      if (from) {
        const g = new THREE.BufferGeometry().setFromPoints([from, dest]);
        it.link = new THREE.Line(g, new THREE.LineBasicMaterial({ color: 0x475569, transparent: true, opacity: 0.8 }));
        root.add(it.link);
      }
      items.set(it.key, it);
    }
    it.target = 1;
    it.dest.copy(dest);
    it.from = from;
    return it;
  }

  function setConfig(c: Config) {
    const seen = new Set<string>();
    const mark = (it: Item) => seen.add(it.key);

    mark(place(make("core", "Domain · Service and Repository interface. No driver imports.", "domain", "core", 0xe2e8f0, hex(CORE_R, 0.6), 0.3), new THREE.Vector3()));

    // Adapters grouped by the port they plug into, in a stable order.
    const bySlot = new Map<number, { key: string; label: string; short: string; color: number; r: number; role?: Role }[]>();
    const push = (slot: number, a: { key: string; label: string; short: string; color: number; r: number; role?: Role }) =>
      bySlot.set(slot, [...(bySlot.get(slot) ?? []), a]);

    push(portSlots.http, { key: `handler-${c.framework}`, label: `REST adapter · ${c.framework}`, short: c.framework, color: frameworkColor[c.framework], r: 0.95, role: "in" });
    push(portSlots.repository, { key: "memory", label: "memory adapter · used in unit tests", short: "memory", color: 0x94a3b8, r: 0.85, role: "repo-mem" });
    if (c.database !== "none") {
      push(portSlots.repository, { key: `db-${c.database}`, label: `${c.database} adapter`, short: c.database, color: databaseColor[c.database], r: 0.85, role: "repo-db" });
    }
    for (const id of c.extras) {
      const e = extraCatalog.find((x) => x.id === id);
      if (e) push(e.slot, { key: `extra-${id}`, label: `${e.label} adapter`, short: e.label.split(" (")[0], color: e.color, r: 0.62 });
    }

    for (const [slot, list] of bySlot) {
      const d = dir(slot);
      const side = new THREE.Vector3(-d.z, 0, d.x); // perpendicular, for spreading siblings
      // The port: a thin plate on the core's face.
      const plate = make(`port-${slot}`, portLabels[slot], portLabels[slot].split(" ")[0] === "Cache" ? "messaging" : portLabels[slot].split(" ")[0].toLowerCase(), "port", 0xf472b6, new THREE.BoxGeometry(0.16, 0.34, 1.5), 0.55);
      plate.obj.rotation.y = -(slot * Math.PI) / 180;
      const plateAt = d.clone().multiplyScalar(APOTHEM + 0.12);
      mark(place(plate, plateAt));

      const gap = list[0].r * 2 + 0.35;
      list.forEach((a, i) => {
        const offset = (i - (list.length - 1) / 2) * gap;
        const dest = d.clone().multiplyScalar(ADAPTER_DIST).addScaledVector(side, offset);
        const it = place(make(a.key, a.label, a.short, "adapter", a.color, hex(a.r)), dest, plateAt.clone().addScaledVector(d, 0.1));
        it.role = a.role;
        mark(it);
      });
    }

    for (const it of items.values()) if (!seen.has(it.key)) it.target = 0;
  }

  // Requests travelling handler -> domain -> repository adapter and back.
  const pulses = [0, 0.33, 0.66].map((phase) => {
    const m = new THREE.Mesh(new THREE.SphereGeometry(0.11, 12, 12), new THREE.MeshBasicMaterial({ color: 0x5eead4 }));
    root.add(m);
    return { m, phase };
  });

  const grid = new THREE.GridHelper(24, 24, 0x334155, 0x1e293b);
  grid.position.y = -0.6;
  (grid.material as THREE.Material).transparent = true;
  (grid.material as THREE.Material).opacity = 0.45;
  scene.add(grid);

  // Hover: highlight a piece and show its name.
  const ray = new THREE.Raycaster();
  const pointer = new THREE.Vector2();
  let hovered: Item | null = null;
  canvas.addEventListener("pointermove", (ev) => {
    const r = canvas.getBoundingClientRect();
    pointer.set(((ev.clientX - r.left) / r.width) * 2 - 1, -((ev.clientY - r.top) / r.height) * 2 + 1);
    ray.setFromCamera(pointer, camera);
    const hit = ray.intersectObjects(meshes, false)[0];
    const item = hit ? ([...items.values()].find((i) => i.obj === hit.object) ?? null) : null;
    hovered = item && item.target === 1 ? item : null;
    canvas.style.cursor = hovered ? "pointer" : "grab";
    if (hovered) {
      tooltip.textContent = hovered.label;
      tooltip.style.transform = `translate(${ev.clientX - r.left + 14}px, ${ev.clientY - r.top + 14}px)`;
      tooltip.hidden = false;
    } else tooltip.hidden = true;
  });
  canvas.addEventListener("pointerleave", () => {
    hovered = null;
    tooltip.hidden = true;
  });

  function resize() {
    const { clientWidth: w, clientHeight: h } = container;
    if (!w || !h) return;
    stageW = w;
    stageH = h;
    renderer.setSize(w, h, false);
    camera.aspect = w / h;
    // Keep the whole hexagon in view on narrow (mobile) stages.
    const a = w / h;
    camera.fov = a < 0.85 ? 56 : a < 1.2 ? 46 : 38;
    camera.updateProjectionMatrix();
  }
  const ro = new ResizeObserver(resize);
  ro.observe(container);
  resize();

  let last = performance.now();
  let t = 0;
  let visible = true;
  let raf = 0;
  const io = new IntersectionObserver(([e]) => {
    visible = e.isIntersecting;
    last = performance.now();
    if (visible && !raf) raf = requestAnimationFrame(frame);
  });
  io.observe(container);

  const A = new THREE.Vector3();
  const B = new THREE.Vector3();
const ORIGIN = new THREE.Vector3();
const L = new THREE.Vector3();

  function frame() {
    raf = 0;
    if (!visible) return; // resumes from the IntersectionObserver
    const now = performance.now();
    const dt = Math.min((now - last) / 1000, 0.05);
    last = now;
    t += dt;
    const still = reduced();
    const ease = Math.min(1, dt * 7);

    let inItem: Item | undefined;
    let mem: Item | undefined;
    let db: Item | undefined;
    for (const [k, it] of items) {
      const s = it.obj.scale.x;
      const next = s + (it.target - s) * ease;
      it.obj.scale.setScalar(Math.max(next, 0.001));
      it.obj.position.x += (it.dest.x - it.obj.position.x) * ease;
      it.obj.position.z += (it.dest.z - it.obj.position.z) * ease;
      it.obj.position.y = still || it.key === "core" || it.key.startsWith("port-") ? 0 : Math.sin(t * 1.4 + it.phase) * 0.08;

      if (it.link && it.from) {
        const pos = it.link.geometry.getAttribute("position") as THREE.BufferAttribute;
        pos.setXYZ(0, it.from.x, 0, it.from.z);
        pos.setXYZ(1, it.obj.position.x, 0, it.obj.position.z);
        pos.needsUpdate = true;
        (it.link.material as THREE.LineBasicMaterial).opacity = 0.8 * Math.min(1, next);
      }

      const goal = it === hovered ? Math.min(1, it.obj.userData.opacity + 0.3) : it.obj.userData.opacity;
      it.mat.opacity += (goal - it.mat.opacity) * Math.min(1, dt * 10);

      if (it.el) {
        // Ports read from just outside their plate, adapters from above.
        L.copy(it.obj.position);
        if (it.kind === "port") L.multiplyScalar(1.28);
        L.y += it.kind === "core" ? 0.45 : 0.5;
        L.project(camera);
        const x = (L.x * 0.5 + 0.5) * stageW;
        const y = (-L.y * 0.5 + 0.5) * stageH;
        it.el.style.transform = `translate(${x}px, ${y}px) translate(-50%, -100%)`;
        it.el.style.opacity = String(Math.min(1, next) * (it === hovered ? 1 : 0.92));
      }

      if (it.target === 0 && next < 0.02) {
        root.remove(it.obj);
        it.el?.remove();
        if (it.link) root.remove(it.link);
        meshes.splice(meshes.indexOf(it.obj), 1);
        items.delete(k);
        continue;
      }
      if (it.target === 1) {
        if (it.role === "in") inItem = it;
        else if (it.role === "repo-mem") mem = it;
        else if (it.role === "repo-db") db = it;
      }
    }

    const repo = db ?? mem;
    for (const p of pulses) {
      if (!inItem || !repo) {
        p.m.visible = false;
        continue;
      }
      p.m.visible = true;
      const ph = still ? 0.5 : (t * 0.2 + p.phase) % 1;
      const q = ph < 0.5 ? ph * 2 : 2 - ph * 2; // 0 -> 1 -> 0
      A.copy(inItem.obj.position);
      B.copy(repo.obj.position);
      if (q < 0.5) p.m.position.lerpVectors(A, ORIGIN, q * 2);
      else p.m.position.lerpVectors(ORIGIN, B, (q - 0.5) * 2);
      p.m.position.y = 0.55;
    }

    controls.update();
    renderer.render(scene, camera);
    raf = requestAnimationFrame(frame);
  }
  raf = requestAnimationFrame(frame);

  return {
    setConfig,
    dispose() {
      cancelAnimationFrame(raf);
      ro.disconnect();
      io.disconnect();
      controls.dispose();
      renderer.dispose();
      canvas.remove();
      labelLayer.remove();
    },
  };
}
