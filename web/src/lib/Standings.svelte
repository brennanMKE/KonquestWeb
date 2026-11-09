<script lang="ts">
  import type { SeatInfo, View } from "./proto";
  import { ownerColor } from "./colors";
  import { SPECTATOR } from "./proto";
  import { store } from "./store.svelte";

  // Mid-game standings, from the player's own view only: planet counts are
  // public, production only where visible, ships only for yourself.
  let { view, seats }: { view: View; seats: SeatInfo[] } = $props();

  let rows = $derived(
    seats
      .map((s) => {
        const planets = view.planets.filter((p) => p.owner === s.seat);
        const hidden = planets.some((p) => p.production === undefined);
        const production = planets.reduce((n, p) => n + (p.production ?? 0), 0);
        // Ship totals: yours always; everyone's for a watcher who sees all.
        const mine = s.seat === view.seat || (view.seat === SPECTATOR && planets.every((p) => p.ships !== undefined));
        const fleets = view.fleets.filter((f) => f.owner === s.seat);
        const ships = mine
          ? planets.reduce((n, p) => n + (p.ships ?? 0), 0) + fleets.reduce((n, f) => n + f.ships, 0)
          : null;
        return { s, planets: planets.length, production: hidden ? null : production, ships, alive: view.players[s.seat]?.alive };
      })
      .sort((a, b) => b.planets - a.planets),
  );
</script>

<table>
  <thead>
    <tr><th>Player</th><th>Planets</th><th>Production</th><th>Ships</th></tr>
  </thead>
  <tbody>
    {#each rows as r (r.s.seat)}
      <tr class:out={!r.alive}>
        <td>
          <span class="dot" style:background={ownerColor(r.s.seat)}></span>
          {r.s.seat === view.seat ? "You" : store.playerName(r.s.seat)}
        </td>
        <td>{r.planets}</td>
        <td>{r.production ?? "?"}</td>
        <td>{r.ships ?? "?"}</td>
      </tr>
    {/each}
  </tbody>
</table>

{#if view.seat !== SPECTATOR}
<dl>
  <dt>Ships built</dt><dd>{view.stats.shipsBuilt}</dd>
  <dt>Ships destroyed</dt><dd>{view.stats.shipsDestroyed}</dd>
  <dt>Ships lost</dt><dd>{view.stats.shipsLost}</dd>
  <dt>Planets conquered</dt><dd>{view.stats.planetsConquered}</dd>
  <dt>Fleets launched</dt><dd>{view.stats.fleetsLaunched}</dd>
</dl>
{/if}

<style>
  table {
    width: 100%;
    border-collapse: collapse;
    margin-bottom: 12px;
  }
  th {
    color: var(--muted);
    font-weight: 500;
    text-align: right;
  }
  th:first-child,
  td:first-child {
    text-align: left;
  }
  td {
    text-align: right;
    padding: 3px 4px;
  }
  tr.out {
    opacity: 0.5;
    text-decoration: line-through;
  }
  .dot {
    display: inline-block;
    width: 8px;
    height: 8px;
    border-radius: 50%;
    margin-right: 4px;
  }
  dl {
    display: grid;
    grid-template-columns: 1fr auto;
    gap: 2px 12px;
    margin: 0;
  }
  dt {
    color: var(--muted);
  }
  dd {
    margin: 0;
    text-align: right;
  }
</style>
