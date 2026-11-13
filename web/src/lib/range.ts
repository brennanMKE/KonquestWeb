// Ranges from a selected planet: how many rounds a fleet takes to each
// other planet, grouped in bands of 3, 5 and 7 rounds, so the player can
// see what's near without hovering each planet.

import type { PlanetView } from "./proto";
import { travelTurns } from "./proto";

export const BANDS = [3, 5, 7] as const;
export type Band = (typeof BANDS)[number] | "far";

/** Rounds from source to every other planet. */
export function turnsFrom(planets: PlanetView[], source: number, speed: number): Map<number, number> {
  const out = new Map<number, number>();
  const from = planets[source];
  if (!from) return out;
  for (const p of planets) if (p.id !== source) out.set(p.id, travelTurns(from.pos, p.pos, speed));
  return out;
}

export function bandOf(turns: number): Band {
  return BANDS.find((b) => turns <= b) ?? "far";
}

/** Ships a planet makes each round, "+?" when that's hidden. */
export const prodLabel = (p: PlanetView) => (p.production === undefined ? "+?" : `+${p.production}`);
