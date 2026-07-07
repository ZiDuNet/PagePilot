import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

const userSource = readFileSync(new URL("../src/App.tsx", import.meta.url), "utf8");
const adminSource = readFileSync(new URL("../../admin/src/App.tsx", import.meta.url), "utf8");

for (const [name, source] of [
  ["user", userSource],
  ["admin", adminSource]
]) {
  test(`${name} deploy errors translate missing description`, () => {
    assert.match(source, /INVALID_DESCRIPTION/);
    assert.match(source, /请填写一句话描述/);
    assert.match(source, /friendlyDeployErrorMessage/);
    assert.match(source, /deployErrorHints[\s\S]*INVALID_DESCRIPTION/);
  });
}
