export interface ToastItem {
  id: number;
  type: "success" | "error" | "warning" | "info";
  message: string;
  duration: number;
}

let counter = 0;

function fire(detail: ToastItem) {
  window.dispatchEvent(new CustomEvent("toast", { detail }));
}

export const toasts = {
  success: (message: string, duration = 3500) =>
    fire({ id: ++counter, type: "success", message, duration }),
  error: (message: string, duration = 5000) =>
    fire({ id: ++counter, type: "error", message, duration }),
  warning: (message: string, duration = 3500) =>
    fire({ id: ++counter, type: "warning", message, duration }),
  info: (message: string, duration = 3500) =>
    fire({ id: ++counter, type: "info", message, duration }),
};
