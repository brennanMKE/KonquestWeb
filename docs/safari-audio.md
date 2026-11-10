# Why sound goes silent in Safari

Parallax makes its sound with the Web Audio API (`web/src/lib/audio.ts`): oscillators and noise, no audio files. This
note lists the well-known reasons Web Audio plays nothing in Safari, mostly on macOS, and what `audio.ts` does about
each one. Researched 2026-11-10. Every claim links to its source below. **Unconfirmed** means no source settled it.

## The usual causes, most likely first

The order is a judgment from how often each shows up in WebKit's bug tracker; there is no frequency data behind it.

1. **"Running" but silent.** The `AudioContext` reports `running` and plays nothing. Seen on macOS with Safari 17
   (macOS 14) and Safari 18.5 (macOS 15.5). Triggers: the tab was in the background, another tab that played audio was
   closed, another app played audio, or time passed. Sometimes `currentTime` stops; sometimes it keeps going.
   [276687], [291892], [263627], [276016], [283419]
2. **No gesture yet, or the gesture was lost.** Audio starts only from a click, key press or tap. Before late 2022,
   WebKit also required rendering to start synchronously inside that handler ([247614]).
3. **Stuck "suspended" or "interrupted", or `resume()` never settles.** After a background tab or a closed laptop lid.
   [281566], [276016], [263627], [MDN state]
4. **The output device or its sample rate changed.** AirPods, Bluetooth, a new default speaker.
   - [232728]: Safari 15, the clock sped up and audio stopped until a reload. Fixed Nov 2021.
   - [275726]: distortion after a speaker change on macOS 14.5. Fixed in macOS 14.6.
5. **Outside the page.** A muted tab (the speaker icon), the Mac's volume or output device. A page can't see any of
   these.
6. **Safari's per-site Auto-Play setting** (Safari → Settings → Websites → Auto-Play). **Unconfirmed** whether it
   affects Web Audio: WebKit's post about it covers only `<audio>` and `<video>` ([autoplay 2017]).
7. **iOS only.** The ring/silent switch mutes Web Audio by default ([237322]). A touch that slides instead of tapping
   doesn't count as a gesture ([248265]).

## Gestures and activation

- **What counts as a gesture** ([user activation], [MDN activation]):
  - `keydown`, except Esc and reserved keys
  - `mousedown`, and `pointerdown` from a mouse
  - `pointerup` from touch or a pen, and `touchend`
  - `click` comes after those, while the activation is still fresh.
- **How long it lasts.** The spec says Web Audio needs *sticky* activation, meaning any gesture so far. WebKit used to
  require the gesture at the moment rendering began; "the user gesture is lost" across an `await` ([247614]). Since
  Nov 2022 WebKit accepts *transient* activation, which lasts a few seconds. The first Safari version with that change
  is **unconfirmed** (probably 16.4).
- **The safe rule:** create or `resume()` the context synchronously inside the event handler, then await.
- **A context made before any gesture** starts `suspended`. A later gesture can resume it, but Safari needs an explicit
  `resume()`; Chrome also starts it on `start()` after an interaction ([Chrome autoplay]).
- **Listening on `document` in the capture phase** works in practice. Howler.js does exactly that for `touchstart`,
  `touchend`, `click` and `keydown` ([Howler]).
- **`navigator.userActivation`** (`hasBeenActive`, `isActive`) is in Safari 16.4 and later ([features explorer]).

## Context states and WebKit bugs

- **`interrupted`** is Safari's extra state. iOS uses it for a tab switch, a minimized window or a locked screen. It's
  now in the Web Audio spec ([TAG 1069], [Chromium interrupted]). The fix is `resume()`. Calling `resume()` during an
  interruption leaves the context `interrupted` until the interruption ends ([MDN state]).
- **Dead after the background or focus loss.**
  - [276016]: iOS 17.5.1, still open in 2025.
  - [263627]: `running` with `currentTime` stopped; suspending and resuming by hand brings it back.
  - Workarounds: suspend then resume on `visibilitychange`, or replace the context.
- **`resume()` never settles** ([281566], iOS 17.6.1; also reported on iOS 26 in [291892]). Workaround: suspend when
  hidden, resume on the next gesture after a short delay.
- **Silent after another tab's audio closes** ([276687], macOS 14 and iOS 17/18). `currentTime` keeps advancing. A new
  tab fixes it, and a reload often doesn't. Suspending and resuming on `visibilitychange` prevents some cases.
- **Another app plays sound** ([283419], iOS 18). The state stays `running` with no sound; `suspend()` then
  `resume()` fixes it.
