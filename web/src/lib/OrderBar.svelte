<script lang="ts">
  // Orders without a mouse: From → To → Ships → Send, driven by the arrow
  // keys or a gamepad. Up and down scroll the current list (each planet is
  // highlighted on the map as you pass it, with its details here), select or
  // right locks it in and moves on, back or left steps back to change it.
  import { store } from "./store.svelte";
  import { orders } from "./orders.svelte";
  import { ownerColor } from "./colors";
  import { distance, travelTurns, type PlanetView } from "./proto";
  import { pads, type PadAction, type PadEdge } from "./pad.svelte";

  let view = $derived(store.view!);

  // The bar shows once someone uses the arrow keys or a gamepad, and stays
  // until closed; mouse players never see it.
  let active = $state(false);
  let visible = $derived(active || pads.connected);
  let speed = $derived(view.settings.shipSpeed);

  type Step = "from" | "to" | "ships" | "send";
  // On the Send button rather than the count; reset whenever the
  // destination changes.
  let onSend = $state(false);
  let step = $derived<Step>(orders.source === null ? "from" : orders.dest === null ? "to" : onSend ? "send" : "ships");
  $effect(() => {
    void orders.dest;
    onSend = false;
  });

  // Sorting. Sources: most ships, or most recently active (sent from or
  // received ships). Destinations: nearest, weakest known, or recent.
  const SRC_SORTS = ["Most ships", "Recent"] as const;
  const DST_SORTS = ["Nearest", "Weakest", "Recent"] as const;
  let srcSort = $state<(typeof SRC_SORTS)[number]>("Most ships");
  let dstSort = $state<(typeof DST_SORTS)[number]>("Nearest");
  const recent = (p: PlanetView) => store.activity[p.id] ?? 0;
  const byName = (a: PlanetView, b: PlanetView) => a.name.localeCompare(b.name);

  let sources = $derived(
    view.planets
      .filter((p) => p.owner === store.seat && (p.ships ?? 0) > 0)
      .sort((a, b) =>
        srcSort === "Recent" ? recent(b) - recent(a) || (b.ships ?? 0) - (a.ships ?? 0) : (b.ships ?? 0) - (a.ships ?? 0) || byName(a, b),
      ),
  );
  let src = $derived(orders.srcPlanet);
  let dests = $derived.by(() => {
    if (!src) return [];
    const from = src;
    const d = (p: PlanetView) => distance(from.pos, p.pos);
    // Unknown garrisons sort after every known one.
    const weak = (p: PlanetView) => p.ships ?? Infinity;
    return view.planets
      .filter((p) => p.id !== from.id)
      .sort((a, b) =>
        dstSort === "Weakest"
          ? weak(a) - weak(b) || d(a) - d(b)
          : dstSort === "Recent"
            ? recent(b) - recent(a) || d(a) - d(b)
            : d(a) - d(b),
      );
  });
  let list = $derived(step === "from" ? sources : step === "to" ? dests : []);

  // The highlighted planet in the current list. It stays put when the list
  // re-sorts or changes, and falls back to the top.
  let browse = $state<number | null>(null);
  $effect(() => {
    if (step !== "from" && step !== "to") return;
    if (!list.some((p) => p.id === browse)) browse = list[0]?.id ?? null;
  });
  // The map highlights it, and the side panel shows its details, but only
  // while the bar is showing: a mouse player never sees the bar, and its
  // cursor must not highlight planets they didn't point at.
  $effect(() => {
    orders.focus = visible && store.myTurn && (step === "from" || step === "to") ? browse : null;
    return () => {
      orders.focus = null;
    };
  });
  let index = $derived(list.findIndex((p) => p.id === browse));
  // A window of the list around the highlighted planet.
  const SHOWN = 5;
  let windowStart = $derived(Math.max(0, Math.min(index - Math.floor(SHOWN / 2), list.length - SHOWN)));
  let shown = $derived(list.slice(windowStart, windowStart + SHOWN));

  function move(d: number) {
    if (!list.length) return;
    const i = Math.max(0, Math.min(list.length - 1, (index < 0 ? 0 : index) + d));
    browse = list[i].id;
  }

  function maxShips() {
    return orders.srcPlanet?.ships ?? 0;
  }
  function setShips(n: number) {
    orders.ships = Math.max(1, Math.min(maxShips(), Math.round(n)));
  }

  function forward() {
    switch (step) {
      case "from":
        if (browse === null) return;
        orders.source = browse;
        orders.dest = null;
        browse = null; // the destination list starts at its top
        break;
      case "to":
        if (browse === null) return;
        orders.dest = browse;
        orders.ships = maxShips();
        break;
      case "ships":
        onSend = true;
        break;
      case "send": {
        const from = orders.source;
        orders.confirm();
        // Nothing stays picked; the list's cursor starts back at the source.
        browse = from;
        break;
      }
    }
  }

  function backward() {
    switch (step) {
      case "to":
        browse = orders.source;
        orders.source = null;
        break;
      case "ships":
        browse = orders.dest;
        orders.dest = null;
        break;
      case "send":
        onSend = false;
        break;
    }
  }

  function cycleSort() {
    if (step === "from") srcSort = SRC_SORTS[(SRC_SORTS.indexOf(srcSort) + 1) % SRC_SORTS.length];
    else if (step === "to") dstSort = DST_SORTS[(DST_SORTS.indexOf(dstSort) + 1) % DST_SORTS.length];
  }

  // Start ends the turn, after a second press to be sure.
  let endArmed = $state(false);
  let endTimer = 0;
  function endTurn() {
    if (!endArmed) {
      endArmed = true;
      clearTimeout(endTimer);
      endTimer = window.setTimeout(() => (endArmed = false), 3000);
      return;
    }
    endArmed = false;
    orders.endTurn();
  }

  /** Handles one action. Up and down step 1 at a tap and 5 while held on the count. */
  export function act(a: PadAction, edge: PadEdge = "press"): boolean {
    if (!store.myTurn || orders.measuring) return false;
    if (!visible) {
      // The first press only brings the bar up.
      active = true;
      return true;
    }
    switch (a) {
      case "up":
      case "down": {
        const d = a === "up" ? -1 : 1;
        if (step === "ships") setShips(orders.ships - d * (edge === "repeat" ? 5 : 1));
        else move(d);
        return true;
      }
      case "right":
        if (step === "send") return true;
        forward();
        return true;
      case "select":
        forward();
        return true;
      case "left":
      case "back":
        backward();
        return true;
      case "sort":
        cycleSort();
        return true;
      case "half":
        if (step === "ships") setShips(Math.ceil(maxShips() / 2));
        return true;
      case "all":
        if (step === "ships") setShips(maxShips());
        return true;
      case "end":
        endTurn();
        return true;
    }
    return false;
  }

  // Keyboard: arrows, Enter and Space, unless typing in a field. A held key
  // repeats; the first repeat waits a moment, then they come every 130 ms.
  let keyDown = 0;
  let lastRepeat = 0;
  const KEYS: Record<string, PadAction> = {
    ArrowUp: "up",
    ArrowDown: "down",
    ArrowLeft: "left",
    ArrowRight: "right",
    Enter: "select",
    " ": "sort",
    Backspace: "back",
    End: "end",
  };
  export function key(e: KeyboardEvent): boolean {
    // Shift+Enter ends the turn, like Start on a gamepad.
    const a = e.key === "Enter" && e.shiftKey ? "end" : KEYS[e.key];
    if (!a || e.metaKey || e.ctrlKey || e.altKey) return false;
    const tag = (e.target as HTMLElement)?.tagName;
    if (tag === "INPUT" || tag === "SELECT" || tag === "TEXTAREA" || (tag === "BUTTON" && (a === "select" || a === "sort"))) return false;
    let edge: PadEdge = "press";
    const now = performance.now();
    if (e.repeat) {
      if (now - keyDown < 400 || now - lastRepeat < 130) {
        e.preventDefault();
        return true;
      }
      lastRepeat = now;
      edge = "repeat";
    } else {
      keyDown = now;
    }
    if (!act(a, edge)) return false;
    e.preventDefault();
    return true;
  }

  $effect(() => pads.listen((a, edge) => act(a, edge)));

  // The planet whose details show: the highlighted one in a list, else the
  // chosen destination or source.
  let detail = $derived(
    step === "from" || step === "to"
      ? (view.planets[browse ?? -1] ?? null)
      : (orders.dstPlanet ?? orders.srcPlanet),
  );
  const stat = (v: number | undefined, digits = 0) => (v === undefined ? "?" : digits ? v.toFixed(digits) : String(v));
  const rounds = (p: PlanetView) => (src && p.id !== src.id ? travelTurns(src.pos, p.pos, speed) : null);
  let hints = $derived(
    pads.connected
      ? "D-pad choose · A select · B back · X sort · LB/RB half/all · Start twice ends the turn"
      : "↑↓ choose · Enter or → select · ← back · Space sort · hold ↑↓ for 5 ships · Shift+Enter twice ends the turn",
  );
