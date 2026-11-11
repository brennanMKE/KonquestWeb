// Order entry and the measure tool, shared by the board and the controls.
// The controls may live in a pop-out window, so this state sits outside any
// one component.

import { store } from "./store.svelte";
import { play } from "./audio";

class Orders {
  // Pick a source you own, then a destination, then a count.
  source = $state<number | null>(null);
  dest = $state<number | null>(null);
  ships = $state(0);
  hover = $state<number | null>(null);
  /** The planet browsed in the order bar (keyboard or gamepad). */
  focus = $state<number | null>(null);
  shipInput = $state<HTMLInputElement | null>(null);

  // Measure tool: pick two planets to see the distance and travel time.
  measuring = $state(false);
  measureA = $state<number | null>(null);
  measureB = $state<number | null>(null);

  tab = $state<"log" | "fleets" | "standings">("log");

  get view() {
    return store.view!;
  }
  get srcPlanet() {
    return this.source !== null ? this.view.planets[this.source] : null;
  }
  get dstPlanet() {
    return this.dest !== null ? this.view.planets[this.dest] : null;
  }
  get infoPlanet() {
    const id = this.hover ?? this.focus;
    return id !== null ? this.view.planets[id] : (this.dstPlanet ?? this.srcPlanet);
  }
  /** Something Esc would cancel. */
  get busy() {
    return this.measuring || this.source !== null;
  }

  cancel = () => {
    this.source = null;
    this.dest = null;
  };

  toggleMeasure = () => {
    this.measuring = !this.measuring;
    this.measureA = this.measureB = null;
  };

  pick = (id: number) => {
    if (this.measuring) {
      if (this.measureA === null || this.measureB !== null) {
        this.measureA = id;
        this.measureB = null;
      } else if (id !== this.measureA) {
        this.measureB = id;
      }
      return;
    }
    if (!store.myTurn) return;
    const p = this.view.planets[id];
    if (this.source === null || this.dest !== null) {
      if (p.owner === store.seat && (p.ships ?? 0) > 0) {
        this.source = id;
        this.dest = null;
      } else if (p.owner === store.seat) {
        store.showToast(`${p.name} has no ships to send.`);
      } else {
        store.showToast("Pick one of your own planets first.");
      }
      return;
    }
    if (id === this.source) {
      this.cancel();
      return;
    }
    this.dest = id;
    this.ships = this.srcPlanet?.ships ?? 0;
    queueMicrotask(() => {
      this.shipInput?.focus();
      this.shipInput?.select();
    });
  };

  confirm = () => {
    if (this.source === null || this.dest === null) return;
    const max = this.srcPlanet?.ships ?? 0;
    const n = Math.floor(this.ships);
    if (!(n >= 1 && n <= max)) {
      store.showToast(`Send 1 to ${max} ships.`);
      return;
    }
    store.send({ type: "send_fleet", from: this.source, to: this.dest, ships: n });
    // The ships are committed now. Take them off the planet at once rather
    // than when the server's reply comes: on a slow link the next order
    // would otherwise start from the old count. The reply's view replaces
    // this one.
    const src = this.view.planets[this.source];
    if (src.ships !== undefined) src.ships -= n;
    // Sent: nothing stays picked. The next order starts fresh.
    this.dest = null;
    this.source = null;
  };

  endTurn = () => {
    this.cancel();
    play("end");
    store.send({ type: "end_turn" });
  };

  /** Esc cancels; planet letters pick planets. Returns true if Esc was used. */
  key = (e: KeyboardEvent): boolean => {
    if (e.key === "Escape") {
      if (!this.busy) return false;
      if (this.measuring) this.toggleMeasure();
      else this.cancel();
      return true;
    }
    const target = e.target as HTMLElement;
    if (target.tagName === "INPUT" || e.metaKey || e.ctrlKey || e.altKey) return false;
    if (/^[a-z]$/i.test(e.key)) {
      const p = this.view.planets.find((p) => p.letter === e.key.toUpperCase());
      if (p) this.pick(p.id);
    }
    return false;
  };
}

export const orders = new Orders();
