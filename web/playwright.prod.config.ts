import { defineConfig } from "@playwright/test";

// Runs the browser tests against a deployed server:
//   PARALLAX_URL=https://game.example.com npx playwright test -c playwright.prod.config.ts
const url = process.env.PARALLAX_URL;
if (!url) throw new Error("Set PARALLAX_URL to the server to test, e.g. https://game.example.com");
export default defineConfig({
  testDir: "e2e",
  timeout: 300_000,
  // Remote hosts and a System 1 model on a Pi answer slower than localhost.
  expect: { timeout: 30_000 },
  use: { baseURL: url, trace: "retain-on-failure" },
});
