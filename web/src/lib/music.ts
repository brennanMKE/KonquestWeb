// Background music that follows the fighting. One 38-second piece in
// D minor, written for Parallax, is rendered in code as three mixes of the
// same loop: calm (pads and bells), tense (adds a pulsing bass and a quiet
// arpeggio) and battle (adds drums, a driving arpeggio and brass stabs).
// All three play in step through <audio> elements, and their volumes
// crossfade as the game heats up: the more battles, and the more often,
// the more intense. Quiet rounds let it settle again.
//
// <audio> rather than Web Audio, because that's what plays reliably in
// Safari here (see docs/safari-audio.md). Off until the player turns it on.

import type { Fleet, GameEvent } from "./proto";
import { loadSetting, loadVolume, saveSetting, wavUrl } from "./wav";

const KEY = "parallax.music";
const VOLUME_KEY = "parallax.musicVolume";

let enabled = loadSetting(KEY, "off") === "on";
let volume = loadVolume(VOLUME_KEY, 0.5);

export const musicOn = () => enabled;
export const musicVolume = () => volume;

/** Turns music on or off. Call it from a click: browsers start playback only then. */
export function setMusic(on: boolean) {
  enabled = on;
  saveSetting(KEY, on ? "on" : "off");
  if (on) start();
  else pauseAll();
}

export function setMusicVolume(v: number) {
  volume = Math.max(0, Math.min(1, v));
  saveSetting(VOLUME_KEY, String(volume));
  applyVolumes();
}

// The slider's level, eased so the low end isn't all loud.
const master = () => 0.6 * volume * volume;

// --- Intensity -------------------------------------------------------------------

// 0 is calm, 0.5 tense, 1 battle. Each round moves it halfway to that
// round's heat, so one battle barely stirs it, while battles round after
// round build it up.
let intensity = 0;

/** The music's intensity, 0 to 1; for tests. */
export const musicIntensity = () => intensity;

/** Sets the intensity outright, as when a game starts or a replay jumps. */
export function setIntensity(v: number) {
  intensity = Math.max(0, Math.min(1, v));
  retarget();
}

/**
 * Feeds one round's results to the music. seat is the listener's seat, or
 * a negative number for a spectator; fleets are those still in flight.
 */
export function noteRound(events: GameEvent[], fleets: Fleet[], seat: number, owners: (planet: number) => number) {
  let battles = 0;
  let ships = 0;
  let heat = 0;
  for (const e of events) {
    if (e.kind === "captured" || e.kind === "held") {
      battles++;
      ships += e.ships;
    }
    if (e.kind === "fallen" || e.kind === "eliminated") heat = 1;
  }
  heat = Math.max(heat, 1 - Math.exp(-(battles * 0.6 + ships / 150)));
  if (seat >= 0) {
    // Enemy fleets on their way to us raise the tension too.
    const incoming = fleets.filter((f) => f.owner !== seat && owners(f.to) === seat).length;
    heat = Math.max(heat, Math.min(0.6, 0.2 * incoming));
  }
  if (events.some((e) => e.kind === "game_over")) {
    setIntensity(0.2);
    return;
  }
  setIntensity(0.5 * intensity + 0.5 * heat);
}

// --- Playback --------------------------------------------------------------------

const MIXES = 3;
let els: (HTMLAudioElement | null)[] = [null, null, null];
let weights = [1, 0, 0]; // current, faded toward target
let target = [1, 0, 0];
let rendering = false;
let fader = 0;
// iOS ignores an element's volume: there, only one mix plays at a time.
let volumeWorks = true;

function retarget() {
  const x = intensity * (MIXES - 1);
  target = target.map((_, i) => Math.max(0, 1 - Math.abs(x - i)));
  if (!musicPlaying()) weights = [...target];
  if (!fader && enabled) fader = window.setInterval(fade, 100);
}

// Fades take about four seconds; equal-power, so the sum doesn't dip.
function fade() {
  let moving = false;
  weights = weights.map((w, i) => {
    const d = target[i] - w;
    if (Math.abs(d) < 0.026) return target[i];
    moving = true;
    return w + Math.sign(d) * 0.025;
  });
  applyVolumes();
  if (!moving) {
    clearInterval(fader);
    fader = 0;
  }
}

