# Parallax: Spec for Claude Code

A browser-based, real-time multiplayer space strategy game in the style of KDE Konquest (originally GNU-Lactic Konquest). Players join a room from any browser, each sees only what their empire would see, and turns resolve on the server over WebSockets.

Name: **Parallax**. Go module path: `parallax`.

## Clean-room rule

Parallax is MIT licensed. Konquest is GPL. **Never read, copy or translate Konquest source code** (`game.cpp`, `planet.h`, `src/players/ai/` and so on), and don't fetch it for reference. Rules come from the public handbook, from playing the original, and from our own design. Where those leave a gap, we pick a house rule and document it in [`RULES.md`](RULES.md). `RULES.md` is the source of truth for game rules; this spec describes intent.

## Goals

- The feel of Konquest: planets with production and kill percentage, fleets that take turns to arrive and can't be recalled, no fighting in flight.
- Real online multiplayer. Each player plays from their own device, so hidden information actually works (the original is hot-seat only).
- Depth: maps can be 3D (see "3D maps").
- One self-hosted binary. Runs on a Mac mini, Pi, or small VPS, reachable over Tailscale or the public internet.
- Computer players to fill empty seats, driven by a System 1 decision model with a rule-based fallback.

## Non-goals (v1)

- Accounts, passwords, matchmaking, rankings.
- Persistence across server restarts (beyond optional save/load of a game file).
- Mobile-first layout. It should be usable on an iPad, but desktop is the target.
- Chat (players will be on Discord or in the same room).

## Rules (summary; details and numbers in RULES.md)

### Board

- A grid of cells, width × height × depth (default 16 × 16 × 1, i.e. flat). Planets sit on cells; most cells are empty space.
- Planets are named A, B, C... after the map is shuffled, so names don't reveal home planets.
- The map generator places players + N neutral planets (default 10), with home planets spread apart. "Reroll map" button in the lobby.

### Planets

- **Production**: ships built per round. Home planets 10; neutrals 5 to 15. Fixed for the life of the planet.
- **Kill percentage**: chance that one of this planet's ships destroys an enemy ship in one exchange. Home planets 0.40; neutrals 0.30 to 0.90. Fixed for the life of the planet.
- Planets start with ships equal to their production. Neutral planets grow by only 1 ship per round.
- Room options:
  - Capture production: a just-captured planet produces in the round it's captured, or waits a round (default).
  - Blind map: hide ships, production and kill percentage of planets you don't own.
  - Neutral stats visible on/off.
  - Max rounds (0 = unlimited).
  - Ship speed (cells per round; default 2).
  - Map size and depth.

### Fleets

- A player sends N ships from one of their planets to any other planet. Ships leave immediately and can't be recalled or redirected.
- Travel time in rounds is the Euclidean distance between cells (in 3D) divided by ship speed, rounded up, minimum 1.
- Fleets never fight each other in flight.
- A fleet arriving at its owner's planet reinforces it. A fleet arriving anywhere else fights.
- A fleet fights with the kill percentage of the planet it was launched from.

### Combat

Exchanges repeat until one side has no ships. In each exchange the defender fires first, then the attacker. If the planet falls, it changes owner and the surviving attackers become its garrison. Exact rule in `RULES.md`.

Track per-player stats (ships built, ships destroyed, ships lost, planets conquered, fleets launched) for the standings screen.

### Elimination and victory

- A player is out when they own no planets and have no fleets in flight.
- Last player standing wins. If max rounds is reached, rank by planets, then total ships.

## 3D maps

Distance is the core skill in this game, so depth has to stay readable.

