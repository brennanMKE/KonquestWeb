# Parallax Rules

This is the source of truth for game rules. The engine in `internal/rules` implements exactly this, and
its tests check it. Parallax is a clean-room design: these rules come from the general shape of the
Konquest family of games and from our own choices, never from Konquest source code. Rules marked
**house rule** are our own decisions.

## Map

- A grid of `width × height × depth` cells. Defaults: 16 × 16 × 1 (a flat map). Width and height are
  4 to 64; depth is 1 to 64.
- The map holds one home planet per player plus `neutrals` neutral planets (default 10). The total
  number of planets may be at most half the cells.
- Each planet occupies its own cell.
- **House rule, placement:** homes are placed one at a time; each picks, from 64 random free cells,
  the one farthest from the homes already placed. Neutrals go on random free cells, preferring cells
  not adjacent (including diagonally, and in 3D) to another planet.
- **House rule, names:** after placement the planets are shuffled, then named A, B, C, …, Z, AA, AB, …
  in that order, so a name doesn't reveal who started where.
- The whole map comes from the game seed, so the same seed and settings give the same map.

## Planets

| | Production | Kill percentage | Starting ships |
|---|---|---|---|
| Home planet | 10 | 0.40 | 10 |
| Neutral planet | random 5 to 15 | random 0.30 to 0.90 (2 decimals) | equal to production |

- Production and kill percentage never change, even when a planet changes owner.
- **House rule:** a neutral planet grows by 1 ship per round, not by its production.

## Turns

- Players have seats 0 to 8 (2 to 9 players). A round is one turn for each living player, in seat
  order.
- On their turn a player may send any number of fleets, then ends the turn. Only the active player
  may act.
- A fleet order names a source planet the player owns, a different destination planet, and a ship
  count from 1 to the ships currently on the source. The ships leave at once; the order is final.
- When the last living player ends their turn, the round resolves (below), then the next round begins
  with the first living seat.

## Travel

- Distance is Euclidean between cell coordinates, in 3D: `sqrt(dx² + dy² + dz²)`.
- **House rule:** travel time in rounds is `ceil(distance / shipSpeed)`, at least 1. `shipSpeed`
  defaults to 2 cells per round.
- A fleet launched in round `r` with travel time `t` arrives during the resolution of round
  `r + t - 1`. So a 1-round fleet arrives at the end of the round it was launched in.
- Fleets never meet or fight in flight.

## Round resolution

At the end of round `r`, in this order:

1. **Arrivals.** Every fleet due this round arrives, one at a time, in launch order (fleet ID). Each
   arrival sees the result of the ones before it, so if Alice captures a neutral and Bob's fleet
   arrives after, Bob fights Alice's new garrison.
   - If the fleet's owner owns the planet, the ships join its garrison (reinforcement).
   - Otherwise it's combat.
2. **Production.** Every owned planet adds its production to its garrison, except a planet captured
   this round, which skips production unless the room option *capture produces immediately* is on
   (**house rule**, default off). Every neutral planet adds 1.
3. **Elimination.** A player with no planets and no fleets in flight is eliminated.
4. **Game over.** The game ends if one or no players remain, or if `maxRounds > 0` and `r ≥ maxRounds`.

## Combat

The attacking fleet uses the kill percentage of the planet it was launched from (fixed at launch).
The defenders use the kill percentage of the planet they're defending.

**House rule, exchanges:** repeat until one side has no ships:

1. The defenders fire: with probability `defenderKill`, the attacker loses one ship.
2. If the attacker has ships left, they fire: with probability `attackerKill`, the defender loses
   one ship.

Defenders firing first gives a small defender's advantage. Kill percentages are always above 0, so
combat ends.

- A planet with 0 ships falls at once with no losses.
- If the defenders reach 0, the planet changes owner, and the surviving attackers become its
  garrison.
- If the attackers reach 0, the planet holds with its surviving defenders.

## Stats

Each player tracks ships built (production on owned planets), ships destroyed (enemy ships, including
neutral ones, killed in combat), ships lost, planets conquered and fleets launched.

## Standings and victory

- The last player remaining wins.
- **House rule:** at max rounds, players rank by planets owned, then by total ships (garrisons plus
  fleets in flight). Players tied on both share a rank; if the top rank is shared there's no winner.

## Visibility

- Your own planets and fleets: everything.
- Other planets: name, position and owner always.
- Enemy planets: ships, production and kill percentage, unless *blind map* is on.
- Neutral planets: ships, production and kill percentage only if *neutral stats* is on (default) and
  *blind map* is off.
- Enemy fleets are never visible.
- Events: you see reinforcements and battles you're part of, plus eliminations and game over.

## Room options

| Option | Default | Effect |
|---|---|---|
| `width`, `height`, `depth` | 16, 16, 1 | Map size |
| `neutrals` | 10 | Neutral planets |
| `shipSpeed` | 2 | Cells per round |
| `blindMap` | off | Hide other planets' stats |
| `neutralStats` | on | Show neutral stats (when not blind) |
| `captureProducesImmediately` | off | A captured planet produces in the round it's captured |
| `maxRounds` | 0 | Round limit (0 = none) |