- **Too many contexts.** An old limit of four was fixed long ago ([95757]). Thousands can still exhaust threads
  ([198964]). Use one shared context.

## What a page can and can't detect

- **It can see:**
  - `ctx.state` and its `statechange` event
  - whether `ctx.currentTime` moves while the state says `running`. This catches the stopped clock ([263627],
    [276016]), but not [276687], where the clock keeps going.
  - `navigator.userActivation.hasBeenActive`
  - a `resume()` that never settles, caught with a timeout
- **It can't see:** a muted tab, the system volume, the output device, or whether anyone hears it. An `AnalyserNode`
  sees only the page's own signal, not real output (inference, **unconfirmed**).
  `navigator.getAutoplayPolicy()` is Firefox-only as far as is known ([MDN getAutoplayPolicy]).
- **What the big libraries do:**
  - [Howler]: plays a one-sample silent buffer and calls `resume()` inside the gesture, keeps listening until it
    plays, and treats `interrupted` like `suspended`.
  - [Phaser]: on becoming visible, `suspend()`, then `resume()` 100 ms later.
  - [Tone.js]: `Tone.start()` from a click or key handler.

## What `audio.ts` does

| Cause | Handled | How |
| --- | --- | --- |
| Needs a gesture | Yes | `touchend`, `click` and `keydown` listeners on `document` (capture phase) create and resume the context synchronously, on every gesture |
| Gesture lost across `await` | Yes | `resume()` is called inside the handler |
| `interrupted` or `suspended` | Yes | Any state but `running` gets `resume()`; a sound that finds it asleep retries |
| `resume()` never settles | Yes | A 1 s timeout; a context that didn't come back is replaced at the next gesture |
| "Running" with a stopped clock | Yes | On a gesture (at most every 5 s), checks that `currentTime` moves; if not, suspends and resumes; if still stopped, replaces the context at the next gesture |
| Silent after hiding the tab | Yes | Suspends when the tab hides and resumes 100 ms after it shows, as Phaser does |
| "Running" and silent with a moving clock ([276687]) | No | A page can't detect it; a new tab fixes it |
| Device or sample-rate change | Partly | No forced sample rate; the clock check covers a stopped clock |
| Muted tab, volume, output device | Can't | The sound button's tip tells the player what to check; turning sound on plays a test tone |
| Per-site Auto-Play | Unknown | Would show as "Sound blocked" if Safari refuses `resume()` |
| iOS silent switch | No, on purpose | `navigator.audioSession.type = "playback"` would override it, but would also stop the player's music |

