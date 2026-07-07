import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

const source = readFileSync(new URL("../src/App.tsx", import.meta.url), "utf8");
const deployPanel = source.slice(
  source.indexOf("function DeployPanel"),
  source.indexOf("function SiteManager")
);

test("后台发布表单的 ZIP 上传使用稳定容器名", () => {
  assert.match(source, /function normalizeUploadedZipPath/);
  assert.match(deployPanel, /path: normalizeUploadedZipPath\(file\.name\)/);
  assert.doesNotMatch(deployPanel, /path: file\.name \|\| "site\.zip"/);
});

test("后台作品标签和访问密码禁用浏览器自动填充", () => {
  assert.match(deployPanel, /autoComplete="off"/);
  assert.match(deployPanel, /autoComplete="new-password"/);
  assert.match(deployPanel, /data-lpignore="true"/);
});

test("后台更新已有发布时可自动补版本描述", () => {
  assert.match(deployPanel, /effectiveDescription/);
  assert.match(deployPanel, /`更新 \$\{code\.trim\(\)\} 的新版本`/);
  assert.match(deployPanel, /description: effectiveDescription/);
});

test("后台入口文件名默认隐藏且不再预填 index.html", () => {
  assert.match(deployPanel, /const \[entry, setEntry\] = useState\(""\)/);
  assert.match(deployPanel, /className="entry-field-toggle"/);
  assert.doesNotMatch(deployPanel, /const \[entry, setEntry\] = useState\("index\.html"\)/);
});

test("后台上传文件路径会清洗，非 ZIP 多文件不强行传 index.html", () => {
  assert.match(source, /function normalizeUploadedFilePath/);
  assert.match(deployPanel, /setEntry\(normalizeUploadedFilePath\(file\.name, "upload"\)\)/);
  assert.match(deployPanel, /normalizeUploadedFilePath\([^)]*rawPath[^)]*, "asset"\)/);
  assert.doesNotMatch(deployPanel, /body\.filename = mainEntry \|\| "index\.html"/);
});
