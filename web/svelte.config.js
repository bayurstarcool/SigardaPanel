import adapter from "@sveltejs/adapter-node";
import { vitePreprocess } from "@sveltejs/vite-plugin-svelte";

/** @type {import('@sveltejs/kit').Config} */
const config = {
  preprocess: vitePreprocess(),
  kit: {
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
  },
};

export default config;
