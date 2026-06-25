import assert from "node:assert/strict";
import { after, test } from "node:test";
import { readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { pathToFileURL } from "node:url";
import ts from "typescript";

const source = readFileSync(new URL("../src/previewScheduler.ts", import.meta.url), "utf8");
const output = ts.transpileModule(source, {
  compilerOptions: {
    module: ts.ModuleKind.ES2020,
    target: ts.ScriptTarget.ES2020,
    isolatedModules: true
  }
}).outputText;
const compiledPath = join(tmpdir(), `pagepilot-preview-scheduler-${Date.now()}.mjs`);
writeFileSync(compiledPath, output);
const { createPreviewScheduler } = await import(pathToFileURL(compiledPath).href);

after(() => {
  rmSync(compiledPath, { force: true });
});

test("预览调度器限制并发并在释放后启动下一个任务", () => {
  const scheduler = createPreviewScheduler(2);
  const started = [];
  const releases = [];

  for (const id of ["a", "b", "c"]) {
    scheduler.enqueue((release) => {
      started.push(id);
      releases.push(release);
    });
  }

  assert.deepEqual(started, ["a", "b"]);
  assert.equal(scheduler.getSnapshot().active, 2);
  assert.equal(scheduler.getSnapshot().queued, 1);

  releases[0]();

  assert.deepEqual(started, ["a", "b", "c"]);
  assert.equal(scheduler.getSnapshot().active, 2);
  assert.equal(scheduler.getSnapshot().queued, 0);
});

test("取消排队任务时不会再启动该任务", () => {
  const scheduler = createPreviewScheduler(1);
  const started = [];
  let releaseFirst = () => {};

  scheduler.enqueue((release) => {
    started.push("first");
    releaseFirst = release;
  });
  const cancelSecond = scheduler.enqueue(() => {
    started.push("second");
  });

  cancelSecond();
  releaseFirst();

  assert.deepEqual(started, ["first"]);
  assert.equal(scheduler.getSnapshot().active, 0);
  assert.equal(scheduler.getSnapshot().queued, 0);
});

test("取消运行中的任务会释放槽位", () => {
  const scheduler = createPreviewScheduler(1);
  const started = [];

  const cancelFirst = scheduler.enqueue(() => {
    started.push("first");
  });
  scheduler.enqueue(() => {
    started.push("second");
  });

  cancelFirst();

  assert.deepEqual(started, ["first", "second"]);
  assert.equal(scheduler.getSnapshot().active, 1);
  assert.equal(scheduler.getSnapshot().queued, 0);
});
