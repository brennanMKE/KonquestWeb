// The announcer: a few big moments spoken with the browser's own speech
// (speechSynthesis), separate from the sound effects. It says when a
// faction falls (its last planet lost, fleets still flying), rises again,
// is destroyed, and who won.

import { loadSetting, loadVolume, saveSetting } from "./wav";

const KEY = "parallax.voice";
const VOLUME_KEY = "parallax.voiceVolume";
// Announcements left waiting beyond this are dropped: old news.
const MAX_QUEUED = 3;

const synth = typeof window !== "undefined" && "speechSynthesis" in window ? window.speechSynthesis : null;
let enabled = loadSetting(KEY, "on") !== "off";
let volume = loadVolume(VOLUME_KEY, 0.9);
let queued = 0;

export const voiceSupported = () => synth !== null;
export const voiceOn = () => enabled;
export const voiceVolume = () => volume;

export function setVoice(on: boolean) {
  enabled = on;
  saveSetting(KEY, on ? "on" : "off");
  if (!on) {
    synth?.cancel();
    queued = 0;
  }
}

export function setVoiceVolume(v: number) {
  volume = Math.max(0, Math.min(1, v));
  saveSetting(VOLUME_KEY, String(volume));
}

// An English voice, the system's default one if it is English.
function pickVoice(): SpeechSynthesisVoice | null {
  const voices = synth?.getVoices() ?? [];
  const en = voices.filter((v) => v.lang.toLowerCase().startsWith("en"));
  return en.find((v) => v.default) ?? en.find((v) => v.localService) ?? en[0] ?? null;
}

/** Speaks a line, after any already queued. */
export function say(text: string) {
  if (!enabled || !synth || !text) return;
  if (queued >= MAX_QUEUED) return;
  const u = new SpeechSynthesisUtterance(text);
  const v = pickVoice();
  if (v) u.voice = v;
  u.volume = volume;
  u.rate = 0.95;
  u.pitch = 0.9;
  queued++;
  const done = () => (queued = Math.max(0, queued - 1));
  u.onend = done;
  u.onerror = (e) => {
    done();
    if (e.error !== "canceled" && e.error !== "interrupted") console.warn(`Parallax: the browser wouldn't speak: ${e.error}`);
  };
  synth.speak(u);
}

// Safari speaks only once a click or key press has started speech, so the
// first one speaks an empty line.
if (synth && typeof document !== "undefined") {
  const unlock = () => {
    if (enabled) synth.speak(new SpeechSynthesisUtterance(""));
    for (const t of ["click", "keydown", "touchend"]) document.removeEventListener(t, unlock, true);
  };
  for (const t of ["click", "keydown", "touchend"]) document.addEventListener(t, unlock, { capture: true, passive: true });
}
