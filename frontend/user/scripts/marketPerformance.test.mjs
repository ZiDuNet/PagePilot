import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

const source = readFileSync(new URL("../src/App.tsx", import.meta.url), "utf8");

test("market list previews are queued and cached", () => {
  assert.match(source, /const MARKET_PREVIEW_CONCURRENCY\s*=\s*3/);
  assert.match(source, /function acquireMarketPreviewSlot/);
  assert.match(source, /function releaseMarketPreviewSlot/);
  assert.match(source, /function runMarketPreviewQueue/);
  assert.match(source, /setPreviewSrc\(withPreviewParam\(appURL\)\)/);
  assert.match(source, /previewIndex=\{index\}/);
});

test("market detail delays the live iframe mount", () => {
  assert.match(source, /detailPreviewReady/);
  assert.match(source, /setDetailPreviewReady\(false\)/);
  assert.match(source, /window\.setTimeout\(\(\) => setDetailPreviewReady\(true\), 220\)/);
  assert.match(source, /detailPreviewReady\s*\?/);
});

test("direct market detail route does not eagerly load the market list", () => {
  assert.match(source, /loadedListQueryRef\s*=\s*useRef\(""\)/);
  assert.match(source, /if \(detailKey\) \{/);
  assert.match(source, /loadedListQueryRef\.current === currentListQueryKey/);
});
