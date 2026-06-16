import { mkdirSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

const DIST_PLACEHOLDER = `# Placeholder so the desktop Go embed (//go:embed all:frontend/dist in main.go)
# resolves before the frontend is built (the Go-only CI job and fresh clones do
# not run a build). Real build output (index.html, assets/) is git-ignored.
# vite empties dist/ on build, so the hook in vite.config.ts re-creates this
# file to keep it tracked.
`;

// vite empties dist/ on each build, which removes the tracked .gitkeep that the
// desktop Go embed (//go:embed all:frontend/dist) depends on. Re-create it after
// the bundle is written so the placeholder stays tracked and the embed always
// resolves on a clean checkout.
function keepDistPlaceholder() {
  return {
    name: "keep-dist-placeholder",
    closeBundle() {
      const dir = resolve(import.meta.dirname, "dist");
      mkdirSync(dir, { recursive: true });
      writeFileSync(resolve(dir, ".gitkeep"), DIST_PLACEHOLDER);
    },
  };
}

export default defineConfig({
  plugins: [react(), keepDistPlaceholder()],
  clearScreen: false,
  build: {
    target: "es2022",
  },
  server: {
    port: 34115,
    strictPort: true,
  },
  test: {
    environment: "jsdom",
    setupFiles: "./src/test/setup.ts",
  },
});
