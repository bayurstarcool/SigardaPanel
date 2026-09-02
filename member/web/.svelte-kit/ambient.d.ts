
// this file is generated — do not edit it


/// <reference types="@sveltejs/kit" />

/**
 * This module provides access to environment variables that are injected _statically_ into your bundle at build time and are limited to _private_ access.
 * 
 * |         | Runtime                                                                    | Build time                                                               |
 * | ------- | -------------------------------------------------------------------------- | ------------------------------------------------------------------------ |
 * | Private | [`$env/dynamic/private`](https://svelte.dev/docs/kit/$env-dynamic-private) | [`$env/static/private`](https://svelte.dev/docs/kit/$env-static-private) |
 * | Public  | [`$env/dynamic/public`](https://svelte.dev/docs/kit/$env-dynamic-public)   | [`$env/static/public`](https://svelte.dev/docs/kit/$env-static-public)   |
 * 
 * Static environment variables are [loaded by Vite](https://vitejs.dev/guide/env-and-mode.html#env-files) from `.env` files and `process.env` at build time and then statically injected into your bundle at build time, enabling optimisations like dead code elimination.
 * 
 * **_Private_ access:**
 * 
 * - This module cannot be imported into client-side code
 * - This module only includes variables that _do not_ begin with [`config.kit.env.publicPrefix`](https://svelte.dev/docs/kit/configuration#env) _and do_ start with [`config.kit.env.privatePrefix`](https://svelte.dev/docs/kit/configuration#env) (if configured)
 * 
 * For example, given the following build time environment:
 * 
 * ```env
 * ENVIRONMENT=production
 * PUBLIC_BASE_URL=http://site.com
 * ```
 * 
 * With the default `publicPrefix` and `privatePrefix`:
 * 
 * ```ts
 * import { ENVIRONMENT, PUBLIC_BASE_URL } from '$env/static/private';
 * 
 * console.log(ENVIRONMENT); // => "production"
 * console.log(PUBLIC_BASE_URL); // => throws error during build
 * ```
 * 
 * The above values will be the same _even if_ different values for `ENVIRONMENT` or `PUBLIC_BASE_URL` are set at runtime, as they are statically replaced in your code with their build time values.
 */
