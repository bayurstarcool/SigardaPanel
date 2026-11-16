export type Result = {
  columns: string[];
  rows: (string | null)[][];
  has_more: boolean;
  truncated?: boolean;
  elapsed_ms?: number;
};

/** One column of a table, from the viewer's "columns" action. */
export type Column = { name: string; type: string; nullable: boolean; key: string; default: string | null; extra: string };

export type TableInfo = { schema: string; name: string; type: string; rows: number; size: number };

// Values longer than this many bytes are cut by the agent (viewerMaxCell)
// and end with "…"; binary values come as "[binary, N bytes]".
const MAX_CELL = 10 << 10;

/** A value shown in the grid that is not the full stored value. */
export function isPartial(v: string | null): boolean {
  if (v === null) return false;
  if (/^\[binary, \d+ bytes\]$/.test(v)) return true;
  return v.endsWith('…') && new TextEncoder().encode(v).length > MAX_CELL;
}

export function toColumns(r: Result): Column[] {
  const idx = (n: string) => r.columns.indexOf(n);
  return r.rows.map((row) => ({
    name: row[idx('name')] as string,
    type: (row[idx('type')] as string) || '',
    nullable: String(row[idx('nullable')] ?? '').toUpperCase() === 'YES',
    key: (row[idx('key')] as string) || '',
    default: row[idx('default')] ?? null,
    extra: (row[idx('extra')] as string) || '',
  }));
}

/** Rough guess whether a console statement changes something (asks for a confirmation). */
export function looksLikeWrite(sql: string): boolean {
  const s = sql.replace(/\/\*[\s\S]*?\*\//g, ' ').replace(/(--|#)[^\n]*/g, ' ').trim().toLowerCase();
  const first = s.match(/^[a-z]+/)?.[0] ?? '';
  if (first === 'with') return /\b(insert|update|delete|merge)\b/.test(s);
  return !['select', 'show', 'describe', 'desc', 'explain', 'values', 'table'].includes(first);
}
