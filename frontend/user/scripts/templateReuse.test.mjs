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
  assert.match(source, /updateTargetCode = item\.code/, "更新已有模式必须自动使用当前作品 code");
  assert.doesNotMatch(source, /填写你拥有的已有发布 code/, "更新已有模式不应再要求手填目标 code");
});

test("源码下载按钮保持可见并用 toast 提示登录", () => {
  assert.match(source, /function friendlyDownloadMessage/, "缺少下载错误友好提示");
  assert.match(source, /请先登录后下载源码/, "匿名下载应提示先登录");
  assert.match(source, /await downloadSourceFile\(downloadURL/, "下载按钮应通过 fetch 捕获 API 错误");
  assert.match(source, /toast\(friendlyDownloadMessage\(err\)\)/, "下载失败应进入右上角 toast");
  assert.match(source, /disabled=\{downloadBusy\}/, "下载按钮只应在请求中防重复点击");
  assert.doesNotMatch(source, /disabled=\{!canDownloadNow\}/, "匿名下载按钮不能因为未登录被隐藏或禁用");
  assert.match(source, /loginRequiredForReuse/, "需要登录的公开作品仍应允许打开复用抽屉");
});
