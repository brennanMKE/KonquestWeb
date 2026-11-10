<script lang="ts">
  // Watching a whole game back, round by round, with nothing hidden.
  import { load, save, store } from "./store.svelte";
  import { unlockAudio } from "./audio";
  import { ownerColor } from "./colors";
  import { SPECTATOR, playerType, type GameEvent } from "./proto";
  import Board from "./Board.svelte";
  import Standings from "./Standings.svelte";

  let { onclose }: { onclose: () => void } = $props();

  let replay = $derived(store.replay!);
  let frames = $derived(replay.frames);
  let last = $derived(frames.length - 1);
  let settings = $derived(replay.settings);

  let index = $state(0);
  let frame = $derived(frames[index]);
  let playing = $state(false);
  const speeds = [0.5, 1, 2, 4];
  let speed = $state(Number(load("parallax.replaySpeed")) || 1);
  $effect(() => save("parallax.replaySpeed", String(speed)));

  const chosen = load("parallax.view");
  // svelte-ignore state_referenced_locally
  let view3d = $state(chosen === "3d" || (chosen === "" && settings.depth > 1));
  function toggleView() {
    view3d = !view3d;
    save("parallax.view", view3d ? "3d" : "2d");
  }
  let board3d = $derived(view3d ? import("./Board3D.svelte") : null);

  // Battle animations, shared with the game's Replays setting.
  let animateBattles = $state(load("parallax.replays") !== "off");
  function toggleBattles() {
    animateBattles = !animateBattles;
    save("parallax.replays", animateBattles ? "on" : "off");
    if (!animateBattles) show(index);
  }

  // Battles animate in 3D when stepping forward one round at a time. While
  // they play, autoplay waits for them.
  let battles = $state<{ round: number; events: GameEvent[] } | null>(null);
  let animating = $state(false);
  let fallback = 0;

  function show(n: number, animate = false) {
    index = Math.max(0, Math.min(last, n));
    clearTimeout(fallback);
    if (animate && view3d && animateBattles && index > 0) {
      battles = { round: frames[index].round, events: frames[index].events };
      animating = true;
      // Never wait forever, say if the 3D view failed to load.
      fallback = window.setTimeout(() => (animating = false), 30_000);
    } else {
      battles = null;
      animating = false;
    }
  }
  function replayDone(round: number) {
    if (battles && round === battles.round) {
      clearTimeout(fallback);
      animating = false;
    }
  }
  $effect(() => () => clearTimeout(fallback));

  function togglePlay() {
    unlockAudio();
    if (playing) {
      playing = false;
      return;
    }
    if (index >= last) show(0);
    playing = true;
  }

  // Autoplay: the next round after a pause, once any battles have played.
  $effect(() => {
    if (!playing || animating) return;
    if (index >= last) {
      playing = false;
      return;
    }
    const t = setTimeout(() => show(index + 1, true), 1200 / speed);
    return () => clearTimeout(t);
  });

  function step(d: number) {
    playing = false;
    show(index + d, d === 1);
  }

  function onkey(e: KeyboardEvent) {
    if ((e.target as HTMLElement)?.tagName === "INPUT" || (e.target as HTMLElement)?.tagName === "SELECT") return;
    if (e.key === "ArrowRight") step(1);
    else if (e.key === "ArrowLeft") step(-1);
    else if (e.key === " ") {
      e.preventDefault();
      togglePlay();
    } else if (e.key === "Home") {
      playing = false;
      show(0);
    } else if (e.key === "End") {
      playing = false;
      show(last);
    } else return;
  }

  // The log of the shown round: fleets sent, then what happened.
  let lines = $derived.by(() => {
    const out: { text: string; tone: string; color?: string }[] = [];
    const v = frame.view;
    const name = (id: number) => v.planets[id]?.name ?? "?";
    for (const f of frame.launched) {
      out.push({
        text: `${store.playerName(f.owner)} sent ${f.ships} ships from ${name(f.from)} to ${name(f.to)}${f.arrivalRound > frame.round ? `, arriving in round ${f.arrivalRound}` : ""}.`,
        tone: "info",
        color: ownerColor(f.owner),
      });
    }
    for (const e of frame.events) {
      const d = store.describe(e, SPECTATOR);
      if (d) out.push({ text: d.text, tone: e.kind === "captured" || e.kind === "eliminated" || e.kind === "game_over" ? "big" : "info", color: e.attacker >= 0 ? ownerColor(e.attacker) : undefined });
    }
    return out;
  });

  let winnerName = $derived(replay.winner >= 0 ? store.playerName(replay.winner) : "");
  let seats = $derived(replay.seats.map((s, seat) => ({ ...s, seat, connected: true })));
</script>

<svelte:window onkeydown={onkey} onpointerdown={unlockAudio} />

