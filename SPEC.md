# Galactic Conquest Web: Spec for Claude Code

A browser-based, real-time multiplayer remake of KDE Konquest (originally GNU-Lactic Konquest). Players join a room from any browser, each sees only what their empire would see, and turns resolve on the server over WebSockets.

Working name: **Konquest Web** (placeholder; see "Naming and licensing").

## Goals

- Faithful to Konquest's rules and feel: grid of planets, ship production, kill percentage, fleets that take turns to arrive, no mid-flight combat.
- Real online multiplayer. Each player plays from their own device, so hidden information actually works (the original is hot-seat only).
- One self-hosted binary. Runs on a Mac mini, Pi, or small VPS, reachable over Tailscale or the public internet.
- Computer players to fill empty seats.

## Non-goals (v1)

- Accounts, passwords, matchmaking, rankings.
- Persistence across server restarts (beyond optional save/load of a game file).
- Mobile-first layout. It should be usable on an iPad, but desktop is the target.
- Chat (players will be on Discord or in the same room).

## Rules (match the original)

Source of truth: the Konquest handbook and `game.cpp` / `planet.h` in the KDE source (sources.debian.org/src/konquest or lxr.kde.org/source/games/konquest). Before implementing the rules engine, read the current upstream `game.cpp` and confirm the numbers below. Where upstream differs, follow upstream and note it in `RULES.md`.

### Board

- Rectangular grid (default 16x16, configurable). Planets sit on cells; most cells are empty space.
- Planets are named A, B, C... in order.
- Map generator places N planets (default: players + 10 neutrals), with home planets spread apart. "Reroll map" button in the lobby.

### Planets

- **Production**: ships built per turn. Neutrals get 5 to 15 (upstream `generatePlanetProduction`). Fixed for the life of the planet.
- **Kill percentage**: 0.30 to 0.90 for neutrals (upstream `generateKillPercentage`). Fixed for the life of the planet.
- **Home planets**: production 10, kill percentage 0.40. One per player.
- **Neutral planets**: start with ships equal to their production, but only build 1 ship per turn while neutral. Confirm against upstream.
- Options from the original's new-game dialog (implement as room settings):
  - Cumulative production (unspent production carries over vs. not). Confirm semantics in upstream.
  - Production after capture (a just-captured planet produces on the next turn vs. waits a turn).
  - Blind map (hide enemy and neutral ship counts / stats).
  - Neutral planets visible stats on/off.
  - Max turns (0 = unlimited).

### Fleets

- A player orders N ships from one of their planets to any other planet. Ships leave immediately and cannot be recalled or redirected.
- Travel time in turns comes from the Euclidean distance between cells. Get the exact formula (distance scale and rounding) from upstream `game.cpp` and use it unchanged.
- Fleets never fight each other in flight.
- Fleet arriving at an own planet: ships are added to the planet's defense ("Reinforcements have arrived").
- Fleet arriving at an enemy or neutral planet: combat.

### Combat (port of upstream logic)

Attackers use the kill percentage of their **source** planet. Defenders use the kill percentage of the planet being defended. Loop until one side has 0 ships:

```
loop:
  aRoll = rand01(); dRoll = rand01()
  if both kill percentages are 0:
      if aRoll < dRoll: defender loses 1 ship  else: attacker loses 1 ship
  else:
      if dRoll < defenderKill: attacker loses 1 ship
      if attacker == 0: planet holds; break
      if aRoll < attackerKill: defender loses 1 ship
      if defender == 0: planet falls; break
```

- If the planet falls, ownership changes and the surviving attacking ships become its defense.
- Track per-player stats (ships built, ships destroyed, ships lost, planets conquered, fleets launched) for the standings screen, as the original does.
- Confirm the order of the defender and attacker rolls and the zero-kill special case against upstream before shipping.

### Turn resolution order

Match upstream's order for: production, fleet arrival, multiple fleets arriving at one planet in the same turn (resolve one at a time in a deterministic order), and capture. Document it in `RULES.md`.

### Elimination and victory

- A player is out when they own no planets and have no fleets in flight.
- Last player standing wins. If max turns is reached, rank by upstream's standings rule (planets, then ships), and note which one in `RULES.md`.

## Turn model (same as KDE)

Players take turns one at a time in seat order, exactly like the original. The only difference is that each player is on their own device, and WebSockets tell everyone whose turn it is.

1. A round starts. The server picks the first living player in seat order and sends them `your_turn` with their view of the state. Everyone else gets `player_turn {seat}` and sees "Alice is moving."
2. The active player gives fleet orders one at a time (source, destination, ship count). As in the original, an order is final once confirmed: ships leave the source immediately and the order can't be cancelled. Esc cancels an order only while it's being entered.
3. The active player clicks **End Turn**. The server moves to the next living player in seat order and signals everyone again.
4. Computer players take their turn when their seat comes up. The server runs the AI, then advances. Add a short configurable delay (default 1 second) so humans can follow along.
5. After the last player in the round ends their turn, the server resolves the round (production, fleet arrivals, combat, captures) in upstream's order, sends each player their filtered `round_result`, and starts the next round with the first seat.

Waiting players:
- Can see the board and hover their own planets and fleets, but can't give orders. The server rejects orders from anyone who isn't the active player.
- Get no information about the active player's orders. They only find out about fleets when the fleets arrive.

Timeouts (room settings, all off by default):
- Turn timer: if the active player doesn't end their turn in time, the server ends it for them, keeping any orders they already gave.
- Disconnect: if the active player disconnects, the server waits for a reconnect grace period, then skips their turn. The host can also switch a disconnected seat to an AI.

