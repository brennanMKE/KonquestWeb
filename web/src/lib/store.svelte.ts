// Client state and the WebSocket connection. The server is authoritative:
// this only records what the server says and sends intents.

import type { ClientMsg, GameEvent, Phase, PlanetView, Replay, SeatInfo, ServerMsg, Settings, Standing, View } from "./proto";
import { NEUTRAL, SPECTATOR } from "./proto";
import { play, type Sound } from "./audio";
import { factionOf, planetName } from "./names";
import type { Faction } from "./names";

export interface LogEntry {
  id: number;
  round: number;
  text: string;
  tone: "good" | "bad" | "info";
}

/** A replay with where to get its game file. */
export interface LoadedReplay extends Replay {
  /** The file's name, for the list of games. */
  name?: string;
  /** The game file itself, when it came from one. */
  file?: string;
  /** Where to download the game file, when it came from a room. */
  download?: string;
}

/** Plays game files back; returns the games and any files that failed. */
export async function replayFiles(files: File[]): Promise<{ games: LoadedReplay[]; errors: string[] }> {
  const results = await Promise.all(
    files.map(async (f) => {
      try {
        const text = await f.text();
        const res = await fetch("/api/replay", { method: "POST", body: text });
        const body = await res.json();
        if (!res.ok) throw new Error(body.error ?? `server said ${res.status}`);
        return { game: { ...(body as Replay), name: f.name, file: text } as LoadedReplay };
      } catch (e) {
        return { error: `${f.name}: ${(e as Error).message}` };
      }
    }),
  );
  return {
    games: results.flatMap((r) => (r.game ? [r.game] : [])),
    errors: results.flatMap((r) => (r.error ? [r.error] : [])),
  };
}

const tokenKey = (room: string) => `parallax.token.${room}`;
export const nameKey = "parallax.name";

function load(key: string): string {
  try {
    return localStorage.getItem(key) ?? "";
  } catch {
    return "";
  }
}

function save(key: string, value: string | null) {
  try {
    if (value === null) localStorage.removeItem(key);
    else localStorage.setItem(key, value);
  } catch {
    // Storage can be unavailable (private mode); the seat just won't
    // survive a reload.
  }
}

export { load, save };

/** Whether this browser holds a seat token for the room. */
export function hasSeat(room: string): boolean {
  return load(tokenKey(room.toUpperCase())) !== "";
}

type ConnState = "connecting" | "open" | "closed";

class Store {
  room = $state("");
  conn = $state<ConnState>("closed");
  /** Set when the server ended the session for good (removed, replaced). */
  fatal = $state("");
  seat = $state(-1);
  /** Set once the server has welcomed us. */
  joined = $state(false);
  /** Whether we host the room (we may be watching rather than seated). */
  hosting = $state(false);
  phase = $state<Phase>("lobby");
  host = $state(-1);
  seats = $state<SeatInfo[]>([]);
  settings = $state<Settings | null>(null);
  mapPreview = $state<PlanetView[] | null>(null);
  aiKinds = $state<string[]>(["greedy"]);
  token = $state("");
  view = $state<View | null>(null);
  round = $state(0);
  active = $state(-1);
  /** When the turn timer ends the current turn (Unix ms), or 0. */
  deadline = $state(0);
  log = $state<LogEntry[]>([]);
  standings = $state<Standing[] | null>(null);
  winner = $state(-1);
  toast = $state("");
  /**
   * The last round each planet saw activity this player knows of: a fleet
   * sent from it, or ships arriving there. For sorting the order bar.
   */
  activity = $state<Record<number, number>>({});
  private noteActivity(planet: number, round: number) {
    if (planet >= 0 && (this.activity[planet] ?? 0) < round) this.activity[planet] = round;
  }
  /** The events of the last resolved round, for the board to replay. */
  lastEvents = $state<{ round: number; events: GameEvent[] } | null>(null);
  /** Set while the 3D board replays battles; it plays their sounds then. */
  playsBattles = false;
  /**
   * True from a round's result until the board has replayed its battles.
   * The game-over screen and sound wait for it, so the final battle plays
   * first.
   */
  replayPending = $state(false);
  /** The latest round waiting for its replay. */
  private pendingRound = 0;
  private afterReplay: (() => void) | null = null;
  private replayFallback = 0;

