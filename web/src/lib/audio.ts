// Sound effects, synthesized with the Web Audio API from oscillators and
// filtered noise; there are no audio files. The primitives follow System 1
// Arcade's audio module (same author, MIT).

const STORE_KEY = "parallax.sound";
const MAX_AT_ONCE = 4;

let enabled = true;
try {
  enabled = localStorage.getItem(STORE_KEY) !== "off";
} catch {
  // Storage unavailable: sound stays on for this visit.
}

let ac: AudioContext | null = null;
let out: GainNode | null = null;
let noiseBuf: AudioBuffer | null = null;

// --- Status ------------------------------------------------------------------

/**
 * What the page knows about sound. It can tell whether the browser lets it
 * play, not whether anyone hears it (volume, a muted tab, the output device).
 * - off: the player turned it off
 * - waiting: no click or key press yet, which browsers require first
 * - running: the browser is playing what we send
 * - blocked: a click or key press came, and the browser still won't play
 * - unsupported: no Web Audio here
 */
export type AudioStatus = "off" | "waiting" | "running" | "blocked" | "unsupported";

let unsupported = false;
let gestured = false;
let lastError = "";
const listeners = new Set<(s: AudioStatus) => void>();

export function audioStatus(): AudioStatus {
  if (!enabled) return "off";
  if (unsupported) return "unsupported";
  if (ac?.state === "running") return "running";
  return gestured ? "blocked" : "waiting";
}

/** Why sound is blocked, when the browser said. */
export const audioError = () => lastError;

/** Calls f with the status now and on every change; returns an unsubscribe. */
export function onAudioStatus(f: (s: AudioStatus) => void): () => void {
  listeners.add(f);
  f(audioStatus());
  return () => listeners.delete(f);
}

let reported: AudioStatus | null = null;
function notify() {
  const s = audioStatus();
  if (s === reported) return;
  reported = s;
  if (s === "blocked") console.warn(`Parallax: the browser isn't playing sound (audio ${ac?.state ?? "not created"})${lastError ? `: ${lastError}` : ""}`);
  if (s === "unsupported") console.warn(`Parallax: no sound: ${lastError || "this browser has no Web Audio"}`);
  for (const f of listeners) f(s);
}

export const soundOn = () => enabled;

export function setSound(on: boolean) {
  enabled = on;
  try {
    localStorage.setItem(STORE_KEY, on ? "on" : "off");
  } catch {
    // Not remembered; fine.
  }
  if (on) unlockAudio();
  else if (ac?.state === "running") ac.suspend().catch(() => {});
  notify();
}

/**
 * Creates or resumes the AudioContext. Browsers keep it suspended until a
 * user gesture, so this runs on the first key press or click. Resolves once
 * the browser has answered, with the status then.
 */
export function unlockAudio(e?: Event | "retry"): Promise<AudioStatus> {
  if (!enabled) return Promise.resolve(audioStatus());
  // Only a click or key press counts toward "blocked": not a sound retrying,
  // not a tab coming back.
  if (e !== "retry" && e?.type !== "visibilitychange") gestured = true;
  // A closed context can't come back; start a new one.
  if (ac?.state === "closed") ac = null;
  if (!ac) {
    const AC = window.AudioContext ?? (window as unknown as { webkitAudioContext?: typeof AudioContext }).webkitAudioContext;
    if (!AC) {
      unsupported = true;
      notify();
      return Promise.resolve(audioStatus());
    }
    try {
      ac = new AC();
    } catch (err) {
      unsupported = true;
      lastError = `couldn't start audio: ${(err as Error).message}`;
      notify();
      return Promise.resolve(audioStatus());
    }
    ac.addEventListener("statechange", notify);
    const comp = ac.createDynamicsCompressor();
    comp.connect(ac.destination);
    out = ac.createGain();
    out.gain.value = 0.35;
    out.connect(comp);
    noiseBuf = ac.createBuffer(1, ac.sampleRate, ac.sampleRate);
    const d = noiseBuf.getChannelData(0);
    for (let i = 0; i < d.length; i++) d[i] = Math.random() * 2 - 1;
  }
  // Suspended, or "interrupted" (Safari, after sleep or an output device
  // change): resume. Anything but running needs it. Some browsers never
  // settle the promise when they refuse, so don't wait more than a second.
  if (ac.state === "running") {
    notify();
    return Promise.resolve(audioStatus());
  }
  const ctx = ac;
  return Promise.race([
    ctx.resume().then(
      () => (lastError = ""),
      (err: Error) => (lastError = err.message),
    ),
    new Promise((r) => setTimeout(r, 1000)),
  ]).then(() => {
    notify();
    return audioStatus();
  });
}

