import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

const source = readFileSync(new URL("../src/App.tsx", import.meta.url), "utf8");
const loginScreen = source.slice(
  source.indexOf("function LoginScreen"),
  source.indexOf("function Overview")
);

test("登录页将后端认证错误转换为中文友好提示", () => {
  assert.match(source, /function friendlyAuthErrorMessage/);
  assert.match(source, /username or password is incorrect/);
  assert.match(source, /用户名或密码不正确，请检查后重新输入。/);
  assert.match(source, /captcha is incorrect or expired/);
  assert.match(source, /验证码不正确或已过期，请刷新后重试。/);
  assert.match(loginScreen, /setError\(friendlyAuthErrorMessage\(err\)\)/);
  assert.doesNotMatch(loginScreen, /setError\(err instanceof Error \? err\.message : String\(err\)\)/);
});
