<script lang="ts">
  import { onMount } from "svelte";
  import * as THREE from "three";
  import { OrbitControls } from "three/examples/jsm/controls/OrbitControls.js";
  import { CSS2DObject, CSS2DRenderer } from "three/examples/jsm/renderers/CSS2DRenderer.js";
  import type { Fleet, PlanetView } from "./proto";
  import { travelTurns } from "./proto";
  import { ownerHex } from "./colors";

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

  // Layers sit further apart than cells so height reads clearly.
  const layerGap = 1.5;

  let host: HTMLDivElement;
  let hover = $state<number | null>(null);

  let scene: THREE.Scene;
  let camera: THREE.PerspectiveCamera;
  let renderer: THREE.WebGLRenderer;
  let labels: CSS2DRenderer;
  let controls: OrbitControls;
  let dynamic: THREE.Group; // rebuilt whenever the view changes
  let pickables: THREE.Mesh[] = [];
  let rings: THREE.Mesh[] = []; // selection rings turn to face the camera
  let webgl = $state(true);

  const sphere = new THREE.SphereGeometry(1, 24, 16);
  const floorDot = new THREE.CircleGeometry(0.12, 16).rotateX(-Math.PI / 2);

  function toWorld(p: PlanetView): THREE.Vector3 {
    return new THREE.Vector3(
      p.pos.x + 0.5 - width / 2,
      p.pos.z * layerGap - ((depth - 1) * layerGap) / 2,
      p.pos.y + 0.5 - height / 2,
    );
  }
  const floorY = () => -((depth - 1) * layerGap) / 2 - 0.6;
  const radius = (p: PlanetView) => 0.26 + Math.min(1, (p.ships ?? 10) / 200) * 0.12;

  function label(text: string, cls: string): CSS2DObject {
    const el = document.createElement("div");
    el.className = cls;
    el.textContent = text;
    return new CSS2DObject(el);
  }

  function dispose(obj: THREE.Object3D) {
    obj.traverse((o) => {
      if (o instanceof CSS2DObject) o.element.remove();
      const m = (o as THREE.Mesh).material as THREE.Material | THREE.Material[] | undefined;
      if (m) (Array.isArray(m) ? m : [m]).forEach((x) => x.dispose());
      const g = (o as THREE.Mesh).geometry as THREE.BufferGeometry | undefined;
      if (g && g !== sphere && g !== floorDot) g.dispose();
    });
  }

  // The static frame: floor grid and the outline of the playing volume.
  function buildFrame() {
    const size = Math.max(width, height);
    const grid = new THREE.GridHelper(size, size, 0x252c44, 0x1c2236);
    grid.position.y = floorY();
    scene.add(grid);
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

      // Drop line and floor dot: where the planet sits on the map's floor.
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

      const tag = label(p.ships !== undefined ? `${p.name} ${p.ships}` : p.name, p.owner === seat ? "tag mine" : "tag");
      tag.position.copy(at).y += radius(p) + 0.25;
      dynamic.add(tag);
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
      const cone = new THREE.Mesh(new THREE.ConeGeometry(0.1, 0.3, 12), new THREE.MeshBasicMaterial({ color }));
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
      tag.position.lerpVectors(a, b, 0.5).y += 0.3;
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
    camera.position.set(0, span * 1.05, span * 1.25);
    controls = new OrbitControls(camera, renderer.domElement);
    controls.enableDamping = true;
    controls.maxDistance = span * 3;
    controls.minDistance = 2;
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
      host.replaceChildren();
    };
  });
</script>

<div class="board3d" bind:this={host} data-testid="board-3d">
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
  .board3d :global(.tag) {
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
