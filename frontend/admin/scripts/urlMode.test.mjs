import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

const source = readFileSync(new URL("../src/App.tsx", import.meta.url), "utf8");

test("后台应用链接优先使用服务端返回的 URL 契约", () => {
  assert.match(source, /interface SiteItem[\s\S]*?url\?: string;/);
  assert.match(source, /interface SiteItem[\s\S]*?pathUrl\?: string;/);
  assert.match(source, /interface SiteItem[\s\S]*?domainUrl\?: string;/);
  assert.match(source, /interface AdminVersionItem[\s\S]*?url\?: string;/);
  assert.match(source, /interface AdminVersionItem[\s\S]*?pathUrl\?: string;/);
  assert.match(source, /interface AdminVersionItem[\s\S]*?domainUrl\?: string;/);

  const resolver = source.slice(source.indexOf("function appURLFromResponse"), source.indexOf("function withPreviewParam"));
  assert.match(resolver, /if \(canonicalURL\) return canonicalURL;/);
  assert.match(resolver, /appURLMode\?\.toLowerCase\(\) === "domain" && urls\.domainUrl/);
  assert.match(resolver, /if \(urls\.pathUrl\) return sameSiteURL\(urls\.pathUrl\);/);
  assert.match(source, /appURLForSite\(config, site\)/);
  assert.match(source, /appURLForVersion\(config, versionCode, version\)/);
});

test("后台部署成功弹窗不会猜测路径路由", () => {
  const deployPanel = source.slice(source.indexOf("function DeployPanel"), source.indexOf("function SitesPanel"));
  assert.match(deployPanel, /const resultAppURL = result/);
  assert.match(deployPanel, /appURLFromResponse\(config, result\)/);
  assert.match(deployPanel, /configuredAppURL\(config, result\.code\)/);
});
