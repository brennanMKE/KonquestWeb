<script lang="ts">
  import { onAudioStatus, setSound, setSoundVolume, soundOn, soundVolume, testSound } from "./audio";
  import { musicOn, musicVolume, setMusic, setMusicVolume } from "./music";
  import { say, setVoice, setVoiceVolume, voiceOn, voiceSupported, voiceVolume } from "./voice";

  // Audio settings: sound effects, music and the announcer, each with its
  // own switch and volume. The browser remembers them.
  let open = $state(false);
  let root = $state<HTMLElement | null>(null);
  // The panel opens toward the side with room: rightward near the left edge.
  let alignLeft = $state(false);
  function toggle() {
    if (!open && root) alignLeft = root.getBoundingClientRect().right < 300;
    open = !open;
  }

  let effects = $state(soundOn());
  let effectsVol = $state(Math.round(soundVolume() * 100));
  let music = $state(musicOn());
  let musicVol = $state(Math.round(musicVolume() * 100));
  let voice = $state(voiceOn());
  let voiceVol = $state(Math.round(voiceVolume() * 100));
  // The sound button beside this can turn effects off too.
  $effect(() => onAudioStatus((s) => (effects = s !== "off")));

  function toggleEffects() {
    setSound(effects);
    if (effects) testSound();
  }
  function toggleVoice() {
    setVoice(voice);
    if (voice) say("Announcer on.");
  }

  function outside(e: MouseEvent) {
    if (open && root && !root.contains(e.target as Node)) open = false;
  }
  function onkey(e: KeyboardEvent) {
    if (open && e.key === "Escape") {
      open = false;
      e.stopPropagation();
    }
  }
</script>

<svelte:window onclick={outside} onkeydown={onkey} />

<div class="audio" bind:this={root}>
  <button class="small" onclick={toggle} aria-expanded={open} title="Sound effects, music and the announcer">Audio ▾</button>
  {#if open}
    <div class="panel" class:left={alignLeft} role="group" aria-label="Audio settings">
      <label class="row">
        <input type="checkbox" bind:checked={effects} onchange={toggleEffects} />
        <span>Sound effects</span>
        <input
          type="range"
          min="0"
          max="100"
          aria-label="Sound effects volume"
          bind:value={effectsVol}
          oninput={() => setSoundVolume(effectsVol / 100)}
          onchange={() => effects && testSound()}
          disabled={!effects}
        />
      </label>
      <label class="row">
        <input type="checkbox" bind:checked={music} onchange={() => setMusic(music)} />
        <span>Music</span>
        <input
          type="range"
          min="0"
          max="100"
          aria-label="Music volume"
          bind:value={musicVol}
          oninput={() => setMusicVolume(musicVol / 100)}
          disabled={!music}
        />
      </label>
      <label class="row">
        <input type="checkbox" bind:checked={voice} onchange={toggleVoice} disabled={!voiceSupported()} />
        <span>Announcer</span>
        <input
          type="range"
          min="0"
          max="100"
          aria-label="Announcer volume"
          bind:value={voiceVol}
          oninput={() => setVoiceVolume(voiceVol / 100)}
          onchange={() => voice && say("Announcer volume.")}
          disabled={!voice || !voiceSupported()}
        />
      </label>
      {#if !voiceSupported()}<p class="muted">This browser can't speak announcements.</p>{/if}
    </div>
  {/if}
</div>

<style>
  .audio {
    position: relative;
    display: inline-block;
  }
  .panel {
    position: absolute;
    right: 0;
    top: calc(100% + 6px);
    z-index: 50;
    background: var(--panel);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 10px 12px;
    display: grid;
    gap: 8px;
    min-width: 280px;
    box-sizing: border-box;
    max-width: calc(100vw - 32px);
    box-shadow: 0 8px 24px rgb(0 0 0 / 0.4);
    text-align: left;
  }
  .panel.left {
    right: auto;
    left: 0;
  }
  .row {
    display: grid;
    grid-template-columns: auto 1fr 120px;
    align-items: center;
    gap: 8px;
    font-size: 13px;
    white-space: nowrap;
  }
  .row input[type="range"] {
    width: 100%;
    min-width: 0;
    margin: 0;
    padding: 0;
    accent-color: var(--accent);
  }
  .panel p {
    margin: 0;
    font-size: 12px;
  }
</style>
