import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

const source = readFileSync(new URL("../src/App.tsx", import.meta.url), "utf8");

test("创作市场应用分类筛选包含未分类入口", () => {
  assert.match(source, /const UNCATEGORIZED_CATEGORY_FILTER\s*=\s*"__uncategorized"/);
  assert.match(source, /label:\s*"未分类"/);
  assert.match(source, /note:\s*"未设置应用分类"/);
  assert.match(source, /params\.set\("category",\s*category\)/);
});