declare module '$env/static/private' {
	export const BROWSERBASE_PROXIES: string;
	export const _HERMES_GATEWAY: string;
	export const HERMES_SESSION_KEY: string;
	export const USER: string;
	export const npm_config_user_agent: string;
	export const TERMINAL_CONTAINER_CPU: string;
	export const HERMES_SESSION_USER_NAME: string;
	export const TERMINAL_CWD: string;
	export const HERMES_REAL_HOME: string;
	export const npm_node_execpath: string;
	export const SHLVL: string;
	export const npm_config_noproxy: string;
	export const HOME: string;
	export const HERMES_UI_SESSION_ID: string;
	export const OLDPWD: string;
	export const TERMINAL_DOCKER_FORWARD_ENV: string;
	export const BROWSER_SESSION_TIMEOUT: string;
	export const HERMES_HOME: string;
	export const npm_package_json: string;
	export const TELEGRAM_REQUIRE_MENTION: string;
	export const TERMINAL_DOCKER_IMAGE: string;
	export const HERMES_EXEC_ASK: string;
	export const TERMINAL_DOCKER_MOUNT_CWD_TO_WORKSPACE: string;
	export const TERMINAL_CONTAINER_MEMORY: string;
	export const MANAGERPID: string;
	export const SSL_CERT_FILE: string;
	export const TERMINAL_CONTAINER_PERSISTENT: string;
	export const npm_config_userconfig: string;
	export const npm_config_local_prefix: string;
	export const HERMES_SESSION_USER_ID: string;
	export const SYSTEMD_EXEC_PID: string;
	export const HERMES_SESSION_CHAT_NAME: string;
	export const GSM_SKIP_SSH_AGENT_WORKAROUND: string;
	export const TERMINAL_MODAL_IMAGE: string;
	export const DBUS_SESSION_BUS_ADDRESS: string;
	export const HERMES_TELEGRAM_DISABLE_FALLBACK_IPS: string;
	export const COLOR: string;
	export const TERMINAL_DOCKER_VOLUMES: string;
	export const TERMINAL_ENV: string;
	export const HERMES_SESSION_MESSAGE_ID: string;
	export const LOGNAME: string;
	export const HERMES_SESSION_THREAD_ID: string;
	export const IMAGE_TOOLS_DEBUG: string;
	export const JOURNAL_STREAM: string;
	export const _: string;
	export const npm_config_prefix: string;
	export const npm_config_npm_version: string;
	export const group_sessions_per_user: string;
	export const MEMORY_PRESSURE_WATCH: string;
	export const BROWSERBASE_ADVANCED_STEALTH: string;
	export const TERMINAL_DAYTONA_IMAGE: string;
	export const npm_config_cache: string;
	export const HERMES_SESSION_CHAT_ID: string;
	export const TERMINAL_LIFETIME_SECONDS: string;
	export const TERMINAL_TIMEOUT: string;
	export const HERMES_GATEWAY_BUSY_INPUT_MODE: string;
	export const npm_config_node_gyp: string;
	export const PATH: string;
	export const TELEGRAM_REACTIONS: string;
	export const INVOCATION_ID: string;
	export const NODE: string;
	export const npm_package_name: string;
	export const XDG_RUNTIME_DIR: string;
	export const LANG: string;
	export const HERMES_QUIET: string;
	export const HERMES_SESSION_PROFILE: string;
	export const MOA_TOOLS_DEBUG: string;
	export const HERMES_MAX_ITERATIONS: string;
	export const npm_lifecycle_script: string;
	export const SSH_AUTH_SOCK: string;
	export const HERMES_SESSION_ID: string;
	export const DEBIAN_FRONTEND: string;
	export const SHELL: string;
	export const npm_package_version: string;
	export const npm_lifecycle_event: string;
	export const QT_ACCESSIBILITY: string;
	export const WEB_TOOLS_DEBUG: string;
	export const VISION_TOOLS_DEBUG: string;
	export const HERMES_SESSION_PLATFORM: string;
	export const HERMES_TELEGRAM_HTTP_CONNECT_TIMEOUT: string;
	export const TERMINAL_CONTAINER_DISK: string;
	export const npm_config_globalconfig: string;
	export const npm_config_init_module: string;
	export const PWD: string;
	export const _config_version: string;
	export const npm_execpath: string;
	export const TELEGRAM_HOME_CHANNEL_THREAD_ID: string;
	export const XDG_DATA_DIRS: string;
	export const npm_config_global_prefix: string;
	export const npm_command: string;
	export const BROWSER_INACTIVITY_TIMEOUT: string;
	export const MEMORY_PRESSURE_WRITE: string;
	export const TERMINAL_SINGULARITY_IMAGE: string;
	export const TERMINAL_HOME_MODE: string;
	export const HERMES_SESSION_SOURCE: string;
	export const INIT_CWD: string;
	export const EDITOR: string;
	export const NODE_ENV: string;
}

/**
 * This module provides access to environment variables that are injected _statically_ into your bundle at build time and are _publicly_ accessible.
 * 
 * |         | Runtime                                                                    | Build time                                                               |
 * | ------- | -------------------------------------------------------------------------- | ------------------------------------------------------------------------ |
 * | Private | [`$env/dynamic/private`](https://svelte.dev/docs/kit/$env-dynamic-private) | [`$env/static/private`](https://svelte.dev/docs/kit/$env-static-private) |
 * | Public  | [`$env/dynamic/public`](https://svelte.dev/docs/kit/$env-dynamic-public)   | [`$env/static/public`](https://svelte.dev/docs/kit/$env-static-public)   |
 * 
 * Static environment variables are [loaded by Vite](https://vitejs.dev/guide/env-and-mode.html#env-files) from `.env` files and `process.env` at build time and then statically injected into your bundle at build time, enabling optimisations like dead code elimination.
 * 
 * **_Public_ access:**
 * 
 * - This module _can_ be imported into client-side code
 * - **Only** variables that begin with [`config.kit.env.publicPrefix`](https://svelte.dev/docs/kit/configuration#env) (which defaults to `PUBLIC_`) are included
 * 
 * For example, given the following build time environment:
 * 
 * ```env
 * ENVIRONMENT=production
 * PUBLIC_BASE_URL=http://site.com
 * ```
 * 
 * With the default `publicPrefix` and `privatePrefix`:
 * 
 * ```ts
 * import { ENVIRONMENT, PUBLIC_BASE_URL } from '$env/static/public';
 * 
 * console.log(ENVIRONMENT); // => throws error during build
 * console.log(PUBLIC_BASE_URL); // => "http://site.com"
 * ```
 * 
 * The above values will be the same _even if_ different values for `ENVIRONMENT` or `PUBLIC_BASE_URL` are set at runtime, as they are statically replaced in your code with their build time values.
 */
declare module '$env/static/public' {
	
}

