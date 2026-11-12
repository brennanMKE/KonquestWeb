// Background music: a slow ambient loop of soft chords, a low drone and
// sparse bells, written for Parallax. It's generated in code once, the
// first time music is turned on, and loops through an <audio> element,
// apart from the sound effects. Off until the player turns it on.

import { loadSetting, loadVolume, saveSetting, wavUrl } from "./wav";

const KEY = "parallax.music";
const VOLUME_KEY = "parallax.musicVolume";

let enabled = loadSetting(KEY, "off") === "on";
let volume = loadVolume(VOLUME_KEY, 0.5);
let el: HTMLAudioElement | null = null;

export const musicOn = () => enabled;
export const musicVolume = () => volume;

/** Turns music on or off. Call it from a click: browsers start playback only then. */
export function setMusic(on: boolean) {
  enabled = on;
  saveSetting(KEY, on ? "on" : "off");
  if (on) start();
  else el?.pause();
}

export function setMusicVolume(v: number) {
  volume = Math.max(0, Math.min(1, v));
  saveSetting(VOLUME_KEY, String(volume));
  if (el) el.volume = level();
}

// The slider's level, eased so the low end isn't all loud.
const level = () => 0.6 * volume * volume;

function start() {
  if (typeof Audio === "undefined") return;
  if (!el) {
    el = new Audio(render());
    el.loop = true;
  }
  el.volume = level();
  if (el.paused) el.play().catch((err: Error) => console.warn(`Parallax: the browser wouldn't play the music: ${err.message}`));
}

/** Whether the music element is playing; for tests. */
export const musicPlaying = () => !!el && !el.paused;

// --- The track -----------------------------------------------------------------

const RATE = 22050;
const CHORD = 12; // seconds per chord
const midi = (n: number) => 440 * 2 ** ((n - 69) / 12);
// A minor: Am(add9), Fmaj7, C/G, Em7, each with its root an octave down.
const CHORDS = [
  { notes: [57, 60, 64, 71], root: 45 },
  { notes: [53, 57, 60, 64], root: 41 },
  { notes: [55, 60, 64, 71], root: 43 },
  { notes: [52, 55, 59, 62], root: 40 },
];
const BELLS = [69, 72, 74, 76, 79, 81]; // A minor pentatonic

function render(): string {
  const len = CHORDS.length * CHORD;
  const n = len * RATE;
  const dry = new Float32Array(n);
  // A sine table, stepped through by phase: much faster than Math.sin.
  const T = 4096;
  const table = new Float32Array(T);
  for (let i = 0; i < T; i++) table[i] = Math.sin((2 * Math.PI * i) / T);

  // One voice from `at` seconds for `dur`, wrapping around the loop's end
  // so the loop has no seam. env gives its level at time t into the note.
  const voice = (freq: number, at: number, dur: number, gain: number, env: (t: number) => number) => {
    const start = Math.round(at * RATE);
    const count = Math.round(dur * RATE);
    const step = (freq * T) / RATE;
    let ph = 0;
    for (let i = 0; i < count; i++) {
      const v = table[ph & (T - 1)] * gain * env(i / RATE);
      dry[(((start + i) % n) + n) % n] += v;
      ph += step;
      if (ph >= T * 1e6) ph -= T * 1e6;
    }
  };

  // Pads: each chord swells in and out over 18 s, overlapping its
  // neighbours by 3 s on each side, with a slow tremolo.
  const span = CHORD + 6;
  CHORDS.forEach((c, k) => {
    const at = k * CHORD - 3;
    const swell = (t: number) => Math.sin((Math.PI * t) / span) ** 2;
    c.notes.forEach((m, j) => {
      const f = midi(m);
      const trem = (t: number) => swell(t) * (0.85 + 0.15 * Math.sin(2 * Math.PI * (0.11 + j * 0.03) * t + j));
      voice(f * 0.9985, at, span, 0.05, trem);
      voice(f * 1.0015, at, span, 0.05, trem);
      voice(f * 2, at, span, 0.008, trem);
    });
    voice(midi(c.root), at, span, 0.09, swell);
    voice(midi(c.root - 12), at, span, 0.05, swell);
  });

  // Bells: now and then, a soft pentatonic note with a long fade.
  let seed = 7;
  const rand = () => ((seed = (seed * 1103515245 + 12345) & 0x7fffffff) / 0x7fffffff);
  for (let t = 1.5; t < len; t += 1.5) {
    if (rand() < 0.45) {
      const f = midi(BELLS[Math.floor(rand() * BELLS.length)]);
      const ring = (x: number) => Math.min(1, x / 0.01) * Math.exp(-x / 0.9);
      voice(f, t, 4, 0.045, ring);
      voice(f * 3.01, t, 2, 0.006, ring);
    }
  }

  // A spacious echo: two feedback delays, run around the loop twice so the
  // tails at the end carry into the start.
  const wet = new Float32Array(n);
  const delays = [Math.round(0.37 * RATE), Math.round(0.53 * RATE)];
  for (const d of delays) {
    const buf = new Float32Array(n);
    for (let i = 0; i < 2 * n; i++) {
      const j = i % n;
      buf[j] = dry[j] + 0.45 * buf[(j - d + n) % n];
    }
    for (let j = 0; j < n; j++) wet[j] += buf[j] - dry[j];
  }
  const outBuf = new Float32Array(n);
  let peak = 0;
  // A gentle low-pass, warmed up over the loop's end for a seamless start.
  let lp = 0;
  for (let i = n - RATE; i < 2 * n; i++) {
    const j = i % n;
    lp += 0.25 * (dry[j] + 0.35 * wet[j] - lp);
    if (i >= n) {
      outBuf[j] = lp;
      peak = Math.max(peak, Math.abs(lp));
    }
  }
  const scale = peak > 0 ? 0.7 / peak : 1;
  for (let j = 0; j < n; j++) outBuf[j] *= scale;
  return wavUrl(outBuf, RATE);
}

// Music that was on last visit starts with the first click or key press;
// browsers won't start it before one. It pauses while the tab is hidden.
if (typeof document !== "undefined") {
  const resume = () => {
    if (enabled && (!el || el.paused) && document.visibilityState === "visible") start();
  };
  for (const t of ["click", "keydown", "touchend"]) document.addEventListener(t, resume, { capture: true, passive: true });
  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "hidden") el?.pause();
    else if (enabled && el) start();
  });
}
