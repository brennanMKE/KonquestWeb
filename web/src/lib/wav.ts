// Encodes mono samples (-1..1) as a 16-bit PCM WAV, served from a blob URL.
// The start chime and the music play through <audio> elements this way.

export function wavUrl(samples: Float32Array, rate: number): string {
  const n = samples.length;
  const b = new DataView(new ArrayBuffer(44 + n * 2));
  const str = (o: number, v: string) => {
    for (let i = 0; i < v.length; i++) b.setUint8(o + i, v.charCodeAt(i));
  };
  str(0, "RIFF");
  b.setUint32(4, 36 + n * 2, true);
  str(8, "WAVEfmt ");
  b.setUint32(16, 16, true);
  b.setUint16(20, 1, true); // PCM
  b.setUint16(22, 1, true); // mono
  b.setUint32(24, rate, true);
  b.setUint32(28, rate * 2, true);
  b.setUint16(32, 2, true);
  b.setUint16(34, 16, true);
  str(36, "data");
  b.setUint32(40, n * 2, true);
  for (let i = 0; i < n; i++) b.setInt16(44 + i * 2, Math.max(-1, Math.min(1, samples[i])) * 0x7fff, true);
  return URL.createObjectURL(new Blob([b.buffer], { type: "audio/wav" }));
}

/** A saved setting, or the fallback when storage is unavailable. */
export function loadSetting(key: string, fallback: string): string {
  try {
    return localStorage.getItem(key) ?? fallback;
  } catch {
    return fallback;
  }
}

export function saveSetting(key: string, value: string) {
  try {
    localStorage.setItem(key, value);
  } catch {
    // Not remembered; fine.
  }
}

/** A volume from 0 to 1, saved as a string. */
export function loadVolume(key: string, fallback: number): number {
  const v = Number(loadSetting(key, String(fallback)));
  return Number.isFinite(v) ? Math.max(0, Math.min(1, v)) : fallback;
}
