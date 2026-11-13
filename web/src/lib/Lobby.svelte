<script lang="ts">
  import { untrack } from "svelte";
  import { store } from "./store.svelte";
  import { unlockAudio } from "./audio";
  import { ownerColor } from "./colors";
  import Board from "./Board.svelte";
  import { AI_NAMES, type Settings } from "./proto";

  let link = $derived(`${location.origin}/?room=${store.room}`);
  // The server lists its default kind first.
  let picked = $state<string | null>(null);
  // A kind the server stops offering (its model went down) drops back to the
  // default.
  let aiKind = $derived(picked && store.aiKinds.includes(picked) ? picked : (store.aiKinds[0] ?? "planner"));
  const aiNames = AI_NAMES;
  let copied = $state(false);

  // The host edits a draft; changes go to the server, which validates and
  // broadcasts the result back.
  let draft = $state<Settings | null>(null);
  // Fields typed since the last send. The server's echo of an earlier change
  // must not wipe them out before they're sent.
  let dirty = new Set<keyof Settings>();
  $effect(() => {
    const server = store.settings;
    if (!server) return;
    const next = { ...server };
    const old = untrack(() => draft);
    if (old) for (const k of dirty) (next as Record<string, unknown>)[k] = old[k];
    draft = next;
  });
  function edit<K extends keyof Settings>(key: K, value: Settings[K]) {
    if (!draft) return;
    draft[key] = value;
    dirty.add(key);
  }

  // Map shapes, with a ship speed and neutral count that suit each size.
  const shapes = [
    { name: "Cube 32³", width: 32, height: 32, depth: 32, shipSpeed: 4, neutrals: 16 },
    { name: "Cube 16³", width: 16, height: 16, depth: 16, shipSpeed: 3, neutrals: 12 },
    { name: "Shallow 3D", width: 16, height: 16, depth: 4, shipSpeed: 2, neutrals: 10 },
    { name: "Flat", width: 16, height: 16, depth: 1, shipSpeed: 2, neutrals: 10 },
  ];
  let shape = $derived(
    shapes.find((s) => draft && s.width === draft.width && s.height === draft.height && s.depth === draft.depth)?.name ??
      "Custom",
  );
  function setShape(name: string) {
    const s = shapes.find((s) => s.name === name);
    if (!draft || !s) return;
    draft = { ...draft, width: s.width, height: s.height, depth: s.depth, shipSpeed: s.shipSpeed, neutrals: s.neutrals };
    apply();
  }

  // Watching: the host leaves their seat to watch the computers play. A
  // watcher wants time to follow each move, so the delay goes to 3 s.
  function setWatch(watch: boolean) {
    store.send({ type: "watch", watch });
    if (watch && draft && draft.aiDelayMs < 3000) {
      draft = { ...draft, aiDelayMs: 3000 };
      apply();
    }
  }

  function apply() {
    dirty.clear();
    if (draft) store.send({ type: "set_settings", settings: { ...draft } });
  }

  async function copy() {
    try {
      await navigator.clipboard.writeText(link);
      copied = true;
      setTimeout(() => (copied = false), 1500);
    } catch {
      store.showToast("Couldn't copy; select the link instead.");
    }
  }

  // [setting, label, min, max, step, scale]: the input shows value / scale,
  // so millisecond settings can be edited in seconds.
  const numberFields: [keyof Settings, string, number, number, number, number][] = [
    ["width", "Width", 4, 64, 1, 1],
    ["height", "Height", 4, 64, 1, 1],
    ["depth", "Depth (1 = flat)", 1, 64, 1, 1],
    ["neutrals", "Neutral planets", 0, 60, 1, 1],
    ["shipSpeed", "Ship speed", 0.5, 10, 0.5, 1],
    ["maxRounds", "Max rounds (0 = none)", 0, 999, 1, 1],
    ["turnTimerMs", "Turn timer, s (0 = off)", 0, 3600, 5, 1000],
    ["disconnectGraceMs", "Skip away players after, s (0 = wait)", 0, 600, 5, 1000],
    ["aiDelayMs", "Computer delay, s", 0, 10, 0.25, 1000],
  ];
  const boolFields: [keyof Settings, string][] = [
    ["blindMap", "Blind map"],
    ["neutralStats", "Show neutral stats"],
    ["captureProducesImmediately", "Captured planets produce at once"],
    ["fairMap", "Fair starting positions"],
  ];
</script>

<svelte:window onpointerdown={unlockAudio} />

