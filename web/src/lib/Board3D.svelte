<script lang="ts">
  import { onMount } from "svelte";
  import * as THREE from "three";
  import { TrackballControls } from "three/examples/jsm/controls/TrackballControls.js";
  import { CSS2DObject, CSS2DRenderer } from "three/examples/jsm/renderers/CSS2DRenderer.js";
  import type { Fleet, GameEvent, PlanetView } from "./proto";
  import { play } from "./audio";
  import { soundFor } from "./store.svelte";
  import { travelTurns } from "./proto";
  import { ownerHex } from "./colors";
  import { territory } from "./territory";

  // Same props as Board.svelte, so the game can swap views.
  interface Props {
    planets: PlanetView[];
    width: number;
    height: number;
    depth: number;
    shipSpeed: number;
    seat?: number;
    fleets?: Fleet[];
    round?: number;
    source?: number | null;
    dest?: number | null;
    onpick?: (id: number) => void;
    onhover?: (id: number | null) => void;
    /** The last round's events; each new round's battles are replayed. */
    replay?: { round: number; events: GameEvent[] } | null;
    /** Called when a round's replay has finished (or had nothing to show). */
    onreplaydone?: () => void;
  }

  let {
    planets,
    width,
    height,
    depth,
    shipSpeed,
    seat = -2,
    fleets = [],
    round = 0,
    source = null,
    dest = null,
    onpick,
    onhover,
    replay = null,
    onreplaydone,
  }: Props = $props();

  // A shallow map spreads its few layers apart so height reads clearly; a
  // cube keeps cells cubic.
  let layerGap = $derived(depth <= 4 ? 1.5 : 1);
  let shallow = $derived(depth > 1 && depth <= 4);
  // Planets, arrows and labels grow with the map so a big cube stays legible.
  let unit = $derived(Math.max(1, Math.max(width, height, depth) / 16));

  let host: HTMLDivElement;
  let hover = $state<number | null>(null);

  let scene: THREE.Scene;
  let camera: THREE.PerspectiveCamera;
  let renderer: THREE.WebGLRenderer;
  let labels: CSS2DRenderer;
  let controls: TrackballControls;
  let dynamic: THREE.Group; // rebuilt whenever the view changes
  let pickables: THREE.Mesh[] = [];
  let rings: THREE.Mesh[] = []; // selection rings turn to face the camera
  let webgl = $state(true);
  let resetView = $state<() => void>(() => {});

  const sphere = new THREE.SphereGeometry(1, 24, 16);
  const floorDot = new THREE.CircleGeometry(0.12, 16).rotateX(-Math.PI / 2);
  const box = new THREE.BoxGeometry(1, 1, 1);

  function toWorld(p: PlanetView): THREE.Vector3 {
    return new THREE.Vector3(
      p.pos.x + 0.5 - width / 2,
      p.pos.z * layerGap - ((depth - 1) * layerGap) / 2,
      p.pos.y + 0.5 - height / 2,
    );
  }
  const floorY = () => -((depth - 1) * layerGap) / 2 - 0.6;
  const radius = (p: PlanetView) => (0.26 + Math.min(1, (p.ships ?? 10) / 200) * 0.12) * unit;

  // A label is anchored at a point and drawn in screen space, just right of
  // it, so it stays upright and beside its planet however the map is turned.
  function label(text: string, cls: string): CSS2DObject {
    const anchor = document.createElement("div");
    anchor.className = "anchor";
    const el = document.createElement("span");
    el.className = cls;
    el.textContent = text;
    anchor.appendChild(el);
    return new CSS2DObject(anchor);
  }

  function dispose(obj: THREE.Object3D) {
    obj.traverse((o) => {
      if (o instanceof CSS2DObject) o.element.remove();
      const m = (o as THREE.Mesh).material as THREE.Material | THREE.Material[] | undefined;
      if (m) (Array.isArray(m) ? m : [m]).forEach((x) => x.dispose());
      const g = (o as THREE.Mesh).geometry as THREE.BufferGeometry | undefined;
      if (g && g !== sphere && g !== floorDot && g !== box) g.dispose();
    });
  }

  // The static frame: the outline of the playing volume, a floor grid on
  // flat and shallow maps, and a faint lattice of points through a cube.
  function buildFrame() {
    if (depth <= 4) {
      const size = Math.max(width, height);
      const grid = new THREE.GridHelper(size, size, 0x252c44, 0x1c2236);
      grid.position.y = floorY();
      scene.add(grid);
    } else {
      const step = Math.max(1, Math.round(Math.max(width, height, depth) / 8));
      const pts: number[] = [];
      for (let x = 0; x <= width; x += step)
        for (let y = 0; y <= height; y += step)
          for (let z = 0; z <= depth; z += step)
            pts.push(x - width / 2, z * layerGap - ((depth - 1) * layerGap) / 2 - 0.5, y - height / 2);
      const geo = new THREE.BufferGeometry().setAttribute("position", new THREE.Float32BufferAttribute(pts, 3));
      scene.add(
        new THREE.Points(geo, new THREE.PointsMaterial({ color: 0x3a4466, size: 0.12 * unit, sizeAttenuation: true })),
      );
    }
    const box = new THREE.BoxGeometry(width, (depth - 1) * layerGap + 1.2, height);
    const edges = new THREE.LineSegments(
      new THREE.EdgesGeometry(box),
      new THREE.LineBasicMaterial({ color: 0x252c44 }),
    );
    edges.position.y = floorY() + ((depth - 1) * layerGap + 1.2) / 2;
    scene.add(edges);
  }

  function rebuild() {
    if (!dynamic) return;
    for (const child of [...dynamic.children]) {
      dispose(child);
      dynamic.remove(child);
    }
    pickables = [];
    rings = [];

    for (const p of planets) {
      const at = toWorld(p);
      const color = new THREE.Color(ownerHex(shownOwner.get(p.id) ?? p.owner));
      const selected = p.id === source || p.id === dest;
      const mesh = new THREE.Mesh(
        sphere,
        new THREE.MeshStandardMaterial({
          color,
          emissive: color,
          emissiveIntensity: selected ? 0.9 : p.id === hover ? 0.6 : 0.25,
          roughness: 0.5,
        }),
      );
      mesh.scale.setScalar(radius(p));
      mesh.position.copy(at);
      mesh.userData.id = p.id;
      dynamic.add(mesh);
      pickables.push(mesh);

      if (selected) {
        const ring = new THREE.Mesh(
          new THREE.TorusGeometry(radius(p) + 0.14, 0.03, 8, 40),
          new THREE.MeshBasicMaterial({ color: 0xe6e9f2 }),
        );
        ring.position.copy(at);
        ring.lookAt(camera.position);
        rings.push(ring);
        dynamic.add(ring);
      }

      // On a shallow map, a drop line and floor dot show where the planet
      // sits on the floor. In a cube the lattice gives depth instead.
      if (shallow || depth === 1) {
        const base = new THREE.Vector3(at.x, floorY(), at.z);
        dynamic.add(
          new THREE.Line(
            new THREE.BufferGeometry().setFromPoints([at, base]),
            new THREE.LineBasicMaterial({ color, transparent: true, opacity: 0.35 }),
          ),
        );
        const dot = new THREE.Mesh(floorDot, new THREE.MeshBasicMaterial({ color, transparent: true, opacity: 0.5 }));
        dot.position.copy(base).y += 0.01;
        dynamic.add(dot);
      }

      // Labels are screen-space, so they stay upright and readable however
      // the cube is turned.
      const tag = label(p.ships !== undefined ? `${p.name} ${p.ships}` : p.name, p.owner === seat ? "tag mine" : "tag");
      tag.position.copy(at);
      dynamic.add(tag);
    }

    // Territory: translucent tinted blocks, one instanced mesh per owner, so
    // each empire reads as a coloured cloud with pockets where it has pushed
    // into another's space.
    const byOwner = new Map<number, ReturnType<typeof territory>>();
    for (const b of territory(planets, width, height, depth)) {
      if (!byOwner.has(b.owner)) byOwner.set(b.owner, []);
      byOwner.get(b.owner)!.push(b);
    }
    const m = new THREE.Matrix4();
    for (const [owner, blocks] of byOwner) {
      const mat = new THREE.MeshBasicMaterial({
        color: new THREE.Color(ownerHex(owner)),
        transparent: true,
        opacity: depth > 1 ? 0.045 : 0.12,
        depthWrite: false,
      });
      const mesh = new THREE.InstancedMesh(box, mat, blocks.length);
      blocks.forEach((b, i) => {
        const h = b.depth * layerGap;
        m.makeScale(b.size * 0.98, h * 0.98, b.size * 0.98);
        m.setPosition(
          b.x + b.size / 2 - width / 2,
          (b.z + b.depth / 2) * layerGap - ((depth - 1) * layerGap) / 2 - layerGap / 2,
          b.y + b.size / 2 - height / 2,
        );
        mesh.setMatrixAt(i, m);
      });
      mesh.renderOrder = -1;
      dynamic.add(mesh);
    }

    for (const f of fleets) {
      const a = toWorld(planets[f.from]);
      const b = toWorld(planets[f.to]);
      const color = new THREE.Color(ownerHex(f.owner));
      const line = new THREE.Line(
        new THREE.BufferGeometry().setFromPoints([a, b]),
        new THREE.LineDashedMaterial({ color, dashSize: 0.15, gapSize: 0.12, transparent: true, opacity: 0.6 }),
      );
      line.computeLineDistances();
      dynamic.add(line);
      const turns = f.arrivalRound - f.launchRound + 1;
      const t = Math.min(1, Math.max(0, (round - f.launchRound) / turns));
      const cone = new THREE.Mesh(new THREE.ConeGeometry(0.1 * unit, 0.3 * unit, 12), new THREE.MeshBasicMaterial({ color }));
      cone.position.lerpVectors(a, b, t);
      cone.quaternion.setFromUnitVectors(new THREE.Vector3(0, 1, 0), b.clone().sub(a).normalize());
      dynamic.add(cone);
    }

    const target = dest ?? (source !== null && hover !== source ? hover : null);
    if (source !== null && target !== null) {
      const a = toWorld(planets[source]);
      const b = toWorld(planets[target]);
      const aim = new THREE.Line(
        new THREE.BufferGeometry().setFromPoints([a, b]),
        new THREE.LineDashedMaterial({ color: 0xe6e9f2, dashSize: 0.2, gapSize: 0.12 }),
      );
      aim.computeLineDistances();
      dynamic.add(aim);
      const turns = travelTurns(planets[source].pos, planets[target].pos, shipSpeed);
      const tag = label(`${turns} ${turns === 1 ? "round" : "rounds"}`, "tag aim");
      tag.position.lerpVectors(a, b, 0.5);
      dynamic.add(tag);
    }
  }

  // Rebuild whenever anything shown changes.
  $effect(() => {
    void [planets, fleets, round, source, dest, hover, seat, shownOwner];
    rebuild();
  });

  // --- Camera moves -------------------------------------------------------

  // One camera move at a time, eased, toward a position and target.
  let move: {
    fromPos: THREE.Vector3;
    fromTarget: THREE.Vector3;
    fromUp: THREE.Vector3;
    toPos: THREE.Vector3;
    toTarget: THREE.Vector3;
    toUp: THREE.Vector3;
    start: number;
    dur: number;
    done: () => void;
  } | null = null;

  function flyTo(pos: THREE.Vector3, target: THREE.Vector3, up: THREE.Vector3, dur = 700): Promise<void> {
    return new Promise((done) => {
      move?.done();
      move = {
        fromPos: camera.position.clone(),
        fromTarget: controls.target.clone(),
        fromUp: camera.up.clone(),
        toPos: pos.clone(),
        toTarget: target.clone(),
        toUp: up.clone().normalize(),
        start: performance.now(),
        dur,
        done,
      };
    });
  }

  function stepMove(now: number) {
    if (!move) return;
    const t = Math.min(1, (now - move.start) / move.dur);
    const k = t < 0.5 ? 2 * t * t : 1 - (-2 * t + 2) ** 2 / 2; // ease in-out
    camera.position.lerpVectors(move.fromPos, move.toPos, k);
    controls.target.lerpVectors(move.fromTarget, move.toTarget, k);
    camera.up.lerpVectors(move.fromUp, move.toUp, k).normalize();
    camera.lookAt(controls.target);
    if (t >= 1) {
      const m = move;
      move = null;
      m.done();
    }
  }

  const span = () => Math.max(width, height, depth * layerGap);
  const presets: Record<string, [THREE.Vector3, THREE.Vector3]> = {
    Top: [new THREE.Vector3(0, 2.3, 0.001), new THREE.Vector3(0, 0, -1)],
    Front: [new THREE.Vector3(0, 0, 2.3), new THREE.Vector3(0, 1, 0)],
    Side: [new THREE.Vector3(2.3, 0, 0), new THREE.Vector3(0, 1, 0)],
    Corner: [new THREE.Vector3(0.7, 0.9, 1.75), new THREE.Vector3(0, 1, 0)],
  };
  function viewFrom(name: string) {
    const [dir, up] = presets[name];
    flyTo(dir.clone().multiplyScalar(span()), new THREE.Vector3(), up);
  }

  // Focus on a planet from the current direction, close enough to see it.
  function focusOn(id: number, dur = 700) {
    const at = toWorld(planets[id]);
    const dir = camera.position.clone().sub(controls.target).normalize();
    // Far enough back to see the planet's neighbourhood, not just its tint.
    return flyTo(at.clone().add(dir.multiplyScalar(Math.max(12, span() * 0.45))), at, camera.up, dur);
  }

  // Frame several planets at once: look at their middle from the current
  // direction, backed out just far enough that all of them fit on screen.
  function frameAll(ids: number[], dur = 800) {
    if (ids.length === 1) return focusOn(ids[0], dur);
    const points = ids.map((id) => toWorld(planets[id]));
    const box = new THREE.Box3().setFromPoints(points);
    const center = box.getCenter(new THREE.Vector3());
    const radius = Math.max(...points.map((p) => p.distanceTo(center))) + 2 * unit;
    const vfov = THREE.MathUtils.degToRad(camera.fov);
    const hfov = 2 * Math.atan(Math.tan(vfov / 2) * camera.aspect);
    const dist = Math.max(12, (radius / Math.sin(Math.min(vfov, hfov) / 2)) * 1.1);
    const dir = camera.position.clone().sub(controls.target).normalize();
    return flyTo(center.clone().add(dir.multiplyScalar(dist)), center, camera.up, dur);
  }

  // Arrivals: planets where our fleets land at the end of this round.
  let arrivals = $derived([
    ...new Set(fleets.filter((f) => f.owner === seat && f.arrivalRound <= round).map((f) => f.to)),
  ]);
  let arrivalIndex = 0;
  function nextArrival() {
    if (!arrivals.length) return;
    const id = arrivals[arrivalIndex++ % arrivals.length];
    focusOn(id);
    hover = id;
    onhover?.(id);
  }

  // --- Battle replays -----------------------------------------------------

  // While a battle plays, a planet shows its owner from before it.
  let shownOwner = $state(new Map<number, number>());
  let replayed = -1;
  let replaying = $state(false);
  let skip = false;
  const fx = new THREE.Group();
  type Anim = (now: number) => boolean; // returns true when finished
  let anims: Anim[] = [];

  const wait = (ms: number) => new Promise<void>((r) => setTimeout(r, skip ? 0 : ms));

  $effect(() => {
    if (replay && replay.round !== replayed && renderer) {
      replayed = replay.round;
      noteInput();
      runReplay(replay.events);
    }
  });

  async function runReplay(events: GameEvent[]) {
    // Battles only: a fleet landing on its own side's planet is a
    // reinforcement, not a fight. The events come in the order they
    // happened, which matters when one planet changes hands several times in
    // a round.
    const all = events.filter(
      (e) => (e.kind === "captured" || e.kind === "held") && e.planet >= 0 && e.attacker !== e.defender,
    );
    // Show the most important few (ours first, then the biggest), but play
    // them in the order they happened.
    const chosen = new Set(
      [...all].sort((a, b) => Number(involves(b)) - Number(involves(a)) || b.ships - a.ships).slice(0, 6),
    );
    const battles = all.filter((e) => chosen.has(e));
    if (!battles.length) {
      play(...events.map((e) => soundFor(e, seat)).filter((s) => s !== null));
      onreplaydone?.();
      return;
    }
    replaying = true;
    skip = false;
    // Show every planet that changed hands as it was before the round: the
    // defender of its first battle.
    const before = new Map<number, number>();
    for (const e of all) if (!before.has(e.planet)) before.set(e.planet, e.defender);
    shownOwner = before;
    const home = { pos: camera.position.clone(), target: controls.target.clone(), up: camera.up.clone() };
    // One camera move for the whole round: frame every battle, then play
    // them in place, in the order they happened.
    await frameAll([...new Set(battles.map((e) => e.planet))]);
    for (const e of battles) {
      if (skip) break;
      await battle(e);
    }
    lastInput = performance.now(); // a lull starts after the replay
    shownOwner = new Map();
    for (const a of anims) a(Infinity);
    anims = [];
    if (skip) {
      play(...events.map((e) => soundFor(e, seat)).filter((s) => s !== null));
      skip = false;
    }
    await flyTo(home.pos, home.target, home.up, 600);
    replaying = false;
    onreplaydone?.();
  }

  const involves = (e: GameEvent) => e.attacker === seat || e.defender === seat || seat === -2;

  // One battle: the attackers converge on the planet, the exchanges flash,
  // then the planet changes hands (or rings, if it held).
  async function battle(e: GameEvent) {
    const at = toWorld(planets[e.planet]);
    // The planet shows whoever held it when this fleet arrived.
    shownOwner = new Map(shownOwner).set(e.planet, e.defender);
    const attackColor = new THREE.Color(ownerHex(e.attacker));
    const ships = Math.min(10, Math.max(2, Math.round(Math.sqrt(e.ships))));
    const cone = new THREE.ConeGeometry(0.1 * unit, 0.3 * unit, 10);
    const mat = new THREE.MeshBasicMaterial({ color: attackColor });
    const start = performance.now();
    for (let i = 0; i < ships; i++) {
      const from = at.clone().add(new THREE.Vector3().randomDirection().multiplyScalar(4 * unit));
      const m = new THREE.Mesh(cone, mat);
      m.quaternion.setFromUnitVectors(new THREE.Vector3(0, 1, 0), at.clone().sub(from).normalize());
      fx.add(m);
      const delay = i * 40;
      anims.push((now) => {
        const t = Math.min(1, Math.max(0, (now - start - delay) / 700));
        m.position.lerpVectors(from, at, t * 0.8);
        if (t >= 1 || now === Infinity) {
          fx.remove(m);
          return true;
        }
        return false;
      });
    }
    await wait(650);

    // Exchanges: a flash for each burst of losses, near the planet.
    const flashes = Math.min(12, Math.max(3, Math.round(Math.sqrt(e.attackerLost + e.defenderLost) * 2)));
    for (let i = 0; i < flashes && !skip; i++) {
      flash(at.clone().add(new THREE.Vector3().randomDirection().multiplyScalar(0.6 * unit)), i % 2 ? 0xffd27a : 0xffffff);
      play(i % 3 === 2 ? "boom" : "shot");
      await wait(110);
    }

    // Outcome.
    if (e.kind === "captured") shownOwner = new Map(shownOwner).set(e.planet, e.attacker);
    ring(at, new THREE.Color(ownerHex(e.kind === "captured" ? e.attacker : e.defender)));
    const s = soundFor(e, seat);
    if (s) play(s);
    await wait(700);
    cone.dispose();
    mat.dispose();
  }

  function flash(at: THREE.Vector3, color: number) {
    const m = new THREE.Mesh(sphere, new THREE.MeshBasicMaterial({ color, transparent: true }));
    m.position.copy(at);
    fx.add(m);
    const start = performance.now();
    anims.push((now) => {
      const t = Math.min(1, (now - start) / 300);
      m.scale.setScalar((0.1 + t * 0.35) * unit);
      (m.material as THREE.MeshBasicMaterial).opacity = 1 - t;
      if (t >= 1) {
        fx.remove(m);
        (m.material as THREE.Material).dispose();
        return true;
      }
      return false;
    });
  }

  function ring(at: THREE.Vector3, color: THREE.Color) {
    const geo = new THREE.TorusGeometry(1, 0.04, 8, 48);
    const m = new THREE.Mesh(geo, new THREE.MeshBasicMaterial({ color, transparent: true }));
    m.position.copy(at);
    m.lookAt(camera.position);
    fx.add(m);
    const start = performance.now();
    anims.push((now) => {
      const t = Math.min(1, (now - start) / 800);
      m.scale.setScalar((0.4 + t * 1.6) * unit);
      (m.material as THREE.MeshBasicMaterial).opacity = 1 - t;
      if (t >= 1) {
        fx.remove(m);
        geo.dispose();
        (m.material as THREE.Material).dispose();
        return true;
      }
      return false;
    });
  }

  function skipReplay() {
    if (replaying) skip = true;
  }

  // --- Idle drift ---------------------------------------------------------

  // In a lull (no input, no replay, no order being entered) the camera backs
  // out to show the whole map and slowly turns it. Any input stops it where
  // it is.
  const idleAfterMs = 10_000;
  const driftRadPerSec = 0.12;
  let lastInput = performance.now();
  let idling = false;
  const yAxis = new THREE.Vector3(0, 1, 0);

  function noteInput() {
    lastInput = performance.now();
    if (idling) {
      idling = false;
      move?.done();
      move = null;
    }
  }

  function stepIdle(now: number, dt: number) {
    const lull = !replaying && source === null && dest === null && now - lastInput > idleAfterMs;
    if (!lull) return false;
    if (!idling) {
      idling = true;
      // Back out to an overview, level, looking at the centre.
      const dir = camera.position.clone().sub(controls.target);
      dir.y = Math.max(dir.y, 0.35 * dir.length());
      flyTo(dir.normalize().multiplyScalar(span() * 2.1), new THREE.Vector3(), yAxis, 2500);
      return true;
    }
    if (move) return true; // still backing out
    camera.position.applyAxisAngle(yAxis, driftRadPerSec * dt);
    camera.up.copy(yAxis);
    camera.lookAt(controls.target);
    return true;
  }

  // Picking: a click is a press and release without much dragging, so
  // orbiting the camera never picks a planet by accident.
  const raycaster = new THREE.Raycaster();
  function planetAt(e: PointerEvent): number | null {
    const r = renderer.domElement.getBoundingClientRect();
    const ndc = new THREE.Vector2(((e.clientX - r.left) / r.width) * 2 - 1, -((e.clientY - r.top) / r.height) * 2 + 1);
    raycaster.setFromCamera(ndc, camera);
    const hit = raycaster.intersectObjects(pickables)[0];
    return hit ? (hit.object.userData.id as number) : null;
  }

  onMount(() => {
    try {
      renderer = new THREE.WebGLRenderer({ antialias: true });
    } catch {
      webgl = false;
      return;
    }
    renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
    renderer.setClearColor(0x0b0e17);
    host.appendChild(renderer.domElement);
    labels = new CSS2DRenderer();
    labels.domElement.className = "labels";
    host.appendChild(labels.domElement);

    scene = new THREE.Scene();
    scene.add(new THREE.AmbientLight(0xffffff, 1.2));
    const sun = new THREE.DirectionalLight(0xffffff, 2);
    sun.position.set(5, 10, 7);
    scene.add(sun);
    dynamic = new THREE.Group();
    scene.add(dynamic);
    scene.add(fx);
    buildFrame();

    const span = Math.max(width, height, depth * layerGap);
    camera = new THREE.PerspectiveCamera(45, 1, 0.1, span * 10);
    // Free rotation: the map turns and flips like a model in the hand.
    controls = new TrackballControls(camera, renderer.domElement);
    controls.rotateSpeed = 3;
    controls.zoomSpeed = 1.2;
    controls.panSpeed = 0.6;
    controls.dynamicDampingFactor = 0.15;
    controls.maxDistance = span * 3;
    controls.minDistance = 2;
    resetView = () => {
      camera.up.set(0, 1, 0);
      camera.position.set(span * 0.7, span * 0.9, span * 1.75);
      controls.target.set(0, 0, 0);
      camera.lookAt(0, 0, 0);
    };
    resetView();
    rebuild();

    let down: { x: number; y: number } | null = null;
    const el = renderer.domElement;
    const onDown = (e: PointerEvent) => {
      noteInput();
      skipReplay();
      down = { x: e.clientX, y: e.clientY };
    };
    const onKey = (e: KeyboardEvent) => {
      noteInput();
      if (e.key === "Escape") skipReplay();
    };
    window.addEventListener("keydown", onKey);
    const onWheel = () => noteInput();
    el.addEventListener("wheel", onWheel, { passive: true });
    const onUp = (e: PointerEvent) => {
      if (down && Math.hypot(e.clientX - down.x, e.clientY - down.y) < 6) {
        const id = planetAt(e);
        if (id !== null) onpick?.(id);
      }
      down = null;
    };
    const onMove = (e: PointerEvent) => {
      noteInput();
      if (down) return;
      const id = planetAt(e);
      if (id !== hover) {
        hover = id;
        onhover?.(id);
      }
      el.style.cursor = id === null ? "grab" : "pointer";
    };
    const onLeave = () => {
      hover = null;
      onhover?.(null);
    };
    el.addEventListener("pointerdown", onDown);
    el.addEventListener("pointerup", onUp);
    el.addEventListener("pointermove", onMove);
    el.addEventListener("pointerleave", onLeave);

    const resize = () => {
      const w = host.clientWidth;
      const h = host.clientHeight;
      if (!w || !h) return;
      renderer.setSize(w, h);
      labels.setSize(w, h);
      camera.aspect = w / h;
      camera.updateProjectionMatrix();
      controls.handleResize();
    };
    const ro = new ResizeObserver(resize);
    ro.observe(host);
    resize();

    let frame = 0;
    let prev = performance.now();
    const loop = () => {
      frame = requestAnimationFrame(loop);
      const now = performance.now();
      const dt = Math.min(0.1, (now - prev) / 1000);
      prev = now;
      const drifting = stepIdle(now, dt);
      if (move) stepMove(now);
      else if (!drifting) controls.update();
      anims = anims.filter((a) => !a(now));
      for (const r of rings) r.lookAt(camera.position);
      renderer.render(scene, camera);
      labels.render(scene, camera);
    };
    loop();

    return () => {
      window.removeEventListener("keydown", onKey);
      el.removeEventListener("wheel", onWheel);
      cancelAnimationFrame(frame);
      ro.disconnect();
      controls.dispose();
      dispose(scene);
      renderer.dispose();
      sphere.dispose();
      floorDot.dispose();
      box.dispose();
      host.replaceChildren();
    };
  });
