import { defineEnvVars } from "@sveltejs/kit/env";

// Server-only environment of the web process (systemd unit / web.env).
export const variables = defineEnvVars({
  SIGARDAPANEL_API_URL: {
    description: "URL of the Go API (default http://127.0.0.1:7700).",
    schema: (value) => value || undefined,
  },
  SIGARDAPANEL_PMA_SECRET: {
    description:
      "Key (min. 32 characters) that encrypts phpMyAdmin tickets; set by install.sh phpmyadmin.",
    schema: (value) => value || undefined,
  },
});