<div class="replay">
  <header>
    <strong>Parallax</strong>
    <span class="muted">Replay{replay.room ? ` of room ${replay.room}` : ""}</span>
    <span>
      {#if index === 0}Start{:else}Round {frame.round}{/if}
      <span class="muted">of {frames[last].round}</span>
    </span>
    {#if replay.over && index === last}
      <span class="result">{winnerName ? `${winnerName} wins` : "No winner"}</span>
    {:else if !replay.over}
      <span class="muted">Saved before the end</span>
    {/if}
    <span class="spacer"></span>
    {#each seats as s (s.seat)}
      <span class="player" class:out={!frame.view.players[s.seat]?.alive} title={playerType(s)}>
        <span class="dot" style:background={ownerColor(s.seat)}></span>{store.playerName(s.seat)}
      </span>
    {/each}
    <button class="small" onclick={onclose}>Close replay</button>
  </header>

  <main>
    <div class="board-wrap">
      {#snippet boardProps(Component: typeof Board)}
        <Component
          planets={frame.view.planets}
          width={settings.width}
          height={settings.height}
          depth={settings.depth}
          shipSpeed={settings.shipSpeed}
          seat={SPECTATOR}
          fleets={frame.view.fleets}
          round={frame.view.round}
          replay={battles}
          onreplaydone={replayDone}
        />
      {/snippet}
      {#if board3d}
        {#await board3d}
          <p class="muted">Loading the 3D view…</p>
        {:then mod}
          {@render boardProps(mod.default)}
        {:catch}
          <p class="error">Couldn't load the 3D view.</p>
        {/await}
      {:else}
        {@render boardProps(Board)}
      {/if}
      <div class="view-toggle">
        <button class="small" onclick={toggleView} title="Switch between the map and a 3D view you can turn">
          {view3d ? "Map view" : "3D view"}
        </button>
        {#if view3d}
          <button class="small" onclick={toggleBattles} aria-pressed={animateBattles} title="Play each round's battles in 3D">
            Battles {animateBattles ? "on" : "off"}
          </button>
        {/if}
      </div>
    </div>

    <aside>
      <section class="transport" aria-label="Replay controls">
        <div class="buttons">
          <button onclick={() => step(-index)} disabled={index === 0} title="Back to the start (Home)" aria-label="Start">⏮</button>
          <button onclick={() => step(-1)} disabled={index === 0} title="Previous round (←)" aria-label="Previous round">◀</button>
          <button class="primary play" onclick={togglePlay} title="Play or pause (Space)">{playing ? "Pause" : index >= last ? "Play again" : "Play"}</button>
          <button onclick={() => step(1)} disabled={index >= last} title="Next round (→)" aria-label="Next round">▶</button>
          <button onclick={() => step(last - index)} disabled={index >= last} title="To the end (End)" aria-label="End">⏭</button>
        </div>
        <input
          type="range"
          min="0"
          max={last}
          value={index}
          oninput={(e) => {
            playing = false;
            show(Number((e.currentTarget as HTMLInputElement).value));
          }}
          aria-label="Round"
        />
        <label class="speed">
          Speed
          <select bind:value={speed}>
            {#each speeds as s (s)}<option value={s}>{s}×</option>{/each}
          </select>
        </label>
        {#if view3d && animateBattles}<p class="hint">Battles play in 3D when you step forward or play. Click or Esc skips them.</p>{/if}
      </section>

      <section class="standings">
        <Standings view={frame.view} {seats} />
      </section>

      <section class="log" aria-label="What happened">
        <h3>{index === 0 ? "The start" : `Round ${frame.round}`}</h3>
        {#each lines as l, i (i)}
          <p class:big={l.tone === "big"}>
            {#if l.color}<span class="dot" style:background={l.color}></span>{/if}{l.text}
          </p>
        {:else}
          <p class="muted">{index === 0 ? "Every player at home. Press Play to watch." : "Nothing happened this round."}</p>
        {/each}
      </section>
    </aside>
  </main>
</div>

<style>
  .replay {
    display: flex;
    flex-direction: column;
    height: 100vh;
  }
  header {
    display: flex;
    align-items: center;
    gap: 16px;
    padding: 8px 16px;
    background: var(--panel);
    border-bottom: 1px solid var(--border);
    flex-wrap: wrap;
  }
  .spacer {
    flex: 1;
  }
  .result {
    color: var(--accent);
    font-weight: 700;
  }
  .player {
    display: flex;
    align-items: center;
    gap: 4px;
    font-size: 13px;
  }
  .player.out {
    text-decoration: line-through;
    opacity: 0.5;
  }
  .dot {
    display: inline-block;
    width: 10px;
    height: 10px;
    border-radius: 50%;
    flex: none;
    margin-right: 6px;
  }
  main {
    flex: 1;
    display: flex;
    min-height: 0;
  }
  .board-wrap {
    flex: 1;
    min-width: 0;
    padding: 16px;
    position: relative;
  }
  .view-toggle {
    position: absolute;
    top: 24px;
    left: 24px;
    display: flex;
    gap: 4px;
    opacity: 0.85;
    z-index: 1;
  }
  aside {
    width: 340px;
    display: flex;
    flex-direction: column;
    border-left: 1px solid var(--border);
    background: var(--panel);
    min-height: 0;
  }
  @media (max-width: 760px) {
    main {
      flex-direction: column;
    }
    aside {
      width: auto;
      border-left: none;
      border-top: 1px solid var(--border);
    }
  }
  section {
    padding: 12px 16px;
    border-bottom: 1px solid var(--border);
  }
  .buttons {
    display: flex;
    gap: 4px;
  }
  .buttons button {
    padding: 4px 8px;
  }
  .buttons .play {
    flex: 1;
  }
  input[type="range"] {
    width: 100%;
    margin: 10px 0 6px;
  }
  .speed {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 13px;
    color: var(--muted);
  }
  .hint {
    font-size: 12px;
    color: var(--muted);
    margin: 8px 0 0;
  }
  .log {
    flex: 1;
    overflow-y: auto;
    font-size: 13px;
    border-bottom: none;
  }
  .log h3 {
    margin: 0 0 8px;
    font-size: 14px;
  }
  .log p {
    margin: 0 0 6px;
    line-height: 1.35;
  }
  .log p.big {
    font-weight: 600;
  }
</style>
