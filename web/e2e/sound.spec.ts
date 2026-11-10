import { expect, test, type Page } from "@playwright/test";

// Counts the oscillators the page starts: each sound starts at least one.
async function countSounds(page: Page) {
  await page.addInitScript(() => {
    const w = window as unknown as { oscillators: number };
    w.oscillators = 0;
    const orig = AudioContext.prototype.createOscillator;
    AudioContext.prototype.createOscillator = function () {
      w.oscillators++;
      return orig.call(this);
    };
  });
}
const sounds = (page: Page) => page.evaluate(() => (window as unknown as { oscillators: number }).oscillators);

async function startGame(page: Page) {
  await page.goto("/");
  await page.getByLabel("Your name").fill("Ann");
  await page.getByRole("button", { name: "Create game" }).click();
  await page.getByRole("button", { name: "Add computer" }).click();
  await page.getByRole("button", { name: "Start game" }).click();
  await expect(page.getByText("Your turn")).toBeVisible();
}

test("the sound button shows when sound runs, and plays a test tone", async ({ page }) => {
  await countSounds(page);
  await startGame(page);
  const button = page.locator("button.sound");
  await page.mouse.click(5, 300);
  await expect(button).toHaveAttribute("data-audio", "running");
  await button.click();
  await expect(button).toHaveText("Sound off");
  const before = await sounds(page);
  await button.click();
  await expect(button).toHaveAttribute("data-audio", "running");
  await expect.poll(() => sounds(page)).toBeGreaterThan(before);
});

test("the sound button says so when the browser won't play", async ({ page }) => {
  await page.addInitScript(() => {
    // A browser that refuses: the context stays suspended and resume fails.
    Object.defineProperty(BaseAudioContext.prototype, "state", { get: () => "suspended" });
    AudioContext.prototype.resume = () => Promise.reject(new Error("not allowed"));
  });
  await startGame(page);
  const button = page.locator("button.sound");
  await page.mouse.click(5, 300);
  await expect(button).toHaveText("Sound blocked");
  await expect(button).toHaveAttribute("title", /not allowed/);
});