/**
 * This module provides access to environment variables set _dynamically_ at runtime and that are limited to _private_ access.
 * 
 * |         | Runtime                                                                    | Build time                                                               |
 * | ------- | -------------------------------------------------------------------------- | ------------------------------------------------------------------------ |
 * | Private | [`$env/dynamic/private`](https://svelte.dev/docs/kit/$env-dynamic-private) | [`$env/static/private`](https://svelte.dev/docs/kit/$env-static-private) |
 * | Public  | [`$env/dynamic/public`](https://svelte.dev/docs/kit/$env-dynamic-public)   | [`$env/static/public`](https://svelte.dev/docs/kit/$env-static-public)   |
 * 
 * Dynamic environment variables are defined by the platform you're running on. For example if you're using [`adapter-node`](https://github.com/sveltejs/kit/tree/main/packages/adapter-node) (or running [`vite preview`](https://svelte.dev/docs/kit/cli)), this is equivalent to `process.env`.
 * 
 * **_Private_ access:**
 * 
 * - This module cannot be imported into client-side code
 * - This module includes variables that _do not_ begin with [`config.kit.env.publicPrefix`](https://svelte.dev/docs/kit/configuration#env) _and do_ start with [`config.kit.env.privatePrefix`](https://svelte.dev/docs/kit/configuration#env) (if configured)
 * 
 * > [!NOTE] In `dev`, `$env/dynamic` includes environment variables from `.env`. In `prod`, this behavior will depend on your adapter.
 * 
 * > [!NOTE] To get correct types, environment variables referenced in your code should be declared (for example in an `.env` file), even if they don't have a value until the app is deployed:
 * >
 * > ```env
 * > MY_FEATURE_FLAG=
 * > ```
 * >
 * > You can override `.env` values from the command line like so:
 * >
 * > ```sh
 * > MY_FEATURE_FLAG="enabled" npm run dev
 * > ```
 * 
 * For example, given the following runtime environment:
 * 
 * ```env
 * ENVIRONMENT=production
 * PUBLIC_BASE_URL=http://site.com
 * ```
 * 
 * With the default `publicPrefix` and `privatePrefix`:
 * 
 * ```ts
 * import { env } from '$env/dynamic/private';
 * 
 * console.log(env.ENVIRONMENT); // => "production"
 * console.log(env.PUBLIC_BASE_URL); // => undefined
 * ```
 */
declare module '$env/dynamic/private' {
	export const env: {
		BROWSERBASE_PROXIES: string;
		_HERMES_GATEWAY: string;
		HERMES_SESSION_KEY: string;
		USER: string;
		npm_config_user_agent: string;
		TERMINAL_CONTAINER_CPU: string;
		HERMES_SESSION_USER_NAME: string;
		TERMINAL_CWD: string;
		HERMES_REAL_HOME: string;
		npm_node_execpath: string;
		SHLVL: string;
		npm_config_noproxy: string;
		HOME: string;
		HERMES_UI_SESSION_ID: string;
		OLDPWD: string;
		TERMINAL_DOCKER_FORWARD_ENV: string;
		BROWSER_SESSION_TIMEOUT: string;
		HERMES_HOME: string;
		npm_package_json: string;
		TELEGRAM_REQUIRE_MENTION: string;
		TERMINAL_DOCKER_IMAGE: string;
		HERMES_EXEC_ASK: string;
		TERMINAL_DOCKER_MOUNT_CWD_TO_WORKSPACE: string;
		TERMINAL_CONTAINER_MEMORY: string;
		MANAGERPID: string;
		SSL_CERT_FILE: string;
		TERMINAL_CONTAINER_PERSISTENT: string;
		npm_config_userconfig: string;
		npm_config_local_prefix: string;
		HERMES_SESSION_USER_ID: string;
		SYSTEMD_EXEC_PID: string;
		HERMES_SESSION_CHAT_NAME: string;
		GSM_SKIP_SSH_AGENT_WORKAROUND: string;
		TERMINAL_MODAL_IMAGE: string;
		DBUS_SESSION_BUS_ADDRESS: string;
		HERMES_TELEGRAM_DISABLE_FALLBACK_IPS: string;
		COLOR: string;
		TERMINAL_DOCKER_VOLUMES: string;
		TERMINAL_ENV: string;
		HERMES_SESSION_MESSAGE_ID: string;
		LOGNAME: string;
		HERMES_SESSION_THREAD_ID: string;
		IMAGE_TOOLS_DEBUG: string;
		JOURNAL_STREAM: string;
		_: string;
		npm_config_prefix: string;
		npm_config_npm_version: string;
		group_sessions_per_user: string;
		MEMORY_PRESSURE_WATCH: string;
		BROWSERBASE_ADVANCED_STEALTH: string;
		TERMINAL_DAYTONA_IMAGE: string;
		npm_config_cache: string;
		HERMES_SESSION_CHAT_ID: string;
		TERMINAL_LIFETIME_SECONDS: string;
		TERMINAL_TIMEOUT: string;
		HERMES_GATEWAY_BUSY_INPUT_MODE: string;
		npm_config_node_gyp: string;
		PATH: string;
		TELEGRAM_REACTIONS: string;
		INVOCATION_ID: string;
		NODE: string;
		npm_package_name: string;
		XDG_RUNTIME_DIR: string;
		LANG: string;
		HERMES_QUIET: string;
		HERMES_SESSION_PROFILE: string;
		MOA_TOOLS_DEBUG: string;
		HERMES_MAX_ITERATIONS: string;
		npm_lifecycle_script: string;
		SSH_AUTH_SOCK: string;
		HERMES_SESSION_ID: string;
		DEBIAN_FRONTEND: string;
		SHELL: string;
		npm_package_version: string;
		npm_lifecycle_event: string;
		QT_ACCESSIBILITY: string;
		WEB_TOOLS_DEBUG: string;
		VISION_TOOLS_DEBUG: string;
		HERMES_SESSION_PLATFORM: string;
		HERMES_TELEGRAM_HTTP_CONNECT_TIMEOUT: string;
		TERMINAL_CONTAINER_DISK: string;
		npm_config_globalconfig: string;
		npm_config_init_module: string;
		PWD: string;
		_config_version: string;
		npm_execpath: string;
		TELEGRAM_HOME_CHANNEL_THREAD_ID: string;
		XDG_DATA_DIRS: string;
		npm_config_global_prefix: string;
		npm_command: string;
		BROWSER_INACTIVITY_TIMEOUT: string;
		MEMORY_PRESSURE_WRITE: string;
		TERMINAL_SINGULARITY_IMAGE: string;
		TERMINAL_HOME_MODE: string;
		HERMES_SESSION_SOURCE: string;
		INIT_CWD: string;
		EDITOR: string;
		NODE_ENV: string;
		[key: `PUBLIC_${string}`]: undefined;
		[key: `${string}`]: string | undefined;
	}
}

