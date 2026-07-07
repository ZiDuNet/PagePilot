import {
  AlertTriangle,
  Bot,
  ClipboardList,
  Code2,
  Copy,
  Download,
  Eye,
  FileArchive,
  FileText,
  FileUp,
  Heart,
  KeyRound,
  LayoutDashboard,
  Link as LinkIcon,
  Lock,
  Monitor,
  Pin,
  RefreshCw,
  Save,
  ScrollText,
  Search,
  Settings,
  ShieldCheck,
  Trash2,
  Upload,
  UserPlus,
  Users,
  Workflow
} from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { buildDeviceInfoRows, formatDeviceInfoSummary } from "./deviceInfo";

declare module "react" {
  interface InputHTMLAttributes<T> {
    webkitdirectory?: string;
  }
}

type Tab =
  | "overview"
  | "account"
  | "deploy"
  | "sites"
  | "categories"
  | "screens"
  | "tokens"
  | "users"
  | "anonymous"
  | "audit"
  | "config"
  | "skill"
  | "apiDocs";

type DeployMode = "single" | "multi";
type SkillTab = "skill" | "mcp";
type ScreenPickTab = "mine" | "market";
type SiteViewMode = "list" | "render";
type ReusePolicyValue = "auto" | "allow" | "deny";
type SecurityModeValue = "auto" | "strict" | "compatible" | "trusted";

const PREVIEW_IFRAME_SANDBOX =
  "allow-scripts allow-forms allow-popups allow-popups-to-escape-sandbox allow-downloads allow-modals allow-top-navigation-by-user-activation";

interface SessionInfo {
  success?: boolean;
  mode?: string;
  tokenId?: string;
  userId?: string;
  username?: string;
  label?: string;
  isAdmin?: boolean;
  needsSetup?: boolean;
  loginMethod?: string;
}

interface RuntimeConfig {
  currentBaseURL?: string;
  mode?: string;
  corsAllowOrigins?: string;
  embedPolicy?: "any" | "self" | "allowlist" | "deny";
  embedAllowOrigins?: string;
  cooldownSeconds?: number;
  version?: string;
  appURL?: {
    appURLMode?: string;
    appDomainSuffix?: string;
    appURLScheme?: string;
    appURLPort?: string;
  };
  anonymousPolicy?: { deployLimit?: number };
  registrationAllowed?: boolean;
  limits?: {
    maxSingleFileBytes?: number;
    maxSiteTotalBytes?: number;
    maxFilesPerSite?: number;
  };
  email?: {
    verificationEnabled?: boolean;
    smtpConfigured?: boolean;
    smtpHost?: string;
    smtpFrom?: string;
    smtpSecure?: string;
  };
  storage?: {
    backend?: string;
    hostedDir?: string;
    ossProvider?: string;
    ossEndpoint?: string;
    ossBucket?: string;
    ossPrefix?: string;
    ossPublicBaseURL?: string;
    ossConfigured?: boolean;
  };
}

interface SiteItem {
  code: string;
  publicId?: string;
  ownerTokenId?: string;
  ownerUsername?: string;
  title?: string;
  description?: string;
  currentVersion?: number;
  versionCount?: number;
  totalSize?: number;
  viewCount?: number;
  likeCount?: number;
  reuseCount?: number;
  templateSourceCode?: string;
  templateSourceVersion?: number;
  favoriteCount?: number;
  status?: string;
  visibility?: string;
  reusePolicy?: ReusePolicyValue;
  sourceDownloadPolicy?: ReusePolicyValue;
  securityMode?: SecurityModeValue;
  category?: string;
  tags?: string[];
  filename?: string;
  accessProtected?: boolean;
  isPinned?: boolean;
  createdAt?: string;
  lastVersionAt?: string;
}

interface BundleDetail {
  kind?: string;
  kindLabel?: string;
  root?: string;
  mainEntry?: string;
  securityMode?: string;
  siteSecurityMode?: SecurityModeValue;
  effectiveSecurityMode?: string;
  fileCount?: number;
  totalSize?: number;
  tree?: ContentFile[];
  entryNote?: string;
}

interface ContentFile {
  path: string;
  size?: number;
  sha256?: string;
  isBinary?: boolean;
}

interface ReuseDetail {
  downloadUrl?: string;
  detailUrl?: string;
  cli?: string;
  agentPrompt?: string;
  mcp?: Record<string, unknown>;
  allowReuse?: boolean;
  allowDownload?: boolean;
  policyNote?: string;
  templateSourceCode?: string;
  templateSourceVersion?: number;
}

interface SiteDetailResponse {
  success?: boolean;
  site: SiteItem;
  versions?: any[];
  bundle?: BundleDetail;
  files?: ContentFile[];
  reuse?: ReuseDetail;
}

interface MarketplaceItem {
  code: string;
  title?: string;
  description?: string;
  status?: string;
  accessProtected?: boolean;
  likeCount?: number;
  viewCount?: number;
  versionCount?: number;
}

interface MarketCategoryInfo {
  slug: string;
  label: string;
  note?: string;
}

interface MarketCategoriesResponse {
  categories?: MarketCategoryInfo[];
}

interface ScreenItem {
  id: string;
  name?: string;
  deviceName?: string;
  ownerUsername?: string;
  status?: string;
  currentSiteCode?: string;
  currentVersion?: number;
  lastSeenAt?: string;
  runtime?: string;
  appVersion?: string;
  deviceInfo?: unknown;
  screenshotRequestedAt?: string;
  screenshotAt?: string;
  commandType?: string;
  commandRequestedAt?: string;
  commandCompletedAt?: string;
}

interface ScreenScreenshotCommand {
  requestId?: string;
  requestedAt?: string;
}

interface ScreenScreenshotResponse {
  success?: boolean;
  screen?: ScreenItem;
  screenshot?: ScreenScreenshotCommand;
}

interface ScreenshotDialog {
  screenId: string;
  screenName: string;
  status: "waiting" | "ready" | "error";
  message: string;
  requestedAt?: string;
  imageUrl?: string;
}

interface TokenItem {
  id: string;
  label?: string;
  isAdmin?: boolean;
  isRevoked?: boolean;
  ownerUserId?: string;
  ownerUsername?: string;
  expiresAt?: string;
  createdAt?: string;
  lastUsedAt?: string;
}

interface UserItem {
  id: string;
  username: string;
  email?: string;
  emailVerified: boolean;
  isAdmin: boolean;
  isActive: boolean;
  deployLimit: number;
  deployCount: number;
  remaining: number;
  createdAt?: string;
  lastLoginAt?: string;
}

interface AnonymousSession {
  id: string;
  agentId?: string;
  agentLabel?: string;
  deviceIp?: string;
  userAgent?: string;
  deployCount?: number;
  remaining?: number;
  claimedByUserId?: string;
  claimedAt?: string;
  createdAt?: string;
  lastUsedAt?: string;
}

interface AuditLogItem {
  id: number;
  actorType?: string;
  actorId?: string;
  actorRole?: string;
  action?: string;
  result?: string;
  siteCode?: string;
  targetType?: string;
  targetId?: string;
  ip?: string;
  userAgent?: string;
  detail?: unknown;
  createdAt?: string;
}

interface AuditLogResponse {
  success?: boolean;
  logs?: AuditLogItem[];
  total?: number;
  page?: number;
  pageSize?: number;
}

interface DeployFile {
  path: string;
  content: string;
  contentBase64?: string;
  isText: boolean;
  size: number;
}

interface Captcha {
  id?: string;
  prompt?: string;
  image?: string;
}

class APIError extends Error {
  status?: number;
  body?: unknown;
}

interface StructuredAPIErrorPayload {
  errorCode?: string;
  detail?: string;
  stage?: string;
  hint?: string;
  requestId?: string;
  retryAfter?: number;
  [key: string]: unknown;
}

const navItems: Array<{ tab: Tab; label: string; icon: React.ReactNode; adminOnly?: boolean }> = [
  { tab: "overview", label: "总览", icon: <LayoutDashboard size={18} /> },
  { tab: "deploy", label: "发布应用", icon: <Upload size={18} /> },
  { tab: "sites", label: "应用管理", icon: <ClipboardList size={18} /> },
  { tab: "categories", label: "应用分类", icon: <FileText size={18} />, adminOnly: true },
  { tab: "screens", label: "屏幕管理", icon: <Monitor size={18} /> },
  { tab: "tokens", label: "API 令牌", icon: <KeyRound size={18} /> },
  { tab: "account", label: "账号设置", icon: <ShieldCheck size={18} /> },
  { tab: "users", label: "用户管理", icon: <Users size={18} />, adminOnly: true },
  { tab: "anonymous", label: "匿名 Agent", icon: <Bot size={18} />, adminOnly: true },
  { tab: "audit", label: "审计日志", icon: <ScrollText size={18} />, adminOnly: true },
  { tab: "config", label: "运行设置", icon: <Settings size={18} />, adminOnly: true },
  { tab: "skill", label: "Skill & MCP", icon: <Workflow size={18} />, adminOnly: true },
  { tab: "apiDocs", label: "API 文档", icon: <Code2 size={18} /> }
];

function initialAdminTab(): Tab {
  if (typeof location === "undefined") return "overview";
  const tab = new URLSearchParams(location.search).get("tab") as Tab | null;
  return tab && navItems.some((item) => item.tab === tab) ? tab : "overview";
}

function authHeaders(headers: Record<string, string> = {}) {
  if (typeof location !== "undefined" && !headers["X-Hostctl-Current-Origin"]) {
    headers["X-Hostctl-Current-Origin"] = location.origin;
  }
  const token = localStorage.getItem("hostctl-admin-token") || localStorage.getItem("hostctl-token") || "";
  return token && !headers.Authorization ? { ...headers, Authorization: `Bearer ${token}` } : headers;
}

function userMessage(message: string) {
  const map: Record<string, string> = {
    "category slug must use lowercase letters, numbers, and hyphens": "分类标识只能使用小写字母、数字和连字符，例如 landing-page。",
    "category slug must be unique": "分类标识不能重复。",
    "category label is required": "请填写分类名称。",
    "category slug is invalid": "分类标识格式不正确。"
  };
  return map[message] || message;
}

async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = authHeaders((init.headers as Record<string, string>) || {});
  const res = await fetch(path, { credentials: "same-origin", cache: init.method ? "no-store" : "default", ...init, headers });
  const text = await res.text();
  let body: any = null;
  try {
    body = text ? JSON.parse(text) : null;
  } catch {
    body = text;
  }
  if (!res.ok) {
    const err = new APIError(userMessage(body?.detail || body?.errorCode || `HTTP ${res.status}`));
    err.status = res.status;
    err.body = body;
    throw err;
  }
  return body as T;
}

