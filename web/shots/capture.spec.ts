// Captures promotional images from a seeded computer game played back in
// the replay viewer's 3D view. The game file comes from cmd/sim (see
// `make shots`); SHOT_GAME overrides it.
import { execFileSync } from "node:child_process";
import { mkdirSync, readFileSync, rmSync } from "node:fs";
import { resolve } from "node:path";
import { expect, test, type Page } from "@playwright/test";
import type { Replay } from "../src/lib/proto";

const root = resolve(import.meta.dirname, "..", "..");
const game = process.env.SHOT_GAME ?? resolve(root, ".run/shot.json");
const images = resolve(root, "docs/images");

// Only the board: no header, side panel or buttons.
const boardOnly = `
  .replay header, .replay aside, .view-toggle, .board3d .views, .board3d .hint { display: none !important; }
  .replay .board-wrap { padding: 0 !important; }
`;

/** The most eventful round in the middle of the game: many fleets in flight, many empires. */
function pickFrame(r: Replay): number {
  let best = 1;
  let bestScore = -1;
  const n = r.frames.length;
  for (let i = Math.floor(n * 0.2); i < Math.floor(n * 0.7); i++) {
    const v = r.frames[i].view;
    const owners = new Set(v.planets.filter((p) => p.owner >= 0).map((p) => p.owner)).size;
    const score = v.fleets.length * owners;
    if (score > bestScore) {
      bestScore = score;
      best = i;
    }
  }
  return best;
}

async function openReplay(page: Page, file: Buffer, autocam = false) {
  await page.addInitScript((autocam) => {
    localStorage.setItem("parallax.view", "3d");
    localStorage.setItem("parallax.autocam", autocam ? "on" : "off");
    localStorage.setItem("parallax.sound", "off");
    localStorage.setItem("parallax.replays", "on");
  }, autocam);
  await page.goto("/");
  await page.addStyleTag({ content: boardOnly });
  await page.locator("label", { hasText: "Watch saved games" }).locator("input").setInputFiles({
    name: "game.json",
    mimeType: "application/json",
    buffer: file,
  });
  await expect(page.getByTestId("board-3d")).toBeVisible();
}

async function showFrame(page: Page, i: number) {
  await page.locator(".replay input[type=range]").evaluate((el, i) => {
    const input = el as HTMLInputElement;
    input.value = String(i);
    input.dispatchEvent(new Event("input", { bubbles: true }));
  }, i);
  // Let the scene settle: labels place themselves after a frame or two.
  await page.waitForTimeout(1500);
}

test("link preview and README images", async ({ browser, request }) => {
  const file = readFileSync(game);
  const replay = (await (await request.post("/api/replay", { data: file.toString() })).json()) as Replay;
  const frame = pickFrame(replay);
  console.log(`Using round ${replay.frames[frame].round} of ${replay.frames.length - 1}`);
  mkdirSync(images, { recursive: true });

  // The link preview: 1200 × 630, the size link unfurls expect.
  const og = await browser.newPage({ viewport: { width: 1200, height: 630 } });
  await openReplay(og, file);
  await showFrame(og, frame);
  // The name and tagline, for a link shared anywhere.
  await og.evaluate(() => {
    const el = document.createElement("div");
    el.innerHTML = "<h1>Parallax</h1><p>Conquer the galaxy, one fleet at a time.</p><p class='sub'>Multiplayer space strategy in your browser</p>";
    el.style.cssText =
      "position:fixed;left:0;top:0;padding:40px 64px 40px 48px;z-index:10;pointer-events:none;font-family:system-ui,sans-serif;" +
      "background:radial-gradient(ellipse at 30% 40%, rgb(11 14 22 / 0.85) 40%, rgb(11 14 22 / 0) 75%)";
    const [h, p, sub] = el.children as unknown as HTMLElement[];
    h.style.cssText = "margin:0;font-size:64px;letter-spacing:0.08em;color:#fff;text-shadow:0 2px 16px #000";
    p.style.cssText = "margin:6px 0 0;font-size:24px;color:#cfd6ea;text-shadow:0 2px 10px #000";
    sub.style.cssText = "margin:4px 0 0;font-size:18px;color:#8a94b2;text-shadow:0 2px 10px #000";
    document.body.append(el);
  });
  await og.screenshot({ path: resolve(root, "web/public/og.jpg"), type: "jpeg", quality: 88 });
  await og.close();

  // The README picture, sharp on high-density screens.
  const hero = await browser.newPage({ viewport: { width: 1200, height: 630 }, deviceScaleFactor: 2 });
  await openReplay(hero, file);
  await showFrame(hero, frame);
  await hero.screenshot({ path: resolve(images, "map.jpg"), type: "jpeg", quality: 82 });
  await hero.close();

  // A few rounds of battles, recorded and turned into a GIF with ffmpeg. The
  // camera stays wide, so the whole map shifts as empires grow.
  const videoDir = resolve(root, ".run/shot-video");
  rmSync(videoDir, { recursive: true, force: true });
  const ctx = await browser.newContext({
    viewport: { width: 960, height: 540 },
    recordVideo: { dir: videoDir, size: { width: 960, height: 540 } },
  });
  const page = await ctx.newPage();
  await openReplay(page, file);
  await showFrame(page, Math.max(1, frame - 3));
  await page.keyboard.press(" ");
  await page.waitForTimeout(20_000);
  const video = await page.video()!.path();
  await ctx.close();
  try {
    execFileSync("ffmpeg", [
      "-y", "-loglevel", "error", "-ss", "2", "-t", "16", "-i", video,
      "-vf", "fps=12,scale=720:-1:flags=lanczos,split[a][b];[a]palettegen=max_colors=128[p];[b][p]paletteuse=dither=bayer:bayer_scale=4",
      resolve(images, "battles.gif"),
    ]);
  } catch (e) {
    console.log(`Skipped the GIF (ffmpeg): ${(e as Error).message}`);
  }
});