/**
 * This module provides access to environment variables set _dynamically_ at runtime and that are _publicly_ accessible.
 * 
 * |         | Runtime                                                                    | Build time                                                               |
 * | ------- | -------------------------------------------------------------------------- | ------------------------------------------------------------------------ |
 * | Private | [`$env/dynamic/private`](https://svelte.dev/docs/kit/$env-dynamic-private) | [`$env/static/private`](https://svelte.dev/docs/kit/$env-static-private) |
 * | Public  | [`$env/dynamic/public`](https://svelte.dev/docs/kit/$env-dynamic-public)   | [`$env/static/public`](https://svelte.dev/docs/kit/$env-static-public)   |
 * 
 * Dynamic environment variables are defined by the platform you're running on. For example if you're using [`adapter-node`](https://github.com/sveltejs/kit/tree/main/packages/adapter-node) (or running [`vite preview`](https://svelte.dev/docs/kit/cli)), this is equivalent to `process.env`.
 * 
 * **_Public_ access:**
 * 
 * - This module _can_ be imported into client-side code
 * - **Only** variables that begin with [`config.kit.env.publicPrefix`](https://svelte.dev/docs/kit/configuration#env) (which defaults to `PUBLIC_`) are included
 * 
 * > [!NOTE] In `dev`, `$env/dynamic` includes environment variables from `.env`. In `prod`, this behavior will depend on your adapter.
 * 
 * > [!NOTE] To get correct types, environment variables referenced in your code should be declared (for example in an `.env` file), even if they don't have a value until the app is deployed:
 * >
 * > ```env
 * > MY_FEATURE_FLAG=
 * > ```
 * >
 * > You can override `.env` values from the command line like so:
 * >
 * > ```sh
 * > MY_FEATURE_FLAG="enabled" npm run dev
 * > ```
 * 
 * For example, given the following runtime environment:
 * 
 * ```env
 * ENVIRONMENT=production
 * PUBLIC_BASE_URL=http://example.com
 * ```
 * 
 * With the default `publicPrefix` and `privatePrefix`:
 * 
 * ```ts
 * import { env } from '$env/dynamic/public';
 * console.log(env.ENVIRONMENT); // => undefined, not public
 * console.log(env.PUBLIC_BASE_URL); // => "http://example.com"
 * ```
 * 
 * ```
 * 
 * ```
 */
declare module '$env/dynamic/public' {
	export const env: {
		[key: `PUBLIC_${string}`]: string | undefined;
	}
}