function formatSize(value?: number) {
  const n = Number(value || 0);
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / 1024 / 1024).toFixed(2)} MB`;
}

function fileDirectory(path: string): string {
  const idx = path.lastIndexOf("/");
  return idx > 0 ? path.slice(0, idx) : "根目录";
}

function FileTreeExplorer({
  files,
  totalSize,
  onCopy
}: {
  files: ContentFile[];
  totalSize?: number;
  onCopy: (value?: string) => void | Promise<void>;
}) {
  const [query, setQuery] = useState("");
  const normalizedQuery = query.trim().toLowerCase();
  const folders = new Set<string>();
  let computedSize = 0;
  for (const file of files) {
    computedSize += Number(file.size || 0);
    const parts = file.path.split("/").filter(Boolean);
    for (let i = 1; i < parts.length; i += 1) {
      folders.add(parts.slice(0, i).join("/"));
    }
  }
  const filtered = normalizedQuery
    ? files.filter((file) => `${file.path} ${file.sha256 || ""}`.toLowerCase().includes(normalizedQuery))
    : files;
  const visible = filtered.slice(0, 160);

  return (
    <div className="file-tree-panel">
      <div className="file-tree-toolbar">
        <label className="file-tree-search">
          <Search size={13} />
          <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索路径、文件名或 SHA" />
        </label>
        <div className="file-tree-stats" aria-label="文件树统计">
          <span>{files.length} 文件</span>
          <span>{folders.size} 目录</span>
          <span>{formatSize(totalSize || computedSize)}</span>
        </div>
      </div>
      <div className="detail-file-tree enhanced">
        {visible.map((file) => (
          <div className={`file-tree-row ${file.isBinary ? "is-binary" : ""}`} key={file.path}>
            <button className="file-tree-path-button" type="button" title="复制文件路径" onClick={() => void onCopy(file.path)}>
              <code>{file.path}</code>
              <small>{fileDirectory(file.path)} · {file.isBinary ? "二进制" : "文本"}</small>
            </button>
            <div className="file-tree-meta">
              <em>{formatSize(file.size || 0)}</em>
              {file.sha256 && (
                <button type="button" title="复制 SHA-256" onClick={() => void onCopy(file.sha256)}>
                  <Copy size={11} />SHA
                </button>
              )}
            </div>
          </div>
        ))}
        {!visible.length && <span className="file-tree-empty">没有匹配的文件</span>}
      </div>
      {filtered.length > visible.length && (
        <p className="file-tree-more">已显示前 {visible.length} 个文件，可继续搜索缩小范围。</p>
      )}
    </div>
  );
}

function structuredAPIError(err: unknown): StructuredAPIErrorPayload | null {
  if (!(err instanceof APIError)) return null;
  if (!err.body || typeof err.body !== "object" || Array.isArray(err.body)) return null;
  return err.body as StructuredAPIErrorPayload;
}

function plainErrorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

function friendlyAuthErrorMessage(err: unknown): string {
  const payload = structuredAPIError(err);
  const fallback = plainErrorMessage(err);
  const detail = String(payload?.detail || fallback || "").toLowerCase();
  const code = String(payload?.errorCode || "").toUpperCase();
  const stage = String(payload?.stage || "").toLowerCase();

  if (detail.includes("username or password is incorrect") || (code === "UNAUTHORIZED" && stage === "auth")) {
    return "用户名或密码不正确，请检查后重新输入。";
  }
  if (detail.includes("captcha is incorrect or expired") || stage === "captcha") {
    return "验证码不正确或已过期，请刷新后重试。";
  }
  if (detail.includes("email verification code is incorrect or expired") || stage === "email") {
    return "邮箱验证码不正确或已过期，请重新获取。";
  }
  if (detail.includes("username already exists or password is invalid") || stage === "register") {
    return "用户名已存在，或密码不符合要求。请换一个用户名并使用至少 8 位密码。";
  }
  if (detail.includes("login required")) {
    return "请先登录后再继续操作。";
  }
  return fallback || "操作失败，请稍后重试。";
}

function errorStageLabel(stage?: string): string {
  const normalized = String(stage || "").toLowerCase();
  const labels: Record<string, string> = {
    zip_bundle: "ZIP / Bundle 识别",
    validate: "发布参数校验",
    file_path: "文件路径校验",
    content: "内容校验",
    load_site: "站点读取",
    source_download: "源码下载"
  };
  return labels[normalized] || stage || "发布处理";
}

function deployErrorTitle(payload: StructuredAPIErrorPayload | null): string {
  const code = String(payload?.errorCode || "").toUpperCase();
  const stage = String(payload?.stage || "").toLowerCase();
  if (code.startsWith("ZIP_") || stage === "zip_bundle") return "ZIP / 多文件包需要调整";
  if (code === "INVALID_DESCRIPTION") return "请补充应用描述";
  if (code === "CONTENT_TOO_LARGE") return "上传内容超过限制";
  if (code === "INVALID_FILE_PATH") return "文件路径不符合要求";
  if (code === "INVALID_CUSTOM_CODE") return "自定义 code 不可用";
  if (code === "RATE_LIMITED") return "发布过于频繁";
  return "发布失败";
}

function isCommonDeployValidationError(payload: StructuredAPIErrorPayload | null): boolean {
  const code = String(payload?.errorCode || "").toUpperCase();
  return code === "INVALID_DESCRIPTION";
}

function friendlyDeployErrorMessage(payload: StructuredAPIErrorPayload | null, fallback: string): string {
  const code = String(payload?.errorCode || "").toUpperCase();
  if (code === "INVALID_DESCRIPTION") {
    return "请填写一句话描述，说明这个应用是做什么的。";
  }
  return payload?.detail || fallback;
}

function deployErrorHints(payload: StructuredAPIErrorPayload | null): string[] {
  const hints = new Set<string>();
  const code = String(payload?.errorCode || "").toUpperCase();
  const stage = String(payload?.stage || "").toLowerCase();
  if (code === "INVALID_DESCRIPTION") {
    hints.add("描述会展示在创作市场和后台列表里，请控制在 240 个字符以内。");
    hints.add("例如：面向门店大屏展示的新品活动页。");
    return Array.from(hints);
  }
  if (payload?.hint) hints.add(String(payload.hint));
  if (stage === "zip_bundle" || code.startsWith("ZIP_")) {
    hints.add("请确认 ZIP 里只有一个可发布的网站根目录，优先放置 index.html 或 README.md。");
    hints.add("如果入口文件不在根目录，可以在入口文件名里填写真实相对路径后再发布。");
  }
  if (code === "ZIP_AMBIGUOUS_ENTRY") {
    hints.add("检测到多个候选入口，请把每个网站拆成独立 ZIP，或只保留一个入口。");
  }
  if (code === "ZIP_ENTRY_MISSING") {
    hints.add("没有找到 HTML 或 Markdown 入口，请添加 index.html、README.md 或明确填写入口文件名。");
  }
  if (code === "ZIP_UNSAFE_PATH" || code === "INVALID_FILE_PATH") {
    hints.add("文件路径只能使用相对路径，不能包含 ..、盘符、反斜杠开头或空路径段。");
  }
  if (code === "ZIP_FILE_TOO_LARGE" || code === "ZIP_TOTAL_TOO_LARGE" || code === "ZIP_TOO_MANY_FILES" || code === "CONTENT_TOO_LARGE") {
    hints.add("请压缩资源、删除无关文件，或调整单文件、文件数和整站大小上限。");
  }
  if (!hints.size) {
    hints.add("请检查标题、描述、code、入口文件和上传内容是否完整，再重新发布。");
  }
  return Array.from(hints);
}

function DeployErrorPanel({ message, error }: { message: string; error: StructuredAPIErrorPayload | null }) {
  const [copied, setCopied] = useState(false);
  const hints = deployErrorHints(error);
  const displayMessage = friendlyDeployErrorMessage(error, message);
  const diagnostics = JSON.stringify({ message, ...(error || {}) }, null, 2);
  const copyDiagnostics = async () => {
    await navigator.clipboard.writeText(diagnostics);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1600);
  };

  return (
    <div className="deploy-error-card" role="alert">
      <div className="deploy-error-head">
        <span className="deploy-error-icon"><AlertTriangle size={18} /></span>
        <div>
          <strong>{deployErrorTitle(error)}</strong>
          <p>{displayMessage}</p>
        </div>
      </div>
      {error && !isCommonDeployValidationError(error) && (
        <div className="deploy-error-badges">
          {error.errorCode && <code>{error.errorCode}</code>}
          {error.stage && <span>{errorStageLabel(error.stage)}</span>}
          {error.retryAfter && <span>{error.retryAfter}s 后重试</span>}
          {error.requestId && <span>请求 {error.requestId}</span>}
        </div>
      )}
      <ul className="deploy-error-hints">
        {hints.map((hint) => <li key={hint}>{hint}</li>)}
      </ul>
      {error && (
        <button className="deploy-error-copy" type="button" onClick={() => void copyDiagnostics()}>
          <Copy size={13} />{copied ? "已复制诊断信息" : "复制诊断信息"}
        </button>
      )}
    </div>
  );
}

function formatDate(value?: string) {
  if (!value) return "-";
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? value : d.toLocaleString("zh-CN", { hour12: false });
}

function textSize(text: string) {
  return new Blob([text]).size;
}

function parseAdminTagInput(value: string): string[] {
  const seen = new Set<string>();
  return value
    .split(/[,，;；\n]/)
    .map((item) => item.trim().replace(/^#+/, ""))
    .filter(Boolean)
    .filter((item) => {
      const key = item.toLowerCase();
      if (seen.has(key)) return false;
      seen.add(key);
      return true;
    })
    .slice(0, 6)
    .map((item) => Array.from(item).slice(0, 24).join(""));
}

function isTextFile(name: string) {
  return /\.(html?|css|js|mjs|json|txt|md|svg|xml|csv|webmanifest|map)$/i.test(name);
}

function isZipFile(name: string) {
  return /\.zip$/i.test(name);
}

function normalizeUploadedZipPath(name: string) {
  return isZipFile(name) ? "site.zip" : name;
}

function isDeployEntrypointFile(name: string) {
  return /\.(html?|md|markdown)$/i.test(name);
}

function preferredDeployEntryPath(files: DeployFile[], hint = "") {
  const cleanHint = hint.trim();
  if (cleanHint && isDeployEntrypointFile(cleanHint) && files.some((file) => file.path === cleanHint)) {
    return cleanHint;
  }
  const preferred = ["index.html", "index.htm", "README.md", "readme.md", "README.markdown", "readme.markdown"];
  for (const want of preferred) {
    const hit = files.find((file) => file.path.toLowerCase() === want.toLowerCase());
    if (hit) return hit.path;
  }
  return files.find((file) => isDeployEntrypointFile(file.path))?.path || "";
}

function toBase64(buffer: ArrayBuffer) {
  const bytes = new Uint8Array(buffer);
  let binary = "";
  for (let i = 0; i < bytes.length; i += 0x8000) {
    binary += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
  }
  return btoa(binary);
}

function sleep(ms: number) {
  return new Promise((resolve) => window.setTimeout(resolve, ms));
}

function currentOrigin() {
  return typeof location === "undefined" ? "https://pagepilot.example.com" : location.origin;
}

function currentBaseURL() {
  return currentOrigin().replace(/\/+$/, "");
}

function sameSiteURL(url?: string) {
  if (!url) return "";
  if (!/^https?:\/\//i.test(url)) return url;
  try {
    const parsed = new URL(url);
    if (!parsed.pathname.startsWith("/agent/") && !parsed.pathname.startsWith("/deploy/") && !parsed.pathname.startsWith("/api/")) {
      return url;
    }
    return `${currentOrigin()}${parsed.pathname}${parsed.search}${parsed.hash}`;
  } catch {
    return url;
  }
}

function skillDownloadPath() {
  return "/skill/pagep.zip";
}

function siteURL(code: string) {
  return `/agent/${encodeURIComponent(code)}/`;
}

function securityModeLabel(mode?: string) {
  if (mode === "strict") return "严格";
  if (mode === "trusted") return "受信任";
  if (mode === "compatible") return "兼容";
  if (mode === "standard") return "标准";
  return mode || "标准";
}

export default function App() {
  const [authChecking, setAuthChecking] = useState(true);
  const [session, setSession] = useState<SessionInfo | null>(null);
  const [config, setConfig] = useState<RuntimeConfig | null>(null);
  const [requestedAuthMode] = useState<"login" | "register">(() => {
    if (typeof window === "undefined") return "login";
    return new URLSearchParams(window.location.search).get("mode") === "register" ? "register" : "login";
  });
  const [activeTab, setActiveTab] = useState<Tab>(() => initialAdminTab());
  const [toast, setToast] = useState("");
  const [error, setError] = useState("");

  const isAdmin = !!session?.isAdmin;
  const visibleNav = navItems.filter((item) => !item.adminOnly || isAdmin);

  const selectTab = useCallback((tab: Tab) => {
    setActiveTab(tab);
    if (typeof history !== "undefined") {
      const url = new URL(location.href);
      url.searchParams.set("tab", tab);
      history.replaceState({}, "", `${url.pathname}${url.search}${url.hash}`);
    }
  }, []);

  const showToast = useCallback((message: string) => {
    setToast(message);
    window.clearTimeout((showToast as any).timer);
    (showToast as any).timer = window.setTimeout(() => setToast(""), 2200);
  }, []);

  const refreshSession = useCallback(async (token?: string) => {
    if (typeof token === "string") {
      if (token) localStorage.setItem("hostctl-admin-token", token);
      else localStorage.removeItem("hostctl-admin-token");
    }
    const data = await api<SessionInfo>("/api/admin/session");
    setSession(data);
    return data;
  }, []);

  const probeSession = useCallback(async () => {
    const res = await fetch("/api/admin/session", {
      credentials: "same-origin",
      cache: "no-store",
      headers: authHeaders()
    });
    const data = await res.json().catch(() => ({}));
    setSession(data as SessionInfo);
    return data as SessionInfo;
  }, []);

  const refreshConfig = useCallback(async () => {
    const data = await api<RuntimeConfig>("/api/config");
    setConfig(data);
    return data;
  }, []);

  useEffect(() => {
    async function boot() {
      try {
        await refreshConfig();
      } catch {
        setConfig(null);
      }
      try {
        await probeSession();
      } catch (err) {
        if (err instanceof APIError && err.status === 401 && err.body && typeof err.body === "object") {
          setSession(err.body as SessionInfo);
        } else {
          setSession(null);
        }
      } finally {
        setAuthChecking(false);
      }
    }
    void boot();
  }, [probeSession, refreshConfig]);

  useEffect(() => {
    const current = navItems.find((item) => item.tab === activeTab);
    if (session?.success && current?.adminOnly && !isAdmin) {
      selectTab("overview");
    }
  }, [activeTab, isAdmin, selectTab, session?.success]);

  useEffect(() => {
    if (!error) return;
    const timer = window.setTimeout(() => setError(""), 5200);
    return () => window.clearTimeout(timer);
  }, [error]);

  function logout() {
    localStorage.removeItem("hostctl-admin-token");
    fetch("/api/admin/logout", { method: "POST" }).finally(() => location.reload());
  }

  if (authChecking) {
    return <div className="auth-loading">正在进入 PagePilot 后台...</div>;
  }

  if (!session?.success) {
    return (
      <LoginScreen
        config={config}
        needsSetup={!!session?.needsSetup}
        requestedMode={requestedAuthMode}
        onAuthed={(next) => setSession(next)}
        onConfig={setConfig}
      />
    );
  }

  return (
    <div className="admin-shell">
      <aside className="sidebar">
        <a className="brand" href="/">
          <Logo />
          <span>PagePilot</span>
        </a>
        <nav className="nav-list" aria-label="后台导航">
          {visibleNav.map((item) => (
            <button
              className={activeTab === item.tab ? "nav-item active" : "nav-item"}
              key={item.tab}
              type="button"
              onClick={() => selectTab(item.tab)}
            >
              {item.icon}
              <span>{item.label}</span>
            </button>
          ))}
        </nav>
        <div className="side-footer">
          <span className="session-pill">{session.username || session.label || session.tokenId || "当前用户"}</span>
          <a className="button ghost" href="/">返回首页</a>
          <button className="button ghost" type="button" onClick={logout}>退出登录</button>
        </div>
      </aside>
      <main className="main">
        <header className="topline">
          <div>
            <span className="eyebrow">{session.mode === "dev" ? "DEV" : "PROD"} · {isAdmin ? "管理员" : "用户"}</span>
            <h1>{tabTitle(activeTab, isAdmin)}</h1>
            <p>{tabSubtitle(activeTab, isAdmin)}</p>
          </div>
          <div className="top-actions">
            <button className="button" type="button" onClick={() => void refreshConfig().then(() => showToast("配置已刷新"))}>
              <RefreshCw size={16} />刷新配置
            </button>
          </div>
        </header>

        {error && <div className="alert error global-alert"><span>{error}</span><button type="button" onClick={() => setError("")}>关闭</button></div>}
        {activeTab === "overview" && <Overview config={config} session={session} setError={setError} setTab={selectTab} />}
        {activeTab === "account" && <AccountPanel session={session} showToast={showToast} />}
        {activeTab === "deploy" && <DeployPanel config={config} showToast={showToast} setError={setError} />}
        {activeTab === "sites" && <SitesPanel isAdmin={isAdmin} showToast={showToast} setError={setError} />}
        {activeTab === "categories" && isAdmin && <CategoriesPanel showToast={showToast} setError={setError} />}
        {activeTab === "screens" && <ScreensPanel isAdmin={isAdmin} showToast={showToast} setError={setError} />}
        {activeTab === "tokens" && <TokensPanel isAdmin={isAdmin} showToast={showToast} setError={setError} />}
        {activeTab === "users" && isAdmin && <UsersPanel showToast={showToast} setError={setError} />}
        {activeTab === "anonymous" && isAdmin && <AnonymousPanel setError={setError} />}
        {activeTab === "audit" && isAdmin && <AuditPanel setError={setError} />}
        {activeTab === "config" && isAdmin && <ConfigPanel config={config} onConfig={setConfig} showToast={showToast} setError={setError} />}
        {activeTab === "skill" && isAdmin && <SkillMCPPanel config={config} showToast={showToast} setError={setError} />}
        {activeTab === "apiDocs" && <ApiDocsPanel config={config} />}
      </main>
      {toast && <div className="toast">{toast}</div>}
    </div>
  );
}

function tabTitle(tab: Tab, isAdmin: boolean) {
  const titles: Record<Tab, string> = {
    overview: isAdmin ? "管理总览" : "我的工作台",
    account: "账号设置",
    deploy: "发布应用",
    sites: isAdmin ? "应用管理" : "我的应用",
    categories: "应用分类",
    screens: "屏幕管理",
    tokens: "API 令牌",
    users: "用户管理",
    anonymous: "匿名管理",
    audit: "审计日志",
    config: "运行设置",
    skill: "Skill / MCP / CLI",
    apiDocs: "API 文档"
  };
  return titles[tab];
}

function tabSubtitle(tab: Tab, isAdmin: boolean) {
  const subtitles: Record<Tab, string> = {
    overview: isAdmin ? "查看全站发布、用户、Agent 和屏幕状态。" : "查看你的发布、Token 和屏幕操作入口。",
    account: "修改密码，确认当前登录身份。",
    deploy: "上传 HTML、Markdown、ZIP 或多文件静态站点，新建应用或追加版本。",
    sites: isAdmin ? "管理全站应用、归属、加密、分类、数据和版本。" : "管理你发布的应用、访问密码和版本。",
    categories: "维护创作市场应用分类，供发布页、市场筛选、Skill/MCP 共用。",
    screens: "绑定屏幕、选择应用投放、刷新 WebView 和下发截图指令。",
    tokens: "创建永久或临时 API Token，供 Skill、MCP、CLI 和 Agent 调用。",
    users: "创建账号、调整额度、停用或删除用户。",
    anonymous: "查看未登录发布产生的网页匿名和 Agent 匿名 session。",
    audit: "追踪发布、站点、Token、屏幕、用户和运行设置的关键操作。",
    config: "调整应用泛域名、上传额度、CORS 和匿名额度。",
    skill: "维护 Skill 下载包，并查看 MCP/CLI 接入说明。",
    apiDocs: "查看 PagePilot HTTP API、认证方式和 OpenAPI 入口。"
  };
  return subtitles[tab];
}

function LoginScreen({
  config,
  needsSetup,
  requestedMode,
  onAuthed,
  onConfig
}: {
  config: RuntimeConfig | null;
  needsSetup: boolean;
  requestedMode: "login" | "register";
  onAuthed: (session: SessionInfo) => void;
  onConfig: (cfg: RuntimeConfig) => void;
}) {
  const [authMode, setAuthModeState] = useState<"login" | "register">(requestedMode);
  const [captcha, setCaptcha] = useState<Captcha>({});
  const [username, setUsername] = useState("");
  const [email, setEmail] = useState("");
  const [emailCode, setEmailCode] = useState("");
  const [password, setPassword] = useState("");
  const [captchaAnswer, setCaptchaAnswer] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [emailBusy, setEmailBusy] = useState(false);
  const registrationAllowed = needsSetup || config?.registrationAllowed !== false;
  const emailVerificationEnabled = !!config?.email?.verificationEnabled;
  const smtpReady = !!config?.email?.smtpConfigured;

  const setAuthMode = useCallback((next: "login" | "register") => {
    setAuthModeState(next);
    if (typeof window === "undefined") return;
    const url = new URL(window.location.href);
    if (next === "register") {
      url.searchParams.set("mode", "register");
    } else {
      url.searchParams.delete("mode");
    }
    window.history.replaceState({}, "", `${url.pathname}${url.search}${url.hash}`);
  }, []);

  const loadCaptcha = useCallback(async () => {
    const data = await api<Captcha>("/api/auth/captcha");
    setCaptcha(data);
    setCaptchaAnswer("");
  }, []);

  useEffect(() => {
    void loadCaptcha();
    api<RuntimeConfig>("/api/config").then(onConfig).catch(() => undefined);
  }, [loadCaptcha, onConfig]);

  useEffect(() => {
    if (!registrationAllowed && authMode === "register") {
      setAuthMode("login");
    }
  }, [authMode, registrationAllowed, setAuthMode]);

  useEffect(() => {
    if (registrationAllowed && requestedMode === "register") {
      setAuthModeState("register");
    }
  }, [registrationAllowed, requestedMode]);

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    setError("");
    setBusy(true);
    try {
      if (!username.trim() || !password) throw new Error("请输入用户名和密码");
      if (!captcha.id || !captchaAnswer.trim()) throw new Error("请输入验证码");
      if (needsSetup) {
        await api("/api/admin/setup", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ username: username.trim(), password, captchaId: captcha.id, captcha: captchaAnswer.trim() })
        });
        setAuthMode("login");
        setPassword("");
        await loadCaptcha();
        setError("管理员已创建，请登录");
        return;
      }
      if (authMode === "register") {
        if (!registrationAllowed) throw new Error("当前服务器已关闭新用户注册，请联系管理员添加账号。");
        if (emailVerificationEnabled && !email.trim()) throw new Error("请输入邮箱地址");
        if (emailVerificationEnabled && !emailCode.trim()) throw new Error("请输入邮箱验证码");
        await api("/api/auth/register", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ username: username.trim(), email: email.trim(), password, captchaId: captcha.id, captcha: captchaAnswer.trim(), emailCode: emailCode.trim() })
        });
        setAuthMode("login");
        setEmail("");
        setEmailCode("");
        setPassword("");
        await loadCaptcha();
        setError("注册成功，请登录");
        return;
      }
      await api("/api/admin/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username: username.trim(), password, captchaId: captcha.id, captcha: captchaAnswer.trim() })
      });
      localStorage.removeItem("hostctl-admin-token");
      const session = await api<SessionInfo>("/api/admin/session");
      onAuthed(session);
    } catch (err) {
      localStorage.removeItem("hostctl-admin-token");
      setError(friendlyAuthErrorMessage(err));
      void loadCaptcha();
    } finally {
      setBusy(false);
    }
  }

  async function requestEmailCode() {
    setError("");
    if (!emailVerificationEnabled) return;
    if (!smtpReady) {
      setError("当前服务器已开启邮箱验证，但 SMTP 尚未配置，请联系管理员。");
      return;
    }
    if (!email.trim()) {
      setError("请输入邮箱地址");
      return;
    }
    if (!captcha.id || !captchaAnswer.trim()) {
      setError("请先输入图片验证码");
      return;
    }
    setEmailBusy(true);
    try {
      await api("/api/auth/email-code", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email: email.trim(), captchaId: captcha.id, captcha: captchaAnswer.trim() })
      });
      setError("邮箱验证码已发送，10 分钟内有效");
      await loadCaptcha();
    } catch (err) {
      setError(friendlyAuthErrorMessage(err));
      void loadCaptcha();
    } finally {
      setEmailBusy(false);
    }
  }

  const title = needsSetup ? "创建管理员" : authMode === "register" ? "注册账号" : "用户登录";
  const success = error.includes("成功") || error.includes("已创建") || error.includes("success");

  return (
    <main className="login-page login-page-refined">
      <section className="login-card login-card-refined">
        <div className="brand login-brand"><Logo /><span>PagePilot</span></div>
        <span className="eyebrow">USER CENTER</span>
        <h1>{title}</h1>
        <p>{needsSetup ? "第一次启动需要创建管理员账号，之后可以在账号设置里修改密码。" : "登录后进入用户中心；管理员账号会自动显示更多管理能力。"}</p>
        <form onSubmit={submit}>
          <label className="field"><span>用户名</span><input value={username} onChange={(event) => setUsername(event.target.value)} autoComplete="username" placeholder="pagepilot" /></label>
          {authMode === "register" && emailVerificationEnabled && (
            <>
              <label className="field"><span>邮箱</span><input value={email} onChange={(event) => setEmail(event.target.value)} autoComplete="email" inputMode="email" placeholder="name@example.com" /></label>
              <label className="field">
                <span>邮箱验证码</span>
                <div className="inline-input">
                  <input value={emailCode} onChange={(event) => setEmailCode(event.target.value.replace(/\D/g, "").slice(0, 6))} inputMode="numeric" placeholder="6 位数字" />
                  <button className="button" type="button" disabled={emailBusy || !smtpReady} onClick={() => void requestEmailCode()}>{emailBusy ? "发送中..." : "获取验证码"}</button>
                </div>
                {!smtpReady && <em className="field-hint">服务器已开启邮箱验证，但 SMTP 尚未配置。</em>}
              </label>
            </>
          )}
          <label className="field"><span>密码</span><input value={password} onChange={(event) => setPassword(event.target.value)} type="password" autoComplete="current-password" placeholder="至少 8 个字符" /></label>
          <label className="field captcha-field">
            <span>{captcha.prompt || "验证码"}</span>
            {captcha.image && <img className="captcha-image" src={captcha.image} alt="验证码" />}
            <div className="inline-input">
              <input value={captchaAnswer} onChange={(event) => setCaptchaAnswer(event.target.value.replace(/\D/g, "").slice(0, 4))} inputMode="numeric" placeholder="4 位数字" />
              <button className="button" type="button" onClick={() => void loadCaptcha()}>换一张</button>
            </div>
          </label>
          {error && <div className={success ? "alert success" : "alert error"}>{error}</div>}
          <button className="button primary full" type="submit" disabled={busy}>{busy ? "提交中..." : needsSetup ? "创建管理员" : authMode === "register" ? "注册账号" : "登录"}</button>
        </form>
        {!needsSetup && registrationAllowed && <button className="button ghost full" type="button" onClick={() => setAuthMode(authMode === "register" ? "login" : "register")}>{authMode === "register" ? "已有账号，去登录" : "没有账号，去注册"}</button>}
        {!needsSetup && !registrationAllowed && <div className="hint-line">当前服务器已关闭公开注册，请联系管理员创建账号。</div>}
        <div className="hint-line">当前服务：{currentOrigin()}</div>
      </section>
      <aside className="login-aside">
        <strong>一个入口，两种权限</strong>
        <span>普通用户只看到自己的应用、收藏、令牌和屏幕；管理员登录后才会出现用户、匿名、运行设置等全站能力。</span>
        <div><ShieldCheck size={18} /><em>后台使用 Cookie Session 登录，API Token 在用户中心单独创建。</em></div>
      </aside>
    </main>
  );
}

function Overview({ config, session, setError, setTab }: { config: RuntimeConfig | null; session: SessionInfo; setError: (msg: string) => void; setTab: (tab: Tab) => void }) {
  const [sites, setSites] = useState<SiteItem[]>([]);
  const [screens, setScreens] = useState<ScreenItem[]>([]);
  const [tokens, setTokens] = useState<TokenItem[]>([]);
  const [anonymous, setAnonymous] = useState<AnonymousSession[]>([]);

  const isAdmin = !!session.isAdmin;

  useEffect(() => {
    async function load() {
      try {
        const [siteData, screenData, tokenData] = await Promise.all([
          api<{ sites?: SiteItem[] }>("/api/admin/sites"),
          api<{ screens?: ScreenItem[] }>("/api/screens"),
          api<{ tokens?: TokenItem[] }>("/api/tokens")
        ]);
        setSites(siteData.sites || []);
        setScreens(screenData.screens || []);
        setTokens(tokenData.tokens || []);
        if (isAdmin) {
          const anonData = await api<{ sessions?: AnonymousSession[] }>("/api/admin/anonymous-sessions");
          setAnonymous(anonData.sessions || []);
        }
      } catch (err) {
        setError(err instanceof Error ? err.message : String(err));
      }
    }
    void load();
  }, [isAdmin, setError]);

  const versions = sites.reduce((sum, site) => sum + Number(site.versionCount || 0), 0);
  const storage = sites.reduce((sum, site) => sum + Number(site.totalSize || 0), 0);

  return (
    <section className="page-grid">
      <div className="stats-grid">
        <Metric label="Sites" value={String(sites.length)} note="已发布站点" />
        <Metric label="Versions" value={String(versions)} note="累计版本" />
        <Metric label="Screens" value={String(screens.length)} note="已绑定屏幕" />
        <Metric label="Storage" value={formatSize(storage)} note="版本文件总量" />
        <Metric label="Tokens" value={String(tokens.filter((t) => !t.isRevoked).length)} note="可用 Token" />
        {isAdmin && <Metric label="Anonymous" value={String(anonymous.length)} note={`每个 session ${config?.anonymousPolicy?.deployLimit ?? "-"} 次`} />}
      </div>
      <div className="panel">
        <div className="panel-head">
          <div>
            <h2>常用操作</h2>
            <p>高频动作只保留在总览，其他页面专注处理当前任务。</p>
          </div>
        </div>
        <div className="quick-actions">
          <button className="action-card" type="button" onClick={() => setTab("deploy")}><Upload size={18} /><strong>发布应用</strong><span>新建或追加版本</span></button>
          <button className="action-card" type="button" onClick={() => setTab("sites")}><ClipboardList size={18} /><strong>管理站点</strong><span>加密、删除、版本</span></button>
          <button className="action-card" type="button" onClick={() => setTab("screens")}><Monitor size={18} /><strong>硬件屏幕</strong><span>投放和截图</span></button>
          <button className="action-card" type="button" onClick={() => setTab("tokens")}><KeyRound size={18} /><strong>Agent 接入</strong><span>创建永久/临时 Token</span></button>
        </div>
      </div>
      <div className="panel">
        <div className="panel-head">
          <div><h2>最近站点</h2><p>最新更新在前。</p></div>
        </div>
        <div className="table-wrap compact">
          <table>
            <thead><tr><th>Code</th><th>状态</th><th>版本</th><th>修改</th></tr></thead>
            <tbody>
              {sites.slice(0, 8).map((site) => (
                <tr key={site.code}>
                  <td><code>{site.code}</code></td>
                  <td>{statusBadge(site.status || "active", site.accessProtected)}</td>
                  <td>v{site.currentVersion || "-"} · {site.versionCount || 0}</td>
                  <td>{formatDate(site.lastVersionAt || site.createdAt)}</td>
                </tr>
              ))}
              {!sites.length && <tr><td colSpan={4}>暂无站点。</td></tr>}
            </tbody>
          </table>
        </div>
      </div>
    </section>
  );
}

function AccountPanel({ session, showToast }: { session: SessionInfo; showToast: (msg: string) => void }) {
  const [oldPassword, setOldPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [error, setError] = useState("");

  async function save() {
    setError("");
    try {
      await api("/api/account/password", {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ oldPassword, newPassword })
      });
      setOldPassword("");
      setNewPassword("");
      showToast("密码已修改");
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  return (
    <section className="two-col">
      <div className="panel">
        <div className="panel-head"><div><h2>账号信息</h2><p>当前登录身份。</p></div></div>
        <InfoRow label="用户名" value={session.username || session.label || "-"} />
        <InfoRow label="角色" value={session.isAdmin ? "管理员" : "用户"} />
        <InfoRow label="登录方式" value={session.loginMethod || "token/dev"} />
        <InfoRow label="用户 ID" value={session.userId || session.tokenId || "-"} />
      </div>
      <div className="panel">
        <div className="panel-head"><div><h2>修改密码</h2><p>修改后请用新密码重新登录其他设备。</p></div></div>
        <label className="field"><span>当前密码</span><input type="password" value={oldPassword} onChange={(event) => setOldPassword(event.target.value)} /></label>
        <label className="field"><span>新密码</span><input type="password" value={newPassword} onChange={(event) => setNewPassword(event.target.value)} placeholder="至少 8 位" /></label>
        {error && <div className="alert error">{error}</div>}
        <button className="button primary" type="button" onClick={save}><Save size={16} />保存新密码</button>
      </div>
    </section>
  );
}

function DeployPanel({ config, showToast, setError: setGlobalError }: { config: RuntimeConfig | null; showToast: (msg: string) => void; setError: (msg: string) => void }) {
  const [mode, setMode] = useState<DeployMode>("single");
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [code, setCode] = useState("");
  const [append, setAppend] = useState(false);
  const [visibility, setVisibility] = useState("public");
  const [category, setCategory] = useState("");
  const [tagsInput, setTagsInput] = useState("");
  const [categories, setCategories] = useState<MarketCategoryInfo[]>([]);
  const [password, setPassword] = useState("");
  const [content, setContent] = useState("");
  const [entry, setEntry] = useState("index.html");
  const [files, setFiles] = useState<DeployFile[]>([]);
  const [result, setResult] = useState<any>(null);
  const [deployError, setDeployError] = useState("");
  const [deployErrorDetail, setDeployErrorDetail] = useState<StructuredAPIErrorPayload | null>(null);
  const [busy, setBusy] = useState(false);
  const fileInput = useRef<HTMLInputElement | null>(null);
  const dirInput = useRef<HTMLInputElement | null>(null);

  const totalSize = mode === "single" ? textSize(content) : files.reduce((sum, file) => sum + file.size, 0);
  const setError = (message: string) => {
    setDeployError(message);
    setDeployErrorDetail(null);
    setGlobalError("");
  };
  const setValidationError = (message: string, errorCode = "INVALID_INPUT") => {
    setDeployError(message);
    setDeployErrorDetail({ errorCode, stage: "validate" });
    setGlobalError("");
  };

  useEffect(() => {
    api<MarketCategoriesResponse>("/api/market/categories")
      .then((data) => setCategories(data.categories || []))
      .catch(() => setCategories([]));
  }, []);

  async function readFiles(list: FileList | null) {
    if (!list?.length) return;
    if (mode === "single") {
      const file = list[0];
      if (isZipFile(file.name)) {
        setMode("multi");
        setEntry("");
        setFiles([{
          path: normalizeUploadedZipPath(file.name),
          content: "",
          contentBase64: toBase64(await file.arrayBuffer()),
          isText: false,
          size: file.size
        }]);
        return;
      }
      setEntry(file.name || "index.html");
      setContent(await file.text());
      return;
    }
    const loaded = await Promise.all(Array.from(list).map(async (file) => {
      const text = isTextFile(file.name);
      return {
        path: ((file as File & { webkitRelativePath?: string }).webkitRelativePath || file.name).replace(/\\/g, "/").replace(/^\.\//, ""),
        content: text ? await file.text() : "",
        contentBase64: text ? undefined : toBase64(await file.arrayBuffer()),
        isText: text,
        size: file.size
      };
    }));
    setFiles((prev) => {
      const next = new Map(prev.map((file) => [file.path, file]));
      loaded.forEach((file) => next.set(file.path, file));
      return Array.from(next.values());
    });
  }

  function payload() {
    const valid = files.filter((file) => file.path.trim());
    const mainEntry = preferredDeployEntryPath(valid, entry);
    const isSingleZip = valid.length === 1 && isZipFile(valid[0].path);
    const effectiveDescription = description.trim() || (append && code.trim() ? `更新 ${code.trim()} 的新版本` : "");
    const body: any = {
      title: title.trim() || undefined,
      description: effectiveDescription,
      enableCustomCode: append || !!code.trim(),
      customCode: append || code.trim() ? code.trim() : undefined,
      createVersion: append,
      visibility: append ? undefined : visibility,
      category: append ? undefined : (category || undefined),
      tags: append ? undefined : parseAdminTagInput(tagsInput),
      accessPassword: !append && password.trim() ? password.trim() : undefined
    };
    if (mode === "single") {
      body.filename = entry.trim() || "index.html";
      body.content = content;
    } else {
      if (isSingleZip) {
        const zipEntryHint = entry.trim();
        if (zipEntryHint && isDeployEntrypointFile(zipEntryHint)) body.filename = zipEntryHint;
      } else {
        body.filename = mainEntry || "index.html";
      }
      body.files = valid.map((file) => file.contentBase64
        ? { path: file.path.trim(), contentBase64: file.contentBase64 }
        : { path: file.path.trim(), content: file.content });
    }
    return body;
  }

  async function submit() {
    setError("");
    if (append && !code.trim()) return setError("更新现有发布必须填写已有 code");
    const effectiveDescription = description.trim() || (append && code.trim() ? `更新 ${code.trim()} 的新版本` : "");
    if (!effectiveDescription) return setValidationError("请填写一句话描述，说明这个应用是做什么的。", "INVALID_DESCRIPTION");
    if (mode === "single" && !content.trim()) return setError("请粘贴或上传 HTML 内容");
    if (mode === "multi" && !files.length) return setError("请上传多文件项目");
    const isSingleZipUpload = mode === "multi" && files.length === 1 && isZipFile(files[0].path);
    if (mode === "multi" && !files.some((file) => isDeployEntrypointFile(file.path)) && !isSingleZipUpload) {
      return setError("多文件项目需要至少包含一个 HTML/Markdown 入口文件，或上传单个 ZIP 包");
    }
    setBusy(true);
    try {
      const data = await api<any>("/api/deploy", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload())
      });
      setResult(data);
      showToast("部署成功");
    } catch (err) {
      const message = plainErrorMessage(err);
      setDeployError(message);
      setDeployErrorDetail(structuredAPIError(err));
      setGlobalError("");
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="two-col deploy-layout">
      <div className="panel">
        <div className="panel-head"><div><h2>{append ? "更新现有发布" : "发布新应用"}</h2><p>支持 HTML、Markdown、ZIP、多文件目录、公开/未公开、访问密码和追加版本。</p></div></div>
        <div className="segmented">
          <button className={mode === "single" ? "active" : ""} type="button" onClick={() => setMode("single")}>单文件</button>
          <button className={mode === "multi" ? "active" : ""} type="button" onClick={() => setMode("multi")}>多文件项目</button>
        </div>
        <label className="field"><span>标题</span><input value={title} onChange={(event) => setTitle(event.target.value)} placeholder="有意义的中文名字" /></label>
        <label className="field"><span>一句话描述 *</span><input value={description} onChange={(event) => setDescription(event.target.value)} maxLength={240} placeholder="这个应用是做什么的" /></label>
        <label className="field"><span>自定义 / 更新 code</span><input className="mono" value={code} onChange={(event) => setCode(event.target.value)} placeholder={append ? "填写已有 code，例如 my-landing" : "可选，例如 my-landing"} /></label>
        <label className="check-line"><input checked={append} type="checkbox" onChange={(event) => setAppend(event.target.checked)} />更新现有发布，追加为新版本</label>
        {append && <div className="hint-box">更新必须填写已有 <code>code</code>。它不会创建新链接，只给原站点追加版本；公开方式和访问密码沿用原设置。</div>}
        <div className="form-grid">
          <label className="field"><span>可见性</span><select value={visibility} disabled={append} onChange={(event) => setVisibility(event.target.value)}><option value="public">公开进创作市场</option><option value="unlisted">未公开，仅链接访问</option></select></label>
          <label className="field">
            <span>访问密码</span>
            <input
              type="password"
              value={password}
              disabled={append}
              onChange={(event) => setPassword(event.target.value)}
              placeholder="可选，至少 4 位"
              autoComplete="new-password"
              data-lpignore="true"
              name="pagepilot-admin-access-password"
            />
          </label>
        </div>
        <div className="form-grid">
          <label className="field">
            <span>作品分类（可选）</span>
            <select value={category} disabled={append} onChange={(event) => setCategory(event.target.value)}>
              <option value="">暂不分类</option>
              {categories.map((item) => <option value={item.slug} key={item.slug}>{item.label}</option>)}
            </select>
          </label>
          <label className="field">
            <span>作品标签</span>
            <input
              value={tagsInput}
              disabled={append}
              onChange={(event) => setTagsInput(event.target.value)}
              placeholder="官网, 看板, 活动页"
              autoComplete="off"
              data-lpignore="true"
              name="pagepilot-admin-tags"
            />
          </label>
        </div>

        {mode === "single" ? (
          <>
            <label className="field"><span>入口文件名</span><input value={entry} onChange={(event) => setEntry(event.target.value)} placeholder="index.html 或 README.md" /></label>
            <label className="upload-zone">
              <input type="file" accept=".html,.htm,.md,.markdown,.zip" onChange={(event) => void readFiles(event.target.files)} />
              <FileUp size={18} />上传 HTML / Markdown / ZIP
            </label>
            <textarea className="code-input" value={content} onChange={(event) => setContent(event.target.value)} placeholder="<!doctype html>..." />
          </>
        ) : (
          <>
            <input ref={fileInput} className="hidden" type="file" multiple onChange={(event) => void readFiles(event.target.files)} />
            <input ref={dirInput} className="hidden" type="file" multiple webkitdirectory="" onChange={(event) => void readFiles(event.target.files)} />
            <div className="upload-box" onDragOver={(event) => event.preventDefault()} onDrop={(event) => { event.preventDefault(); void readFiles(event.dataTransfer.files); }}>
              <strong>上传多文件静态站点</strong>
              <span>保留 CSS、JS、图片、字体等相对路径，优先使用 index.html 或 README.md 作为入口；单 ZIP 会交给服务端识别。</span>
              <div className="actions">
                <button className="button" type="button" onClick={() => fileInput.current?.click()}>选择多个文件</button>
                <button className="button" type="button" onClick={() => dirInput.current?.click()}>选择目录</button>
              </div>
            </div>
            <div className="file-list">
              {files.map((file) => (
                <div className="file-row" key={file.path}>
                  <code>{file.path}</code>
                  <span>{file.isText ? "text" : "bin"} · {formatSize(file.size)}</span>
                  <button className="icon-button danger" type="button" onClick={() => setFiles((prev) => prev.filter((item) => item.path !== file.path))}><Trash2 size={14} /></button>
                </div>
              ))}
              {!files.length && <div className="empty">还没有选择文件。</div>}
            </div>
          </>
        )}
        <div className="summary-line"><span>大小 {formatSize(totalSize)}</span><span>上限 {formatSize(config?.limits?.maxSiteTotalBytes)}</span></div>
        <button className="button primary full" type="button" disabled={busy} onClick={submit}><Upload size={16} />{busy ? "部署中..." : "立即部署"}</button>
        {deployError && <DeployErrorPanel message={deployError} error={deployErrorDetail} />}
      </div>
      <div className="panel">
        <div className="panel-head"><div><h2>结果与预览</h2><p>部署成功后可复制链接和进入版本管理。</p></div></div>
        {result ? (
          <div className="result-box">
            {(() => {
              const appURL = sameSiteURL(result.url);
              return (
                <>
            <InfoRow label="Code" value={result.code} />
            <InfoRow label="访问地址" value={appURL} copy />
            <InfoRow label="版本" value={`v${result.versionNumber || "-"}`} />
            <InfoRow label="大小" value={formatSize(result.size)} />
            <div className="actions">
              <a className="button primary" href={appURL} target="_blank" rel="noreferrer"><Eye size={16} />打开</a>
              <button className="button" type="button" onClick={() => navigator.clipboard.writeText(appURL)}><Copy size={16} />复制</button>
              <a className="button" href={`/deploy/${encodeURIComponent(result.id || result.code)}`} target="_blank" rel="noreferrer">详情</a>
            </div>
            <iframe title="部署预览" src={appURL} sandbox={PREVIEW_IFRAME_SANDBOX} />
                </>
              );
            })()}
          </div>
        ) : (
          <div className="empty tall">还没有部署结果。</div>
        )}
      </div>
    </section>
  );
}

function SitesPanel({ isAdmin, showToast, setError }: { isAdmin: boolean; showToast: (msg: string) => void; setError: (msg: string) => void }) {
  const [sites, setSites] = useState<SiteItem[]>([]);
  const [categories, setCategories] = useState<MarketCategoryInfo[]>([]);
  const [query, setQuery] = useState("");
  const [status, setStatus] = useState("");
  const [visibility, setVisibility] = useState("");
  const [category, setCategory] = useState("");
  const [tagFilter, setTagFilter] = useState("");
  const [kind, setKind] = useState("");
  const [ownerScope, setOwnerScope] = useState("");
  const [viewMode, setViewMode] = useState<SiteViewMode>("list");
  const [versions, setVersions] = useState<any[] | null>(null);
  const [versionCode, setVersionCode] = useState("");
  const [tagEditor, setTagEditor] = useState<{ site: SiteItem; value: string } | null>(null);
  const [detail, setDetail] = useState<SiteDetailResponse | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);

  const load = useCallback(async () => {
    try {
      const [siteData, categoryData] = await Promise.all([
        api<{ sites?: SiteItem[] }>("/api/admin/sites"),
        api<MarketCategoriesResponse>("/api/market/categories")
      ]);
      setSites(siteData.sites || []);
      setCategories(categoryData.categories || []);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }, [setError]);

  useEffect(() => { void load(); }, [load]);

  const allTags = useMemo(() => {
    const tags = new Set<string>();
    sites.forEach((site) => (site.tags || []).forEach((tag) => {
      const value = tag.trim();
      if (value) tags.add(value);
    }));
    return Array.from(tags).sort((a, b) => a.localeCompare(b, "zh-CN"));
  }, [sites]);

  const filtered = useMemo(() => sites.filter((site) => {
    const text = query.trim().toLowerCase();
    const tagText = (site.tags || []).join(" ").toLowerCase();
    const hit = !text
      || site.code.toLowerCase().includes(text)
      || (site.ownerUsername || "").toLowerCase().includes(text)
      || (site.ownerTokenId || "").toLowerCase().includes(text)
      || (site.filename || "").toLowerCase().includes(text)
      || tagText.includes(text);
    const statusHit = !status || site.status === status;
    const visibilityHit = !visibility || site.visibility === visibility;
    const categoryHit = !category || (site.category || "") === category;
    const tagHit = !tagFilter || (site.tags || []).some((tag) => tag.toLowerCase() === tagFilter.toLowerCase());
    const ownerHit = !ownerScope
      || (ownerScope === "registered" && String(site.ownerTokenId || "").startsWith("user:"))
      || (ownerScope === "anonymous" && String(site.ownerTokenId || "").startsWith("anon:"))
      || (ownerScope === "token" && site.ownerTokenId && !String(site.ownerTokenId).startsWith("user:") && !String(site.ownerTokenId).startsWith("anon:"));
    const kindHit = !kind
      || (kind === "protected" && site.accessProtected)
      || (kind === "featured" && site.isPinned)
      || (kind === "html" && /\.(html?|htm)$/i.test(site.filename || ""))
      || (kind === "md" && /\.(md|markdown)$/i.test(site.filename || ""));
    return hit && statusHit && visibilityHit && categoryHit && tagHit && ownerHit && kindHit;
  }), [category, kind, ownerScope, query, sites, status, tagFilter, visibility]);
  const hasActiveFilters = Boolean(query || status || visibility || ownerScope || category || tagFilter || kind);

  function clearFilters() {
    setQuery("");
    setStatus("");
    setVisibility("");
    setOwnerScope("");
    setCategory("");
    setTagFilter("");
    setKind("");
  }

  async function setPassword(site: SiteItem) {
    const next = window.prompt(site.accessProtected ? "留空并确认即可清除访问密码。" : "设置访问密码，至少 4 位。");
    if (next == null) return;
    await api(`/api/deploys/${encodeURIComponent(site.code)}/access`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ password: next })
    });
    showToast(next ? "访问密码已设置" : "访问密码已清除");
    await load();
  }

  async function toggleVisibility(site: SiteItem) {
    const visibility = site.visibility === "unlisted" ? "public" : "unlisted";
    await api(`/api/deploys/${encodeURIComponent(site.code)}/visibility`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ visibility })
    });
    showToast(visibility === "public" ? "已公开进创作市场" : "已设为未公开");
    await load();
  }

  async function togglePin(site: SiteItem) {
    await api(`/api/admin/sites/${encodeURIComponent(site.code)}/pin`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ pinned: !site.isPinned })
    });
    showToast(site.isPinned ? "已取消置顶" : "已置顶");
    await load();
  }

  async function updateReusePolicy(site: SiteItem, reusePolicy: ReusePolicyValue, sourceDownloadPolicy: ReusePolicyValue) {
    const data = await api<{ site: SiteItem }>(`/api/admin/sites/${encodeURIComponent(site.code)}/reuse-policy`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ reusePolicy, sourceDownloadPolicy })
    });
    showToast("源码下载和复用策略已更新");
    setSites((current) => current.map((item) => item.code === site.code ? { ...item, ...data.site } : item));
    await openSiteDetail(data.site || site);
    await load();
  }

  async function updateSecurityMode(site: SiteItem, securityMode: SecurityModeValue) {
    const data = await api<{ site: SiteItem }>(`/api/admin/sites/${encodeURIComponent(site.code)}/security-mode`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ securityMode })
    });
    showToast("运行安全模式已更新");
    setSites((current) => current.map((item) => item.code === site.code ? { ...item, ...data.site } : item));
    await openSiteDetail(data.site || site);
    await load();
  }

  async function updateCategory(site: SiteItem, nextCategory: string) {
    await api(`/api/admin/sites/${encodeURIComponent(site.code)}/category`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ category: nextCategory || null })
    });
    showToast("应用分类已更新");
    await load();
  }

  async function saveTags() {
    if (!tagEditor) return;
    await api(`/api/admin/sites/${encodeURIComponent(tagEditor.site.code)}/tags`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ tags: parseAdminTagInput(tagEditor.value) })
    });
    showToast("应用标签已更新");
    setTagEditor(null);
    await load();
  }

  async function deleteSite(site: SiteItem) {
    if (!window.confirm(`确认删除 ${site.code} 及其全部版本？`)) return;
    await api(`/api/admin/sites/${encodeURIComponent(site.code)}`, { method: "DELETE" });
    showToast("站点已删除");
    await load();
  }

  async function openVersions(site: SiteItem) {
    setVersionCode(site.code);
    const data = await api<any>(`/api/deploys/${encodeURIComponent(site.code)}/versions`);
    setVersions((data.versions || []).slice().sort((a: any, b: any) => Number(b.versionNumber) - Number(a.versionNumber)));
  }

  async function openSiteDetail(site: SiteItem) {
    setDetailLoading(true);
    try {
      const data = await api<SiteDetailResponse>(`/api/admin/sites/${encodeURIComponent(site.code)}`);
      setDetail(data);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setDetailLoading(false);
    }
  }

  async function versionAction(action: string, version: number, locked?: boolean) {
    if (!versionCode) return;
    if (action === "current") {
      await api(`/api/deploys/${encodeURIComponent(versionCode)}/current`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ versionNumber: version })
      });
    } else if (action === "lock") {
      await api(`/api/deploys/${encodeURIComponent(versionCode)}/versions/${version}/lock`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ locked: !locked })
      });
    } else if (action === "delete") {
      if (!window.confirm(`确认删除 v${version}？`)) return;
      await api(`/api/deploys/${encodeURIComponent(versionCode)}/versions/${version}`, { method: "DELETE" });
    }
    await openVersions({ code: versionCode });
    await load();
  }

  return (
    <section className="panel">
      <div className="panel-head">
        <div><h2>{isAdmin ? "全站应用" : "我的应用"}</h2><p>加密、未公开、删除、置顶和版本管理都在这里。</p></div>
        <button className="button" type="button" onClick={() => void load()}><RefreshCw size={16} />刷新</button>
      </div>
      <div className="toolbar site-toolbar">
        <div className="toolbar-summary">
          <strong>{filtered.length}</strong>
          <span>/ {sites.length} 个应用</span>
          {hasActiveFilters && <em>已启用筛选</em>}
        </div>
        <label className="search-box"><Search size={16} /><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索 code、owner、标签" /></label>
        <div className="filter-row">
          <select value={status} onChange={(event) => setStatus(event.target.value)}><option value="">全部状态</option><option value="active">运行中</option><option value="inactive">已下架</option></select>
          <select value={visibility} onChange={(event) => setVisibility(event.target.value)}><option value="">全部可见性</option><option value="public">公开</option><option value="unlisted">未公开</option></select>
          <select value={ownerScope} onChange={(event) => setOwnerScope(event.target.value)}><option value="">全部归属</option><option value="registered">注册用户</option><option value="anonymous">匿名发布</option><option value="token">Token/其他</option></select>
          <select value={category} onChange={(event) => setCategory(event.target.value)}><option value="">全部分类</option>{categories.map((item) => <option value={item.slug} key={item.slug}>{item.label}</option>)}</select>
          <select value={tagFilter} onChange={(event) => setTagFilter(event.target.value)}><option value="">全部标签</option>{allTags.map((tag) => <option value={tag} key={tag}>#{tag}</option>)}</select>
          <select value={kind} onChange={(event) => setKind(event.target.value)}><option value="">全部类型</option><option value="html">HTML</option><option value="md">Markdown</option><option value="protected">加密</option><option value="featured">精选</option></select>
        </div>
        <div className="toolbar-tail">
          {hasActiveFilters && <button className="button compact" type="button" onClick={clearFilters}>清空筛选</button>}
          <div className="segmented-control" role="group" aria-label="视图">
            <button className={viewMode === "list" ? "active" : ""} type="button" onClick={() => setViewMode("list")}>列表</button>
            <button className={viewMode === "render" ? "active" : ""} type="button" onClick={() => setViewMode("render")}>渲染</button>
          </div>
        </div>
      </div>
      {viewMode === "list" ? (
      <div className="table-wrap">
        <table>
          <thead><tr><th>Code</th><th>归属</th><th>分类 / 标签</th><th>状态</th><th>版本</th><th>数据</th><th>大小</th><th>修改</th><th>操作</th></tr></thead>
          <tbody>
            {filtered.map((site) => (
              <tr key={site.code}>
                <td><code>{site.code}</code></td>
                <td>{site.ownerUsername || site.ownerTokenId || "-"}</td>
                <td>
                  {isAdmin ? (
                    <select className="compact-select" value={site.category || ""} onChange={(event) => void updateCategory(site, event.target.value)}>
                      <option value="">未分类</option>
                      {categories.map((item) => <option value={item.slug} key={item.slug}>{item.label}</option>)}
                    </select>
                  ) : (
                    categories.find((item) => item.slug === site.category)?.label || site.category || "-"
                  )}
                  <div className="tag-row editable">
                    {(site.tags || []).slice(0, 4).map((tag) => <span key={tag}>#{tag}</span>)}
                    <button type="button" onClick={() => setTagEditor({ site, value: (site.tags || []).join(", ") })}>{site.tags?.length ? "编辑标签" : "添加标签"}</button>
                  </div>
                </td>
                <td><div className="badge-stack">{site.isPinned && <span className="badge amber">置顶</span>}{statusBadge(site.status || "active", site.accessProtected)}<span className="badge slate">{site.visibility === "unlisted" ? "未公开" : "公开"}</span></div></td>
                <td>v{site.currentVersion || "-"} · {site.versionCount || 0}</td>
                <td>{site.viewCount || 0} 访 · {site.likeCount || 0} 赞 · {site.favoriteCount || 0} 藏 · {site.reuseCount || 0} 复用</td>
                <td>{formatSize(site.totalSize)}</td>
                <td>{formatDate(site.lastVersionAt || site.createdAt)}</td>
                <td>
                  <div className="actions tight">
                    <a className="button compact" href={siteURL(site.code)} target="_blank" rel="noreferrer">打开</a>
                    <button className="button compact" type="button" onClick={() => void setPassword(site)}><Lock size={14} />加密</button>
                    <button className="button compact" type="button" onClick={() => void toggleVisibility(site)}>{site.visibility === "unlisted" ? "公开" : "未公开"}</button>
                    <button className="button compact" type="button" onClick={() => void openSiteDetail(site)}>详情</button>
                    <button className="button compact" type="button" onClick={() => void openVersions(site)}>版本</button>
                    {isAdmin && <button className="icon-button" type="button" title="置顶" onClick={() => void togglePin(site)}><Pin size={14} /></button>}
                    <button className="icon-button danger" type="button" title="删除" onClick={() => void deleteSite(site)}><Trash2 size={14} /></button>
                  </div>
                </td>
              </tr>
            ))}
            {!filtered.length && <tr><td colSpan={9}>暂无应用。</td></tr>}
          </tbody>
        </table>
      </div>
      ) : (
        <div className="site-render-grid">
          {filtered.map((site) => (
            <article className="site-render-card" key={site.code}>
              <div className="site-thumb"><iframe title={`${site.code} 预览`} src={`${siteURL(site.code)}?preview=1`} sandbox={PREVIEW_IFRAME_SANDBOX} /></div>
              <div className="site-render-body">
                <div><strong>{site.code}</strong><span>{site.ownerUsername || site.ownerTokenId || "-"}</span></div>
                <p>{categories.find((item) => item.slug === site.category)?.label || site.category || "未分类"} · {site.viewCount || 0} 访 · {site.likeCount || 0} 赞 · {site.favoriteCount || 0} 藏</p>
                <div className="tag-row editable">
                  {(site.tags || []).slice(0, 5).map((tag) => <span key={tag}>#{tag}</span>)}
                  <button type="button" onClick={() => setTagEditor({ site, value: (site.tags || []).join(", ") })}>{site.tags?.length ? "编辑标签" : "添加标签"}</button>
                </div>
                <select className="compact-select" value={site.category || ""} onChange={(event) => void updateCategory(site, event.target.value)}>
                  <option value="">未分类</option>
                  {categories.map((item) => <option value={item.slug} key={item.slug}>{item.label}</option>)}
                </select>
                <div className="actions tight">
                  <a className="button compact" href={siteURL(site.code)} target="_blank" rel="noreferrer">打开</a>
                  <button className="button compact" type="button" onClick={() => void openSiteDetail(site)}>详情</button>
                  <button className="button compact" type="button" onClick={() => void openVersions(site)}>版本</button>
                  {isAdmin && <button className="button compact" type="button" onClick={() => void togglePin(site)}>{site.isPinned ? "取消精选" : "精选"}</button>}
                  <button className="button compact danger" type="button" onClick={() => void deleteSite(site)}><Trash2 size={14} />删除</button>
                </div>
              </div>
            </article>
          ))}
          {!filtered.length && <div className="empty tall">暂无应用。</div>}
        </div>
      )}
      {(detail || detailLoading) && (
        <Modal title={`站点详情 ${detail?.site?.code || ""}`} onClose={() => setDetail(null)}>
          {detailLoading && !detail ? (
            <div className="empty">正在加载站点详情...</div>
          ) : detail ? (
            <SiteDetailModal detail={detail} isAdmin={isAdmin} onSavePolicy={updateReusePolicy} onSaveSecurityMode={updateSecurityMode} />
          ) : null}
        </Modal>
      )}
      {versions && (
        <Modal title={`版本管理 ${versionCode}`} onClose={() => setVersions(null)}>
          <div className="version-list">
            {versions.map((version) => (
              <div className="version-card" key={version.versionNumber}>
                <div><strong>v{version.versionNumber}</strong><span>{version.isCurrent && <span className="badge green">当前</span>} {version.isLocked && <span className="badge amber">锁定</span>} {statusBadge(version.status || "active")}</span></div>
                <p>{formatDate(version.createdAt)} · {formatSize(version.fileSize || version.size)}</p>
                <div className="actions">
                  <a className="button compact" href={`/agent/${encodeURIComponent(versionCode)}/versions/${version.versionNumber}/`} target="_blank" rel="noreferrer">预览</a>
                  {!version.isCurrent && <button className="button compact" type="button" onClick={() => void versionAction("current", version.versionNumber)}>设为当前</button>}
                  <button className="button compact" type="button" onClick={() => void versionAction("lock", version.versionNumber, version.isLocked)}>{version.isLocked ? "解锁" : "锁定"}</button>
                  {!version.isLocked && <button className="button compact danger" type="button" onClick={() => void versionAction("delete", version.versionNumber)}>删除</button>}
                </div>
              </div>
            ))}
          </div>
        </Modal>
      )}
      {tagEditor && (
        <Modal title={`编辑标签 ${tagEditor.site.code}`} onClose={() => setTagEditor(null)}>
          <div className="form-stack">
            <label>
              <span>标签</span>
              <input
                value={tagEditor.value}
                onChange={(event) => setTagEditor({ ...tagEditor, value: event.target.value })}
                placeholder="官网, 看板, 活动页"
              />
            </label>
            <p className="muted">最多 6 个标签，用逗号、空格或分号分隔。标签会用于创作市场搜索、后台筛选和作品卡片展示。</p>
            <div className="actions right">
              <button className="button" type="button" onClick={() => setTagEditor(null)}>取消</button>
              <button className="button primary" type="button" onClick={() => void saveTags()}>保存标签</button>
            </div>
          </div>
        </Modal>
      )}
    </section>
  );
}

function normalizeReusePolicyValue(value?: string): ReusePolicyValue {
  return value === "allow" || value === "deny" ? value : "auto";
}

function normalizeSecurityModeValue(value?: string): SecurityModeValue {
  return value === "strict" || value === "compatible" || value === "trusted" ? value : "auto";
}

function SiteDetailModal({
  detail,
  isAdmin,
  onSavePolicy,
  onSaveSecurityMode
}: {
  detail: SiteDetailResponse;
  isAdmin: boolean;
  onSavePolicy: (site: SiteItem, reusePolicy: ReusePolicyValue, sourceDownloadPolicy: ReusePolicyValue) => Promise<void>;
  onSaveSecurityMode: (site: SiteItem, securityMode: SecurityModeValue) => Promise<void>;
}) {
  const files = detail.files || [];
  const tree = detail.bundle?.tree || files;
  const mcpText = detail.reuse?.mcp ? JSON.stringify(detail.reuse.mcp, null, 2) : "";
  const [reusePolicy, setReusePolicy] = useState<ReusePolicyValue>(normalizeReusePolicyValue(detail.site.reusePolicy));
  const [sourceDownloadPolicy, setSourceDownloadPolicy] = useState<ReusePolicyValue>(normalizeReusePolicyValue(detail.site.sourceDownloadPolicy));
  const [securityMode, setSecurityMode] = useState<SecurityModeValue>(normalizeSecurityModeValue(detail.site.securityMode));
  const [savingPolicy, setSavingPolicy] = useState(false);
  const [savingSecurity, setSavingSecurity] = useState(false);
  const savedReusePolicy = normalizeReusePolicyValue(detail.site.reusePolicy);
  const savedSourceDownloadPolicy = normalizeReusePolicyValue(detail.site.sourceDownloadPolicy);
  const savedSecurityMode = normalizeSecurityModeValue(detail.site.securityMode);
  const policyChanged = reusePolicy !== savedReusePolicy || sourceDownloadPolicy !== savedSourceDownloadPolicy;
  const securityChanged = securityMode !== savedSecurityMode;

  useEffect(() => {
    setReusePolicy(normalizeReusePolicyValue(detail.site.reusePolicy));
    setSourceDownloadPolicy(normalizeReusePolicyValue(detail.site.sourceDownloadPolicy));
    setSecurityMode(normalizeSecurityModeValue(detail.site.securityMode));
  }, [detail.site.code, detail.site.reusePolicy, detail.site.sourceDownloadPolicy, detail.site.securityMode]);

  const copy = async (value?: string) => {
    if (!value) return;
    await navigator.clipboard.writeText(value);
  };
  const savePolicy = async () => {
    setSavingPolicy(true);
    try {
      await onSavePolicy(detail.site, reusePolicy, sourceDownloadPolicy);
    } finally {
      setSavingPolicy(false);
    }
  };
  const saveSecurityMode = async () => {
    setSavingSecurity(true);
    try {
      await onSaveSecurityMode(detail.site, securityMode);
    } finally {
      setSavingSecurity(false);
    }
  };
  return (
    <div className="site-detail-modal">
      <div className="detail-info-grid admin-detail-grid">
        <span><em>Code</em><strong>{detail.site.code}</strong></span>
        <span><em>归属</em><strong>{detail.site.ownerUsername || detail.site.ownerTokenId || "-"}</strong></span>
        <span><em>状态</em><strong>{detail.site.status || "active"} · {detail.site.visibility || "unlisted"}</strong></span>
        <span><em>访问</em><strong>{detail.site.accessProtected ? "已加密" : "未加密"}</strong></span>
        <span><em>复用</em><strong>{detail.site.reuseCount || 0} 次</strong></span>
        <span><em>来源</em><strong>{detail.site.templateSourceCode ? `${detail.site.templateSourceCode} v${detail.site.templateSourceVersion || "-"}` : "原始发布"}</strong></span>
      </div>
      {isAdmin && (
        <section className="admin-detail-section policy-editor">
          <div className="detail-info-head"><ShieldCheck size={15} /><strong>源码下载与模板复用</strong></div>
          <p className="muted">默认自动策略：公开且未加密的作品可下载和复用；加密、不公开或下架作品默认禁止。加密作品即使策略为允许，也不会提供源码下载，需先清除访问密码。</p>
          <div className="policy-editor-grid">
            <label>
              <span>源码下载</span>
              <select value={sourceDownloadPolicy} onChange={(event) => setSourceDownloadPolicy(event.target.value as ReusePolicyValue)}>
                <option value="auto">自动：加密默认禁止</option>
                <option value="allow">显式允许</option>
                <option value="deny">显式禁止</option>
              </select>
            </label>
            <label>
              <span>模板复用</span>
              <select value={reusePolicy} onChange={(event) => setReusePolicy(event.target.value as ReusePolicyValue)}>
                <option value="auto">自动：跟随源码下载</option>
                <option value="allow">显式允许</option>
                <option value="deny">显式禁止</option>
              </select>
            </label>
          </div>
          <div className="actions right">
            <button className="button primary compact" type="button" disabled={!policyChanged || savingPolicy} onClick={() => void savePolicy()}>
              <Save size={13} />{savingPolicy ? "保存中" : "保存策略"}
            </button>
          </div>
        </section>
      )}
      {isAdmin && (
        <section className="admin-detail-section policy-editor">
          <div className="detail-info-head"><ShieldCheck size={15} /><strong>运行安全模式</strong></div>
          <p className="muted">自动模式使用发布包识别结果；严格更安全，兼容适合需要更多浏览器能力的站点，受信任仅用于已审查内容。</p>
          <div className="policy-editor-grid">
            <label>
              <span>站点策略</span>
              <select value={securityMode} onChange={(event) => setSecurityMode(event.target.value as SecurityModeValue)}>
                <option value="auto">自动：跟随 Bundle 识别</option>
                <option value="strict">严格：优先隔离</option>
                <option value="compatible">兼容：放宽运行能力</option>
                <option value="trusted">受信任：仅限可信内容</option>
              </select>
            </label>
            <span className="policy-hint">
              生效：{securityModeLabel(detail.bundle?.effectiveSecurityMode || detail.bundle?.securityMode)}
            </span>
          </div>
          <div className="actions right">
            <button className="button primary compact" type="button" disabled={!securityChanged || savingSecurity} onClick={() => void saveSecurityMode()}>
              <Save size={13} />{savingSecurity ? "保存中" : "保存安全模式"}
            </button>
          </div>
        </section>
      )}
      {detail.bundle && (
        <section className="admin-detail-section">
          <div className="detail-info-head"><FileArchive size={15} /><strong>Bundle / 安全</strong></div>
          <div className="detail-info-grid admin-detail-grid">
            <span><em>类型</em><strong>{detail.bundle.kindLabel || detail.bundle.kind || "-"}</strong></span>
            <span><em>入口</em><strong>{detail.bundle.mainEntry || detail.site.filename || "-"}</strong></span>
            {detail.bundle.root && <span><em>根目录</em><strong>{detail.bundle.root}</strong></span>}
            <span><em>Bundle 识别</em><strong>{securityModeLabel(detail.bundle.securityMode)}</strong></span>
            <span><em>站点策略</em><strong>{securityModeLabel(detail.bundle.siteSecurityMode || detail.site.securityMode)}</strong></span>
            <span><em>生效模式</em><strong>{securityModeLabel(detail.bundle.effectiveSecurityMode || detail.bundle.securityMode)}</strong></span>
            <span><em>文件</em><strong>{detail.bundle.fileCount || files.length || 0} 个 · {formatSize(detail.bundle.totalSize || detail.site.totalSize || 0)}</strong></span>
          </div>
          {detail.bundle.entryNote && <p className="muted">{detail.bundle.entryNote}</p>}
        </section>
      )}
      {!!tree.length && (
        <section className="admin-detail-section">
          <div className="detail-info-head"><FileText size={15} /><strong>完整文件树</strong></div>
          <FileTreeExplorer
            files={tree}
            totalSize={detail.bundle?.totalSize || detail.site.totalSize}
            onCopy={copy}
          />
        </section>
      )}
      {detail.reuse && (
        <section className="admin-detail-section">
          <div className="detail-info-head"><Workflow size={15} /><strong>复用参数</strong></div>
          {detail.site.templateSourceCode && (
            <p className="muted">这个站点基于 {detail.site.templateSourceCode} v{detail.site.templateSourceVersion || "-"} 二次创作。</p>
          )}
          {detail.reuse.policyNote && <p className="muted">{detail.reuse.policyNote}</p>}
          <div className="detail-copy-stack">
            {detail.reuse.agentPrompt && <button className="button compact" type="button" onClick={() => void copy(detail.reuse?.agentPrompt)}><Copy size={13} />复制 Agent 提示词</button>}
            {detail.reuse.cli && <button className="button compact" type="button" onClick={() => void copy(detail.reuse?.cli)}><Code2 size={13} />复制 CLI 命令</button>}
            {mcpText && <button className="button compact" type="button" onClick={() => void copy(mcpText)}><Copy size={13} />复制 MCP 参数</button>}
            {detail.reuse.downloadUrl && <a className="button compact" href={detail.reuse.downloadUrl}><Download size={13} />下载源文件</a>}
            {!detail.reuse.allowReuse && <span className="muted-line">当前策略不开放复用。</span>}
          </div>
        </section>
      )}
      <section className="admin-detail-section">
        <div className="detail-info-head"><ScrollText size={15} /><strong>版本</strong></div>
        <div className="version-list compact">
          {(detail.versions || []).slice(0, 8).map((version) => (
            <div className="version-card compact" key={version.versionNumber}>
              <div><strong>v{version.versionNumber}</strong><span>{version.isCurrent && <span className="badge green">当前</span>} {version.isLocked && <span className="badge amber">锁定</span>}</span></div>
              <p>{formatDate(version.createdAt)} · {formatSize(version.size || version.fileSize || 0)}</p>
            </div>
          ))}
          {!detail.versions?.length && <span className="muted-line">暂无版本记录</span>}
        </div>
      </section>
      {isAdmin && <SiteAuditTrail siteCode={detail.site.code} />}
    </div>
  );
}

function SiteAuditTrail({ siteCode }: { siteCode: string }) {
  const [logs, setLogs] = useState<AuditLogItem[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [localError, setLocalError] = useState("");

  const load = useCallback(async () => {
    if (!siteCode) return;
    setLoading(true);
    setLocalError("");
    try {
      const params = new URLSearchParams({
        siteCode,
        page: "1",
        pageSize: "6"
      });
      const data = await api<AuditLogResponse>(`/api/admin/audit-logs?${params.toString()}`);
      setLogs(data.logs || []);
      setTotal(Number(data.total || 0));
    } catch (err) {
      setLocalError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, [siteCode]);

  useEffect(() => { void load(); }, [load]);

  return (
    <section className="admin-detail-section site-audit-trail">
      <div className="detail-section-head">
        <div className="detail-info-head"><ScrollText size={15} /><strong>最近审计</strong></div>
        <button className="button compact" type="button" onClick={() => void load()} disabled={loading}>
          <RefreshCw size={13} />{loading ? "刷新中" : "刷新"}
        </button>
      </div>
      <p className="muted">按当前站点 code 过滤最近操作，便于快速追踪发布、加密、可见性、复用和安全策略变化。</p>
      {localError && <div className="alert error compact-alert">{localError}</div>}
      <div className="site-audit-list">
        {logs.map((log) => (
          <article className="site-audit-item" key={log.id}>
            <div className="site-audit-main">
              <span className={`badge ${log.result === "success" ? "green" : "amber"}`}>{auditResultLabel(log.result)}</span>
              <strong>{auditActionLabel(log.action)}</strong>
              <code>{log.action || "-"}</code>
              <time>{formatDate(log.createdAt)}</time>
            </div>
            <div className="site-audit-meta">
              <span>{actorRoleLabel(log.actorRole)} · {log.actorId || "-"}</span>
              <span>{log.targetType || "-"} · {log.targetId || "-"}</span>
              <span>{log.ip || "-"}</span>
            </div>
            <pre className="site-audit-detail">{formatAuditDetail(log.detail)}</pre>
          </article>
        ))}
        {!logs.length && !loading && <span className="muted-line">暂无这个站点的审计记录。</span>}
        {loading && !logs.length && <span className="muted-line">正在加载审计记录...</span>}
      </div>
      {!!total && <span className="muted-line">共匹配 {total} 条，当前显示最近 {Math.min(logs.length, 6)} 条；完整筛选请进入审计日志页。</span>}
    </section>
  );
}

function normalizeCategorySlug(value: string) {
  return value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9-]+/g, "-")
    .replace(/-+/g, "-")
    .replace(/^-|-$/g, "");
}

function CategoriesPanel({ showToast, setError }: { showToast: (msg: string) => void; setError: (msg: string) => void }) {
  const [categories, setCategories] = useState<MarketCategoryInfo[]>([]);
  const [query, setQuery] = useState("");
  const [draft, setDraft] = useState<{ index: number | null; item: MarketCategoryInfo } | null>(null);
  const [saving, setSaving] = useState(false);

  const load = useCallback(async () => {
    try {
      const data = await api<MarketCategoriesResponse>("/api/market/categories");
      setCategories(data.categories || []);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }, [setError]);

  useEffect(() => { void load(); }, [load]);

  const filtered = useMemo(() => {
    const text = query.trim().toLowerCase();
    if (!text) return categories;
    return categories.filter((item) => item.slug.includes(text) || item.label.toLowerCase().includes(text) || (item.note || "").toLowerCase().includes(text));
  }, [categories, query]);

  function openEditor(item?: MarketCategoryInfo, index: number | null = null) {
    setDraft({ index, item: item ? { ...item } : { slug: "", label: "", note: "" } });
  }

  function applyDraft() {
    if (!draft) return;
    const next = {
      slug: normalizeCategorySlug(draft.item.slug),
      label: draft.item.label.trim(),
      note: draft.item.note?.trim() || undefined
    };
    if (!next.label) return setError("请填写分类名称。");
    if (!next.slug) return setError("请填写分类标识，格式如 landing-page。");
    const duplicate = categories.some((item, index) => index !== draft.index && item.slug === next.slug);
    if (duplicate) return setError("分类标识不能重复。");
    setCategories((current) => draft.index == null ? [...current, next] : current.map((item, index) => index === draft.index ? next : item));
    setDraft(null);
  }

  async function saveAll() {
    setSaving(true);
    try {
      await api("/api/admin/market/categories", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ categories })
      });
      showToast("应用分类已保存");
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSaving(false);
    }
  }

  function removeAt(index: number) {
    if (!window.confirm("确认删除这个分类？已有应用不会被删除，只会显示为原始分类标识。")) return;
    setCategories((current) => current.filter((_, itemIndex) => itemIndex !== index));
  }

  return (
    <section className="panel">
      <div className="panel-head">
        <div><h2>应用分类</h2><p>维护创作市场的二级分类。发布端、Skill、MCP 会读取这组分类供 Agent 选择。</p></div>
        <div className="actions">
          <button className="button" type="button" onClick={() => openEditor()}><FileText size={16} />新增分类</button>
          <button className="button primary" type="button" disabled={saving} onClick={() => void saveAll()}><Save size={16} />{saving ? "保存中..." : "保存分类"}</button>
        </div>
      </div>
      <div className="toolbar compact-toolbar">
        <label className="search-box"><Search size={16} /><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索分类名称、标识或备注" /></label>
        <span className="summary-chip">{categories.length} 个分类</span>
      </div>
      <div className="category-list">
        {filtered.map((item) => {
          const index = categories.findIndex((categoryItem) => categoryItem.slug === item.slug);
          return (
            <div className="category-row" key={item.slug}>
              <div>
                <strong>{item.label}</strong>
                <code>{item.slug}</code>
              </div>
              <p>{item.note || "暂无备注"}</p>
              <div className="actions tight">
                <button className="button compact" type="button" onClick={() => openEditor(item, index)}>编辑</button>
                <button className="icon-button danger" type="button" title="删除" onClick={() => removeAt(index)}><Trash2 size={14} /></button>
              </div>
            </div>
          );
        })}
        {!filtered.length && <div className="empty">没有匹配的分类。</div>}
      </div>
      {draft && (
        <Modal title={draft.index == null ? "新增分类" : "编辑分类"} onClose={() => setDraft(null)}>
          <div className="form-grid">
            <label><span>分类名称</span><input value={draft.item.label} onChange={(event) => setDraft({ ...draft, item: { ...draft.item, label: event.target.value } })} placeholder="例如 数据看板" /></label>
            <label><span>分类标识</span><input value={draft.item.slug} onChange={(event) => setDraft({ ...draft, item: { ...draft.item, slug: normalizeCategorySlug(event.target.value) } })} placeholder="例如 dashboard" /></label>
            <label className="full"><span>备注</span><textarea value={draft.item.note || ""} onChange={(event) => setDraft({ ...draft, item: { ...draft.item, note: event.target.value } })} placeholder="给管理员和 Agent 的分类说明" /></label>
          </div>
          <div className="actions modal-actions">
            <button className="button" type="button" onClick={() => setDraft(null)}>取消</button>
            <button className="button primary" type="button" onClick={applyDraft}>应用</button>
          </div>
        </Modal>
      )}
    </section>
  );
}

function ScreensPanel({ showToast, setError }: { isAdmin: boolean; showToast: (msg: string) => void; setError: (msg: string) => void }) {
  const [screens, setScreens] = useState<ScreenItem[]>([]);
  const [sites, setSites] = useState<SiteItem[]>([]);
  const [market, setMarket] = useState<MarketplaceItem[]>([]);
  const [pairingCode, setPairingCode] = useState("");
  const [screenName, setScreenName] = useState("");
  const [pickScreen, setPickScreen] = useState<ScreenItem | null>(null);
  const [pickTab, setPickTab] = useState<ScreenPickTab>("mine");
  const [screenshotDialog, setScreenshotDialog] = useState<ScreenshotDialog | null>(null);
  const screenshotSeq = useRef(0);

  const load = useCallback(async () => {
    try {
      const [screenData, siteData, marketData] = await Promise.all([
        api<{ screens?: ScreenItem[] }>("/api/screens"),
        api<{ sites?: SiteItem[] }>("/api/admin/sites"),
        api<{ deploys?: MarketplaceItem[] }>("/api/deploys?pageSize=100&sort=newest")
      ]);
      setScreens(screenData.screens || []);
      setSites(siteData.sites || []);
      setMarket(marketData.deploys || []);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }, [setError]);

  useEffect(() => { void load(); }, [load]);

  async function bind() {
    if (!pairingCode.trim()) return setError("请输入配对码");
    try {
      await api("/api/screens/bind", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ pairingCode: pairingCode.trim(), name: screenName.trim() || undefined })
      });
      setPairingCode("");
      setScreenName("");
      showToast("屏幕已绑定");
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  async function publish(screen: ScreenItem, code: string) {
    try {
      await api(`/api/screens/${encodeURIComponent(screen.id)}/publish`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ code })
      });
      showToast("投放已下发");
      setPickScreen(null);
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  function closeScreenshotDialog() {
    screenshotSeq.current += 1;
    setScreenshotDialog((current) => {
      if (current?.imageUrl) URL.revokeObjectURL(current.imageUrl);
      return null;
    });
  }

  async function fetchScreenshotBlob(screen: ScreenItem, after?: string) {
    const query = new URLSearchParams({ t: String(Date.now()) });
    if (after) query.set("after", after);
    const res = await fetch(`/api/screens/${encodeURIComponent(screen.id)}/screenshot?${query.toString()}`, {
      headers: authHeaders(),
      cache: "no-store"
    });
    if (!res.ok) {
      const body = await res.json().catch(() => ({}));
      const err = new APIError(body.detail || body.errorCode || `HTTP ${res.status}`);
      err.status = res.status;
      err.body = body;
      throw err;
    }
    return res.blob();
  }

  async function waitForScreenshot(screen: ScreenItem, after: string, seq: number) {
    const deadline = Date.now() + 35_000;
    while (Date.now() < deadline) {
      if (seq !== screenshotSeq.current) return;
      try {
        const blob = await fetchScreenshotBlob(screen, after);
        if (seq !== screenshotSeq.current) return;
        const imageUrl = URL.createObjectURL(blob);
        setScreenshotDialog((current) => {
          if (current?.imageUrl) URL.revokeObjectURL(current.imageUrl);
          return {
            screenId: screen.id,
            screenName: screen.name || screen.id,
            status: "ready",
            message: "截图已返回",
            requestedAt: after,
            imageUrl
          };
        });
        await load();
        return;
      } catch (err) {
        if (err instanceof APIError && err.status && err.status !== 404) {
          throw err;
        }
        await sleep(900);
      }
    }
    throw new Error("等待截图超时，请确认屏幕在线后重试");
  }

  async function requestScreenshot(screen: ScreenItem) {
    const screenName = screen.name || screen.id;
    const seq = screenshotSeq.current + 1;
    screenshotSeq.current = seq;
    setScreenshotDialog((current) => {
      if (current?.imageUrl) URL.revokeObjectURL(current.imageUrl);
      return {
        screenId: screen.id,
        screenName,
        status: "waiting",
        message: "正在下发截图指令，等待屏幕返回图片..."
      };
    });
    try {
      const data = await api<ScreenScreenshotResponse>(`/api/screens/${encodeURIComponent(screen.id)}/screenshot`, { method: "POST" });
      const requestedAt = data.screenshot?.requestedAt || new Date().toISOString();
      setScreenshotDialog((current) => current ? { ...current, requestedAt, message: "指令已送达，正在等待屏幕截图..." } : current);
      await waitForScreenshot(screen, requestedAt, seq);
    } catch (err) {
      setScreenshotDialog((current) => current ? {
        ...current,
        status: "error",
        message: err instanceof Error ? err.message : String(err)
      } : current);
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  async function command(screen: ScreenItem, type: string) {
    try {
      if (type === "screenshot") {
        await requestScreenshot(screen);
      } else {
        await api(`/api/screens/${encodeURIComponent(screen.id)}/command`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ type })
        });
        showToast(`指令已下发：${type}`);
      }
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  async function viewScreenshot(screen: ScreenItem) {
    try {
      const blob = await fetchScreenshotBlob(screen);
      const imageUrl = URL.createObjectURL(blob);
      setScreenshotDialog((current) => {
        if (current?.imageUrl) URL.revokeObjectURL(current.imageUrl);
        return {
          screenId: screen.id,
          screenName: screen.name || screen.id,
          status: "ready",
          message: "最近一次截图",
          imageUrl
        };
      });
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  async function unbind(screen: ScreenItem) {
    if (!window.confirm(`确认解绑 ${screen.name || screen.id}？`)) return;
    try {
      await api(`/api/screens/${encodeURIComponent(screen.id)}`, { method: "DELETE" });
      showToast("屏幕已解绑");
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  return (
    <section className="page-grid">
      <div className="two-col">
        <div className="panel">
          <div className="panel-head"><div><h2>绑定屏幕</h2><p>屏幕 APP 首次启动后显示 6 位配对码，5 分钟内有效。</p></div></div>
          <label className="field"><span>配对码</span><input className="mono" value={pairingCode} onChange={(event) => setPairingCode(event.target.value)} placeholder="123456" /></label>
          <label className="field"><span>屏幕名称</span><input value={screenName} onChange={(event) => setScreenName(event.target.value)} placeholder="大厅屏 / 门店一号屏" /></label>
          <button className="button primary" type="button" onClick={() => void bind()}>绑定屏幕</button>
        </div>
        <div className="panel">
          <div className="panel-head"><div><h2>投放规则</h2><p>后台不会让用户手写 code 投屏，统一从自己的站点或创作市场选择。</p></div></div>
          <InfoRow label="权限" value="仅注册用户可投屏" />
          <InfoRow label="内容" value="投放 App URL 和播放 manifest" />
          <InfoRow label="远程指令" value="刷新、截图、休眠、唤醒、软关机" />
        </div>
      </div>
      <div className="panel">
        <div className="panel-head"><div><h2>硬件屏幕</h2><p>查看在线状态、设备信息和当前播放内容。</p></div><button className="button" type="button" onClick={() => void load()}><RefreshCw size={16} />刷新</button></div>
        <div className="screen-grid">
          {screens.map((screen) => (
            <article className="screen-card" key={screen.id}>
              <div className="screen-head"><strong>{screen.name || screen.id}</strong>{statusBadge(screen.status || "unknown")}</div>
              <InfoRow label="当前应用" value={screen.currentSiteCode || "未投放"} />
              <InfoRow label="最后在线" value={formatDate(screen.lastSeenAt)} />
              <DeviceInfoBlock screen={screen} />
              <div className="actions">
                <button className="button compact primary" type="button" onClick={() => setPickScreen(screen)}>投放</button>
                <button className="button compact" type="button" onClick={() => void command(screen, "refresh")}>刷新</button>
                <button className="button compact" type="button" onClick={() => void command(screen, "screenshot")}>截图</button>
                <button className="button compact" type="button" onClick={() => void command(screen, "sleep")}>休眠</button>
                <button className="button compact" type="button" onClick={() => void command(screen, "wake")}>唤醒</button>
                <button className="button compact" type="button" onClick={() => void command(screen, "shutdown")}>软关机</button>
                <button className="button compact" type="button" onClick={() => void viewScreenshot(screen)}>查看截图</button>
                <button className="button compact danger" type="button" onClick={() => void unbind(screen)}>解绑</button>
              </div>
            </article>
          ))}
          {!screens.length && <div className="empty">还没有绑定屏幕。</div>}
        </div>
      </div>
      {pickScreen && (
        <Modal title={`投放到 ${pickScreen.name || pickScreen.id}`} onClose={() => setPickScreen(null)}>
          <div className="segmented">
            <button className={pickTab === "mine" ? "active" : ""} type="button" onClick={() => setPickTab("mine")}>我的站点</button>
            <button className={pickTab === "market" ? "active" : ""} type="button" onClick={() => setPickTab("market")}>创作市场</button>
          </div>
          <div className="choice-list">
            {(pickTab === "mine" ? sites : market).map((site) => (
              <button className="choice-card" type="button" key={site.code} onClick={() => void publish(pickScreen, site.code)}>
                <strong>{site.code}</strong>
                <span>{"title" in site ? site.title || site.description || "创作市场作品" : `${site.versionCount || 0} 个版本`}</span>
              </button>
            ))}
          </div>
        </Modal>
      )}
      {screenshotDialog && (
        <Modal title={`屏幕截图 · ${screenshotDialog.screenName}`} onClose={closeScreenshotDialog}>
          {screenshotDialog.status === "waiting" && (
            <div className="screenshot-wait">
              <span className="spinner" aria-hidden="true" />
              <strong>{screenshotDialog.message}</strong>
              <p>{screenshotDialog.requestedAt ? `请求时间：${formatDate(screenshotDialog.requestedAt)}` : "窗口会在图片返回后自动显示。"}</p>
            </div>
          )}
          {screenshotDialog.status === "ready" && screenshotDialog.imageUrl && (
            <>
              <div className="screenshot-meta">{screenshotDialog.message}</div>
              <img className="screenshot" src={screenshotDialog.imageUrl} alt="屏幕截图" />
            </>
          )}
          {screenshotDialog.status === "error" && (
            <div className="alert error">{screenshotDialog.message}</div>
          )}
        </Modal>
      )}
    </section>
  );
}

function TokensPanel({ isAdmin, showToast, setError }: { isAdmin: boolean; showToast: (msg: string) => void; setError: (msg: string) => void }) {
  const [tokens, setTokens] = useState<TokenItem[]>([]);
  const [users, setUsers] = useState<UserItem[]>([]);
  const [label, setLabel] = useState("");
  const [ttl, setTtl] = useState("");
  const [owner, setOwner] = useState("");
  const [adminToken, setAdminToken] = useState(false);
  const [created, setCreated] = useState("");

  const load = useCallback(async () => {
    try {
      const data = await api<{ tokens?: TokenItem[] }>("/api/tokens");
      setTokens(data.tokens || []);
      if (isAdmin) {
        const userData = await api<{ users?: UserItem[] }>("/api/admin/users");
        setUsers(userData.users || []);
        if (!owner && userData.users?.[0]) setOwner(userData.users[0].id);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }, [isAdmin, owner, setError]);

  useEffect(() => { void load(); }, [load]);

  async function create() {
    if (!label.trim()) return setError("请填写 Token 标签");
    const body: any = { label: label.trim(), isAdmin: isAdmin ? adminToken : false };
    if (isAdmin && owner) body.ownerUserId = owner;
    if (ttl) body.ttlSeconds = Number(ttl);
    const data = await api<any>("/api/token", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body)
    });
    setCreated(data.token || "");
    setLabel("");
    setAdminToken(false);
    showToast("Token 已创建，明文只显示一次");
    await load();
  }

  async function revoke(token: TokenItem) {
    if (!window.confirm(`确认吊销 ${token.label || token.id}？`)) return;
    await api(`/api/tokens/${encodeURIComponent(token.id)}`, { method: "DELETE" });
    showToast("Token 已吊销");
    await load();
  }

  return (
    <section className="two-col">
      <div className="panel">
        <div className="panel-head"><div><h2>创建 Token</h2><p>默认永久；选择有效期后为临时 Token。</p></div></div>
        <label className="field"><span>标签</span><input value={label} onChange={(event) => setLabel(event.target.value)} placeholder="ci-bot / screen-agent" /></label>
        {isAdmin && <label className="field"><span>归属用户</span><select value={owner} onChange={(event) => setOwner(event.target.value)}>{users.map((user) => <option value={user.id} key={user.id}>{user.username}{user.isAdmin ? " · admin" : ""}</option>)}</select></label>}
        <label className="field"><span>有效期</span><select value={ttl} onChange={(event) => setTtl(event.target.value)}><option value="">永久</option><option value="1800">30 分钟</option><option value="86400">1 天</option><option value="604800">7 天</option><option value="2592000">30 天</option></select></label>
        {isAdmin && <label className="check-line"><input type="checkbox" checked={adminToken} onChange={(event) => setAdminToken(event.target.checked)} />管理员 Token</label>}
        <button className="button primary" type="button" onClick={() => void create()}><KeyRound size={16} />创建 Token</button>
        {created && <div className="token-created"><strong>Token 明文</strong><code>{created}</code><button className="button compact" type="button" onClick={() => navigator.clipboard.writeText(created)}><Copy size={14} />复制</button></div>}
      </div>
      <div className="panel">
        <div className="panel-head"><div><h2>Token 列表</h2><p>吊销后 Agent 需要换 Token。</p></div><button className="button" type="button" onClick={() => void load()}><RefreshCw size={16} />刷新</button></div>
        <div className="table-wrap compact">
          <table>
            <thead><tr><th>标签</th><th>归属</th><th>权限</th><th>有效期</th><th>操作</th></tr></thead>
            <tbody>
              {tokens.map((token) => (
                <tr key={token.id}>
                  <td>{token.label || token.id}<br /><small>{token.id}</small></td>
                  <td>{token.ownerUsername || token.ownerUserId || "-"}</td>
                  <td>{token.isAdmin ? <span className="badge amber">管理员</span> : <span className="badge slate">用户</span>} {token.isRevoked && <span className="badge rose">已吊销</span>}</td>
                  <td>{token.expiresAt ? formatDate(token.expiresAt) : "永久"}</td>
                  <td><button className="button compact danger" type="button" disabled={token.isRevoked} onClick={() => void revoke(token)}>吊销</button></td>
                </tr>
              ))}
              {!tokens.length && <tr><td colSpan={5}>暂无 Token。</td></tr>}
            </tbody>
          </table>
        </div>
      </div>
    </section>
  );
}

function UsersPanel({ showToast, setError }: { showToast: (msg: string) => void; setError: (msg: string) => void }) {
  const [users, setUsers] = useState<UserItem[]>([]);
  const [username, setUsername] = useState("");
  const [email, setEmail] = useState("");
  const [emailVerified, setEmailVerified] = useState(false);
  const [password, setPassword] = useState("");
  const [limit, setLimit] = useState(20);
  const [isAdmin, setIsAdmin] = useState(false);

  const load = useCallback(async () => {
    try {
      const data = await api<{ users?: UserItem[] }>("/api/admin/users");
      setUsers(data.users || []);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }, [setError]);

  useEffect(() => { void load(); }, [load]);

  async function create() {
    const normalizedEmail = email.trim();
    await api("/api/admin/users", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        username,
        email: normalizedEmail,
        emailVerified: normalizedEmail ? emailVerified : false,
        password,
        deployLimit: limit,
        isAdmin
      })
    });
    setUsername("");
    setEmail("");
    setEmailVerified(false);
    setPassword("");
    setIsAdmin(false);
    showToast("用户已创建");
    await load();
  }

  async function update(user: UserItem) {
    await api(`/api/admin/users/${encodeURIComponent(user.id)}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        username: user.username,
        email: user.email || "",
        emailVerified: Boolean(user.email && user.emailVerified),
        deployLimit: user.deployLimit,
        isAdmin: user.isAdmin,
        isActive: user.isActive
      })
    });
    showToast("用户已保存");
    await load();
  }

  async function remove(user: UserItem) {
    if (!window.confirm(`确认删除用户 ${user.username}？`)) return;
    await api(`/api/admin/users/${encodeURIComponent(user.id)}`, { method: "DELETE" });
    showToast("用户已删除");
    await load();
  }

  return (
    <section className="two-col">
      <div className="panel">
        <div className="panel-head">
          <div>
            <h2>添加用户</h2>
            <p>创建普通用户或管理员账号；邮箱用于注册验证、通知和后续账号找回。</p>
          </div>
        </div>
        <label className="field"><span>用户名</span><input value={username} onChange={(event) => setUsername(event.target.value)} /></label>
        <label className="field"><span>邮箱</span><input type="email" value={email} onChange={(event) => {
          setEmail(event.target.value);
          if (!event.target.value.trim()) setEmailVerified(false);
        }} placeholder="user@example.com" /></label>
        <label className="check-line"><input type="checkbox" checked={emailVerified} disabled={!email.trim()} onChange={(event) => setEmailVerified(event.target.checked)} />邮箱已验证</label>
        <label className="field"><span>初始密码</span><input type="password" value={password} onChange={(event) => setPassword(event.target.value)} placeholder="至少 8 位" /></label>
        <label className="field"><span>部署额度</span><input type="number" value={limit} onChange={(event) => setLimit(Number(event.target.value))} /></label>
        <label className="check-line"><input type="checkbox" checked={isAdmin} onChange={(event) => setIsAdmin(event.target.checked)} />管理员</label>
        <button className="button primary" type="button" onClick={() => void create()}><UserPlus size={16} />创建用户</button>
      </div>
      <div className="panel">
        <div className="panel-head">
          <div>
            <h2>用户列表</h2>
            <p>维护邮箱、验证状态、部署额度、角色和启用状态。</p>
          </div>
          <button className="button" type="button" onClick={() => void load()}><RefreshCw size={16} />刷新</button>
        </div>
        <div className="table-wrap compact">
          <table>
            <thead><tr><th>用户</th><th>邮箱</th><th>额度</th><th>角色</th><th>状态</th><th>操作</th></tr></thead>
            <tbody>
              {users.map((user, index) => (
                <tr key={user.id}>
                  <td><input value={user.username} onChange={(event) => setUsers((prev) => prev.map((item, i) => i === index ? { ...item, username: event.target.value } : item))} /><br /><small>{user.id}</small></td>
                  <td>
                    <input type="email" value={user.email || ""} onChange={(event) => setUsers((prev) => prev.map((item, i) => i === index ? { ...item, email: event.target.value, emailVerified: event.target.value ? item.emailVerified : false } : item))} />
                    <label className="check-line compact"><input type="checkbox" checked={Boolean(user.email && user.emailVerified)} disabled={!user.email} onChange={(event) => setUsers((prev) => prev.map((item, i) => i === index ? { ...item, emailVerified: event.target.checked } : item))} />{user.emailVerified ? "已验证" : "未验证"}</label>
                  </td>
                  <td><input type="number" value={user.deployLimit} onChange={(event) => setUsers((prev) => prev.map((item, i) => i === index ? { ...item, deployLimit: Number(event.target.value) } : item))} /><small>已用 {user.deployCount}</small></td>
                  <td><label className="check-line compact"><input type="checkbox" checked={user.isAdmin} onChange={(event) => setUsers((prev) => prev.map((item, i) => i === index ? { ...item, isAdmin: event.target.checked } : item))} />管理员</label></td>
                  <td><label className="check-line compact"><input type="checkbox" checked={user.isActive} onChange={(event) => setUsers((prev) => prev.map((item, i) => i === index ? { ...item, isActive: event.target.checked } : item))} />启用</label></td>
                  <td><div className="actions tight"><button className="button compact" type="button" onClick={() => void update(user)}>保存</button><button className="button compact danger" type="button" onClick={() => void remove(user)}>删除</button></div></td>
                </tr>
              ))}
              {!users.length && <tr><td colSpan={6}>暂无用户。</td></tr>}
            </tbody>
          </table>
        </div>
      </div>
    </section>
  );
}
function AnonymousPanel({ setError }: { setError: (msg: string) => void }) {
  const [sessions, setSessions] = useState<AnonymousSession[]>([]);
  const [limit, setLimit] = useState(0);

  const load = useCallback(async () => {
    try {
      const data = await api<{ sessions?: AnonymousSession[]; deployLimit?: number }>("/api/admin/anonymous-sessions");
      setSessions(data.sessions || []);
      setLimit(Number(data.deployLimit || 0));
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }, [setError]);

  useEffect(() => { void load(); }, [load]);

  return (
    <section className="panel">
      <div className="panel-head"><div><h2>匿名 Agent</h2><p>所有未登录发布都会记录。网页匿名按浏览器 cookie；Agent 匿名按本地 sessionId 和 X-Hostctl-Session。</p></div><button className="button" type="button" onClick={() => void load()}><RefreshCw size={16} />刷新</button></div>
      <div className="stats-grid small">
        <Metric label="Policy" value={String(limit)} note="每个匿名 session 可部署次数" />
        <Metric label="Scope" value="发布" note="网页匿名与 Agent 匿名统一统计" />
        <Metric label="Agents" value={String(sessions.length)} note="最近活跃匿名身份" />
      </div>
      <div className="table-wrap">
        <table>
          <thead><tr><th>Session</th><th>Agent 标记</th><th>IP / UA</th><th>部署</th><th>剩余</th><th>状态</th><th>最近使用</th></tr></thead>
          <tbody>
            {sessions.map((session) => (
              <tr key={session.id}>
                <td><code>{session.id}</code></td>
                <td>{session.agentLabel || "-"}<br /><small>{session.agentId || "网页匿名或未上报"}</small></td>
                <td>{session.deviceIp || "-"}<br /><small>{session.userAgent || "-"}</small></td>
                <td>{session.deployCount || 0}</td>
                <td>{session.remaining ?? "-"}</td>
                <td>{session.claimedByUserId ? <span className="badge green">已绑定用户</span> : <span className="badge slate">匿名</span>}</td>
                <td>{formatDate(session.lastUsedAt)}</td>
              </tr>
            ))}
            {!sessions.length && <tr><td colSpan={7}>暂无未登录发布记录。</td></tr>}
          </tbody>
        </table>
      </div>
    </section>
  );
}

const auditActorTypeOptions = [
  { value: "user", label: "注册用户" },
  { value: "token", label: "Token / Skill / MCP" },
  { value: "anonymous", label: "匿名 Agent" },
  { value: "browser", label: "浏览器上报" },
  { value: "unknown", label: "未知来源" }
];

const auditActionOptions = [
  { value: "account.password", label: "修改账号密码" },
  { value: "anonymous.claim", label: "匿名绑定用户" },
  { value: "auth.login", label: "账号登录" },
  { value: "auth.logout", label: "账号登出" },
  { value: "auth.register", label: "账号注册" },
  { value: "config.market_categories", label: "更新应用分类" },
  { value: "config.update", label: "更新运行配置" },
  { value: "deploy.create", label: "新建发布" },
  { value: "deploy.version.create", label: "追加版本" },
  { value: "screen.bind", label: "绑定屏幕" },
  { value: "screen.command.request", label: "下发屏幕指令" },
  { value: "screen.publish", label: "屏幕投放" },
  { value: "screen.screenshot.request", label: "请求屏幕截图" },
  { value: "screen.unbind", label: "解绑屏幕" },
  { value: "security.csp_report", label: "CSP 安全上报" },
  { value: "site.access_login", label: "访问密码验证" },
  { value: "site.access_password", label: "设置访问密码" },
  { value: "site.category", label: "应用分类" },
  { value: "site.delete", label: "删除站点" },
  { value: "site.pin", label: "置顶状态" },
  { value: "site.primary_strategy", label: "主版本策略" },
  { value: "site.reuse_policy", label: "复用策略" },
  { value: "site.security_mode", label: "安全模式" },
  { value: "site.tags", label: "标签分类" },
  { value: "site.visibility", label: "可见性" },
  { value: "skill.package_upload", label: "上传 Skill 包" },
  { value: "token.create", label: "创建 Token" },
  { value: "token.revoke", label: "吊销 Token" },
  { value: "user.create", label: "创建用户" },
  { value: "user.update", label: "更新用户" },
  { value: "user.delete", label: "删除用户" },
  { value: "version.current", label: "切换当前版本" },
  { value: "version.delete", label: "删除版本" },
  { value: "version.lock", label: "版本锁定" },
  { value: "version.overwrite", label: "覆盖版本" },
  { value: "version.status", label: "版本状态" }
];

const legacyAuditActionLabels: Record<string, string> = {
  "site.append": "追加版本（旧版）",
  "site.create": "新建发布（旧版）",
  "skill.upload": "上传 Skill 包（旧版）",
  "token.delete": "吊销 Token（旧版）",
  "version.rollback": "版本回滚（旧版）"
};

const auditActionLabels: Record<string, string> = {
  ...Object.fromEntries(auditActionOptions.map((option) => [option.value, option.label])),
  ...legacyAuditActionLabels
};

function AuditPanel({ setError }: { setError: (msg: string) => void }) {
  const [logs, setLogs] = useState<AuditLogItem[]>([]);
  const [auditUsers, setAuditUsers] = useState<UserItem[]>([]);
  const [auditSites, setAuditSites] = useState<SiteItem[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(30);
  const [filters, setFilters] = useState({
    q: "",
    actorType: "",
    action: "",
    siteCode: "",
    actorId: "",
    actorRole: "",
    result: "",
    targetType: "",
    since: "",
    until: ""
  });
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    async function loadFilterOptions() {
      try {
        const [userData, siteData] = await Promise.all([
          api<{ users?: UserItem[] }>("/api/admin/users"),
          api<{ sites?: SiteItem[] }>("/api/admin/sites")
        ]);
        setAuditUsers(userData.users || []);
        setAuditSites(siteData.sites || []);
      } catch (err) {
        setError(err instanceof Error ? err.message : String(err));
      }
    }
    void loadFilterOptions();
  }, [setError]);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const params = new URLSearchParams();
      params.set("page", String(page));
      params.set("pageSize", String(pageSize));
      for (const [key, value] of Object.entries(filters)) {
        const trimmed = String(value || "").trim();
        if (!trimmed) continue;
        if (key === "since" || key === "until") {
          params.set(key, new Date(trimmed).toISOString());
        } else {
          params.set(key, trimmed);
        }
      }
      const data = await api<AuditLogResponse>(`/api/admin/audit-logs?${params.toString()}`);
      setLogs(data.logs || []);
      setTotal(Number(data.total || 0));
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, [filters, page, pageSize, setError]);

  useEffect(() => { void load(); }, [load]);

  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const updateFilter = (key: keyof typeof filters, value: string) => {
    setFilters((prev) => ({ ...prev, [key]: value }));
    setPage(1);
  };

  return (
    <section className="panel audit-panel">
      <div className="panel-head">
        <div>
          <h2>审计日志</h2>
          <p>记录关键操作的时间、操作者、来源 IP、目标对象和结构化详情。</p>
        </div>
        <button className="button" type="button" onClick={() => void load()} disabled={loading}>
          <RefreshCw size={16} />{loading ? "刷新中" : "刷新"}
        </button>
      </div>
      <div className="filter-row audit-filter-row">
        <label className="field"><span>关键词</span><input value={filters.q} onChange={(event) => updateFilter("q", event.target.value)} placeholder="动作、站点、对象或详情" /></label>
        <label className="field"><span>操作者类型</span><select value={filters.actorType} onChange={(event) => updateFilter("actorType", event.target.value)}><option value="">全部来源</option>{auditActorTypeOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select></label>
        <label className="field"><span>用户</span><select value={filters.actorType === "user" ? filters.actorId : ""} onChange={(event) => { updateFilter("actorType", event.target.value ? "user" : ""); updateFilter("actorId", event.target.value); }}><option value="">全部用户</option>{auditUsers.map((user) => <option value={user.id} key={user.id}>{user.username}{user.isAdmin ? " · 管理员" : ""}</option>)}</select></label>
        <label className="field"><span>动作</span><select value={filters.action} onChange={(event) => updateFilter("action", event.target.value)}><option value="">全部动作</option>{auditActionOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select></label>
        <label className="field"><span>站点</span><select value={filters.siteCode} onChange={(event) => updateFilter("siteCode", event.target.value)}><option value="">全部站点</option>{auditSites.map((site) => <option value={site.code} key={site.code}>{site.code}{site.ownerUsername ? ` · ${site.ownerUsername}` : ""}</option>)}</select></label>
        <label className="field"><span>操作者 ID</span><input value={filters.actorId} onChange={(event) => updateFilter("actorId", event.target.value)} placeholder="用户 ID / token ID / 匿名 session" /></label>
        <label className="field"><span>角色</span><select value={filters.actorRole} onChange={(event) => updateFilter("actorRole", event.target.value)}><option value="">全部</option><option value="admin">管理员</option><option value="user">用户</option><option value="anonymous">匿名</option></select></label>
        <label className="field"><span>结果</span><select value={filters.result} onChange={(event) => updateFilter("result", event.target.value)}><option value="">全部</option><option value="success">成功</option><option value="failed">失败</option></select></label>
        <label className="field"><span>对象类型</span><select value={filters.targetType} onChange={(event) => updateFilter("targetType", event.target.value)}><option value="">全部</option><option value="site">站点</option><option value="version">版本</option><option value="token">Token</option><option value="screen">屏幕</option><option value="user">用户</option><option value="config">配置</option></select></label>
      </div>
      <div className="filter-row audit-time-row">
        <label className="field"><span>开始时间</span><input type="datetime-local" value={filters.since} onChange={(event) => updateFilter("since", event.target.value)} /></label>
        <label className="field"><span>结束时间</span><input type="datetime-local" value={filters.until} onChange={(event) => updateFilter("until", event.target.value)} /></label>
        <label className="field"><span>每页</span><select value={pageSize} onChange={(event) => { setPageSize(Number(event.target.value)); setPage(1); }}><option value={20}>20 条</option><option value={30}>30 条</option><option value={50}>50 条</option><option value={100}>100 条</option></select></label>
        <div className="toolbar-tail audit-pager">
          <button className="button compact" type="button" disabled={page <= 1} onClick={() => setPage((value) => Math.max(1, value - 1))}>上一页</button>
          <span>{page} / {totalPages}</span>
          <button className="button compact" type="button" disabled={page >= totalPages} onClick={() => setPage((value) => Math.min(totalPages, value + 1))}>下一页</button>
        </div>
      </div>
      <div className="summary-line"><span>共 {total} 条记录</span><span>{loading ? "正在加载..." : "已加载"}</span></div>
      <div className="table-wrap">
        <table className="audit-table">
          <thead><tr><th>时间</th><th>动作</th><th>结果</th><th>操作者</th><th>目标</th><th>来源</th><th>详情</th></tr></thead>
          <tbody>
            {logs.map((log) => (
              <tr key={log.id}>
                <td>{formatDate(log.createdAt)}</td>
                <td>
                  <strong>{auditActionLabel(log.action)}</strong>
                  <br /><code>{log.action || "-"}</code>
                </td>
                <td><span className={`badge ${log.result === "success" ? "green" : "amber"}`}>{auditResultLabel(log.result)}</span></td>
                <td>
                  <span className={`badge ${log.actorRole === "admin" ? "green" : log.actorRole === "anonymous" ? "amber" : "slate"}`}>{actorRoleLabel(log.actorRole)}</span>
                  <br /><small>{log.actorType || "-"} · {log.actorId || "-"}</small>
                </td>
                <td>{log.siteCode && <code>{log.siteCode}</code>}<br /><small>{log.targetType || "-"} · {log.targetId || "-"}</small></td>
                <td>{log.ip || "-"}<br /><small>{log.userAgent || "-"}</small></td>
                <td><pre className="audit-detail">{formatAuditDetail(log.detail)}</pre></td>
              </tr>
            ))}
            {!logs.length && <tr><td colSpan={7}>暂无审计记录。</td></tr>}
          </tbody>
        </table>
      </div>
    </section>
  );
}

function actorRoleLabel(role?: string) {
  if (role === "admin") return "管理员";
  if (role === "anonymous") return "匿名";
  if (role === "user") return "用户";
  return "未知";
}

function auditResultLabel(result?: string) {
  if (result === "success") return "成功";
  if (result === "failed") return "失败";
  return result || "未知";
}

function auditActionLabel(action?: string) {
  return auditActionLabels[action || ""] || action || "-";
}

function formatAuditDetail(detail: unknown) {
  if (detail == null) return "{}";
  if (typeof detail === "string") return detail;
  try {
    return JSON.stringify(detail, null, 2);
  } catch {
    return String(detail);
  }
}

function ConfigPanel({ config, onConfig, showToast, setError }: { config: RuntimeConfig | null; onConfig: (cfg: RuntimeConfig) => void; showToast: (msg: string) => void; setError: (msg: string) => void }) {
  const [draft, setDraft] = useState({
    appURLMode: "path",
    appDomainSuffix: "",
    appURLScheme: "https",
    appURLPort: "",
    anonymousDeployLimit: 5,
    cooldownSeconds: 10,
    maxSingleMB: 1,
    maxTotalMB: 10,
    maxFiles: 100,
    cors: "",
    embedPolicy: "any" as "any" | "self" | "allowlist" | "deny",
    embedAllowOrigins: ""
  });

  useEffect(() => {
    if (!config) return;
    setDraft({
      appURLMode: config.appURL?.appURLMode || "path",
      appDomainSuffix: config.appURL?.appDomainSuffix || "",
      appURLScheme: config.appURL?.appURLScheme || "https",
      appURLPort: config.appURL?.appURLPort || "",
      anonymousDeployLimit: config.anonymousPolicy?.deployLimit ?? 5,
      cooldownSeconds: config.cooldownSeconds ?? 10,
      maxSingleMB: Number(((config.limits?.maxSingleFileBytes || 0) / 1024 / 1024).toFixed(2)),
      maxTotalMB: Number(((config.limits?.maxSiteTotalBytes || 0) / 1024 / 1024).toFixed(2)),
      maxFiles: config.limits?.maxFilesPerSite || 100,
      cors: config.corsAllowOrigins || "",
      embedPolicy: config.embedPolicy || "any",
      embedAllowOrigins: config.embedAllowOrigins || ""
    });
  }, [config]);

  async function save() {
    try {
      const data = await api<RuntimeConfig>("/api/config", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          appURLMode: draft.appURLMode,
          appDomainSuffix: draft.appDomainSuffix,
          appURLScheme: draft.appURLScheme,
          appURLPort: draft.appURLPort,
          anonymousDeployLimit: Number(draft.anonymousDeployLimit),
          cooldownSeconds: Number(draft.cooldownSeconds),
          maxSingleFileBytes: Math.round(Number(draft.maxSingleMB) * 1024 * 1024),
          maxSiteTotalBytes: Math.round(Number(draft.maxTotalMB) * 1024 * 1024),
          maxFilesPerSite: Number(draft.maxFiles),
          corsAllowOrigins: draft.cors,
          embedPolicy: draft.embedPolicy,
          embedAllowOrigins: draft.embedAllowOrigins
        })
      });
      onConfig(data);
      showToast("运行设置已保存");
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  const requestBaseURLPreview = currentBaseURL();
  const baseURLPreview = requestBaseURLPreview;
  const portText = String(draft.appURLPort || "").trim();
  const portSuffix = portText ? `:${portText}` : "";
  const domainSuffix = String(draft.appDomainSuffix || "apps.example.com").replace(/^\.+/, "");
  const sampleCode = "demo-app";
  const pathURL = `${baseURLPreview}/agent/${sampleCode}/`;
  const domainURL = `${draft.appURLScheme}://${sampleCode}.${domainSuffix}${portSuffix}/`;
  const modeText = draft.appURLMode === "domain" ? "只生成泛域名链接" : draft.appURLMode === "dual" ? "同时保留路径和泛域名链接" : "默认生成 /agent/{code} 路径链接";
  const mainSiteText = "主站链接跟随当前访问域名，无需配置";
  const embedModeText = draft.embedPolicy === "deny" ? "禁止任何网站 iframe 嵌入应用" : draft.embedPolicy === "self" ? "只允许本站嵌入应用" : draft.embedPolicy === "allowlist" ? "本站和白名单来源可嵌入应用" : "允许任意网站 iframe 嵌入应用";

  return (
    <section className="panel config-panel">
      <div className="panel-head">
        <div>
          <h2>运行设置</h2>
          <p>主站链接会自动跟随当前访问域名；这里主要管理应用泛域名、上传限制、跨域和嵌入策略。</p>
        </div>
      </div>

      <div className="config-layout">
        <div className="config-main">
          <section className="config-section">
            <div className="config-section-head">
              <strong>主站访问入口</strong>
              <span>自动跟随当前浏览器域名</span>
            </div>
            <div className="readonly-callout">
              <div>
                <span>当前主站</span>
                <code>{baseURLPreview}</code>
              </div>
              <em>首页、后台、/agents/、/screens/、二维码、Skill/MCP 文案和路径模式 /agent/{"{code}"} 都使用当前打开 PagePilot 的域名或 IP，不需要在后台配置入口地址。</em>
            </div>
          </section>

          <section className="config-section">
            <div className="config-section-head">
              <strong>应用链接规则</strong>
              <span>决定发布后的应用 URL 怎么生成</span>
            </div>
            <div className="form-grid">
              <label className="field rich-field">
                <span>访问模式</span>
                <select value={draft.appURLMode} onChange={(event) => setDraft({ ...draft, appURLMode: event.target.value })}>
                  <option value="path">路径模式：/agent/{"{code}"}</option>
                  <option value="domain">泛域名模式：{"{code}"}.domain</option>
                  <option value="dual">双模式兼容：路径 + 泛域名</option>
                </select>
                <em>没有泛域名 DNS 或反向代理时选路径模式。已经配置泛域名时可以选泛域名或双模式。</em>
              </label>
              <label className="field rich-field">
                <span>泛域名后缀</span>
                <input className="mono" value={draft.appDomainSuffix} onChange={(event) => setDraft({ ...draft, appDomainSuffix: event.target.value })} placeholder="apps.example.com" />
                <em>只填域名后缀，不要写协议。例如 apps.example.com，对应 demo-app.apps.example.com。</em>
              </label>
            </div>
            <div className="form-grid">
              <label className="field rich-field">
                <span>应用协议</span>
                <select value={draft.appURLScheme} onChange={(event) => setDraft({ ...draft, appURLScheme: event.target.value })}>
                  <option value="https">https</option>
                  <option value="http">http</option>
                </select>
                <em>泛域名链接使用的协议。公网生产环境建议 https。</em>
              </label>
              <label className="field rich-field">
                <span>应用端口</span>
                <input className="mono" value={draft.appURLPort} onChange={(event) => setDraft({ ...draft, appURLPort: event.target.value })} placeholder="留空 / 443 / 1143" />
                <em>只在泛域名链接需要显式端口时填写。标准 443 通常可以留空；你的 1143 场景可填 1143。</em>
              </label>
            </div>
          </section>

          <section className="config-section">
            <div className="config-section-head">
              <strong>发布限额</strong>
              <span>控制匿名发布、上传大小和滥用防护</span>
            </div>
            <div className="form-grid three">
              <label className="field rich-field">
                <span>匿名额度</span>
                <input type="number" min={0} value={draft.anonymousDeployLimit} onChange={(event) => setDraft({ ...draft, anonymousDeployLimit: Number(event.target.value) })} />
                <em>未登录网页或匿名 Agent 最多可发布次数；注册用户不受这个匿名额度影响。</em>
              </label>
              <label className="field rich-field">
                <span>部署冷却秒</span>
                <input type="number" min={0} value={draft.cooldownSeconds} onChange={(event) => setDraft({ ...draft, cooldownSeconds: Number(event.target.value) })} />
                <em>两次发布之间的最短间隔，用于防刷。</em>
              </label>
              <label className="field rich-field">
                <span>文件数上限</span>
                <input type="number" min={1} value={draft.maxFiles} onChange={(event) => setDraft({ ...draft, maxFiles: Number(event.target.value) })} />
                <em>多文件静态站点单次最多上传多少个文件。</em>
              </label>
            </div>
            <div className="form-grid">
              <label className="field rich-field">
                <span>单文件上限 MB</span>
                <input type="number" min={0.1} step={0.1} value={draft.maxSingleMB} onChange={(event) => setDraft({ ...draft, maxSingleMB: Number(event.target.value) })} />
                <em>限制单个 HTML、JS、CSS、图片等文件大小。</em>
              </label>
              <label className="field rich-field">
                <span>整站上限 MB</span>
                <input type="number" min={0.1} step={0.1} value={draft.maxTotalMB} onChange={(event) => setDraft({ ...draft, maxTotalMB: Number(event.target.value) })} />
                <em>限制一次发布的所有文件总大小。</em>
              </label>
            </div>
          </section>

          <section className="config-section">
            <div className="config-section-head">
              <strong>跨域与嵌入</strong>
              <span>CORS 管 API，iframe 管应用嵌入</span>
            </div>
            <div className="embed-policy-inline">
              <label className="field rich-field">
                <span>iframe 嵌入</span>
                <select value={draft.embedPolicy} onChange={(event) => setDraft({ ...draft, embedPolicy: event.target.value as "any" | "self" | "allowlist" | "deny" })}>
                  <option value="any">允许任意网站嵌入</option>
                  <option value="self">只允许本站嵌入</option>
                  <option value="allowlist">本站 + 白名单来源</option>
                  <option value="deny">禁止被任何网站嵌入</option>
                </select>
                <em>控制外部网站是否能 iframe 嵌入应用 URL；它会写入应用内容的 CSP frame-ancestors，和 CORS 不是一回事。</em>
              </label>
              {draft.embedPolicy === "allowlist" && (
                <label className="field rich-field">
                  <span>允许嵌入来源</span>
                  <textarea
                    value={draft.embedAllowOrigins}
                    onChange={(event) => setDraft({ ...draft, embedAllowOrigins: event.target.value })}
                    placeholder={"https://portal.example.com\nhttps://display.example.com"}
                  />
                  <em>必须包含 http(s) 协议，不要带路径；多个来源可换行或逗号分隔。</em>
                </label>
              )}
            </div>
            <label className="field rich-field">
              <span>CORS 允许来源</span>
              <textarea
                value={draft.cors}
                onChange={(event) => setDraft({ ...draft, cors: event.target.value })}
                placeholder={"留空表示不开放跨域 API\nhttps://studio.example.com\nhttps://admin.example.com"}
              />
              <em>只在外部网站需要用 fetch/XHR 调 PagePilot API 时填写；iframe 嵌入应用 URL 请使用上面的嵌入策略。</em>
            </label>
          </section>
        </div>

        <aside className="config-preview">
          <div className="preview-card">
            <span>当前链接策略</span>
            <strong>{modeText}</strong>
            <div className="preview-row">
              <small>主站来源</small>
              <code>{mainSiteText}</code>
            </div>
            <div className="preview-row">
              <small>路径链接</small>
              <code>{pathURL}</code>
            </div>
            <div className="preview-row">
              <small>泛域名链接</small>
              <code>{domainURL}</code>
            </div>
            <div className="preview-row">
              <small>iframe 嵌入</small>
              <code>{embedModeText}</code>
            </div>
          </div>
          <div className="preview-card muted">
            <span>会被影响</span>
            <ul>
              <li>主站按钮、复制链接、二维码和 Skill/MCP 文案使用当前访问域名</li>
              <li>路径模式应用地址固定为当前主站下的 /agent/{"{code}"}</li>
              <li>只有启用泛域名应用访问时，才需要填写应用域名后缀</li>
              <li>CORS 和 iframe 嵌入策略分别控制 API 跨域和应用被外站嵌入</li>
            </ul>
          </div>
          <div className="preview-card muted">
            <span>部署能力状态</span>
            <div className="preview-row">
              <small>注册邮箱验证</small>
              <code>{config?.email?.verificationEnabled ? (config.email.smtpConfigured ? "已开启，SMTP 已配置" : "已开启，缺少 SMTP") : "未开启"}</code>
            </div>
            <div className="preview-row">
              <small>文件存储</small>
              <code>{config?.storage?.backend === "oss" ? `OSS / ${config.storage.ossBucket || "未配置 bucket"}` : `本地文件系统 / ${config?.storage?.hostedDir || "-"}`}</code>
            </div>
            <p className="muted-line">邮箱验证和 OSS 属于启动级配置，请通过环境变量或 Docker Compose 管理，不在后台热更新。</p>
          </div>
          <button className="button primary full" type="button" onClick={() => void save()}><Save size={16} />保存运行设置</button>
        </aside>
      </div>
    </section>
  );
}

