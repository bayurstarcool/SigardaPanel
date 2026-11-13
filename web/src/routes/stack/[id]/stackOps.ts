import { apiFetch } from '$lib/api';

/** A stack action as the API accepts it. */
export type StackActionBody = {
  package: string;
  action: string;
  version?: string;
  extension?: string;
};

/** Progress/result of a package operation (agent background operation). */
export type StackOp = {
  op_id?: string;
  package: string;
  action: string;
  version?: string;
  extension?: string;
  status: 'running' | 'succeeded' | 'failed';
  ok: boolean;
  done: boolean;
  message: string;
  output?: string;
  duration_ms?: number;
  info?: any;
};

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

/**
 * Polls a background operation until it is done. Transient polling errors
 * (agent restarting, proxy hiccup) are retried for a while before giving up.
 */
export async function pollStackOp(
  token: string | null,
  serverId: string,
  opId: string,
  onProgress?: (op: StackOp) => void,
): Promise<StackOp> {
  let failures = 0;
  for (;;) {
    try {
      const op: StackOp = await apiFetch(token, 'GET', `servers/${serverId}/stack/ops/${opId}`);
      failures = 0;
      onProgress?.(op);
      if (op.done) return op;
    } catch (e: any) {
      failures++;
      if (failures >= 20 || /not found/i.test(e?.message ?? '')) {
        throw new Error(
          `Lost track of the operation (${e?.message ?? 'error'}). It may still be running on the server — refresh in a minute.`,
        );
      }
    }
    await sleep(2000);
  }
}

/**
 * Runs a stack action. Package operations (install/uninstall/update/
 * extensions) come back as a background operation that is polled until it
 * finishes; service actions answer directly. The result is only "ok" when the
 * agent verified the new state.
 */
export async function runStackAction(
  token: string | null,
  serverId: string,
  body: StackActionBody,
  onProgress?: (op: StackOp) => void,
): Promise<StackOp> {
  const res: any = await apiFetch(token, 'POST', `servers/${serverId}/stack/action`, body);
  if (res?.op_id) {
    onProgress?.(res as StackOp);
    return pollStackOp(token, serverId, res.op_id, onProgress);
  }
  return {
    package: body.package,
    action: body.action,
    version: body.version,
    status: res?.ok ? 'succeeded' : 'failed',
    ok: !!res?.ok,
    done: true,
    message: res?.message || `${body.action} ${body.package} ${res?.ok ? 'done' : 'failed'}`,
    output: res?.output || '',
    info: res?.info,
  };
}

export function statusBadge(status: string | undefined): { label: string; badge: string } {
  switch (status) {
    case 'running':
      return { label: 'Running', badge: 'badge-success' };
    case 'starting':
      return { label: 'Starting', badge: 'badge-warning' };
    case 'installed':
      return { label: 'Installed', badge: 'badge-info' };
    case 'stopped':
      return { label: 'Stopped', badge: 'badge-warning' };
    case 'failed':
      return { label: 'Failed', badge: 'badge-danger' };
    case 'missing':
      return { label: 'Not installed', badge: '' };
    default:
      return { label: 'Unknown', badge: '' };
  }
}

export function describeAction(body: StackActionBody): string {
  const what = [body.package === 'php' ? 'PHP' : body.package, body.version, body.extension].filter(Boolean).join(' ');
  const verb: Record<string, string> = {
    install: 'Installing',
    uninstall: 'Uninstalling',
    update: 'Updating',
    'install-extension': 'Installing extension',
    'uninstall-extension': 'Removing extension',
    restart: 'Restarting',
    start: 'Starting',
    stop: 'Stopping',
    reload: 'Reloading',
    'set-default': 'Setting default CLI to',
  };
  return `${verb[body.action] ?? body.action} ${what}`;
}
