import { defineConfig } from "astro/config";
import sitemap from "@astrojs/sitemap";

// Served from https://doittikorn.github.io/krok/ (GitHub Pages project site).
export default defineConfig({
  site: "https://doittikorn.github.io",
  base: "/krok",
  trailingSlash: "always",
  integrations: [sitemap()],
  // three.js is loaded lazily, so its size does not block first paint.
  vite: { build: { chunkSizeWarningLimit: 800 } },
});