function SkillMCPPanel({ config, showToast, setError }: { config: RuntimeConfig | null; showToast: (msg: string) => void; setError: (msg: string) => void }) {
  const [tab, setTab] = useState<SkillTab>("skill");
  const [meta, setMeta] = useState<any>(null);
  const [zipFile, setZipFile] = useState<File | null>(null);
  const [uploading, setUploading] = useState(false);
  const base = currentBaseURL();

  const load = useCallback(async () => {
    try {
      const data = await api<any>("/api/admin/skill");
      setMeta(data);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }, [setError]);

  useEffect(() => { void load(); }, [load]);

  async function uploadPackage() {
    if (!zipFile) return;
    setUploading(true);
    try {
      const body = new FormData();
      body.append("file", zipFile);
      const data = await api<any>("/api/admin/skill/package", {
        method: "POST",
        body
      });
      setMeta((prev: any) => ({ ...(prev || {}), package: data.package }));
      setZipFile(null);
      showToast("Skill 下载包已上传");
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setUploading(false);
    }
  }

  const skillURL = `${base}${skillDownloadPath()}`;
  const pkg = meta?.package;
  const packageLabel = pkg?.source === "uploaded" ? "已上传覆盖包" : pkg?.exists ? "内置默认包" : "未上传";
  const prompt = [
    "PAGEPILOT SKILL",
    `请从 ${skillURL} 下载并安装 pagep Skill。`,
    `安装后使用 \`pagep doctor --server ${base}\` 检查连接，然后用它把网页发布到 PagePilot。`
  ].join("\n");

  return (
    <section className="panel">
      <div className="panel-head">
        <div><h2>Skill & MCP</h2><p>Skill 下载包由后台上传维护；MCP 是 stdio 工具服务，两者共用 Token 与权限模型。</p></div>
        <div className="actions">
          <a className="button" href={skillDownloadPath()} target="_blank" rel="noreferrer"><Download size={16} />下载 Skill</a>
          {tab === "skill" && <button className="button primary" type="button" onClick={() => void uploadPackage()} disabled={!zipFile || uploading}><Upload size={16} />{uploading ? "上传中..." : "上传 ZIP"}</button>}
        </div>
      </div>
      <div className="segmented slim">
        <button className={tab === "skill" ? "active" : ""} type="button" onClick={() => setTab("skill")}>Skill</button>
        <button className={tab === "mcp" ? "active" : ""} type="button" onClick={() => setTab("mcp")}>MCP</button>
      </div>
      {tab === "skill" ? (
        <div className="skill-layout">
          <aside>
            <label className="upload-zone skill-zip-upload">
              <input type="file" accept=".zip,application/zip" onChange={(event) => setZipFile(event.target.files?.[0] || null)} />
              <Upload size={20} />
              <strong>{zipFile ? zipFile.name : "上传 pagep.zip"}</strong>
              <span>{zipFile ? formatSize(zipFile.size) : "这个 ZIP 会成为 /skill/pagep.zip 的固定下载包"}</span>
            </label>
            <div className="meta-box">
              <InfoRow label="下载包" value={packageLabel} />
              <InfoRow label="包大小" value={pkg?.exists ? formatSize(pkg?.size) : "-"} />
              <InfoRow label="更新时间" value={formatDate(pkg?.updatedAt)} />
              <InfoRow label="SHA256" value={pkg?.sha256 || "-"} />
            </div>
            <div className="copy-panel">
              <strong>复制给 AGENT</strong>
              <pre>{prompt}</pre>
              <button className="button compact" type="button" onClick={() => navigator.clipboard.writeText(prompt)}><Copy size={14} />复制</button>
            </div>
          </aside>
          <div className="skill-package-guide">
            <h3>只维护下载包</h3>
            <p>后台不再直接编辑 Skill 源文件。需要调整 Skill 时，在本地修改并打包成 <code>pagep.zip</code> 后上传，服务器会把它作为固定下载包提供给 Agent。</p>
            <ul>
              <li>内置默认包用于新部署兜底，避免下载 404。</li>
              <li>上传覆盖包后，下载地址保持 <code>/skill/pagep.zip</code>，旧的 <code>/skill/hostctl-deploy.zip</code> 兼容可用。</li>
              <li>复用创作市场作品时传 templateSourceCode / template_source_code，服务端会记录来源并增加复用次数。</li>
              <li>发布后的应用 URL 以后端接口返回为准，Skill/MCP 不自行拼接。</li>
            </ul>
          </div>
        </div>
      ) : (
        <div className="doc-grid">
          <DocBlock title="启动方式" lines={[`pagep-mcp --server ${base} --token YOUR_TOKEN`, "私有服务器请替换 --server；屏幕能力必须使用注册用户 Token。"]} />
        <DocBlock title="发布工具" lines={["deploy_site: 新建发布或追加版本，支持多文件、访问密码和 template_source_code 模板来源记录。", "get_site_content: 读取站点源码和文件清单。", "set_site_access_password: 设置或清除访问密码。", "claim_anonymous_session: 将匿名发布归属到注册用户。"]} />
          <DocBlock title="屏幕工具" lines={["list_screens: 查询用户屏幕。", "publish_screen: 从自己的站点或创作市场选择投放。", "request_screen_screenshot: 下发一次截图指令。", "send_screen_command: refresh、sleep、wake、shutdown。"]} />
          <DocBlock title="匿名身份" lines={["Agent 匿名以 X-Hostctl-Session 为所有权依据。", "X-Hostctl-Agent-Id、Agent-Label、IP、UA 只用于后台展示和排查。", "网页匿名使用浏览器 HttpOnly cookie，同样计入匿名发布。"]} />
        </div>
      )}
    </section>
  );
}

function Metric({ label, value, note }: { label: string; value: string; note: string }) {
  return <div className="metric"><span>{label}</span><strong>{value}</strong><em>{note}</em></div>;
}

function InfoRow({ label, value, copy }: { label: string; value: string; copy?: boolean }) {
  return (
    <div className="info-row">
      <span>{label}</span>
      <strong>{value}</strong>
      {copy && <button className="icon-button" type="button" onClick={() => navigator.clipboard.writeText(value)}><Copy size={13} /></button>}
    </div>
  );
}

function ApiDocsPanel({ config }: { config: RuntimeConfig | null }) {
  const base = currentBaseURL();
  const sections = [
    { id: "auth", label: "认证" },
    { id: "deploy", label: "发布" },
    { id: "market", label: "市场" },
    { id: "version", label: "版本" },
    { id: "screens", label: "屏幕" }
  ];
  const endpoints = [
    ["POST", "/api/deploy", "发布 HTML、Markdown、ZIP 或多文件静态站点"],
    ["GET", "/api/deploys", "查询创作市场和当前用户内容"],
    ["GET", "/api/deploy/content", "读取或下载源码"],
    ["GET", "/api/deploys/{code}/versions", "查看版本历史"],
    ["PATCH", "/api/deploys/{code}/current", "切换当前版本"],
    ["POST", "/api/token", "创建 API Token"],
    ["GET", "/api/screens", "查询当前用户屏幕"],
    ["POST", "/api/screens/{id}/publish", "投放应用到屏幕"]
  ];
  return (
    <section className="api-doc-layout">
      <aside className="api-doc-sidebar">
        <span className="eyebrow">HTTP API</span>
        <strong>PagePilot API 文档</strong>
        <p>API、Skill、MCP 和 CLI 共用同一套发布、源码、版本、屏幕和 Token 能力。</p>
        <nav>
          {sections.map((item) => <a href={`#${item.id}`} key={item.id}>{item.label}</a>)}
        </nav>
        <a className="button primary full" href="/openapi.json" target="_blank" rel="noreferrer"><Download size={16} />下载 OpenAPI</a>
        <button className="button full" type="button" onClick={() => navigator.clipboard.writeText(`${base}/openapi.json`)}><Copy size={16} />复制 OpenAPI 地址</button>
      </aside>
      <div className="api-doc-content">
        <section id="auth" className="panel">
          <div className="panel-head"><div><h2>认证方式</h2><p>浏览器、脚本、Agent 和匿名发布分别使用不同身份，但权限边界一致。</p></div></div>
          <DocBlock title="认证头" lines={["Session Cookie: 登录后台后浏览器自动携带。", "API Token: Authorization: Bearer <token>，在后台 API 令牌页创建。", "匿名 Session: X-Hostctl-Session，只能管理该匿名身份自己的应用。"]} />
        </section>
        <section id="deploy" className="panel">
          <div className="panel-head"><div><h2>发布应用</h2><p>HTML、Markdown、ZIP 和多文件站点走同一发布模型。</p></div></div>
          <DocBlock title="发布格式" lines={["content + filename: 单 HTML 或 Markdown。", "files[]: 多文件目录，文本用 content，二进制用 contentBase64。", "单 ZIP 文件: path 以 .zip 结尾，contentBase64 上传，服务端自动解压并识别入口。"]} />
          <DocBlock title="当前限制" lines={[`Base URL: ${base}`, `单文件上限: ${formatSize(config?.limits?.maxSingleFileBytes)}`, `整站上限: ${formatSize(config?.limits?.maxSiteTotalBytes)}`, `文件数上限: ${config?.limits?.maxFilesPerSite ?? "-"}`]} />
        </section>
        <section id="market" className="panel">
          <div className="panel-head"><div><h2>创作市场</h2><p>公开内容可被搜索、筛选、收藏和点赞；公开未加密内容可按策略下载源文件。</p></div></div>
          <div className="endpoint-list admin-endpoints">
            {endpoints.slice(1, 3).map(([method, path, desc]) => (
              <div className="endpoint-row" key={`${method}-${path}`}><span className={`method ${method}`}>{method}</span><code>{path}</code><p>{desc}</p></div>
            ))}
          </div>
        </section>
        <section id="version" className="panel">
          <div className="panel-head"><div><h2>版本管理</h2><p>每次更新追加版本；当前版本、锁定、删除和回滚由权限控制。</p></div></div>
          <div className="endpoint-list admin-endpoints">
            {endpoints.slice(3, 5).map(([method, path, desc]) => (
              <div className="endpoint-row" key={`${method}-${path}`}><span className={`method ${method}`}>{method}</span><code>{path}</code><p>{desc}</p></div>
            ))}
          </div>
        </section>
        <section id="screens" className="panel">
          <div className="panel-head"><div><h2>屏幕能力</h2><p>注册屏幕、投放应用、截图和运行命令由登录用户 Token 管理。</p></div></div>
          <div className="endpoint-list admin-endpoints">
            {endpoints.slice(6).map(([method, path, desc]) => (
              <div className="endpoint-row" key={`${method}-${path}`}><span className={`method ${method}`}>{method}</span><code>{path}</code><p>{desc}</p></div>
            ))}
          </div>
        </section>
      </div>
    </section>
  );
}

function DeviceInfoBlock({ screen }: { screen: ScreenItem }) {
  const rows = buildDeviceInfoRows({
    deviceName: screen.deviceName,
    appVersion: screen.appVersion,
    runtime: screen.runtime,
    deviceInfo: screen.deviceInfo
  });
  const summary = formatDeviceInfoSummary({
    deviceName: screen.deviceName,
    appVersion: screen.appVersion,
    runtime: screen.runtime,
    deviceInfo: screen.deviceInfo
  });

  return (
    <div className="device-info-block">
      <div className="device-info-title">
        <span>设备信息</span>
        <strong>{summary}</strong>
      </div>
      {rows.length ? (
        <div className="device-info-grid">
          {rows.map((row) => (
            <div className={row.priority ? "device-info-item priority" : "device-info-item"} key={row.key}>
              <span>{row.label}</span>
              <strong title={row.value}>{row.value}</strong>
            </div>
          ))}
        </div>
      ) : (
        <div className="device-info-empty">暂无设备上报信息</div>
      )}
    </div>
  );
}

function statusBadge(status: string, protectedSite = false) {
  if (protectedSite) return <span className="badge amber">加密</span>;
  if (status === "active") return <span className="badge green">运行中</span>;
  if (status === "inactive") return <span className="badge rose">已下架</span>;
  return <span className="badge slate">{status}</span>;
}

function Modal({ title, children, onClose }: { title: string; children: React.ReactNode; onClose: () => void }) {
  return (
    <div className="modal" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}>
      <div className="modal-card">
        <div className="modal-head"><strong>{title}</strong><button className="button ghost" type="button" onClick={onClose}>关闭</button></div>
        {children}
      </div>
    </div>
  );
}

function DocBlock({ title, lines }: { title: string; lines: string[] }) {
  return <div className="doc-block"><h3>{title}</h3><pre>{lines.join("\n")}</pre></div>;
}

function Logo() {
  return (
    <img className="logo" src="/brand/pagepilot-logo.png" alt="" aria-hidden="true" />
  );
}
