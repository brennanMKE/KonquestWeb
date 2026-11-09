// Wire types. These mirror internal/proto and internal/rules in Go.

export const NEUTRAL = -1;

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
  name: string;
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

export type EventKind = "reinforced" | "captured" | "held" | "eliminated" | "game_over";

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

export interface SeatInfo {
  seat: number;
  name: string;
  kind: "human" | "ai";
  aiKind?: string;
  connected: boolean;
}

export type Phase = "lobby" | "playing" | "over";

export type ServerMsg =
  | { type: "welcome"; seat: number; token: string; room: string }
  | {
      type: "lobby";
      room: string;
      phase: Phase;
      host: number;
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
  | { type: "remove_seat"; seat: number }
  | { type: "replace_with_ai"; seat: number; kind?: string }
  | { type: "reroll_map" }
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
