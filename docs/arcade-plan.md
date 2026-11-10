> A plan from an earlier session for arcade-controller play. The order bar (`web/src/lib/OrderBar.svelte`) now covers keyboard and gamepad orders more simply than section 2.3; see docs/STATUS.md.

# Plan: Playing Parallax with an arcade controller (8-way stick + 6 buttons + Start/Select, 1–2 players)

Repo: `/Users/brennan/Developer/brennanMKE/KonquestWeb`. All paths below are absolute. Planning only; nothing was changed.

## 0. What the codebase gives us today (findings that shape the design)

- **Input today is mouse + a few keys.** `web/src/lib/Game.svelte` (`onkey`, lines 85–91) sends every keydown to `orders.key()`; `web/src/lib/orders.svelte.ts` (`key`, lines 104–118) treats any single letter `a–z` as a planet pick via `p.letter` and Esc as cancel. `Replay.svelte` `onkey` (lines 131–146) handles Arrow/Space/Home/End. `Board3D.svelte` listens for Escape to skip a replay (line 701). All of this stays for mouse/keyboard play; arcade mode must *pre-empt* it because I‑PAC layouts emit letters (P2 stick = R/F/D/G, P2 buttons = A/S/Q/W/I/K) that would otherwise pick planets.
- **Order state is already a module singleton** (`orders`): `source`, `dest`, `ships`, `hover`, `measuring`, `tab`, `pick(id)`, `confirm()`, `endTurn()`, `cancel()`. A controller only needs to drive these; the server protocol (`send_fleet`, `end_turn`) is untouched.
- **Both boards share one Props interface** (`planets, width, height, depth, shipSpeed, seat, fleets, round, source, dest, onpick, onhover, replay, onreplaydone`) and are swapped through the `{#snippet boardProps(Component)}` in `Game.svelte` lines 139–167 and `Replay.svelte` 233–260. Adding a `cursor` prop and a couple of exported functions to both boards is the natural seam.
- **Board3D already has the camera primitives we need**: `flyTo`, `focusOn(id)`, `frameAll`, `viewFrom(name)` with `presets` Top/Front/Side/Corner (lines 302–385), `resetView`, `toggleAutoCamera`, `skipReplay`, `noteInput` (idle drift after 10 s, line 604), `TrackballControls` (line 677) with `controls.target`. Planet world positions come from `toWorld(p)`; projecting with `camera` gives screen space for free. The CSS2D labels (`label()`, lines 98–113) are DOM `<span class="tag">`, so a focused label can get a class and be scaled with CSS.
- **Landing/App flow**: `App.svelte` reads `?room`, `?replay`, `?join=1`; `store.leave()` is `location.href = "/"` (store.svelte.ts line 335). Landing already hosts `DemoBackdrop` (a live AI game on the 3D map): a ready-made attract screen. `POST /api/rooms` creates a room; `add_ai`, `set_settings`, `start_game` are host messages, so a one-button "quick game" is purely client-side.
- **Seats**: one WebSocket = one seat; `join` without a token creates a new human seat (`internal/room/room.go` lines 395–435). Nothing stops one page from opening a second socket for a second local seat, which is how hot-seat can be done without server changes.
- **Rooms close 10 min after no human is connected** (`room.go` line 98) and `-state` keeps rooms across restarts; both matter for idle-timeout-to-attract behaviour.
- **Tests**: Playwright only (`web/e2e/*.spec.ts`, config `web/playwright.config.ts` builds the binary and serves on :8787). There is no unit runner; pure functions (spatial nav, mapping, repeat) deserve one (vitest, or a second Playwright project with no browser).
- **Gesture gotchas**: `unlockAudio()` runs on keydown/pointerdown (Game.svelte line 95). Gamepad button presses are not reliably treated as user activation by Chrome, so a cabinet must launch with autoplay allowed and in kiosk fullscreen rather than relying on `requestFullscreen()` from a pad press. Keyboard encoders (I‑PAC) are real key events and are fine.

## 1. Input layer

### 1.1 Files to add

