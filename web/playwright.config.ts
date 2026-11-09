import { defineConfig } from "@playwright/test";

// Builds the client, embeds it in the server binary and runs it on a test
// port. Run with `npm run e2e` (or `make e2e`).
export default defineConfig({
  testDir: "e2e",
  timeout: 120_000,
  use: { baseURL: "http://127.0.0.1:8787", trace: "retain-on-failure" },
  webServer: {
    command:
      "npm run build && cd .. && go build -tags embed -o bin/server ./cmd/server && bin/server -addr 127.0.0.1:8787 -system1 off",
    url: "http://127.0.0.1:8787",
    reuseExistingServer: false,
    timeout: 120_000,
  },
});
