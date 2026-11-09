# Konquest Web

A browser-based, real-time multiplayer space strategy game in the style of KDE Konquest (originally
GNU-Lactic Konquest). Players join a room from any browser, each sees only what their own empire would
see, and turns resolve on an authoritative Go server over WebSockets.

"Konquest Web" is a working name. The final name and art will be original.

**Status:** planning. See [SPEC.md](SPEC.md) for the design and milestones.

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

A simple rule-based player is planned as well, so games work without a model running.

## Planned architecture

| Path | What |
|---|---|
| `internal/rules` | pure, deterministic game engine: state + orders + seed → state + events |
| `internal/room` | lobby, seats, settings, turn lifecycle, timers (one goroutine per room) |
| `internal/ai` | computer players: System 1 advisor and a rule-based fallback |
| `internal/proto` | WebSocket message types |
| `cmd/server` | single binary with the frontend embedded |
| `web/` | TypeScript + Vite + Svelte client, board drawn in SVG |

## License

[MIT](LICENSE). This is a clean-room implementation written from the game's publicly documented
rules and observed behaviour. It contains no code from KDE Konquest, which is GPL licensed, and
contributions must not copy or translate GPL source. It is not affiliated with KDE and uses none of
its name, icons or branding.
