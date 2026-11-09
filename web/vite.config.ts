import { defineConfig } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";

// In development, run the Go server on :8080 and `npm run dev`; Vite proxies
// the API and WebSocket to it.
export default defineConfig({
  plugins: [svelte()],
  // three.js (about 570 kB, 140 kB gzipped) loads only when a player opens
  // the 3D view, so its chunk may be large.
  build: { chunkSizeWarningLimit: 700 },
  server: {
    proxy: {
      "/api": "http://localhost:8080",
      "/ws": { target: "ws://localhost:8080", ws: true },
    },
  },
});
