// User preferences persisted in localStorage
import { writable } from "svelte/store";
import { browser } from "$app/environment";

export type EditorTheme =
  | "github-dark"
  | "monokai"
  | "dracula"
  | "nord"
  | "one-dark"
  | "tokyo-night"
  | "ayu-dark"
  | "solarized-dark"
  | "solarized-light"
  | "catppuccin"
  | "gruvbox-dark"
  | "vscode-dark"
  | "light";

export interface Preferences {
  editorTheme: EditorTheme;
  editorFontSize: number;
}

const STORAGE_KEY = "sigarda_preferences";

const defaults: Preferences = {
  editorTheme: "github-dark",
  editorFontSize: 13,
};

function load(): Preferences {
  if (!browser) return defaults;
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (raw) return { ...defaults, ...JSON.parse(raw) };
  } catch {}
  return defaults;
}

function createPrefs() {
  const stored = load();
  const { subscribe, set, update } = writable<Preferences>(stored);

  return {
    subscribe,
    set(prefs: Preferences) {
      if (browser) localStorage.setItem(STORAGE_KEY, JSON.stringify(prefs));
      set(prefs);
    },
    update(fn: (p: Preferences) => Preferences) {
      update((current) => {
        const next = fn(current);
        if (browser) localStorage.setItem(STORAGE_KEY, JSON.stringify(next));
        return next;
      });
    },
  };
}

export const prefs = createPrefs();

export const editorThemes: {
  id: EditorTheme;
  name: string;
  bg: string;
  preview: string[];
}[] = [
  // Dark themes
  {
    id: "github-dark",
    name: "GitHub Dark",
    bg: "#0d1117",
    preview: ["#79c0ff", "#d2a8ff", "#7ee787", "#ffa657", "#ff7b72"],
  },
  {
    id: "vscode-dark",
    name: "VS Code Dark",
    bg: "#1e1e1e",
    preview: ["#569cd6", "#c586c0", "#6a9955", "#dcdcaa", "#ce9178"],
  },
  {
    id: "tokyo-night",
    name: "Tokyo Night",
    bg: "#1a1b26",
    preview: ["#7aa2f7", "#bb9af7", "#9ece6a", "#e0af68", "#f7768e"],
  },
  {
    id: "catppuccin",
    name: "Catppuccin Mocha",
    bg: "#1e1e2e",
    preview: ["#89b4fa", "#cba6f7", "#a6e3a1", "#f9e2af", "#f38ba8"],
  },
  {
    id: "ayu-dark",
    name: "Ayu Dark",
    bg: "#0d1017",
    preview: ["#39bae6", "#d2a6ff", "#7fd962", "#ffb454", "#ff6b68"],
  },
  {
    id: "gruvbox-dark",
    name: "Gruvbox Dark",
    bg: "#282828",
    preview: ["#83a598", "#d3869b", "#b8bb26", "#fabd2f", "#fb4934"],
  },
  {
    id: "solarized-dark",
    name: "Solarized Dark",
    bg: "#002b36",
    preview: ["#268bd2", "#6c71c4", "#859900", "#b58900", "#dc322f"],
  },
  {
    id: "monokai",
    name: "Monokai",
    bg: "#272822",
    preview: ["#f92672", "#a6e22e", "#e6db74", "#66d9ef", "#ae81ff"],
  },
  {
    id: "dracula",
    name: "Dracula",
    bg: "#282a36",
    preview: ["#ff79c6", "#50fa7b", "#f1fa8c", "#8be9fd", "#bd93f9"],
  },
  {
    id: "nord",
    name: "Nord",
    bg: "#2e3440",
    preview: ["#bf616a", "#a3be8c", "#ebcb8b", "#81a1c1", "#b48ead"],
  },
  {
    id: "one-dark",
    name: "One Dark",
    bg: "#282c34",
    preview: ["#e06c75", "#98c379", "#e5c07b", "#61afef", "#c678dd"],
  },
  // Light themes
  {
    id: "solarized-light",
    name: "Solarized Light",
    bg: "#fdf6e3",
    preview: ["#268bd2", "#6c71c4", "#859900", "#b58900", "#dc322f"],
  },
  {
    id: "light",
    name: "Light",
    bg: "#ffffff",
    preview: ["#0451a5", "#001080", "#008000", "#795e26", "#a31515"],
  },
];