function applyVolumes() {
  const lead = leader();
  els.forEach((el, i) => {
    if (!el) return;
    if (volumeWorks) {
      // A silent mix is realigned with the leader before it fades in.
      if (weights[i] === 0 && target[i] > 0 && lead && lead !== el) align(el, lead);
      el.volume = master() * Math.sin((weights[i] * Math.PI) / 2);
    } else {
      const want = target.indexOf(Math.max(...target)) === i;
      if (want && el.paused && enabled) {
        if (lead && lead !== el) align(el, lead);
        el.play().catch(() => {});
      } else if (!want && !el.paused) el.pause();
    }
  });
}

// The loudest playing mix, which the others follow.
function leader(): HTMLAudioElement | null {
  let best = -1;
  els.forEach((el, i) => {
    if (el && !el.paused && (best < 0 || weights[i] > weights[best])) best = i;
  });
  return best < 0 ? null : els[best];
}

function align(el: HTMLAudioElement, lead: HTMLAudioElement) {
  if (Math.abs(el.currentTime - lead.currentTime) > 0.05) el.currentTime = lead.currentTime;
}

function pauseAll() {
  for (const el of els) el?.pause();
}

function makeEl(url: string): HTMLAudioElement {
  const el = new Audio(url);
  el.loop = true;
  el.volume = 0;
  return el;
}

function start() {
  if (typeof Audio === "undefined") return;
  if (!els[0]) {
    // The calm mix first, inside the click, so music starts at once; the
    // others follow a moment later.
    const s = stems();
    els[0] = makeEl(wavUrl(mix(s, 0), RATE));
    const probe = els[0];
    probe.volume = 0.5;
    volumeWorks = Math.abs(probe.volume - 0.5) < 0.01;
    probe.volume = 0;
    if (!rendering) {
      rendering = true;
      setTimeout(() => {
        moreStems(s);
        for (let i = 1; i < MIXES; i++) els[i] = makeEl(wavUrl(mix(s, i), RATE));
        playAll();
      }, 50);
    }
  }
  playAll();
}

function playAll() {
  if (!enabled || document.visibilityState !== "visible") return;
  const lead = leader() ?? els[0];
  els.forEach((el, i) => {
    if (!el) return;
    if (!volumeWorks && target.indexOf(Math.max(...target)) !== i) return;
    if (lead && el !== lead && !lead.paused) align(el, lead);
    if (el.paused) el.play().catch((err: Error) => console.warn(`Parallax: the browser wouldn't play the music: ${err.message}`));
  });
  applyVolumes();
  // Keep the mixes in step: nudge any silent one that has drifted.
  clearInterval(syncer);
  syncer = window.setInterval(() => {
    const l = leader();
    els.forEach((el, i) => {
      if (el && l && el !== l && !el.paused && weights[i] < 0.05) align(el, l);
    });
  }, 2000);
}
let syncer = 0;

/** Whether music is playing; for tests. */
export const musicPlaying = () => els.some((el) => !!el && !el.paused);

// --- The score -------------------------------------------------------------------

const RATE = 22050;
const BPM = 100;
const BEAT = 60 / BPM;
const BAR = 4 * BEAT;
const BARS = 16;
const LEN = BARS * BAR; // 38.4 s
const N = Math.round(LEN * RATE);
const midi = (n: number) => 440 * 2 ** ((n - 69) / 12);

// Two bars each: Dm Bb F C, then Dm Bb Gm A, the A major pulling back home.
const CHORDS = [
  { notes: [62, 65, 69, 74], root: 38 },
  { notes: [62, 65, 70, 74], root: 34 },
  { notes: [60, 65, 69, 72], root: 41 },
  { notes: [60, 64, 67, 72], root: 36 },
  { notes: [62, 65, 69, 74], root: 38 },
  { notes: [62, 65, 70, 74], root: 34 },
  { notes: [62, 67, 70, 74], root: 43 },
  { notes: [61, 64, 69, 73], root: 33 },
];
const CHORD = 2 * BAR;
const BELLS = [74, 77, 79, 81, 84, 86]; // D minor pentatonic

const T = 4096;
const SINE = new Float32Array(T);
for (let i = 0; i < T; i++) SINE[i] = Math.sin((2 * Math.PI * i) / T);

type Stems = { pads: Float32Array; pulse?: Float32Array; drums?: Float32Array; lead?: Float32Array };

