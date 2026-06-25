type PreviewRelease = () => void;
type PreviewTask = (release: PreviewRelease) => void;

interface QueueItem {
  id: number;
  task: PreviewTask;
  release?: PreviewRelease;
  status: "queued" | "running" | "cancelled" | "completed";
}

export interface PreviewScheduler {
  enqueue: (task: PreviewTask) => () => void;
  getSnapshot: () => { active: number; queued: number };
}

export function createPreviewScheduler(maxActive: number): PreviewScheduler {
  const limit = Math.max(1, Math.floor(maxActive));
  const queue: QueueItem[] = [];
  let active = 0;
  let nextId = 1;

  function drain() {
    while (active < limit) {
      const item = queue.shift();
      if (!item) return;
      if (item.status === "cancelled") continue;

      item.status = "running";
      active += 1;
      let released = false;
      item.release = () => {
        if (released) return;
        released = true;
        item.status = item.status === "cancelled" ? "cancelled" : "completed";
        active = Math.max(0, active - 1);
        drain();
      };
      item.task(item.release);
    }
  }

  return {
    enqueue(task) {
      const item: QueueItem = { id: nextId, task, status: "queued" };
      nextId += 1;
      queue.push(item);
      drain();

      return () => {
        if (item.status === "cancelled") return;
        if (item.status === "queued") {
          item.status = "cancelled";
          const index = queue.findIndex((queued) => queued.id === item.id);
          if (index >= 0) queue.splice(index, 1);
          return;
        }
        item.status = "cancelled";
        item.release?.();
      };
    },
    getSnapshot() {
      return { active, queued: queue.length };
    }
  };
}
