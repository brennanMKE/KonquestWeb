import { NEUTRAL } from "./proto";

// One color per seat, distinguishable on the dark board.
const seatColors = [
  "#4f9dff", // blue
  "#ff6b5b", // red
  "#4fd18b", // green
  "#f5c542", // yellow
  "#c77dff", // violet
  "#ff9f43", // orange
  "#3dd6d0", // teal
  "#ff7eb6", // pink
  "#b8c45a", // olive
];

/** Neutral planets; matches --neutral in app.css. */
export const neutralHex = "#6b7286";

export function ownerColor(owner: number): string {
  return owner === NEUTRAL ? "var(--neutral)" : seatColors[owner % seatColors.length];
}

/** Like ownerColor, but always a hex value (for WebGL). */
export function ownerHex(owner: number): string {
  return owner === NEUTRAL ? neutralHex : seatColors[owner % seatColors.length];
}
