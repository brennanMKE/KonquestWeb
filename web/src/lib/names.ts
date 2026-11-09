// Factions and planet names (written for Parallax; see factions.json).
//
// Every seat belongs to a faction, and a planet is called by its holder's
// name for it: it changes when the planet is taken and comes back if the
// old owner retakes it. Neutral worlds use catalogue names. Everything is
// derived from the room code, seat and planet number, so every player in a
// room sees the same names without the server sending any.

import data from "./factions.json";
import { NEUTRAL } from "./proto";

export interface Faction {
  id: string;
  name: string;
  short: string;
  adjective: string;
  style: string;
  planets: string[];
}

export const factions: Faction[] = data.factions;
const neutral: string[] = data.neutral;

// FNV-1a, so a room code always maps to the same numbers.
function hash(s: string): number {
  let h = 0x811c9dc5;
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 0x01000193);
  }
  return h >>> 0;
}

// A deterministic shuffle of the factions for a room.
const orders = new Map<string, Faction[]>();
function factionOrder(room: string): Faction[] {
  let order = orders.get(room);
  if (!order) {
    order = [...factions];
    let h = hash(room) || 1;
    for (let i = order.length - 1; i > 0; i--) {
      h = Math.imul(h ^ (h >>> 15), 0x2c1b3c6d) >>> 0;
      const j = h % (i + 1);
      [order[i], order[j]] = [order[j], order[i]];
    }
    orders.set(room, order);
  }
  return order;
}

/** The faction playing a seat. */
export function factionOf(room: string, seat: number): Faction | null {
  return seat >= 0 ? factionOrder(room)[seat % factions.length] : null;
}

/** What the holder of planet id calls it. */
export function planetName(room: string, id: number, owner: number): string {
  const f = owner === NEUTRAL ? null : factionOf(room, owner);
  const list = f ? f.planets : neutral;
  const offset = hash(room + (f ? f.id : "neutral")) % list.length;
  const name = list[(id + offset) % list.length];
  // Maps bigger than a name list reuse names with a numeral.
  const round = Math.floor(id / list.length);
  return round ? `${name} ${round + 1}` : name;
}
