# Parallax

A browser-based, real-time multiplayer space strategy game in the style of KDE Konquest (originally
GNU-Lactic Konquest). Players join a room from any browser, each sees only what their own empire would
see, and turns resolve on an authoritative Go server over WebSockets.

The name nods to stellar parallax, the way astronomers measure the distance to stars.

**Status:** all six milestones done: a playable online game with rule-based and System 1 computer players,
save/load, and flat or 3D maps with an optional orbitable 3D view. See [SPEC.md](SPEC.md) for the design and milestones, and
[RULES.md](RULES.md) for the exact rules.

```sh
make test                          # rules and AI tests
make sim ARGS="-seed 7 -depth 4"   # a seeded AI-vs-AI game on a 3D map, with its turn log
make build && bin/server            # one binary with the web client; open http://localhost:8080
make e2e                           # browser tests (Playwright)
```

## How it plays

- A grid of planets. Each planet builds ships every turn and has a kill percentage that decides how
  well its ships fight.
- On your turn, send fleets from your planets to any other planet. Fleets take several turns to
  arrive and can't be recalled.
- A fleet arriving at your own planet reinforces it. A fleet arriving anywhere else fights, and if it
  wins it captures the planet.
- Last empire standing wins.

Unlike the original, which is hot-seat only, every player uses their own device, so hidden
information really is hidden: the server never sends a player data they aren't allowed to see.

## Computer players

Empty seats are filled by computer players driven by a **System 1 decision model**, such as the
open-weights [Laya](https://huggingface.co/convaiinnovations/laya), using the approach from
[System 1 Arcade](https://github.com/brennanMKE/System1Arcade):

1. The game describes the situation in short plain-English sentences, each with a typed question
   ("Planet D has 12 ships and builds 9 a turn. Our fleet from B would arrive in 3 turns with 20
   ships. Is attacking D a good idea? yes / no").
2. The model answers with calibrated probabilities in one fast forward pass.
3. The game's own strategy code turns those answers into fleet orders.

The game does the arithmetic and hands the model conclusions in words; the model judges the
situation. Any HTTP endpoint that speaks the same `{state, questions}` → `{answers}` shape can be
plugged in, and every prompt has an "oracle" answer computed from the true game state, so decision
quality can be measured.

Every smart computer player shares one strategy, the **planner**. Each turn it asks two kinds of question:

- for each of its planets an enemy could reach within four rounds: *is this planet in danger or safe?*
  ("Planet F is in danger: enemy ships nearby would probably capture it.")
- for each promising attack: *is this attack a good bet or a bad bet?*

Then it decides in priority order: planets in danger keep all their ships, and safe planets that can arrive
before the enemy send exactly enough reinforcements to bring the enemy's odds under 30%; next it makes the
good-bet attacks; last, safe planets far from the enemy move their leftover ships to the planet nearest the
enemy. Enemy fleets are invisible, so "danger" means what nearby enemy planets *could* do.

Three players answer the questions differently:

| Player | Answers from | Where |
|---|---|---|
| **Planner** | its own estimates of what it can see | rooms (the default computer) |
| **System 1** | a System 1 model such as Laya | rooms, with `-system1` |
| **Oracle** | the true game state | `cmd/sim` only, for measuring |

`Greedy`, the original simple player, is still offered: it only attacks, never defends, and hoards ships.
Over 20 four-player games against three Greedy players, Planner won 12 (chance is 5); one on one it won 13
of 20.

### Running with a model

Start any endpoint with that shape. For example, with System 1 Arcade's Laya server:

```sh
python agents/laya_server.py --port 8799          # in the System1Arcade checkout, with Laya installed
bin/server -system1 http://127.0.0.1:8799/predict # hosts can now add "System 1 model" computers
```

`-system1-key` sends an API key as a Bearer token; `-system1-batch=false` sends one request per state for
endpoints that don't take batches. If the model can't be reached, that computer plays as Planner for the
turn.

### Measuring a model

`cmd/sim` plays seeded games and checks every answer against the oracle:

```sh
go run ./cmd/sim -q -players 3 -ai system1,greedy,greedy -url http://127.0.0.1:8799/predict -v
```

With Laya on an M-series Mac, the first wording ("An attack from E on D, an enemy planet that builds a
fair number of ships, would probably lose, arriving soon.") agreed with the oracle on 79–86% of
questions; every miss was a "good" answer for a losing attack. Leading with the outcome raised that to
91–93%. Making the answer options repeat the state's words ("Our attack on D is a bad bet: it would
probably lose." asked *a good bet or a bad bet?*) reached 100% of 1,764 questions over five games, at
30–140 ms per decision.

The danger questions, written the same way from the start, scored 100% on their first run: Laya agreed
with the oracle on all 1,925 questions (attacks and dangers) over three games against Planners. On a blind
map agreement falls to 77%. That isn't Laya misreading: the sentences there rest on guesses about hidden
ship counts, while the oracle sees the real ones. Better estimates of hidden garrisons would close most of
that gap.

## Watching the computers play

In the lobby, add three computers, tick **Watch only**, and start. The computer delay goes to 3 seconds
so you can follow each move (set it higher if you like). With no human playing you see everything:
every fleet, every planet's stats and every battle, replayed in 3D each round.

## Architecture

| Path | What |
|---|---|
| `internal/rules` | pure, deterministic game engine: map, turns, combat, views, standings, replay |
| `internal/room` | lobby, seats, settings, turn lifecycle, timers (one goroutine per room) |
| `internal/ai` | computer players: System 1 advisor and a rule-based fallback |
| `internal/proto` | WebSocket message types |
| `cmd/sim` | plays a seeded game between computer players and prints the log |
| `cmd/server` | single binary with the frontend embedded |
| `web/` | TypeScript + Vite + Svelte client, board drawn in SVG |

A `Dockerfile` builds the same single binary into a small image (`docker build -t parallax .`, then
`docker run -p 8080:8080 parallax`). It hasn't been test-built yet.

For client development, run `make dev` (the Go server on :8080) and `cd web && npm run dev`; Vite
proxies the API and WebSocket to the Go server.

## License

[MIT](LICENSE). This is a clean-room implementation written from the game's publicly documented
rules and observed behaviour. It contains no code from KDE Konquest, which is GPL licensed, and
contributions must not copy or translate GPL source. It is not affiliated with KDE and uses none of
its name, icons or branding.
