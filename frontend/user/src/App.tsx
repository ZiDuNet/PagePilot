import {
  Bot,
  ChevronLeft,
  ChevronRight,
  Code2,
  Copy,
  Download,
  ExternalLink,
  FileArchive,
  FileCode2,
  Heart,
  KeyRound,
  Layers,
  Lock,
  Monitor,
  PackageOpen,
  Rocket,
  Search,
  ShieldCheck,
  Sparkles,
  Upload,
  Workflow
} from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "./api";
import { createPreviewScheduler } from "./previewScheduler";
import type {
  DeployFilePayload,
  DeployResponse,
  MarketplaceDeploy,
  RuntimeConfig,
  ScreenInfo,
  SessionInfo
} from "./types";

type Page = "home" | "market" | "agents" | "deploy" | "apiDocs" | "screens";
type SortMode = "newest" | "oldest" | "likes_desc" | "views_desc";
type AgentDocTab = "skill" | "mcp";
type PreviewStatus = "idle" | "queued" | "loading" | "slow" | "loaded";

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

interface ScreensResponse {
  screens?: ScreenInfo[];
}

const marketplacePreviewScheduler = createPreviewScheduler(3);
const previewSlowHintMs = 4200;
const previewSlotReleaseMs = 8500;

const navItems: Array<{ page: Page; label: string; href: string }> = [
  { page: "market", label: "创作市场", href: "/market" },
  { page: "agents", label: "Agent", href: "/agents/" },
  { page: "deploy", label: "手动部署", href: "/deploy.html" },
  { page: "apiDocs", label: "API 文档", href: "/api-docs.html" }
];

function getPageFromPath(pathname: string): Page {
  if (pathname.startsWith("/market")) return "market";
  if (pathname.startsWith("/deploy.html")) return "deploy";
  if (pathname.startsWith("/agents")) return "agents";
  if (pathname.startsWith("/screens")) return "screens";
  if (pathname.startsWith("/api-docs")) return "apiDocs";
  return "home";
}

