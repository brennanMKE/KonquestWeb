<script lang="ts">
  import { load, nameKey, save, store } from "./store.svelte";

  interface Props {
    room?: string;
    onenter: (room: string, name: string) => void;
  }
  let { room = "", onenter }: Props = $props();

  let name = $state(load(nameKey));
  // The room only seeds the input; later prop changes shouldn't overwrite typing.
  // svelte-ignore state_referenced_locally
  let code = $state(room);
  let error = $state("");
  let busy = $state(false);

  function remember() {
    save(nameKey, name.trim());
  }

  async function create() {
    busy = true;
    error = "";
    try {
      const res = await fetch("/api/rooms", { method: "POST" });
      if (!res.ok) throw new Error(`server said ${res.status}`);
      const { code } = await res.json();
      remember();
      onenter(code, name.trim());
    } catch (e) {
      error = `Couldn't create a game: ${(e as Error).message}`;
    } finally {
      busy = false;
    }
  }

  async function loadFile(e: Event) {
    const file = (e.currentTarget as HTMLInputElement).files?.[0];
    if (!file) return;
    busy = true;
    error = "";
    try {
      const res = await fetch("/api/load", { method: "POST", body: await file.text() });
      const body = await res.json();
      if (!res.ok) throw new Error(body.error ?? `server said ${res.status}`);
      remember();
      onenter(body.code, name.trim());
    } catch (e) {
      error = `Couldn't load that game: ${(e as Error).message}`;
    } finally {
      busy = false;
    }
  }

  async function watchFile(e: Event) {
    const file = (e.currentTarget as HTMLInputElement).files?.[0];
    if (!file) return;
    busy = true;
    error = "";
    try {
      const res = await fetch("/api/replay", { method: "POST", body: await file.text() });
      const body = await res.json();
      if (!res.ok) throw new Error(body.error ?? `server said ${res.status}`);
      store.openReplay(body);
    } catch (e) {
      error = `Couldn't play that game back: ${(e as Error).message}`;
    } finally {
      busy = false;
    }
  }

  async function join(e: SubmitEvent) {
    e.preventDefault();
    const c = code.trim().toUpperCase();
    if (!c) return;
    busy = true;
    error = "";
    try {
      const res = await fetch(`/api/rooms/${encodeURIComponent(c)}`);
      if (res.status === 404) {
        error = `There's no game with code ${c}.`;
        return;
      }
      if (!res.ok) throw new Error(`server said ${res.status}`);
      remember();
      onenter(c, name.trim());
    } catch (e) {
      error = `Couldn't join: ${(e as Error).message}`;
    } finally {
      busy = false;
    }
  }
</script>

<main class="landing">
  <h1>Parallax</h1>
  <p class="tag">Conquer the galaxy, one fleet at a time.</p>

  <label class="field">
    <span>Your name</span>
    <input bind:value={name} maxlength="24" placeholder="Commander" autocomplete="nickname" />
  </label>

  {#if room}
    <form onsubmit={join}>
      <button class="primary" disabled={busy}>Join game {room}</button>
    </form>
  {:else}
    <button class="primary" onclick={create} disabled={busy}>Create game</button>
    <form class="join" onsubmit={join}>
      <input bind:value={code} maxlength="4" placeholder="Code" aria-label="Room code" />
      <button disabled={busy || !code.trim()}>Join</button>
    </form>
    <label class="load">
      <input type="file" accept=".json,application/json" onchange={loadFile} disabled={busy} />
      <span>Load a saved game…</span>
    </label>
    <label class="load">
      <input type="file" accept=".json,application/json" onchange={watchFile} disabled={busy} />
      <span>Watch a saved game…</span>
    </label>
  {/if}

  {#if error}<p class="error">{error}</p>{/if}
</main>

<style>
  .landing {
    max-width: 360px;
    margin: 12vh auto 0;
    padding: 0 16px;
    display: flex;
    flex-direction: column;
    gap: 16px;
  }
  h1 {
    margin: 0;
    font-size: 48px;
    letter-spacing: 0.08em;
  }
  .tag {
    margin: -8px 0 8px;
    color: var(--muted);
  }
  .join {
    display: flex;
    gap: 8px;
  }
  .join input {
    flex: 1;
    text-transform: uppercase;
    letter-spacing: 0.2em;
  }
  .load {
    color: var(--accent);
    cursor: pointer;
    font-size: 14px;
    text-align: center;
  }
  .load input {
    position: absolute;
    width: 1px;
    height: 1px;
    opacity: 0;
  }
  .load:focus-within span {
    outline: 2px solid var(--accent);
  }
  form > .primary,
  .primary {
    width: 100%;
  }
</style>
