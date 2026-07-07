import {
  Button,
  Steps,
  Tag,
  Timeline
} from "antd";
import {
  AlertTriangle,
  Bot,
  Bookmark,
  ChevronLeft,
  Code2,
  Copy,
  Download,
  Eye,
  ExternalLink,
  FileArchive,
  FileCode2,
  FileText,
  Heart,
  KeyRound,
  Layers,
  Lock,
  Monitor,
  PackageOpen,
  Rocket,
  Search,
  ShieldCheck,
  Smartphone,
  Sparkles,
  Tablet,
  Trash2,
  Upload,
  UserRound,
  Workflow
} from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { APIError, api } from "./api";
import type {
  DeployFilePayload,
  DeployResponse,
  MarketCategoryInfo,
  MarketplaceDeploy,
  RuntimeConfig,
  ScreenInfo,
  SessionInfo
} from "./types";

type Page = "home" | "market" | "agents" | "deploy" | "screens";
type SortMode = "hot" | "newest" | "featured" | "oldest" | "likes_desc" | "views_desc";
type MarketCategory = string;
type MarketKind = "all" | "html" | "md" | "protected" | "featured" | "mine" | "favorites";
type AgentDocTab = "skill" | "mcp";
type PreviewStatus = "idle" | "loading" | "loaded";
type PreviewViewport = "desktop" | "tablet" | "mobile";

const PREVIEW_IFRAME_SANDBOX =
  "allow-scripts allow-forms allow-popups allow-popups-to-escape-sandbox allow-downloads allow-modals allow-top-navigation-by-user-activation";

interface EditableDeployFile {
  path: string;
  content: string;
  contentBase64?: string;
  isText?: boolean;
  size?: number;
}

interface MarketplaceResponse {
  deploys?: MarketplaceDeploy[];
  total?: number;
  page?: number;
  pageSize?: number;
}

interface MarketCategoriesResponse {
  categories?: MarketCategoryInfo[];
}

interface VersionItem {
  id?: string;
  versionId?: string;
  versionNumber: number;
  templateSourceCode?: string;
  templateSourceVersion?: number;
  status?: string;
  isCurrent?: boolean;
  isLocked?: boolean;
  likeCount?: number;
  createdAt?: string;
}

interface VersionsResponse {
  versions?: VersionItem[];
}

interface ScreensResponse {
  screens?: ScreenInfo[];
}

