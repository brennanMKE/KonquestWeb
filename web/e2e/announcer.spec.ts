import { expect, test } from "@playwright/test";

// Records what the page asks the browser to say, and what it plays.
test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => {
    const w = window as unknown as { spoken: string[]; played: number; media: HTMLMediaElement[] };
    w.spoken = [];
    w.played = 0;
    w.media = [];
    if ("speechSynthesis" in window) {
      window.speechSynthesis.speak = (u: SpeechSynthesisUtterance) => {
        if (u.text) w.spoken.push(u.text);
        setTimeout(() => u.onend?.(new Event("end") as SpeechSynthesisEvent), 10);
      };
    }
    const play = HTMLMediaElement.prototype.play;
    HTMLMediaElement.prototype.play = function () {
      w.played++;
      if (!w.media.includes(this)) w.media.push(this);
      return play.call(this);
    };
  });
});
const spoken = (page: import("@playwright/test").Page) => page.evaluate(() => (window as unknown as { spoken: string[] }).spoken);

test("the audio panel turns each part on and off and sets its volume", async ({ page }) => {
  await page.goto("/");
  await page.getByRole("button", { name: "Audio ▾" }).click();
  const music = page.getByLabel("Music", { exact: true });
  await expect(music).not.toBeChecked(); // music starts off
  await expect(page.getByLabel("Music volume")).toBeDisabled();
  await music.check();
  await expect.poll(() => page.evaluate(() => (window as unknown as { played: number }).played)).toBeGreaterThan(0);
  await page.getByLabel("Music volume").fill("30");

  await page.getByLabel("Sound effects volume").fill("40");
  await page.getByLabel("Sound effects", { exact: true }).uncheck();
  await expect(page.locator("button.sound")).toHaveText("Sound off");

  const voice = page.getByLabel("Announcer", { exact: true });
  await expect(voice).toBeChecked(); // the announcer starts on
  await page.getByLabel("Announcer volume").fill("70");
  await voice.uncheck();
  await voice.check();
  await expect.poll(() => spoken(page)).toContain("Announcer on.");

  const saved = await page.evaluate(() => ({ ...localStorage }));
  expect(saved).toMatchObject({
    "parallax.music": "on",
    "parallax.musicVolume": "0.3",
    "parallax.sound": "off",
    "parallax.soundVolume": "0.4",
    "parallax.voice": "on",
    "parallax.voiceVolume": "0.7",
  });
  // Remembered on the next visit.
  await page.reload();
  await page.getByRole("button", { name: "Audio ▾" }).click();
  await expect(page.getByLabel("Music", { exact: true })).toBeChecked();
  await expect(page.getByLabel("Sound effects", { exact: true })).not.toBeChecked();
  await expect(page.getByLabel("Music volume")).toHaveValue("30");
});

test("the announcer calls a fall and a rise in a replay", async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem("parallax.view", "2d"));
  await page.goto("/");
  await page
    .locator("label", { hasText: "Watch saved games" })
    .locator("input")
    .setInputFiles(new URL("./fixtures/fallen-risen.json", import.meta.url).pathname);
  await expect(page.getByRole("button", { name: "Close replay" })).toBeVisible();
  await page.locator("body").click({ position: { x: 5, y: 5 } });
  for (let i = 0; i < 70; i++) await page.keyboard.press("ArrowRight");
  await expect.poll(() => spoken(page)).toEqual(expect.arrayContaining([expect.stringMatching(/ has fallen\.$/), expect.stringMatching(/ has risen again!$/)]));
  // Only the big moments are spoken: nothing about single battles.
  for (const s of await spoken(page)) expect(s).toMatch(/fallen|risen|destroyed|galaxy|war is over/);
});

test("the music grows more intense as battles come thick and fast", async ({ page }) => {
  await page.addInitScript(() => {
    localStorage.setItem("parallax.view", "2d");
    localStorage.setItem("parallax.music", "on");
  });
  await page.goto("/");
  await page
    .locator("label", { hasText: "Watch saved games" })
    .locator("input")
    .setInputFiles(new URL("./fixtures/fallen-risen.json", import.meta.url).pathname);
  await expect(page.getByRole("button", { name: "Close replay" })).toBeVisible();
  await page.locator("body").click({ position: { x: 5, y: 5 } }); // the click that starts music
  // Calm, tense and battle mixes all play in step; at first only calm is heard.
  const volumes = () =>
    page.evaluate(() => (window as unknown as { media: HTMLMediaElement[] }).media.filter((m) => !m.paused).map((m) => m.volume));
  await expect.poll(async () => (await volumes()).length).toBe(3);
  const [calm, tense, battle] = await volumes();
  expect(calm).toBeGreaterThan(0);
  expect(tense + battle).toBe(0);
  // A run of rounds full of battles brings the louder mixes in.
  for (let i = 0; i < 70; i++) await page.keyboard.press("ArrowRight");
  await expect.poll(async () => { const v = await volumes(); return v[1] + v[2]; }, { timeout: 15000 }).toBeGreaterThan(0.01);
});
