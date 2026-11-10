// Gamepads, through the browser's Gamepad API. Pads in the "standard"
// layout (most modern controllers) map their D-pad, left stick and face
// buttons to a few actions; each press is an event, and a held direction
// repeats. Polled once a frame while a pad is connected.

export type PadAction = "up" | "down" | "left" | "right" | "select" | "back" | "sort" | "half" | "all" | "end";
export type PadEdge = "press" | "repeat";
type Listener = (action: PadAction, edge: PadEdge) => void;

// Standard mapping: https://w3c.github.io/gamepad/#remapping
const BUTTONS: Record<number, PadAction> = {
  0: "select", // A / Cross
  1: "back", // B / Circle
  2: "sort", // X / Square
  4: "half", // left bumper
  5: "all", // right bumper
  9: "end", // Start / Options
  12: "up",
  13: "down",
  14: "left",
  15: "right",
};
const REPEATS = new Set<PadAction>(["up", "down", "left", "right"]);
const REPEAT_AFTER_MS = 400;
const REPEAT_EVERY_MS = 130;
// A stick counts as pushed past 0.6 and released under 0.35, so it doesn't
// flicker at the edge.
const PUSH = 0.6;
const RELEASE = 0.35;

class Pads {
  /** Whether a gamepad is connected (and has been pressed once, in Chrome). */
  connected = $state(false);
  private listeners = new Set<Listener>();
  private held = new Map<PadAction, { since: number; last: number }>();
  private stick = new Set<PadAction>();
  private frame = 0;

  constructor() {
    if (typeof window === "undefined" || !("getGamepads" in navigator)) return;
    window.addEventListener("gamepadconnected", () => this.start());
    window.addEventListener("gamepaddisconnected", () => {
      this.connected = this.pads().length > 0;
    });
    // A pad already connected before the page loaded.
    if (this.pads().length) this.start();
  }

  /** Listens for actions until the returned function is called. */
  listen(f: Listener): () => void {
    this.listeners.add(f);
    return () => this.listeners.delete(f);
  }

  private pads(): Gamepad[] {
    return [...(navigator.getGamepads?.() ?? [])].filter((p): p is Gamepad => !!p && p.connected);
  }

  private start() {
    this.connected = true;
    if (!this.frame) this.frame = requestAnimationFrame(this.poll);
  }

  private poll = (now: number) => {
    const pads = this.pads();
    if (!pads.length) {
      this.connected = false;
      this.frame = 0;
      this.held.clear();
      return;
    }
    const down = new Set<PadAction>();
    for (const pad of pads) {
      pad.buttons.forEach((b, i) => {
        const a = BUTTONS[i];
        if (a && b.pressed) down.add(a);
      });
      // The left stick, with hysteresis.
      const [x = 0, y = 0] = pad.axes;
      const axis = (v: number, neg: PadAction, pos: PadAction) => {
        for (const [a, on] of [
          [neg, -v],
          [pos, v],
        ] as const) {
          if (on >= PUSH) this.stick.add(a);
          else if (on <= RELEASE) this.stick.delete(a);
        }
      };
      axis(x, "left", "right");
      axis(y, "up", "down");
    }
    for (const a of this.stick) down.add(a);

    for (const a of down) {
      const h = this.held.get(a);
      if (!h) {
        this.held.set(a, { since: now, last: now });
        this.emit(a, "press");
      } else if (REPEATS.has(a) && now - h.since >= REPEAT_AFTER_MS && now - h.last >= REPEAT_EVERY_MS) {
        h.last = now;
        this.emit(a, "repeat");
      }
    }
    for (const a of [...this.held.keys()]) if (!down.has(a)) this.held.delete(a);
    this.frame = requestAnimationFrame(this.poll);
  };

  private emit(a: PadAction, edge: PadEdge) {
    for (const f of this.listeners) f(a, edge);
  }
}

export const pads = new Pads();
