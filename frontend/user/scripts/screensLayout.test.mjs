import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

const source = readFileSync(new URL("../src/App.tsx", import.meta.url), "utf8");
const styles = readFileSync(new URL("../src/styles.css", import.meta.url), "utf8");

test("screens page uses a productized management layout", () => {
  assert.match(source, /screen-page-v3/);
  assert.match(source, /screen-hero-v2/);
  assert.match(source, /screen-hero-steps/);
  assert.match(source, /screen-metric-row/);
  assert.match(source, /screen-dashboard-grid/);
  assert.doesNotMatch(source, /screen-device-demo/);
});

test("screens page has responsive layout constraints", () => {
  for (const className of [
    ".screen-page-v3",
    ".screen-hero-v2",
    ".screen-hero-steps",
    ".screen-metric-row",
    ".screen-dashboard-grid",
    ".screen-table-card"
  ]) {
    assert.match(styles, new RegExp(className.replace(".", "\\.")));
  }
  assert.match(styles, /@media \(max-width: 900px\)[\s\S]*screen-dashboard-grid/);
});
