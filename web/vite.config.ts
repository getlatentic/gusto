/// <reference types="vitest/config" />
import { resolve } from "node:path"
import tailwindcss from "@tailwindcss/vite"
import react from "@vitejs/plugin-react"
import { defineConfig, type Plugin } from "vite"

// The gusto server serves this app: it builds into the Go package that embeds it, under /app/, and the server fills the
// <!--gusto-head--> mark with its own tokens, faces and theme script. `npm run dev` runs against a gusto server on :8700,
// which signs the request in and answers /app/api.
const gusto = "http://127.0.0.1:8700"

function gustoHeadInDev(): Plugin {
  return {
    name: "gusto-head-in-dev",
    apply: "serve",
    transformIndexHtml: (html) =>
      html.replace(
        "<!--gusto-head-->",
        ["tokens", "fonts", "theme"]
          .map((name) => `<link rel="stylesheet" href="/static/${name}.css">`)
          .join("") + `<script src="/static/look.js"></script>`
      ),
  }
}

export default defineConfig({
  base: "/app/",
  plugins: [react(), tailwindcss(), gustoHeadInDev()],
  resolve: {
    alias: {
      "@": resolve(import.meta.dirname, "./src"),
    },
  },
  build: {
    outDir: resolve(import.meta.dirname, "../internal/web/spa/dist"),
    emptyOutDir: true,
    assetsDir: "assets",
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
  },
  server: {
    proxy: {
      "/app/api": gusto,
      "/static": gusto,
      "/login": gusto,
      "/logout": gusto,
    },
  },
})