  /**
   * The board calls this when it has replayed a round. Only the latest
   * round's replay ends the wait: an earlier round finishing must not
   * release the final scores while the last battles still play.
   */
  replayDone(round = Infinity) {
    if (round < this.pendingRound) return;
    clearTimeout(this.replayFallback);
    this.replayPending = false;
    const f = this.afterReplay;
    this.afterReplay = null;
    f?.();
  }

  /**
   * Saved games being watched back, over everything else: one from a
   * finished room, or several uploaded files played in turn.
   */
  replays = $state<LoadedReplay[]>([]);
  replayIndex = $state(0);

  /** The game being watched back, or null. */
  get replay(): LoadedReplay | null {
    return this.replays[this.replayIndex] ?? null;
  }

  /**
   * Adds games to watch. Outside a room each game names its players and
   * planets from its own room code, as its players saw them.
   */
  openReplays(list: LoadedReplay[], append = false) {
    const named = list.map((r) => {
      const room = this.joined ? this.room : r.room || "REPLAY";
      const frames = r.frames.map((f) => ({
        ...f,
        view: { ...f.view, planets: f.view.planets.map((p) => ({ ...p, letter: p.letter ?? p.name, name: planetName(room, p.id, p.owner) })) },
      }));
      return { ...r, frames };
    });
    if (append && this.replays.length) {
      this.replays = [...this.replays, ...named];
    } else {
      this.replays = named;
      this.selectReplay(0);
    }
  }

  /** Shows the i-th game of the list. */
  selectReplay(i: number) {
    const n = this.replays.length;
    if (!n) return;
    this.replayIndex = ((i % n) + n) % n;
    const r = this.replays[this.replayIndex];
    if (!this.joined) {
      this.room = r.room || "REPLAY";
      this.seats = r.seats.map((s, seat) => ({ ...s, seat, connected: true }));
    }
  }

  closeReplays() {
    this.replays = [];
    this.replayIndex = 0;
  }

  /** Fetches and opens a finished room's replay; reports failure. */
  async watchReplay(room: string): Promise<boolean> {
    try {
      const res = await fetch(`/api/rooms/${encodeURIComponent(room)}/replay`);
      const body = await res.json();
      if (!res.ok) throw new Error(body.error ?? `server said ${res.status}`);
      this.openReplays([{ ...(body as Replay), download: `/api/rooms/${encodeURIComponent(room)}/save` }]);
      return true;
    } catch (e) {
      this.showToast(`Couldn't load the replay: ${(e as Error).message}`);
      return false;
    }
  }

  private ws: WebSocket | null = null;
  private name = "";
  private retry = 0;
  private logId = 0;
  private toastTimer = 0;

  get isHost() {
    return this.hosting;
  }
  get watching() {
    return this.seat === SPECTATOR;
  }
  get myTurn() {
    return this.phase === "playing" && this.active === this.seat;
  }

  /** A seat's faction. */
  faction(seat: number): Faction | null {
    return factionOf(this.room, seat);
  }

  /** A seat's display name: a computer goes by its faction, a human by name. */
  playerName(seat: number): string {
    const s = this.seats[seat];
    if (!s) return `Player ${seat + 1}`;
    return s.kind === "ai" ? (this.faction(seat)?.short ?? s.name) : s.name;
  }

  seatName(seat: number): string {
    if (seat === NEUTRAL) return "neutral";
    if (seat === this.seat) return "you";
    return this.playerName(seat);
  }

  /** A planet's current name. */
  planetName(id: number): string {
    return this.view?.planets[id]?.name ?? "?";
  }

  /** What a planet is called by a given holder. */
  planetNameFor(id: number, owner: number): string {
    return planetName(this.room, id, owner);
  }

  /** Gives every planet in a view its holder's name, keeping the letter. */
  private named<T extends { planets: PlanetView[] }>(v: T): T {
    return {
      ...v,
      planets: v.planets.map((p) => ({ ...p, letter: p.letter ?? p.name, name: planetName(this.room, p.id, p.owner) })),
    };
  }

  join(room: string, name: string) {
    this.room = room.toUpperCase();
    this.name = name;
    this.fatal = "";
    this.connect();
  }

