export type DeployPreflightMode = "single" | "multi";

export interface DeployPreflightLimits {
  maxSingleFileBytes?: number;
  maxSiteTotalBytes?: number;
  maxFilesPerSite?: number;
}

export interface DeployPreflightFile {
  path: string;
  content: string;
  contentBase64?: string;
  isText?: boolean;
  size?: number;
}

export interface DeployPreflightIssue {
  code: string;
  detail: string;
  hint?: string;
}

export interface DeployPreflightReport {
  success: boolean;
  sourceType: "single" | "multi" | "zip";
  kind?: "single_html" | "markdown" | "static_site" | "zip_site";
  mainEntry?: string;
  count: number;
  bytes: number;
  limits: Required<DeployPreflightLimits>;
  errors: DeployPreflightIssue[];
  warnings: DeployPreflightIssue[];
}

const DEFAULT_LIMITS: Required<DeployPreflightLimits> = {
  maxSingleFileBytes: 1 << 20,
  maxSiteTotalBytes: 10 << 20,
  maxFilesPerSite: 100
};

const PAGE_EXTENSIONS = /\.(html?|md|markdown)$/i;
const HTML_EXTENSIONS = /\.html?$/i;
const MARKDOWN_EXTENSIONS = /\.(md|markdown)$/i;
const ZIP_EXTENSIONS = /\.zip$/i;
const PREFERRED_ENTRIES = ["index.html", "index.htm", "README.md", "readme.md", "README.markdown", "readme.markdown"];

function normalizeLimits(limits?: DeployPreflightLimits): Required<DeployPreflightLimits> {
  const source = limits || {};
  return {
    maxSingleFileBytes: Number.isFinite(source.maxSingleFileBytes) && Number(source.maxSingleFileBytes) > 0
      ? Math.floor(Number(source.maxSingleFileBytes))
      : DEFAULT_LIMITS.maxSingleFileBytes,
    maxSiteTotalBytes: Number.isFinite(source.maxSiteTotalBytes) && Number(source.maxSiteTotalBytes) > 0
      ? Math.floor(Number(source.maxSiteTotalBytes))
      : DEFAULT_LIMITS.maxSiteTotalBytes,
    maxFilesPerSite: Number.isFinite(source.maxFilesPerSite) && Number(source.maxFilesPerSite) > 0
      ? Math.floor(Number(source.maxFilesPerSite))
      : DEFAULT_LIMITS.maxFilesPerSite
  };
}

function utf8Bytes(value: string): number {
  return new TextEncoder().encode(value).byteLength;
}

function base64Bytes(value: string): number {
  const normalized = value.replace(/\s+/g, "");
  if (!normalized) return 0;
  const padding = normalized.endsWith("==") ? 2 : normalized.endsWith("=") ? 1 : 0;
  return Math.max(0, Math.floor((normalized.length * 3) / 4) - padding);
}

function fileBytes(file: DeployPreflightFile): number {
  if (Number.isFinite(file.size) && Number(file.size) >= 0) return Math.floor(Number(file.size));
  if (file.contentBase64) return base64Bytes(file.contentBase64);
  return utf8Bytes(file.content || "");
}