- **Engine:** 3D from day one. Every planet has (x, y, z), and travel uses 3D distance. A flat map is depth 1 (z = 0). This costs almost nothing.
- **Client v1:** the flat SVG board. On depth > 1 maps each planet shows its layer as a `z` label and draws larger the higher it is (a 2.5D view), which keeps clicking and iPad play easy. A shadow-and-stalk cue was tried and dropped: it hid under the planets and cluttered the board.
- **Map shape** (lobby): Cube 32³ (the default; 16 neutrals, ship speed 4), Cube 16³, Shallow 3D 16 × 16 × 4, Flat 16 × 16, or custom sizes. Planets get one sector each (see `RULES.md`), so a cube is filled through its whole volume.
- **3D view** (the default on deep maps, optional on any): a three.js board the player can turn and flip freely like a model (trackball rotation, zoom, pan, and a Reset view button). A cube shows a faint lattice of points through its volume; with a drop line and floor dot under every planet so height and position read clearly, labels with name and ships, fleets in flight, and the aim line with travel time. A click picks a planet only if the pointer didn't drag, so orbiting never picks by accident. A hidden list of planet buttons keeps it usable by keyboard and screen readers, and planet letters still work. three.js loads only when a player opens the view, and the choice is remembered per browser.
- Between Greedy players, games last about as long on every shape (mean 74, 71 and 68 rounds over ten seeds for flat, shallow and deep). Whether 3D is more fun than flat is still for playtests to decide; the flat map stays the default.
- **Territory:** every block of space (one cell on a flat map, an eighth of the largest side in 3D) belongs to the owner of its nearest planet and is tinted their colour: translucent blocks in the cube, tinted cells on the map. Empires visibly grow as they take planets, and a planet taken deep in enemy space makes a pocket until the planets between join up. Neutral space stays clear. It's computed in the client from planet ownership, which is public.
- **Camera:** buttons for Top, Front, Side and Corner views, Reset view, and Arrivals, which steps through planets where your fleets land at the end of this round.
- **Battle replays:** after each round the 3D camera flies to each battle you can see (your own first): the attackers converge, the exchanges flash and crackle, then the planet changes colour or rings as it holds, with the outcome's sound. A click or Esc skips; a toggle turns replays off.
- **Idle drift:** after 10 seconds with no input, no replay and no order being entered, the camera backs out to show the whole map, levels it and turns it slowly. Any input stops it where it is.
- **Sound:** synthesized with Web Audio (no files), following System 1 Arcade's approach: your turn, launches, end turn, reinforcements, battle exchanges, captures, holds, losses, eliminations, win and game over. A header toggle turns it off.
- The measure-distance tool matters more in 3D. Show turns for any pair of planets.

## Factions and names

Every seat plays one of 18 original factions (`web/src/lib/factions.json`), each with its own naming style: the stately Veyrin Concord, the martial Khar-Dovak Ascendancy, the alphanumeric Lattice, the trickster Fennick Mummers, and others. Computers go by their faction; humans keep their own name, with their faction shown in the lobby.

A planet is called by its holder's name for it, so it's renamed when captured and gets its old name back if retaken. Unclaimed worlds have catalogue names ("Survey 118", "Halden's Find"). The server's letters (A, B, C) stay as keyboard shortcuts. Factions and names are derived in the client from the room code, seat and planet number, so every player in a room sees the same names and the protocol is unchanged.

## Turn model

Players take turns one at a time in seat order, like the original. Each player is on their own device, and WebSockets tell everyone whose turn it is.

1. A round starts. The server picks the first living player in seat order and sends them `your_turn` with their view of the state. Everyone else gets `player_turn {seat}` and sees "Alice is moving."
2. The active player gives fleet orders one at a time (source, destination, ship count). An order is final once confirmed: ships leave the source immediately and the order can't be cancelled. Esc cancels an order only while it's being entered.
3. The active player clicks **End Turn**. The server moves to the next living player in seat order and signals everyone again.
4. Computer players take their turn when their seat comes up. The server runs the AI, then advances, with a short configurable delay (default 1 second) so humans can follow along.
5. After the last player in the round ends their turn, the server resolves the round (arrivals, combat, captures, production) as described in `RULES.md`, sends each player their filtered `round_result`, and starts the next round with the first seat.

Turn order lives in the rules engine (`internal/rules`), so the actions log alone replays a game.

Waiting players:
- Can see the board and hover their own planets and fleets, but can't give orders. The server rejects orders from anyone who isn't the active player.
- Get no information about the active player's orders. They only find out about fleets when the fleets arrive.

Timeouts (room settings, all off by default):
- Turn timer (`turnTimerMs`): if the active player doesn't end their turn in time, the server ends it for them, keeping any orders they already gave. Turn messages carry the `deadline`.
- Disconnect (`disconnectGraceMs`): if the active player is disconnected, the server waits for the grace period, then skips their turn. With no grace period it waits; the host can always hand a disconnected seat to a computer (`replace_with_ai`).