<main class="lobby">
  <section class="left">
    <h1>Room {store.room}</h1>
    <div class="share">
      <input readonly value={link} aria-label="Invite link" onfocus={(e) => e.currentTarget.select()} />
      <button onclick={copy}>{copied ? "Copied" : "Copy link"}</button>
    </div>

    <h2>Players</h2>
    <ul class="seats">
      {#each store.seats as s (s.seat)}
        <li>
          <span class="dot" style:background={ownerColor(s.seat)}></span>
          <span class="name">
            {s.kind === "ai" ? store.faction(s.seat)?.name : s.name}
            {#if s.kind === "human"}<small>{store.faction(s.seat)?.name}</small>{/if}
          </span>
          {#if s.seat === store.seat}<span class="badge">you</span>{/if}
          {#if s.seat === store.host && store.host >= 0}<span class="badge">host</span>{/if}
          {#if s.kind === "ai"}<span class="badge">{aiNames[s.aiKind ?? ""]?.replace(/ \(.*\)$/, "") ?? "computer"}</span>{/if}
          {#if !s.connected}<span class="badge warn">away</span>{/if}
          {#if store.isHost && s.seat !== store.host}
            <button class="small" onclick={() => store.send({ type: "remove_seat", seat: s.seat })}>Remove</button>
          {/if}
        </li>
      {/each}
    </ul>

    {#if store.watching}
      <p class="muted">You're watching{store.isHost ? " and hosting" : ""}. You won't take a seat.</p>
    {/if}
    {#if store.isHost}
      <label class="check watch">
        <input type="checkbox" checked={store.watching} onchange={(e) => setWatch(e.currentTarget.checked)} />
        <span>Watch only: I don't play, the computers do</span>
      </label>
      <div class="actions">
        <span class="add-ai">
          <button onclick={() => store.send({ type: "add_ai", kind: aiKind })} disabled={store.seats.length >= 9}>
            Add computer
          </button>
          {#if store.aiKinds.length > 1}
            <select value={aiKind} onchange={(e) => (picked = e.currentTarget.value)} aria-label="Computer player type">
              {#each store.aiKinds as k (k)}<option value={k}>{aiNames[k] ?? k}</option>{/each}
            </select>
          {/if}
        </span>
        <button onclick={() => store.send({ type: "reroll_map" })}>Reroll map</button>
        <button class="primary" onclick={() => store.send({ type: "start_game" })} disabled={store.seats.length < 2}>
          Start game
        </button>
      </div>
    {:else}
      <p class="muted">Waiting for the host to start the game.</p>
    {/if}

    {#if draft}
      <h2>Settings</h2>
      <form class="settings" onchange={apply} onsubmit={(e) => e.preventDefault()}>
        <fieldset disabled={!store.isHost}>
          <label class="wide">
            <span>Map shape</span>
            <select value={shape} onchange={(e) => setShape(e.currentTarget.value)}>
              {#each shapes as s (s.name)}<option>{s.name}</option>{/each}
              <option disabled>Custom</option>
            </select>
          </label>
          {#each numberFields as [key, label, min, max, step, scale]}
            <label>
              <span>{label}</span>
              <input
                type="number"
                {min}
                {max}
                {step}
                value={(draft[key] as number) / scale}
                oninput={(e) => edit(key, Math.round(e.currentTarget.valueAsNumber * scale) as Settings[typeof key])}
              />
            </label>
          {/each}
          {#each boolFields as [key, label]}
            <label class="check">
              <input
                type="checkbox"
                checked={draft[key] as boolean}
                oninput={(e) => edit(key, e.currentTarget.checked as Settings[typeof key])}
              />
              <span>{label}</span>
            </label>
          {/each}
        </fieldset>
      </form>
    {/if}
  </section>

  <section class="preview">
    {#if store.mapPreview && store.settings}
      <Board
        planets={store.mapPreview}
        width={store.settings.width}
        height={store.settings.height}
        depth={store.settings.depth}
        shipSpeed={store.settings.shipSpeed}
      />
    {:else}
      <p class="muted">Add another player to see the map.</p>
    {/if}
  </section>
</main>

<style>
  .lobby {
    display: grid;
    grid-template-columns: minmax(300px, 380px) 1fr;
    gap: 24px;
    padding: 24px;
    min-height: 100vh;
    box-sizing: border-box;
  }
  @media (max-width: 760px) {
    .lobby {
      grid-template-columns: 1fr;
      padding: 16px;
    }
  }
  h1 {
    margin: 0 0 12px;
  }
  h2 {
    font-size: 14px;
    text-transform: uppercase;
    letter-spacing: 0.1em;
    color: var(--muted);
    margin: 24px 0 8px;
  }
  .share {
    display: flex;
    gap: 8px;
  }
  .share input {
    flex: 1;
    min-width: 0;
  }
  .seats {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .seats li {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 6px 8px;
    background: var(--panel);
    border-radius: 6px;
  }
  .dot {
    width: 12px;
    height: 12px;
    border-radius: 50%;
    flex: none;
  }
  .name small {
    display: block;
    font-size: 12px;
    color: var(--muted);
  }
  .name {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .watch {
    display: flex;
    align-items: center;
    gap: 6px;
    margin-top: 12px;
  }
  .add-ai {
    display: flex;
    gap: 4px;
  }
  select {
    font: inherit;
    color: inherit;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 0 8px;
  }
  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    margin-top: 12px;
  }
  .settings fieldset {
    border: none;
    padding: 0;
    margin: 0;
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 8px 12px;
  }
  .settings label {
    display: flex;
    flex-direction: column;
    gap: 2px;
    font-size: 13px;
    color: var(--muted);
  }
  .settings label.wide {
    grid-column: span 2;
  }
  .settings select {
    padding: 8px 12px;
  }
  .settings label.check {
    flex-direction: row;
    align-items: center;
    gap: 6px;
    grid-column: span 2;
    color: var(--text);
  }
  .preview {
    min-height: 320px;
    height: calc(100vh - 48px);
    position: sticky;
    top: 24px;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  @media (max-width: 760px) {
    .preview {
      position: static;
      height: auto;
      aspect-ratio: 1;
    }
  }
</style>
