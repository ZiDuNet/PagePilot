import { readFile } from "node:fs/promises";
import test from "node:test";
import assert from "node:assert/strict";

const scriptPath = new URL("./docker-upgrade-qa.mjs", import.meta.url);

test("Docker 升级 QA 脚本覆盖真实旧库容器升级链路", async () => {
  const source = await readFile(scriptPath, "utf8");

  for (const expected of [
    "legacy_upgrade_dbcheck.go",
    "--mode\", \"seed",
    "--mode\", \"verify",
    "docker",
    "compose",
    "up",
    "--build",
    "down",
    "/var/lib/hostctl",
    "/var/www/hosted",
    "/api/admin/sites",
    "/api/deploys?q=Legacy",
    "/agent/legacy-demo/",
    "/api/deploys/legacy-secret/access",
    "/api/deploy/content?code=legacy-secret&download=1",
    "/api/screens",
    "/api/admin/audit-logs?siteCode=legacy-demo",
    "/api/tokens",
    "/api/admin/anonymous-sessions",
    "/skill/pagep.zip",
  ]) {
    assert.match(source, new RegExp(expected.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }

  assert.match(source, /docker compose[\s\S]*up[\s\S]*--build/);
  assert.match(source, /docker compose[\s\S]*down/);
  assert.match(source, /keep/i, "脚本应支持保留临时目录便于服务器排查");
});
