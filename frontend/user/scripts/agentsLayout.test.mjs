import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

const source = readFileSync(new URL("../src/App.tsx", import.meta.url), "utf8");
const styles = readFileSync(new URL("../src/styles.css", import.meta.url), "utf8");

test("Agent 页面使用独立布局钩子", () => {
  assert.match(source, /className="content-page agents-page"/);
  assert.match(source, /className="sub-hero agents-hero"/);
  assert.match(source, /className="info-grid agents-info-grid"/);
});

test("Agent Hero 高度更紧凑", () => {
  assert.match(styles, /\.agents-hero\s*{[\s\S]*min-height:\s*unset/);
  assert.match(styles, /\.agents-hero\s*{[\s\S]*padding:\s*clamp\(24px,\s*4vw,\s*44px\)/);
});

test("Agent 底部信息卡片自适应且不溢出", () => {
  assert.match(styles, /\.agents-info-grid\s*{[\s\S]*grid-template-columns:\s*repeat\(auto-fit,\s*minmax\(min\(100%,\s*260px\),\s*1fr\)\)/);
  assert.match(styles, /\.agents-info-grid\s*{[\s\S]*min-width:\s*0/);
  assert.match(styles, /\.agents-info-grid \.feature-tile\s*{[\s\S]*grid-template-columns:\s*38px minmax\(0,\s*1fr\)/);
  assert.match(styles, /\.agents-info-grid \.feature-tile span\s*{[\s\S]*overflow-wrap:\s*anywhere/);
  assert.match(styles, /\.content-page \.info-grid:not\(\.agents-info-grid\)/);
});
