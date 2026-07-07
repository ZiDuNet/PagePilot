import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

const source = readFileSync(new URL("../src/App.tsx", import.meta.url), "utf8");
const deployPage = source.slice(
  source.indexOf("function DeployPage"),
  source.indexOf("function DeployResult")
);

test("手动部署使用后端当前字段 customCode，而不是旧字段 code", () => {
  assert.match(deployPage, /payload\.customCode = customCode\.trim\(\)/);
  assert.match(deployPage, /payload\.enableCustomCode = true/);
  assert.doesNotMatch(deployPage, /payload\.code = customCode\.trim\(\)/);
});

test("更新已有发布时继承已选站点标题和描述", () => {
  assert.match(deployPage, /selectedUpdatableSite/);
  assert.match(deployPage, /description\.trim\(\) \|\| selectedUpdatableSite\?\.description/);
  assert.match(deployPage, /title\.trim\(\) \|\| selectedUpdatableSite\?\.title/);
});

test("单 ZIP 上传使用稳定容器名，避免中文 ZIP 文件名被当作站内路径", () => {
  assert.match(source, /function normalizeUploadedZipPath/);
  assert.match(deployPage, /path: normalizeUploadedZipPath\(file\.name\)/);
  assert.doesNotMatch(deployPage, /path: file\.name \|\| "site\.zip"/);
});

test("入口文件名默认隐藏且不再预填 index.html", () => {
  assert.match(deployPage, /const \[filename, setFilename\] = useState\(""\)/);
  assert.match(deployPage, /className="entry-field-toggle"/);
  assert.doesNotMatch(deployPage, /const \[filename, setFilename\] = useState\("index\.html"\)/);
});

test("上传文件名会清洗后作为入口提示，粘贴源码不强制传 filename", () => {
  assert.match(source, /function normalizeUploadedFilePath/);
  assert.match(deployPage, /setFilename\(normalizeUploadedFilePath\(file\.name, "upload"\)\)/);
  assert.match(deployPage, /normalizeUploadedFilePath\(rawPath, "asset"\)/);
  assert.doesNotMatch(deployPage, /setFilename\(file\.name \|\| "index\.html"\)/);
});
