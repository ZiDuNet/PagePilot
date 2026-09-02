import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

const source = readFileSync(new URL("../src/App.tsx", import.meta.url), "utf8");
const types = readFileSync(new URL("../src/types.ts", import.meta.url), "utf8");

test("市场当前链接优先使用服务端 URL 契约", () => {
  assert.match(types, /export interface MarketplaceDeploy[\s\S]*?url\?: string;/);
  assert.match(types, /export interface MarketplaceDeploy[\s\S]*?pathUrl\?: string;/);
  assert.match(types, /export interface MarketplaceDeploy[\s\S]*?domainUrl\?: string;/);

  const resolver = source.slice(source.indexOf("function appURLFromResponse"), source.indexOf("function appURLForDeploy"));
  assert.match(resolver, /if \(urls\.url\) return sameSiteURL\(urls\.url\);/);
  assert.match(resolver, /mode === "domain" && urls\.domainUrl/);
  assert.match(resolver, /if \(urls\.pathUrl\) return sameSiteURL\(urls\.pathUrl\);/);

  const currentURL = source.slice(source.indexOf("function appURLForDeploy"), source.indexOf("function appURLForVersion"));
  assert.match(currentURL, /const responseURL = appURLFromResponse\(config, item\);/);
  assert.match(currentURL, /if \(responseURL\) return responseURL;/);
  assert.match(currentURL, /if \(!version && item\.filePath\) return sameSiteURL\(item\.filePath\);/);
  assert.match(currentURL, /return buildAppURL\(config, item\.code, version\);/);
});

test("市场历史版本链接优先使用服务端 URL 契约", () => {
  const versionItem = source.slice(source.indexOf("interface VersionItem"), source.indexOf("interface VersionsResponse"));
  assert.match(versionItem, /url\?: string;/);
  assert.match(versionItem, /pathUrl\?: string;/);
  assert.match(versionItem, /domainUrl\?: string;/);
  assert.match(source, /function appURLForVersion\(config: RuntimeConfig \| null, code: string, version: VersionItem\)/);
  assert.match(source, /return appURLFromResponse\(config, version\) \|\| buildAppURL\(config, code, version\.versionNumber\);/);
  assert.match(source, /const versionURL = appURLForVersion\(config, item\.code, version\);/);
});
