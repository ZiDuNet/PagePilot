import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

const source = readFileSync(new URL("../src/App.tsx", import.meta.url), "utf8");

test("后台应用列表支持批量复用策略更新", () => {
  assert.match(source, /const \[selectedSiteCodes, setSelectedSiteCodes\]/);
  assert.match(source, /function toggleVisibleSelection\(\)/);
  assert.match(source, /async function applyBulkReusePolicy\(\)/);
  assert.match(source, /\/api\/admin\/sites\/\$\{encodeURIComponent\(code\)\}\/reuse-policy/);
  assert.match(source, />选择当前筛选结果</);
  assert.match(source, /"应用策略"/);
});
