<script lang="ts">
  import { load, save, store } from "./store.svelte";
  import { unlockAudio } from "./audio";
  import SoundToggle from "./SoundToggle.svelte";
  import AudioPanel from "./AudioPanel.svelte";
  import { ownerColor } from "./colors";
  import { playerType } from "./proto";
  import { orders } from "./orders.svelte";
  import { popOut, type Popout } from "./popout";
  import Board from "./Board.svelte";
  import Controls from "./Controls.svelte";
  import OrderBar from "./OrderBar.svelte";

  let view = $derived(store.view!);
  let settings = $derived(view.settings);

  // A new game starts with no half-entered orders.
  orders.cancel();
  orders.measuring = false;
  orders.hover = null;

  // The 3D view loads three.js on first use; the choice is remembered.
  // Deep maps open in 3D unless this browser chose the map view.
  const chosen = load("parallax.view");
  // svelte-ignore state_referenced_locally
  let view3d = $state(chosen === "3d" || (chosen === "" && view.settings.depth > 1));
  function toggleView() {
    view3d = !view3d;
    save("parallax.view", view3d ? "3d" : "2d");
  }
  let board3d = $derived(view3d ? import("./Board3D.svelte") : null);

  // Battles play out in the 3D view as each round ends, unless turned off.
  let replays = $state(load("parallax.replays") !== "off");
  function toggleReplays() {
    replays = !replays;
    save("parallax.replays", replays ? "on" : "off");
  }
  $effect(() => {
    store.playsBattles = view3d && replays;
    // Leaving the 3D view (or turning replays off) ends any wait for one.
    if (!store.playsBattles && store.replayPending) store.replayDone();
  });

  // Full page: the board alone fills the window, ready for the system's full
  // screen. Esc returns to the usual layout.
  let fullPage = $state(false);
  let fullHint = $state(false);
  function enterFullPage() {
    fullPage = true;
    fullHint = true;
    setTimeout(() => (fullHint = false), 3000);
  }

  // The controls and log can move to a window of their own.
  let popout = $state<Popout | null>(null);
  function togglePopout() {
    if (popout) {
      popout.close();
      return;
    }
    popout = popOut(store.room, () => (popout = null));
    if (!popout) store.showToast("The browser blocked the window. Allow pop-ups for this site and try again.");
  }
  $effect(() => () => popout?.close());

  // Turn timer countdown.
  let now = $state(Date.now());
  $effect(() => {
    const t = setInterval(() => (now = Date.now()), 250);
    return () => clearInterval(t);
  });
  let secondsLeft = $derived(store.deadline ? Math.max(0, Math.ceil((store.deadline - now) / 1000)) : null);

  // A finished turn or a new view drops any half-entered order.
  $effect(() => {
    if (!store.myTurn) orders.cancel();
  });

  // Keyboard: planet letters pick planets. Esc skips the battles (the 3D
  // board handles that), else cancels an order, else leaves the full page.
  function onkey(e: KeyboardEvent) {
    unlockAudio();
    if (e.key === "Escape" && store.replayPending) return;
    if (bar?.key(e)) return;
    if (orders.key(e)) return;
    if (e.key === "Escape" && fullPage) fullPage = false;
  }

  let activeName = $derived(store.active >= 0 ? store.playerName(store.active) : "");
  let bar = $state<OrderBar | null>(null);
</script>

<svelte:window onkeydown={onkey} onpointerdown={unlockAudio} />

