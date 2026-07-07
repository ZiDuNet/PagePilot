import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

const source = readFileSync(new URL("../src/App.tsx", import.meta.url), "utf8");

test("后台审计日志提供产品化筛选控件", () => {
  assert.match(source, /const auditActionOptions = \[/, "缺少动作预设列表");
  assert.match(source, /const auditActorTypeOptions = \[/, "缺少操作者类型预设");
  assert.match(source, /actorType: ""/, "filters 缺少 actorType");
  assert.match(source, /setAuditUsers/, "审计日志未加载用户列表");
  assert.match(source, /setAuditSites/, "审计日志未加载站点列表");
  assert.match(source, /\/api\/admin\/users/, "审计日志未请求用户列表");
  assert.match(source, /\/api\/admin\/sites/, "审计日志未请求站点列表");
  assert.match(source, /auditUsers\.map/, "缺少用户下拉选项渲染");
  assert.match(source, /auditSites\.map/, "缺少站点下拉选项渲染");
  assert.match(source, /auditActionOptions\.map/, "缺少动作预设渲染");
  assert.match(source, /auditActorTypeOptions\.map/, "缺少操作者类型渲染");
  assert.match(source, /type="datetime-local"/, "缺少时间范围过滤");
});

test("后台审计日志能筛选并读懂访问密码验证动作", () => {
  assert.match(source, /site\.access_login/, "缺少访问密码验证动作预设");
  assert.match(source, /访问密码验证/, "缺少访问密码验证中文标签");
  assert.match(
    source,
    /auditActionLabel\(log\.action\)/,
    "审计日志表格未使用中文动作标签"
  );
});

test("后台审计日志动作预设覆盖后端真实落库动作", () => {
  const requiredActions = [
    "account.password",
    "auth.login",
    "auth.logout",
    "auth.register",
    "deploy.create",
    "deploy.version.create",
    "screen.bind",
    "screen.unbind",
    "site.category",
    "site.primary_strategy",
    "skill.package_upload",
    "token.revoke",
    "user.update",
    "version.current",
    "version.delete",
    "version.overwrite",
    "version.status"
  ];

  for (const action of requiredActions) {
    assert.match(source, new RegExp(`value: "${action.replace(".", "\\.")}"`), `缺少动作预设 ${action}`);
  }

  assert.doesNotMatch(source, /value: "token\.delete"/, "不应继续用 token.delete 作为筛选动作");
  assert.doesNotMatch(source, /value: "skill\.upload"/, "不应继续用 skill.upload 作为筛选动作");
});
