export type SavePhase = 'idle' | 'pending' | 'saving' | 'saved' | 'error' | 'conflict';
export interface SaveStatus {phase: SavePhase; detail: string;}
export interface SaveResult<T> {value: T; version: number;}

/** One active client, one request at a time. A failed request never advances the baseline. */
export function createOperationsAutosave<T, R extends SaveResult<T>>(options: {
  save: (key: string, value: T, version: number) => Promise<R>;
  accepted: (key: string, result: R, unchanged: boolean) => void;
  status: (value: SaveStatus) => void;
  delay?: number;
}) {
  let key = '', version = 0, baseline = '', latest = '';
  let timer: ReturnType<typeof setTimeout> | undefined;
  let running: Promise<void> | undefined;
  let failure: Error | undefined;
  let conflicted = false, disposed = false, applied = false;
  const report = (phase: SavePhase, detail = '') => {if (!disposed) options.status({phase, detail});};
  const clear = () => {if (timer) clearTimeout(timer); timer = undefined;};
  const pending = () => !!key && latest !== baseline;
  function schedule() {
    clear();
    if (disposed || conflicted || running || !pending()) return;
    timer = setTimeout(() => {timer = undefined; void flush().catch(() => {});}, options.delay ?? 650);
  }
  async function run() {
    while (!disposed && pending()) {
      const submitted = latest, client = key;
      report('saving');
      try {
        const result = await options.save(client, JSON.parse(submitted) as T, version);
        if (disposed) return;
        const unchanged = latest === submitted;
        baseline = JSON.stringify(result.value);
        version = result.version;
        applied = true;
        if (unchanged) latest = baseline;
        failure = undefined;
        options.accepted(client, result, unchanged);
      } catch (cause) {
        if (disposed) return;
        failure = cause instanceof Error ? cause : new Error(String(cause));
        conflicted = (cause as {status?: number})?.status === 409;
        report(conflicted ? 'conflict' : 'error', failure.message);
        throw failure;
      }
    }
    if (!disposed) report('saved');
  }
  function flush(): Promise<void> {
    clear();
    if (disposed || !key) return Promise.resolve();
    if (running) return running;
    if (conflicted) return Promise.reject(failure);
    if (!pending()) return Promise.resolve();
    running = Promise.resolve().then(run).finally(() => {
      running = undefined;
      // Include edits queued between run's last status update and promise settlement.
      if (!disposed && !failure && !conflicted && pending()) return flush();
    });
    return running;
  }
  return {
    reset(client: string, value: T, revision: number) {
      if (running) throw new Error('保存尚未完成，不能切换客户端。');
      clear(); key = client; version = revision;
      baseline = latest = JSON.stringify(value);
      failure = undefined; conflicted = false; applied = false;
      report('idle');
    },
    edit(value: T) {
      if (disposed || !key) return;
      const next = JSON.stringify(value);
      if (next === latest) return;
      latest = next;
      if (conflicted) return;
      failure = undefined;
      report(running ? 'saving' : pending() ? 'pending' : applied ? 'saved' : 'idle');
      schedule();
    },
    flush,
    // Reload may explicitly discard a pending edit, but must await an already dispatched write.
    async pause() {clear(); await running?.catch(() => {}); clear();},
    resume() {if (!failure) schedule();},
    dispose() {disposed = true; clear();},
  };
}
