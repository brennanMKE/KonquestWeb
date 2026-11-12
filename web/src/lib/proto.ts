// Wire types. These mirror internal/proto and internal/rules in Go.

export const NEUTRAL = -1;
/** The seat of someone watching rather than playing. */
export const SPECTATOR = -2;

export interface Pos {
  x: number;
  y: number;
  z: number;
}

export interface Settings {
  width: number;
  height: number;
  depth: number;
  neutrals: number;
  shipSpeed: number;
  blindMap: boolean;
  neutralStats: boolean;
  captureProducesImmediately: boolean;
  maxRounds: number;
  aiDelayMs: number;
  turnTimerMs: number;
  disconnectGraceMs: number;
}

export interface PlanetView {
  id: number;
  /** The display name, set by the client from the holder's faction. */
  name: string;
  /** The server's letter name, kept as the keyboard shortcut. */
  letter?: string;
  pos: Pos;
  owner: number;
  ships?: number;
  production?: number;
  kill?: number;
}

export interface Fleet {
  id: number;
  owner: number;
  from: number;
  to: number;
  ships: number;
  kill: number;
  launchRound: number;
  arrivalRound: number;
}

export interface Stats {
  shipsBuilt: number;
  shipsDestroyed: number;
  shipsLost: number;
  planetsConquered: number;
  fleetsLaunched: number;
}

export interface View {
  seat: number;
  round: number;
  active: number;
  settings: Omit<Settings, "aiDelayMs" | "turnTimerMs" | "disconnectGraceMs">;
  planets: PlanetView[];
  fleets: Fleet[];
  players: { seat: number; alive: boolean }[];
  stats: Stats;
  over: boolean;
  winner: number;
}

export type EventKind = "reinforced" | "captured" | "held" | "eliminated" | "fallen" | "risen" | "game_over";

export interface GameEvent {
  kind: EventKind;
  round: number;
  planet: number;
  attacker: number;
  defender: number;
  ships: number;
  attackerLost: number;
  defenderLost: number;
  remaining: number;
}

export interface Standing {
  seat: number;
  rank: number;
  planets: number;
  ships: number;
  alive: boolean;
  stats: Stats;
}

/** The whole game after one round, for watching it back. */
export interface Frame {
  round: number;
  launched: Fleet[];
  events: GameEvent[];
  view: View;
  standings: Standing[];
}

export interface SavedSeat {
  name: string;
  kind: "human" | "ai";
  aiKind?: string;
}

/** A whole game: frame 0 is the start, then one frame per round. */
export interface Replay {
  room: string;
  settings: Settings;
  seats: SavedSeat[];
  frames: Frame[];
  over: boolean;
  winner: number;
}

export interface SeatInfo {
  seat: number;
  name: string;
  kind: "human" | "ai";
  aiKind?: string;
  connected: boolean;
}

export type Phase = "lobby" | "playing" | "over";

export type ServerMsg =
  | { type: "welcome"; seat: number; token: string; room: string; host: boolean }
  | {
      type: "lobby";
      room: string;
      phase: Phase;
      host: number;
      hostName: string;
      seats: SeatInfo[];
      settings: Settings;
      mapPreview: PlanetView[] | null;
      aiKinds: string[];
    }
  | { type: "your_turn"; round: number; view: View; deadline?: number }
  | { type: "player_turn"; round: number; seat: number; name: string; view: View; deadline?: number }
  | { type: "fleet_ack"; fleetId: number; arrivalRound: number; view: View }
  | { type: "error"; code: string; message: string }
  | { type: "seat_status"; seat: number; connected: boolean }
  | { type: "round_result"; round: number; events: GameEvent[]; view: View }
  | { type: "game_over"; winner: number; standings: Standing[] };

export type ClientMsg =
  | { type: "join"; name: string; token?: string }
  | { type: "set_settings"; settings: Settings }
  | { type: "add_ai"; kind?: string }
  | { type: "new_game" }
  | { type: "remove_seat"; seat: number }
  | { type: "replace_with_ai"; seat: number; kind?: string }
  | { type: "reroll_map" }
  | { type: "watch"; watch: boolean }
  | { type: "start_game" }
  | { type: "send_fleet"; from: number; to: number; ships: number }
  | { type: "end_turn" };

/** Euclidean distance between cells. */
export function distance(a: Pos, b: Pos): number {
  return Math.hypot(a.x - b.x, a.y - b.y, a.z - b.z);
}

/** Rounds a fleet takes between two cells; matches rules.TravelTurns. */
export function travelTurns(a: Pos, b: Pos, speed: number): number {
  return Math.max(1, Math.ceil(distance(a, b) / speed - 1e-9));
}

/** Names of the computer player kinds, for menus. */
export const AI_NAMES: Record<string, string> = {
  planner: "Planner (rules)",
  greedy: "Greedy (simple)",
  system1: "System 1 model",
};

/** Short name of who plays a seat: a person, or the kind of computer. */
export function playerType(seat: { kind: string; aiKind?: string } | undefined): string {
  if (!seat) return "";
  if (seat.kind !== "ai") return "Person";
  return AI_NAMES[seat.aiKind ?? ""]?.replace(/ \(.*\)$/, "").replace(/ model$/, "") ?? "Computer";
}
