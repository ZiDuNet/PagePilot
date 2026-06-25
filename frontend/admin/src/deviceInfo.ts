export interface DeviceInfoInput {
  deviceName?: string;
  appVersion?: string;
  runtime?: string;
  deviceInfo?: unknown;
}

export interface DeviceInfoRow {
  key: string;
  label: string;
  value: string;
  priority: boolean;
}

type DeviceInfoRecord = Record<string, unknown>;

const knownKeys = new Set([
  "deviceName",
  "manufacturer",
  "brand",
  "model",
  "device",
  "host",
  "hostname",
  "machineName",
  "os",
  "osType",
  "platform",
  "system",
  "distro",
  "osVersion",
  "androidVersion",
  "androidRelease",
  "androidSdk",
  "sdk",
  "screenWidthPx",
  "screenHeightPx",
  "width",
  "height",
  "resolution",
  "orientation",
  "densityDpi",
  "density",
  "scaleFactor",
  "locale",
  "language",
  "timeZone",
  "timezone",
  "runtime",
  "appVersion",
  "webView",
  "webViewRuntime",
  "webViewClass",
  "webviewVersion",
  "webViewVersion",
  "webView2Version",
  "x5Version",
  "x5Loaded",
  "x5Init",
  "x5Diagnostic",
  "cpuAbi",
  "arch",
  "architecture",
  "desktop",
  "networkType",
  "ip",
  "mac",
  "socketStatus",
  "wsStatus"
]);

const labelMap: Record<string, string> = {
  manufacturer: "厂商",
  brand: "品牌",
  model: "设备型号",
  device: "设备代号",
  host: "主机名",
  hostname: "主机名",
  machineName: "主机名",
  osVersion: "系统版本",
  distro: "发行版",
  locale: "语言",
  language: "语言",
  timeZone: "时区",
  timezone: "时区",
  webViewClass: "WebView 类",
  webviewVersion: "WebView 版本",
  webViewVersion: "WebView 版本",
  webView2Version: "WebView2 版本",
  x5Init: "X5 初始化",
  x5Diagnostic: "X5 诊断",
  cpuAbi: "CPU ABI",
  arch: "CPU 架构",
  architecture: "CPU 架构",
  desktop: "桌面环境",
  networkType: "网络类型",
  ip: "IP",
  mac: "MAC",
  socketStatus: "WebSocket",
  wsStatus: "WebSocket"
};

export function buildDeviceInfoRows(input: DeviceInfoInput): DeviceInfoRow[] {
  const data = normalizeDeviceInfo(input.deviceInfo);
  const rows: DeviceInfoRow[] = [];
  const used = new Set<string>();

  addRow(rows, used, "deviceName", "设备名称", firstText(input.deviceName, data.deviceName, data.hostname, data.host, data.machineName), true);
  addRow(rows, used, "osType", "系统类型", detectOSType(data), true);
  addRow(rows, used, "osVersion", "系统版本", formatOSVersion(data), true);
  addRow(rows, used, "model", "设备型号", formatModel(data), true);
  addRow(rows, used, "resolution", "分辨率", formatResolution(data), true);
  addRow(rows, used, "orientation", "屏幕方向", formatOrientation(data.orientation), true);
  addRow(rows, used, "density", "像素密度", formatDensity(data), true);
  addRow(rows, used, "locale", "语言", firstText(data.locale, data.language), false);
  addRow(rows, used, "timeZone", "时区", firstText(data.timeZone, data.timezone), false);
  addRow(rows, used, "appVersion", "APP 版本", firstText(input.appVersion, data.appVersion), true);
  addRow(rows, used, "runtime", "运行时", firstText(input.runtime, data.runtime), true);
  addRow(rows, used, "webViewRuntime", "WebView", firstText(data.webViewRuntime, data.webView, data.webViewVersion, data.webviewVersion, data.webView2Version), true);
  addRow(rows, used, "webViewClass", "WebView 类", textValue(data.webViewClass), false);
  addRow(rows, used, "x5Status", "X5 状态", formatX5(data), true);
  addRow(rows, used, "x5Init", "X5 初始化", textValue(data.x5Init), false);
  addRow(rows, used, "x5Diagnostic", "X5 诊断", textValue(data.x5Diagnostic), false);
  addRow(rows, used, "cpuAbi", "CPU ABI", firstText(data.cpuAbi, data.arch, data.architecture), true);
  addRow(rows, used, "networkType", "网络类型", textValue(data.networkType), false);
  addRow(rows, used, "ip", "IP", textValue(data.ip), false);
  addRow(rows, used, "mac", "MAC", textValue(data.mac), false);
  addRow(rows, used, "socketStatus", "WebSocket", firstText(data.socketStatus, data.wsStatus), true);

  for (const [key, value] of Object.entries(data)) {
    if (used.has(key) || knownKeys.has(key)) continue;
    const text = textValue(value);
    if (text) rows.push({ key, label: labelMap[key] || key, value: text, priority: false });
  }

  return rows;
}

