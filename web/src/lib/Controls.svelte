<script lang="ts">
  // The side panel: orders, measuring, planet details and the log. It shows
  // beside the board, or on its own in a pop-out window.
  import { store } from "./store.svelte";
  import { orders } from "./orders.svelte";
  import { ownerColor } from "./colors";
  import { distance, travelTurns } from "./proto";
  import FleetList from "./FleetList.svelte";
  import Standings from "./Standings.svelte";

  let { popped = false, ondock }: { popped?: boolean; ondock?: () => void } = $props();

  let view = $derived(store.view!);
  let settings = $derived(view.settings);
  let srcPlanet = $derived(orders.srcPlanet);
  let dstPlanet = $derived(orders.dstPlanet);
  let infoPlanet = $derived(orders.infoPlanet);
  let activeName = $derived(store.active >= 0 ? store.playerName(store.active) : "");
  let alive = $derived(view.players[store.seat]?.alive ?? false);

  // Turn timer countdown, for the pop-out's own status line.
  let now = $state(Date.now());
  $effect(() => {
    if (!popped) return;
    const t = setInterval(() => (now = Date.now()), 250);
    return () => clearInterval(t);
  });
  let secondsLeft = $derived(store.deadline ? Math.max(0, Math.ceil((store.deadline - now) / 1000)) : null);

  function stat(v: number | undefined, digits = 0) {
    return v === undefined ? "?" : digits ? v.toFixed(digits) : String(v);
  }

  function submit(e: SubmitEvent) {
    e.preventDefault();
    orders.confirm();
  }

  let logEl = $state<HTMLElement | null>(null);
  $effect(() => {
    void store.log.length;
    void orders.tab;
    logEl?.scrollTo({ top: logEl.scrollHeight });
  });
</script>

