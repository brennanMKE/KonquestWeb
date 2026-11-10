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
    if (store.joined) store.replay = null;
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
  if (startRoom && hasSeat(startRoom)) enter(startRoom, load(nameKey));
</script>

{#if store.replay}
  <Replay onclose={closeReplay} />
{:else if replayRoom && !entered}
  <main class="message">
    {#if replayFailed}
      <p>There's no finished game to watch in room {replayRoom}. Rooms close 10 minutes after everyone leaves.</p>
      <a href="/">Back to the start</a>
    {:else}
      <p class="muted">Loading the replay of room {replayRoom}…</p>
    {/if}
  </main>
{:else if !entered}
  <Landing {room} onenter={enter} />
{:else if store.fatal}
  <main class="message">
    <p>{store.fatal}</p>
    <a href="/">Back to the start</a>
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
