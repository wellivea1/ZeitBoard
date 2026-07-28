export interface CoalescedRefresh {
  request: () => void;
  supersede: () => void;
  dispose: () => void;
}

export function createCoalescedRefresh<T>(
  load: () => Promise<T>,
  onSuccess: (value: T) => void,
  onError?: (reason: unknown) => void,
): CoalescedRefresh {
  let active = true;
  let running = false;
  let rerun = false;
  let revision = 0;

  const run = async (runRevision: number) => {
    try {
      const value = await load();
      if (active && runRevision === revision) onSuccess(value);
    } catch (reason) {
      if (active && runRevision === revision) onError?.(reason);
    } finally {
      running = false;
      if (active && rerun) {
        rerun = false;
        running = true;
        void run(revision);
      }
    }
  };

  return {
    request: () => {
      if (!active) return;
      revision += 1;
      if (running) {
        rerun = true;
        return;
      }
      running = true;
      void run(revision);
    },
    supersede: () => {
      if (!active) return;
      revision += 1;
      rerun = false;
    },
    dispose: () => {
      active = false;
      rerun = false;
      revision += 1;
    },
  };
}
