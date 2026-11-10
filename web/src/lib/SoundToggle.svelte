<script lang="ts">
  import { audioError, onAudioStatus, setSound, testSound, type AudioStatus } from "./audio";

  // The sound button for the game and the replay viewer. It says what the
  // browser is doing with sound, not just the setting: a page can't hear
  // its own output, but it can tell when the browser refuses to play.
  let status = $state<AudioStatus>("waiting");
  $effect(() => onAudioStatus((s) => (status = s)));

  const labels: Record<AudioStatus, string> = {
    off: "Sound off",
    waiting: "Sound on",
    running: "Sound on",
    blocked: "Sound blocked",
    unsupported: "No sound",
  };
  let title = $derived(
    {
      off: "Turn sound on (plays a test tone)",
      waiting: "Sound starts with your first click or key press. Click to test it.",
      running: "Turn sound off. If you hear nothing, check the volume, the output device and whether the tab is muted.",
      blocked: `The browser isn't playing sound${audioError() ? ` (${audioError()})` : ""}. Click to try again. In Safari, check Settings → Websites → Auto-Play for this site.`,
      unsupported: "This browser can't play the game's sound.",
    }[status],
  );

  function onclick() {
    if (status === "running") setSound(false);
    else testSound();
  }
</script>

<button
  class="small sound"
  class:warn={status === "blocked" || status === "unsupported"}
  {onclick}
  aria-pressed={status !== "off"}
  {title}
  data-audio={status}
>
  {labels[status]}
</button>

<style>
  .warn {
    border-color: #ffc46b;
    color: #ffc46b;
  }
</style>
