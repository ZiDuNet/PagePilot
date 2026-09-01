import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

const appSource = readFileSync(new URL("../src/App.tsx", import.meta.url), "utf8");
const styles = readFileSync(new URL("../src/styles.css", import.meta.url), "utf8");

test("后台导航标记当前页面并提供跳过链接", () => {
  assert.match(appSource, /className=\"skip-link\" href=\"#admin-main\"/);
  assert.match(appSource, /id=\"admin-main\" tabIndex=\{-1\}/);
  assert.match(appSource, /aria-current=\{activeTab === item\.tab \? \"page\" : undefined\}/);
});

test("后台操作控件提供键盘焦点样式和实时反馈语义", () => {
  assert.match(styles, /:where\(button, a, input, select, textarea, summary, \[tabindex\]\):focus-visible/);
  assert.match(styles, /\.skip-link:focus-visible/);
  assert.match(styles, /prefers-reduced-motion: reduce/);
  assert.match(appSource, /role=\"status\" aria-live=\"polite\" aria-atomic=\"true\"/);
  assert.match(appSource, /className=\"alert error global-alert\" role=\"alert\"/);
  assert.match(appSource, /aria-label=\"关闭\" title=\"关闭\" onClick=\{onClose\}><X size=\{16\} \/>/);
});

test("后台总览在数据请求完成前不会显示误导性的空状态", () => {
  assert.match(appSource, /className=\"page-grid overview-page\" aria-busy=\{loading\}/);
  assert.match(appSource, /value=\{loading \? \"\.\.\.\"/);
  assert.match(appSource, /loading && <tr><td colSpan=\{4\} className=\"table-loading\">正在加载站点/);
  assert.match(appSource, /!loading && !sites\.length && <tr><td colSpan=\{4\}>暂无站点/);
});
