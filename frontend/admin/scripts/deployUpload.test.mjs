import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

const source = readFileSync(new URL("../src/App.tsx", import.meta.url), "utf8");

test("后台发布入口支持 Markdown 和单 ZIP 上传", () => {
  assert.match(source, /function isZipFile\(name: string\)/);
  assert.match(source, /function isDeployEntrypointFile\(name: string\)/);
  assert.match(source, /accept="\.html,\.htm,\.md,\.markdown,\.zip"/);
  assert.match(source, /isZipFile\(file\.name\)/);
  assert.match(source, /const isSingleZipUpload = mode === "multi" && files\.length === 1 && isZipFile\(files\[0\]\.path\);/);
  assert.match(source, /!files\.some\(\(file\) => isDeployEntrypointFile\(file\.path\)\) && !isSingleZipUpload/);
  assert.doesNotMatch(source, /files\.some\(\(file\) => \/\\\.html\?/);
});
