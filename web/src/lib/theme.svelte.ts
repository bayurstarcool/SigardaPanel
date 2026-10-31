type Theme = "dark" | "light";

// app.html applies the saved (or system) theme before first paint; the store
// starts from whatever class is already on <html> so the two never disagree.
function initialTheme(): Theme {
  if (typeof document === "undefined") return "dark";
  return document.documentElement.classList.contains("light") ? "light" : "dark";
}

function createThemeStore() {
  let theme = $state<Theme>(initialTheme());

  function applyTheme(t: Theme) {
    const html = document.documentElement;
    html.classList.toggle("light", t === "light");
    html.classList.toggle("dark", t === "dark");
  }

  function set(t: Theme) {
    theme = t;
    try {
      localStorage.setItem("sigarda_theme", t);
    } catch {
      // storage unavailable (private mode) — theme still applies for this visit
    }
    applyTheme(t);
  }

  function toggle() {
    set(theme === "dark" ? "light" : "dark");
  }

  function init() {
    theme = initialTheme();
    applyTheme(theme);
  }

  return {
    get current() {
      return theme;
    },
    get isDark() {
      return theme === "dark";
    },
    get isLight() {
      return theme === "light";
    },
    toggle,
    set,
    init,
  };
}

export const theme = createThemeStore();
