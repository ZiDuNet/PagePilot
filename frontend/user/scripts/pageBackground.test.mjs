import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

const userStyles = readFileSync(new URL("../src/styles.css", import.meta.url), "utf8");
const adminStyles = readFileSync(new URL("../../admin/src/styles.css", import.meta.url), "utf8");

test("全局根背景不再使用旧的浅蓝色块", () => {
  assert.doesNotMatch(userStyles, /#eaf8ff/i, "用户端不应再暴露 #eaf8ff 兜底背景");
  assert.doesNotMatch(adminStyles, /#eaf8ff/i, "后台端不应再暴露 #eaf8ff 兜底背景");
});

test("用户端根节点使用白色兜底背景", () => {
  assert.match(userStyles, /--bg:\s*#ffffff/i);
  assert.match(userStyles, /html\s*{[^}]*background:\s*#ffffff/i);
});

test("创作市场在白色卡片下使用柔和中性底色", () => {
  assert.match(userStyles, /\.page-market-main\s*{[\s\S]*background:\s*#f6f7f9/i);
  assert.match(userStyles, /\.page-market-main \.market-page\s*{[\s\S]*background:\s*#f6f7f9/i);
});

test("后台端根节点使用白色兜底背景", () => {
  assert.match(adminStyles, /--bg:\s*#ffffff/i);
  assert.match(adminStyles, /html,\s*body,\s*#root\s*{[^}]*background:\s*#ffffff/i);
});
