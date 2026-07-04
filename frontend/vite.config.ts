import { defineConfig } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";

// Vite config for the Wails-hosted Svelte front-end. Wails embeds the
// frontend/dist directory produced by `vite build`.
export default defineConfig({
  plugins: [svelte()],
  build: {
    target: "es2020",
    outDir: "dist",
    emptyOutDir: true,
  },
  server: {
    strictPort: true,
    host: "127.0.0.1",
  },
});
