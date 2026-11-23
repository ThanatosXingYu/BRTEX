// Background browser tabs must not keep downloading snapshots. A poll that
// returns a promise completes before the next interval starts; focus refreshes
// immediately, without creating a second request while one is still running.
export function setVisibleInterval(callback: () => unknown | Promise<unknown>, delay: number): () => void {
  let stopped = false;
  let running = false;
  let timer: ReturnType<typeof setTimeout> | undefined;
  const visible = () => typeof document === "undefined" || document.visibilityState !== "hidden";
  const clear = () => {
    if (timer !== undefined) clearTimeout(timer);
    timer = undefined;
  };
  const schedule = () => {
    clear();
    if (!stopped && visible()) timer = setTimeout(() => void run(), delay);
  };
  const run = async () => {
    clear();
    if (stopped || running || !visible()) return;
    running = true;
    try {
      await callback();
    } catch {
      // Existing loaders own their error UI; transient polling failures retry.
    } finally {
      running = false;
      schedule();
    }
  };
  const onVisibility = () => {
    clear();
    if (visible()) void run();
  };
  if (typeof document !== "undefined") document.addEventListener("visibilitychange", onVisibility);
  schedule();
  return () => {
    stopped = true;
    clear();
    if (typeof document !== "undefined") document.removeEventListener("visibilitychange", onVisibility);
  };
}
