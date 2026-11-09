<script lang="ts">
  import { onMount } from "svelte";
  import * as THREE from "three";
  import { TrackballControls } from "three/examples/jsm/controls/TrackballControls.js";
  import { CSS2DObject, CSS2DRenderer } from "three/examples/jsm/renderers/CSS2DRenderer.js";
  import type { Fleet, PlanetView } from "./proto";
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
      const color = new THREE.Color(ownerHex(p.owner));
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
    void [planets, fleets, round, source, dest, hover, seat];
    rebuild();
  });

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
    const onDown = (e: PointerEvent) => (down = { x: e.clientX, y: e.clientY });
    const onUp = (e: PointerEvent) => {
      if (down && Math.hypot(e.clientX - down.x, e.clientY - down.y) < 6) {
        const id = planetAt(e);
        if (id !== null) onpick?.(id);
      }
      down = null;
    };
    const onMove = (e: PointerEvent) => {
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
    const loop = () => {
      frame = requestAnimationFrame(loop);
      controls.update();
      for (const r of rings) r.lookAt(camera.position);
      renderer.render(scene, camera);
      labels.render(scene, camera);
    };
    loop();

    return () => {
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
    <button class="reset small" onclick={() => resetView()} title="Put the camera back where it started">Reset view</button>
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
  .reset {
    position: absolute;
    top: 8px;
    right: 8px;
    opacity: 0.85;
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
