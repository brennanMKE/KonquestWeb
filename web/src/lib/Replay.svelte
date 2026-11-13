<script lang="ts">
  // Watching saved games back, round by round, with nothing hidden. Several
  // games can play in turn, on a loop, filling the page to leave up.
  import { load, replayFiles, save, store } from "./store.svelte";
  import { unlockAudio } from "./audio";
  import { ownerColor } from "./colors";
  import { SPECTATOR, playerType, type GameEvent } from "./proto";
  import Board from "./Board.svelte";
  import Standings from "./Standings.svelte";
  import SoundToggle from "./SoundToggle.svelte";
  import AudioPanel from "./AudioPanel.svelte";
  import { say } from "./voice";
  import { noteRound, setIntensity } from "./music";

  let { onclose }: { onclose: () => void } = $props();

  let replay = $derived(store.replay!);
  let frames = $derived(replay.frames);
  let last = $derived(frames.length - 1);
  let settings = $derived(replay.settings);
  let games = $derived(store.replays.length);

  let index = $state(0);
  let frame = $derived(frames[Math.min(index, last)]);
  let playing = $state(false);
  const speeds = [0.5, 1, 2, 4];
  let speed = $state(Number(load("parallax.replaySpeed")) || 1);
  $effect(() => save("parallax.replaySpeed", String(speed)));
  // svelte-ignore state_referenced_locally
  let loop = $state(games > 1);

  const chosen = load("parallax.view");
  // svelte-ignore state_referenced_locally
  let view3d = $state(chosen === "3d" || (chosen === "" && settings.depth > 1));
  function toggleView() {
    view3d = !view3d;
    save("parallax.view", view3d ? "3d" : "2d");
  }
  let board3d = $derived(view3d ? import("./Board3D.svelte") : null);

  // Battles play out in 3D, as in the game; the setting is shared with it.
  let animateBattles = $state(load("parallax.replays") !== "off");
  function toggleBattles() {
    animateBattles = !animateBattles;
    save("parallax.replays", animateBattles ? "on" : "off");
    if (!animateBattles) show(index);
  }

  // Battles play when stepping forward one round at a time. While they
  // play, autoplay waits for them.
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
      if (animate && index > 0) announce(frames[index].events);
    }
  }
  function battlesDone(round: number) {
    if (battles && round === battles.round) {
      clearTimeout(fallback);
      animating = false;
      announce(battles.events);
    }
  }
  // The announcer speaks a round's big moments when it's played forward:
  // after its battles, when they're shown.
  function announce(events: GameEvent[]) {
    const v = frames[index]?.view;
    if (v) noteRound(events, v.fleets, SPECTATOR, (id) => v.planets[id]?.owner ?? -1);
    for (const e of events) {
      const line = store.announcement(e, SPECTATOR);
      if (line) say(line);
    }
  }
  $effect(() => () => clearTimeout(fallback));

  // Another game starts from its beginning.
  let current: unknown = null;
  $effect(() => {
    if (replay !== current) {
      current = replay;
      setIntensity(0);
      show(0);
    }
  });

  function togglePlay() {
    unlockAudio();
    if (playing) {
      playing = false;
      return;
    }
    if (index >= last) show(0);
    playing = true;
  }

  // Autoplay: the next round after a pause, once its battles have played.
  // At the end of a game the result stays up a while, then a loop moves on
  // to the next game, or starts this one again.
  const endPauseMs = 8000;
  $effect(() => {
    if (!playing || animating) return;
    if (index < last) {
      const t = setTimeout(() => show(index + 1, true), 1200 / speed);
      return () => clearTimeout(t);
    }
    if (!loop) {
      playing = false;
      return;
    }
    const t = setTimeout(() => {
      if (games > 1) store.selectReplay(store.replayIndex + 1);
      else show(0);
    }, endPauseMs);
    return () => clearTimeout(t);
  });

  function step(d: number) {
    playing = false;
    show(index + d, d === 1);
  }

  function pickGame(i: number) {
    store.selectReplay(i);
  }

  // Full page: only the board, with a caption, to leave up and watch.
  let fullPage = $state(false);
  let fullHint = $state(false);
  function enterFullPage() {
    fullPage = true;
    fullHint = true;
    setTimeout(() => (fullHint = false), 3000);
    if (!playing) togglePlay();
  }

  function onkey(e: KeyboardEvent) {
    const tag = (e.target as HTMLElement)?.tagName;
    if (tag === "INPUT" || tag === "SELECT") return;
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
    } else if (e.key === "Escape" && fullPage && !animating) fullPage = false;
  }

  // Download the game file: the uploaded file, or the room's.
  let fileUrl = $derived(replay.file ? URL.createObjectURL(new Blob([replay.file], { type: "application/json" })) : "");
  $effect(() => {
    const u = fileUrl;
    return () => {
      if (u) URL.revokeObjectURL(u);
    };
  });
  let downloadHref = $derived(fileUrl || replay.download || "");
  let downloadName = $derived(replay.name ?? `parallax-${replay.room || "game"}.json`);

  let adding = $state(false);
  async function addGames(e: Event) {
    const input = e.currentTarget as HTMLInputElement;
    const files = [...(input.files ?? [])];
    input.value = "";
    if (!files.length) return;
    adding = true;
    const { games: added, errors } = await replayFiles(files);
    adding = false;
    if (errors.length) store.showToast(`Couldn't play back ${errors.join("; ")}`);
    if (added.length) {
      store.openReplays(added, true);
      loop = true;
    }
  }

  // What happened in the shown round: fleets sent, then the battles.
  let lines = $derived.by(() => {
    const out: { text: string; big: boolean; color?: string }[] = [];
    const v = frame.view;
    const name = (id: number) => v.planets[id]?.name ?? "?";
    for (const f of frame.launched) {
      out.push({
        text: `${store.playerName(f.owner)} sent ${f.ships} ships from ${name(f.from)} to ${name(f.to)}${f.arrivalRound > frame.round ? `, arriving in round ${f.arrivalRound}` : ""}.`,
        big: false,
        color: ownerColor(f.owner),
      });
    }
    for (const e of frame.events) {
      const d = store.describe(e, SPECTATOR);
      if (d) {
        out.push({
          text: d.text,
          big: e.kind === "captured" || e.kind === "eliminated" || e.kind === "fallen" || e.kind === "risen" || e.kind === "game_over",
          color: e.attacker >= 0 ? ownerColor(e.attacker) : undefined,
        });
      }
    }
    return out;
  });

  let winnerName = $derived(replay.winner >= 0 ? store.playerName(replay.winner) : "");
  let result = $derived(replay.over ? (winnerName ? `${winnerName} wins` : "No winner") : "Saved before the end");
  let seats = $derived(replay.seats.map((s, seat) => ({ ...s, seat, connected: true })));
  let gameLabel = (r: { name?: string; room: string }, i: number) => r.name?.replace(/\.json$/i, "") || (r.room ? `Room ${r.room}` : `Game ${i + 1}`);
