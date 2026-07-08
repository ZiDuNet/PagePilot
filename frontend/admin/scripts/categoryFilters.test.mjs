import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

const source = readFileSync(new URL("../src/App.tsx", import.meta.url), "utf8");

test("后台应用分类筛选包含未分类选项", () => {
  assert.match(source, /const UNCATEGORIZED_CATEGORY_FILTER\s*=\s*"__uncategorized"/);
  assert.match(source, /value=\{UNCATEGORIZED_CATEGORY_FILTER\}>未分类<\/option>/);
  assert.match(source, /category === UNCATEGORIZED_CATEGORY_FILTER/);
});

test("后台应用分类管理把分类排序放在底部区域", () => {
  assert.match(source, /className="category-sort-section"/);
  assert.match(source, />分类排序</);
  assert.match(source, /function moveCategory/);
  assert.match(source, />上移<\/button>/);
  assert.match(source, />下移<\/button>/);
});