function formatSize(bytes?: number): string {
  const n = Number(bytes || 0);
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / 1024 / 1024).toFixed(2)} MB`;
}

function formatDate(value?: string): string {
  if (!value) return "-";
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return d.toLocaleDateString("zh-CN");
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

  useEffect(() => {
    const onPop = () => setPage(getPageFromPath(location.pathname));
    window.addEventListener("popstate", onPop);
    return () => window.removeEventListener("popstate", onPop);
  }, []);

  const navigate = useCallback((next: Page, href: string) => {
    setPage(next);
    history.pushState({}, "", href);
    window.scrollTo({ top: 0, behavior: "smooth" });
  }, []);

  return (
    <div className="app-shell">
      <TopNav page={page} session={session} onNavigate={navigate} />
      <main className="page-main">
        {page === "home" && <HomePage config={config} onNavigate={navigate} />}
        {page === "market" && <MarketPage config={config} />}
        {page === "deploy" && <DeployPage config={config} />}
        {page === "agents" && <AgentsPage config={config} />}
        {page === "screens" && <ScreensPage />}
        {page === "apiDocs" && <ApiDocsPage />}
      </main>
      <Footer />
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
          {session?.success ? (
            <a className="nav-button dark" href="/admin">管理</a>
          ) : (
            <a className="nav-button dark" href="/admin?mode=login">用户登录</a>
          )}
        </div>
      </div>
    </header>
  );
}

function Logo() {
  return (
    <svg className="logo-mark" viewBox="0 0 96 96" aria-hidden="true">
      <defs>
        <linearGradient id="logo-bg-react" x1="6" x2="88" y1="6" y2="90">
          <stop stopColor="#101348" />
          <stop offset="0.55" stopColor="#172D67" />
          <stop offset="1" stopColor="#155E75" />
        </linearGradient>
        <linearGradient id="logo-window-react" x1="32" x2="66" y1="35" y2="74">
          <stop stopColor="#FFFFFF" />
          <stop offset="1" stopColor="#EAF7FF" />
        </linearGradient>
        <linearGradient id="logo-bar-react" x1="31" x2="67" y1="35" y2="49">
          <stop stopColor="#60A5FA" />
          <stop offset="1" stopColor="#8B5CF6" />
        </linearGradient>
      </defs>
      <rect x="5" y="5" width="86" height="86" rx="26" fill="url(#logo-bg-react)" />
      <path d="M13 67C32 45 60 36 84 29" fill="none" stroke="#22D3EE" strokeLinecap="round" strokeWidth="4" opacity="0.82" />
      <path d="M17 73C35 59 61 49 86 43" fill="none" stroke="#A78BFA" strokeLinecap="round" strokeWidth="3" opacity="0.55" />
      <rect x="15" y="22" width="25" height="14" rx="6" fill="#F8FAFC" opacity="0.96" />
      <path d="M20 29h7M23.5 25.5v7" stroke="#312E81" strokeLinecap="round" strokeWidth="2" />
      <circle cx="32" cy="27" r="2" fill="#86EFAC" />
      <circle cx="36" cy="31" r="2" fill="#FB7185" />
      <rect x="61" y="20" width="20" height="20" rx="6" fill="#24195F" stroke="#C084FC" strokeWidth="2" />
      <rect x="65" y="24" width="5" height="5" rx="1.2" fill="#F472B6" />
      <rect x="72" y="24" width="5" height="5" rx="1.2" fill="#86EFAC" />
      <rect x="65" y="31" width="5" height="5" rx="1.2" fill="#38BDF8" />
      <rect x="72" y="31" width="5" height="5" rx="1.2" fill="#FBBF24" />
      <rect x="31" y="35" width="36" height="39" rx="11" fill="url(#logo-window-react)" />
      <path d="M31 45a10 10 0 0 1 10-10h16a10 10 0 0 1 10 10v5H31z" fill="url(#logo-bar-react)" />
      <circle cx="39" cy="44" r="2.7" fill="#F472B6" />
      <circle cx="48" cy="44" r="2.7" fill="#FBBF24" />
      <circle cx="57" cy="44" r="2.7" fill="#86EFAC" />
      <path d="M38 58q3-4 7 0M53 58q3-4 7 0" fill="none" stroke="#111827" strokeLinecap="round" strokeWidth="3" />
      <path d="M45 66q4 4 9 0" fill="#FB7185" stroke="#831843" strokeLinecap="round" strokeWidth="1.6" />
    </svg>
  );
}

function HomePage({
  config,
  onNavigate
}: {
  config: RuntimeConfig | null;
  onNavigate: (page: Page, href: string) => void;
}) {
  return (
    <div className="home-page">
      <HeroBarrage />
      <section className="hero-band">
        <div className="hero-copy">
          <div className="eyebrow"><Sparkles size={16} />Agent 生成，PagePilot 上线</div>
          <h1>告诉 Agent 需求，秒级上线应用</h1>
          <p>
            PagePilot 面向 AI 生成网页：支持单 HTML、多文件站点、ZIP 包、Markdown、访问密码、版本管理和创作市场。
            你负责想法，Agent 负责生成，PagePilot 负责发布、分享和复用。
          </p>
          <div className="hero-actions">
            <a
              className="button primary"
              href="/agents/"
              onClick={(event) => {
                event.preventDefault();
                onNavigate("agents", "/agents/");
              }}
            >
              <Bot size={18} />交给 Agent 部署
            </a>
            <a
              className="button"
              href="/market"
              onClick={(event) => {
                event.preventDefault();
                onNavigate("market", "/market");
              }}
            >
              <PackageOpen size={18} />进入创作市场
            </a>
          </div>
        </div>
        <div className="feature-board" aria-label="PagePilot 特性">
          <FeatureTile icon={<Rocket />} title="秒级上线应用" desc="HTML、Markdown、ZIP 和多文件静态站点都能发布。" />
          <FeatureTile icon={<Lock />} title="可加密网页" desc="给页面设置访问密码，公开展示和访问权限分开控制。" />
          <FeatureTile icon={<Layers />} title="版本管理" desc="每次修改形成新版本，可回滚、锁定，也能保留短链。" />
          <FeatureTile icon={<PackageOpen />} title="创作市场" desc="下载源文件、复制 CLI 命令，或交给 Agent/MCP 继续创作。" />
        </div>
      </section>

      <section className="stat-strip">
        <Metric label="匿名发布额度" value={config?.anonymousPolicy?.deployLimit == null ? "-" : String(config.anonymousPolicy.deployLimit)} note="用于未登录 Agent 快速试用" />
        <Metric label="发布冷却" value={`${config?.cooldownSeconds ?? "-"}s`} note="防刷限流，不影响正常迭代" />
        <Metric label="整站上限" value={formatSize(config?.limits?.maxSiteTotalBytes)} note="适合完整静态网站包" />
        <Metric label="文件数量" value={config?.limits?.maxFilesPerSite == null ? "-" : String(config.limits.maxFilesPerSite)} note="ZIP 和目录上传共用限制" />
      </section>

      <section className="flow-section">
        <div className="section-head">
          <div>
            <h2>从想法到链接，只走一条线</h2>
            <p>借鉴 jpage 的复用链路，但 PagePilot 把 Agent、版本和访问控制放在同一个产品闭环里。</p>
          </div>
        </div>
        <div className="flow-grid">
          <FlowStep label="01" title="告诉 Agent 需求" text="让 Agent 生成网页、修复样式、整理多文件结构。" />
          <FlowStep label="02" title="发布到 PagePilot" text="使用 pagep Skill、MCP 或手动部署，把文件包推到服务器。" />
          <FlowStep label="03" title="分享或加密" text="短链公开、访问密码、版本历史和屏幕投放按需开启。" />
          <FlowStep label="04" title="进入创作市场" text="公开作品可以下载源码、复制 CLI，或交给 Agent 二次创作。" />
        </div>
      </section>
    </div>
  );
}

function HeroBarrage() {
  const prompts = [
    "帮我做一个打砖块小游戏", "把这份日报做成网页", "生成一个手机端抽签页面", "做个 AI 对话登录页",
    "用 HTML 做产品定价页", "做一个课堂计分板", "把表格变成可视化看板", "做个活动报名页",
    "帮我部署到 PagePilot", "生成后直接给我链接", "顺手写个项目简介", "代码不变，样式升级",
    "做一个 SaaS 状态页", "生成一个 API 测试页面", "做个二维码落地页", "把 Markdown 转成漂亮网页"
  ];
  const lanes: Array<[string[], boolean, number, number, number]> = [
    [prompts.slice(0, 10), false, 52, 6, 0.38],
    [prompts.slice(6), true, 66, 18, 0.30],
    [prompts.slice(8).concat(prompts.slice(0, 4)), false, 58, 30, 0.26],
    [prompts.slice(3, 13), true, 72, 42, 0.22],
    [prompts.slice(0, 8), false, 64, 55, 0.26],
    [prompts.slice(8), true, 78, 69, 0.20],
    [prompts.slice(4, 14), false, 70, 82, 0.22]
  ];

  return (
    <div className="hero-barrage" aria-hidden="true">
      {lanes.map(([items, reverse, duration, top, opacity], index) => (
        <div
          className={`barrage-lane ${reverse ? "reverse" : ""}`}
          key={index}
          style={{
            "--duration": `${duration}s`,
            "--lane-opacity": opacity,
            top: `${top}%`
          } as React.CSSProperties}
        >
          {[...items, ...items].map((item, chipIndex) => (
            <span className="barrage-chip" key={`${item}-${chipIndex}`}>{item}</span>
          ))}
        </div>
      ))}
    </div>
  );
}

function FeatureTile({ icon, title, desc }: { icon: React.ReactNode; title: string; desc: string }) {
  return (
    <div className="feature-tile">
      <span className="tile-icon">{icon}</span>
      <strong>{title}</strong>
      <span>{desc}</span>
    </div>
  );
}

function FlowStep({ label, title, text }: { label: string; title: string; text: string }) {
  return (
    <article className="flow-step">
      <span>{label}</span>
      <strong>{title}</strong>
      <p>{text}</p>
    </article>
  );
}

function Metric({ label, value, note }: { label: string; value: string; note: string }) {
  return (
    <div className="metric">
      <span>{label}</span>
      <strong>{value}</strong>
      <em>{note}</em>
    </div>
  );
}

function MarketPage({ config }: { config: RuntimeConfig | null }) {
  const pageSize = 24;
  const [items, setItems] = useState<MarketplaceDeploy[]>([]);
  const [selected, setSelected] = useState<MarketplaceDeploy | null>(null);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [query, setQuery] = useState("");
  const [sort, setSort] = useState<SortMode>("newest");
  const [loading, setLoading] = useState(true);
  const pageCount = Math.max(1, Math.ceil(total / pageSize));
  const safePage = Math.min(page, pageCount);
  const pinnedCount = items.filter((item) => item.isPinned).length;

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const params = new URLSearchParams({
        page: String(page),
        pageSize: String(pageSize),
        sort
      });
      if (query.trim()) params.set("q", query.trim());
      const data = await api<MarketplaceResponse>(`/api/deploys?${params.toString()}`);
      setItems(data.deploys || []);
      setTotal(data.total || 0);
    } finally {
      setLoading(false);
    }
  }, [page, query, sort]);

  useEffect(() => {
    const timer = window.setTimeout(load, 160);
    return () => window.clearTimeout(timer);
  }, [load]);

  useEffect(() => {
    if (!loading && page > pageCount) setPage(pageCount);
  }, [loading, page, pageCount]);

  return (
    <section className="market-page">
      <div className="sub-hero market-hero">
        <div className="eyebrow"><PackageOpen size={16} />创作市场</div>
        <h1>复用好作品，而不是重新发明每个页面</h1>
        <p>
          市场提供三种复用方式：下载源文件、复制 CLI 命令、交给 Agent/MCP。公开作品可被预览和点赞；
          加密作品仍会保护内容访问。
        </p>
        <div className="hero-actions">
          <a className="button primary" href="/agents/"><Bot size={18} />交给 Agent 部署</a>
          <a className="button" href="/deploy.html"><Upload size={18} />手动部署</a>
        </div>
      </div>

      <section className="market-section">
        <div className="section-head market-head">
          <div>
            <h2>作品列表</h2>
            <p>
              {loading
                ? "正在加载作品"
                : `共 ${total} 个公开作品，${pinnedCount ? `本页 ${pinnedCount} 个置顶，` : ""}可预览、下载和复用。`}
            </p>
          </div>
          <div className="filters market-filters">
            <label className="search-box">
              <Search size={16} />
              <input
                value={query}
                onChange={(event) => {
                  setPage(1);
                  setQuery(event.target.value);
                }}
                placeholder="搜索标题、描述或 code"
              />
            </label>
            <select
              value={sort}
              onChange={(event) => {
                setPage(1);
                setSort(event.target.value as SortMode);
              }}
            >
              <option value="newest">最新发布</option>
              <option value="likes_desc">点赞最多</option>
              <option value="views_desc">访问最多</option>
              <option value="oldest">最早发布</option>
            </select>
          </div>
        </div>

        <div className="quick-filters" aria-label="快速筛选">
          {[
            ["", "全部作品"],
            ["后台 dashboard 数据", "后台与看板"],
            ["落地页 产品 品牌", "产品与落地页"],
            ["工具 表单 计算器", "工具与表单"],
            ["Markdown 文档 报告", "Markdown 与报告"]
          ].map(([value, label]) => (
            <button
              key={label}
              className={query === value ? "active" : ""}
              type="button"
              onClick={() => {
                setPage(1);
                setQuery(value);
              }}
            >
              {label}
            </button>
          ))}
        </div>

        {loading ? (
          <div className="card-grid">{Array.from({ length: 6 }).map((_, index) => <div className="skeleton" key={index} />)}</div>
        ) : (
          <div className="card-grid">
            {items.map((item) => <MarketplaceCard key={item.code} item={item} onChanged={load} onUse={setSelected} />)}
            {!items.length && <div className="empty-wide">还没有找到作品。换个关键词试试，或先让 Agent 发布一个新页面。</div>}
          </div>
        )}

        {!loading && total > 0 && (
          <div className="market-pagination" aria-label="创作市场分页">
            <div className="page-note">
              管理员置顶优先展示，其余作品继续按当前排序排列。当前实例整站上传上限：{formatSize(config?.limits?.maxSiteTotalBytes)}。
            </div>
            <div className="pager">
              <button className="button compact" type="button" disabled={safePage <= 1} onClick={() => setPage((current) => Math.max(1, current - 1))}>
                <ChevronLeft size={15} />上一页
              </button>
              <span className="page-indicator">第 {safePage} / {pageCount} 页</span>
              <button className="button compact" type="button" disabled={safePage >= pageCount} onClick={() => setPage((current) => Math.min(pageCount, current + 1))}>
                下一页<ChevronRight size={15} />
              </button>
            </div>
          </div>
        )}
      </section>

      {selected && <MarketUseDrawer item={selected} onClose={() => setSelected(null)} />}
    </section>
  );
}

function MarketplaceCard({
  item,
  onChanged,
  onUse
}: {
  item: MarketplaceDeploy;
  onChanged: () => void;
  onUse: (item: MarketplaceDeploy) => void;
}) {
  const previewRef = useRef<HTMLIFrameElement | null>(null);
  const cardRef = useRef<HTMLElement | null>(null);
  const [previewStatus, setPreviewStatus] = useState<PreviewStatus>("idle");
  const title = item.title || item.code || "未命名作品";
  const appURL = sameSiteURL(`/agent/${encodeURIComponent(item.code)}/`);
  const detailURL = item.publicId || item.id ? `/deploy/${encodeURIComponent(item.publicId || item.id || "")}` : appURL;
  const isLocked = Boolean(item.accessProtected);

  useEffect(() => {
    const iframe = previewRef.current;
    const node = cardRef.current;
    if (!iframe || !node || isLocked) return;

    let cancelQueued = false;
    let releaseTimer = 0;
    let slowTimer = 0;
    const loadHandler = () => {
      window.clearTimeout(slowTimer);
      setPreviewStatus("loaded");
      releaseTimer = window.setTimeout(() => marketplacePreviewScheduler.release(), previewSlotReleaseMs);
    };

    const observer = new IntersectionObserver((entries) => {
      if (!entries.some((entry) => entry.isIntersecting) || iframe.src || cancelQueued) return;
      setPreviewStatus("queued");
      marketplacePreviewScheduler
        .request()
        .then((release) => {
          if (cancelQueued) {
            release();
            return;
          }
          setPreviewStatus("loading");
          slowTimer = window.setTimeout(() => setPreviewStatus((current) => current === "loaded" ? current : "slow"), previewSlowHintMs);
          iframe.addEventListener("load", loadHandler, { once: true });
          iframe.src = `${appURL}?preview=1`;
        })
        .catch(() => setPreviewStatus("slow"));
    }, { rootMargin: "240px 0px" });

    observer.observe(node);
    return () => {
      cancelQueued = true;
      window.clearTimeout(releaseTimer);
      window.clearTimeout(slowTimer);
      iframe.removeEventListener("load", loadHandler);
      observer.disconnect();
      marketplacePreviewScheduler.release();
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
            loading="eager"
            scrolling="no"
            sandbox="allow-scripts allow-forms allow-popups allow-downloads allow-modals"
          />
        )}
        {!isLocked && previewStatus !== "loaded" && (
          <span className={`preview-status ${previewStatus === "slow" ? "slow" : ""}`}>
            {previewStatus === "slow" ? "预览加载较慢" : "准备预览"}
          </span>
        )}
      </div>
      <a className="open-corner" href={appURL} target="_blank" rel="noreferrer">
        打开<ExternalLink size={13} />
      </a>
      <div className="card-body">
        <div className="card-title-row">
          <div className="title-wrap">
            <h3>{title}</h3>
            <code>{item.code}</code>
          </div>
          <div className="badge-row">
            {item.isPinned && <span className="badge amber">置顶</span>}
            {isLocked && <span className="badge rose">加密</span>}
            {item.visibility === "public" && <span className="badge green">公开</span>}
          </div>
        </div>
        <p>{item.description || "这个作品还没有描述。"}</p>
        <div className="meta-grid">
          <span>赞 {item.likeCount || 0}</span>
          <span>访问 {item.viewCount || 0}</span>
          <span>{item.versionCount || 1} 个版本</span>
          <span>{formatDate(item.updatedAt || item.createdAt)}</span>
        </div>
        <div className="card-actions">
          <button className="button compact primary" type="button" onClick={() => onUse(item)}>
            <Workflow size={14} />使用
          </button>
          <a className="button compact" href={detailURL}>详情</a>
          {!isLocked && <a className="button compact" href={`/api/deploy/content?code=${encodeURIComponent(item.code)}&download=1`}><Download size={14} />源码</a>}
          <button className="icon-button" type="button" aria-label="点赞" onClick={like}><Heart size={15} /></button>
        </div>
      </div>
    </article>
  );
}

function MarketUseDrawer({ item, onClose }: { item: MarketplaceDeploy; onClose: () => void }) {
  const appURL = sameSiteURL(`/agent/${encodeURIComponent(item.code)}/`);
  const downloadURL = `/api/deploy/content?code=${encodeURIComponent(item.code)}&download=1`;
  const title = item.title || item.code;
  const cli = [
    `pagep get ${item.code} --download --output ./pagepilot-downloads`,
    `pagep deploy ./pagepilot-downloads/${item.code} --title "${title} remix" --description "基于 PagePilot 创作市场作品 ${item.code} 二次创作"`
  ].join("\n");
  const agentPrompt = [
    `请从 PagePilot 创作市场复用作品 code=${item.code}。`,
    "先下载源文件并检查文件结构、入口 HTML、资源引用和交互逻辑。",
    "在此基础上按我的新需求修改，然后作为新作品发布；不要覆盖原作品，除非我明确提供并确认原 code 的所有权。"
  ].join("\n");

  return (
    <div className="drawer show" role="dialog" aria-modal="true" aria-label="作品复用">
      <button className="drawer-shade" type="button" aria-label="关闭" onClick={onClose} />
      <section className="drawer-panel">
        <div className="drawer-head">
          <div>
            <span className="mini-label">USE FROM MARKET</span>
            <h2>{title}</h2>
          </div>
          <button className="button compact" type="button" onClick={onClose}>关闭</button>
        </div>
        <div className="drawer-body">
          <div className="drawer-preview">
            {item.accessProtected ? (
              <div className="locked-preview"><Lock size={28} /><strong>作品已加密</strong><span>请先打开作品输入访问密码</span></div>
            ) : (
              <iframe src={appURL} title={`${title} 预览`} sandbox="allow-scripts allow-forms allow-popups allow-downloads allow-modals" />
            )}
          </div>
          <div className="use-grid">
            <UseCard icon={<Download />} title="1. 下载源文件" text="多文件站点会下载 ZIP，单文件页面会下载源码。">
              {item.accessProtected ? <button className="button full" disabled>源码受密码保护</button> : <a className="button primary full" href={downloadURL}>下载源文件</a>}
            </UseCard>
            <UseCard icon={<Code2 />} title="2. CLI 命令行" text="复制命令到终端，下载后作为新作品发布。">
              <button className="button full" type="button" onClick={() => navigator.clipboard.writeText(cli)}><Copy size={14} />复制 CLI</button>
            </UseCard>
            <UseCard icon={<Bot />} title="3. Agent / MCP" text="复制给 Agent，由创作市场提供复用边界。">
              <button className="button full" type="button" onClick={() => navigator.clipboard.writeText(agentPrompt)}><Copy size={14} />复制指令</button>
            </UseCard>
          </div>
          <DocBlock title="CLI" lines={cli.split("\n")} />
          <DocBlock title="Agent / MCP 指令" lines={agentPrompt.split("\n")} />
        </div>
      </section>
    </div>
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

function DeployPage({ config }: { config: RuntimeConfig | null }) {
  const [mode, setMode] = useState<"single" | "multi">("single");
  const [filename, setFilename] = useState("index.html");
  const [content, setContent] = useState("<!doctype html>\n<html lang=\"zh-CN\">\n<head>\n  <meta charset=\"utf-8\">\n  <title>PagePilot App</title>\n</head>\n<body>\n  <h1>Hello PagePilot</h1>\n</body>\n</html>");
  const [files, setFiles] = useState<EditableDeployFile[]>([{ path: "index.html", content: "<h1>Hello PagePilot</h1>", isText: true, size: 24 }]);
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [visibility, setVisibility] = useState<"public" | "unlisted">("public");
  const [accessPassword, setAccessPassword] = useState("");
  const [enableCustom, setEnableCustom] = useState(false);
  const [customCode, setCustomCode] = useState("");
  const [createVersion, setCreateVersion] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [result, setResult] = useState<DeployResponse | null>(null);

  const totalSize = mode === "single" ? fileTextSize(content) : files.reduce((sum, file) => sum + (file.size ?? fileTextSize(file.content)), 0);
  const ready = mode === "single"
    ? content.trim() && filename.trim()
    : files.length > 0 && files.every((file) => file.path.trim() && (file.contentBase64 || file.content.trim()));

  const readUploaded = async (list: FileList | null) => {
    if (!list?.length) return;
    if (mode === "single") {
      const file = list[0];
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
    setResult(null);
    try {
      const payload: Record<string, unknown> = {
        title: title.trim(),
        description: description.trim(),
        visibility,
        accessPassword: accessPassword.trim() || undefined
      };
      if (enableCustom || createVersion) payload.code = customCode.trim();
      if (createVersion) payload.createVersion = true;
      if (mode === "single") {
        payload.filename = filename.trim() || "index.html";
        payload.content = content;
      } else {
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
      setError(err instanceof Error ? err.message : String(err));
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
            <a className="button" href="/api-docs.html"><Code2 size={18} />查看 API</a>
          </div>
        </div>
        <div className="preview-box">
          {mode === "single" ? (
            <iframe title="实时预览" srcDoc={content} sandbox="allow-scripts allow-forms allow-popups allow-downloads allow-modals" />
          ) : (
            <div>
              <FileArchive size={36} />
              <strong>多文件站点</strong>
              <span>共 {files.length} 个文件，部署后按 index.html 作为入口访问。</span>
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
        <div className="field-grid">
          <label className="field">
            <span>市场可见性</span>
            <select value={visibility} onChange={(event) => setVisibility(event.target.value as "public" | "unlisted")}>
              <option value="public">进入创作市场</option>
              <option value="unlisted">不进入市场</option>
            </select>
          </label>
          <label className="field">
            <span>访问密码</span>
            <input value={accessPassword} onChange={(event) => setAccessPassword(event.target.value)} type="password" placeholder="可选" />
          </label>
        </div>
        <label className="check-line">
          <input type="checkbox" checked={enableCustom || createVersion} disabled={createVersion} onChange={(event) => setEnableCustom(event.target.checked)} />
          自定义短链后缀
        </label>
        {(enableCustom || createVersion) && (
          <input className="standalone-input mono" value={customCode} onChange={(event) => setCustomCode(event.target.value)} placeholder={createVersion ? "输入已有 code" : "my-landing"} />
        )}
        <label className="check-line">
          <input
            type="checkbox"
            checked={createVersion}
            onChange={(event) => {
              setCreateVersion(event.target.checked);
              if (event.target.checked) setEnableCustom(true);
            }}
          />
          更新已有发布，追加为新版本
        </label>
        {createVersion && <div className="hint-box">只有站点所有者或管理员可以更新已有 code；不会顶掉其他用户的作品。</div>}

        {mode === "single" ? (
          <>
            <label className="field">
              <span>入口文件名</span>
              <input value={filename} onChange={(event) => setFilename(event.target.value)} placeholder="index.html 或 README.md" />
            </label>
            <label className="upload-line">
              <input type="file" accept=".html,.htm,.md,.txt" onChange={(event) => void readUploaded(event.target.files)} />
              <Upload size={18} />上传 HTML / Markdown
            </label>
            <textarea className="code-input" value={content} onChange={(event) => setContent(event.target.value)} placeholder="<!doctype html>..." />
          </>
        ) : (
          <>
            <label className="upload-line">
              <input type="file" multiple webkitdirectory="" onChange={(event) => void readUploaded(event.target.files)} />
              <Upload size={18} />上传目录
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
        {error && <div className="error-box">{error}</div>}
        {result && <DeployResult result={result} />}
      </aside>
    </section>
  );
}

function DeployResult({ result }: { result: DeployResponse }) {
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
    <div className="result-box">
      <strong>部署成功</strong>
      {rows.map(([label, value]) => (
        <div className="result-row" key={label}>
          <span>{label}</span>
          <code>{value}</code>
          <button type="button" onClick={() => navigator.clipboard.writeText(value)}><Copy size={14} /></button>
        </div>
      ))}
      <div className="card-actions">
        <a className="button compact primary" href={appURL} target="_blank" rel="noreferrer">打开</a>
        {result.id && <a className="button compact" href={`/deploy/${encodeURIComponent(result.id)}`}>详情</a>}
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
        <FeatureTile icon={<KeyRound />} title="Token 登录" desc="登录后在后台创建 Token，Agent 使用 Token 管理自己的站点和屏幕。" />
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
            "修改源码后使用 pagep deploy 发布为新作品。",
            "不要默认覆盖原作品；只有用户明确提供并确认原 code 所有权时才更新。"
          ]}
        />
        <DocBlock
          title="绑定用户"
          lines={[
            "pagep token create --label agent --ttl-seconds 2592000",
            "pagep claim-session --token YOUR_TOKEN",
            "claim-session 会把当前匿名 session 发布过的站点绑定到这个用户。"
          ]}
        />
        <DocBlock
          title="屏幕投放"
          lines={[
            "pagep screens list --token YOUR_TOKEN",
            "pagep screens publish SCREEN_ID --code my-landing --token YOUR_TOKEN",
            "pagep screens command SCREEN_ID refresh --token YOUR_TOKEN"
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
            "Agent 复用公开作品时默认发布为新作品。",
            "加密作品必须先通过访问密码授权。"
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

  return (
    <section className="content-page">
      <div className="sub-hero screen-hero">
        <div className="eyebrow"><Monitor size={16} />屏幕投放</div>
        <h1>把 PagePilot 应用发布到广告屏</h1>
        <p>Android 屏幕 App 一次绑定到用户后，可由后台、Skill 或 MCP 投放应用、刷新 WebView、截图和远程控制。</p>
        <div className="hero-actions">
          <a className="button primary" href="/admin"><Monitor size={18} />管理屏幕</a>
          <a className="button" href="/agents/"><Bot size={18} />用 Agent 投屏</a>
        </div>
      </div>
      <div className="panel-table">
        <div className="section-head">
          <div>
            <h2>我的屏幕</h2>
            <p>{error ? "登录注册用户后可查看自己的屏幕。" : `服务器：${currentOrigin()}`}</p>
          </div>
        </div>
        {error ? <div className="empty-wide">{error}</div> : (
          <div className="screen-list">
            {screens.map((screen) => (
              <div className="screen-row" key={screen.id}>
                <strong>{screen.name || screen.id}</strong>
                <span>{screen.status || "未知"} / {screen.currentSiteCode || "空闲"}</span>
                <code>{formatDeviceInfo(screen.deviceInfo)}</code>
              </div>
            ))}
            {!screens.length && <div className="empty-wide">还没有绑定屏幕。</div>}
          </div>
        )}
      </div>
    </section>
  );
}

function ApiDocsPage() {
  const endpoints = [
    ["POST", "/api/deploy", "发布 HTML、Markdown、ZIP 或多文件静态站点"],
    ["GET", "/api/deploys", "创作市场列表，支持搜索、点赞排行和管理员置顶"],
    ["GET", "/api/deploy/content", "读取或下载源码，支持多文件 ZIP"],
    ["POST", "/api/session", "创建或读取匿名 Agent 会话"],
    ["POST", "/api/session/claim", "把匿名发布认领到当前用户"],
    ["POST", "/api/deploys/{code}/access", "输入访问密码，获得短时访问票据"],
    ["GET", "/api/screens", "列出当前用户绑定的屏幕"],
    ["POST", "/api/screens/{screenId}/publish", "投放应用到屏幕"],
    ["POST", "/api/screens/{screenId}/command", "刷新、休眠、唤醒或软关机"]
  ];
  return (
    <section className="content-page">
      <div className="sub-hero">
        <div className="eyebrow"><Code2 size={16} />API 文档</div>
        <h1>一套 API 服务前台、Agent 和屏幕</h1>
        <p>React 前台、pagep Skill、MCP 和屏幕 App 使用同一套 API。完整机器可读契约请查看 OpenAPI。</p>
        <div className="hero-actions">
          <a className="button primary" href="/openapi.json">查看 openapi.json</a>
          <a className="button" href="/agents/">交给 Agent 部署</a>
        </div>
      </div>
      <div className="endpoint-list">
        {endpoints.map(([method, path, desc]) => (
          <div className="endpoint-row" key={`${method}-${path}`}>
            <span className={`method ${method}`}>{method}</span>
            <code>{path}</code>
            <p>{desc}</p>
          </div>
        ))}
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

function Footer() {
  return (
    <footer className="footer">
      <span>PagePilot / Agent 网页发布 / 创作市场 / Screen 投放</span>
      <a href="/admin">返回后台</a>
    </footer>
  );
}

declare module "react" {
  interface InputHTMLAttributes<T> {
    webkitdirectory?: string;
  }
}