<div class="controls" class:popped>
  {#if popped}
    <section class="status">
      <div class="line">
        <strong>Round {store.round}{settings.maxRounds ? ` of ${settings.maxRounds}` : ""}</strong>
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
      </div>
      <div class="line">
        <span class="muted">Room {store.room}</span>
        {#if store.conn !== "open"}<span class="badge warn">Reconnecting…</span>{/if}
        <span class="spacer"></span>
        <button class="small" onclick={ondock}>Put back in the game</button>
      </div>
    </section>
  {/if}

  <section class="orders">
    {#if store.phase === "over"}
      <p>The game is over.</p>
      <div class="again">
        {#if store.isHost}
          <button class="primary" onclick={() => store.playAgain()} title="Back to the lobby with the same players and a new map">Play again</button>
        {:else}
          <p class="hint">Waiting for the host to start another game.</p>
        {/if}
        <button onclick={() => store.leave()} title="Go to the start page to create or join another room">New room</button>
      </div>
    {:else if store.watching}
      <p>You're watching. {activeName} is moving.</p>
      <p class="hint">Drag to turn the map. Each round's battles replay in 3D.</p>
    {:else if !alive}
      <p class="muted">You've been eliminated. You can keep watching.</p>
    {:else if !store.myTurn}
      <p class="muted">Waiting for {activeName}. You can look around the map.</p>
    {:else if srcPlanet && dstPlanet}
      <form onsubmit={submit}>
        <p>
          From <b>{srcPlanet.name}</b> to <b>{dstPlanet.name}</b>,
          {travelTurns(srcPlanet.pos, dstPlanet.pos, settings.shipSpeed)} rounds
        </p>
        <div class="row">
          <input
            bind:this={orders.shipInput}
            type="number"
            min="1"
            max={srcPlanet.ships}
            bind:value={orders.ships}
            aria-label="Ships to send"
          />
          <span class="muted">of {srcPlanet.ships}</span>
          <button class="primary">Send</button>
          <button type="button" onclick={orders.cancel}>Cancel</button>
        </div>
        <p class="hint">Enter to send, Esc to cancel. Orders are final.</p>
      </form>
    {:else if srcPlanet}
      <p>Sending from <b>{srcPlanet.name}</b> ({srcPlanet.ships} ships). Pick a destination.</p>
      <p class="hint">Click the planet or type its letter. Esc cancels.</p>
    {:else}
      <p>Pick one of your planets to send a fleet from.</p>
      {#if popped}<p class="hint">Click planets on the map, or type their letters here.</p>{/if}
    {/if}
    {#if store.myTurn}
      <button class="primary end" onclick={orders.endTurn}>End turn</button>
    {/if}
    <button class="measure" class:on={orders.measuring} onclick={orders.toggleMeasure}>
      {orders.measuring ? "Stop measuring" : "Measure distance"}
    </button>
  </section>

  {#if orders.measuring}
    <section class="measuring">
      {#if orders.measureA !== null && orders.measureB !== null}
        {@const a = view.planets[orders.measureA]}
        {@const b = view.planets[orders.measureB]}
        <p>
          <b>{a.name}</b> to <b>{b.name}</b>: {distance(a.pos, b.pos).toFixed(1)} cells,
          <b>{travelTurns(a.pos, b.pos, settings.shipSpeed)}</b> rounds
        </p>
      {:else if orders.measureA !== null}
        <p>From <b>{view.planets[orders.measureA].name}</b>. Pick a second planet.</p>
      {:else}
        <p>Pick two planets to measure.</p>
      {/if}
    </section>
  {/if}

  <section class="info">
    {#if infoPlanet}
      <h3>
        <span class="dot" style:background={ownerColor(infoPlanet.owner)}></span>
        {infoPlanet.name}
        {#if infoPlanet.letter}<span class="key" title="Type this letter to pick it">{infoPlanet.letter}</span>{/if}
      </h3>
      <dl>
        <dt>Owner</dt>
        <dd>{store.seatName(infoPlanet.owner)}</dd>
        <dt>Ships</dt>
        <dd>{stat(infoPlanet.ships)}</dd>
        <dt>Production</dt>
        <dd>{stat(infoPlanet.production)}</dd>
        <dt>Kill rate</dt>
        <dd>{stat(infoPlanet.kill, 2)}</dd>
        <dt>Position</dt>
        <dd>
          {infoPlanet.pos.x}, {infoPlanet.pos.y}{settings.depth > 1 ? `, ${infoPlanet.pos.z}` : ""}
        </dd>
        {#if srcPlanet && infoPlanet.id !== srcPlanet.id}
          <dt>From {srcPlanet.name}</dt>
          <dd>
            {distance(srcPlanet.pos, infoPlanet.pos).toFixed(1)} cells,
            {travelTurns(srcPlanet.pos, infoPlanet.pos, settings.shipSpeed)} rounds
          </dd>
        {/if}
      </dl>
    {:else}
      <p class="muted">Hover a planet for details.</p>
    {/if}
  </section>

  <nav class="tabs" aria-label="Panels">
    <button class:on={orders.tab === "log"} onclick={() => (orders.tab = "log")}>Log</button>
    <button class:on={orders.tab === "fleets"} onclick={() => (orders.tab = "fleets")}>Fleets ({view.fleets.length})</button>
    <button class:on={orders.tab === "standings"} onclick={() => (orders.tab = "standings")}>Standings</button>
  </nav>
  <section class="log" bind:this={logEl}>
    {#if orders.tab === "fleets"}
      <FleetList {view} />
    {:else if orders.tab === "standings"}
      <Standings {view} seats={store.seats} />
    {:else}
      {#each store.log as entry (entry.id)}
        <p class={entry.tone}><span class="r">{entry.round}</span>{entry.text}</p>
      {:else}
        <p class="muted">Battle reports appear here.</p>
      {/each}
    {/if}
  </section>
</div>

<style>
  .controls {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-height: 0;
  }
  .controls.popped {
    height: 100vh;
    background: var(--panel);
  }
  section {
    padding: 12px 16px;
    border-bottom: 1px solid var(--border);
  }
  p {
    margin: 0 0 8px;
  }
  .status .line {
    display: flex;
    align-items: center;
    gap: 12px;
    flex-wrap: wrap;
  }
  .status .line + .line {
    margin-top: 6px;
    font-size: 13px;
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
  .timer {
    font-variant-numeric: tabular-nums;
    font-weight: 600;
  }
  .timer.low {
    color: var(--bad);
  }
  .dot {
    display: inline-block;
    width: 10px;
    height: 10px;
    border-radius: 50%;
    flex: none;
  }
  .again {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    align-items: center;
  }
  .again .hint {
    flex-basis: 100%;
    margin: 0;
  }
  .row {
    display: flex;
    gap: 8px;
    align-items: center;
  }
  .row input {
    width: 80px;
  }
  .hint {
    font-size: 12px;
    color: var(--muted);
    margin-top: 8px;
  }
  .end,
  .measure {
    width: 100%;
    margin-top: 8px;
  }
  .measure.on,
  .tabs button.on {
    border-color: var(--accent);
    color: var(--accent);
  }
  .tabs {
    display: flex;
    gap: 4px;
    padding: 8px 16px 0;
  }
  .tabs button {
    flex: 1;
    padding: 4px 6px;
    font-size: 13px;
  }
  .key {
    font-size: 11px;
    font-weight: 600;
    padding: 1px 5px;
    border: 1px solid var(--border);
    border-radius: 4px;
    color: var(--muted);
  }
  .info h3 {
    margin: 0 0 8px;
    display: flex;
    align-items: center;
    gap: 8px;
  }
  dl {
    display: grid;
    grid-template-columns: auto 1fr;
    gap: 2px 12px;
    margin: 0;
    font-size: 14px;
  }
  dt {
    color: var(--muted);
  }
  dd {
    margin: 0;
  }
  .log {
    flex: 1;
    overflow-y: auto;
    font-size: 13px;
    border-bottom: none;
  }
  .log p {
    margin: 0 0 6px;
    line-height: 1.35;
  }
  .log .r {
    display: inline-block;
    min-width: 24px;
    color: var(--muted);
  }
  .log .good {
    color: var(--good);
  }
  .log .bad {
    color: var(--bad);
  }
</style>
