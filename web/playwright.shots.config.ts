import { defineConfig } from "@playwright/test";

// Captures the promotional images: the link preview (public/og.jpg) and
// the README pictures (../docs/images). Run with `npm run shots` (or
// `make shots`), which first plays a seeded computer game with cmd/sim.
export default defineConfig({
  testDir: "shots",
  timeout: 300_000,
  use: { baseURL: "http://127.0.0.1:8788" },
  webServer: {
    command:
      "npm run build && cd .. && go build -tags embed -o bin/server ./cmd/server && bin/server -addr 127.0.0.1:8788",
    url: "http://127.0.0.1:8788",
    reuseExistingServer: false,
    timeout: 120_000,
  },
});
