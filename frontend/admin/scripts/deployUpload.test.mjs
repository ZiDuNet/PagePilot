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

test("后台多文件上传分别暴露文件与目录选择器", () => {
  assert.match(source, /ref=\{fileInput\} className="deploy-upload-input" type="file" multiple onChange=\{handlePickerChange\}/);
  assert.match(source, /ref=\{dirInput\} className="deploy-upload-input" type="file" multiple webkitdirectory="" onChange=\{handlePickerChange\}/);
  assert.match(source, /onClick=\{\(\) => fileInput\.current\?\.click\(\)\}/);
  assert.match(source, /onClick=\{\(\) => dirInput\.current\?\.click\(\)\}/);
  assert.match(source, /function handlePickerChange\(event: React\.ChangeEvent<HTMLInputElement>\)/);
  assert.match(source, /event\.currentTarget\.value = "";/);
});
