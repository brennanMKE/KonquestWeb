<script lang="ts">
  import type { Fleet, GameEvent, PlanetView } from "./proto";
  import { travelTurns } from "./proto";
  import { ownerColor } from "./colors";
  import { territory } from "./territory";
  import { bandOf, prodLabel, turnsFrom } from "./range";

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
    /** A planet being browsed in the order bar: highlighted, like a hover. */
    focus?: number | null;
    /** Battle replays are a 3D view feature; the map ignores this. */
    replay?: { round: number; events: GameEvent[] } | null;
    onreplaydone?: (round: number) => void;
    /** Shows rounds from this planet to every other one. */
    rangeFrom?: number | null;
    /** Shows each planet's production beside its ships. */
    stats?: boolean;
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
    focus = null,
    rangeFrom = null,
    stats = false,
  }: Props = $props();

  let reach = $derived(rangeFrom === null ? null : turnsFrom(planets, rangeFrom, shipSpeed));

  let hover = $state<number | null>(null);

  const cx = (p: PlanetView) => p.pos.x + 0.5;
  const cy = (p: PlanetView) => p.pos.y + 0.5;
  // 2.5D: on a deep map, higher layers draw larger and carry a z label.
  const radius = (p: PlanetView) => (depth > 1 ? 0.27 + (0.1 * p.pos.z) / (depth - 1) : 0.32);

  // Territory, seen from above: on a flat map each cell; on a deep map each
  // column, by the nearest planet in the flattened map.
  let regions = $derived(
    territory(
      planets.map((p) => ({ ...p, pos: { ...p.pos, z: 0 } })),
      width,
      height,
      1,
    ),
  );

  // Draw lower layers first so higher planets sit on top.
  let ordered = $derived([...planets].sort((a, b) => a.pos.z - b.pos.z));

  // The line from the chosen source to the destination or hovered planet.
  let hovered = $derived(focus ?? hover);
  let target = $derived(dest ?? (source !== null && hovered !== source ? hovered : null));
  let line = $derived(
    source !== null && target !== null ? { a: planets[source], b: planets[target] } : null,
  );

  function fleetPos(f: Fleet) {
    const a = planets[f.from];
    const b = planets[f.to];
    const turns = f.arrivalRound - f.launchRound + 1;
    const t = Math.min(1, Math.max(0, (round - f.launchRound) / turns));
    const x = cx(a) + (cx(b) - cx(a)) * t;
    const y = cy(a) + (cy(b) - cy(a)) * t;
    const angle = (Math.atan2(cy(b) - cy(a), cx(b) - cx(a)) * 180) / Math.PI;
    return { x, y, angle, a, b };
  }

  function setHover(id: number | null) {
    hover = id;
    onhover?.(id);
  }
</script>

<svg
  class="board"
  viewBox="-0.2 -0.2 {width + 0.4} {height + 0.4}"
  role="application"
  aria-label="Star map"
  onmouseleave={() => setHover(null)}