</script>

<svelte:window onkeydown={onkey} onpointerdown={unlockAudio} />

<div class="replay" class:full={fullPage}>
  <header>
    <strong>Parallax</strong>
    <span class="muted">Replay{replay.room ? ` of room ${replay.room}` : ""}{games > 1 ? ` · game ${store.replayIndex + 1} of ${games}` : ""}</span>
    <span>
      {#if index === 0}Start{:else}Round {frame.round}{/if}
      <span class="muted">of {frames[last].round}</span>
    </span>
    {#if index === last || !replay.over}
      <span class="result" class:muted={!replay.over}>{result}</span>
    {/if}
    <span class="spacer"></span>
    {#each seats as s (s.seat)}
      <span class="player" class:out={!frame.view.players[s.seat]?.alive} title={playerType(s)}>
        <span class="dot" style:background={ownerColor(s.seat)}></span>{store.playerName(s.seat)}
      </span>
    {/each}
    <SoundToggle />
    <AudioPanel />
    {#if downloadHref}
      <a class="download" href={downloadHref} download={downloadName}>Download game</a>
    {/if}
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
          onreplaydone={battlesDone}
        />
      {/snippet}
      <!-- Each game gets a fresh board: maps differ in size and shape. -->
      {#key replay}
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
      {/key}
      <div class="view-toggle">
        <button class="small" onclick={toggleView} title="Switch between the map and a 3D view you can turn">
          {view3d ? "Map view" : "3D view"}
        </button>
        {#if view3d}
          <button class="small" onclick={toggleBattles} aria-pressed={animateBattles} title="Play out each round's battles in 3D">
            Battles {animateBattles ? "on" : "off"}
          </button>
        {/if}
        {#if fullPage}
          <button class="small" onclick={togglePlay}>{playing ? "Pause" : "Play"}</button>
          <button class="small" onclick={() => (fullPage = false)}>Exit full page</button>
        {:else}
          <button class="small" onclick={enterFullPage} title="Show only the map and play, ready for full screen. Esc to return.">Fill page</button>
        {/if}
      </div>
      {#if fullPage}
        <div class="caption" aria-live="polite">
          <div>
            {#if games > 1}<span class="muted">Game {store.replayIndex + 1} of {games} · </span>{/if}
            {#if index === 0}Start{:else}Round {frame.round} of {frames[last].round}{/if}
          </div>
          {#if index === last}<div class="big">{result}</div>{/if}
          <div class="names">
            {#each seats as s (s.seat)}
              <span class="player" class:out={!frame.view.players[s.seat]?.alive}>
                <span class="dot" style:background={ownerColor(s.seat)}></span>{store.playerName(s.seat)}
                <span class="muted">{frame.standings.find((x) => x.seat === s.seat)?.planets ?? 0}</span>
              </span>
            {/each}
          </div>
        </div>
        {#if fullHint}<div class="full-hint">Press Esc to return</div>{/if}
      {/if}
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
        <div class="options">
          <label>
            Speed
            <select bind:value={speed}>
              {#each speeds as s (s)}<option value={s}>{s}×</option>{/each}
            </select>
          </label>
          <label><input type="checkbox" bind:checked={loop} /> Loop</label>
        </div>
        {#if view3d && animateBattles}<p class="hint">Battles play out in 3D when you step forward or play. Click or Esc skips them.</p>{/if}
      </section>

      <section class="games" aria-label="Games">
        {#if games > 1}
          <ol>
            {#each store.replays as r, i (r)}
              <li>
                <button class:on={i === store.replayIndex} onclick={() => pickGame(i)}>
                  {gameLabel(r, i)} <span class="muted">· {r.frames[r.frames.length - 1].round} rounds</span>
                </button>
              </li>
            {/each}
          </ol>
        {/if}
        <label class="add">
          <input type="file" accept=".json,application/json" multiple onchange={addGames} disabled={adding} />
          <span>{adding ? "Adding…" : "Add saved games…"}</span>
        </label>
      </section>

      <section class="standings">
        <Standings view={frame.view} {seats} />
      </section>

      <section class="log" aria-label="What happened">
        <h3>{index === 0 ? "The start" : `Round ${frame.round}`}</h3>
        {#each lines as l, i (i)}
          <p class:big={l.big}>
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
  .result.muted {
    color: var(--muted);
    font-weight: 400;
  }
  .download {
    color: var(--accent);
    font-size: 13px;
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
  .options {
    display: flex;
    gap: 16px;
    align-items: center;
    font-size: 13px;
    color: var(--muted);
  }
  .options label {
    display: flex;
    align-items: center;
    gap: 6px;
  }
  .hint {
    font-size: 12px;
    color: var(--muted);
    margin: 8px 0 0;
  }
  .games ol {
    list-style: none;
    margin: 0 0 8px;
    padding: 0;
    max-height: 160px;
    overflow-y: auto;
  }
  .games li button {
    width: 100%;
    text-align: left;
    padding: 3px 8px;
    font-size: 13px;
    border-color: transparent;
    background: none;
  }
  .games li button.on {
    border-color: var(--accent);
    color: var(--accent);
  }
  .add {
    position: relative;
    display: inline-block;
    font-size: 13px;
    color: var(--accent);
    cursor: pointer;
  }
  .add input {
    position: absolute;
    inset: 0;
    opacity: 0;
    cursor: pointer;
  }
  .add:focus-within span {
    outline: 2px solid var(--accent);
    outline-offset: 2px;
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
  /* Full page: the board edge to edge, with a caption. */
  .replay.full header,
  .replay.full aside {
    display: none;
  }
  .replay.full .board-wrap {
    padding: 0;
  }
  .replay.full .view-toggle {
    top: 8px;
    left: 8px;
    opacity: 0;
    transition: opacity 0.3s;
  }
  .replay.full .board-wrap:hover .view-toggle,
  .replay.full .view-toggle:focus-within {
    opacity: 0.85;
  }
  .caption {
    position: absolute;
    z-index: 1;
    left: 16px;
    bottom: 16px;
    padding: 8px 12px;
    border-radius: 8px;
    background: rgb(0 0 0 / 0.55);
    font-size: 14px;
    pointer-events: none;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .caption .big {
    font-size: 22px;
    font-weight: 700;
    color: var(--accent);
  }
  .caption .names {
    display: flex;
    flex-wrap: wrap;
    gap: 4px 12px;
  }
  .full-hint {
    position: absolute;
    z-index: 2;
    top: 16px;
    left: 50%;
    transform: translateX(-50%);
    background: rgb(0 0 0 / 0.6);
    padding: 6px 14px;
    border-radius: 6px;
    font-size: 14px;
    pointer-events: none;
  }
</style>
