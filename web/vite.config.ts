import adapter from "@sveltejs/adapter-node";
import { sveltekit } from "@sveltejs/kit/vite";
import { vitePreprocess } from "@sveltejs/vite-plugin-svelte";
import tailwindcss from "@tailwindcss/vite";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [
    tailwindcss(),
    sveltekit({
      preprocess: vitePreprocess(),
      adapter: adapter({ out: "build" }),
      // SvelteKit adds nonces/hashes for its own inline bootstrap script, so
      // script-src needs no 'unsafe-inline'. Nothing is loaded from other origins.
      csp: {
        mode: "auto",
        directives: {
          "default-src": ["self"],
          "script-src": ["self"],
          "style-src": ["self", "unsafe-inline"],
          "img-src": ["self", "data:", "blob:"],
          "font-src": ["self", "data:"],
          "connect-src": ["self", "ws:", "wss:"],
          "worker-src": ["self", "blob:"],
          "object-src": ["none"],
          "base-uri": ["self"],
          "form-action": ["self"],
          "frame-ancestors": ["none"],
        },
      },
      alias: {
        $lib: "src/lib",
        $components: "src/components",
      },
    }),
  ],
  server: {
    host: "0.0.0.0",
    allowedHosts: [
      "panel.sigarda.com",
      "panel.sigarda.dev",
      "localhost",
      "127.0.0.1",
    ],
    proxy: {
      "/api/proxy": {
        target: "http://localhost:8080",
        changeOrigin: true,
        rewrite: (path) => "/api/v1/" + path.slice("/api/proxy/".length),
        timeout: 300000,
      },
      "/api/auth": {
        target: "http://localhost:8080",
        changeOrigin: true,
        rewrite: (path) => "/api/v1" + path.slice("/api".length),
        timeout: 300000,
      },
      "/api/metrics": {
        target: "http://localhost:8080",
        changeOrigin: true,
        rewrite: (path) => "/api/v1" + path.slice("/api".length),
        timeout: 300000,
      },
    },
  },
});