The sound button shows the status: `Sound on`, `Sound off`, `Sound blocked` (a gesture came and the browser still
won't play) or `No sound` (no Web Audio). A blocked context also logs a warning in the console with the browser's
reason.

## If a player hears nothing

1. Click anywhere on the page, then read the sound button.
2. **Sound off:** click it. It plays a two-note tone.
3. **Sound blocked:** hover it for the reason. Check Safari → Settings → Websites → Auto-Play for the site.
4. **Sound on, but silent:**
   - Check the tab's speaker icon, the Mac's volume and its output device.
   - Click the button twice (off, then on) to play the tone again.
   - Still silent: open the game in a new tab. This is the fix for [276687].

## Sources

- [BaseAudioContext.state](https://developer.mozilla.org/en-US/docs/Web/API/BaseAudioContext/state)
- [Autoplay guide for media and Web Audio APIs](https://developer.mozilla.org/en-US/docs/Web/Media/Guides/Autoplay)
- [User activation](https://developer.mozilla.org/en-US/docs/Web/Security/User_activation)
- [Navigator.getAutoplayPolicy()](https://developer.mozilla.org/en-US/docs/Web/API/Navigator/getAutoplayPolicy)
- [AudioSession.type](https://developer.mozilla.org/en-US/docs/Web/API/AudioSession/type)
- [WebKit blog: The User Activation API](https://webkit.org/blog/13862/the-user-activation-api/)
- [WebKit blog: Auto-Play Policy Changes for macOS](https://webkit.org/blog/7734/auto-play-policy-changes-for-macos/)
- [Web Platform features explorer: User activation](https://web-platform-dx.github.io/web-features-explorer/features/user-activation)
- [WebKit bug 247614: only require a transient user activation for Web Audio rendering](https://bugs.webkit.org/show_bug.cgi?id=247614)
- [WebKit bug 276016: Web Audio stops after losing focus](https://bugs.webkit.org/show_bug.cgi?id=276016)
- [WebKit bug 281566: AudioContext.resume() never resolves after the background](https://bugs.webkit.org/show_bug.cgi?id=281566)
- [WebKit bug 276687: Web Audio goes silent when another tab with audio closes](https://bugs.webkit.org/show_bug.cgi?id=276687)
- [WebKit bug 291892: AudioContext silent after returning from the background](https://bugs.webkit.org/show_bug.cgi?id=291892)
- [WebKit bug 263627: AudioContext not consistently resumed in the foreground](https://bugs.webkit.org/show_bug.cgi?id=263627)
- [WebKit bug 283419: AudioContext stays running when another app plays sound](https://bugs.webkit.org/show_bug.cgi?id=283419)
- [WebKit bug 217606: Web Audio broken after switching apps ("interrupted")](https://bugs.webkit.org/show_bug.cgi?id=217606)
- [WebKit bug 232728: currentTime speeds up and audio stops with Bluetooth](https://bugs.webkit.org/show_bug.cgi?id=232728)
- [WebKit bug 275726: robotic audio after a speaker change](https://bugs.webkit.org/show_bug.cgi?id=275726)
- [WebKit bug 248265: AudioContext fails to resume after touchend](https://bugs.webkit.org/show_bug.cgi?id=248265)
- [WebKit bug 237322: Web Audio muted by the iOS ringer switch](https://lists.webkit.org/pipermail/webkit-unassigned/2022-March/1024413.html)
- [WebKit bug 95757: old AudioContext limit](https://bugs.webkit.org/show_bug.cgi?id=95757)
- [WebKit bug 198964: thread exhaustion from many contexts](https://bugs.webkit.org/show_bug.cgi?id=198964)
- [Chrome: Web Audio, Autoplay Policy and Games](https://developer.chrome.com/blog/web-audio-autoplay)
- [Chromium: Intent to Ship: AudioContext Interrupted State](https://groups.google.com/a/chromium.org/g/blink-dev/c/iNj-OIZ1T3Q)
- [W3C TAG review 1069: AudioContext "interrupted" state](https://tag-github-bot.w3.org/gh/w3ctag/design-reviews/1069)
- [Howler.js (MIT): `_unlockAudio`, `_autoResume`](https://github.com/goldfire/howler.js/blob/master/src/howler.core.js)
- [Phaser (MIT): WebAudioSoundManager](https://github.com/phaserjs/phaser/blob/master/src/sound/webaudio/WebAudioSoundManager.js)
- [Tone.js `start()`](https://tonejs.github.io/docs/15.0.4/functions/start.html)

[MDN state]: https://developer.mozilla.org/en-US/docs/Web/API/BaseAudioContext/state
[MDN activation]: https://developer.mozilla.org/en-US/docs/Web/Security/User_activation
[MDN getAutoplayPolicy]: https://developer.mozilla.org/en-US/docs/Web/API/Navigator/getAutoplayPolicy
[user activation]: https://webkit.org/blog/13862/the-user-activation-api/
[autoplay 2017]: https://webkit.org/blog/7734/auto-play-policy-changes-for-macos/
[features explorer]: https://web-platform-dx.github.io/web-features-explorer/features/user-activation
[247614]: https://bugs.webkit.org/show_bug.cgi?id=247614
[276016]: https://bugs.webkit.org/show_bug.cgi?id=276016
[281566]: https://bugs.webkit.org/show_bug.cgi?id=281566
[276687]: https://bugs.webkit.org/show_bug.cgi?id=276687
[291892]: https://bugs.webkit.org/show_bug.cgi?id=291892
[263627]: https://bugs.webkit.org/show_bug.cgi?id=263627
[283419]: https://bugs.webkit.org/show_bug.cgi?id=283419
[232728]: https://bugs.webkit.org/show_bug.cgi?id=232728
[275726]: https://bugs.webkit.org/show_bug.cgi?id=275726
[248265]: https://bugs.webkit.org/show_bug.cgi?id=248265
[237322]: https://lists.webkit.org/pipermail/webkit-unassigned/2022-March/1024413.html
[95757]: https://bugs.webkit.org/show_bug.cgi?id=95757
[198964]: https://bugs.webkit.org/show_bug.cgi?id=198964
[Chrome autoplay]: https://developer.chrome.com/blog/web-audio-autoplay
[Chromium interrupted]: https://groups.google.com/a/chromium.org/g/blink-dev/c/iNj-OIZ1T3Q
[TAG 1069]: https://tag-github-bot.w3.org/gh/w3ctag/design-reviews/1069
[Howler]: https://github.com/goldfire/howler.js/blob/master/src/howler.core.js
[Phaser]: https://github.com/phaserjs/phaser/blob/master/src/sound/webaudio/WebAudioSoundManager.js
[Tone.js]: https://tonejs.github.io/docs/15.0.4/functions/start.html
