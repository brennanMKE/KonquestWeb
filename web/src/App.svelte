<script lang="ts">
  import { hasSeat, load, nameKey, store } from "./lib/store.svelte";
  import Landing from "./lib/Landing.svelte";
  import Lobby from "./lib/Lobby.svelte";
  import Game from "./lib/Game.svelte";
  import Replay from "./lib/Replay.svelte";

  const params = new URLSearchParams(location.search);
  const startRoom = (params.get("room") ?? "").toUpperCase();
  // /?replay=ABCD watches a finished room's game back.
  const replayRoom = (params.get("replay") ?? "").toUpperCase();
  let replayFailed = $state(false);
  if (replayRoom) store.watchReplay(replayRoom).then((ok) => (replayFailed = !ok));

  function closeReplay() {
    if (store.joined) store.closeReplays();
    else location.href = "/";
  }
  let room = $state(startRoom);
  let entered = $state(false);

  function enter(code: string, name: string) {
    room = code.toUpperCase();
    history.replaceState(null, "", `/?room=${room}`);
    entered = true;
    store.join(room, name);
  }

  // A player coming back to a room they hold a seat in rejoins straight away.
  // ?join=1 comes from "Start a new game" below, with the name already saved.
  if (startRoom && (hasSeat(startRoom) || params.get("join") === "1")) enter(startRoom, load(nameKey));

  // Never a dead end: from any message, start a new game in one click.
  let starting = $state(false);
  let startError = $state("");
  async function startNew() {
    starting = true;
    startError = "";
    try {
      const res = await fetch("/api/rooms", { method: "POST" });
      if (!res.ok) throw new Error(`server said ${res.status}`);
      const { code } = await res.json();
      location.href = `/?room=${code}&join=1`;
    } catch (e) {
      startError = `Couldn't start a game: ${(e as Error).message}`;
      starting = false;
    }
  }
</script>

{#if store.replay}
  <Replay onclose={closeReplay} />
{:else if replayRoom && !entered}
  <main class="message">
    {#if replayFailed}
      <p>There's no finished game to watch in room {replayRoom}. Rooms close 10 minutes after everyone leaves.</p>
      <div class="next">
        <button class="primary" onclick={startNew} disabled={starting}>Start a new game</button>
        <a href="/">Back to the start</a>
      </div>
    {:else}
      <p class="muted">Loading the replay of room {replayRoom}…</p>
    {/if}
  </main>
{:else if !entered}
  <Landing {room} onenter={enter} />
{:else if store.fatal}
  <main class="message">
    <p>{store.fatal}</p>
    <div class="next">
      <button class="primary" onclick={startNew} disabled={starting}>Start a new game</button>
      <a href="/">Join another game</a>
    </div>
    {#if startError}<p class="error">{startError}</p>{/if}
  </main>
{:else if !store.joined}
  <main class="message"><p class="muted">Joining room {room}…</p></main>
{:else if store.phase === "lobby"}
  <Lobby />
{:else if store.view}
  <Game />
{:else}
  <main class="message"><p class="muted">Loading the game…</p></main>
{/if}

{#if store.toast}
  <div class="toast" role="status">{store.toast}</div>
{/if}

<style>
  .message {
    max-width: 420px;
    margin: 20vh auto 0;
    padding: 0 16px;
    text-align: center;
  }
  .next {
    display: flex;
    gap: 16px;
    justify-content: center;
    align-items: center;
    flex-wrap: wrap;
  }
  .toast {
    position: fixed;
    z-index: 200;
    bottom: 24px;
    left: 50%;
    transform: translateX(-50%);
    background: var(--panel);
    border: 1px solid var(--border);
    padding: 10px 16px;
    border-radius: 8px;
    max-width: calc(100vw - 32px);
  }
</style>