</script>

<div class="board3d" bind:this={host} data-testid="board-3d">
  {#if webgl}
    <div class="views" role="toolbar" aria-label="Camera">
      {#each Object.keys(presets) as name (name)}
        <button class="small" onclick={() => viewFrom(name)} title="View the map from the {name.toLowerCase()}">{name}</button>
      {/each}
      <button
        class="small"
        onclick={nextArrival}
        disabled={!arrivals.length}
        title="Focus on planets where your fleets arrive at the end of this round"
      >
        Arrivals{arrivals.length ? ` (${arrivals.length})` : ""}
      </button>
      <button class="small" onclick={() => resetView()} title="Put the camera back where it started">Reset view</button>
    </div>
    {#if replaying}
      <p class="replaying">Replaying battles · click or Esc to skip</p>
    {/if}
    <p class="hint">Drag to turn · scroll to zoom · right-drag to pan</p>
  {/if}
  {#if !webgl}
    <p class="muted">This browser can't show the 3D view. Switch back to the map view.</p>
  {/if}
  <!-- Planets as buttons for keyboard and screen-reader users. -->
  <ul class="sr-only" aria-label="Planets">
    {#each planets as p (p.id)}
      <li><button class:mine={p.owner === seat} aria-label="Planet {p.name}" onclick={() => onpick?.(p.id)}></button></li>
    {/each}
  </ul>
</div>

<style>
  .board3d {
    position: relative;
    width: 100%;
    height: 100%;
    overflow: hidden;
    border-radius: 8px;
  }
  .board3d :global(canvas) {
    display: block;
    touch-action: none;
  }
  .board3d :global(.labels) {
    position: absolute;
    inset: 0;
    pointer-events: none;
  }
  .board3d :global(.anchor) {
    width: 0;
    height: 0;
  }
  .board3d :global(.tag) {
    position: absolute;
    left: 9px;
    top: -14px;
    font-size: 11px;
    font-weight: 600;
    color: var(--text);
    text-shadow: 0 0 3px var(--bg), 0 0 3px var(--bg);
    white-space: nowrap;
  }
  .board3d :global(.tag.mine) {
    color: var(--accent);
  }
  .board3d :global(.tag.aim) {
    font-weight: 400;
  }
  .views {
    position: absolute;
    top: 8px;
    right: 8px;
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: 4px;
    max-width: 70%;
    opacity: 0.9;
  }
  .replaying {
    position: absolute;
    bottom: 28px;
    left: 50%;
    transform: translateX(-50%);
    margin: 0;
    padding: 4px 10px;
    border-radius: 6px;
    background: rgb(19 24 40 / 0.85);
    font-size: 13px;
    pointer-events: none;
  }
  .hint {
    position: absolute;
    bottom: 6px;
    right: 10px;
    margin: 0;
    font-size: 11px;
    color: var(--muted);
    pointer-events: none;
  }
  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip-path: inset(50%);
    margin: 0;
    padding: 0;
  }
</style>