```
web/src/lib/arcade/
  config.svelte.ts   arcade on/off, URL flag, persisted ArcadeConfig, scale, idle, credits
  actions.ts         Action enum, PadEvent, Binding/Profile types, default profiles
  pad.svelte.ts      the poller: gamepads + keyboard -> edge-detected PadEvents with repeat; handler stack
  focus.ts           spatial focus navigation over DOM focusables (for landing/lobby/game-over/menus)
  spatial.ts         nextInDirection() over 2D points (shared by both boards)
  ArcadeSetup.svelte calibration / remap wizard
  Legend.svelte      bottom button legend (context-sensitive)
  Keyboard.svelte    on-screen keyboard / letter wheel for name and room code
  Menu.svelte        generic stick-driven list of options (used for the in-game menu, quick-game menu, confirms)
```

### 1.2 Abstract actions

```ts
// actions.ts
export type Action =
  | "up" | "down" | "left" | "right"
  | "b1" | "b2" | "b3" | "b4" | "b5" | "b6"
  | "start" | "select" | "coin";
export type Edge = "press" | "repeat" | "release";
export interface PadEvent { player: 0 | 1; action: Action; edge: Edge; t: number }
```

Semantics used across every screen (keep them constant so the legend teaches them once):

| Action | Meaning |
|---|---|
| stick | move cursor / focus; in count mode adjust ships |
| b1 | Pick / Confirm / Send |
| b2 | Back / Cancel / Skip replay |
| b3 | Measure (toggle) / Info |
| b4 | Camera: hold + stick orbits, hold + b5/b6 zooms, tap cycles presets |
| b5 / b6 | Previous / next own planet; in count mode −10 / +10; in menus prev/next tab |
| start | End turn (with confirm) / Play / Start game / "Press Start" |
| select | Menu overlay (view, battles, auto camera, sound, log, standings, leave) |
| coin | optional credit; doubles as `select` when credits are off |

### 1.3 Bindings and profiles

```ts
export type Binding =
  | { kind: "button"; index: number }
  | { kind: "axis"; index: number; sign: 1 | -1 }        // digital stick on an analog axis, ±1
  | { kind: "hat"; index: number; value: number };        // HID hat reported as one axis with stepped values (Firefox)
export interface PadProfile { match: RegExp | string; name: string; bindings: Partial<Record<Action, Binding>> }
export interface KeyProfile { name: string; keys: Record<string /* KeyboardEvent.code */, { player: 0 | 1; action: Action }> }
export interface ArcadeConfig {
  enabled: boolean;
  scale: number;                 // 1.0 desktop, 1.5 cabinet default
  idleMs: number;                // back to attract; 0 = never
  coins: boolean;
  players: Array<{ source: "gamepad" | "keyboard"; gamepadId?: string; profile: PadProfile }>;
  keyProfile: KeyProfile;
  letters: boolean;              // allow planet-letter shortcuts (off when a key profile uses letters)
}
```

Default profiles shipped in `actions.ts`:
- **`standard`** (`gamepad.mapping === "standard"`): dpad = buttons 12–15 OR axes 0/1; b1..b6 = buttons 0,1,2,3,4,5; select = 8; start = 9.
- **`zero-delay`** (mapping `""`, id matching `/Generic|USB Joystick|DragonRise|0079/i`): stick = axes 0 (x) and 1 (y) at ±1; b1..b6 = buttons 0–5; select = 8; start = 9 (K9/K10 on most boards). Anything else is fixed by the wizard.
- **`ipac`** (MAME keyboard layout): P1 `ArrowUp/Down/Left/Right`, `ControlLeft`=b1, `AltLeft`=b2, `Space`=b3, `ShiftLeft`=b4, `KeyZ`=b5, `KeyX`=b6, `Digit1`=start, `Digit5`=coin; P2 `KeyR/F/D/G`, `KeyA`=b1, `KeyS`=b2, `KeyQ`=b3, `KeyW`=b4, `KeyI`=b5, `KeyK`=b6, `Digit2`=start, `Digit6`=coin. Note the hazard: with this layout P1‑b1 (Ctrl) held while P2 presses W sends Ctrl+W to the browser. The docs should recommend remapping the I‑PAC to non-modifier keys (or its gamepad mode) and the code must `preventDefault()` every mapped keydown in arcade mode.

Persist to localStorage `parallax.arcade` (JSON) through the existing `load/save` helpers in `store.svelte.ts` (lines 51–69). Profiles are keyed by `gamepad.id` so a cabinet remembers its board.

### 1.4 Polling, edges, deadzone, repeat (`pad.svelte.ts`)

