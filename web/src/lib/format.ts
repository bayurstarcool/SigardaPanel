type DateInput = string | number | Date | null | undefined;

/**
 * Parse a timestamp from the API into a Date, or null when it is missing or
 * unparseable. Tolerates every shape the backend emits:
 *  - RFC 3339 / ISO ("2026-12-04T11:41:55Z", with fractional seconds/offset)
 *  - SQLite CURRENT_TIMESTAMP ("2026-12-04 11:41:55", always UTC)
 *  - Go's time.Time.String() ("2026-12-04 11:41:55.264167352 +0000 UTC m=+7776485.03")
 *  - unix seconds or milliseconds (numbers)
 */
export function parseDate(value: DateInput): Date | null {
  if (value === null || value === undefined || value === "") return null;
  if (value instanceof Date) return isNaN(value.getTime()) ? null : value;
  if (typeof value === "number") {
    if (!isFinite(value) || value <= 0) return null;
    // Anything below 1e11 is treated as unix seconds.
    return new Date(value < 1e11 ? value * 1000 : value);
  }
  let s = String(value).trim();
  if (!s || s.startsWith("0001-01-01")) return null; // Go zero time
  // Go monotonic clock suffix: " m=+7776485.03"
  s = s.replace(/\s+m=[+-][\d.]+$/, "");
  const m = s.match(
    /^(\d{4}-\d{2}-\d{2})[T ](\d{2}:\d{2}(?::\d{2})?)(\.\d+)?\s*(Z|[+-]\d{2}:?\d{2})?(?:\s+[A-Z]{2,5})?$/,
  );
  if (m) {
    const [, date, time, frac, zone] = m;
    // JS keeps milliseconds only; Go emits up to nanoseconds.
    const ms = frac ? frac.slice(0, 4).padEnd(4, "0") : "";
    let tz = zone || "Z"; // no zone = SQLite UTC
    if (/^[+-]\d{4}$/.test(tz)) tz = `${tz.slice(0, 3)}:${tz.slice(3)}`;
    const d = new Date(`${date}T${time.length === 5 ? time + ":00" : time}${ms}${tz}`);
    return isNaN(d.getTime()) ? null : d;
  }
  const d = new Date(s);
  return isNaN(d.getTime()) ? null : d;
}

export function fmtRelative(value?: DateInput): string {
  const d = parseDate(value);
  if (!d) return "—";
  const diff = Math.round((Date.now() - d.getTime()) / 1000);
  const future = diff < 0;
  const s = Math.abs(diff);
  let out: string;
  if (s < 60) out = `${s}s`;
  else if (s < 3600) out = `${Math.round(s / 60)}m`;
  else if (s < 86400) out = `${Math.round(s / 3600)}h`;
  else out = `${Math.round(s / 86400)}d`;
  return future ? `in ${out}` : `${out} ago`;
}

export function fmtDate(value?: DateInput): string {
  const d = parseDate(value);
  if (!d) return "—";
  return d.toLocaleDateString("id-ID", {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}

export function fmtDateTime(value?: DateInput): string {
  const d = parseDate(value);
  if (!d) return "—";
  return d.toLocaleString("id-ID", {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

/** Whole days from now until value (negative once it has passed), or null. */
export function daysUntil(value?: DateInput): number | null {
  const d = parseDate(value);
  if (!d) return null;
  return Math.floor((d.getTime() - Date.now()) / 864e5);
}

export type ExpiryTone = "ok" | "warn" | "danger" | "expired";

/** Certificate expiry tone: expired (past), danger (<7d), warn (<30d), ok. */
export function expiryTone(value?: DateInput): ExpiryTone | null {
  const d = parseDate(value);
  if (!d) return null;
  const ms = d.getTime() - Date.now();
  if (ms <= 0) return "expired";
  if (ms < 7 * 864e5) return "danger";
  if (ms < 30 * 864e5) return "warn";
  return "ok";
}

/** Short human label for days left until an expiry date. */
export function fmtDaysLeft(value?: DateInput): string {
  const d = parseDate(value);
  if (!d) return "";
  const ms = d.getTime() - Date.now();
  if (ms <= 0) {
    const over = Math.floor(-ms / 864e5);
    return over === 0 ? "expired today" : `expired ${over}d ago`;
  }
  const days = Math.floor(ms / 864e5);
  return days === 0 ? "< 1d left" : `${days}d left`;
}

export function fmtBytes(bytes?: number): string {
  if (!bytes) return "—";
  const units = ["B", "KB", "MB", "GB"];
  let i = 0;
  let v = bytes;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(1)} ${units[i]}`;
}

export function fmtBytesFull(bytes: number): string {
  if (bytes === 0) return "0 B";
  const sizes = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.min(
    Math.floor(Math.log(bytes) / Math.log(1024)),
    sizes.length - 1,
  );
  const v = bytes / Math.pow(1024, i);
  return v < 10
    ? v.toFixed(1) + " " + sizes[i]
    : Math.round(v) + " " + sizes[i];
}