## Computer players

Computer players follow the approach from [System 1 Arcade](https://github.com/brennanMKE/System1Arcade): a System 1 decision model such as [Laya](https://huggingface.co/convaiinnovations/laya) answers typed questions about short plain-English states with calibrated probabilities, and the game's own strategy code turns those answers into orders.

- **The game does the arithmetic; the model judges.** Laya can't count or compare numbers. Each prompt gives conclusions in words: "Planet D is 3 rounds away. It will have about 14 defenders when our fleet arrives, and we can send 30 ships with a weaker kill rate. Is attacking D now a good idea? yes / no".
- **One short state per question**, sent as a batch per decision.
- **Options repeat the state's own words.**
- **Strategy lives in code.** The planner asks whether each of its planets an enemy could reach is in danger and whether each candidate attack is a good bet, then `Decide` defends (hold and reinforce in time), attacks, and moves spare ships from the rear to the front.
- **Oracle.** Every question has a correct answer computed from the true game state (e.g. by simulating the battle many times), so decision quality can be measured, as in System1Arcade.
- **Endpoint.** Same HTTP shape as System1Arcade: `POST {state, questions}` or `{batch}` → `{answers}`, with an optional Bearer token. The server can run Laya locally or call a custom endpoint.
- **Fallback.** `Planner` answers the same questions from its own estimates and needs no model; it's the default computer and the System 1 player's fallback. `Greedy` (attack only) remains as a simple baseline.
- AI players see only their own per-player view, never the full state.

## Architecture

The server is authoritative. Clients only send intents and render what they're told.

### Server: Go

- Single binary with the built frontend embedded via `embed`.
- WebSockets: `github.com/coder/websocket`.
- Packages:
  - `internal/rules`: pure, deterministic game engine. No I/O. Map generation, turn order, orders, round resolution, per-player views, standings. Most of the tests go here.
  - `internal/room`: lobby, seats, settings, turn lifecycle, timers. One goroutine per room owns its state (actor pattern: no shared mutexes on game state).
  - `internal/ai`: computer players: the rule-based `Greedy` player and the System 1 advisor.
  - `internal/proto`: message types and JSON encoding.
  - `cmd/server`: flags `-addr`, `-dev`.
  - `cmd/sim`: plays a seeded game between AIs and prints the log.
- RNG: seeded per game (`math/rand/v2` PCG). Store the seed and all actions so games are replayable, which makes combat bugs reproducible.
- State in memory. Optional `/save` and `/load` endpoints write/read a JSON game file (seed + settings + actions log).

### Client

- TypeScript + Vite. Svelte for the lobby and panels, with the board drawn in SVG (simple hit-testing, scales cleanly, easy to style). See "3D maps" for depth.
- Board shows planets on the grid with owner colors. Hovering a planet shows name, owner, ships, production and kill percentage, limited to what that player is allowed to see.
- Order entry: click source, click destination, type a ship count (default to all, Enter to confirm, Esc to cancel).
- Panels: fleet overview (own fleets, ship counts, arrival round), standings, turn log ("Planet D has held against an attack from Alice"), measure-distance tool (click two planets, show distance and turns).
- Mid-game standings use only the player's own view: everyone's planet count, production where visible, and ships only for yourself (other players' totals would reveal fleets in flight).
- Reconnects automatically. On reconnect the server resends the full view for that player.

### Hidden information

The server builds a **per-player view** each turn and never sends data a player shouldn't see. Hiding enemy data only in the client doesn't count. Rules:

- Own planets: everything.
- Enemy/neutral planets: name, position and owner always. Ships, production and kill percentage only if the room settings allow it.
- Fleets: only your own. Enemy fleets are invisible until they arrive.
- Turn log: a player only gets events that involve them (their attacks, attacks on their planets, their reinforcements), plus global events such as a player being eliminated.

## Protocol

JSON over one WebSocket per client at `/ws?room=CODE`. Every message has `{"type": ..., ...}`.

Client to server:
- `join {name, token?}`: `token` resumes a seat after reconnect (stored in localStorage).
- `set_settings {...}` (host only, lobby only)
- `add_ai {kind}` / `remove_seat {seat}` (host only, lobby only). `kind` defaults to `greedy`.
- `replace_with_ai {seat, kind?}` (host only): hand a disconnected player's seat to a computer player.
- `reroll_map` (host only)
- `watch {watch}` (host only, lobby only): the host leaves their seat to watch instead of play, or takes a seat again. A watcher's seat is `-2`. In a game with no human players a watcher sees everything (every planet's stats, every fleet, every battle); otherwise only what any player could see.
- `start_game` (host only)
- `send_fleet {from, to, ships}`: active player only. Final once accepted.
- `end_turn`: active player only.

Server to client:
- `welcome {seat, token, room, host}`: also re-sent when a seat number changes after a removal or when the host starts or stops watching.
- `lobby {room, phase, host, seats, settings, mapPreview, aiKinds}`: sent on every lobby change, at game start and at game over. `phase` is `lobby`, `playing` or `over`; `mapPreview` (positions and owners only) is sent only in the lobby. `aiKinds` lists the computer players this server offers (`greedy`, plus `system1` when started with `-system1`).
- `your_turn {round, view, deadline?}`: sent to the active player.
- `player_turn {round, seat, name, view, deadline?}`: sent to everyone else, with their own view.
- `fleet_ack {fleetId, arrivalRound}` / `error {code, message}`
- `seat_status {seat, connected}`
- `round_result {round, events, view}`: sent to each player after the round resolves, with only the events and state that player may see.
- `game_over {winner, standings}`

HTTP: `POST /api/rooms` creates a room and returns `{code}`; `GET /api/rooms/{code}` checks that a room exists. Rooms close after 10 minutes with no human connected.

Save and load: `GET /api/rooms/{code}/save?token=HOST_TOKEN` downloads `{version, seed, round, settings, seats, actions}` (host only, since the actions log reveals every player's orders). `POST /api/load` with that file replays it into a new room and returns `{code}`. Human seats start unclaimed; a player joining without a token takes the seat with their name, or the first free one. The first human seat is the host.

Validate every client message on the server: it's the sender's turn, they own the source planet, they have enough ships, the game is in the right phase, and host-only actions come from the host.

## Rooms and lobby

- Landing page: "Create game" gives a short room code and shareable URL; "Join" takes a code.
- Lobby: player list with colors, host controls (settings, add AI, reroll map, start), map preview.
- Up to 9 seats total (humans + AI).
- A player who disconnects mid-game keeps their seat. Host can switch them to AI after a timeout.

## Testing

- `rules`: table-driven tests for map generation (2D and 3D), production, travel time, reinforcement, combat (fixed seeds and statistical checks), several fleets arriving at one planet in the same round, elimination and every room option.
- Turn order: seat rotation, skipping eliminated players, and orders rejected from players who aren't active.
- Golden replay test: seed + actions log produces the same final state every time.
- `ai`: AI-vs-AI games finish without illegal orders; the System 1 advisor's oracle scores well, and per-question model accuracy is measurable.
- `room`: simulated clients over an in-process WebSocket server to test the turn lifecycle, AI turns, turn timer, disconnect skip, reconnect mid-turn, and that no message to a waiting player reveals the active player's orders.
- One Playwright smoke test: two browsers join, take turns for three rounds, and one captures a neutral.

## Deployment

- `make build` produces one binary (frontend built and embedded).
- Runs behind Tailscale with no extra config. For public hosting, put it behind Caddy for TLS (WebSockets pass through).
- Optional Dockerfile.

## Milestones

1. `rules` engine + tests (3D-capable), `RULES.md`, the `Greedy` AI, and `cmd/sim`, which plays a seeded game between AIs and prints the log.
2. Server rooms + protocol, AI seats using `Greedy`.
3. Client: lobby, flat SVG board, order entry, End Turn, turn log.
4. Fleet overview, standings, measure distance, room options, turn timer, 2.5D height display.
5. System 1 advisor with oracle and accuracy tooling, save/load, Playwright test, polish.
6. 3D map shapes and an optional three.js board.

## Naming and licensing

- MIT licensed (see `LICENSE`). Clean-room as described above.
- Original name (Parallax) and art. No KDE icons, names or branding.
