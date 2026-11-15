import { resolve } from "node:path"
import tailwindcss from "@tailwindcss/vite"
import react from "@vitejs/plugin-react"
import { defineConfig, type Plugin } from "vite"
import { configDefaults } from "vitest/config"

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
    // The product's pages, and gusto cloud's own pages, which the front door serves with its landing page among them.
    rollupOptions: {
      input: {
        index: resolve(import.meta.dirname, "index.html"),
        cloud: resolve(import.meta.dirname, "cloud.html"),
      },
    },
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
    // The app's stylesheet reaches the test that reads its rules; every other stylesheet is left out, as vitest leaves them.
    css: { include: [/src\/index\.css/] },
    // e2e/ is Playwright's: those tests drive a real server in a real browser, and run with npm run e2e.
    exclude: [...configDefaults.exclude, "e2e/**"],
  },
  server: {
    // The page scripts the server serves are read by the tests that run them.
    fs: { allow: [".", "../internal/web/static"] },
    proxy: {
      "/app/api": gusto,
      "/static": gusto,
      "/login": gusto,
      "/logout": gusto,
    },
  },
})