  private connect() {
    const proto = location.protocol === "https:" ? "wss" : "ws";
    const ws = new WebSocket(`${proto}://${location.host}/ws?room=${encodeURIComponent(this.room)}`);
    this.ws = ws;
    this.conn = "connecting";
    ws.onopen = () => {
      this.conn = "open";
      this.retry = 0;
      const token = load(tokenKey(this.room));
      this.send({ type: "join", name: this.name, ...(token ? { token } : {}) });
    };
    ws.onmessage = (e) => this.handle(JSON.parse(e.data) as ServerMsg);
    ws.onclose = () => {
      if (this.ws !== ws) return;
      this.conn = "closed";
      this.ws = null;
      if (this.fatal) return;
      // Back off up to 10 s between reconnect attempts.
      const delay = Math.min(10000, 500 * 2 ** this.retry++);
      setTimeout(() => this.reconnect(), delay);
    };
  }

  /**
   * Connects again unless the room is gone: rooms live in the server's
   * memory, so a restart or 10 idle minutes ends them.
   */
  private async reconnect() {
    try {
      const res = await fetch(`/api/rooms/${encodeURIComponent(this.room)}`);
      if (res.status === 404) {
        this.fatal = `Room ${this.room} has ended. Rooms close after 10 minutes with nobody in them.`;
        save(tokenKey(this.room), null);
        return;
      }
    } catch {
      // The server is unreachable; keep trying.
    }
    this.connect();
  }

  send(msg: ClientMsg) {
    if (this.ws?.readyState === WebSocket.OPEN) this.ws.send(JSON.stringify(msg));
  }

  showToast(text: string) {
    this.toast = text;
    clearTimeout(this.toastTimer);
    this.toastTimer = window.setTimeout(() => (this.toast = ""), 4000);
  }

  private addLog(round: number, text: string, tone: LogEntry["tone"] = "info") {
    // A watcher has no side, so nothing is good or bad news.
    if (this.watching) tone = "info";
    this.log.push({ id: this.logId++, round, text, tone });
  }

  /** Forgets the last game, for the next one in this room. */
  private clearGame() {
    this.view = null;
    this.round = 0;
    this.active = -1;
    this.deadline = 0;
    this.log = [];
    this.standings = null;
    this.winner = -1;
    this.lastEvents = null;
    this.activity = {};
    clearTimeout(this.replayFallback);
    this.replayPending = false;
    this.pendingRound = 0;
    this.afterReplay = null;
  }

  /** The host takes the room back to the lobby for another game. */
  playAgain() {
    this.send({ type: "new_game" });
  }

  /** Leaves this room for the start page, to create or join another. */
  leave() {
    location.href = "/";
  }

  private handle(m: ServerMsg) {
    switch (m.type) {
      case "welcome":
        this.seat = m.seat;
        this.joined = true;
        this.hosting = m.host;
        this.token = m.token;
        save(tokenKey(m.room), m.token);
        break;
      case "lobby":
        // Back in the lobby after a game: the host is setting up another.
        if (m.phase === "lobby" && this.phase !== "lobby") this.clearGame();
        this.phase = m.phase;
        this.host = m.host;
        this.seats = m.seats;
        this.settings = m.settings;
        this.mapPreview = m.mapPreview ? this.named({ planets: m.mapPreview }).planets : null;
        this.aiKinds = m.aiKinds;
        break;
      case "your_turn":
        if (this.active !== this.seat || this.round !== m.round) play("turn");
        this.view = this.named(m.view);
        this.round = m.round;
        this.active = this.seat;
        this.deadline = m.deadline ?? 0;
        break;
      case "player_turn":
        this.view = this.named(m.view);
        this.round = m.round;
        this.active = m.seat;
        this.deadline = m.deadline ?? 0;
        break;
      case "fleet_ack": {
        this.view = this.named(m.view);
        const f = m.view.fleets.find((f) => f.id === m.fleetId);
        play("launch");
        if (f) {
          this.noteActivity(f.from, this.round);
          this.addLog(
            this.round,
            `Fleet of ${f.ships} sent from ${this.planetName(f.from)} to ${this.planetName(f.to)}, arriving in round ${f.arrivalRound}.`,
          );
        }
        break;
      }
      case "round_result":
        this.view = this.named(m.view);
        for (const e of m.events) {
          this.addEvent(e);
          this.noteActivity(e.planet, m.round);
        }
        // A replaying board plays each battle's sound as its battle ends.
        if (!this.playsBattles) play(...m.events.map((e) => soundFor(e, this.seat)).filter((s): s is Sound => s !== null));
        this.lastEvents = { round: m.round, events: m.events };
        if (this.playsBattles) {
          this.replayPending = true;
          this.pendingRound = m.round;
          // Never wait forever, say if the 3D view failed to load.
          clearTimeout(this.replayFallback);
          this.replayFallback = window.setTimeout(() => this.replayDone(), 60_000);
        }
        break;
      case "seat_status":
        if (this.seats[m.seat]) this.seats[m.seat].connected = m.connected;
        break;
      case "game_over":
        if (this.phase !== "over") {
          const sound = () => play(m.winner === this.seat ? "win" : "gameover");
          if (this.replayPending) this.afterReplay = sound;
          else sound();
        }
        this.standings = m.standings;
        this.winner = m.winner;
        this.phase = "over";
        this.deadline = 0;
        break;
      case "error":
        this.onError(m.code, m.message);
        break;
    }
  }