function isSafeRelativePath(path: string): boolean {
  const normalized = path;
  if (!normalized || normalized.includes("\\") || normalized.includes("\0") || normalized.startsWith("/") || normalized.startsWith("//")) return false;
  if (/^[A-Za-z]:/.test(normalized)) return false;
  if (utf8Bytes(normalized) > 255) return false;
  const segments = normalized.split("/");
  if (segments.length > 16 || /[\p{Cc}\p{Cf}<>:"|?*]/u.test(normalized)) return false;
  return segments.every((segment) => {
    if (!segment || segment === "." || segment === "..") return false;
    if (segment.endsWith(".") || segment.endsWith(" ")) return false;
    const stem = segment.replace(/\.[^.]*$/, "");
    return !/^(con|prn|aux|nul|com[1-9]|lpt[1-9])$/i.test(stem);
  });
}

function isPagePath(path: string): boolean {
  return PAGE_EXTENSIONS.test(path);
}

function inferSingleKind(content: string): "html" | "markdown" {
  const trimmed = content.trim().replace(/^\ufeff/, "");
  if (looksLikeFullHTML(trimmed)) {
    return "html";
  }
  if (markdownSignalScore(trimmed) >= 2) return "markdown";
  if (looksLikeHTML(trimmed)) return "html";
  return "markdown";
}

function inferSingleEntry(filename: string, content: string): string {
  const hint = filename.trim();
  const kind = inferSingleKind(content);
  if (!hint) return kind === "markdown" ? "README.md" : "index.html";
  const stem = hint.replace(/\.(html?|md|markdown)$/i, "");
  return `${stem}.${kind === "markdown" ? "md" : "html"}`;
}

function looksLikeHTML(content: string): boolean {
  const lower = content.trim().toLowerCase();
  if (!lower.includes("<") || !lower.includes(">")) return false;
  return ["main", "section", "article", "nav", "header", "footer", "div", "p", "h1", "h2", "h3", "ul", "ol", "table", "form", "button", "canvas", "svg", "script", "style"]
    .some((tag) => lower.includes(`<${tag}`));
}

function looksLikeFullHTML(content: string): boolean {
  const lower = content.trim().toLowerCase();
  return lower.startsWith("<!doctype html") ||
    lower.startsWith("<html") ||
    lower.includes("<html") ||
    lower.includes("<body") ||
    lower.includes("<head");
}

// Keep this heuristic aligned with the server so Markdown containing raw HTML
// or fenced examples does not unexpectedly become an HTML entry.
function markdownSignalScore(content: string): number {
  let score = 0;
  const lines = content.replace(/\r\n/g, "\n").split("\n");
  for (const line of lines) {
    const trimmed = line.trim();
    if (!trimmed) continue;
    if (trimmed.startsWith("# ") || trimmed.startsWith("## ") || trimmed.startsWith("### ") || trimmed.startsWith("```")) {
      score += 2;
    } else if (trimmed.startsWith("- [ ] ") || trimmed.startsWith("- [x] ")) {
      score += 2;
    } else if (trimmed.startsWith("- ") || trimmed.startsWith("* ") || trimmed.startsWith("> ")) {
      score += 1;
    } else if (trimmed.includes("](") || (trimmed.startsWith("|") && trimmed.endsWith("|"))) {
      score += 1;
    }
    if (score >= 2) return score;
  }
  return score;
}

function addIssue(target: DeployPreflightIssue[], code: string, detail: string, hint?: string) {
  target.push({ code, detail, ...(hint ? { hint } : {}) });
}

function selectMainEntry(files: Array<DeployPreflightFile & { bytes: number }>, filename: string): string {
  const hint = filename.trim();
  if (hint && isPagePath(hint)) {
    const exact = files.find((file) => file.path === hint);
    if (exact) return exact.path;
  }
  for (const preferred of PREFERRED_ENTRIES) {
    const match = files.find((file) => file.path.toLowerCase() === preferred.toLowerCase());
    if (match) return match.path;
  }
  return files.find((file) => HTML_EXTENSIONS.test(file.path))?.path ||
    files.find((file) => MARKDOWN_EXTENSIONS.test(file.path))?.path || "";
}

function finish(report: DeployPreflightReport): DeployPreflightReport {
  report.success = report.errors.length === 0;
  return report;
}

export function inspectDeployInput({
  mode,
  content,
  filename,
  files,
  limits
}: {
  mode: DeployPreflightMode;
  content: string;
  filename: string;
  files: DeployPreflightFile[];
  limits?: DeployPreflightLimits;
}): DeployPreflightReport {
  const normalizedLimits = normalizeLimits(limits);
  const report: DeployPreflightReport = {
    success: false,
    sourceType: mode === "single" ? "single" : "multi",
    count: 0,
    bytes: 0,
    limits: normalizedLimits,
    errors: [],
    warnings: []
  };

  if (mode === "single") {
    const value = content || "";
    const bytes = utf8Bytes(value);
    const trimmedBytes = utf8Bytes(value.trim());
    report.count = 1;
    report.bytes = bytes;
    if (!value.trim()) {
      addIssue(report.errors, "ENTRY_MISSING", "单文件内容为空，无法识别入口页面。", "粘贴 HTML 或 Markdown 内容后再发布。");
      return finish(report);
    }
    if (filename.trim() && !isSafeRelativePath(filename.trim())) {
      addIssue(report.errors, "INVALID_FILE_PATH", `入口文件名 ${JSON.stringify(filename.trim())} 不是安全的相对路径。`, "不要使用 ..、绝对路径、盘符或空路径段。");
    }
    if (bytes > normalizedLimits.maxSingleFileBytes) {
      addIssue(report.errors, "CONTENT_TOO_LARGE", `单文件大小为 ${bytes} bytes，超过上限 ${normalizedLimits.maxSingleFileBytes} bytes。`, "删除无关内容，或让管理员调整单文件上限。");
    }
    if (bytes > normalizedLimits.maxSiteTotalBytes) {
      addIssue(report.errors, "ZIP_TOTAL_TOO_LARGE", `整站大小为 ${bytes} bytes，超过上限 ${normalizedLimits.maxSiteTotalBytes} bytes。`, "精简源码或让管理员调整整站大小上限。");
    }
    const kind = inferSingleKind(value);
    const mainEntry = inferSingleEntry(filename, value);
    report.mainEntry = mainEntry;
    report.kind = kind === "markdown" ? "markdown" : "single_html";
    if (kind === "markdown" && trimmedBytes < 3) {
      addIssue(report.errors, "INVALID_INPUT", "主 Markdown 入口太短。", "至少提供一个标题或一段完整文字。");
    }
    if (kind === "html" && (trimmedBytes < 32 || !looksLikeHTML(value))) {
      addIssue(report.errors, "INVALID_INPUT", "主 HTML 入口不像一个完整页面。", "提供包含 <html>、<body>、<main>、<script> 或 <style> 等标签的 HTML。");
    }
    return finish(report);
  }

  if (!files.length) {
    addIssue(report.errors, "ENTRY_MISSING", "还没有选择要部署的文件。", "选择文件、目录或 ZIP 后再继续。");
    return finish(report);
  }

  const prepared = files.map((file) => ({ ...file, path: file.path.trim(), bytes: fileBytes(file) }));
  report.count = prepared.length;
  report.bytes = prepared.reduce((sum, file) => sum + file.bytes, 0);
  if (prepared.length > normalizedLimits.maxFilesPerSite) {
    addIssue(report.errors, "ZIP_TOO_MANY_FILES", `已选择 ${prepared.length} 个文件，超过上限 ${normalizedLimits.maxFilesPerSite} 个。`, "删除构建产物或让管理员调整文件数量上限。");
  }

  const seen = new Set<string>();
  for (const file of prepared) {
    if (!isSafeRelativePath(file.path)) {
      addIssue(report.errors, "ZIP_UNSAFE_PATH", `文件路径 ${JSON.stringify(file.path)} 不是安全的相对路径。`, "路径不能包含 ..、绝对路径、盘符或空路径段。");
    }
    if (seen.has(file.path)) {
      addIssue(report.errors, "ZIP_DUPLICATE_PATH", `文件路径 ${file.path} 重复。`, "删除重复文件，或调整目录层级后再上传。");
    }
    seen.add(file.path);
    if (file.bytes > normalizedLimits.maxSingleFileBytes) {
      addIssue(report.errors, "ZIP_FILE_TOO_LARGE", `文件 ${file.path} 为 ${file.bytes} bytes，超过单文件上限 ${normalizedLimits.maxSingleFileBytes} bytes。`, "压缩或拆分大资源，或让管理员调整单文件上限。");
    }
  }
  if (report.bytes > normalizedLimits.maxSiteTotalBytes) {
    addIssue(report.errors, "ZIP_TOTAL_TOO_LARGE", `整站大小为 ${report.bytes} bytes，超过上限 ${normalizedLimits.maxSiteTotalBytes} bytes。`, "删除无关资源或让管理员调整整站大小上限。");
  }

  if (filename.trim() && !isSafeRelativePath(filename.trim())) {
    addIssue(report.errors, "INVALID_FILE_PATH", `入口文件名 ${JSON.stringify(filename.trim())} 不是安全的相对路径。`, "不要使用 ..、绝对路径、盘符或空路径段。");
  }

  if (prepared.length === 1 && ZIP_EXTENSIONS.test(prepared[0].path)) {
    report.sourceType = "zip";
    report.kind = "zip_site";
    if (prepared[0].bytes === 0) {
      addIssue(report.errors, "ZIP_OPEN_FAILED", "ZIP 文件为空，无法读取归档内容。", "选择一个有效且非空的 ZIP 文件后再继续。");
    }
    report.warnings.push({
      code: "ZIP_SERVER_INSPECTION",
      detail: "ZIP 内容会在服务端解压并检查入口与路径安全。",
      hint: "优先在 ZIP 中放置 index.html 或 README.md，并避免上传无关构建产物。"
    });
    return finish(report);
  }

  if (filename.trim() && isPagePath(filename.trim()) && !prepared.some((file) => file.path === filename.trim())) {
    report.warnings.push({ code: "ENTRY_HINT_NOT_FOUND", detail: `没有找到指定入口 ${filename.trim()}，将自动识别入口。`, hint: "检查入口文件名或清空入口覆盖，让系统按 index.html 优先级识别。" });
  }

  const mainEntry = selectMainEntry(prepared, filename);
  report.mainEntry = mainEntry;
  if (!mainEntry) {
    addIssue(report.errors, "ZIP_ENTRY_MISSING", "没有找到 HTML 或 Markdown 入口。", "添加 index.html、README.md，或填写已有入口文件名。");
    return finish(report);
  }
  const main = prepared.find((file) => file.path === mainEntry);
  if (!main || main.contentBase64 || main.isText === false) {
    addIssue(report.errors, "ZIP_ENTRY_MISSING", `入口 ${mainEntry} 不是可读取的文本页面。`, "确保 HTML/Markdown 入口以文本文件上传。");
  } else if (MARKDOWN_EXTENSIONS.test(mainEntry)) {
    report.kind = "markdown";
    if (utf8Bytes(main.content.trim()) < 3) addIssue(report.errors, "INVALID_INPUT", "主 Markdown 入口太短。", "至少提供一个标题或一段完整文字。");
  } else {
    report.kind = "static_site";
    if (utf8Bytes(main.content.trim()) < 32 || !looksLikeHTML(main.content)) {
      addIssue(report.errors, "INVALID_INPUT", `入口 ${mainEntry} 不像一个完整 HTML 页面。`, "提供有效 HTML，或把 Markdown 文件作为入口。");
    }
  }
  return finish(report);
}
