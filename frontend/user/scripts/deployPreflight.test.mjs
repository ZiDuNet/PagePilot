import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

const appSource = readFileSync(new URL("../src/App.tsx", import.meta.url), "utf8");
const preflightSource = readFileSync(new URL("../src/deployPreflight.ts", import.meta.url), "utf8");
const styles = readFileSync(new URL("../src/styles.css", import.meta.url), "utf8");

test("部署页在提交前运行本地预检并使用服务端限额", () => {
  assert.match(appSource, /import \{ inspectDeployInput \} from "\.\/deployPreflight"/);
  assert.match(appSource, /const (?:input)?Preflight = useMemo\(\(\) => inspectDeployInput/);
  assert.match(appSource, /disabled=\{busy \|\| uploading \|\| !ready \|\| !preflight\.success\}/);
  assert.match(preflightSource, /maxSingleFileBytes/);
  assert.match(preflightSource, /maxSiteTotalBytes/);
  assert.match(preflightSource, /maxFilesPerSite/);
});

test("预检覆盖入口识别、路径安全、重复文件和 ZIP 提示", () => {
  assert.match(preflightSource, /ZIP_UNSAFE_PATH/);
  assert.match(preflightSource, /ZIP_DUPLICATE_PATH/);
  assert.doesNotMatch(preflightSource, /file\.bytes === 0/);
  assert.match(preflightSource, /ZIP_ENTRY_MISSING/);
  assert.match(preflightSource, /ZIP_SERVER_INSPECTION/);
  assert.match(preflightSource, /PREFERRED_ENTRIES/);
  assert.match(preflightSource, /function markdownSignalScore/);
  assert.match(preflightSource, /function looksLikeFullHTML/);
  assert.match(preflightSource, /const trimmedBytes = utf8Bytes\(value\.trim\(\)\)/);
  assert.match(preflightSource, /utf8Bytes\(main\.content\.trim\(\)\)/);
  assert.ok(preflightSource.indexOf("filename.trim() && !isSafeRelativePath") < preflightSource.indexOf("prepared.length === 1 && ZIP_EXTENSIONS"));
});

test("多文件上传提供独立的文件与目录选择入口", () => {
  assert.match(appSource, /filePickerRef/);
  assert.match(appSource, /directoryPickerRef/);
  assert.match(appSource, /event\.currentTarget\.files \? Array\.from\(event\.currentTarget\.files\)/);
  assert.match(appSource, /选择文件/);
  assert.match(appSource, /选择目录/);
  assert.match(appSource, /function stripCommonUploadRoot/);
  assert.match(appSource, /function stripUploadRootFromFilename/);
  assert.match(appSource, /key=\{f\.id\}/);
  assert.match(appSource, /x\.id !== f\.id/);
  assert.match(appSource, /const clearUploadedFiles/);
  assert.doesNotMatch(appSource, /const empty = selectedFiles\.find/);
  assert.match(appSource, /aria-pressed=\{mode === "single"\}/);
  assert.match(appSource, /uploading/);
  assert.doesNotMatch(appSource, /filePickerRef[^\n]*accept=/);
  assert.match(appSource, /className="entry-field-toggle"/);
  assert.match(styles, /\.deploy-upload-actions/);
  assert.match(styles, /\.deploy-preflight/);
});

test("移动端部署页和页面外壳裁剪横向溢出", () => {
  assert.match(styles, /html,[\s\n]+body\s*\{[\s\S]*overflow-x:\s*clip/);
  assert.match(styles, /@media \(max-width: 480px\)[\s\S]*\.deploy-page-v2/);
});