// Adds a voice into buf from `at` seconds for `dur`, wrapping around the
// loop's end so the loop has no seam. harmonics are amplitudes of 1x, 2x, …
function voice(
  buf: Float32Array,
  freq: number,
  at: number,
  dur: number,
  gain: number,
  env: (t: number) => number,
  harmonics: number[] = [1],
) {
  const start = Math.round(at * RATE);
  const count = Math.round(dur * RATE);
  const step = (freq * T) / RATE;
  const usable = harmonics.filter((_, k) => freq * (k + 1) < RATE / 2);
  let ph = 0;
  for (let i = 0; i < count; i++) {
    let v = 0;
    for (let k = 0; k < usable.length; k++) v += SINE[Math.floor(ph * (k + 1)) & (T - 1)] * usable[k];
    buf[(((start + i) % N) + N) % N] += v * gain * env(i / RATE);
    ph += step;
    if (ph >= T * 1e5) ph -= T * 1e5;
  }
}

let seed = 11;
const rand = () => (seed = (seed * 1103515245 + 12345) & 0x7fffffff) / 0x7fffffff;

function stems(): Stems {
  seed = 11;
  const dry = new Float32Array(N);
  // Pads: each chord swells in and out, overlapping its neighbours.
  const span = CHORD + 3;
  CHORDS.forEach((c, k) => {
    const at = k * CHORD - 1.5;
    const swell = (t: number) => Math.sin((Math.PI * t) / span) ** 2;
    c.notes.forEach((m, j) => {
      const f = midi(m);
      const trem = (t: number) => swell(t) * (0.85 + 0.15 * Math.sin(2 * Math.PI * (0.13 + j * 0.04) * t + j));
      voice(dry, f * 0.998, at, span, 0.045, trem);
      voice(dry, f * 1.002, at, span, 0.045, trem);
    });
    voice(dry, midi(c.root + 12), at, span, 0.07, swell);
  });
  // Bells, now and then.
  for (let t = BEAT; t < LEN; t += BEAT * 1.5) {
    if (rand() < 0.35) {
      const f = midi(BELLS[Math.floor(rand() * BELLS.length)]);
      const ring = (x: number) => Math.min(1, x / 0.01) * Math.exp(-x / 0.8);
      voice(dry, f, t, 3.5, 0.04, ring, [1, 0, 0.12]);
    }
  }
  return { pads: echo(dry) };
}

function moreStems(s: Stems) {
  // Pulse: the root in eighth notes, plucked, with a low tom on each bar.
  const pulse = new Float32Array(N);
  const pluck = (t: number) => Math.min(1, t / 0.004) * Math.exp(-t / 0.12);
  for (let i = 0; i < BARS * 8; i++) {
    const t = (i * BEAT) / 2;
    const c = CHORDS[Math.floor(t / CHORD)];
    const accent = i % 8 === 0 ? 1.3 : i % 2 === 0 ? 1 : 0.75;
    voice(pulse, midi(c.root), t, 0.3, 0.16 * accent, pluck, [1, 0.5, 0.25]);
  }
  for (let b = 0; b < BARS; b++) drum(pulse, b * BAR, "tom", 0.5);

  // Drums: kick on 1 and 3 (and the and of 4), snare on 2 and 4, hats.
  const drums = new Float32Array(N);
  for (let b = 0; b < BARS; b++) {
    const t0 = b * BAR;
    drum(drums, t0, "kick", 0.9);
    drum(drums, t0 + 2 * BEAT, "kick", 0.8);
    drum(drums, t0 + 3.5 * BEAT, "kick", 0.55);
    drum(drums, t0 + BEAT, "snare", 0.55);
    drum(drums, t0 + 3 * BEAT, "snare", 0.6);
    if (b % 4 === 3) for (let k = 0; k < 4; k++) drum(drums, t0 + 3 * BEAT + (k * BEAT) / 4, "snare", 0.25 + k * 0.08);
    for (let k = 0; k < 16; k++) drum(drums, t0 + (k * BEAT) / 4, "hat", k % 4 === 2 ? 0.3 : 0.15);
  }

  // Lead: a sixteenth-note arpeggio of the chord, and brass stabs on each chord.
  const lead = new Float32Array(N);
  const tick = (t: number) => Math.min(1, t / 0.003) * Math.exp(-t / 0.09);
  for (let i = 0; i < BARS * 16; i++) {
    const t = (i * BEAT) / 4;
    const c = CHORDS[Math.floor(t / CHORD)];
    const order = [0, 1, 2, 3, 2, 1, 3, 2];
    voice(lead, midi(c.notes[order[i % 8]] + 12), t, 0.15, 0.05, tick, [1, 0.45, 0.3, 0.2]);
  }
  CHORDS.forEach((c, k) => {
    for (const at of [k * CHORD, k * CHORD + BAR + 2.5 * BEAT]) {
      const stab = (t: number) => Math.min(1, t / 0.02) * Math.exp(-t / 0.35);
      for (const m of c.notes.slice(0, 3)) voice(lead, midi(m - 12), at, 0.9, 0.05, stab, [1, 0.7, 0.5, 0.35, 0.25, 0.15]);
    }
  });
  Object.assign(s, { pulse, drums, lead: lowpass(lead, 0.5) });
}

