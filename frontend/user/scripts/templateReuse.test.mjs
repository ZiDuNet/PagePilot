import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

const source = readFileSync(new URL("../src/App.tsx", import.meta.url), "utf8");

test("市场复用抽屉展示源文件结构并分离 MCP 参数", () => {
  assert.match(source, /const sourceFiles = /, "缺少源文件列表汇总");
  assert.match(source, /const sourceSummary = /, "缺少源文件结构摘要");
  assert.match(source, /源文件结构/, "抽屉缺少源文件结构区块");
  assert.match(source, /source-structure-panel/, "缺少源文件结构样式钩子");
  assert.match(source, /复制 MCP/, "缺少独立 MCP 复制按钮");
  assert.match(source, /DocBlock title="MCP 参数"/, "缺少 MCP 参数独立代码块");
  assert.match(source, /item\.reuse\?\.mcp/, "MCP 参数必须来自服务端复用详情");
});
