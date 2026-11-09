<script lang="ts">
  import { load, save, store } from "./store.svelte";
  import { ownerColor } from "./colors";
  import { distance, travelTurns } from "./proto";
  import Board from "./Board.svelte";
  import FleetList from "./FleetList.svelte";
  import Standings from "./Standings.svelte";

  let view = $derived(store.view!);
  let settings = $derived(view.settings);

  // Order entry: pick a source you own, then a destination, then a count.
  let source = $state<number | null>(null);
  let dest = $state<number | null>(null);
  let ships = $state(0);
  let hover = $state<number | null>(null);
  let shipInput = $state<HTMLInputElement | null>(null);

  // Measure tool: pick two planets to see the distance and travel time.
  let measuring = $state(false);
  let measureA = $state<number | null>(null);
  let measureB = $state<number | null>(null);

  let tab = $state<"log" | "fleets" | "standings">("log");

  // The 3D view loads three.js on first use; the choice is remembered.
  let view3d = $state(load("parallax.view") === "3d");
  function toggleView() {
    view3d = !view3d;
    save("parallax.view", view3d ? "3d" : "2d");
  }
  let board3d = $derived(view3d ? import("./Board3D.svelte") : null);

  // Turn timer countdown.
  let now = $state(Date.now());
  $effect(() => {
    const t = setInterval(() => (now = Date.now()), 250);
    return () => clearInterval(t);
  });
  let secondsLeft = $derived(store.deadline ? Math.max(0, Math.ceil((store.deadline - now) / 1000)) : null);

  let srcPlanet = $derived(source !== null ? view.planets[source] : null);
  let dstPlanet = $derived(dest !== null ? view.planets[dest] : null);
  let infoPlanet = $derived(hover !== null ? view.planets[hover] : (dstPlanet ?? srcPlanet));

  // A finished turn or a new view drops any half-entered order.
  $effect(() => {
    if (!store.myTurn) cancel();
  });

  function cancel() {
    source = null;
    dest = null;
  }

  function toggleMeasure() {
    measuring = !measuring;
    measureA = measureB = null;
  }

  function pick(id: number) {
    if (measuring) {
      if (measureA === null || measureB !== null) {
        measureA = id;
        measureB = null;
      } else if (id !== measureA) {
        measureB = id;
      }
      return;
    }
    if (!store.myTurn) return;
    const p = view.planets[id];
    if (source === null || dest !== null) {
      if (p.owner === store.seat && (p.ships ?? 0) > 0) {
        source = id;
        dest = null;
      } else if (p.owner === store.seat) {
        store.showToast(`Planet ${p.name} has no ships to send.`);
      } else {
        store.showToast("Pick one of your own planets first.");
      }
      return;
    }
    if (id === source) {
      cancel();
      return;
    }
    dest = id;
    ships = srcPlanet?.ships ?? 0;
    queueMicrotask(() => shipInput?.select());
  }

  function confirm(e: SubmitEvent) {
    e.preventDefault();
    if (source === null || dest === null) return;
    const n = Math.floor(ships);
    if (!(n >= 1 && n <= (srcPlanet?.ships ?? 0))) {
      store.showToast(`Send 1 to ${srcPlanet?.ships ?? 0} ships.`);
      return;
    }
    store.send({ type: "send_fleet", from: source, to: dest, ships: n });
    cancel();
  }

  function endTurn() {
    cancel();
    store.send({ type: "end_turn" });
  }

  // Keyboard: planet letters pick planets, Esc cancels, as in the original.
  function onkey(e: KeyboardEvent) {
    if (e.key === "Escape") {
      if (measuring) toggleMeasure();
      else cancel();
      return;
    }
    const target = e.target as HTMLElement;
    if (target.tagName === "INPUT" || e.metaKey || e.ctrlKey || e.altKey) return;
    if (/^[a-z]$/i.test(e.key)) {
      const p = view.planets.find((p) => p.name === e.key.toUpperCase());
      if (p) pick(p.id);
    }
  }

  let activeName = $derived(store.seats[store.active]?.name ?? "");
  let alive = $derived(view.players[store.seat]?.alive ?? false);

  function stat(v: number | undefined, digits = 0) {
    return v === undefined ? "?" : digits ? v.toFixed(digits) : String(v);
  }

  let logEl = $state<HTMLElement | null>(null);
  $effect(() => {
    void store.log.length;
    logEl?.scrollTo({ top: logEl.scrollHeight });
  });
</script>

<svelte:window onkeydown={onkey} />