Confirm against upstream `game.cpp` that fleets launched during a round are only resolved at the end of the round (after the last player moves), and match it.

## Architecture

The server is authoritative. Clients only send intents and render what they're told.

### Server: Go

- Single binary with the built frontend embedded via `embed`.
- WebSockets: `github.com/coder/websocket` (maintained successor to nhooyr/websocket).
- Packages:
  - `internal/rules`: pure, deterministic game engine. No I/O. Takes `State + Orders + RNG seed` and returns `State + Events`. Most of the tests go here.
  - `internal/room`: lobby, seats, settings, turn lifecycle, timers. One goroutine per room owns its state (actor pattern: no shared mutexes on game state).
  - `internal/ai`: computer players. Implement the upstream AI levels (Default, Defensive, Offensive, Advanced) by reading `src/players/ai/` upstream. At least one level ships in v1.
  - `internal/proto`: message types and JSON encoding.
  - `cmd/server`: flags `-addr`, `-dev`.
- RNG: seeded per game (`math/rand/v2` PCG). Store the seed and all orders so games are replayable, which makes combat bugs reproducible.
- State in memory. Optional `/save` and `/load` endpoints write/read a JSON game file (seed + settings + orders log).

### Client

- TypeScript + Vite. Svelte for the lobby and panels, with the board drawn in SVG (simple hit-testing, scales cleanly, easy to style).
- Board shows planets on the grid with owner colors. Hovering a planet shows name, owner, ships, production and kill percentage, limited to what that player is allowed to see.
- Order entry: click source, click destination, type a ship count (default to all, Enter to confirm, Esc to cancel). Same flow as the original.
- Panels: fleet overview (own fleets, ship counts, arrival turn), standings, turn log of messages ("Planet D has held against an attack from Alice"), measure-distance tool (click two planets, show distance and turns).
- Reconnects automatically. On reconnect the server resends the full view for that player.

### Hidden information

The server builds a **per-player view** each turn and never sends data a player shouldn't see. Hiding enemy data only in the client doesn't count. Rules:

- Own planets: everything.
- Enemy/neutral planets: name and owner always. Ships, production and kill percentage only if the room settings allow it.
- Fleets: only your own. Enemy fleets are invisible until they arrive, as in the original.
- Turn log: a player only gets events that involve them (their attacks, attacks on their planets, their reinforcements), plus global events such as a player being eliminated.

## Protocol

JSON over one WebSocket per client at `/ws?room=CODE`. Every message has `{"type": ..., ...}`.

Client to server:
- `join {name, token?}`: `token` resumes a seat after reconnect (stored in localStorage).
- `set_settings {...}` (host only, lobby only)
- `add_ai {level}` / `remove_seat {seat}` (host only)
- `reroll_map` (host only)
- `start_game` (host only)
- `send_fleet {from, to, ships}`: active player only. Final once accepted.
- `end_turn`: active player only.

Server to client:
- `welcome {seat, token, room}`
- `lobby {players, settings, mapPreview}`
- `your_turn {round, view, deadline?}`: sent to the active player. `view` is that player's state, described above.
- `player_turn {round, seat, name, deadline?}`: sent to everyone else.
- `fleet_ack {fleetId, arrivalRound}` / `error {code, message}`
- `seat_status {seat, connected}`: lets everyone see who has dropped.
- `round_result {round, events, view}`: sent to each player after the round resolves. Contains only the events and state that player may see.
- `game_over {standings}`

Validate every client message on the server: it's the sender's turn, they own the source planet, they have enough ships, the game is in the right phase, and host-only actions come from the host.

## Rooms and lobby

- Landing page: "Create game" gives a short room code and shareable URL; "Join" takes a code.
- Lobby: player list with colors, host controls (settings, add AI, reroll map, start), map preview.
- Up to 9 seats total (humans + AI), like the original.
- A player who disconnects mid-game keeps their seat. Host can switch them to AI after a timeout.

## Testing

- `rules`: table-driven tests for production, travel time, reinforcement, combat (fixed seeds), several fleets arriving at one planet in the same round, elimination and every room option.
- Turn order: seat rotation, skipping eliminated players, AI turns, turn timer, disconnect skip, and orders rejected from players who aren't active.
- Golden replay test: seed + orders log produces the same final state every time.
- `room`: simulated clients over an in-process WebSocket server to test the turn lifecycle, reconnect mid-turn, and that no message to a waiting player reveals the active player's orders.
- One Playwright smoke test: two browsers join, take turns for three rounds, and one captures a neutral.

## Deployment

- `make build` produces one binary (frontend built and embedded).
- Runs behind Tailscale with no extra config. For public hosting, put it behind Caddy for TLS (WebSockets pass through).
- Optional Dockerfile.

## Milestones

1. `rules` engine + tests, verified against upstream source. Includes a CLI that plays a seeded game between AIs and prints the log.
2. Server rooms + protocol + one AI level.
3. Client: lobby, board, order entry, End Turn, turn log.
4. Fleet overview, standings, measure distance, room options, turn timer.
5. Remaining AI levels, save/load, Playwright test, polish.

## Naming and licensing

- Konquest is GPL-2.0-or-later. Porting its combat/AI code line by line makes this project a derivative work, so either license this project GPL-2.0-or-later (simplest, and recommended) or implement only from the documented rules without translating source. Decide before milestone 1 and record the choice in `README.md`.
- Pick an original name and art. Don't use KDE's icon or branding.
- Prior art worth a look: github.com/spacepal/Spacepal-Archive (an unfinished online Konquest).
