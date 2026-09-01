import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

const source = readFileSync(new URL("../src/api.ts", import.meta.url), "utf8");
const auth = source.slice(source.indexOf("export function authHeaders"), source.indexOf("export async function api"));
const entrypoint = readFileSync(new URL("../src/main.tsx", import.meta.url), "utf8");

test("用户端认证不从 Web Storage 注入令牌", () => {
  assert.doesNotMatch(auth, /localStorage|sessionStorage|hostctl-(?:admin-)?token/);
});

test("用户端启动时清理旧版 Web Storage 令牌", () => {
  assert.match(entrypoint, /localStorage\.removeItem\("hostctl-token"\)/);
  assert.match(entrypoint, /localStorage\.removeItem\("hostctl-admin-token"\)/);
  assert.match(entrypoint, /sessionStorage\.removeItem\("hostctl-token"\)/);
  assert.match(entrypoint, /sessionStorage\.removeItem\("hostctl-admin-token"\)/);
  assert.doesNotMatch(entrypoint, /(?:localStorage|sessionStorage)\.getItem\(/);
});