/** Turns sound on if need be and plays a short tone, to check it's heard. */
export async function testSound(): Promise<AudioStatus> {
  if (!enabled) setSound(true);
  const s = await unlockAudio();
  if (s === "running") play("turn");
  return s;
}

// Browsers suspend audio in a hidden tab or across sleep. Coming back to
// the tab resumes it, once a gesture has allowed sound at all.
if (typeof document !== "undefined") {
  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "visible" && ac && enabled) unlockAudio(new Event("visibilitychange"));
  });
  // iOS Safari doesn't count pointerdown (a touch starting) as a gesture
  // that may start audio; touchend and click are. Listen on every page,
  // not only the game's, so any tap or key press turns sound on.
  for (const type of ["touchend", "click", "keydown"]) {
    document.addEventListener(type, unlockAudio, { capture: true, passive: true });
  }
}

// --- Primitives --------------------------------------------------------------

// env shapes a gain node: a quick attack to vol, then an exponential decay.
function env(t: number, dur: number, vol: number, attack = 0.004): GainNode {
  const g = ac!.createGain();
  g.gain.setValueAtTime(0.0001, t);
  g.gain.linearRampToValueAtTime(vol, t + attack);
  g.gain.exponentialRampToValueAtTime(0.0001, t + dur);
  g.connect(out!);
  return g;
}

interface ToneOpts {
  type?: OscillatorType;
  vol?: number;
  to?: number;
  delay?: number;
  lowpass?: number;
  attack?: number;
}

// tone plays one note, optionally sliding to another pitch.
function tone(freq: number, dur: number, { type = "square", vol = 0.15, to, delay = 0, lowpass, attack }: ToneOpts = {}) {
  const t = ac!.currentTime + delay;
  const o = ac!.createOscillator();
  o.type = type;
  o.frequency.setValueAtTime(freq, t);
  if (to) o.frequency.exponentialRampToValueAtTime(to, t + dur);
  let node: AudioNode = env(t, dur, vol, attack);
  if (lowpass) {
    const f = ac!.createBiquadFilter();
    f.type = "lowpass";
    f.frequency.value = lowpass;
    f.connect(node);
    node = f;
  }
  o.connect(node);
  o.start(t);
  o.stop(t + dur + 0.02);
}

interface NoiseOpts {
  vol?: number;
  type?: BiquadFilterType;
  freq?: number;
  to?: number;
  q?: number;
  delay?: number;
  attack?: number;
}

// noise plays filtered white noise; the filter can sweep from freq to `to`.
function noise(dur: number, { vol = 0.2, type = "lowpass", freq = 2000, to, q = 1, delay = 0, attack }: NoiseOpts = {}) {
  const t = ac!.currentTime + delay;
  const s = ac!.createBufferSource();
  s.buffer = noiseBuf;
  const f = ac!.createBiquadFilter();
  f.type = type;
  f.Q.value = q;
  f.frequency.setValueAtTime(freq, t);
  if (to) f.frequency.exponentialRampToValueAtTime(to, t + dur);
  s.connect(f);
  f.connect(env(t, dur, vol, attack));
  s.start(t, Math.random() * 0.5);
  s.stop(t + dur + 0.02);
}

// notes plays a sequence of pitches, one every step seconds (0 rests).
function notes(freqs: number[], step: number, opts: ToneOpts & { len?: number } = {}) {
  freqs.forEach((f, i) => {
    if (f) tone(f, opts.len ?? step * 0.9, { ...opts, delay: (opts.delay ?? 0) + i * step });
  });
}

// Note frequencies (Hz).
const A3 = 220.0,
  C4 = 261.6,
  E4 = 329.6,
  G4 = 392.0,
  A4 = 440.0,
  C5 = 523.3,
  D5 = 587.3,
  E5 = 659.3,
  G5 = 784.0,
  C6 = 1046.5;

