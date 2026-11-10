<script lang="ts">
  // The start page's backdrop: a computer game playing out on the 3D map,
  // round by round, while the camera slowly turns. The server makes a fresh
  // game every minute (GET /api/demo); when one ends, the next begins.
  import { onMount } from "svelte";
  import type { Component } from "svelte";
  import { store } from "./store.svelte";
  import { factionOf, planetName } from "./names";
  import { ownerColor } from "./colors";
  import { SPECTATOR, type GameEvent, type Replay } from "./proto";

  /** The game as the server sent it, for the replay viewer. */
  let raw: Replay | null = null;
  /** The same game with its planets named, as the board shows them. */
  let demo = $state<Replay | null>(null);
  let index = $state(0);
  let battles = $state<{ round: number; events: GameEvent[] } | null>(null);
  let Board = $state<Component<Record<string, unknown>> | null>(null);
  let failed = $state(false);

  const reduced = typeof matchMedia === "function" && matchMedia("(prefers-reduced-motion: reduce)").matches;
  const roundMs = 1400;

  let frame = $derived(demo ? demo.frames[index] : null);
  let names = $derived.by(() => {
    if (!demo) return [];
    // Leaders first, by planets held.
    return [...(frame?.standings ?? [])]
      .filter((s) => s.alive)
      .slice(0, 6)
      .map((s) => ({ seat: s.seat, planets: s.planets, name: factionOf(demo!.room, s.seat)?.short ?? `Player ${s.seat + 1}` }));
  });

  async function next() {
    try {
      const res = await fetch("/api/demo");
      if (!res.ok) throw new Error(String(res.status));
      const r = (await res.json()) as Replay;
      raw = r;
      // Planets are named by their holder's faction, from the demo's own
      // room code, as in a real game.
      demo = {
        ...r,
        frames: r.frames.map((f) => ({
          ...f,
          view: { ...f.view, planets: f.view.planets.map((p) => ({ ...p, letter: p.name, name: planetName(r.room, p.id, p.owner) })) },
        })),
      };
      // Join a few rounds in, once empires have spread and fleets fly.
      index = Math.min(r.frames.length - 1, reduced ? Math.floor(r.frames.length / 3) : 8);
      battles = null;
    } catch {
      failed = true;
    }
  }

  let timer = 0;
  let waiting = false;
  function step() {
    if (!demo || reduced) return;
    if (index >= demo.frames.length - 1) {
      // Hold the final map a moment, then a new game.
      timer = window.setTimeout(next, 4000);
      return;
    }
    index++;
    const f = demo.frames[index];
    battles = { round: f.round, events: f.events };
    waiting = true;
    // The board calls back when the battles have played; never wait long.
    timer = window.setTimeout(advance, 12_000);
  }
  function advance() {
    if (!waiting) return;
    waiting = false;
    clearTimeout(timer);
    timer = window.setTimeout(step, roundMs);
  }

  $effect(() => {
    if (demo && Board && !reduced) {
      clearTimeout(timer);
      timer = window.setTimeout(step, 2500);
    }
    return () => clearTimeout(timer);
  });

  onMount(() => {
    // Start after the page has painted: three.js is a big download.
    const start = () => {
      next();
      import("./Board3D.svelte")
        .then((m) => (Board = m.default as unknown as Component<Record<string, unknown>>))
        .catch(() => (failed = true));
    };
    const idle = (window as unknown as { requestIdleCallback?: (f: () => void) => number }).requestIdleCallback;
    if (idle) idle(start);
    else setTimeout(start, 200);
  });

  function watch() {
    if (raw) store.openReplays([raw]);
  }
</script>

{#if !failed}
  <div class="backdrop" aria-hidden="true">
    {#if demo && Board && frame}
      <Board
        planets={frame.view.planets}
        width={demo.settings.width}
        height={demo.settings.height}
        depth={demo.settings.depth}
        shipSpeed={demo.settings.shipSpeed}
        seat={SPECTATOR}
        fleets={frame.view.fleets}
        round={frame.view.round}
        replay={battles}
        onreplaydone={advance}
        ambient
      />
    {/if}
  </div>
  <div class="veil"></div>
  {#if demo && frame}
    <aside class="caption">
      <div class="title">
        <span class="live"></span>Live demo · {demo.seats.length} computers ·
        {index === 0 ? "Start" : `Round ${frame.round}`}
      </div>
      <div class="names">
        {#each names as n (n.seat)}
          <span><span class="dot" style:background={ownerColor(n.seat)}></span>{n.name} <span class="muted">{n.planets}</span></span>
        {/each}
      </div>
      <button class="small" onclick={watch} title="Open this game in the replay viewer">Watch this game</button>
    </aside>
  {/if}
{/if}

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    z-index: 0;
  }
  .backdrop :global(.board3d) {
    height: 100%;
  }
  /* Wide screens: the form on the left, the map beside it. */
  @media (min-width: 900px) {
    .backdrop {
      left: min(30vw, 420px);
    }
  }
  /* A dark wash where the form sits, so it reads over the map. */
  .veil {
    position: fixed;
    inset: 0;
    z-index: 0;
    pointer-events: none;
    background: linear-gradient(to bottom, rgb(11 14 23 / 0.5), rgb(11 14 23 / 0) 40%);
  }
  @media (min-width: 900px) {
    .veil {
      background: linear-gradient(to right, rgb(11 14 23 / 0.9) 0, rgb(11 14 23 / 0.6) min(30vw, 420px), rgb(11 14 23 / 0) 55%);
    }
  }
  .caption {
    position: fixed;
    z-index: 2;
    right: 16px;
    bottom: 16px;
    max-width: min(420px, calc(100vw - 32px));
    padding: 10px 12px;
    border-radius: 8px;
    background: rgb(11 14 23 / 0.7);
    border: 1px solid var(--border);
    font-size: 13px;
    display: flex;
    flex-direction: column;
    gap: 6px;
    align-items: flex-start;
  }
  .title {
    display: flex;
    align-items: center;
    gap: 6px;
  }
  .live {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: #e5484d;
    animation: pulse 1.6s ease-in-out infinite;
  }
  @keyframes pulse {
    50% {
      opacity: 0.3;
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .live {
      animation: none;
    }
  }
  .names {
    display: flex;
    flex-wrap: wrap;
    gap: 4px 12px;
  }
  .dot {
    display: inline-block;
    width: 8px;
    height: 8px;
    border-radius: 50%;
    margin-right: 5px;
  }
  @media (max-width: 760px) {
    .names {
      display: none;
    }
  }
</style>
