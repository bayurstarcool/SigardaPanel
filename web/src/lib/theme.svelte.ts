type Theme = "dark" | "light";

function createThemeStore() {
  let theme = $state<Theme>(
    (typeof window !== "undefined" &&
      (localStorage.getItem("sigarda_theme") as Theme)) ||
      "dark",
  );

  function toggle() {
    theme = theme === "dark" ? "light" : "dark";
    localStorage.setItem("sigarda_theme", theme);
    applyTheme(theme);
  }

  function set(t: Theme) {
    theme = t;
    localStorage.setItem("sigarda_theme", theme);
    applyTheme(theme);
  }

  function applyTheme(t: Theme) {
    const html = document.documentElement;
    if (t === "light") {
      html.classList.add("light");
      html.classList.remove("dark");
    } else {
      html.classList.add("dark");
      html.classList.remove("light");
    }
  }

  function init() {
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