<div class="game">
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
    {#if store.isHost}
      <a class="save" href="/api/rooms/{store.room}/save?token={store.token}" download>Save game</a>
    {/if}
    {#if store.conn !== "open"}<span class="badge warn">Reconnecting…</span>{/if}
    {#each store.seats as s (s.seat)}
      <span class="player" class:out={!view.players[s.seat]?.alive} class:away={!s.connected} title={s.name}>
        <span class="dot" style:background={ownerColor(s.seat)}></span>{s.seat === store.seat ? "You" : s.name}
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
          source={measuring ? measureA : source}
          dest={measuring ? measureB : dest}
          onpick={pick}
          onhover={(id) => (hover = id)}
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
      <button class="view-toggle small" onclick={toggleView} title="Switch between the map and a 3D view you can orbit">
        {view3d ? "Map view" : "3D view"}
      </button>
    </div>

    <aside>
      <section class="orders">
        {#if store.phase === "over"}
          <p>The game is over.</p>
        {:else if !alive}
          <p class="muted">You've been eliminated. You can keep watching.</p>
        {:else if !store.myTurn}
          <p class="muted">Waiting for {activeName}. You can look around the map.</p>
        {:else if srcPlanet && dstPlanet}
          <form onsubmit={confirm}>
            <p>
              From <b>{srcPlanet.name}</b> to <b>{dstPlanet.name}</b>,
              {travelTurns(srcPlanet.pos, dstPlanet.pos, settings.shipSpeed)} rounds
            </p>
            <div class="row">
              <input
                bind:this={shipInput}
                type="number"
                min="1"
                max={srcPlanet.ships}
                bind:value={ships}
                aria-label="Ships to send"
              />
              <span class="muted">of {srcPlanet.ships}</span>
              <button class="primary">Send</button>
              <button type="button" onclick={cancel}>Cancel</button>
            </div>
            <p class="hint">Enter to send, Esc to cancel. Orders are final.</p>
          </form>
        {:else if srcPlanet}
          <p>Sending from <b>{srcPlanet.name}</b> ({srcPlanet.ships} ships). Pick a destination.</p>
          <p class="hint">Click the planet or type its letter. Esc cancels.</p>
        {:else}
          <p>Pick one of your planets to send a fleet from.</p>
        {/if}
        {#if store.myTurn}
          <button class="primary end" onclick={endTurn}>End turn</button>
        {/if}
        <button class="measure" class:on={measuring} onclick={toggleMeasure}>
          {measuring ? "Stop measuring" : "Measure distance"}
        </button>
      </section>

      {#if measuring}
        <section class="measuring">
          {#if measureA !== null && measureB !== null}
            {@const a = view.planets[measureA]}
            {@const b = view.planets[measureB]}
            <p>
              <b>{a.name}</b> to <b>{b.name}</b>: {distance(a.pos, b.pos).toFixed(1)} cells,
              <b>{travelTurns(a.pos, b.pos, settings.shipSpeed)}</b> rounds
            </p>
          {:else if measureA !== null}
            <p>From <b>{view.planets[measureA].name}</b>. Pick a second planet.</p>
          {:else}
            <p>Pick two planets to measure.</p>
          {/if}
        </section>
      {/if}

      <section class="info">
        {#if infoPlanet}
          <h3>
            <span class="dot" style:background={ownerColor(infoPlanet.owner)}></span>
            Planet {infoPlanet.name}
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
        <button class:on={tab === "log"} onclick={() => (tab = "log")}>Log</button>
        <button class:on={tab === "fleets"} onclick={() => (tab = "fleets")}>Fleets ({view.fleets.length})</button>
        <button class:on={tab === "standings"} onclick={() => (tab = "standings")}>Standings</button>
      </nav>
      <section class="log" bind:this={logEl}>
        {#if tab === "fleets"}
          <FleetList {view} />
        {:else if tab === "standings"}
          <Standings {view} seats={store.seats} />
        {:else}
          {#each store.log as entry (entry.id)}
            <p class={entry.tone}><span class="r">{entry.round}</span>{entry.text}</p>
          {:else}
            <p class="muted">Battle reports appear here.</p>
          {/each}
        {/if}
      </section>
    </aside>
  </main>

  {#if store.standings}
    <div class="over">
      <div class="card">
        <h2>
          {#if store.winner === store.seat}You win!{:else if store.winner >= 0}{store.seatName(store.winner)} wins{:else}No winner{/if}
        </h2>
        <table>
          <thead>
            <tr><th>#</th><th>Player</th><th>Planets</th><th>Ships</th><th>Built</th><th>Destroyed</th><th>Lost</th><th>Conquered</th></tr>
          </thead>
          <tbody>
            {#each store.standings as s (s.seat)}
              <tr>
                <td>{s.rank}</td>
                <td><span class="dot" style:background={ownerColor(s.seat)}></span> {store.seats[s.seat]?.name}</td>
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
        <button onclick={() => (store.standings = null)}>Back to the map</button>
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
    opacity: 0.85;
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
  aside section {
    padding: 12px 16px;
    border-bottom: 1px solid var(--border);
  }
  aside p {
    margin: 0 0 8px;
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
    border-bottom: none !important;
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
  .over {
    position: fixed;
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
  td:nth-child(2) {
    text-align: left;
  }
  th {
    color: var(--muted);
    font-weight: 500;
  }
</style>
