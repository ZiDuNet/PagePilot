import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

const source = readFileSync(new URL("../src/App.tsx", import.meta.url), "utf8");
const auth = source.slice(source.indexOf("function authHeaders"), source.indexOf("function userMessage"));
const entrypoint = readFileSync(new URL("../src/main.tsx", import.meta.url), "utf8");

test("管理端认证不从 Web Storage 注入令牌", () => {
  assert.doesNotMatch(auth, /localStorage|sessionStorage|hostctl-(?:admin-)?token/);
});

test("管理端启动时清理旧版 Web Storage 令牌", () => {
  assert.match(entrypoint, /localStorage\.removeItem\("hostctl-token"\)/);
  assert.match(entrypoint, /localStorage\.removeItem\("hostctl-admin-token"\)/);
  assert.match(entrypoint, /sessionStorage\.removeItem\("hostctl-token"\)/);
  assert.match(entrypoint, /sessionStorage\.removeItem\("hostctl-admin-token"\)/);
  assert.doesNotMatch(entrypoint, /(?:localStorage|sessionStorage)\.getItem\(/);
});

test("管理端收到 401 时不会继续使用失效会话", () => {
  assert.match(source, /res\.status === 401/);
  assert.match(source, /hostctl:session-expired/);
  assert.match(source, /setSession\(null\)/);
});

test("管理端登出失败时保留页面并显示可重试提示", () => {
  assert.match(source, /async function logout/);
  assert.match(source, /if \(!res\.ok\)/);
  assert.match(source, /退出登录失败，请检查网络后重试/);
});