- One `requestAnimationFrame` loop started when arcade mode is on (independent of Board3D's loop so it runs on every screen). Each frame: `navigator.getGamepads()` (always poll; `gamepadconnected`/`disconnected` only log and trigger auto-assignment). Do not depend on the events: Chrome only lists a pad after it has been pressed once, and the Playwright stub (section 5) never fires them.
- Keyboard: `keydown`/`keyup` on `window` (capture phase, before `Game.svelte`'s handler) maintain a `heldKeys: Set<code>`; ignore `e.repeat` so repeat timing is uniform with pads; `preventDefault()` when the code is mapped. On `blur`/`visibilitychange` clear all held state (prevents stuck buttons).
- Per player, a `pressed: Set<Action>` is recomputed each frame from bindings; diff against the previous frame → `press`/`release`. Repeat: per held action record `since`; emit `repeat` after 350 ms, then every 90 ms; stick-count mode asks for an accelerating ramp (the consumer decides the step from `holdMs`: <1 s step 1, <2.5 s step 5, else 10).
- **Deadzone with hysteresis** for digital sticks on analog axes: press when `|v| ≥ 0.6`, release when `|v| ≤ 0.35`. 8-way: `up` and `right` can both be pressed (diagonal); `spatial.ts` turns the pressed set into a unit direction vector `(dx, dy)` normalised, so diagonals are real directions in the planet picker.
- **Player assignment**: `players[k].gamepadId` matches `gamepad.id`; unassigned pads are claimed by the first free slot when any button is pressed; keyboard profile rows carry their player index. Up to 2 players; others ignored.
- **Handler stack**: `pad.push(handler): () => void` where `handler(e: PadEvent): boolean` (true = consumed). The topmost handler sees events first; a modal (menu, confirm, keyboard) pushes itself in an `$effect` and pops on cleanup. Screens below still get `release` edges so held state doesn't leak. Every event also calls `arcade.noteInput()` (idle timer) and `unlockAudio()`.
- `pad.state` (`$state`) exposes `connected: {index, id, profile}[]` and the last raw snapshot for the wizard.

### 1.5 Calibration / remap UI (`ArcadeSetup.svelte`)

Reached from the attract screen with `select` held 2 s or with `?arcade=1&setup=1`. Steps: "Press any button on Player 1's controls" (assigns the pad or detects keyboard mode), then for each action "Push UP … Push RIGHT … Press button 1 …" capturing whichever button/axis/key changed from the baseline snapshot (axis → `{kind:"axis", sign}`; same axis giving different values for up and right → `hat`). Shows a live diagram of the 6 buttons lighting up to verify, "Save" writes the profile keyed by `gamepad.id`. Usable with the pad itself once stick+b1+b2 are known (defaults fall back to the shipped profile mid-wizard). Also lets a mouse user remap, for the Playwright test.

## 2. Interaction design per screen (stick + buttons only, no mouse, no typing)

Global in arcade mode: a `Legend` bar along the bottom listing what each button does *right now*; focus ring 4 px accent with glow; `html.arcade { font-size: calc(16px * var(--ui-scale)) }`; hover-only hints hidden; toasts bigger and centred.

### 2.1 Landing → Attract (`Landing.svelte`, `DemoBackdrop.svelte`)
- In arcade mode the landing becomes the attract screen: the demo fills the whole viewport (drop the `left: min(30vw, 420px)` offset), big title, a blinking "PRESS START", the caption with the demo standings, and, if `hasSeat(lastRoom)` and `GET /api/rooms/{code}` is 200, "Resume game ABCD".
- `start` → `Menu` with: **Quick game** (vs 1–5 computers, stick left/right sets count; b1 starts), **Create room**, **Join room**, **Resume** (if any), **Watch demo** (opens the replay viewer on the demo like "Watch this game"), **Setup controls**.
- Quick game: `POST /api/rooms`, `store.join(code, name)`, then after `welcome` send `add_ai` ×N (the `aiKinds[0]` default), optionally `set_settings` with `turnTimerMs` from config, then `start_game`; the lobby is skipped visually (show "Starting…"). Lives in `arcade/quickgame.ts`.
- Name entry: default `P1`/`P2` or the saved `parallax.name`; a "Change name" entry opens `Keyboard.svelte` (grid of A–Z, 0–9, space, ⌫, Done; stick moves, b1 types, b2 backspace, start = done; 24 chars max as today). Room code uses the same component in **4-letter wheel mode**: four slots, up/down change the letter, left/right move slot, b1 joins, like high-score entry.
- Load/Watch saved files are hidden in arcade mode (no file dialog without a mouse).

### 2.2 Lobby (`Lobby.svelte`)
Generic spatial focus navigation (`focus.ts`) over the existing form is enough here, plus three control-type rules:
- `<input type=number>`: when focused, left/right change by `step` (and b5/b6 by 10×step), then `onchange` fires through the existing `apply()`.
- `<select>`: left/right change the option (Map shape, computer kind).
- checkbox: b1 toggles. Buttons: b1 clicks.
- b2 or `start` in the lobby = Start game (host); for non-hosts start does nothing. b6 = Add computer shortcut, b5 = Remove last computer. The invite link row is skipped; instead show the room code in 96 px type ("Join on another cabinet with code ABCD").
- Focus order starts on **Start game** so Start twice from the attract menu gets you into a game.

### 2.3 The game (`Game.svelte`, `orders.svelte.ts`, boards)
Add `cursor = $state<number|null>(null)` to `orders` (the planet under the "joystick cursor"), and a `mode` derived from existing state: `map` (no dest), `count` (source and dest set), `measure`.

- **Layout**: arcade mode forces `fullPage` and replaces the `<aside><Controls/></aside>` with an `ArcadeHud.svelte` overlay: top strip (round, whose turn, timer, players), right panel (order state: "From Veyrin Prime (34) → pick a destination", cursor planet info in large type with distance/rounds from the source, last 3 log lines), bottom `Legend`. `Controls.svelte` stays for mouse play.
- **Map mode**: stick moves `orders.cursor` with the spatial algorithm (section 3) and also sets `orders.hover = cursor` so the existing aim line, travel-time label and info panel follow it for free. b1 = `orders.pick(cursor)` (existing rules: first pick must be your planet with ships; toasts explain). b2 = `orders.cancel()` if `orders.busy`, else `skipReplay()` if a battle is playing, else open the menu. b3 = `orders.toggleMeasure()`; in measure mode b1 picks A then B and the HUD shows cells/rounds. b5/b6 = cursor to previous/next *own* planet sorted by `letter` (and by `ships` desc with b4 held). b4 tap = next camera preset (Corner→Top→Front→Side→Corner), b4 hold + stick = orbit, b4 hold + b5/b6 = zoom out/in. `start` = confirm dialog "End turn? b1 Yes / b2 No" (shows how many own planets still have ships unsent; a hold-to-confirm alternative is also fine, but the dialog is easier to test and explain). `select` = menu.
- **Count mode** (after dest is picked, `orders.ships` defaults to all): big readout "SEND 34 / 34" with a bar; up/down ±1 with the accelerating repeat, left/right ±10, b5 = half, b6 = all, b1 = `orders.confirm()`, b2 = `orders.cancel()`. After a send the cursor stays on the source (you often send from several planets; the source is where you were). Add `orders.step(n)` that clamps to `[1, srcPlanet.ships]`.
- **Waiting for another player / watching / eliminated**: stick and camera still work, b1 toasts as today (`pick` returns early when `!store.myTurn`), b2 skips replays.
- **Reading planet info and the log**: the HUD's info block is always the cursor planet. `select` menu has **Log**, **Fleets** and **Standings** pages (reuse `FleetList.svelte`, `Standings.svelte`, and the log markup from `Controls.svelte`) scrolled with up/down, page with b5/b6, b2 back.
- **Menu** (`Menu.svelte` over `Game`): Map view / 3D view (`toggleView`), Battles on/off, Auto camera on/off, Sound, Reset view, Arrivals (steps `nextArrival`), Log, Fleets, Standings, Hand seat to computer (host, for away players), Leave game (confirm). Items are plain `{label, onpick}` so the same component serves the attract menu and confirms.
- **Turn start**: when `your_turn` arrives put the cursor on the own planet with the most ships (or keep it if still owned) and, in 3D, `focusOn` it only if it's off-screen (section 3.3).

### 2.4 Game over (`Game.svelte` `.over` card)
Focus nav over the card's buttons; focus starts on **Play again** (host) or **Watch replay**; b2 = Back to the map; idle → attract. Scale the table (it is `font-size: 14px` today).

### 2.5 Replay viewer (`Replay.svelte`)
left/right = `step(∓1)`, up/down = speed (`speeds` index), b1 = `togglePlay()`, b2 = `onclose`, b5/b6 = previous/next game (`store.selectReplay`), b4 = camera as in the game, b3 = toggle Battles, `start` = Fill page toggle (`enterFullPage` / exit), `select` = Loop. Arcade mode opens the viewer already in `fullPage`. Its `onkey` keeps working for real keyboards; the pad handler is pushed above it.

### 2.6 Attract mode
The arcade landing *is* attract mode (2.1). Idle on any screen (`arcade.idleMs`, default 3 min on landing/lobby/game-over, 8 min in-game but only while it is your turn or the game is over, never while you are watching computers play) → `location.href = "/?arcade=1"`; the seat token survives so "Resume" appears if the room is still alive (10-minute server grace). While in attract, a demo frame counter in the caption shows it is live.

## 3. Joystick selection on the 3D cube (and the 2D map)

### 3.1 Board API additions (both `Board.svelte` and `Board3D.svelte`)
Add a prop `cursor?: number | null` and export from `<script>` (Svelte 5 component exports, reachable via `bind:this`):

```ts
export function screenPositions(): { id: number; x: number; y: number; depth: number; onScreen: boolean }[];
export function orbit(dx: number, dy: number): void;      // radians, around controls.target (3D); no-op in 2D
export function zoom(factor: number): void;               // 3D: move camera along view dir within min/maxDistance; 2D: no-op
export function ensureVisible(id: number): void;          // 3D: pan if off-screen (3.3); 2D: no-op
export function nextPreset(): void;                        // 3D only
export function skip(): void;                              // skipReplay()
```

- 3D: `toWorld(p).project(camera)` → NDC; `x = (ndc.x+1)/2*w`, `y = (1-ndc.y)/2*h`, `depth = ndc.z`, `onScreen = |ndc.x|≤1 && |ndc.y|≤1 && ndc.z<1`. Compute once per call (22 planets, trivial).
- 2D: `x = cx(p)`, `y = cy(p)` in cell units (the viewBox preserves aspect ratio, so directions in cell space equal screen directions).
- Cursor drawing: 3D adds a bright reticle (a `TorusGeometry` ring like the selection rings that `lookAt(camera)` each frame, plus a `.tag.focus` class on the label for a bigger, glowing label); the cursor also raises `emissiveIntensity` like `hover` does today (line 172). 2D draws an SVG ring with corner brackets. Both read `cursor` in the same `$effect` that already tracks `source/dest/hover`.
- `orbit(dx,dy)`: rotate `camera.position - controls.target` about `camera.up` by `dx` and about `right = up × dir` by `dy`, then `camera.lookAt(target)`; call `noteInput()` so idle drift stops. The stick calls it with `±1.6 rad/s · dt` while b4 is held (continuous, from the pad loop at frame rate using held state, not edges). `zoom(f)` scales the camera distance, clamped to `controls.minDistance/maxDistance`.

### 3.2 Spatial next-planet algorithm (`spatial.ts`, pure, unit-tested)

```ts
export interface Pt { id: number; x: number; y: number; depth?: number }
export function nextInDirection(pts: Pt[], from: number, dir: { dx: number; dy: number }, opts = { cone: 0.5, lateral: 2.5, wrap: false }): number | null {
  const c = pts.find((p) => p.id === from);
  if (!c) return pts[0]?.id ?? null;
  const len = Math.hypot(dir.dx, dir.dy) || 1; const ux = dir.dx / len, uy = dir.dy / len;
  let best: Pt | null = null, bestScore = Infinity;
  for (const p of pts) {
    if (p.id === from) continue;
    const vx = p.x - c.x, vy = p.y - c.y;
    const d = Math.hypot(vx, vy); if (d < 1e-6) continue;
    const cos = (vx * ux + vy * uy) / d;
    if (cos < opts.cone) continue;                       // outside a 120° cone (cone = cos 60°)
    const forward = d * cos, lateral = Math.sqrt(Math.max(0, d * d - forward * forward));
    let score = forward + opts.lateral * lateral;        // off-axis costs 2.5× as much as distance
    if (p.depth !== undefined && c.depth !== undefined) score += 0.001 * Math.abs(p.depth - c.depth); // tie-break: nearer in depth
    if (score < bestScore) { bestScore = score; best = p; }
  }
  if (best) return best.id;
  if (!opts.wrap) return null;                            // caller plays a "bump" tick
  // Wrap: the planet farthest *against* the direction.
  let far: Pt | null = null, farD = -Infinity;
  for (const p of pts) { if (p.id === from) continue; const back = -((p.x - c.x) * ux + (p.y - c.y) * uy); if (back > farD) { farD = back; far = p; } }
  return far?.id ?? null;
}
```

Notes:
- Screen y grows downward, so stick `up` is `dy = -1`. Diagonals come from the pressed set (`{up,right}` → `(1,-1)`).
- 2D map uses `wrap: true` (a bounded grid, wrapping feels natural). 3D uses `wrap: false` by default: wrapping across a cube is disorienting; instead the HUD shows "nothing that way, turn the camera (hold 4)". Both are `ArcadeConfig` options.
- Overlap in 3D (two planets stacked on one screen point): the `depth` tie-break picks the nearer one; a second push in the same direction will not reach the farther one, so b5/b6 (cycle own planets) and the camera orbit are the escape hatches. Consider a "stacked" hint when `|v| < 12 px` for another planet.
- Off-screen planets are still candidates (the camera pans to them, 3.3), which keeps every planet reachable without orbiting.
- Cost: 22 planets, 1 projection pass per stick edge/repeat. No caching needed.

### 3.3 Camera follow when the cursor moves (3D)
`ensureVisible(id)`: if `onScreen` with margin (|ndc| ≤ 0.85 and ndc.z < 1) do nothing. Otherwise **pan, don't zoom**: `delta = toWorld(p) - controls.target`, `flyTo(camera.position + delta, controls.target + delta, camera.up, 350)`. The existing `focusOn` zooms to `span*0.45`, which is too jumpy for cursor moves; keep it for b4 tap variants ("Focus cursor" in the menu). On `your_turn` and after a battle replay the auto camera may have moved; `ensureVisible(cursor)` runs once afterwards. The cursor is also `noteInput()` so idle drift never fights the player.

### 3.4 Fast source selection
b5/b6 cycle own planets with `ships > 0`, sorted by `letter`; the first b5/b6 press after `your_turn` lands on the planet with the most ships. Combined with "count defaults to all", a typical attack is: b6 (source), stick toward target, b1, b1.

## 4. Cabinet concerns

- **Arcade flag**: `?arcade=1` turns it on and persists `parallax.arcade.enabled`; `?arcade=0` turns it off. `App.svelte` reads it once; `store.leave()` and `startNew()` keep `?arcade=1` on the URL they navigate to (`web/src/lib/store.svelte.ts` line 335; `App.svelte` line 43). Extra params for tests/ops: `&idle=ms`, `&pad=ipac|zero-delay|standard`, `&setup=1`, `&scale=1.5`, `&coins=1`.
- **Kiosk**: document in a new `ARCADE.md`: Chrome `--kiosk --app=http://host:8080/?arcade=1 --autoplay-policy=no-user-gesture-required --noerrdialogs --disable-session-crashed-bubble`; Firefox `--kiosk`. Explain why: pad presses aren't a user gesture for fullscreen/audio, keyboard encoders are. Add `GET /arcade` → redirect to `/?arcade=1` in `internal/server/server.go` only if a bare path is wanted on the cabinet (optional, one line).
- **Readability**: `--ui-scale` (default 1.5 in arcade mode) applied via `html.arcade` in `app.css`; `.tag` labels in Board3D 11 px → `calc(11px * var(--ui-scale))` and the 2D `.name/.ships` font sizes scaled; focus ring `outline: 4px solid var(--accent); box-shadow: 0 0 12px var(--accent)`; HUD panels on `rgb(11 14 23 / .75)` so they read over the map. Minimum 24 px body text at 1.5×.
- **Idle timeout**: `arcade.noteInput()` on every pad event; a single `setInterval` in `config.svelte.ts` checks `Date.now() - lastInput > idleMs` with the per-screen rules in 2.6.
- **Credits (optional, `coins: true`)**: `coin` increments `parallax.credits`; Quick game / Create / Join need ≥1 credit per local player and consume it at `start_game`; attract shows "CREDITS 0 / INSERT COIN". Free play when `coins` is off (default).
- **Two players**:
  1. *One cabinet, one seat (baseline, phases 1–4)*: both panels drive the same seat; `player` in `PadEvent` is ignored. Co-op by default, zero protocol work.
  2. *One cabinet per seat (free)*: two cabinets join the same room by code (letter wheel). Hidden information works exactly as online.
  3. *Hot-seat on one cabinet (phase 5)*: a second WebSocket from the same page takes a second human seat (no server change). In `store.svelte.ts`, factor the per-connection state (`ws, seat, token, view, round, active, deadline, log, standings`) into a `Session` and keep `sessions: Session[]` + `current` in the store; the public fields become getters over `sessions[current]`, `send()` writes to `sessions[current].ws`, `handle()` is bound per session. On `your_turn` for a local session the store switches `current` *after* a handoff interstitial ("PLAYER 2: press any button", board hidden so P1's fleets in flight aren't visible). Tokens are stored as `parallax.token.ROOM` and `parallax.token.ROOM.2`. Only pad `k` (or any pad if one) drives session `k`. Attract shows "P2 press Start to join" during the lobby; P2 pressing start calls `store.joinLocal("P2")`.
  Online rooms are unchanged: a cabinet is just one (or two) ordinary seats, and online friends can join its room code.

## 5. Phased implementation

### Phase 0: flag, scale, kiosk docs (0.5 day)
- Change: `web/src/App.svelte` (parse `?arcade`, add `html.arcade` class, keep the flag on internal navigations), `web/src/app.css` (`--ui-scale`, focus ring, `.arcade` overrides), `web/src/lib/store.svelte.ts` (`leave()` keeps the flag).
- Add: `web/src/lib/arcade/config.svelte.ts`, `ARCADE.md` (kiosk launch lines, encoder notes, I‑PAC modifier hazard).
- Test: e2e `arcade-flag.spec.ts`: `/?arcade=1` sets the class and survives `store.leave()`.

### Phase 1: input layer + stub harness (2–3 days)
- Add: `actions.ts`, `pad.svelte.ts`, `ArcadeSetup.svelte`, `Legend.svelte`; `web/e2e/arcade/pad.ts` (stub helpers); vitest (`web/package.json` devDependency, `npm run test:unit`) with `web/src/lib/arcade/*.test.ts` for binding decode, hysteresis, repeat timing, keyboard profile.
- Change: `Game.svelte`, `Replay.svelte` keydown handlers early-return when `pad.consumedKey(e)` is true; `orders.key()` skips letters when `!arcade.letters`.
- Test: e2e `arcade-pad.spec.ts`: stubbed pad presses light the setup diagram; a keyboard profile press (`page.keyboard.down("Digit1")`) is recognised; blur clears held state.

### Phase 2: DOM focus navigation, attract menu, quick game, lobby, game over (2–3 days)
- Add: `focus.ts` (candidates = visible `button, [href], input, select, [tabindex]:not([tabindex="-1"])` inside the active screen root; score by the same cone/lateral rule as 3.2 on `getBoundingClientRect()` centres; wrap within the screen), `Menu.svelte`, `Keyboard.svelte`, `quickgame.ts`.
- Change: `Landing.svelte` (arcade branch: attract layout, Press Start, menu, resume), `DemoBackdrop.svelte` (full-bleed when arcade), `Lobby.svelte` (number/select/checkbox stick rules, big room code, start-focused), `Game.svelte` game-over card focus.
- Test: e2e `arcade-quickgame.spec.ts`: `/?arcade=1`, stub pad, `start` → Quick game → "Your turn" visible; `arcade-join.spec.ts`: wheel enters a code created by a second context and joins; lobby number field changes with left/right and reaches the server (`toHaveValue`).

### Phase 3: in-game cursor, boards, count mode, camera, HUD (3–4 days)
- Add: `spatial.ts` (+ unit tests with fixtures: a 3×3 grid, overlapping points, off-cone cases, wrap), `ArcadeHud.svelte`, `arcade/gameInput.ts` (the handler for the game screen: maps PadEvents to `orders` and board calls, holds `mode`).
- Change: `orders.svelte.ts` (`cursor`, `step(n)`, `cycleOwn(dir)`), `Board.svelte` and `Board3D.svelte` (`cursor` prop, reticle, exports from 3.1), `Game.svelte` (`bind:this` on the board inside the snippet, mount `ArcadeHud` + `gameInput` in arcade mode, force `fullPage`), `Controls.svelte` untouched.
- Expose for tests: `data-cursor={cursorLetter}` on `.board-wrap` and `data-mode` on the HUD.
- Test: e2e `arcade-play.spec.ts` on the Flat map: cycle with b6 to an own planet, compute the expected neighbour in the page from `.planet` circle centres (like `nearestTarget` in `smoke.spec.ts`) for a given direction, press that direction, assert `data-cursor`, b1, b1 → "Fleet of N sent"; `start`, b1 → round 2. `arcade-3d.spec.ts` on the cube: read `.tag` label boxes, assert that after `right` the cursor's label x is greater than before and the planet is the min-score candidate computed in-page; b4+right changes label positions (orbit); b2 during a battle sets `data-battles="idle"` quickly.

### Phase 4: replay, attract polish, idle, credits (1–2 days)
- Change: `Replay.svelte` (pad handler, arcade opens in full page), `Landing.svelte` (idle, credits), `config.svelte.ts` (idle interval).
- Test: `arcade-idle.spec.ts` with `&idle=1500`: lobby returns to attract; in-game idle does not fire while "X is moving"; `arcade-replay.spec.ts`: `left/right` step the round caption.

### Phase 5: hot-seat (2–3 days)
- Change: `store.svelte.ts` (Session refactor, `joinLocal`), `Game.svelte` (handoff interstitial), `Landing.svelte` (P2 join), `pad.svelte.ts` (player gating).
- Test: `arcade-hotseat.spec.ts`: two stubbed pads; P2 joins; P1's "Fleet of" log line is absent after the handoff; only pad 2 acts on P2's turn; `toHaveCount(2)` human seats in the lobby. Also `go test ./...` unchanged (no server change).

### Phase 6: polish and docs (1–2 days)
- README "Arcade cabinet" section pointing to `ARCADE.md`; SPEC.md "Arcade mode" paragraph; `make e2e` includes the new specs; sound cues for cursor move (tick), bump, confirm.

Total: roughly 11–17 days of work; phases 0–3 (one cabinet, one seat) are the playable core at about 8–10 days.

### 5.1 Playwright gamepad stub (how)
`web/e2e/arcade/pad.ts`:

```ts
export async function installPad(page: Page, id = "Generic USB Joystick (Vendor: 0079 Product: 0006)") {
  await page.addInitScript((id) => {
    const mk = (index: number) => ({ id, index, connected: true, mapping: "", timestamp: 0,
      axes: [0, 0], buttons: Array.from({ length: 12 }, () => ({ pressed: false, touched: false, value: 0 })) });
    const pads: any[] = [mk(0), mk(1)];
    (window as any).__pads = {
      press: (i: number, b: number, on = true) => { pads[i].buttons[b] = { pressed: on, touched: on, value: on ? 1 : 0 }; pads[i].timestamp++; },
      axis: (i: number, a: number, v: number) => { pads[i].axes[a] = v; pads[i].timestamp++; },
    };
    Object.defineProperty(navigator, "getGamepads", { configurable: true, value: () => pads.slice() });
  }, id);
}
export async function tap(page: Page, player: number, action: Action, holdMs = 60) { /* press, waitForTimeout(holdMs), release, waitForTimeout(40) */ }
```
Because `pad.svelte.ts` polls `getGamepads()` every frame and never requires `gamepadconnected`, the stub needs no events. Keyboard-encoder paths are tested with `page.keyboard.down/up("Digit1")`. Timing-sensitive repeat logic is unit-tested with an injected clock, not in the browser.

## 6. Risks and decisions to confirm
- Chrome's user-activation rule means sound and fullscreen must come from kiosk flags on pad-only cabinets (documented, not fixable in code).
- I‑PAC default modifiers (Ctrl/Alt/Shift) can trigger browser shortcuts with P2 letters; recommend a custom I‑PAC layout or gamepad mode; `preventDefault` mitigates but cannot stop OS-level combos.
- Hot-seat (phase 5) is the only part touching `store.svelte.ts` structurally; everything else is additive.
- 3D wrap is off by default; confirm after playtest.
- `ARCADE.md` must follow AGENTS.md: no real host names.

### Critical Files for Implementation
- web/src/lib/orders.svelte.ts (add `cursor`, `step`, `cycleOwn`; gate letter shortcuts)
- web/src/lib/Board3D.svelte (cursor prop + reticle, `screenPositions`, `orbit`, `zoom`, `ensureVisible`, `nextPreset`, `skip` exports)
- web/src/lib/Game.svelte (arcade layout, `bind:this` on the board, mount HUD + game input handler, confirm dialogs)
- web/src/lib/store.svelte.ts (arcade flag on navigation; phase 5 Session refactor + `joinLocal`)
- web/src/lib/Landing.svelte (attract screen, Press Start menu, quick game, name/code entry)
- New: web/src/lib/arcade/pad.svelte.ts and web/src/lib/arcade/spatial.ts (the input poller and the selection algorithm)