// --- Sounds ------------------------------------------------------------------

const SOUNDS = {
  /** It's your turn: a bright two-note chime. */
  turn: () => notes([G5, C6], 0.09, { type: "triangle", vol: 0.18, len: 0.25 }),
  /** A fleet leaves: an engine whoosh rising away. */
  launch: () => {
    noise(0.45, { vol: 0.16, type: "bandpass", freq: 300, to: 2400, q: 1.4, attack: 0.08 });
    tone(140, 0.4, { type: "sawtooth", vol: 0.04, to: 420, lowpass: 1200, attack: 0.06 });
  },
  /** End turn: a soft click. */
  end: () => tone(660, 0.04, { type: "triangle", vol: 0.1, to: 440 }),
  /** Ships join one of your planets. */
  reinforced: () => notes([C5, E5], 0.06, { type: "triangle", vol: 0.12 }),
  /** One exchange of fire: a short crackle. Used by the battle animation. */
  shot: () => {
    noise(0.08, { vol: 0.12, type: "highpass", freq: 2500 });
    tone(900, 0.06, { type: "square", vol: 0.03, to: 300 });
  },
  /** A ship destroyed: a dull pop. */
  boom: () => {
    noise(0.25, { vol: 0.22, freq: 1200, to: 120 });
    tone(110, 0.2, { type: "triangle", vol: 0.12, to: 45 });
  },
  /** You took a planet: a rising fanfare. */
  captured: () => {
    notes([C5, E5, G5, C6], 0.08, { vol: 0.08 });
    notes([C4, E4, G4, C5], 0.08, { type: "triangle", vol: 0.18 });
  },
  /** One of your planets held against an attack: a shield ring. */
  held: () => {
    tone(A4, 0.5, { type: "sine", vol: 0.15 });
    tone(A4 * 1.5, 0.5, { type: "sine", vol: 0.08, delay: 0.04 });
  },
  /** Your attack failed. */
  repelled: () => {
    noise(0.3, { vol: 0.18, type: "bandpass", freq: 600, to: 150, q: 1.2 });
    tone(196, 0.35, { type: "triangle", vol: 0.15, to: 98 });
  },
  /** You lost a planet: falling tones over a rumble. */
  lost: () => {
    noise(0.7, { vol: 0.25, freq: 900, to: 60 });
    notes([E4, C4, A3], 0.14, { type: "sawtooth", vol: 0.07, lowpass: 1400, len: 0.18 });
  },
  /** A player is eliminated: a low warble. */
  eliminated: () => tone(160, 0.9, { type: "sawtooth", vol: 0.08, to: 40, lowpass: 700 }),
  /** You won the game. */
  win: () => {
    notes([C5, E5, G5, C6, 0, G5, C6], 0.11, { vol: 0.08 });
    notes([C4, G4, C5, E5, 0, D5, E5], 0.11, { type: "triangle", vol: 0.2 });
  },
  /** The game ended without you winning. */
  gameover: () => {
    const seq = [G4, E4, C4, 196.0];
    notes(seq, 0.16, { type: "triangle", vol: 0.3, len: 0.2 });
    notes(seq, 0.16, { type: "square", vol: 0.05, len: 0.14 });
  },
};

export type Sound = keyof typeof SOUNDS;

// Most important first: when more sounds are due than we play, the rest are
// dropped.
const PRIORITY: Sound[] = [
  "win",
  "gameover",
  "lost",
  "captured",
  "eliminated",
  "held",
  "repelled",
  "reinforced",
  "turn",
  "launch",
  "boom",
  "shot",
  "end",
];

/** Plays a set of sounds, the most important few at once. */
export function play(...names: Sound[]) {
  if (!enabled || !ac || names.length === 0) return;
  if (ac.state !== "running") {
    // Try to wake it (this works once a gesture has unlocked it before);
    // this sound is skipped, the next ones play.
    unlockAudio("retry");
    return;
  }
  const list = [...new Set(names)].sort((a, b) => PRIORITY.indexOf(a) - PRIORITY.indexOf(b));
  for (const n of list.slice(0, MAX_AT_ONCE)) {
    try {
      SOUNDS[n]();
    } catch {
      // A failed sound never breaks the game.
    }
  }
}
