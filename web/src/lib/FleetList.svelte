<script lang="ts">
  import type { View } from "./proto";
  import { ownerColor } from "./colors";

  let { view }: { view: View } = $props();

  let fleets = $derived([...view.fleets].sort((a, b) => a.arrivalRound - b.arrivalRound || a.id - b.id));
  const name = (id: number) => view.planets[id]?.name ?? "?";
</script>

{#if fleets.length}
  <table>
    <thead>
      <tr><th>Ships</th><th>Route</th><th>Arrives</th></tr>
    </thead>
    <tbody>
      {#each fleets as f (f.id)}
        {@const left = f.arrivalRound - view.round}
        {@const to = view.planets[f.to]}
        <tr>
          <td>{f.ships}</td>
          <td>
            {name(f.from)} → <span class="dot" style:background={ownerColor(to.owner)}></span>{to.name}
            {#if to.owner === view.seat}<span class="muted">(reinforce)</span>{/if}
          </td>
          <td>
            round {f.arrivalRound}
            <span class="muted">{left <= 0 ? "(end of this round)" : `(in ${left})`}</span>
          </td>
        </tr>
      {/each}
    </tbody>
  </table>
{:else}
  <p class="muted">No fleets in flight.</p>
{/if}

<style>
  table {
    width: 100%;
    border-collapse: collapse;
  }
  th {
    text-align: left;
    color: var(--muted);
    font-weight: 500;
  }
  th,
  td {
    padding: 3px 4px;
    vertical-align: top;
  }
  .dot {
    display: inline-block;
    width: 8px;
    height: 8px;
    border-radius: 50%;
    margin-right: 3px;
  }
</style>
