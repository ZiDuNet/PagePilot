import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

const source = readFileSync(new URL("../src/App.tsx", import.meta.url), "utf8");
const expectedSandbox =
  "allow-scripts allow-forms allow-popups allow-popups-to-escape-sandbox allow-downloads allow-modals allow-top-navigation-by-user-activation";

test("前台预览 iframe 使用集中 sandbox 策略", () => {
  const match = source.match(/const PREVIEW_IFRAME_SANDBOX\s*=\s*"([^"]+)";/);
  assert.ok(match, "PREVIEW_IFRAME_SANDBOX 常量缺失");
  assert.equal(match[1], expectedSandbox);
  assert.equal((source.match(/sandbox=\{PREVIEW_IFRAME_SANDBOX\}/g) || []).length, 3);
  assert.equal((source.match(/sandbox="allow-scripts/g) || []).length, 0);
  assert.equal(match[1].includes("allow-same-origin"), false);
});