  private onError(code: string, message: string) {
    switch (code) {
      case "replaced":
        this.fatal = "This seat was opened in another window.";
        break;
      case "removed":
        this.fatal = "The host removed you from the room.";
        save(tokenKey(this.room), null);
        break;
      case "bad_token":
        // The seat is gone. In the lobby we can join fresh; mid-game we can't.
        save(tokenKey(this.room), null);
        if (this.phase === "lobby" && !this.joined) {
          this.send({ type: "join", name: this.name });
        } else {
          this.fatal = "Your seat is no longer in this game.";
        }
        break;
      case "wrong_phase":
        if (!this.joined) this.fatal = "This game has already started.";
        else this.showToast(message);
        break;
      case "room_full":
        this.fatal = "This room is full.";
        break;
      default:
        this.showToast(message);
    }
    if (this.fatal) this.ws?.close();
  }

  private addEvent(e: GameEvent) {
    const d = this.describe(e, this.seat);
    if (d) this.addLog(e.round, d.text, d.tone);
  }

  /** An event as a log line, from seat's side of it. */
  describe(e: GameEvent, me: number): { text: string; tone: LogEntry["tone"] } | null {
    // A planet is named by whoever held it when the event happened.
    const planet = e.planet >= 0 ? this.planetNameFor(e.planet, e.defender) : "";
    const who = (seat: number) => (seat === NEUTRAL ? "neutral" : seat === me ? "you" : this.playerName(seat));
    switch (e.kind) {
      case "reinforced":
        return { text: `Reinforcements have arrived at ${planet}: ${e.ships} ships, now ${e.remaining}.`, tone: "info" };
      case "captured":
        if (e.attacker === me) {
          return {
            text: `${planet} has fallen to you (from ${who(e.defender)}) and is now ${this.planetNameFor(e.planet, e.attacker)}. ${e.remaining} of ${e.ships} ships survived.`,
            tone: "good",
          };
        }
        return {
          text: `${planet} has fallen to ${who(e.attacker)}${me === SPECTATOR ? ` (from ${who(e.defender)})` : ""} and is now ${this.planetNameFor(e.planet, e.attacker)}.`,
          tone: me === SPECTATOR ? "info" : "bad",
        };
      case "held":
        if (e.attacker === me) {
          return { text: `${planet} has held against your attack: ${e.ships} ships lost, ${e.defenderLost} defenders destroyed.`, tone: "bad" };
        }
        return {
          text: `${planet}${me === SPECTATOR ? ` (${who(e.defender)})` : ""} has held against an attack from ${who(e.attacker)}: ${e.ships} attackers destroyed, ${e.remaining} defenders left.`,
          tone: me === SPECTATOR ? "info" : "good",
        };
      case "eliminated":
        return { text: e.attacker === me ? "You have been eliminated." : `${who(e.attacker)} has been eliminated.`, tone: e.attacker === me ? "bad" : "info" };
      case "game_over":
        return {
          text: e.attacker < 0 ? "The game is over. No winner." : `The game is over. ${e.attacker === me ? "You win!" : who(e.attacker) + " wins."}`,
          tone: "info",
        };
    }
    return null;
  }
}

export const store = new Store();

/** The sound for an event, from seat's side of it. */
export function soundFor(e: GameEvent, seat: number): Sound | null {
  if (seat === SPECTATOR) {
    // A watcher hears every battle the same way.
    return e.kind === "captured" ? "captured" : e.kind === "held" ? "held" : e.kind === "eliminated" ? "eliminated" : null;
  }
  switch (e.kind) {
    case "reinforced":
      return "reinforced";
    case "captured":
      return e.attacker === seat ? "captured" : e.defender === seat ? "lost" : null;
    case "held":
      return e.attacker === seat ? "repelled" : e.defender === seat ? "held" : null;
    case "eliminated":
      return "eliminated";
  }
  return null;
}
