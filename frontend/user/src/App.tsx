import {
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
  Upload,
  UserRound,
  Workflow
} from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { api } from "./api";
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

const navItems: Array<{ page: Page; label: string; href: string }> = [
  { page: "home", label: "首页", href: "/" },
  { page: "market", label: "创作市场", href: "/market" },
  { page: "deploy", label: "手动部署", href: "/deploy.html" },
  { page: "screens", label: "广告屏", href: "/screens/" }
];

function getPageFromPath(pathname: string): Page {
  if (pathname.startsWith("/market")) return "market";
  if (pathname.startsWith("/deploy.html")) return "deploy";
  if (pathname.startsWith("/agents")) return "agents";
  if (pathname.startsWith("/screens")) return "screens";
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
      <main className={`page-main ${page === "market" ? "market-main" : ""}`}>
        {page === "home" && <HomePage config={config} onNavigate={navigate} />}
        {page === "market" && <MarketPage config={config} session={session} />}
        {page === "deploy" && <DeployPage config={config} />}
        {page === "agents" && <AgentsPage config={config} />}
        {page === "screens" && <ScreensPage />}
      </main>
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
    <svg className="logo" viewBox="0 0 96 96" aria-hidden="true">
      <defs>
        <linearGradient id="pagepilot-logo-bg" x1="8" x2="88" y1="8" y2="88"><stop stopColor="#0B102F" /><stop offset="0.54" stopColor="#0F4C81" /><stop offset="1" stopColor="#0891B2" /></linearGradient>
        <linearGradient id="pagepilot-logo-wing" x1="18" x2="78" y1="24" y2="72"><stop stopColor="#67E8F9" /><stop offset="1" stopColor="#A78BFA" /></linearGradient>
        <linearGradient id="pagepilot-logo-page" x1="31" x2="70" y1="33" y2="76"><stop stopColor="#FFFFFF" /><stop offset="1" stopColor="#E0F7FF" /></linearGradient>
      </defs>
      <rect x="6" y="6" width="84" height="84" rx="25" fill="url(#pagepilot-logo-bg)" />
      <path d="M18 61C34 37 58 25 82 18C75 42 62 65 38 78L41 58L18 61Z" fill="url(#pagepilot-logo-wing)" opacity="0.95" />
      <path d="M28 62L16 80L38 72" fill="none" stroke="#67E8F9" strokeWidth="4" strokeLinecap="round" strokeLinejoin="round" opacity="0.8" />
      <rect x="30" y="32" width="38" height="42" rx="11" fill="url(#pagepilot-logo-page)" stroke="rgba(255,255,255,.72)" strokeWidth="2" />
      <path d="M30 44a12 12 0 0 1 12-12h14a12 12 0 0 1 12 12v5H30z" fill="#0EA5E9" />
      <circle cx="39" cy="42" r="2.5" fill="#F472B6" /><circle cx="49" cy="42" r="2.5" fill="#FDE68A" /><circle cx="59" cy="42" r="2.5" fill="#86EFAC" />
      <path d="M40 58h18M40 66h12" stroke="#0F172A" strokeWidth="4" strokeLinecap="round" opacity="0.78" />
      <rect x="64" y="54" width="18" height="18" rx="6" fill="#111827" stroke="#67E8F9" strokeWidth="2" />
      <path d="M69 63h8M73 59v8" stroke="#67E8F9" strokeWidth="2.4" strokeLinecap="round" />
    </svg>
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

function HomePage({
  config,
  onNavigate
}: {
  config: RuntimeConfig | null;
  onNavigate: (page: Page, href: string) => void;
}) {
  const publishLimit = config?.anonymousPolicy?.deployLimit == null ? "-" : String(config.anonymousPolicy.deployLimit);
  const maxSize = formatSize(config?.limits?.maxSiteTotalBytes);
  const fileLimit = config?.limits?.maxFilesPerSite == null ? "-" : String(config.limits.maxFilesPerSite);

  return (
    <div className="home-page">
      <HeroBarrage />
      <section className="hero-band hero-band-refined">
        <div className="hero-copy">
          <div className="eyebrow"><Sparkles size={16} />Agent native publishing platform</div>
          <h1>把想法交给 Agent，把上线交给 PagePilot</h1>
          <p>
            面向 AI 生成应用的发布平台：HTML、Markdown、ZIP、多文件静态站点都能一键上线。
            PagePilot 继续负责访问密码、版本回滚、锁定下架、屏幕投放和创作市场复用。
          </p>
          <div className="hero-pills">
            <span><Bot size={15} />告诉 Agent 需求</span>
            <span><Rocket size={15} />秒级上线应用</span>
            <span><Lock size={15} />访问密码加密</span>
            <span><Layers size={15} />版本管理回滚</span>
          </div>
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
        <div className="hero-product-flow" aria-label="PagePilot 发布流程">
          <div className="hero-flow-header">
            <span>PAGEPILOT PIPELINE</span>
            <strong>从需求到可访问链接</strong>
          </div>
          <div className="flow-node active">
            <Bot size={18} />
            <div><strong>Agent 生成</strong><span>HTML / MD / ZIP / 多文件站点</span></div>
          </div>
          <div className="flow-node">
            <Upload size={18} />
            <div><strong>PagePilot 发布</strong><span>路径或泛域名模式，保留原 code 版本链</span></div>
          </div>
          <div className="flow-node">
            <ShieldCheck size={18} />
            <div><strong>权限与安全</strong><span>公开、私有、访问密码、隔离预览</span></div>
          </div>
          <div className="flow-node">
            <PackageOpen size={18} />
            <div><strong>创作市场复用</strong><span>收藏点赞、下载源码、CLI/MCP 二次创作</span></div>
          </div>
          <div className="hero-flow-footer">
            <span><strong>{publishLimit}</strong><em>匿名额度</em></span>
            <span><strong>{maxSize}</strong><em>整站上限</em></span>
            <span><strong>{fileLimit}</strong><em>文件数</em></span>
          </div>
        </div>
      </section>

      <section className="capability-section home-capabilities">
        <div className="section-head">
          <div>
            <h2>不是普通 HTML 托管，是 Agent 的交付控制台</h2>
            <p>把 jpage 这类作品市场的复用体验吸收进来，但 PagePilot 更强调版本、加密、后台治理和屏幕投放。</p>
          </div>
        </div>
        <div className="feature-board feature-board-open">
          <FeatureTile icon={<Rocket />} title="秒级上线应用" desc="HTML、Markdown、ZIP 和多文件静态站点都能发布。" />
          <FeatureTile icon={<Lock />} title="可加密网页" desc="给页面设置访问密码，公开展示和访问权限分开控制。" />
          <FeatureTile icon={<Layers />} title="版本管理" desc="每次修改形成新版本，可查看、回滚、锁定和下架。" />
          <FeatureTile icon={<PackageOpen />} title="创作市场" desc="公开作品可预览、收藏、下载源码，并交给 Agent/MCP 继续创作。" />
          <FeatureTile icon={<Monitor />} title="广告屏投放" desc="绑定屏幕，远程发布、刷新、截图，适合展厅和门店大屏。" />
          <FeatureTile icon={<KeyRound />} title="CLI / MCP / API" desc="网页、pagep CLI、Skill 和 MCP 围绕同一套发布能力。" />
        </div>
      </section>

      <section className="flow-section">
        <div className="section-head">
          <div>
            <h2>从想法到链接，只走一条线</h2>
            <p>用户说需求，Agent 生成页面，PagePilot 负责上线、治理和复用；后续迭代继续追加版本，不把作品散成一堆地址。</p>
          </div>
        </div>
        <div className="flow-grid">
          <FlowStep label="01" title="告诉 Agent 需求" text="让 Agent 生成网页、修复样式、整理多文件结构。" />
          <FlowStep label="02" title="发布到 PagePilot" text="使用 pagep Skill、MCP 或手动部署，把文件包推到服务器。" />
          <FlowStep label="03" title="分享或加密" text="公开展示、访问密码、版本历史和屏幕投放按需开启。" />
          <FlowStep label="04" title="进入创作市场" text="公开作品可以下载源码、复制 CLI，或交给 Agent 二次创作。" />
        </div>
      </section>
    </div>
  );
}


function MarketPage({ config, session }: { config: RuntimeConfig | null; session: SessionInfo | null }) {
  const pageSize = 24;
  const [items, setItems] = useState<MarketplaceDeploy[]>([]);
  const [selected, setSelected] = useState<MarketplaceDeploy | null>(null);
  const [detail, setDetail] = useState<MarketplaceDeploy | null>(null);
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

  const openDetail = useCallback(async (item: MarketplaceDeploy) => {
    setDetailLoading(true);
    setDetail(item);
    try {
      const key = item.publicId || item.id || item.code;
      const data = await api<MarketplaceDeploy>(`/api/deploys/${encodeURIComponent(key)}`);
      setDetail(data);
    } finally {
      setDetailLoading(false);
    }
  }, []);

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
                  setDetail(null);
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
                  setDetail(null);
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
            <span>加密作品会出现在市场，但预览和源码下载会继续受访问密码保护。</span>
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
                {kind === "all" && (
                  <div className="market-inline-actions">
                    <a className="button compact primary" href="/agents/"><Bot size={15} />交给 Agent</a>
                    <a className="button compact" href="/deploy.html"><Upload size={15} />手动部署</a>
                  </div>
                )}
              </div>
            </div>
          )}

          {detail ? (
            <MarketDetailViewFull
              item={detail}
              loading={detailLoading}
              session={session}
              onBack={() => setDetail(null)}
              onUse={setSelected}
            />
          ) : (
            <>
              <div className="market-feed-head">
                <div>
                  <h2>{categories.find((item) => item.key === category)?.label || "全部作品"}</h2>
                  <p>
                    {loading
                      ? "正在加载作品"
                      : `共 ${total} 个作品，${pinnedCount ? `本页 ${pinnedCount} 个精选，` : ""}可预览、复用和交给 Agent 二次创作。`}
                  </p>
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

      {selected && <MarketUseDrawer item={selected} session={session} onClose={() => setSelected(null)} />}
    </section>
  );
}

