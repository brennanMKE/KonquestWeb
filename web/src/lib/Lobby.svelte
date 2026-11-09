<script lang="ts">
  import { store } from "./store.svelte";
  import { ownerColor } from "./colors";
  import Board from "./Board.svelte";
  import type { Settings } from "./proto";

  let link = $derived(`${location.origin}/?room=${store.room}`);
  let copied = $state(false);

  // The host edits a draft; changes go to the server, which validates and
  // broadcasts the result back.
  let draft = $state<Settings | null>(null);
  $effect(() => {
    if (store.settings) draft = { ...store.settings };
  });

  function apply() {
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
  ];
</script>

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
          <span class="name">{s.name}</span>
          {#if s.seat === store.seat}<span class="badge">you</span>{/if}
          {#if s.seat === store.host}<span class="badge">host</span>{/if}
          {#if s.kind === "ai"}<span class="badge">computer</span>{/if}
          {#if !s.connected}<span class="badge warn">away</span>{/if}
          {#if store.isHost && s.seat !== store.host}
            <button class="small" onclick={() => store.send({ type: "remove_seat", seat: s.seat })}>Remove</button>
          {/if}
        </li>
      {/each}
    </ul>

    {#if store.isHost}
      <div class="actions">
        <button onclick={() => store.send({ type: "add_ai" })} disabled={store.seats.length >= 9}>Add computer</button>
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
          {#each numberFields as [key, label, min, max, step, scale]}
            <label>
              <span>{label}</span>
              <input
                type="number"
                {min}
                {max}
                {step}
                value={(draft[key] as number) / scale}
                oninput={(e) => draft && ((draft[key] as number) = Math.round(e.currentTarget.valueAsNumber * scale))}
              />
            </label>
          {/each}
          {#each boolFields as [key, label]}
            <label class="check">
              <input
                type="checkbox"
                checked={draft[key] as boolean}
                oninput={(e) => draft && ((draft[key] as boolean) = e.currentTarget.checked)}
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
  .name {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
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
