import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

const appSource = readFileSync(new URL("../src/App.tsx", import.meta.url), "utf8");
const styles = readFileSync(new URL("../src/styles.css", import.meta.url), "utf8");

test("market cards use jpage-style adaptive grid and preview ratio", () => {
  assert.match(styles, /\.market-card-grid\s*{[\s\S]*minmax\(280px,\s*1fr\)/);
  assert.match(styles, /\.market-page \.preview-pane\s*{[\s\S]*aspect-ratio:\s*4\s*\/\s*3/);
});

test("market card actions are split into top icons, hover primary actions and footer stats", () => {
  assert.match(appSource, /className="market-card-top-actions"/);
  assert.match(appSource, /className="market-card-stats"/);
  assert.match(appSource, /className="market-icon-action"/);
  assert.match(appSource, /className="card-hover-actions"[\s\S]*onDetail\(item\)[\s\S]*onUse\(item\)/);
  assert.doesNotMatch(appSource, /className="card-hover-actions"[\s\S]*toggleFavorite[\s\S]*deleteSite/);
});

test("market previews use live iframe rendering and app URL settings", () => {
  assert.match(appSource, /const MARKET_CARD_IFRAME_SANDBOX\s*=\s*"allow-scripts"/);
  assert.match(appSource, /src=\{previewSrc\}/);
  assert.match(appSource, /setPreviewSrc\(withPreviewParam\(appURL\)\)/);
  assert.match(appSource, /function appURLForDeploy/);
  assert.match(appSource, /item\.filePath/);
  assert.match(appSource, /appURLMode === "domain"/);
});