function MarketplaceCard({
  item,
  onChanged,
  onUse,
  onDetail
}: {
  item: MarketplaceDeploy;
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
    try {
      await api(`/api/deploys/${encodeURIComponent(item.code)}/favorite`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ favorited: !item.favorited })
      });
      onChanged();
    } catch (err) {
      window.alert(err instanceof Error ? err.message : "请先登录，或先发布一次以建立本浏览器身份。");
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
            sandbox="allow-scripts allow-forms allow-popups allow-downloads allow-modals"
          />
        )}
        <div className="card-quickbar">
          <span><Eye size={13} />{item.viewCount || 0}</span>
          <span><Layers size={13} />{item.versionCount || 1}</span>
          <button className={`quick-icon ${item.favorited ? "active" : ""}`} type="button" aria-label="收藏" onClick={toggleFavorite}>
            <Bookmark size={14} />{item.favoriteCount || 0}
          </button>
          <button className="quick-icon" type="button" aria-label="点赞" onClick={like}>
            <Heart size={14} />{item.likeCount || 0}
          </button>
          <a className="quick-open" href={appURL} target="_blank" rel="noreferrer" aria-label="打开应用">
            <ExternalLink size={14} />
          </a>
        </div>
      </div>
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
        <div className="market-card-tags">
          <span>{marketFileType(item)}</span>
          {item.category && <span>{marketCategoryLabel(item.category)}</span>}
          {item.owned && <span>我的发布</span>}
          {item.accessProtected && <span>访问密码</span>}
        </div>
        <div className="card-footnote">
          <span>{formatDate(item.updatedAt || item.createdAt)}</span>
        </div>
        <div className="card-actions">
          <button className="button compact primary" type="button" onClick={() => onUse(item)}>
            <Workflow size={14} />使用
          </button>
          <button className="button compact" type="button" onClick={() => onDetail(item)}>详情</button>
        </div>
      </div>
    </article>
  );
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
  const canDownload = Boolean(session?.success) && !isLocked;
  const canManage = Boolean(item.owned || session?.isAdmin);
  const [versions, setVersions] = useState<VersionItem[]>([]);
  const [viewport, setViewport] = useState<PreviewViewport>("desktop");
  const [busyVersion, setBusyVersion] = useState<number | null>(null);
  const [showAllVersions, setShowAllVersions] = useState(false);

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
  const visibleVersions = showAllVersions ? versions : versions.slice(0, 3);
  const updateURL = `/deploy.html?code=${encodeURIComponent(item.code)}&version=1`;

  return (
    <section className="market-detail-layout">
      <div className="market-detail-stage">
        <div className="detail-preview-frame">
          <div className="detail-preview-toolbar">
            <div>
              <strong>v{currentVersion?.versionNumber || item.versionCount || 1} 预览</strong>
              <span>{isLocked ? "已加密，打开后输入访问密码" : "桌面、平板、手机三端查看"}</span>
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
              <iframe src={`${appURL}?preview=1`} title={`${item.title || item.code} 预览`} sandbox="allow-scripts allow-forms allow-popups allow-downloads allow-modals" />
            )}
          </div>
        </div>
      </div>
      <aside className="market-detail-card">
        <button className="button compact" type="button" onClick={onBack}><ChevronLeft size={15} />返回列表</button>
        <div>
          <span className="mini-label">{loading ? "LOADING" : "MARKET DETAIL"}</span>
          <h2>{item.title || item.code}</h2>
          <p>{item.description || "这个作品还没有描述。"}</p>
        </div>
        <div className="market-detail-meta">
          <span><strong>{marketCategoryLabel(item.category)}</strong><em>分类</em></span>
          <span><strong>{marketFileType(item)}</strong><em>类型</em></span>
          <span><strong>{item.likeCount || 0}</strong><em>点赞</em></span>
          <span><strong>{item.viewCount || 0}</strong><em>访问</em></span>
          <span><strong>{item.versionCount || versions.length || 1}</strong><em>版本</em></span>
          <span><strong>{formatDate(item.updatedAt || item.createdAt)}</strong><em>更新</em></span>
        </div>
        <div className="market-detail-actions">
          <button className="button primary full" type="button" onClick={() => onUse(item)}><Workflow size={16} />使用此模板</button>
          <a className="button full" href={appURL} target="_blank" rel="noreferrer"><ExternalLink size={16} />打开应用</a>
          <button className="button full" type="button" onClick={() => void copyText(appURL)}><Copy size={16} />复制链接</button>
          {canManage && <a className="button full" href={updateURL}><Upload size={16} />更新版本</a>}
          {canDownload ? (
            <a className="button full" href={`/api/deploy/content?code=${encodeURIComponent(item.code)}&download=1`}><Download size={16} />下载源文件</a>
          ) : (
            <a className="button full" href="/admin?mode=login"><Download size={16} />登录后下载源文件</a>
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
          {versions.length > 3 && (
            <button className="button compact full" type="button" onClick={() => setShowAllVersions((value) => !value)}>
              {showAllVersions ? "收起版本" : `查看更多版本（${versions.length - 3}）`}
            </button>
          )}
          {!versions.length && <span className="muted-line">暂无版本记录</span>}
        </div>
      </aside>
    </section>
  );
}

