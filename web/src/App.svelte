<script lang="ts">
  import { hasSeat, load, nameKey, store } from "./lib/store.svelte";
  import Landing from "./lib/Landing.svelte";
  import Lobby from "./lib/Lobby.svelte";
  import Game from "./lib/Game.svelte";

  const params = new URLSearchParams(location.search);
  const startRoom = (params.get("room") ?? "").toUpperCase();
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

{#if !entered}
  <Landing {room} onenter={enter} />
{:else if store.fatal}
  <main class="message">
    <p>{store.fatal}</p>
    <a href="/">Back to the start</a>
  </main>
{:else if store.seat < 0}
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
