import { defineConfig } from "@playwright/test";

// Runs the browser tests against the live site: npx playwright test -c playwright.prod.config.ts
// Override the address with PARALLAX_URL.
export default defineConfig({
  testDir: "e2e",
  timeout: 300_000,
  // Remote hosts and a System 1 model on a Pi answer slower than localhost.
  expect: { timeout: 30_000 },
  use: { baseURL: process.env.PARALLAX_URL ?? "https://parallax.sstools.co", trace: "retain-on-failure" },
});