export function formatDeviceInfoSummary(input: DeviceInfoInput): string {
  const rows = buildDeviceInfoRows(input);
  const byLabel = new Map(rows.map((row) => [row.label, row.value]));
  const parts = [
    byLabel.get("系统版本") || byLabel.get("系统类型"),
    byLabel.get("设备型号") || byLabel.get("设备名称"),
    byLabel.get("分辨率"),
    byLabel.get("屏幕方向")
  ].filter(Boolean);
  return parts.length ? parts.join(" · ") : "-";
}

export function normalizeDeviceInfo(info: unknown): DeviceInfoRecord {
  if (!info) return {};
  if (typeof info === "string") {
    const trimmed = info.trim();
    if (!trimmed) return {};
    try {
      const parsed = JSON.parse(trimmed) as unknown;
      return normalizeDeviceInfo(parsed);
    } catch {
      return { raw: trimmed };
    }
  }
  if (typeof info !== "object" || Array.isArray(info)) return {};
  return info as DeviceInfoRecord;
}

function addRow(rows: DeviceInfoRow[], used: Set<string>, key: string, label: string, value: string, priority: boolean) {
  if (!value) return;
  rows.push({ key, label, value, priority });
  used.add(key);
}

function detectOSType(data: DeviceInfoRecord): string {
  const raw = firstText(data.osType, data.os, data.platform, data.system);
  const source = raw || (data.androidRelease || data.androidSdk || data.androidVersion || data.sdk ? "Android" : "");
  const lower = source.toLowerCase();
  if (lower.includes("android")) return "Android";
  if (lower.includes("windows") || lower === "win32") return "Windows";
  if (lower.includes("linux")) return "Linux";
  if (lower.includes("darwin") || lower.includes("mac")) return "macOS";
  return source;
}

function formatOSVersion(data: DeviceInfoRecord): string {
  const osType = detectOSType(data);
  if (osType === "Android") {
    const release = firstText(data.androidRelease, data.androidVersion);
    const sdk = firstText(data.androidSdk, data.sdk);
    if (release && sdk) return `Android ${release} / SDK ${sdk}`;
    if (release) return `Android ${release}`;
    if (sdk) return `Android SDK ${sdk}`;
  }
  const version = firstText(data.osVersion, data.systemVersion);
  if (!version) return osType;
  if (osType && !version.toLowerCase().includes(osType.toLowerCase())) {
    return `${osType} ${version}`;
  }
  return version;
}

function formatModel(data: DeviceInfoRecord): string {
  const maker = firstText(data.manufacturer, data.brand);
  const model = textValue(data.model);
  if (maker && model && !model.toLowerCase().includes(maker.toLowerCase())) return `${maker} ${model}`;
  return model || maker;
}

function formatResolution(data: DeviceInfoRecord): string {
  const resolution = textValue(data.resolution);
  if (resolution) return resolution.replace(/[×*]/g, " x ");
  const width = firstText(data.screenWidthPx, data.width);
  const height = firstText(data.screenHeightPx, data.height);
  return width && height ? `${width} x ${height}` : "";
}

function formatOrientation(value: unknown): string {
  const text = textValue(value);
  if (!text) return "";
  const lower = text.toLowerCase();
  if (lower.includes("landscape") || lower === "横屏") return "横屏";
  if (lower.includes("portrait") || lower === "竖屏") return "竖屏";
  return text;
}

function formatDensity(data: DeviceInfoRecord): string {
  const dpi = firstText(data.densityDpi);
  const density = firstText(data.density, data.scaleFactor);
  if (dpi && density) return `${dpi} dpi / ${density}`;
  if (dpi) return `${dpi} dpi`;
  return density;
}

function formatX5(data: DeviceInfoRecord): string {
  const loaded = data.x5Loaded;
  const version = textValue(data.x5Version);
  const status = typeof loaded === "boolean" ? (loaded ? "已加载" : "未加载") : "";
  if (status && version) return `${status} / ${version}`;
  return status || version;
}

function firstText(...values: unknown[]): string {
  for (const value of values) {
    const text = textValue(value);
    if (text) return text;
  }
  return "";
}

function textValue(value: unknown): string {
  if (value == null) return "";
  if (typeof value === "string") return value.trim();
  if (typeof value === "number" || typeof value === "boolean") return String(value);
  try {
    return JSON.stringify(value);
  } catch {
    return "";
  }
}