interface DeploySiteItem {
  code: string;
  title?: string;
  filename?: string;
  currentVersion?: number;
  versionCount?: number;
  visibility?: string;
  updatedAt?: string;
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

interface SiteListResponse {
  sites?: DeploySiteItem[];
}

const navItems: Array<{ page: Page; label: string; href: string }> = [
  { page: "home", label: "首页", href: "/" },
  { page: "market", label: "创作市场", href: "/market" },
  { page: "deploy", label: "手动部署", href: "/deploy" },
  { page: "screens", label: "广告屏", href: "/screens/" }
];

function getPageFromPath(pathname: string): Page {
  if (pathname.startsWith("/market")) return "market";
  if (pathname.startsWith("/deploy")) return "deploy";
  if (pathname.startsWith("/agents")) return "agents";
  if (pathname.startsWith("/screens")) return "screens";
  return "home";
}

function getMarketDetailKeyFromPath(pathname: string): string {
  const match = pathname.match(/^\/market\/([^/?#]+)/);
  if (!match?.[1]) return "";
  try {
    return decodeURIComponent(match[1]);
  } catch {
    return match[1];
  }
}

function formatSize(bytes?: number): string {
  const n = Number(bytes || 0);
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / 1024 / 1024).toFixed(2)} MB`;
}

interface FileTreeEntry {
  path: string;
  size?: number;
  sha256?: string;
  isBinary?: boolean;
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
  files: FileTreeEntry[];
  totalSize?: number;
  onCopy: (value: string) => void | Promise<void>;
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
  const visible = filtered.slice(0, 120);

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
                <button type="button" title="复制 SHA-256" onClick={() => void onCopy(file.sha256 || "")}>
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
  if (code === "CONTENT_TOO_LARGE") return "上传内容超过限制";
  if (code === "INVALID_FILE_PATH") return "文件路径不符合要求";
  if (code === "INVALID_CUSTOM_CODE") return "自定义 code 不可用";
  if (code === "RATE_LIMITED") return "发布过于频繁";
  return "发布失败";
}

function deployErrorHints(payload: StructuredAPIErrorPayload | null): string[] {
  const hints = new Set<string>();
  const code = String(payload?.errorCode || "").toUpperCase();
  const stage = String(payload?.stage || "").toLowerCase();
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
    hints.add("请压缩资源、删除无关文件，或让管理员调整单文件、文件数和整站大小上限。");
  }
  if (!hints.size) {
    hints.add("请检查标题、描述、code、入口文件和上传内容是否完整，再重新发布。");
  }
  return Array.from(hints);
}

function DeployErrorPanel({ message, error }: { message: string; error: StructuredAPIErrorPayload | null }) {
  const [copied, setCopied] = useState(false);
  const hints = deployErrorHints(error);
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
          <p>{error?.detail || message}</p>
        </div>
      </div>
      {error && (
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

function formatDate(value?: string): string {
  if (!value) return "-";
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return d.toLocaleDateString("zh-CN");
}

function formatDateTime(value?: string): string {
  if (!value) return "-";
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  const pad = (n: number) => String(n).padStart(2, "0");
  return d.getFullYear() + "-" + pad(d.getMonth() + 1) + "-" + pad(d.getDate()) + " " + pad(d.getHours()) + ":" + pad(d.getMinutes()) + ":" + pad(d.getSeconds());
}

function parseTagInput(value: string): string[] {
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

function formatDeviceInfo(info: unknown): string {
  if (!info) return "-";
  if (typeof info === "string") return info || "-";
  if (typeof info !== "object") return String(info);

  const data = info as Record<string, unknown>;
  const screenWidth = Number(data.screenWidthPx || data.widthPx || 0);
  const screenHeight = Number(data.screenHeightPx || data.heightPx || 0);
  const model = [data.manufacturer || data.brand, data.model].filter(Boolean).join(" ");
  const android = data.androidRelease || data.androidVersion || data.androidSdk;
  const resolution = screenWidth && screenHeight ? `${screenWidth} x ${screenHeight}` : data.resolution;
  const runtime = data.webViewRuntime || data.webView || data.x5Diagnostic || data.x5Version;
  const values = [model, android ? `Android ${android}` : "", resolution, data.orientation, runtime]
    .map((value) => String(value || "").trim())
    .filter(Boolean);

  if (values.length) return values.join(" / ");
  try {
    return JSON.stringify(info);
  } catch {
    return "-";
  }
}

function fileTextSize(text: string): number {
  return new Blob([text]).size;
}

function isTextUpload(name: string): boolean {
  return /\.(html?|css|js|mjs|json|txt|md|svg|xml|csv|webmanifest|map)$/i.test(name);
}

function isZipUpload(name: string): boolean {
  return /\.zip$/i.test(name);
}

function arrayBufferToBase64(buffer: ArrayBuffer): string {
  const bytes = new Uint8Array(buffer);
  let binary = "";
  for (let i = 0; i < bytes.length; i += 0x8000) {
    binary += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
  }
  return btoa(binary);
}

function currentOrigin(): string {
  return typeof location === "undefined" ? "https://pagepilot.example.com" : location.origin;
}

function currentBaseURL(): string {
  return currentOrigin().replace(/\/+$/, "");
}

function sameSiteURL(url?: string): string {
  if (!url) return "";
  if (url.startsWith("/")) return `${currentOrigin()}${url}`;
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

function skillDownloadPath(): string {
  return "/skill/pagep.zip";
}

function useRuntime() {
  const [config, setConfig] = useState<RuntimeConfig | null>(null);
  const [session, setSession] = useState<SessionInfo | null>(null);

  useEffect(() => {
    api<RuntimeConfig>("/api/config").then(setConfig).catch(() => setConfig(null));
    api<SessionInfo>("/api/admin/session").then(setSession).catch(() => setSession(null));
  }, []);

  return { config, session };
}

export function App() {
  const [page, setPage] = useState<Page>(() => getPageFromPath(location.pathname));
  const { config, session } = useRuntime();
  const [toast, setToast] = useState("");

  useEffect(() => {
    const onPop = () => setPage(getPageFromPath(location.pathname));
    window.addEventListener("popstate", onPop);
    return () => window.removeEventListener("popstate", onPop);
  }, []);

  useEffect(() => {
    let timer: number | undefined;
    const onToast = (event: Event) => {
      const detail = (event as CustomEvent<string>).detail;
      setToast(typeof detail === "string" && detail ? detail : "操作失败，请稍后重试。");
      if (timer) window.clearTimeout(timer);
      timer = window.setTimeout(() => setToast(""), 2600);
    };
    window.addEventListener("pagepilot-toast", onToast);
    return () => {
      if (timer) window.clearTimeout(timer);
      window.removeEventListener("pagepilot-toast", onToast);
    };
  }, []);

  const navigate = useCallback((next: Page, href: string) => {
    setPage(next);
    history.pushState({}, "", href);
    window.scrollTo({ top: 0, behavior: "smooth" });
  }, []);

  return (
    <div className="app-shell">
      <TopNav page={page} session={session} onNavigate={navigate} />
      <main className={`page-main ${page === "market" ? "market-main" : ""}`}>
        {page === "home" && <HomePage config={config} onNavigate={navigate} />}
        {page === "market" && <MarketPage config={config} session={session} />}
        {page === "deploy" && <DeployPage config={config} session={session} />}
        {page === "agents" && <AgentsPage config={config} />}
        {page === "screens" && <ScreensPage />}
      </main>
      {toast && <div className="page-toast" role="status">{toast}</div>}
      <Footer config={config} />
    </div>
  );
}

function TopNav({
  page,
  session,
  onNavigate
}: {
  page: Page;
  session: SessionInfo | null;
  onNavigate: (page: Page, href: string) => void;
}) {
  return (
    <header className="topbar">
      <div className="nav-inner">
        <a
          className="brand"
          href="/"
          onClick={(event) => {
            event.preventDefault();
            onNavigate("home", "/");
          }}
        >
          <Logo />
          <span>PagePilot</span>
        </a>
        <nav className="nav-links" aria-label="主导航">
          {navItems.map((item) => (
            <a
              key={item.page}
              className={page === item.page ? "active" : ""}
              href={item.href}
              onClick={(event) => {
                event.preventDefault();
                onNavigate(item.page, item.href);
              }}
            >
              {item.label}
            </a>
          ))}
        </nav>
        <div className="nav-actions">
          <a className="nav-button ghost" href="/agents/">
            <Bot size={16} />Agent
          </a>
          {session?.success ? (
            <a className="nav-button dark" href="/admin"><UserRound size={16} />用户中心</a>
          ) : (
            <a className="nav-button dark" href="/admin?mode=login"><UserRound size={16} />登录</a>
          )}
        </div>
      </div>
    </header>
  );
}

function Logo() {
  return (
    <img className="logo" src="/brand/pagepilot-logo.png" alt="" aria-hidden="true" />
  );
}

function HeroBarrage() {
  const lanes = [
    ["生成 HTML 应用", "发布 Markdown 文档", "上传 ZIP 站点", "访问密码保护", "版本回滚"],
    ["创作市场复用", "Agent 继续修改", "多文件静态站点", "复制 CLI 命令", "屏幕投放"],
    ["自定义 code", "公开 / 私有", "收藏与点赞", "源码下载", "MCP 接入"],
    ["隔离预览", "锁定版本", "分类筛选", "匿名发布", "用户中心"],
    ["PagePilot", "HTML", "Markdown", "ZIP", "Agent"],
    ["快速上线", "安全分享", "版本管理", "二次创作", "API"]
  ];

  return (
    <div className="hero-barrage" aria-hidden="true">
      {lanes.map((lane, index) => (
        <div
          className={`barrage-lane ${index % 2 ? "reverse" : ""}`}
          style={{ "--duration": `${52 + index * 5}s` } as React.CSSProperties}
          key={index}
        >
          {[...lane, ...lane, ...lane].map((item, itemIndex) => (
            <span className="barrage-chip" key={`${index}-${itemIndex}`}>{item}</span>
          ))}
        </div>
      ))}
    </div>
  );
}

function FeatureTile({ icon, title, desc }: { icon: React.ReactNode; title: string; desc: string }) {
  return (
    <div className="feature-tile">
      <span>{icon}</span>
      <strong>{title}</strong>
      <p>{desc}</p>
    </div>
  );
}

function FlowStep({ label, title, text }: { label: string; title: string; text: string }) {
  return (
    <div className="flow-step">
      <span>{label}</span>
      <strong>{title}</strong>
      <p>{text}</p>
    </div>
  );
}

function HeroAiCanvas() {
  const canvasRef = useRef<HTMLCanvasElement | null>(null);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const reduceMotion = window.matchMedia("(prefers-reduced-motion: reduce)");
    const context = canvas.getContext("2d");
    if (!context) return;
    const cnv = canvas;
    const ctx = context;

    let frame = 0;
    let width = 0;
    let height = 0;
    let raf = 0;
    const nodes = Array.from({ length: 34 }, (_, index) => ({
      baseX: (Math.sin(index * 2.17) * 0.5 + 0.5) * 0.92 + 0.04,
      baseY: (Math.cos(index * 1.73) * 0.5 + 0.5) * 0.82 + 0.08,
      drift: 0.6 + (index % 7) * 0.12,
      phase: index * 0.73,
      radius: 1.2 + (index % 4) * 0.35
    }));

    function resize() {
      const rect = cnv.getBoundingClientRect();
      const dpr = Math.min(window.devicePixelRatio || 1, 2);
      width = Math.max(1, rect.width);
      height = Math.max(1, rect.height);
      cnv.width = Math.floor(width * dpr);
      cnv.height = Math.floor(height * dpr);
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    }

    function draw() {
      frame += 0.008;
      ctx.clearRect(0, 0, width, height);
      const points = nodes.map((node) => {
        const x = node.baseX * width + Math.sin(frame * node.drift + node.phase) * 18;
        const y = node.baseY * height + Math.cos(frame * node.drift + node.phase) * 14;
        return { ...node, x, y };
      });

      points.forEach((point, index) => {
        const pulse = Math.sin(frame * 2.6 + point.phase) * 0.5 + 0.5;
        ctx.fillStyle = index % 5 === 0 ? `rgba(166, 135, 255, ${0.18 + pulse * 0.16})` : `rgba(70, 126, 255, ${0.12 + pulse * 0.10})`;
        ctx.beginPath();
        ctx.arc(point.x, point.y, point.radius + pulse * 1.2, 0, Math.PI * 2);
        ctx.fill();
      });

      if (!reduceMotion.matches) raf = window.requestAnimationFrame(draw);
    }

    resize();
    draw();
    window.addEventListener("resize", resize);
    return () => {
      window.cancelAnimationFrame(raf);
      window.removeEventListener("resize", resize);
    };
  }, []);

  return <canvas ref={canvasRef} className="hero-ai-canvas" aria-hidden="true" />;
}

function HomePage({
  config,
  onNavigate
}: {
  config: RuntimeConfig | null;
  onNavigate: (page: Page, href: string) => void;
}) {
  const homeRef = useRef<HTMLDivElement | null>(null);
  const [homePreviewTab, setHomePreviewTab] = useState<"agent" | "publish" | "result">("agent");
  const publishLimit = config?.anonymousPolicy?.deployLimit == null ? "-" : String(config.anonymousPolicy.deployLimit);
  const maxSize = formatSize(config?.limits?.maxSiteTotalBytes);
  const fileLimit = config?.limits?.maxFilesPerSite == null ? "-" : String(config.limits.maxFilesPerSite);
  const capabilities = [
    ["多格式应用发布", "HTML、Markdown、ZIP、多文件静态站点都能上线；Markdown 文档页也作为一等应用进入预览、市场和二次修改。"],
    ["Agent 原生交付", "pagep CLI、Skill、MCP 和 HTTP API 围绕同一套发布能力组织，让 Agent 生成后可以直接部署、更新和读取状态。"],
    ["访问与安全控制", "公开展示、私有内容、访问密码和隔离预览分层处理，避免用户页面影响后台或父页面上下文。"],
    ["版本治理", "每次修改都形成版本链，可查看历史、复制链接、锁定、回滚或下架，适合持续迭代的 Agent 应用。"],
    ["创作市场复用", "公开作品可被发现、预览、收藏、点赞、下载源码，也可以交给 Agent/MCP 作为下一次创作参考。"],
    ["广告屏投放", "把 PagePilot 应用投放到 Android 屏幕，支持远程刷新、截图回传和现场展示确认。"]
  ];
  const releaseNotes = [
    {
      version: "V0.2",
      title: "PagePilot 品牌、创作市场和多文件发布",
      date: "2026-06",
      body: "支持 HTML / Markdown / ZIP、多文件静态站点、分类、收藏、点赞、版本历史、广告屏菜单和 pagep Skill。"
    },
    {
      version: "V0.1",
      title: "从 Agent 发布 API 起步",
      date: "2026-05",
      body: "建立部署 API、访问链接、基础后台、Token 与匿名 Agent session，让 AI 生成页面可以直接上线。"
    }
  ];
  const previewTabs: Array<{ key: "agent" | "publish" | "result"; label: string; content: string; meta: string }> = [
    {
      key: "agent",
      label: "Agent 输出",
      content: "<main>\\n  <h1>新品活动页</h1>\\n  <section data-chart=\\\"budget\\\"></section>\\n</main>\\n\\n/assets/hero.png\\n/docs/README.md",
      meta: "Agent 生成 HTML、Markdown、图片和多文件目录"
    },
    {
      key: "publish",
      label: "PagePilot 发布",
      content: "pagep deploy ./dist \\\\\\n  --title \\\"新品活动页\\\" \\\\\\n  --description \\\"新品活动落地页\\\" \\\\\\n  --category landing \\\\\\n  --access-password optional",
      meta: "自动识别入口文件，记录版本、分类、访问策略"
    },
    {
      key: "result",
      label: "交付结果",
      content: "✓ 已生成可访问链接\\n✓ 已记录 v3 版本\\n✓ 已启用访问密码\\n✓ 可进入创作市场 / 广告屏投放",
      meta: "适合官网、看板、文档、活动页、屏幕展示和 Agent 二次创作"
    }
  ];
  const activePreview = previewTabs.find((item) => item.key === homePreviewTab) || previewTabs[0];

  useEffect(() => {
    const root = homeRef.current;
    if (!root) return;
    const homeRoot = root;
    const reduceMotion = window.matchMedia("(prefers-reduced-motion: reduce)");
    let locked = false;

    function onWheel(event: WheelEvent) {
      if (event.deltaY <= 0 || Math.abs(event.deltaY) < 18 || reduceMotion.matches || locked) return;
      const screens = Array.from(homeRoot.querySelectorAll<HTMLElement>("[data-home-screen]"));
      const viewport = window.innerHeight;
      const current = screens.find((screen) => {
        const rect = screen.getBoundingClientRect();
        return rect.top <= 96 && rect.bottom > 140;
      });
      if (current) {
        const rect = current.getBoundingClientRect();
        const hasUnreadLongContent = rect.height > viewport + 140 && rect.top < 80 && rect.bottom > viewport + 160;
        if (hasUnreadLongContent) return;
      }
      const next = screens.find((screen) => screen.getBoundingClientRect().top > 120);
      if (!next) return;
      event.preventDefault();
      locked = true;
      next.scrollIntoView({ behavior: "smooth", block: "start" });
      window.setTimeout(() => { locked = false; }, 760);
    }

    window.addEventListener("wheel", onWheel, { passive: false });
    return () => window.removeEventListener("wheel", onWheel);
  }, []);

  return (
    <div className="home-page ant-home" ref={homeRef}>
      <section className="ant-hero-section" data-home-screen>
        <HeroAiCanvas />
        <div className="ant-hero-copy">
          <div className="ant-home-tags">
            {['HTML', 'Markdown', 'ZIP', 'Agent', 'MCP', 'Screen'].map((item) => <Tag key={item}>{item}</Tag>)}
          </div>
          <h1 className="hero-brand-word">
            <Logo />
            <span><em>Page</em><b>Pilot</b></span>
          </h1>
          <h2>Agent 生成应用的发布控制台</h2>
          <p>让 Agent 负责生成，让 PagePilot 负责上线、加密、版本、市场复用和广告屏投放。它不是一个 HTML 托管页，而是一套 AI 时代的应用交付工作流。</p>
          <div className="ant-hero-actions">
            <Button type="primary" size="large" icon={<Bot size={18} />} onClick={() => onNavigate('agents', '/agents/')}>交给 Agent 部署</Button>
            <Button size="large" icon={<Upload size={18} />} onClick={() => onNavigate('deploy', '/deploy')}>手动发布</Button>
          </div>
          <div className="hero-proof-row" aria-label="PagePilot 核心能力">
            <span><Rocket size={16} />秒级上线</span>
            <span><Lock size={16} />访问加密</span>
            <span><Layers size={16} />版本回滚</span>
            <span><PackageOpen size={16} />市场复用</span>
          </div>
        </div>
        <div className="hero-flow-motion" aria-label="Agent 生成并发布到 PagePilot">
          <div className="flow-core"><Logo /><strong>PagePilot</strong></div>
          <div className="flow-agent"><Bot size={22} /><span>Agent</span></div>
          <div className="flow-output flow-html"><FileCode2 size={18} /><span>HTML</span></div>
          <div className="flow-output flow-md"><FileText size={18} /><span>Markdown</span></div>
          <div className="flow-output flow-zip"><FileArchive size={18} /><span>ZIP</span></div>
          <div className="flow-path flow-path-a" />
          <div className="flow-path flow-path-b" />
          <div className="flow-path flow-path-c" />
          <div className="flow-launch"><Rocket size={18} /><span>生成可访问链接</span></div>
          <div className="flow-feature flow-secure"><Lock size={15} /><span>访问密码</span></div>
          <div className="flow-feature flow-version"><Layers size={15} /><span>版本管理</span></div>
          <div className="flow-feature flow-market"><PackageOpen size={15} /><span>进入创作市场</span></div>
          <div className="flow-feature flow-screen"><Monitor size={15} /><span>广告屏投放</span></div>
        </div>
        <div className="hero-scroll-cue" aria-hidden="true"><span>继续了解 PagePilot</span><i /></div>
      </section>

      <section className="home-screen home-second-screen" data-home-screen>
        <div className="ant-section ant-instant-section">
          <div className="ant-section-copy">
            <span>TRY IT NOW</span>
            <h2>一次发布，补齐应用交付闭环</h2>
            <p>PagePilot 把 Agent 生成的文件变成可访问、可治理、可复用、可投放的应用资产。你只负责说清需求，剩下的交给发布链路。</p>
            <div className="delivery-chain" aria-label="PagePilot 发布链路">
              <div><em>01</em><strong>接收产物</strong><span>HTML / Markdown / ZIP / 多文件站点</span></div>
              <div><em>02</em><strong>发布治理</strong><span>访问密码、分类、版本、锁定和下架</span></div>
              <div><em>03</em><strong>交付复用</strong><span>创作市场、源码下载、Agent 二次创作、广告屏投放</span></div>
            </div>
          </div>
          <div className="ant-live-editor" aria-label="PagePilot 快速发布预览">
            <div className="editor-tabs" role="tablist" aria-label="发布预览步骤">
              {previewTabs.map((tab) => (
                <button
                  key={tab.key}
                  type="button"
                  role="tab"
                  aria-selected={homePreviewTab === tab.key}
                  className={homePreviewTab === tab.key ? "active" : ""}
                  onClick={() => setHomePreviewTab(tab.key)}
                >
                  {tab.label}
                </button>
              ))}
            </div>
            <pre>{activePreview.content}</pre>
            <div className="preview-result-bar">
              <span>{activePreview.meta}</span>
              <Button type="link" onClick={() => onNavigate('deploy', '/deploy')}>立即发布</Button>
            </div>
          </div>
        </div>

        <div className="second-feature-strip" aria-label="PagePilot 核心能力">
          <span>FEATURES</span>
          <div>
            {capabilities.map(([title], index) => <strong key={title}><em>{String(index + 1).padStart(2, '0')}</em>{title}</strong>)}
          </div>
        </div>
      </section>

      <section className="home-screen home-third-screen" data-home-screen>
        <div className="ant-section ant-market-section">
          <div className="ant-section-copy">
            <span>CREATION MARKET</span>
            <h2>把一次性交付变成可复用的应用资产</h2>
            <p>创作市场不是模板橱窗，而是 Agent 继续工作的上下文。公开作品可以被发现、收藏、下载源码、复制 CLI，或者作为下一次生成的参考。</p>
            <Button icon={<PackageOpen size={17} />} onClick={() => onNavigate('market', '/market')}>浏览创作市场</Button>
          </div>
          <div className="market-scenario-list">
            {['产品官网', '数据看板', '文档手册', '活动落地页', '屏幕展示', '效率工具'].map((item) => <button key={item}>{item}</button>)}
          </div>
        </div>

        <div className="ant-section ant-workflow-section">
          <div className="ant-section-copy wide"><span>HOW IT WORKS</span><h2>三步，从 Agent 产物到可访问应用</h2></div>
          <Steps className="ant-home-steps" items={[
            { title: '生成或上传', description: 'Agent 生成 HTML、Markdown、ZIP，或你手动上传目录与文件。' },
            { title: '发布与治理', description: 'PagePilot 生成访问地址，记录版本、权限、分类和市场状态。' },
            { title: '分享与复用', description: '复制链接、投放屏幕、下载源码，或让 Agent 基于旧作品继续迭代。' }
          ]} />
          <div className="ant-stat-line"><div><strong>{publishLimit}</strong><span>匿名额度</span></div><div><strong>{maxSize}</strong><span>整站上限</span></div><div><strong>{fileLimit}</strong><span>文件上限</span></div><div><strong>pagep</strong><span>CLI / Skill</span></div></div>
        </div>
      </section>

      <section className="home-screen home-fourth-screen" data-home-screen>
        <div className="ant-section ant-version-section">
          <div className="ant-section-copy"><span>VERSION HISTORY</span><h2>持续进化的产品能力</h2><p>每次迭代都围绕 Agent 生成应用的真实交付链路展开。后续版本会继续追加到这里，形成可回顾的产品路线。</p></div>
          <Timeline items={releaseNotes.map((note, index) => ({ color: index === 0 ? '#575ff5' : '#001541', children: <><strong>{note.version} · {note.title}</strong><p><span>{note.date}</span>{note.body}</p></> }))} />
        </div>

        <section className="ant-philosophy-section">
          <div><strong>AI-Native</strong><span>让 Agent 成为内容生产者，PagePilot 成为交付系统。</span></div>
          <div><strong>可治理</strong><span>版本、权限、密码、下架、分类和审计是发布平台的核心。</span></div>
          <div><strong>可复用</strong><span>每个公开作品都可以成为下一次生成和二次创作的上下文。</span></div>
        </section>
      </section>
    </div>
  );
}

function MarketPage({ config, session }: { config: RuntimeConfig | null; session: SessionInfo | null }) {
  const pageSize = 24;
  const [items, setItems] = useState<MarketplaceDeploy[]>([]);
  const [selected, setSelected] = useState<MarketplaceDeploy | null>(null);
  const [detail, setDetail] = useState<MarketplaceDeploy | null>(null);
  const [detailKey, setDetailKey] = useState(() => getMarketDetailKeyFromPath(location.pathname));
  const [detailLoading, setDetailLoading] = useState(false);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [query, setQuery] = useState("");
  const [sort, setSort] = useState<SortMode>("hot");
  const [category, setCategory] = useState<MarketCategory>("all");
  const [kind, setKind] = useState<MarketKind>("all");
  const [marketCategories, setMarketCategories] = useState<MarketCategoryInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const listScrollRef = useRef(0);
  const hasMore = items.length < total;
  const pinnedCount = items.filter((item) => item.isPinned).length;
  const categories: Array<{ key: MarketCategory; label: string; note: string; icon: React.ReactNode }> = [
    { key: "all", label: "全部分类", note: "不过滤应用分类", icon: <PackageOpen /> },
    ...marketCategories.map((item) => ({
      key: item.slug,
      label: item.label,
      note: item.note || "管理员维护的市场分类",
      icon: categoryIcon(item.slug)
    }))
  ];
  const kindFilters: Array<{ key: MarketKind; label: string; note: string; icon: React.ReactNode }> = [
    { key: "all", label: "全部类型", note: "不过滤内容形态", icon: <PackageOpen /> },
    { key: "html", label: "HTML", note: "页面和静态站点", icon: <FileCode2 /> },
    { key: "md", label: "Markdown", note: "文档页优先", icon: <FileText /> },
    { key: "protected", label: "加密网页", note: "需要访问密码", icon: <Lock /> },
    { key: "featured", label: "精选优先", note: "管理员推荐", icon: <Sparkles /> },
    { key: "mine", label: "我的发布", note: "当前账号或浏览器", icon: <UserRound /> },
    { key: "favorites", label: "我的收藏", note: "账号或浏览器收藏", icon: <Bookmark /> }
  ];
  const sortTabs: Array<{ key: SortMode; label: string }> = [
    { key: "hot", label: "热门优先" },
    { key: "newest", label: "最新发布" },
    { key: "featured", label: "精选优先" }
  ];
  const selectedCategoryLabel = categories.find((item) => item.key === category)?.label || "全部分类";
  const selectedKindLabel = kindFilters.find((item) => item.key === kind)?.label || "全部类型";
  const feedScope = category === "all" && kind === "all"
    ? "全部分类"
    : [kind !== "all" ? selectedKindLabel : "", category !== "all" ? selectedCategoryLabel : ""].filter(Boolean).join(" / ") || "全部分类";
  const feedSummary = loading
    ? feedScope + " · 正在加载作品"
    : feedScope + " · 共 " + total + " 个作品" + (pinnedCount ? " · 本页 " + pinnedCount + " 个精选" : "") + " · 可预览、复用和交给 Agent 二次创作";

  const load = useCallback(async (nextPage = 1, append = false) => {
    if (append) {
      setLoadingMore(true);
    } else {
      setLoading(true);
    }
    try {
      const params = new URLSearchParams({
        page: String(nextPage),
        pageSize: String(pageSize),
        sort
      });
      if (query.trim()) params.set("q", query.trim());
      if (category !== "all") params.set("category", category);
      if (kind !== "all") params.set("kind", kind);
      const data = await api<MarketplaceResponse>(`/api/deploys?${params.toString()}`);
      setItems((current) => append ? [...current, ...(data.deploys || [])] : (data.deploys || []));
      setTotal(data.total || 0);
      setPage(nextPage);
    } catch (err) {
      if (err instanceof APIError && err.status === 401 && (kind === "mine" || kind === "favorites")) {
        window.dispatchEvent(new CustomEvent("pagepilot-toast", {
          detail: kind === "mine"
            ? "请先登录后查看账号发布；匿名发布只能在当前浏览器会话未失效时识别。"
            : "请先登录后查看收藏，收藏会同步到用户中心。"
        }));
        setKind("all");
        setItems([]);
        setTotal(0);
        setPage(1);
        return;
      }
      throw err;
    } finally {
      if (append) {
        setLoadingMore(false);
      } else {
        setLoading(false);
      }
    }
  }, [query, sort, category, kind]);

  const refresh = useCallback(() => {
    void load(1, false);
  }, [load]);

  const closeDetail = useCallback((restoreScroll = false) => {
    setDetailKey("");
    setDetail(null);
    setDetailLoading(false);
    if (location.pathname.startsWith("/market/")) {
      history.pushState({}, "", "/market");
    }
    if (restoreScroll) {
      window.setTimeout(() => {
        window.scrollTo({ top: listScrollRef.current, behavior: "auto" });
      }, 0);
    }
  }, []);

  const openDetail = useCallback((item: MarketplaceDeploy) => {
    listScrollRef.current = window.scrollY;
    setDetail(item);
    const key = item.publicId || item.id || item.code;
    setDetailKey(key);
    const href = `/market/${encodeURIComponent(key)}`;
    if (location.pathname !== href) {
      history.pushState({}, "", href);
    }
  }, []);

  const backToList = useCallback(() => {
    closeDetail(true);
  }, [closeDetail]);

  useEffect(() => {
    const onPop = () => setDetailKey(getMarketDetailKeyFromPath(location.pathname));
    window.addEventListener("popstate", onPop);
    return () => window.removeEventListener("popstate", onPop);
  }, []);

  useEffect(() => {
    if (!detailKey) {
      setDetail(null);
      setDetailLoading(false);
      return;
    }
    let cancelled = false;
    setDetailLoading(true);
    api<MarketplaceDeploy>(`/api/deploys/${encodeURIComponent(detailKey)}`)
      .then((data) => {
        if (!cancelled) setDetail(data);
      })
      .catch(() => {
        if (!cancelled) setDetail(null);
      })
      .finally(() => {
        if (!cancelled) setDetailLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [detailKey]);

  useEffect(() => {
    api<MarketCategoriesResponse>("/api/market/categories")
      .then((data) => setMarketCategories(data.categories || []))
      .catch(() => setMarketCategories([]));
  }, []);

  useEffect(() => {
    const timer = window.setTimeout(() => {
      void load(1, false);
    }, 160);
    return () => window.clearTimeout(timer);
  }, [load]);

  return (
    <section className="market-page">
      <section className="market-workspace">
        <aside className="market-sidebar" aria-label="创作市场分类">
          <div className="market-sidebar-head">
            <strong>内容筛选</strong>
            <span>{loading ? "加载中" : `${total} 个结果`}</span>
          </div>
          <nav className="market-category-nav">
            {kindFilters.map((item) => (
              <button
                key={item.key}
                className={kind === item.key ? "active" : ""}
                type="button"
                onClick={() => {
                  setPage(1);
                  setKind(item.key);
                  closeDetail();
                }}
              >
                <span className="market-category-icon">{item.icon}</span>
                <span>
                  <strong>{item.label}</strong>
                  <em>{item.note}</em>
                </span>
              </button>
            ))}
          </nav>
          <div className="market-sidebar-head compact">
            <strong>应用分类</strong>
          </div>
          <nav className="market-category-nav compact">
            {categories.map((item) => (
              <button
                key={item.key}
                className={category === item.key ? "active" : ""}
                type="button"
                onClick={() => {
                  setPage(1);
                  setCategory(item.key);
                  closeDetail();
                }}
              >
                <span className="market-category-icon">{item.icon}</span>
                <span>
                  <strong>{item.label}</strong>
                  <em>{item.note}</em>
                </span>
              </button>
            ))}
          </nav>
          <div className="market-sidebar-note">
            <strong>提示</strong>
            <span>加密作品可以凭访问密码浏览；源码下载和模板复用会被禁用。</span>
          </div>
        </aside>

        <div className="market-content">
          {!detail && (
            <div className="market-command-center">
              {kind === "all" && (
                <div className="market-command-copy">
                  <span className="mini-label">CREATION MARKET</span>
                  <strong>复用好作品，而不是重新发明每个页面</strong>
                  <p>下载源文件、复制 CLI，或交给 Agent/MCP 继续创作。公开作品可预览、收藏和点赞。</p>
                </div>
              )}
              <div className="market-command-row">
                <label className="market-search">
                  <Search size={17} />
                  <input
                    value={query}
                    onChange={(event) => {
                      setPage(1);
                      setQuery(event.target.value);
                    }}
                    placeholder="搜索标题、描述或 code"
                  />
                </label>
                <div className="market-sort-tabs" aria-label="排序">
                  {sortTabs.map((item) => (
                    <button
                      key={item.key}
                      className={sort === item.key ? "active" : ""}
                      type="button"
                      onClick={() => {
                        setPage(1);
                        setSort(item.key);
                      }}
                    >
                      {item.label}
                    </button>
                  ))}
                </div>
                <select
                  className="market-sort-select"
                  value={sort}
                  onChange={(event) => {
                    setPage(1);
                    setSort(event.target.value as SortMode);
                  }}
                >
                  <option value="hot">热门优先</option>
                  <option value="newest">最新发布</option>
                  <option value="featured">精选优先</option>
                  <option value="likes_desc">点赞最多</option>
                  <option value="views_desc">访问最多</option>
                  <option value="oldest">最早发布</option>
                </select>
              </div>
            </div>
          )}

          {detail ? (
            <MarketDetailViewFull
              item={detail}
              loading={detailLoading}
              session={session}
              onBack={backToList}
              onUse={setSelected}
            />
          ) : (
            <>
              <div className="market-feed-head">
                <div>
                  <h2 className="market-feed-titleline">{feedSummary}</h2>
                </div>
                <div className="market-feed-meta">整站上限 {formatSize(config?.limits?.maxSiteTotalBytes)}</div>
              </div>

              {loading && !items.length ? (
                <div className="market-card-grid">{Array.from({ length: 8 }).map((_, index) => <div className="market-card-skeleton" key={index} />)}</div>
              ) : (
                <div className="market-card-grid">
                  {items.map((item) => (
                    <MarketplaceCard
                      key={item.code}
                      item={item}
                      session={session}
                      onChanged={refresh}
                      onUse={setSelected}
                      onDetail={openDetail}
                    />
                  ))}
                  {!items.length && (
                    <div className="empty-wide">
                      {kind === "mine"
                        ? "当前账号或本浏览器匿名身份下还没有发布记录。你可以先登录查看账号作品，或发布一个新页面。"
                        : kind === "favorites"
                          ? "当前账号或本浏览器匿名身份下还没有收藏。登录后可跨设备查看收藏。"
                        : "还没有找到作品。换个分类或关键词试试，也可以先让 Agent 发布一个新页面。"}
                    </div>
                  )}
                </div>
              )}

              {!loading && total > 0 && (
                <div className="market-load-more" aria-label="创作市场加载更多">
                  <div className="page-note">
                    精选作品会优先展示，其他作品继续按当前排序排列。已显示 {items.length} / {total} 个作品。
                  </div>
                  {hasMore && (
                    <button className="button compact" type="button" disabled={loadingMore} onClick={() => void load(page + 1, true)}>
                      {loadingMore ? "加载中" : "加载更多"}
                    </button>
                  )}
                </div>
              )}
            </>
          )}
        </div>
      </section>

      {selected && <MarketUseDrawer item={selected} onClose={() => setSelected(null)} />}
    </section>
  );
}

function MarketplaceCard({
  item,
  session,
  onChanged,
  onUse,
  onDetail
}: {
  item: MarketplaceDeploy;
  session: SessionInfo | null;
  onChanged: () => void;
  onUse: (item: MarketplaceDeploy) => void;
  onDetail: (item: MarketplaceDeploy) => void;
}) {
  const previewRef = useRef<HTMLIFrameElement | null>(null);
  const cardRef = useRef<HTMLElement | null>(null);
  const [previewStatus, setPreviewStatus] = useState<PreviewStatus>("idle");
  const title = item.title || item.code || "未命名作品";
  const appURL = sameSiteURL(`/agent/${encodeURIComponent(item.code)}/`);
  const isLocked = Boolean(item.accessProtected);
  const displayTags = (item.tags || []).slice(0, 3);
  const heat = getMarketHeat(item);

  useEffect(() => {
    const iframe = previewRef.current;
    const node = cardRef.current;
    if (!iframe || !node || isLocked) return;

    let cancelQueued = false;
    const loadHandler = () => {
      setPreviewStatus("loaded");
    };

    const observer = new IntersectionObserver((entries) => {
      if (!entries.some((entry) => entry.isIntersecting) || iframe.src || cancelQueued) return;
      setPreviewStatus("loading");
      iframe.addEventListener("load", loadHandler, { once: true });
      iframe.src = `${appURL}?preview=1`;
    }, { rootMargin: "0px 0px", threshold: 0.08 });

    observer.observe(node);
    return () => {
      cancelQueued = true;
      iframe.removeEventListener("load", loadHandler);
      observer.disconnect();
    };
  }, [appURL, isLocked]);

  const like = async () => {
    try {
      await api(`/api/deploys/${encodeURIComponent(item.code)}/like`, { method: "POST" });
      onChanged();
    } catch {
      // 点赞失败不打断浏览。
    }
  };

  const toggleFavorite = async () => {
    if (!session?.success && !session?.userId) {
      window.dispatchEvent(new CustomEvent("pagepilot-toast", { detail: "请先登录后再收藏作品，登录后可在用户中心查看收藏。" }));
      return;
    }
    try {
      await api(`/api/deploys/${encodeURIComponent(item.code)}/favorite`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ favorited: !item.favorited })
      });
      onChanged();
    } catch {
      window.dispatchEvent(new CustomEvent("pagepilot-toast", { detail: "收藏失败，请确认已登录后重试。" }));
    }
  };

  const deleteSite = async () => {
    if (!window.confirm(`确认删除「${title}」？删除后站点和所有版本都会移除。`)) return;
    try {
      await api(`/api/admin/sites/${encodeURIComponent(item.code)}`, { method: "DELETE" });
      onChanged();
    } catch (err) {
      window.alert(err instanceof Error ? err.message : "删除失败，请确认你有权限删除这个作品。");
    }
  };

  return (
    <article className="app-card" ref={cardRef}>
      <div className={`preview-pane state-${previewStatus} ${isLocked ? "is-locked" : ""}`}>
        {isLocked ? (
          <div className="locked-preview">
            <Lock size={28} />
            <strong>已加密</strong>
            <span>输入访问密码后才能查看内容</span>
          </div>
        ) : (
          <iframe
            ref={previewRef}
            title={`${title} 预览`}
            loading="lazy"
            scrolling="no"
            sandbox={PREVIEW_IFRAME_SANDBOX}
          />
        )}
        <div className="card-quickbar" aria-label="作品数据">
          <span title="访问量"><Eye size={13} />{compactNumber(item.viewCount || 0)}</span>
          <span title="复用次数"><Workflow size={13} />{compactNumber(item.reuseCount || 0)}</span>
          <span title="版本数"><Layers size={13} />v{item.versionCount || 1}</span>
        </div>
        <div className="card-hover-actions">
          <button className="button compact dark" type="button" onClick={() => onDetail(item)}>查看详情</button>
          <button className="button compact primary" type="button" onClick={() => onUse(item)}>使用模板</button>
          <button className="quick-action" type="button" aria-label="点赞" onClick={like}>
            <Heart size={15} /><span>{compactNumber(item.likeCount || 0)}</span>
          </button>
          <button className={`quick-action ${item.favorited ? "active" : ""}`} type="button" aria-label="收藏" onClick={toggleFavorite}>
            <Bookmark size={15} /><span>{compactNumber(item.favoriteCount || 0)}</span>
          </button>
          <a className="quick-action" href={appURL} target="_blank" rel="noreferrer" aria-label="打开应用">
            <ExternalLink size={15} /><span>打开</span>
          </a>
          {item.canManage && (
            <button className="quick-action danger" type="button" onClick={() => void deleteSite()}>
              <Trash2 size={15} /><span>删除</span>
            </button>
          )}
        </div>
      </div>
      <div className="card-body">
        <div className="market-card-tags compact">
          <span className="scene">{marketCategoryLabel(item.category)}</span>
          {item.isPinned && <span className="featured">精选</span>}
          <span className="type">{marketFileType(item)}</span>
        </div>
        <div className="card-title-row">
          <div className="title-wrap">
            <h3>{title}</h3>
          </div>
          {heat && <span className={`ct-heat ${heat.level}`}>{heat.label}</span>}
        </div>
        <p>{item.description || "这个作品还没有描述。"}</p>
        <div className="market-card-tags user-tags">
          {displayTags.map((tag) => <span className="user-tag" key={tag}>#{tag}</span>)}
        </div>
        <div className="market-card-footer">
          <span>{item.owned ? "我的发布" : "PagePilot 创作者"}</span>
          <time>{formatDateTime(item.updatedAt || item.createdAt)}</time>
        </div>
      </div>
    </article>
  );
}

function compactNumber(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return "0";
  if (value >= 10000) return `${(value / 10000).toFixed(value >= 100000 ? 0 : 1)}w`;
  if (value >= 1000) return `${(value / 1000).toFixed(value >= 10000 ? 0 : 1)}k`;
  return String(value);
}

function MarketDetailViewFull({
  item,
  loading,
  session,
  onBack,
  onUse
}: {
  item: MarketplaceDeploy;
  loading: boolean;
  session: SessionInfo | null;
  onBack: () => void;
  onUse: (item: MarketplaceDeploy) => void;
}) {
  const appURL = sameSiteURL(`/agent/${encodeURIComponent(item.code)}/`);
  const isLocked = Boolean(item.accessProtected);
  const canDownload = Boolean(item.reuse?.allowDownload && item.reuse?.downloadUrl);
  const canReuse = item.reuse?.allowReuse !== false;
  const downloadUrl = item.reuse?.downloadUrl || `/api/deploy/content?code=${encodeURIComponent(item.code)}&download=1`;
  const canManage = Boolean(item.canManage || item.owned || session?.isAdmin);
  const [versions, setVersions] = useState<VersionItem[]>([]);
  const [viewport, setViewport] = useState<PreviewViewport>("desktop");
  const [busyVersion, setBusyVersion] = useState<number | null>(null);
  const [showAllVersions, setShowAllVersions] = useState(false);
  const displayTags = (item.tags || []).slice(0, 6);
  const detailFiles = item.files || [];
  const treeItems = item.bundle?.tree || [];
  const displayTree = treeItems.length ? treeItems : detailFiles;
  const mcpText = item.reuse?.mcp ? JSON.stringify(item.reuse.mcp, null, 2) : "";

  const loadVersions = useCallback(async () => {
    const data = await api<VersionsResponse>(`/api/deploys/${encodeURIComponent(item.code)}/versions`);
    setVersions((data.versions || []).slice().sort((a, b) => Number(b.versionNumber) - Number(a.versionNumber)));
  }, [item.code]);

  useEffect(() => {
    loadVersions().catch(() => setVersions([]));
  }, [loadVersions]);

  const copyText = async (text: string) => {
    await navigator.clipboard.writeText(text);
  };

  const setCurrentVersion = async (version: number) => {
    setBusyVersion(version);
    try {
      await api(`/api/deploys/${encodeURIComponent(item.code)}/current`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ versionNumber: version })
      });
      await loadVersions();
    } finally {
      setBusyVersion(null);
    }
  };

  const toggleVersionLock = async (version: VersionItem) => {
    setBusyVersion(version.versionNumber);
    try {
      await api(`/api/deploys/${encodeURIComponent(item.code)}/versions/${version.versionNumber}/lock`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ locked: !version.isLocked })
      });
      await loadVersions();
    } finally {
      setBusyVersion(null);
    }
  };

  const deleteVersion = async (version: VersionItem) => {
    if (!window.confirm(`确认删除 v${version.versionNumber}？`)) return;
    setBusyVersion(version.versionNumber);
    try {
      await api(`/api/deploys/${encodeURIComponent(item.code)}/versions/${version.versionNumber}`, { method: "DELETE" });
      await loadVersions();
    } finally {
      setBusyVersion(null);
    }
  };

  const currentVersion = versions.find((version) => version.isCurrent) || versions[0];
  const visibleVersions = showAllVersions ? versions : versions.slice(0, 2);
  const updateURL = `/deploy?code=${encodeURIComponent(item.code)}&version=1`;

  return (
    <section className="market-detail-layout">
      <div className="market-detail-stage">
        <div className="detail-preview-frame">
          <div className="detail-preview-toolbar">
            <div className="detail-toolbar-left">
              <button className="button compact detail-back-button" type="button" onClick={onBack}><ChevronLeft size={15} />返回列表</button>
              <div>
                <strong>v{currentVersion?.versionNumber || item.versionCount || 1} 预览</strong>
                <span>{isLocked ? "已加密，打开后输入访问密码" : "桌面、平板、手机三端查看"}</span>
              </div>
            </div>
            <div className="viewport-tabs" role="group" aria-label="预览尺寸">
              <button className={viewport === "desktop" ? "active" : ""} type="button" title="桌面端" onClick={() => setViewport("desktop")}><Monitor size={15} /></button>
              <button className={viewport === "tablet" ? "active" : ""} type="button" title="平板端" onClick={() => setViewport("tablet")}><Tablet size={15} /></button>
              <button className={viewport === "mobile" ? "active" : ""} type="button" title="手机端" onClick={() => setViewport("mobile")}><Smartphone size={15} /></button>
            </div>
          </div>
          <div className={`market-detail-preview viewport-${viewport} ${isLocked ? "is-locked" : ""}`}>
            {isLocked ? (
              <div className="locked-preview">
                <Lock size={36} />
                <strong>网页已加密</strong>
                <span>打开应用并输入访问密码后才能查看内容。</span>
              </div>
            ) : (
              <iframe src={`${appURL}?preview=1`} title={`${item.title || item.code} 预览`} sandbox={PREVIEW_IFRAME_SANDBOX} />
            )}
          </div>
        </div>
      </div>
      <aside className="market-detail-card">
        <div>
          <span className="mini-label">{loading ? "LOADING" : "MARKET DETAIL"}</span>
          <h2>{item.title || item.code}</h2>
          <p>{item.description || "这个作品还没有描述。"}</p>
        </div>
        <div className="detail-identity">
          <button className="detail-code-box" type="button" onClick={() => void copyText(item.code)} title="复制 code">
            <span>应用 code</span>
            <strong>{item.code}</strong>
          </button>
          <div className="detail-state-row">
            <span>{item.visibility === "public" ? "公开展示" : "仅链接访问"}</span>
            {isLocked && <span>访问密码</span>}
            {item.owned && <span>我的发布</span>}
            {item.isPinned && <span>精选</span>}
          </div>
          {!!displayTags.length && (
            <div className="detail-tag-row">
              {displayTags.map((tag) => <span key={tag}>#{tag}</span>)}
            </div>
          )}
        </div>
        <div className="market-detail-meta compact">
          <span><strong>{marketCategoryLabel(item.category)}</strong><em>分类</em></span>
          <span><strong>{marketFileType(item)}</strong><em>类型</em></span>
          <span><strong>{item.likeCount || 0}</strong><em>点赞</em></span>
          <span><strong>{item.viewCount || 0}</strong><em>访问</em></span>
          <span><strong>{item.reuseCount || 0}</strong><em>复用</em></span>
          <span><strong>{item.versionCount || versions.length || 1}</strong><em>版本</em></span>
          <span><strong>{formatDate(item.updatedAt || item.createdAt)}</strong><em>更新</em></span>
        </div>
        {item.bundle && (
          <div className="detail-info-block">
            <div className="detail-info-head"><FileArchive size={15} /><strong>Bundle 信息</strong></div>
            <div className="detail-info-grid">
              <span><em>类型</em><strong>{item.bundle.kindLabel || item.bundle.kind || "-"}</strong></span>
              <span><em>入口</em><strong>{item.bundle.mainEntry || item.filename || "-"}</strong></span>
              {item.bundle.root && <span><em>根目录</em><strong>{item.bundle.root}</strong></span>}
              <span><em>安全模式</em><strong>{securityModeLabel(item.bundle.effectiveSecurityMode || item.bundle.securityMode)}</strong></span>
              <span><em>文件</em><strong>{item.bundle.fileCount || detailFiles.length || 0} 个 · {formatSize(item.bundle.totalSize || item.fileSize || 0)}</strong></span>
            </div>
            {item.bundle.entryNote && <p>{item.bundle.entryNote}</p>}
          </div>
        )}
        {!!displayTree.length && (
          <div className="detail-info-block">
            <div className="detail-info-head"><FileText size={15} /><strong>完整文件树</strong></div>
            <FileTreeExplorer
              files={displayTree}
              totalSize={item.bundle?.totalSize || item.fileSize}
              onCopy={copyText}
            />
          </div>
        )}
        {item.reuse && (
          <div className="detail-info-block">
            <div className="detail-info-head"><Workflow size={15} /><strong>复用参数</strong></div>
            {item.templateSourceCode && (
              <p>这个作品基于 {item.templateSourceCode} v{item.templateSourceVersion || "-"} 二次创作。</p>
            )}
            {item.reuse.policyNote && <p>{item.reuse.policyNote}</p>}
            <div className="detail-copy-stack">
              {item.reuse.agentPrompt && <button className="button compact full" type="button" onClick={() => void copyText(item.reuse?.agentPrompt || "")}><Copy size={13} />复制 Agent 提示词</button>}
              {item.reuse.cli && <button className="button compact full" type="button" onClick={() => void copyText(item.reuse?.cli || "")}><Code2 size={13} />复制 CLI 命令</button>}
              {mcpText && <button className="button compact full" type="button" onClick={() => void copyText(mcpText)}><PackageOpen size={13} />复制 MCP 参数</button>}
              {!item.reuse.allowReuse && <span className="muted-line">当前作品不开放模板复用。</span>}
            </div>
          </div>
        )}
        <div className="market-detail-actions">
          <button className="button primary full" type="button" disabled={!canReuse} onClick={() => onUse(item)}><Workflow size={16} />{canReuse ? "使用此模板" : "模板复用受限"}</button>
          <a className="button full" href={appURL} target="_blank" rel="noreferrer"><ExternalLink size={16} />打开应用</a>
          <button className="button full" type="button" onClick={() => void copyText(appURL)}><Copy size={16} />复制链接</button>
          {canManage && <a className="button full" href={updateURL}><Upload size={16} />更新版本</a>}
          {canDownload ? (
            <a className="button full" href={downloadUrl}><Download size={16} />下载源文件</a>
          ) : (
            <button className="button full" type="button" disabled><Download size={16} />源码下载受限</button>
          )}
        </div>
        <div className="detail-qr-block">
          <img src={`/api/deploys/${encodeURIComponent(item.code)}/qr`} alt={`${item.code} 二维码`} />
          <span>扫码打开当前应用</span>
        </div>
        <div className="detail-version-list">
          <div className="detail-version-head">
            <strong>版本历史</strong>
            <span>{versions.length} 个版本</span>
          </div>
          {visibleVersions.map((version) => {
            const versionURL = sameSiteURL(`/agent/${encodeURIComponent(item.code)}/versions/${version.versionNumber}/`);
            return (
              <div className={`detail-version-item ${version.isCurrent ? "current" : ""}`} key={version.versionNumber}>
                <div>
                  <strong>v{version.versionNumber}</strong>
                  <span>
                    {version.isCurrent && <em className="badge green">当前</em>}
                    {version.isLocked && <em className="badge amber">锁定</em>}
                    {version.status === "inactive" && <em className="badge rose">下架</em>}
                  </span>
                </div>
                <p>{formatDate(version.createdAt)}</p>
                {version.templateSourceCode && (
                  <p className="muted-line">来源：{version.templateSourceCode} v{version.templateSourceVersion || "-"}</p>
                )}
                <div className="version-actions">
                  <a className="button compact" href={versionURL} target="_blank" rel="noreferrer">查看</a>
                  <button className="button compact" type="button" onClick={() => void copyText(versionURL)}><Copy size={13} />复制</button>
                  {canManage && !version.isCurrent && <button className="button compact" type="button" disabled={busyVersion === version.versionNumber} onClick={() => void setCurrentVersion(version.versionNumber)}>设为当前</button>}
                  {canManage && <button className="button compact" type="button" disabled={busyVersion === version.versionNumber} onClick={() => void toggleVersionLock(version)}>{version.isLocked ? "解锁" : "锁定"}</button>}
                  {canManage && !version.isCurrent && !version.isLocked && <button className="button compact danger" type="button" disabled={busyVersion === version.versionNumber} onClick={() => void deleteVersion(version)}>删除</button>}
                </div>
              </div>
            );
          })}
          {versions.length > 2 && (
            <button className="button compact full detail-version-more" type="button" onClick={() => setShowAllVersions((value) => !value)}>
              {showAllVersions ? "收起版本" : `更多版本（${versions.length - 2}）`}
            </button>
          )}
          {!versions.length && <span className="muted-line">暂无版本记录</span>}
        </div>
      </aside>
    </section>
  );
}

function marketFileType(item: MarketplaceDeploy) {
  if (item.bundle?.kindLabel) return item.bundle.kindLabel;
  const name = (item.filename || item.filePath || "").toLowerCase();
  if (name.endsWith(".md") || name.endsWith(".markdown")) return "MD";
  if (name.endsWith(".html") || name.endsWith(".htm")) return "HTML";
  return "ZIP / Site";
}

function securityModeLabel(mode?: string) {
  if (mode === "strict") return "严格";
  if (mode === "trusted") return "受信任";
  if (mode === "compatible") return "兼容";
  if (mode === "standard") return "标准";
  return mode || "标准";
}

function marketCategoryLabel(category?: string) {
  const labels: Record<string, string> = {
    landing: "活动落地页",
    dashboard: "数据看板",
    docs: "文档报告",
    tool: "效率工具",
    game: "互动游戏",
    screen: "屏幕展示"
  };
  if (!category) return "未分类";
  return labels[category] || category;
}

function getMarketHeat(item: MarketplaceDeploy): { label: string; level: "high" | "warm" } | null {
  const score = (item.viewCount || 0) + (item.likeCount || 0) * 6 + (item.favoriteCount || 0) * 8 + (item.versionCount || 1) * 2;
  if (score >= 80) return { label: "爆款", level: "high" };
  if (score >= 32) return { label: "热门", level: "warm" };
  return null;
}

function categoryIcon(slug?: string) {
  switch (slug) {
    case "landing": return <Rocket />;
    case "dashboard": return <Layers />;
    case "docs": return <FileText />;
    case "tool": return <Workflow />;
    case "game": return <Sparkles />;
    case "screen": return <Monitor />;
    default: return <PackageOpen />;
  }
}

function MarketUseDrawer({ item, onClose }: { item: MarketplaceDeploy; onClose: () => void }) {
  const [reuseMode, setReuseMode] = useState<"new" | "update">("new");
  const [targetCode, setTargetCode] = useState("");
  const fallbackReusable = item.visibility === "public" && !item.accessProtected;
  const canReuse = item.reuse ? item.reuse.allowReuse !== false : fallbackReusable;
  const canDownload = item.reuse ? Boolean(item.reuse.allowDownload && item.reuse.downloadUrl) : fallbackReusable;
  const policyNote = item.reuse?.policyNote || (canReuse ? "公开且未加密的作品可以下载源码并作为模板复用。" : "当前作品未开放源码下载和模板复用。");
  const downloadURL = item.reuse?.downloadUrl || `/api/deploy/content?code=${encodeURIComponent(item.code)}&download=1`;
  const title = item.title || item.code;
  const sourceVersion = item.reuse?.templateSourceVersion || item.versionCount || 1;
  const sourceCode = item.reuse?.templateSourceCode || item.code;
  const sourceFiles = item.bundle?.tree?.length ? item.bundle.tree : (item.files || []);
  const sourceSummary = {
    kind: item.bundle?.kindLabel || marketFileType(item),
    entry: item.bundle?.mainEntry || item.filename || "自动识别",
    root: item.bundle?.root || "根目录",
    downloadType: canDownload
      ? (sourceFiles.length > 1 || item.bundle?.kind === "zip_site" || item.bundle?.kind === "static_site" ? "ZIP 源码包" : "单文件源码")
      : "下载受限",
    fileCount: item.bundle?.fileCount || sourceFiles.length || 1,
    totalSize: formatSize(item.bundle?.totalSize || item.fileSize || 0)
  };
  const visibleSourceFiles = sourceFiles.slice(0, 6);
  const mcpText = item.reuse?.mcp
    ? JSON.stringify(item.reuse.mcp, null, 2)
    : JSON.stringify({
      tool: "deploy_site",
      template_source_code: sourceCode,
      template_source_version: sourceVersion,
      reuse_mode: reuseMode
    }, null, 2);
  const normalizedTargetCode = targetCode.trim();
  const canCopyReuse = canReuse && (reuseMode === "new" || normalizedTargetCode.length > 0);
  const newCli = item.reuse?.cli || [
    `pagep market show ${item.code}`,
    "复制详情响应里的 reuse.cli 后执行；它会包含准确的源码下载路径、template-source-code 和 template-source-version。"
  ].join("\n");
  const updateCli = [
    `pagep get ${item.code} --download --output ./pagepilot-downloads`,
    normalizedTargetCode
      ? `pagep append ${normalizedTargetCode} ./pagepilot-downloads/${item.code} --description "基于 ${title} 复用更新" --template-source-code ${sourceCode} --template-source-version ${sourceVersion}`
      : "pagep append YOUR_EXISTING_CODE ./pagepilot-downloads/{downloaded-folder} --description \"基于市场作品复用更新\" --template-source-code " + sourceCode + " --template-source-version " + sourceVersion
  ].join("\n");
  const cli = reuseMode === "new" ? newCli : updateCli;
  const newAgentPrompt = item.reuse?.agentPrompt || [
    `请从 PagePilot 创作市场复用作品 code=${item.code}。`,
    "先下载源文件并检查文件结构、入口 HTML、资源引用和交互逻辑。",
    "在此基础上按我的新需求修改，然后作为新作品发布，并传 templateSourceCode/templateSourceVersion 记录来源；不要覆盖原作品，除非我明确提供并确认原 code 的所有权。"
  ].join("\n");
  const updateAgentPrompt = [
    `请参考 PagePilot 创作市场作品《${title}》（code=${item.code}，version=${sourceVersion}）的结构和风格。`,
    normalizedTargetCode
      ? `目标是更新我已有的 PagePilot 发布 code=${normalizedTargetCode}，请追加为新版本，不要创建新的 code。`
      : "目标是更新我已有的 PagePilot 发布。开始前必须先让我确认目标 code，再追加为新版本，不要创建新的 code。",
    `发布时传 templateSourceCode=${sourceCode}、templateSourceVersion=${sourceVersion} 记录复用来源；不要复制原作品里的隐私数据、密钥或不可复用内容。`
  ].join("\n");
  const agentPrompt = reuseMode === "new" ? newAgentPrompt : updateAgentPrompt;
  const modeNote = reuseMode === "new"
    ? "新建二创会生成新的 PagePilot code，并记录来源作品。"
    : "更新已有发布需要你拥有目标 code，系统会追加新版本，不会覆盖来源作品。";

  return createPortal(
    <div className="market-use-modal" role="dialog" aria-modal="true" aria-label="作品复用">
      <button className="market-use-shade" type="button" aria-label="关闭" onClick={onClose} />
      <section className="market-use-panel">
        <div className="market-use-head">
          <div>
            <span className="mini-label">USE FROM MARKET</span>
            <h2>{title}</h2>
          </div>
          <button className="button compact" type="button" onClick={onClose}>关闭</button>
        </div>
        <div className="market-use-body">
          <div className="reuse-mode-panel">
            <div className="reuse-mode-tabs" role="tablist" aria-label="复用方式">
              <button className={reuseMode === "new" ? "active" : ""} type="button" role="tab" aria-selected={reuseMode === "new"} onClick={() => setReuseMode("new")}>
                新建二创
              </button>
              <button className={reuseMode === "update" ? "active" : ""} type="button" role="tab" aria-selected={reuseMode === "update"} onClick={() => setReuseMode("update")}>
                更新已有
              </button>
            </div>
            <p>{modeNote}</p>
            {reuseMode === "update" && (
              <label className="reuse-target-code">
                <span>目标 code</span>
                <input value={targetCode} onChange={(event) => setTargetCode(event.target.value)} placeholder="填写你拥有的已有发布 code" />
              </label>
            )}
          </div>
          <div className="source-structure-panel">
            <div className="detail-info-head"><FileArchive size={15} /><strong>源文件结构</strong></div>
            <div className="source-summary-grid">
              <span><em>类型</em><strong>{sourceSummary.kind}</strong></span>
              <span><em>入口</em><strong>{sourceSummary.entry}</strong></span>
              <span><em>根目录</em><strong>{sourceSummary.root}</strong></span>
              <span><em>下载</em><strong>{sourceSummary.downloadType}</strong></span>
              <span><em>文件</em><strong>{sourceSummary.fileCount} 个</strong></span>
              <span><em>大小</em><strong>{sourceSummary.totalSize}</strong></span>
            </div>
            {visibleSourceFiles.length ? (
              <div className="source-file-list">
                {visibleSourceFiles.map((file) => (
                  <button type="button" key={file.path} onClick={() => navigator.clipboard.writeText(file.path)} title="复制文件路径">
                    <FileText size={12} />
                    <code>{file.path}</code>
                    <em>{formatSize(file.size || 0)}</em>
                  </button>
                ))}
                {sourceFiles.length > visibleSourceFiles.length && <span>还有 {sourceFiles.length - visibleSourceFiles.length} 个文件，可在详情页查看完整文件树。</span>}
              </div>
            ) : (
              <p className="muted-line">服务端未返回文件树时，请先下载源文件后检查入口和资源路径。</p>
            )}
          </div>
          <div className="use-grid">
            <UseCard icon={<Download />} title="1. 下载源文件" text="多文件站点会下载 ZIP，单文件页面会下载源码。">
              {canDownload ? (
                <a className="button primary full" href={downloadURL}>下载源文件</a>
              ) : (
                <button className="button full" type="button" disabled>源码下载受限</button>
              )}
            </UseCard>
            <UseCard icon={<Code2 />} title="2. CLI 命令行" text={reuseMode === "new" ? "复制命令到终端，下载后作为新作品发布。" : "复制命令到终端，下载后追加到已有发布。"}>
              <button className="button full" type="button" disabled={!canCopyReuse} onClick={() => navigator.clipboard.writeText(cli)}><Copy size={14} />复制 CLI</button>
            </UseCard>
            <UseCard icon={<Bot />} title="3. Agent / MCP" text="复制给 Agent，由创作市场提供复用边界。">
              <button className="button full" type="button" disabled={!canCopyReuse} onClick={() => navigator.clipboard.writeText(agentPrompt)}><Copy size={14} />复制指令</button>
            </UseCard>
            <UseCard icon={<PackageOpen />} title="4. MCP 参数" text="复制结构化参数给支持 MCP 的客户端，保留来源 code 和版本。">
              <button className="button full" type="button" disabled={!canCopyReuse} onClick={() => navigator.clipboard.writeText(mcpText)}><Copy size={14} />复制 MCP</button>
            </UseCard>
          </div>
          <p className="muted-line">{policyNote}</p>
          <DocBlock title="CLI" lines={cli.split("\n")} />
          <DocBlock title="Agent 指令" lines={agentPrompt.split("\n")} />
          <DocBlock title="MCP 参数" lines={mcpText.split("\n")} />
        </div>
      </section>
    </div>,
    document.body
  );
}

function UseCard({ icon, title, text, children }: { icon: React.ReactNode; title: string; text: string; children: React.ReactNode }) {
  return (
    <article className="use-card">
      <span className="tile-icon">{icon}</span>
      <strong>{title}</strong>
      <p>{text}</p>
      {children}
    </article>
  );
}

function DeployPage({ config, session }: { config: RuntimeConfig | null; session: SessionInfo | null }) {
  const [mode, setMode] = useState<"single" | "multi">("single");
  const [filename, setFilename] = useState("index.html");
  const [content, setContent] = useState("<!doctype html>\n<html lang=\"zh-CN\">\n<head>\n  <meta charset=\"utf-8\">\n  <title>PagePilot App</title>\n</head>\n<body>\n  <h1>Hello PagePilot</h1>\n</body>\n</html>");
  const [files, setFiles] = useState<EditableDeployFile[]>([{ path: "index.html", content: "<h1>Hello PagePilot</h1>", isText: true, size: 24 }]);
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [visibility, setVisibility] = useState<"public" | "unlisted">("unlisted");
  const [deployCategory, setDeployCategory] = useState<MarketCategory>("");
  const [tagsInput, setTagsInput] = useState("");
  const [marketCategories, setMarketCategories] = useState<MarketCategoryInfo[]>([]);
  const [accessPassword, setAccessPassword] = useState("");
  const [enableCustom, setEnableCustom] = useState(false);
  const [customCode, setCustomCode] = useState("");
  const [createVersion, setCreateVersion] = useState(false);
  const [updatableSites, setUpdatableSites] = useState<DeploySiteItem[]>([]);
  const [loadingUpdatableSites, setLoadingUpdatableSites] = useState(false);
  const [updatableSitesError, setUpdatableSitesError] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [errorDetail, setErrorDetail] = useState<StructuredAPIErrorPayload | null>(null);
  const [result, setResult] = useState<DeployResponse | null>(null);
  const canPublishToMarket = Boolean(session?.success || session?.userId);

  const totalSize = mode === "single" ? fileTextSize(content) : files.reduce((sum, file) => sum + (file.size ?? fileTextSize(file.content)), 0);
  const ready = mode === "single"
    ? content.trim() && filename.trim()
    : files.length > 0 && files.every((file) => file.path.trim() && (file.contentBase64 || file.content.trim()));

  useEffect(() => {
    api<MarketCategoriesResponse>("/api/market/categories")
      .then((data) => {
        const next = data.categories || [];
        setMarketCategories(next);
      })
      .catch(() => setMarketCategories([]));
  }, []);

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const code = params.get("code")?.trim();
    const shouldCreateVersion = params.get("version") === "1" || params.get("mode") === "version";
    if (!code || !shouldCreateVersion) return;
    setCreateVersion(true);
    setCustomCode(code);
  }, []);

  useEffect(() => {
    if (!createVersion) {
      setUpdatableSitesError("");
      return;
    }
    let alive = true;
    setLoadingUpdatableSites(true);
    setUpdatableSitesError("");
    api<SiteListResponse>("/api/admin/sites")
      .then((data) => {
        if (!alive) return;
        const sites = (data.sites || []).filter((site) => site.code);
        setUpdatableSites(sites);
        if (!customCode && sites.length) setCustomCode(sites[0].code);
        if (customCode && sites.length && !sites.some((site) => site.code === customCode)) {
          setUpdatableSitesError("当前登录账号或匿名会话无权更新这个发布，请从列表中重新选择。");
          setCustomCode("");
        }
      })
      .catch((err) => {
        if (!alive) return;
        setUpdatableSites([]);
        setCustomCode("");
        setUpdatableSitesError(err instanceof Error ? err.message : "请先登录，或使用创建该发布的匿名会话。");
      })
      .finally(() => {
        if (alive) setLoadingUpdatableSites(false);
      });
    return () => {
      alive = false;
    };
  }, [createVersion]);

  const readUploaded = async (list: FileList | null) => {
    if (!list?.length) return;
    if (mode === "single") {
      const file = list[0];
      if (isZipUpload(file.name)) {
        setMode("multi");
        setFilename("");
        setFiles([{ path: file.name || "site.zip", content: "", contentBase64: arrayBufferToBase64(await file.arrayBuffer()), isText: false, size: file.size }]);
        return;
      }
      setFilename(file.name || "index.html");
      setContent(await file.text());
      return;
    }

    const next: EditableDeployFile[] = [];
    for (const file of Array.from(list)) {
      const rawPath = (file as File & { webkitRelativePath?: string }).webkitRelativePath || file.name;
      const path = rawPath.replace(/\\/g, "/").replace(/^\/+/, "");
      if (isTextUpload(file.name)) {
        const text = await file.text();
        next.push({ path, content: text, isText: true, size: fileTextSize(text) });
      } else {
        next.push({ path, content: "", contentBase64: arrayBufferToBase64(await file.arrayBuffer()), isText: false, size: file.size });
      }
    }
    if (next.length) setFiles(next);
  };

  const submit = async () => {
    setBusy(true);
    setError("");
    setErrorDetail(null);
    setResult(null);
    try {
      if (createVersion && !customCode.trim()) {
        throw new Error("请先从可更新发布列表中选择一个作品。");
      }
      const payload: Record<string, unknown> = {
        title: title.trim(),
        description: description.trim(),
        visibility: canPublishToMarket ? visibility : "unlisted",
        category: createVersion ? undefined : (deployCategory || undefined),
        tags: createVersion ? undefined : parseTagInput(tagsInput),
        accessPassword: accessPassword.trim() || undefined
      };
      if (enableCustom || createVersion) payload.code = customCode.trim();
      if (createVersion) payload.createVersion = true;
      if (mode === "single") {
        payload.filename = filename.trim() || "index.html";
        payload.content = content;
      } else {
        if (filename.trim()) payload.filename = filename.trim();
        payload.files = files.map<DeployFilePayload>((file) => ({
          path: file.path.trim(),
          content: file.contentBase64 ? undefined : file.content,
          contentBase64: file.contentBase64
        }));
      }
      const data = await api<DeployResponse>("/api/deploy", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload)
      });
      setResult(data);
    } catch (err) {
      setError(plainErrorMessage(err));
      setErrorDetail(structuredAPIError(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="tool-layout">
      <div className="tool-main">
        <div className="sub-hero">
          <div className="eyebrow"><Upload size={16} />手动部署</div>
          <h1>上传 HTML、Markdown、目录或 ZIP</h1>
          <p>手动部署适合临时调试和人工发布。长期协作建议使用 Agent、MCP 或 pagep CLI。</p>
          <div className="hero-actions">
            <a className="button primary" href="/agents/"><Bot size={18} />交给 Agent 部署</a>
            <a className="button" href="/market"><PackageOpen size={18} />进入创作市场</a>
          </div>
          <div className="deploy-format-strip" aria-label="支持的发布格式">
            <span><FileCode2 size={15} />HTML</span>
            <span><FileText size={15} />Markdown</span>
            <span><FileArchive size={15} />ZIP</span>
            <span><Layers size={15} />多文件目录</span>
          </div>
        </div>
        <div className="preview-box">
          {mode === "single" ? (
            <iframe title="实时预览" srcDoc={content} sandbox={PREVIEW_IFRAME_SANDBOX} />
          ) : (
            <div>
              <FileArchive size={36} />
              <strong>多文件站点</strong>
              <span>共 {files.length} 个文件；ZIP/目录会自动识别 index.html、README.md 或首个 HTML/Markdown 入口。</span>
            </div>
          )}
        </div>
      </div>
      <aside className="tool-panel">
        <div className="segmented">
          <button className={mode === "single" ? "active" : ""} type="button" onClick={() => setMode("single")}><FileCode2 size={15} />单文件</button>
          <button className={mode === "multi" ? "active" : ""} type="button" onClick={() => setMode("multi")}><FileArchive size={15} />多文件</button>
        </div>
        <label className="field">
          <span>作品标题</span>
          <input value={title} onChange={(event) => setTitle(event.target.value)} placeholder="给作品起一个可读名称" />
        </label>
        <label className="field">
          <span>描述</span>
          <input value={description} onChange={(event) => setDescription(event.target.value)} placeholder="一句话说明用途" />
        </label>
        <div className="field-grid three">
          <label className="field">
            <span>市场可见性</span>
            <select value={visibility} onChange={(event) => setVisibility(event.target.value as "public" | "unlisted")}>
              <option value="public" disabled={!canPublishToMarket}>进入创作市场（登录后可选）</option>
              <option value="unlisted">不进入市场</option>
            </select>
            {!canPublishToMarket && <em>匿名发布默认仅链接访问，登录后可进入创作市场。</em>}
          </label>
          <label className="field">
            <span>作品分类（可选）</span>
            <select value={deployCategory} onChange={(event) => setDeployCategory(event.target.value as MarketCategory)} disabled={createVersion}>
              <option value="">暂不分类</option>
              {marketCategories.map((item) => <option value={item.slug} key={item.slug}>{item.label}</option>)}
            </select>
          </label>
          <label className="field">
            <span>作品标签</span>
            <input value={tagsInput} onChange={(event) => setTagsInput(event.target.value)} disabled={createVersion} placeholder="官网, 看板, 活动页" />
          </label>
        </div>
        <div className="field-grid">
          <label className="field">
            <span>访问密码</span>
            <input value={accessPassword} onChange={(event) => setAccessPassword(event.target.value)} type="password" placeholder="可选" />
          </label>
        </div>
        <label className="check-line">
          <input type="checkbox" checked={enableCustom || createVersion} disabled={createVersion} onChange={(event) => setEnableCustom(event.target.checked)} />
          自定义 code 后缀
        </label>
        {enableCustom && !createVersion && (
          <input className="standalone-input mono" value={customCode} onChange={(event) => setCustomCode(event.target.value)} placeholder={createVersion ? "输入已有 code" : "my-landing"} />
        )}
        <label className="check-line">
          <input
            type="checkbox"
            checked={createVersion}
            onChange={(event) => {
              setCreateVersion(event.target.checked);
              if (event.target.checked) setEnableCustom(false);
            }}
          />
          更新已有发布，追加为新版本
        </label>
        {createVersion && (
          <div className="update-version-picker">
            <label className="field">
              <span>选择要更新的发布</span>
              <select value={customCode} onChange={(event) => setCustomCode(event.target.value)} disabled={loadingUpdatableSites || !updatableSites.length}>
                <option value="">{loadingUpdatableSites ? "正在加载可更新发布..." : "请选择当前账号或匿名会话的发布"}</option>
                {updatableSites.map((site) => (
                  <option value={site.code} key={site.code}>
                    {(site.title || site.code)} / {site.code} / v{site.currentVersion || site.versionCount || 1}
                  </option>
                ))}
              </select>
            </label>
            <div className="hint-box">
              更新只能从当前登录用户或当前匿名 session 拥有的发布中选择；认领后的匿名发布需要登录对应账号更新。
              {updatableSitesError && <strong>{updatableSitesError}</strong>}
              {!loadingUpdatableSites && !updatableSitesError && !updatableSites.length && <strong>暂无可更新发布。</strong>}
            </div>
          </div>
        )}

        {mode === "single" ? (
          <>
            <label className="field">
              <span>入口文件名</span>
              <input value={filename} onChange={(event) => setFilename(event.target.value)} placeholder="index.html 或 README.md" />
            </label>
            <label className="upload-line">
              <input type="file" accept=".html,.htm,.md,.markdown,.txt,.zip" onChange={(event) => void readUploaded(event.target.files)} />
              <Upload size={18} />上传 HTML / Markdown / ZIP
            </label>
            <textarea className="code-input" value={content} onChange={(event) => setContent(event.target.value)} placeholder="<!doctype html>..." />
          </>
        ) : (
          <>
            <label className="field">
              <span>入口文件名（可选）</span>
              <input value={filename} onChange={(event) => setFilename(event.target.value)} placeholder="留空自动识别 index.html 或 README.md" />
            </label>
            <label className="upload-line">
              <input type="file" multiple webkitdirectory="" onChange={(event) => void readUploaded(event.target.files)} />
              <Upload size={18} />上传目录
            </label>
            <label className="upload-line">
              <input type="file" accept=".zip" onChange={(event) => void readUploaded(event.target.files)} />
              <FileArchive size={18} />上传 ZIP 包
            </label>
            <div className="file-editor-list">
              {files.map((file, index) => (
                <div className="file-editor" key={index}>
                  <input value={file.path} onChange={(event) => setFiles((prev) => prev.map((f, i) => i === index ? { ...f, path: event.target.value } : f))} />
                  {file.contentBase64 ? (
                    <div className="binary-file-note">二进制资源 / {formatSize(file.size)} / 将随整站一起上传</div>
                  ) : (
                    <textarea value={file.content} onChange={(event) => setFiles((prev) => prev.map((f, i) => i === index ? { ...f, content: event.target.value, size: fileTextSize(event.target.value) } : f))} />
                  )}
                  <button type="button" onClick={() => setFiles((prev) => prev.filter((_, i) => i !== index))} disabled={files.length === 1}>移除</button>
                </div>
              ))}
              <button className="button" type="button" onClick={() => setFiles((prev) => [...prev, { path: "", content: "", isText: true, size: 0 }])}>新增文件</button>
            </div>
          </>
        )}

        <div className="deploy-summary">
          <span>大小 {formatSize(totalSize)}</span>
          <span>上限 {formatSize(config?.limits?.maxSiteTotalBytes)}</span>
        </div>
        <button className="button primary full" type="button" disabled={!ready || busy} onClick={submit}>
          <Rocket size={18} />{busy ? "部署中..." : "立即部署"}
        </button>
        {error && <DeployErrorPanel message={error} error={errorDetail} />}
      </aside>
      {result && <DeployResult result={result} onClose={() => setResult(null)} />}
    </section>
  );
}

function DeployResult({ result, onClose }: { result: DeployResponse; onClose: () => void }) {
  const appURL = sameSiteURL(result.url);
  const detailURL = sameSiteURL(result.detailUrl || result.url);
  const versionURL = sameSiteURL(result.versionUrl || "");
  const rows = [
    ["code", result.code],
    ["访问地址", appURL],
    ["作品详情", detailURL],
    ["版本预览", versionURL],
    ["版本", result.versionNumber ? `v${result.versionNumber}` : "-"]
  ];
  return (
    <div className="modal-backdrop" role="dialog" aria-modal="true" aria-label="部署成功">
      <div className="result-modal">
        <div className="result-modal-head">
          <div>
            <span className="mini-label">DEPLOY SUCCESS</span>
            <strong>部署成功</strong>
          </div>
          <button className="icon-button" type="button" onClick={onClose} aria-label="关闭">×</button>
        </div>
        <p>应用已经上线，下面这些链接可以直接复制给用户、Agent 或继续进入创作市场复用。</p>
        <div className="result-box">
          {rows.map(([label, value]) => (
            <div className="result-row" key={label}>
              <span>{label}</span>
              <code>{value}</code>
              <button type="button" onClick={() => navigator.clipboard.writeText(value)} aria-label={`复制${label}`}><Copy size={14} /></button>
            </div>
          ))}
        </div>
        <div className="modal-actions">
          <a className="button primary" href={appURL} target="_blank" rel="noreferrer">打开应用</a>
          <a className="button" href="/market" target="_blank" rel="noreferrer">进入创作市场</a>
          <button className="button" type="button" onClick={onClose}>继续部署</button>
        </div>
      </div>
    </div>
  );
}

function AgentsPage({ config }: { config: RuntimeConfig | null }) {
  const baseURL = currentBaseURL();
  const [tab, setTab] = useState<AgentDocTab>("skill");

  return (
    <section className="content-page">
      <div className="sub-hero">
        <div className="eyebrow"><Bot size={16} />Agent</div>
        <h1>让 Agent 直接把网页交给 PagePilot</h1>
        <p>
          下载 pagep Skill 后，Agent 可以发布 HTML、Markdown、ZIP 和多文件站点；也可以从创作市场下载源文件，
          修改后作为新作品发布。MCP 适合支持 stdio JSON-RPC 的客户端。
        </p>
        <div className="hero-actions">
          <a className="button primary" href={skillDownloadPath()}><Download size={18} />下载 pagep Skill</a>
          <a className="button" href="/openapi.json"><Workflow size={18} />OpenAPI</a>
        </div>
      </div>
      <div className="doc-tabs" role="tablist" aria-label="Skill 和 MCP 文档">
        <button className={tab === "skill" ? "active" : ""} type="button" role="tab" aria-selected={tab === "skill"} onClick={() => setTab("skill")}>
          <Download size={16} />Skill
        </button>
        <button className={tab === "mcp" ? "active" : ""} type="button" role="tab" aria-selected={tab === "mcp"} onClick={() => setTab("mcp")}>
          <Workflow size={16} />MCP
        </button>
      </div>
      {tab === "skill" ? <SkillGuide baseURL={baseURL} /> : <MCPGuide baseURL={baseURL} />}
      <div className="info-grid">
        <FeatureTile icon={<KeyRound />} title="令牌接入" desc="登录后在用户中心创建 API Token，Agent 使用令牌管理自己的站点和屏幕。" />
        <FeatureTile icon={<ShieldCheck />} title="匿名 Agent" desc="未登录 Agent 会持久化本地 session，后台能看到名称、IP 和 User-Agent。" />
        <FeatureTile icon={<Monitor />} title="屏幕投放" desc="屏幕绑定、投放、截图和远程命令必须使用注册用户 Token。" />
      </div>
    </section>
  );
}

function SkillGuide({ baseURL }: { baseURL: string }) {
  const server = baseURL || currentBaseURL();

  return (
    <>
      <SkillInstallCard baseURL={server} />
      <div className="doc-grid">
        <DocBlock
          title="新建发布"
          lines={[
            `pagep deploy ./dist --server ${server} --title "中文项目名" --description "一句话说明" --visibility public`,
            "支持单 HTML、Markdown、ZIP 和多文件目录。",
            "访问密码使用 --access-password；不进入市场使用 --visibility unlisted。"
          ]}
        />
        <DocBlock
          title="复用创作市场作品"
          lines={[
            "pagep get demo-code --download --output ./pagepilot-downloads",
            "修改源码后使用 pagep deploy --template-source-code demo-code 发布为新作品。",
            "可从详情页复制带 template-source-version 的完整命令；不要默认覆盖原作品。"
          ]}
        />
        <DocBlock
          title="绑定用户"
          lines={[
            "python scripts/pagep.py token create --label agent --ttl-seconds 2592000",
            "python scripts/pagep.py claim-session --token YOUR_TOKEN",
            "claim-session 会把当前匿名 session 发布过的站点绑定到这个用户。"
          ]}
        />
        <DocBlock
          title="屏幕投放"
          lines={[
            "python scripts/pagep.py screen list --token YOUR_TOKEN",
            "python scripts/pagep.py screen publish --screen SCREEN_ID --app my-landing --token YOUR_TOKEN",
            "python scripts/pagep.py screen refresh SCREEN_ID --token YOUR_TOKEN"
          ]}
        />
      </div>
    </>
  );
}

function MCPGuide({ baseURL }: { baseURL: string }) {
  const server = baseURL || currentBaseURL();

  return (
    <>
      <div className="mcp-card">
        <div>
          <div className="mini-label">PAGEPILOT MCP</div>
          <h2>面向支持 MCP 的 Agent 客户端</h2>
          <p>MCP 走 stdio JSON-RPC，适合把发布、源码读取、版本、Token 和屏幕投放注册为工具。</p>
        </div>
        <a className="button" href="/openapi.json"><Workflow size={18} />查看 OpenAPI</a>
      </div>
      <div className="doc-grid">
        <DocBlock
          title="启动方式"
          lines={[
            `pagep-mcp --server ${server} --token YOUR_TOKEN`,
            "私有服务器请把 --server 改为你的 PagePilot 地址。",
            "没有 Token 时只适合匿名发布；涉及后台和用户资源时必须提供 Token。"
          ]}
        />
        <DocBlock
          title="发布与源码"
          lines={[
            "deploy_site: 发布单 HTML、Markdown、ZIP 或多文件静态站点。",
            "get_site_content: 读取站点源码，供 Agent 二次创作。",
            "list_sites: 查询当前用户或匿名 session 可管理的站点。"
          ]}
        />
        <DocBlock
          title="创作市场提供"
          lines={[
            "市场详情页会生成下载源文件、CLI、Agent/MCP 三种复用方式。",
            "Agent 复用公开作品时默认发布为新作品，并传 template_source_code 记录来源。",
            "加密作品只能授权浏览，不提供源码下载和模板复用。"
          ]}
        />
        <DocBlock
          title="匿名身份"
          lines={[
            "Agent 匿名身份由本地 sessionId 决定，应持久化并发送 X-Hostctl-Session。",
            "X-Hostctl-Agent-Id / X-Hostctl-Agent-Label 作为设备标记，方便后台排查。",
            "IP 和 User-Agent 只做辅助信息，不作为所有权依据。"
          ]}
        />
      </div>
    </>
  );
}

function SkillInstallCard({ baseURL }: { baseURL: string }) {
  const skillURL = `${baseURL}${skillDownloadPath()}`;
  const doctorCommand = `pagep doctor --server ${baseURL}`;
  const agentPrompt = [
    "请从以下地址下载并安装 PagePilot pagep Skill：",
    skillURL,
    `安装后运行 \`${doctorCommand}\` 检查连接。`,
    "之后使用 pagep deploy 发布网页；从创作市场复用作品时，先下载源文件，再按新需求发布为新作品。"
  ].join("\n");

  return (
    <section className="skill-package-card">
      <div className="skill-package-head">
        <div>
          <div className="mini-label">pagep Skill</div>
          <h2>下载并安装 Agent Skill</h2>
          <p>下载地址会按当前访问域名生成，适合内网、反向代理和公网部署场景。</p>
        </div>
        <a className="button primary" href={skillDownloadPath()}>
          <Download size={18} />下载 Skill
        </a>
      </div>
      <div className="package-url">
        <code>{skillURL}</code>
        <button type="button" className="button compact" onClick={() => navigator.clipboard.writeText(skillURL)}>
          <Copy size={14} />复制地址
        </button>
      </div>
      <div className="copy-panel">
        <div className="copy-panel-head">
          <strong>复制给 Agent</strong>
          <button type="button" className="button compact primary" onClick={() => navigator.clipboard.writeText(agentPrompt)}>
            <Copy size={14} />复制
          </button>
        </div>
        <pre>{agentPrompt}</pre>
      </div>
      <div className="package-url">
        <code>{doctorCommand}</code>
        <button type="button" className="button compact" onClick={() => navigator.clipboard.writeText(doctorCommand)}>
          <Copy size={14} />复制命令
        </button>
      </div>
    </section>
  );
}

function ScreensPage() {
  const [screens, setScreens] = useState<ScreenInfo[]>([]);
  const [error, setError] = useState("");

  useEffect(() => {
    api<ScreensResponse>("/api/screens")
      .then((data) => setScreens(data.screens || []))
      .catch((err) => setError(err instanceof Error ? err.message : String(err)));
  }, []);

  const onlineCount = screens.filter((screen) => String(screen.status || "").toLowerCase().includes("online")).length;

  return (
    <section className="content-page screen-page-v2">
      <div className="screen-hero-v2">
        <div className="screen-hero-copy">
          <div className="eyebrow"><Monitor size={16} />广告屏投放</div>
          <h1>把 Agent 生成的应用，投到真实屏幕上</h1>
          <p>屏幕绑定后，PagePilot 可以从后台、Skill、MCP 或 CLI 远程投放应用、刷新 WebView、回传截图，并把线上页面变成可管理的展示终端。</p>
          <div className="screen-hero-actions">
            <a className="button primary" href="/admin?tab=screens"><Monitor size={18} />进入屏幕管理</a>
            <a className="button" href="/market"><PackageOpen size={18} />选择市场作品</a>
          </div>
        </div>
        <div className="screen-device-demo" aria-hidden="true">
          <div className="screen-device-frame">
            <div className="screen-device-top"><span /><span /><span /></div>
            <div className="screen-device-canvas">
              <strong>PagePilot Screen</strong>
              <span>/agent/product-launch/</span>
              <em>refresh · screenshot · publish</em>
            </div>
          </div>
          <div className="screen-command-card">
            <span>pagep screen publish</span>
            <strong>{screens.length || 0} 屏幕 · {onlineCount} 在线</strong>
          </div>
        </div>
      </div>

      <div className="screen-feature-strip">
        <FeatureTile icon={<Monitor />} title="一次绑定" desc="Android 屏幕 App 绑定到账号后，由后台统一管理。" />
        <FeatureTile icon={<Rocket />} title="远程投放" desc="从应用管理或创作市场选择作品，一键发布到屏幕。" />
        <FeatureTile icon={<Eye />} title="截图回传" desc="远程请求截图，确认现场展示是否按预期运行。" />
        <FeatureTile icon={<Workflow />} title="运行命令" desc="支持刷新、休眠、唤醒和基础远程控制。" />
      </div>

      <div className="screen-panel-v2">
        <div className="screen-panel-head">
          <div>
            <span className="mini-label">MY SCREENS</span>
            <h2>我的屏幕</h2>
            <p>{error ? "登录注册用户后可查看自己的屏幕。" : "服务器：" + currentOrigin()}</p>
          </div>
          <a className="button compact" href="/admin?tab=screens">管理绑定</a>
        </div>
        {error ? <div className="empty-wide">{error}</div> : (
          <div className="screen-list-v2">
            {screens.map((screen) => (
              <div className="screen-row-v2" key={screen.id}>
                <div>
                  <strong>{screen.name || screen.id}</strong>
                  <span>{formatDeviceInfo(screen.deviceInfo)}</span>
                </div>
                <div className="screen-row-status">
                  <em>{screen.status || "未知"}</em>
                  <code>{screen.currentSiteCode || "空闲"}</code>
                </div>
              </div>
            ))}
            {!screens.length && <div className="empty-wide">还没有绑定屏幕。进入后台生成绑定码后，在 Android 屏幕 App 中完成绑定。</div>}
          </div>
        )}
      </div>
    </section>
  );
}

function DocBlock({ title, lines }: { title: string; lines: string[] }) {
  return (
    <div className="doc-block">
      <h2>{title}</h2>
      <pre>{lines.join("\n")}</pre>
    </div>
  );
}

function Footer({ config }: { config: RuntimeConfig | null }) {
  const version = config?.version || "dev";
  return (
    <footer className="footer">
      <span>PagePilot / Agent 网页发布 / 创作市场 / Screen 投放</span>
      <code>当前版本 {version}</code>
      <a href="/admin">返回后台</a>
    </footer>
  );
}

declare module "react" {
  interface InputHTMLAttributes<T> {
    webkitdirectory?: string;
  }
}
