import * as THREE from "three";
import { OrbitControls } from "three/examples/jsm/controls/OrbitControls.js";
import { databaseColor, extras as extraCatalog, frameworkColor, type Config } from "./catalog";

interface Item {
  obj: THREE.Object3D;
  target: number;
  key: string;
  label: string;
  mat?: THREE.MeshStandardMaterial;
  edge?: THREE.LineBasicMaterial;
  orbit?: boolean;
}

export interface Scene3D {
  setConfig(c: Config): void;
  dispose(): void;
}

const reduced = () => window.matchMedia("(prefers-reduced-motion: reduce)").matches;

// A stack of ports and adapters: handler -> service -> repository port -> adapters,
// with the chosen extras orbiting it. It mirrors the layout krok generates.
export function createScene(container: HTMLElement, tooltip: HTMLElement): Scene3D {
  const canvas = document.createElement("canvas");
  container.appendChild(canvas);
  const renderer = new THREE.WebGLRenderer({ canvas, antialias: true, alpha: true });
  renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));

  const scene = new THREE.Scene();
  const camera = new THREE.PerspectiveCamera(38, 1, 0.1, 100);
  camera.position.set(7.5, 4.5, 9);

  scene.add(new THREE.AmbientLight(0xffffff, 0.9));
  const key = new THREE.PointLight(0xffffff, 60, 40);
  key.position.set(5, 8, 6);
  scene.add(key);

  const controls = new OrbitControls(camera, canvas);
  controls.enableDamping = true;
  controls.enablePan = false;
  controls.enableZoom = false;
  controls.minPolarAngle = Math.PI * 0.2;
  controls.maxPolarAngle = Math.PI * 0.65;
  controls.autoRotate = !reduced();
  controls.autoRotateSpeed = 0.9;
  controls.target.set(0, 0.6, 0);
  // Stop spinning while the pointer is down so drags feel direct.
  controls.addEventListener("start", () => (controls.autoRotate = false));
  controls.addEventListener("end", () => (controls.autoRotate = !reduced()));

  const root = new THREE.Group();
  scene.add(root);

  const items = new Map<string, Item>();
  const meshes: THREE.Mesh[] = [];

  function slab(k: string, label: string, color: number, w: number, h: number, d: number, x: number, y: number): Item {
    const geo = new THREE.BoxGeometry(w, h, d);
    const mat = new THREE.MeshStandardMaterial({
      color,
      transparent: true,
      opacity: 0.22,
      roughness: 0.4,
      emissive: color,
      emissiveIntensity: 0.15,
    });
    const mesh = new THREE.Mesh(geo, mat);
    const edge = new THREE.LineBasicMaterial({ color, transparent: true, opacity: 0.95 });
    mesh.add(new THREE.LineSegments(new THREE.EdgesGeometry(geo), edge));
    mesh.position.set(x, y, 0);
    mesh.userData.label = label;
    return { obj: mesh, target: 1, key: k, label, mat, edge };
  }

  function gem(k: string, label: string, color: number): Item {
    const geo = new THREE.OctahedronGeometry(0.34);
    const mat = new THREE.MeshStandardMaterial({
      color,
      roughness: 0.3,
      emissive: color,
      emissiveIntensity: 0.55,
    });
    const mesh = new THREE.Mesh(geo, mat);
    mesh.userData.label = label;
    return { obj: mesh, target: 1, key: k, label, mat, orbit: true };
  }

  function add(item: Item) {
    item.obj.scale.setScalar(0.001);
    root.add(item.obj);
    items.set(item.key, item);
    if (item.obj instanceof THREE.Mesh) meshes.push(item.obj);
  }

  function setConfig(c: Config) {
    const hasDB = c.database !== "none";
    const want = new Map<string, () => Item>();
    want.set(`handler-${c.framework}`, () =>
      slab(`handler-${c.framework}`, `REST handler · ${c.framework}`, frameworkColor[c.framework], 4, 0.5, 2.6, 0, 2.5),
    );
    want.set("service", () => slab("service", "Service · domain logic", 0xe2e8f0, 3.2, 0.5, 2.2, 0, 1.25));
    want.set("port", () => slab("port", "Repository port · interface", 0xf472b6, 2.4, 0.22, 1.7, 0, 0.1));
    want.set(`memory-${hasDB}`, () =>
      slab(`memory-${hasDB}`, "memory adapter · used in unit tests", 0x94a3b8, 2.5, 0.5, 2, hasDB ? -1.6 : 0, -1.15),
    );
    if (hasDB) {
      want.set(`db-${c.database}`, () =>
        slab(`db-${c.database}`, `${c.database} adapter`, databaseColor[c.database], 2.5, 0.5, 2, 1.6, -1.15),
      );
    }
    for (const id of c.extras) {
      const e = extraCatalog.find((x) => x.id === id);
      if (e) want.set(`extra-${id}`, () => gem(`extra-${id}`, e.label, e.color));
    }

    for (const [k, item] of items) if (!want.has(k)) item.target = 0;
    for (const [k, make] of want) {
      const existing = items.get(k);
      if (existing) existing.target = 1;
      else add(make());
    }
  }

  // Requests travelling down the stack and back.
  const pulses = [0, 0.33, 0.66].map((phase, i) => {
    const m = new THREE.Mesh(
      new THREE.SphereGeometry(0.09, 12, 12),
      new THREE.MeshBasicMaterial({ color: 0x5eead4 }),
    );
    root.add(m);
    return { m, phase, x: (i - 1) * 0.6 };
  });

  // Faint floor grid for depth.
  const grid = new THREE.GridHelper(20, 20, 0x334155, 0x1e293b);
  grid.position.y = -1.9;
  (grid.material as THREE.Material).transparent = true;
  (grid.material as THREE.Material).opacity = 0.5;
  scene.add(grid);

  // Hover: highlight a layer and show its name.
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
    renderer.setSize(w, h, false);
    camera.aspect = w / h;
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

  function frame() {
    raf = 0;
    if (!visible) return; // resumes from the IntersectionObserver
    const now = performance.now();
    const dt = Math.min((now - last) / 1000, 0.05);
    last = now;
    t += dt;
    const still = reduced();

    // Ease scale toward the target; drop items that finished shrinking.
    const orbiting: Item[] = [];
    for (const [k, it] of items) {
      const s = it.obj.scale.x;
      const next = s + (it.target - s) * Math.min(1, dt * 8);
      it.obj.scale.setScalar(Math.max(next, 0.001));
      if (it.target === 0 && next < 0.02) {
        root.remove(it.obj);
        const i = meshes.indexOf(it.obj as THREE.Mesh);
        if (i >= 0) meshes.splice(i, 1);
        items.delete(k);
        continue;
      }
      if (it.orbit && it.target === 1) orbiting.push(it);
      if (it.mat && !it.orbit) {
        const on = it === hovered;
        it.mat.opacity += ((on ? 0.5 : 0.22) - it.mat.opacity) * Math.min(1, dt * 10);
      }
    }
    orbiting.forEach((it, i) => {
      const a = (i / orbiting.length) * Math.PI * 2 + (still ? 0 : t * 0.45);
      it.obj.position.set(Math.cos(a) * 4.3, 0.7 + Math.sin(a * 2 + i) * 0.9, Math.sin(a) * 4.3);
      if (!still) it.obj.rotation.set(t * 0.8, t * 0.6, 0);
    });

    pulses.forEach((p) => {
      const ph = still ? 0.5 : (t * 0.22 + p.phase) % 1;
      const down = ph < 0.5 ? ph * 2 : 2 - ph * 2; // 0 -> 1 -> 0
      p.m.position.set(p.x, 2.5 - down * 3.65, 0.4);
    });

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
    },
  };
}