function drum(buf: Float32Array, at: number, kind: "kick" | "snare" | "hat" | "tom", gain: number) {
  const start = Math.round(at * RATE);
  const dur = kind === "hat" ? 0.06 : kind === "snare" ? 0.25 : 0.4;
  const count = Math.round(dur * RATE);
  let ph = 0;
  let prev = 0;
  for (let i = 0; i < count; i++) {
    const t = i / RATE;
    let v = 0;
    if (kind === "kick" || kind === "tom") {
      const f = kind === "kick" ? 45 + 90 * Math.exp(-t * 35) : 75 + 40 * Math.exp(-t * 20);
      ph += (f * T) / RATE;
      v = SINE[Math.floor(ph) & (T - 1)] * Math.exp(-t * (kind === "kick" ? 9 : 7));
    } else {
      const noise = rand() * 2 - 1;
      if (kind === "hat") {
        v = (noise - prev) * 0.5 * Math.exp(-t * 70);
      } else {
        ph += (190 * T) / RATE;
        v = noise * 0.6 * Math.exp(-t * 16) + SINE[Math.floor(ph) & (T - 1)] * 0.4 * Math.exp(-t * 25);
      }
      prev = noise;
    }
    buf[(start + i) % N] += v * gain;
  }
}

// A spacious echo: two feedback delays, run around the loop twice so the
// tails at the end carry into the start.
function echo(dry: Float32Array): Float32Array {
  const out = Float32Array.from(dry);
  for (const d of [Math.round(0.36 * RATE), Math.round(0.6 * RATE)]) {
    const buf = new Float32Array(N);
    for (let i = 0; i < 2 * N; i++) {
      const j = i % N;
      buf[j] = dry[j] + 0.4 * buf[(j - d + N) % N];
    }
    for (let j = 0; j < N; j++) out[j] += 0.35 * (buf[j] - dry[j]);
  }
  return lowpass(out, 0.3);
}

// A one-pole low-pass, warmed up over the loop's end for a seamless start.
function lowpass(x: Float32Array, a: number): Float32Array {
  const out = new Float32Array(N);
  let lp = 0;
  for (let i = N - RATE; i < 2 * N; i++) {
    const j = i % N;
    lp += a * (x[j] - lp);
    if (i >= N) out[j] = lp;
  }
  return out;
}

// The three mixes, each a little louder than the last.
function mix(s: Stems, which: number): Float32Array {
  const out = new Float32Array(N);
  const parts: [Float32Array | undefined, number][] =
    which === 0
      ? [[s.pads, 1]]
      : which === 1
        ? [[s.pads, 0.9], [s.pulse, 1], [s.lead, 0.3]]
        : [[s.pads, 0.75], [s.pulse, 1], [s.drums, 1], [s.lead, 1]];
  for (const [p, g] of parts) if (p) for (let i = 0; i < N; i++) out[i] += p[i] * g;
  // Levelled by loudness, not peaks, so the drums don't make the battle
  // mix quieter; a soft limiter rounds off the loudest hits.
  let sum = 0;
  for (let i = 0; i < N; i++) sum += out[i] * out[i];
  const rms = Math.sqrt(sum / N);
  const scale = rms > 0 ? [0.1, 0.13, 0.17][which] / rms : 1;
  for (let i = 0; i < N; i++) out[i] = 0.95 * Math.tanh((out[i] * scale) / 0.95);
  return out;
}

// Music that was on last visit starts with the first click or key press;
// browsers won't start it before one. It pauses while the tab is hidden.
if (typeof document !== "undefined") {
  const resume = () => {
    if (enabled && !musicPlaying() && document.visibilityState === "visible") start();
  };
  for (const t of ["click", "keydown", "touchend"]) document.addEventListener(t, resume, { capture: true, passive: true });
  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "hidden") pauseAll();
    else if (enabled && els[0]) playAll();
  });
}

/** Renders the three mixes, calm to battle; for listening tests. */
export function renderMixes(): Float32Array[] {
  const s = stems();
  moreStems(s);
  return [0, 1, 2].map((i) => mix(s, i));
}
export const MUSIC_RATE = RATE;