>
  <defs>
    <pattern id="grid" width="1" height="1" patternUnits="userSpaceOnUse">
      <path d="M 1 0 L 0 0 0 1" fill="none" stroke="var(--grid)" stroke-width="0.02" />
    </pattern>
  </defs>
  <g class="territory">
    {#each regions as b (`${b.x},${b.y}`)}
      <rect x={b.x} y={b.y} width={b.size} height={b.size} fill={ownerColor(b.owner)} />
    {/each}
  </g>
  <rect x="0" y="0" {width} {height} fill="url(#grid)" stroke="var(--grid)" stroke-width="0.04" />

  {#each fleets as f (f.id)}
    {@const fp = fleetPos(f)}
    <line
      x1={cx(fp.a)}
      y1={cy(fp.a)}
      x2={cx(fp.b)}
      y2={cy(fp.b)}
      stroke={ownerColor(f.owner)}
      stroke-width="0.04"
      stroke-dasharray="0.12 0.12"
      opacity="0.5"
    />
    <g transform="translate({fp.x} {fp.y}) rotate({fp.angle})">
      <path d="M 0.18 0 L -0.12 0.1 L -0.12 -0.1 Z" fill={ownerColor(f.owner)} />
    </g>
  {/each}

  {#if line}
    {@const turns = travelTurns(line.a.pos, line.b.pos, shipSpeed)}
    <line
      class="aim"
      x1={cx(line.a)}
      y1={cy(line.a)}
      x2={cx(line.b)}
      y2={cy(line.b)}
    />
    <text class="turns" x={(cx(line.a) + cx(line.b)) / 2} y={(cy(line.a) + cy(line.b)) / 2 - 0.15}>
      {turns} {turns === 1 ? "round" : "rounds"}
    </text>
  {/if}

  {#each ordered as p (p.id)}
    {@const r = radius(p)}
    {@const turns = reach?.get(p.id)}
    {@const band = turns === undefined ? null : bandOf(turns)}
    <g
      class="planet"
      class:mine={p.owner === seat}
      class:far={band === "far"}
      role="button"
      tabindex="-1"
      aria-label="Planet {p.name}"
      onclick={() => onpick?.(p.id)}
      onkeydown={() => {}}
      onmouseenter={() => setHover(p.id)}
    >
      <circle cx={cx(p)} cy={cy(p)} r="0.48" fill="transparent" />
      {#if p.id === focus}
        <circle class="focus" cx={cx(p)} cy={cy(p)} r={r + 0.28} />
      {/if}
      {#if p.id === source || p.id === dest}
        <circle class="ring" class:dest={p.id === dest} cx={cx(p)} cy={cy(p)} r={r + 0.1} />
      {/if}
      <circle cx={cx(p)} cy={cy(p)} {r} fill={ownerColor(p.owner)} class="body" />
      <text class="name" x={cx(p)} y={cy(p)}>{p.letter ?? p.name}</text>
      {#if p.letter}
        <text class="title" x={cx(p)} y={cy(p) - r - 0.14}>{p.name}</text>
      {/if}
      {#if band !== null && band !== "far"}
        <circle class="band b{band}" cx={cx(p)} cy={cy(p)} r={r + 0.17} />
      {/if}
      {#if p.ships !== undefined || stats}
        <text class="ships" x={cx(p)} y={cy(p) + r + (band !== null && band !== "far" ? 0.34 : 0.2)}
          >{p.ships ?? ""}{#if stats}<tspan class="prod" dx={p.ships === undefined ? 0 : 0.1}>{prodLabel(p)}</tspan>{/if}</text
        >
      {/if}
      {#if turns !== undefined && band !== null}
        <g class="reach" data-turns={turns} data-band={band}>
          <rect x={cx(p) - r - 0.56} y={cy(p) - 0.15} width="0.36" height="0.3" rx="0.09" class="b{band}" />
          <text x={cx(p) - r - 0.38} y={cy(p) + 0.005}>{turns}</text>
        </g>
      {/if}
      {#if depth > 1}
        <text class="z" x={cx(p) + r * 0.8} y={cy(p) - r * 0.8}>z{p.pos.z}</text>
      {/if}
    </g>
  {/each}
</svg>

<style>
  .board {
    width: 100%;
    height: 100%;
    display: block;
    user-select: none;
  }
  .territory rect {
    opacity: 0.13;
    transition: fill 0.6s ease;
  }
  .planet {
    cursor: pointer;
    outline: none;
  }
  .body {
    stroke: rgb(0 0 0 / 0.5);
    stroke-width: 0.03;
  }
  .planet:hover .body {
    stroke: var(--text);
    stroke-width: 0.05;
  }
  .ring {
    fill: none;
    stroke: var(--text);
    stroke-width: 0.06;
  }
  .focus {
    fill: none;
    stroke: #7cc4ff;
    stroke-width: 0.1;
  }
  .ring.dest {
    stroke-dasharray: 0.1 0.08;
  }
  .name {
    font-size: 0.32px;
    font-weight: 700;
    text-anchor: middle;
    dominant-baseline: central;
    fill: #0b0e17;
    pointer-events: none;
  }
  .title {
    font-size: 0.2px;
    text-anchor: middle;
    fill: var(--muted);
    paint-order: stroke;
    stroke: var(--bg);
    stroke-width: 0.06px;
    pointer-events: none;
  }
  .ships {
    font-size: 0.24px;
    text-anchor: middle;
    dominant-baseline: central;
    fill: var(--text);
    pointer-events: none;
  }
  .prod {
    fill: var(--muted);
  }
  .planet.far {
    opacity: 0.4;
  }
  .band {
    fill: none;
    stroke: var(--text);
    stroke-width: 0.035;
    pointer-events: none;
  }
  .band.b3 {
    stroke-width: 0.06;
  }
  .band.b5 {
    stroke-dasharray: 0.12 0.07;
  }
  .band.b7 {
    stroke-dasharray: 0.03 0.07;
    stroke-linecap: round;
  }
  .reach {
    pointer-events: none;
  }
  .reach rect {
    fill: var(--bg);
    stroke: var(--text);
    stroke-width: 0.025;
  }
  .reach rect.b3 {
    stroke-width: 0.045;
  }
  .reach rect.b5 {
    stroke-dasharray: 0.08 0.05;
  }
  .reach rect.b7 {
    stroke-dasharray: 0.025 0.05;
  }
  .reach rect.bfar {
    stroke: none;
  }
  .reach text {
    font-size: 0.2px;
    font-weight: 700;
    text-anchor: middle;
    dominant-baseline: central;
    fill: var(--text);
  }
  .z {
    font-size: 0.19px;
    fill: var(--muted);
    paint-order: stroke;
    stroke: var(--bg);
    stroke-width: 0.06px;
    pointer-events: none;
  }
  .aim {
    stroke: var(--text);
    stroke-width: 0.04;
    stroke-dasharray: 0.15 0.1;
    pointer-events: none;
  }
  .turns {
    font-size: 0.26px;
    text-anchor: middle;
    fill: var(--text);
    paint-order: stroke;
    stroke: var(--bg);
    stroke-width: 0.08px;
    pointer-events: none;
  }
</style>
