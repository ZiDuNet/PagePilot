import assert from "node:assert/strict";
import { after, test } from "node:test";
import { readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { pathToFileURL } from "node:url";
import ts from "typescript";

const source = readFileSync(new URL("../src/deviceInfo.ts", import.meta.url), "utf8");
const output = ts.transpileModule(source, {
  compilerOptions: {
    module: ts.ModuleKind.ES2020,
    target: ts.ScriptTarget.ES2020,
    isolatedModules: true
  }
}).outputText;
const compiledPath = join(tmpdir(), `pagepilot-admin-device-info-${Date.now()}.mjs`);
writeFileSync(compiledPath, output);
const { buildDeviceInfoRows, formatDeviceInfoSummary } = await import(pathToFileURL(compiledPath).href);

after(() => {
  rmSync(compiledPath, { force: true });
});

test("后台设备信息会展开 Android 上报的完整字段", () => {
  const rows = buildDeviceInfoRows({
    deviceName: "HUAWEI ABR-AL80",
    appVersion: "0.1.0",
    runtime: "X5 46999",
    deviceInfo: {
      manufacturer: "HUAWEI",
      brand: "HONOR",
      model: "ABR-AL80",
      androidRelease: "12",
      androidSdk: 31,
      screenWidthPx: 1920,
      screenHeightPx: 1080,
      orientation: "landscape",
      densityDpi: 320,
      density: 2,
      locale: "zh-CN",
      timeZone: "Asia/Shanghai",
      webViewRuntime: "X5 46999",
      webViewClass: "com.tencent.smtt.sdk.WebView",
      x5Version: 46999,
      x5Loaded: true,
      cpuAbi: "arm64-v8a",
      socketStatus: "connected",
      customVendorField: "keep-me"
    }
  });
  const byLabel = Object.fromEntries(rows.map((row) => [row.label, row.value]));

  assert.equal(byLabel["设备名称"], "HUAWEI ABR-AL80");
  assert.equal(byLabel["系统类型"], "Android");
  assert.equal(byLabel["系统版本"], "Android 12 / SDK 31");
  assert.equal(byLabel["分辨率"], "1920 x 1080");
  assert.equal(byLabel["屏幕方向"], "横屏");
  assert.equal(byLabel["像素密度"], "320 dpi / 2");
  assert.equal(byLabel["WebView"], "X5 46999");
  assert.equal(byLabel["X5 状态"], "已加载 / 46999");
  assert.equal(byLabel["WebSocket"], "connected");
  assert.equal(byLabel["customVendorField"], "keep-me");
});

test("后台设备信息支持 Windows 和 Linux 字段", () => {
  const windowsRows = buildDeviceInfoRows({
    runtime: "WebView2 126",
    deviceInfo: {
      osType: "Windows",
      osVersion: "Windows 11 Pro 23H2",
      hostname: "ad-screen-01",
      screenWidthPx: 3840,
      screenHeightPx: 2160,
      orientation: "landscape",
      arch: "x64"
    }
  });
  const linuxRows = buildDeviceInfoRows({
    deviceInfo: {
      platform: "linux",
      distro: "Ubuntu",
      osVersion: "24.04",
      width: 1080,
      height: 1920,
      orientation: "portrait",
      desktop: "Wayland"
    }
  });

  assert.equal(Object.fromEntries(windowsRows.map((row) => [row.label, row.value]))["系统类型"], "Windows");
  assert.equal(Object.fromEntries(windowsRows.map((row) => [row.label, row.value]))["分辨率"], "3840 x 2160");
  assert.equal(Object.fromEntries(linuxRows.map((row) => [row.label, row.value]))["系统类型"], "Linux");
  assert.equal(Object.fromEntries(linuxRows.map((row) => [row.label, row.value]))["分辨率"], "1080 x 1920");
});

test("摘要优先显示系统、型号、分辨率和方向", () => {
  assert.equal(formatDeviceInfoSummary({
    deviceInfo: {
      model: "ABR-AL80",
      androidRelease: "12",
      screenWidthPx: 1080,
      screenHeightPx: 1920,
      orientation: "portrait"
    }
  }), "Android 12 · ABR-AL80 · 1080 x 1920 · 竖屏");
});