<div class="game" class:full={fullPage}>
  <header>
    <strong>Parallax</strong>
    <span class="muted">Room {store.room}</span>
    <span>Round {store.round}{settings.maxRounds ? ` of ${settings.maxRounds}` : ""}</span>
    <span class="turn" class:mine={store.myTurn}>
      {#if store.phase === "over"}
        Game over
      {:else if store.myTurn}
        Your turn
      {:else}
        <span class="dot" style:background={ownerColor(store.active)}></span>
        {activeName} is moving
      {/if}
    </span>
    {#if secondsLeft !== null && store.phase === "playing"}
      <span class="timer" class:low={secondsLeft <= 10}>{Math.floor(secondsLeft / 60)}:{String(secondsLeft % 60).padStart(2, "0")}</span>
    {/if}
    <span class="spacer"></span>
    <SoundToggle />
    <AudioPanel />
    {#if store.isHost || store.phase === "over"}
      <a class="save" href="/api/rooms/{store.room}/save?token={store.token}" download>Save game</a>
    {/if}
    {#if store.conn !== "open"}<span class="badge warn">Reconnecting…</span>{/if}
    {#each store.seats as s (s.seat)}
      <span class="player" class:out={!view.players[s.seat]?.alive} class:away={!s.connected} title={store.faction(s.seat)?.name ?? s.name}>
        <span class="dot" style:background={ownerColor(s.seat)}></span>{s.seat === store.seat ? "You" : store.playerName(s.seat)}
        {#if store.isHost && !s.connected && s.kind === "human" && store.phase === "playing"}
          <button
            class="small"
            title="Let a computer play this seat"
            onclick={() => store.send({ type: "replace_with_ai", seat: s.seat })}>Hand to computer</button
          >
        {/if}
      </span>
    {/each}
  </header>

  <main>
    <div class="board-wrap">
      {#snippet boardProps(Component: typeof Board)}
        <Component
          planets={view.planets}
          width={settings.width}
          height={settings.height}
          depth={settings.depth}
          shipSpeed={settings.shipSpeed}
          seat={store.seat}
          fleets={view.fleets}
          round={view.round}
          source={orders.measuring ? orders.measureA : orders.source}
          dest={orders.measuring ? orders.measureB : orders.dest}
          onpick={orders.pick}
          onhover={(id) => (orders.hover = id)}
          focus={orders.focus}
          replay={replays ? store.lastEvents : null}
          onreplaydone={(round) => store.replayDone(round)}
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
          <button class="small" onclick={toggleReplays} aria-pressed={replays} title="Play out each round's battles in 3D">
            Battles {replays ? "on" : "off"}
          </button>
        {/if}
        {#if !fullPage}
          <button class="small" onclick={enterFullPage} title="Show only the map, ready for full screen. Esc to return.">Fill page</button>
        {/if}
        <button class="small" onclick={togglePopout} aria-pressed={!!popout} title="Move the controls and log to their own window">
          {popout ? "Put controls back" : "Pop out controls"}
        </button>
      </div>
      <OrderBar bind:this={bar} />
      {#if fullPage && fullHint}
        <div class="full-hint">Press Esc to return</div>
      {/if}
    </div>

    {#if !popout}
      <aside>
        <Controls />
      </aside>
    {:else if !fullPage}
      <aside class="away">
        <p class="muted">The controls are in their own window.</p>
        <button onclick={togglePopout}>Put them back</button>
      </aside>
    {/if}
  </main>

  {#if store.standings && !store.replayPending}
    <div class="over">
      <div class="card">
        <h2>
          {#if store.winner === store.seat}You win!{:else if store.winner >= 0}{store.seatName(store.winner)} wins{:else}No winner{/if}
        </h2>
        <table>
          <thead>
            <tr><th>#</th><th>Player</th><th>Type</th><th>Planets</th><th>Ships</th><th>Built</th><th>Destroyed</th><th>Lost</th><th>Conquered</th></tr>
          </thead>
          <tbody>
            {#each store.standings as s (s.seat)}
              <tr>
                <td>{s.rank}</td>
                <td><span class="dot" style:background={ownerColor(s.seat)}></span> {store.playerName(s.seat)}</td>
                <td class="type">{playerType(store.seats.find((x) => x.seat === s.seat))}</td>
                <td>{s.planets}</td>
                <td>{s.ships}</td>
                <td>{s.stats.shipsBuilt}</td>
                <td>{s.stats.shipsDestroyed}</td>
                <td>{s.stats.shipsLost}</td>
                <td>{s.stats.planetsConquered}</td>
              </tr>
            {/each}
          </tbody>
        </table>
        <div class="actions">
          {#if store.isHost}
            <button class="primary" onclick={() => store.playAgain()} title="Back to the lobby with the same players and a new map">Play again</button>
          {:else}
            <span class="muted">Waiting for the host to start another game.</span>
          {/if}
          <button onclick={() => store.watchReplay(store.room)} title="Watch the whole game back, round by round, with nothing hidden">Watch replay</button>
          <button onclick={() => store.leave()} title="Go to the start page to create or join another room">New room</button>
          <button onclick={() => (store.standings = null)}>Back to the map</button>
        </div>
      </div>
    </div>
  {/if}
</div>

<style>
  .game {
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
  .turn {
    display: flex;
    align-items: center;
    gap: 6px;
    color: var(--muted);
  }
  .turn.mine {
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
  .player.away {
    opacity: 0.6;
    font-style: italic;
  }
  .dot {
    display: inline-block;
    width: 10px;
    height: 10px;
    border-radius: 50%;
    flex: none;
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
  .away {
    padding: 16px;
    gap: 8px;
  }
  .away p {
    margin: 0;
  }
  /* Full page: only the board, edge to edge. */
  .game.full header {
    display: none;
  }
  .game.full aside {
    display: none;
  }
  .game.full .board-wrap {
    padding: 0;
  }
  .game.full .view-toggle {
    top: 8px;
    left: 8px;
    opacity: 0;
    transition: opacity 0.3s;
  }
  .game.full .board-wrap:hover .view-toggle,
  .game.full .view-toggle:focus-within {
    opacity: 0.85;
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
  .save {
    color: var(--accent);
    font-size: 13px;
  }
  .timer {
    font-variant-numeric: tabular-nums;
    font-weight: 600;
  }
  .timer.low {
    color: var(--bad);
  }
  .over {
    position: fixed;
    z-index: 100;
    inset: 0;
    background: rgb(0 0 0 / 0.6);
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 16px;
  }
  .card {
    background: var(--panel);
    border: 1px solid var(--border);
    border-radius: 10px;
    padding: 24px;
    max-width: 100%;
    overflow-x: auto;
  }
  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    align-items: center;
  }
  .card h2 {
    margin-top: 0;
  }
  table {
    border-collapse: collapse;
    margin-bottom: 16px;
    font-size: 14px;
  }
  th,
  td {
    padding: 4px 10px;
    text-align: right;
    white-space: nowrap;
  }
  th:nth-child(2),
  td:nth-child(2),
  th:nth-child(3),
  td:nth-child(3) {
    text-align: left;
  }
  td.type {
    color: var(--muted);
  }
  th {
    color: var(--muted);
    font-weight: 500;
  }
</style>
