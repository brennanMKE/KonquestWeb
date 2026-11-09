// Client state and the WebSocket connection. The server is authoritative:
// this only records what the server says and sends intents.

import type { ClientMsg, GameEvent, Phase, PlanetView, SeatInfo, ServerMsg, Settings, Standing, View } from "./proto";
import { NEUTRAL } from "./proto";

export interface LogEntry {
  id: number;
  round: number;
  text: string;
  tone: "good" | "bad" | "info";
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

type ConnState = "connecting" | "open" | "closed";

class Store {
  room = $state("");
  conn = $state<ConnState>("closed");
  /** Set when the server ended the session for good (removed, replaced). */
  fatal = $state("");
  seat = $state(-1);
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

  private ws: WebSocket | null = null;
  private name = "";
  private retry = 0;
  private logId = 0;
  private toastTimer = 0;

  get isHost() {
    return this.seat >= 0 && this.seat === this.host;
  }
  get myTurn() {
    return this.phase === "playing" && this.active === this.seat;
  }

  seatName(seat: number): string {
    if (seat === NEUTRAL) return "neutral";
    if (seat === this.seat) return "you";
    return this.seats[seat]?.name ?? `Player ${seat + 1}`;
  }

  planetName(id: number): string {
    return this.view?.planets[id]?.name ?? "?";
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
      setTimeout(() => this.connect(), delay);
    };
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
    this.log.push({ id: this.logId++, round, text, tone });
  }

  private handle(m: ServerMsg) {
    switch (m.type) {
      case "welcome":
        this.seat = m.seat;
        this.token = m.token;
        save(tokenKey(m.room), m.token);
        break;
      case "lobby":
        this.phase = m.phase;
        this.host = m.host;
        this.seats = m.seats;
        this.settings = m.settings;
        this.mapPreview = m.mapPreview;
        this.aiKinds = m.aiKinds;
        break;
      case "your_turn":
        this.view = m.view;
        this.round = m.round;
        this.active = this.seat;
        this.deadline = m.deadline ?? 0;
        break;
      case "player_turn":
        this.view = m.view;
        this.round = m.round;
        this.active = m.seat;
        this.deadline = m.deadline ?? 0;
        break;
      case "fleet_ack": {
        this.view = m.view;
        const f = m.view.fleets.find((f) => f.id === m.fleetId);
        if (f) {
          this.addLog(
            this.round,
            `Fleet of ${f.ships} sent from ${this.planetName(f.from)} to ${this.planetName(f.to)}, arriving in round ${f.arrivalRound}.`,
          );
        }
        break;
      }
      case "round_result":
        this.view = m.view;
        for (const e of m.events) this.addEvent(e);
        break;
      case "seat_status":
        if (this.seats[m.seat]) this.seats[m.seat].connected = m.connected;
        break;
      case "game_over":
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
        if (this.phase === "lobby" && this.seat < 0) {
          this.send({ type: "join", name: this.name });
        } else {
          this.fatal = "Your seat is no longer in this game.";
        }
        break;
      case "wrong_phase":
        if (this.seat < 0) this.fatal = "This game has already started.";
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
    const planet = `Planet ${this.planetName(e.planet)}`;
    const me = this.seat;
    switch (e.kind) {
      case "reinforced":
        this.addLog(e.round, `Reinforcements have arrived at ${planet}: ${e.ships} ships, now ${e.remaining}.`, "info");
        break;
      case "captured":
        if (e.attacker === me) {
          this.addLog(
            e.round,
            `${planet} has fallen to you (from ${this.seatName(e.defender)}). ${e.remaining} of ${e.ships} ships survived.`,
            "good",
          );
        } else {
          this.addLog(e.round, `${planet} has fallen to ${this.seatName(e.attacker)}.`, "bad");
        }
        break;
      case "held":
        if (e.attacker === me) {
          this.addLog(e.round, `${planet} has held against your attack: ${e.ships} ships lost, ${e.defenderLost} defenders destroyed.`, "bad");
        } else {
          this.addLog(
            e.round,
            `${planet} has held against an attack from ${this.seatName(e.attacker)}: ${e.ships} attackers destroyed, ${e.remaining} defenders left.`,
            "good",
          );
        }
        break;
      case "eliminated":
        this.addLog(e.round, e.attacker === me ? "You have been eliminated." : `${this.seatName(e.attacker)} has been eliminated.`, e.attacker === me ? "bad" : "info");
        break;
      case "game_over":
        this.addLog(e.round, e.attacker < 0 ? "The game is over. No winner." : `The game is over. ${e.attacker === me ? "You win!" : this.seatName(e.attacker) + " wins."}`);
        break;
    }
  }
}

export const store = new Store();