function MarketDetailView({
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
  const canDownload = Boolean(session?.success) && !isLocked;
  const [versions, setVersions] = useState<VersionItem[]>([]);

  useEffect(() => {
    api<VersionsResponse>(`/api/deploys/${encodeURIComponent(item.code)}/versions`)
      .then((data) => setVersions((data.versions || []).slice().sort((a, b) => Number(b.versionNumber) - Number(a.versionNumber))))
      .catch(() => setVersions([]));
  }, [item.code]);

  return (
    <section className="market-detail-layout">
      <div className={`market-detail-preview ${isLocked ? "is-locked" : ""}`}>
        {isLocked ? (
          <div className="locked-preview">
            <Lock size={36} />
            <strong>网页已加密</strong>
            <span>打开应用并输入访问密码后才能查看内容。</span>
          </div>
        ) : (
          <iframe src={appURL} title={`${item.title || item.code} 预览`} sandbox="allow-scripts allow-forms allow-popups allow-downloads allow-modals" />
        )}
      </div>
      <aside className="market-detail-card">
        <button className="button compact" type="button" onClick={onBack}><ChevronLeft size={15} />返回列表</button>
        <div>
          <span className="mini-label">{loading ? "LOADING" : "MARKET DETAIL"}</span>
          <h2>{item.title || item.code}</h2>
          <p>{item.description || "这个作品还没有描述。"}</p>
        </div>
        <div className="market-detail-meta">
          <span><strong>{marketCategoryLabel(item.category)}</strong><em>分类</em></span>
          <span><strong>{marketFileType(item)}</strong><em>类型</em></span>
          <span><strong>{item.likeCount || 0}</strong><em>点赞</em></span>
          <span><strong>{item.viewCount || 0}</strong><em>访问</em></span>
          <span><strong>{item.versionCount || 1}</strong><em>版本</em></span>
          <span><strong>{formatDate(item.updatedAt || item.createdAt)}</strong><em>更新</em></span>
        </div>
        <div className="market-detail-actions">
          <button className="button primary full" type="button" onClick={() => onUse(item)}>
            <Workflow size={16} />使用此模板
          </button>
          <a className="button full" href={appURL} target="_blank" rel="noreferrer"><ExternalLink size={16} />打开应用</a>
          {canDownload ? (
            <a className="button full" href={`/api/deploy/content?code=${encodeURIComponent(item.code)}&download=1`}><Download size={16} />下载源文件</a>
          ) : (
            <a className="button full" href="/admin?mode=login"><Download size={16} />登录后下载源文件</a>
          )}
        </div>
        <div className="detail-qr-block">
          <img src={`/api/deploys/${encodeURIComponent(item.code)}/qr`} alt={`${item.code} 二维码`} />
          <span>扫码打开当前应用</span>
        </div>
        <div className="detail-version-list">
          <strong>历史版本</strong>
          {versions.slice(0, 8).map((version) => (
            <a href={`/agent/${encodeURIComponent(item.code)}/versions/${version.versionNumber}/`} target="_blank" rel="noreferrer" key={version.versionNumber}>
              <span>v{version.versionNumber}{version.isCurrent ? " · 当前" : ""}</span>
              <em>{formatDate(version.createdAt)}</em>
            </a>
          ))}
          {!versions.length && <span className="muted-line">暂无版本记录</span>}
        </div>
      </aside>
    </section>
  );
}

function marketFileType(item: MarketplaceDeploy) {
  const name = (item.filename || item.filePath || "").toLowerCase();
  if (name.endsWith(".md") || name.endsWith(".markdown")) return "MD";
  if (name.endsWith(".html") || name.endsWith(".htm")) return "HTML";
  return "ZIP / Site";
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

function MarketUseDrawer({ item, session, onClose }: { item: MarketplaceDeploy; session: SessionInfo | null; onClose: () => void }) {
  const downloadURL = `/api/deploy/content?code=${encodeURIComponent(item.code)}&download=1`;
  const title = item.title || item.code;
  const canDownload = Boolean(session?.success) && !item.accessProtected;
  const cli = [
    `pagep get ${item.code} --download --output ./pagepilot-downloads`,
    `pagep deploy ./pagepilot-downloads/${item.code} --title "${title} remix" --description "基于 PagePilot 创作市场作品 ${item.code} 二次创作"`
  ].join("\n");
  const agentPrompt = [
    `请从 PagePilot 创作市场复用作品 code=${item.code}。`,
    "先下载源文件并检查文件结构、入口 HTML、资源引用和交互逻辑。",
    "在此基础上按我的新需求修改，然后作为新作品发布；不要覆盖原作品，除非我明确提供并确认原 code 的所有权。"
  ].join("\n");

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
          <div className="use-grid">
            <UseCard icon={<Download />} title="1. 下载源文件" text="多文件站点会下载 ZIP，单文件页面会下载源码。">
              {canDownload ? (
                <a className="button primary full" href={downloadURL}>下载源文件</a>
              ) : item.accessProtected ? (
                <button className="button full" disabled>源码受密码保护</button>
              ) : (
                <a className="button full" href="/admin?mode=login">登录后下载</a>
              )}
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

function DeployPage({ config }: { config: RuntimeConfig | null }) {
  const [mode, setMode] = useState<"single" | "multi">("single");
  const [filename, setFilename] = useState("index.html");
  const [content, setContent] = useState("<!doctype html>\n<html lang=\"zh-CN\">\n<head>\n  <meta charset=\"utf-8\">\n  <title>PagePilot App</title>\n</head>\n<body>\n  <h1>Hello PagePilot</h1>\n</body>\n</html>");
  const [files, setFiles] = useState<EditableDeployFile[]>([{ path: "index.html", content: "<h1>Hello PagePilot</h1>", isText: true, size: 24 }]);
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [visibility, setVisibility] = useState<"public" | "unlisted">("public");
  const [deployCategory, setDeployCategory] = useState<MarketCategory>("");
  const [marketCategories, setMarketCategories] = useState<MarketCategoryInfo[]>([]);
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

  useEffect(() => {
    api<MarketCategoriesResponse>("/api/market/categories")
      .then((data) => {
        const next = data.categories || [];
        setMarketCategories(next);
        setDeployCategory((current) => current || next[0]?.slug || "");
      })
      .catch(() => setMarketCategories([]));
  }, []);

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const code = params.get("code")?.trim();
    const shouldCreateVersion = params.get("version") === "1" || params.get("mode") === "version";
    if (!code || !shouldCreateVersion) return;
    setCreateVersion(true);
    setEnableCustom(true);
    setCustomCode(code);
  }, []);

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
    setResult(null);
    try {
      const payload: Record<string, unknown> = {
        title: title.trim(),
        description: description.trim(),
        visibility,
        category: createVersion ? undefined : deployCategory,
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
            <iframe title="实时预览" srcDoc={content} sandbox="allow-scripts allow-forms allow-popups allow-downloads allow-modals" />
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
              <option value="public">进入创作市场</option>
              <option value="unlisted">不进入市场</option>
            </select>
          </label>
          <label className="field">
            <span>作品分类</span>
            <select value={deployCategory} onChange={(event) => setDeployCategory(event.target.value as MarketCategory)} disabled={createVersion}>
              {marketCategories.map((item) => <option value={item.slug} key={item.slug}>{item.label}</option>)}
            </select>
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
        {createVersion && <div className="hint-box">只有站点所有者或管理员可以更新已有 code；不会覆盖其他用户的作品。</div>}

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
        <a className="button compact" href="/market" target="_blank" rel="noreferrer">创作市场</a>
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
        <div className="screen-capability-grid">
          <FeatureTile icon={<Monitor />} title="屏幕绑定" desc="Android 屏幕 App 一次绑定，后续由账号统一管理。" />
          <FeatureTile icon={<Rocket />} title="远程投放" desc="从后台、Skill 或 MCP 选择 PagePilot 应用投放到屏幕。" />
          <FeatureTile icon={<Eye />} title="截图回传" desc="请求屏幕截图，确认现场展示是否正常。" />
          <FeatureTile icon={<Workflow />} title="运行命令" desc="支持刷新、休眠、唤醒和基础远程控制。" />
        </div>
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
