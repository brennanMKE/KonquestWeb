import type { PlanetView, Pos } from "./proto";
import { NEUTRAL } from "./proto";

/** One block of space and the player whose territory it is. */
export interface Block {
  x: number;
  y: number;
  z: number;
  size: number;
  /** Height in layers (1 on a flat map, size otherwise). */
  depth: number;
  owner: number;
}

/**
 * Territory: every block of space belongs to whoever owns the planet nearest
 * its centre, so an empire's region grows as it takes planets, and a planet
 * taken deep in enemy space makes a pocket until the planets in between
 * fall too. Neutral space is left out. Planet ownership is public, so this
 * reveals nothing a player couldn't work out.
 */
export function territory(planets: PlanetView[], width: number, height: number, depth: number): Block[] {
  const size = blockSize(width, height, depth);
  const zSize = depth > 1 ? size : 1;
  const out: Block[] = [];
  if (!planets.some((p) => p.owner !== NEUTRAL)) return out;
  for (let z = 0; z < depth; z += zSize) {
    for (let y = 0; y < height; y += size) {
      for (let x = 0; x < width; x += size) {
        const c: Pos = {
          x: x + Math.min(size, width - x) / 2,
          y: y + Math.min(size, height - y) / 2,
          z: z + Math.min(zSize, depth - z) / 2,
        };
        let best = Infinity;
        let owner = NEUTRAL;
        for (const p of planets) {
          const dx = p.pos.x + 0.5 - c.x;
          const dy = p.pos.y + 0.5 - c.y;
          const dz = p.pos.z + 0.5 - c.z;
          const d = dx * dx + dy * dy + dz * dz;
          if (d < best) {
            best = d;
            owner = p.owner;
          }
        }
        if (owner !== NEUTRAL) out.push({ x, y, z, size, depth: zSize, owner });
      }
    }
  }
  return out;
}

/** Blocks are single cells on a flat map and 1/8 of the largest side in 3D. */
export function blockSize(width: number, height: number, depth: number): number {
  return depth > 1 ? Math.max(1, Math.round(Math.max(width, height, depth) / 8)) : 1;
}