</script>

{#if store.myTurn && !orders.measuring && visible}
  <div class="bar" role="group" aria-label="Order" data-step={step}>
    {#if step === "from" || step === "to"}
      <div class="list" role="listbox" aria-label={step === "from" ? "Send from" : "Send to"}>
        <div class="head">
          <span>{step === "from" ? "Send from" : `To, from ${src?.name}`}</span>
          <button class="sort" onclick={cycleSort} title="Change the order (Space, or X on a gamepad)">
            {step === "from" ? srcSort : dstSort}
          </button>
        </div>
        {#if windowStart > 0}<div class="more">▲ {windowStart} more</div>{/if}
        {#each shown as p (p.id)}
          {@const r = rounds(p)}
          <button
            class="item"
            class:on={p.id === browse}
            role="option"
            aria-selected={p.id === browse}
            onmouseenter={() => (browse = p.id)}
            onclick={() => {
              browse = p.id;
              forward();
            }}
          >
            <span class="dot" style:background={ownerColor(p.owner)}></span>
            <span class="name">{p.name}</span>
            <span class="num">{stat(p.ships)}</span>
            {#if r !== null}<span class="muted">{r}r</span>{/if}
          </button>
        {:else}
          <div class="muted">No planets with ships to send.</div>
        {/each}
        {#if windowStart + SHOWN < list.length}<div class="more">▼ {list.length - windowStart - SHOWN} more</div>{/if}
      </div>
    {/if}

    {#if detail}
      <div class="detail" aria-live="polite">
        <span class="dot" style:background={ownerColor(detail.owner)}></span>
        <b>{detail.name}</b>
        <span>{store.seatName(detail.owner)}</span>
        <span>ships <b>{stat(detail.ships)}</b></span>
        <span>production <b>{stat(detail.production)}</b></span>
        <span>kill <b>{stat(detail.kill, 2)}</b></span>
        {#if src && detail.id !== src.id}
          <span>{distance(src.pos, detail.pos).toFixed(1)} cells, <b>{travelTurns(src.pos, detail.pos, speed)}</b> rounds</span>
        {/if}
      </div>
    {/if}

    <div class="steps">
      <button class="slot" class:on={step === "from"} onclick={() => step !== "from" && ((browse = orders.source), (orders.source = null))}>
        <small>From</small>{orders.srcPlanet?.name ?? "—"}
      </button>
      <span class="arrow">›</span>
      <button class="slot" class:on={step === "to"} disabled={orders.source === null} onclick={() => orders.dest !== null && ((browse = orders.dest), (orders.dest = null))}>
        <small>To</small>{orders.dstPlanet?.name ?? "—"}
      </button>
      <span class="arrow">›</span>
      <div class="slot ships" class:on={step === "ships"}>
        <small>Ships</small>
        {#if orders.dest !== null}
          <span class="count"><b>{orders.ships}</b> of {maxShips()}</span>
        {:else}—{/if}
      </div>
      <span class="arrow">›</span>
      <button class="slot send primary" class:on={step === "send"} disabled={orders.dest === null} onclick={() => ((onSend = true), forward())}>
        Send
      </button>
    </div>
    <div class="hints">
      <span>{endArmed ? (pads.connected ? "Press Start again to end your turn" : "Press Shift+Enter again to end your turn") : hints}</span>
      {#if !pads.connected}<button class="close" onclick={() => (active = false)} title="Hide the order bar (the arrow keys bring it back)">Hide</button>{/if}
    </div>
  </div>
{/if}

<style>
  .bar {
    position: absolute;
    z-index: 2;
    left: 50%;
    bottom: 16px;
    transform: translateX(-50%);
    width: min(620px, calc(100% - 32px));
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding: 10px 12px;
    border-radius: 10px;
    background: rgb(11 14 23 / 0.85);
    border: 1px solid var(--border);
    font-size: 13px;
  }
  .list {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .head {
    display: flex;
    justify-content: space-between;
    align-items: center;
    color: var(--muted);
    margin-bottom: 2px;
  }
  .sort {
    font-size: 12px;
    padding: 1px 8px;
  }
  .item {
    display: flex;
    align-items: center;
    gap: 8px;
    text-align: left;
    padding: 3px 8px;
    border-color: transparent;
    background: none;
  }
  .item.on {
    border-color: #7cc4ff;
    background: rgb(124 196 255 / 0.12);
  }
  .item .name {
    flex: 1;
  }
  .num {
    font-variant-numeric: tabular-nums;
  }
  .more {
    font-size: 11px;
    color: var(--muted);
    padding-left: 8px;
  }
  .detail {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 4px 12px;
    padding: 6px 8px;
    border-radius: 6px;
    background: rgb(255 255 255 / 0.04);
  }
  .steps {
    display: flex;
    align-items: stretch;
    gap: 4px;
  }
  .slot {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 2px;
    min-width: 0;
    padding: 4px 8px;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: none;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .slot small {
    font-size: 10px;
    color: var(--muted);
    text-transform: uppercase;
    letter-spacing: 0.06em;
  }
  .slot.on {
    border-color: #7cc4ff;
    box-shadow: 0 0 0 1px #7cc4ff;
  }
  .slot.send {
    flex: 0 0 auto;
    align-items: center;
    justify-content: center;
  }
  .arrow {
    align-self: center;
    color: var(--muted);
  }
  .hints {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 8px;
    font-size: 11px;
    color: var(--muted);
  }
  .close {
    font-size: 11px;
    padding: 1px 8px;
  }
  .dot {
    display: inline-block;
    width: 10px;
    height: 10px;
    border-radius: 50%;
    flex: none;
  }
</style>
