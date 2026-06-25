import {
  Bot,
  ChevronLeft,
  ChevronRight,
  Code2,
  Copy,
  Download,
  ExternalLink,
  Heart,
  Home,
  KeyRound,
  Layers,
  Lock,
  Monitor,
  Rocket,
  Search,
  ShieldCheck,
  Sparkles,
  Trash2,
  Upload,
  Workflow
} from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { api, publicURL } from "./api";
import type {
  DeployFilePayload,
  DeployResponse,
  MarketplaceDeploy,
  RuntimeConfig,
  ScreenInfo,
  SessionInfo
} from "./types";

type Page = "home" | "deploy" | "agents" | "screens" | "apiDocs";
type SortMode = "newest" | "oldest" | "likes_desc" | "views_desc";
type AgentDocTab = "skill" | "mcp";

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

const navItems: Array<{ page: Page; label: string; href: string }> = [
  { page: "home", label: "首页", href: "/" },
  { page: "agents", label: "Skill & MCP", href: "/agents/" },
  { page: "screens", label: "屏幕投放", href: "/screens/" },
  { page: "deploy", label: "手动部署", href: "/deploy.html" },
  { page: "apiDocs", label: "API 文档", href: "/api-docs.html" }
];

function getPageFromPath(pathname: string): Page {
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
  return d.toLocaleString("zh-CN", { hour12: false });
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
        {page === "home" && <HomePage config={config} />}
        {page === "deploy" && <DeployPage config={config} />}
        {page === "agents" && <AgentsPage config={config} />}
        {page === "screens" && <ScreensPage config={config} />}
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
            <a className="nav-button dark" href="/admin">后台管理</a>
          ) : (
            <>
              <a className="nav-button ghost" href="/admin">登录</a>
              <a className="nav-button dark" href="/admin?mode=register">注册</a>
            </>
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

function HomePage({ config }: { config: RuntimeConfig | null }) {
  const pageSize = 24;
  const [items, setItems] = useState<MarketplaceDeploy[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [query, setQuery] = useState("");
  const [sort, setSort] = useState<SortMode>("newest");
  const [loading, setLoading] = useState(true);
  const pageCount = Math.max(1, Math.ceil(total / pageSize));
  const safePage = Math.min(page, pageCount);
  const pageStart = total === 0 ? 0 : (safePage - 1) * pageSize + 1;
  const pageEnd = Math.min(total, safePage * pageSize);
  const pinnedCount = items.filter((item) => item.isPinned).length;
  const marketplaceSummary = total === 0
    ? "暂无匹配作品，管理员置顶优先展示。"
    : `共 ${total} 个公开作品，当前显示 ${pageStart}-${pageEnd}，${pinnedCount ? `本页 ${pinnedCount} 个置顶，` : ""}点赞排行保留。`;

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const params = new URLSearchParams({
        page: String(page),
        pageSize: String(pageSize),
        sort,
        q: query
      });
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
    if (!loading && page > pageCount) {
      setPage(pageCount);
    }
  }, [loading, page, pageCount]);

  return (
    <div className="home-page">
      <HeroBarrage />
      <section className="hero-band">
        <div className="hero-copy">
          <div className="eyebrow"><Sparkles size={16} />开放 HTML 应用商城</div>
          <h1>部署、分享，也能投到屏幕</h1>
          <p>
            PagePilot 托管 HTML、CSS、JS 和图片等多文件静态站点。Agent 可以直接发布，注册用户可以把应用投放到绑定的广告屏。
          </p>
          <div className="hero-actions">
            <a className="button primary" href="/deploy.html"><Upload size={18} />手动部署</a>
            <a className="button" href="/agents/"><Bot size={18} />Skill & MCP</a>
            <a className="button" href="/screens/"><Monitor size={18} />屏幕投放</a>
          </div>
        </div>
        <div className="feature-board" aria-label="PagePilot 特性">
          <FeatureTile icon={<Rocket />} title="秒级上线" desc="单 HTML 或多文件整站都能发布。" />
          <FeatureTile icon={<Lock />} title="公开/加密/未公开" desc="商城展示和访问密码分离控制。" />
          <FeatureTile icon={<Layers />} title="版本管理" desc="最新版本优先展示，支持回滚和锁定。" />
          <FeatureTile icon={<Monitor />} title="硬件屏幕" desc="注册用户可投放、刷新、截图和远程休眠。" />
        </div>
      </section>

      <section className="stat-strip">
        <Metric label="在线作品" value={String(total || "-")} note="商城可检索" />
        <Metric label="匿名额度" value={config?.anonymousPolicy?.deployLimit == null ? "-" : String(config.anonymousPolicy.deployLimit)} note="未登录发布记录到匿名 Agent" />
        <Metric label="发布冷却" value={`${config?.cooldownSeconds ?? "-"}s`} note="防滥用限流" />
        <Metric label="整站上限" value={formatSize(config?.limits?.maxSiteTotalBytes)} note="多文件上传" />
      </section>

      <section className="market-section" id="marketplace">
        <div className="section-head market-head">
          <div>
            <h2>应用商城</h2>
            <p>
              {loading
                ? "正在加载作品"
                : marketplaceSummary}
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
              <option value="newest">最新优先</option>
              <option value="likes_desc">点赞最高</option>
              <option value="views_desc">访问最多</option>
              <option value="oldest">最早发布</option>
            </select>
          </div>
        </div>
        {loading ? (
          <div className="card-grid">{Array.from({ length: 6 }).map((_, index) => <div className="skeleton" key={index} />)}</div>
        ) : (
          <div className="card-grid">
            {items.map((item) => <MarketplaceCard key={item.code} item={item} onChanged={load} />)}
            {!items.length && <div className="empty-wide">还没有匹配作品。</div>}
          </div>
        )}
        {!loading && total > 0 && (
          <div className="market-pagination" aria-label="应用商城分页">
            <div className="page-note">
              管理员置顶优先展示，排序规则继续作用于其余作品。
            </div>
            <div className="pager">
              <button
                className="button compact"
                type="button"
                disabled={safePage <= 1}
                onClick={() => setPage((current) => Math.max(1, current - 1))}
              >
                <ChevronLeft size={15} />
                上一页
              </button>
              <span className="page-indicator">第 {safePage} / {pageCount} 页</span>
              <button
                className="button compact"
                type="button"
                disabled={safePage >= pageCount}
                onClick={() => setPage((current) => Math.min(pageCount, current + 1))}
              >
                下一页
                <ChevronRight size={15} />
              </button>
            </div>
          </div>
        )}
      </section>
    </div>
  );
}

function HeroBarrage() {
  const prompts = [
    "帮我做一个打砖块小游戏", "把这份日报做成网页", "生成一个手机端抽签页面", "做个 AI 对话型登录页",
    "用 HTML 做产品定价页", "做一个课堂计分板", "把表格变成可视化看板", "做个活动报名页",
    "帮我部署到 PagePilot", "生成后直接给我链接", "顺手写个项目简介", "代码不要变，样式升级",
    "做一个 SaaS 状态页", "生成一个 API 测试页面", "做个二维码落地页", "把 Markdown 转成漂亮网页"
  ];
  const lanes: Array<[string[], boolean, number, number, number]> = [
    [prompts.slice(0, 10), false, 52, 4, 0.42],
    [prompts.slice(6), true, 66, 13, 0.34],
    [prompts.slice(8).concat(prompts.slice(0, 4)), false, 58, 23, 0.30],
    [prompts.slice(3, 13), true, 72, 33, 0.26],
    [prompts.slice(0, 8), false, 64, 45, 0.30],
    [prompts.slice(8), true, 78, 56, 0.24],
    [prompts.slice(4, 14), false, 70, 67, 0.26],
    [prompts.slice(2, 12), true, 62, 78, 0.22],
    [prompts.slice(7).concat(prompts.slice(0, 5)), false, 84, 89, 0.20]
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

function Metric({ label, value, note }: { label: string; value: string; note: string }) {
  return (
    <div className="metric">
      <span>{label}</span>
      <strong>{value}</strong>
      <em>{note}</em>
    </div>
  );
}

function MarketplaceCard({ item, onChanged }: { item: MarketplaceDeploy; onChanged: () => void }) {
  const previewRef = useRef<HTMLIFrameElement | null>(null);
  const cardRef = useRef<HTMLElement | null>(null);
  const title = item.title || item.code || "未命名应用";
  const appURL = item.filePath || `/agent/${encodeURIComponent(item.code)}/`;
  const detailURL = item.id || item.publicId ? `/deploy/${encodeURIComponent(item.id || item.publicId || "")}` : appURL;

  useEffect(() => {
    if (!cardRef.current || !previewRef.current || item.accessProtected) return;
    const iframe = previewRef.current;
    const observer = new IntersectionObserver((entries) => {
      if (entries.some((entry) => entry.isIntersecting) && !iframe.src) {
        iframe.src = `${appURL}?preview=1`;
        observer.disconnect();
      }
    }, { rootMargin: "260px" });
    observer.observe(cardRef.current);
    return () => observer.disconnect();
  }, [appURL, item.accessProtected]);

  async function like() {
    const data = await api<{ likeCount?: number }>(`/api/deploys/${encodeURIComponent(item.code)}/like`, { method: "POST" });
    item.likeCount = data.likeCount ?? (item.likeCount || 0) + 1;
    onChanged();
  }

  async function deleteSite() {
    if (!window.confirm(`确认删除 ${item.code} 吗？`)) return;
    await api(`/api/admin/sites/${encodeURIComponent(item.code)}`, { method: "DELETE" });
    onChanged();
  }

  async function password() {
    const next = item.accessProtected
      ? window.prompt("留空并确认即可清除访问密码。")
      : window.prompt("设置访问密码，至少 4 位。");
    if (next == null) return;
    await api(`/api/deploys/${encodeURIComponent(item.code)}/access`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ password: next })
    });
    onChanged();
  }

  return (
    <article className="app-card" ref={cardRef}>
      <div className="preview-pane">
        {item.accessProtected ? (
          <div className="locked-preview"><Lock size={20} /><strong>需要访问密码</strong><span>加密站点不在首页加载预览。</span></div>
        ) : (
          <iframe ref={previewRef} title={title} loading="lazy" scrolling="no" sandbox="allow-scripts allow-forms allow-popups allow-downloads" />
        )}
        <a className="open-corner" href={appURL} target="_blank" rel="noreferrer"><ExternalLink size={15} />打开</a>
      </div>
      <div className="card-body">
        <div className="card-title-row">
          <div className="title-wrap">
            <h3 title={title}>{title}</h3>
            <code>{item.code}</code>
          </div>
          <div className="badge-row">
            {item.isPinned && <span className="badge amber">置顶</span>}
            <span className={`badge ${item.status === "inactive" ? "rose" : "green"}`}>{item.accessProtected ? "加密" : item.status === "inactive" ? "已下架" : "运行中"}</span>
          </div>
        </div>
        <p>{item.description || "暂无描述"}</p>
        <div className="meta-grid">
          <span>修改 {formatDate(item.updatedAt)}</span>
          <span>{Number(item.viewCount || 0)} 访问</span>
          <span>{Number(item.likeCount || 0)} 赞</span>
          <span>{Number(item.versionCount || 0)} 版本</span>
        </div>
        <div className="card-actions">
          <a className="button compact" href={detailURL}>详情</a>
          <button className="button compact" type="button" onClick={like}><Heart size={15} />点赞</button>
          <a className="button compact" href={`/api/deploy/content?code=${encodeURIComponent(item.code)}&download=1`}><Download size={15} />下载</a>
          {item.owned && (
            <>
              <button className="icon-button" type="button" title={item.accessProtected ? "修改/清除访问密码" : "设置访问密码"} onClick={password}><Lock size={16} /></button>
              <button className="icon-button danger" type="button" title="删除站点" onClick={deleteSite}><Trash2 size={16} /></button>
            </>
          )}
        </div>
      </div>
    </article>
  );
}

function DeployPage({ config }: { config: RuntimeConfig | null }) {
  const [mode, setMode] = useState<"single" | "multi">("single");
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [filename, setFilename] = useState("index.html");
  const [content, setContent] = useState("");
  const [customCode, setCustomCode] = useState("");
  const [enableCustom, setEnableCustom] = useState(false);
  const [createVersion, setCreateVersion] = useState(false);
  const [visibility, setVisibility] = useState("public");
  const [accessPassword, setAccessPassword] = useState("");
  const [files, setFiles] = useState<EditableDeployFile[]>([{ path: "index.html", content: "", isText: true, size: 0 }]);
  const [result, setResult] = useState<DeployResponse | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const ready = useMemo(() => {
    const hasContent = mode === "single"
      ? content.trim()
      : files.some((f) => f.path.trim() && (f.content.trim() || f.contentBase64));
    const hasEntry = mode === "single" || files.some((f) => /\.html?$/i.test(f.path));
    const targetCodeReady = createVersion ? customCode.trim() : (!enableCustom || customCode.trim());
    return description.trim() && hasContent && hasEntry && Boolean(targetCodeReady);
  }, [content, createVersion, customCode, description, enableCustom, files, mode]);

  const totalSize = mode === "single"
    ? fileTextSize(content)
    : files.reduce((sum, f) => sum + (f.size ?? fileTextSize(f.content)), 0);

  async function readUploaded(fileList: FileList | null) {
    if (!fileList?.length) return;
    if (mode === "single") {
      const file = fileList[0];
      setFilename(file.name || "index.html");
      setContent(await file.text());
      return;
    }
    const loaded = await Promise.all(Array.from(fileList).map(async (file) => ({
      path: (file as File & { webkitRelativePath?: string }).webkitRelativePath || file.name,
      content: isTextUpload(file.name) ? await file.text() : "",
      contentBase64: isTextUpload(file.name) ? undefined : arrayBufferToBase64(await file.arrayBuffer()),
      isText: isTextUpload(file.name),
      size: file.size
    })));
    setFiles(loaded);
  }

  function buildPayload() {
    const targetCode = customCode.trim();
    const payload: Record<string, unknown> = {
      title: title.trim() || undefined,
      description: description.trim(),
      enableCustomCode: createVersion ? true : enableCustom,
      customCode: createVersion || enableCustom ? targetCode : undefined,
      createVersion,
      visibility: createVersion ? undefined : visibility,
      accessPassword: !createVersion && accessPassword.trim() ? accessPassword.trim() : undefined
    };
    if (mode === "single") {
      payload.filename = filename.trim() || "index.html";
      payload.content = content;
    } else {
      const valid = files.filter((f) => f.path.trim());
      const index = valid.find((f) => f.path.toLowerCase().endsWith("index.html"));
      const firstHTML = valid.find((f) => /\.html?$/i.test(f.path));
      payload.filename = (index || firstHTML)?.path || (filename.trim() && /\.html?$/i.test(filename.trim()) ? filename.trim() : "index.html");
      payload.files = valid.map<DeployFilePayload>((f) => f.contentBase64
        ? { path: f.path.trim(), contentBase64: f.contentBase64 }
        : { path: f.path.trim(), content: f.content });
    }
    return payload;
  }

  async function submit() {
    if (!ready || busy) return;
    setBusy(true);
    setError("");
    try {
      const data = await api<DeployResponse>("/api/deploy", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(buildPayload())
      });
      setResult(data);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="tool-layout">
      <div className="tool-main">
        <div className="sub-hero">
          <div className="eyebrow"><Code2 size={16} />手动部署</div>
          <h1>上传 HTML 项目，生成稳定访问地址</h1>
          <p>支持单 HTML、多文件目录、公开/未公开、访问密码和追加版本。登录用户发布会归属到账号；未登录发布会记录为匿名 Agent。</p>
        </div>
        <div className="preview-box">
          {mode === "single" && content ? (
            <iframe title="实时预览" srcDoc={content} sandbox="allow-scripts allow-forms allow-popups allow-downloads" />
          ) : (
            <div>
              <Upload size={34} />
              <strong>实时预览</strong>
              <span>单 HTML 模式会在这里预览；多文件站点请部署后查看。</span>
            </div>
          )}
        </div>
      </div>
      <aside className="tool-panel">
        <div className="segmented">
          <button className={mode === "single" ? "active" : ""} onClick={() => setMode("single")} type="button">单 HTML</button>
          <button className={mode === "multi" ? "active" : ""} onClick={() => setMode("multi")} type="button">多文件</button>
        </div>
        <label className="field">
          <span>标题</span>
          <input value={title} onChange={(event) => setTitle(event.target.value)} placeholder="有意义的中文名字" />
        </label>
        <label className="field">
          <span>一句话描述 *</span>
          <input value={description} onChange={(event) => setDescription(event.target.value)} maxLength={240} placeholder="这个应用是做什么的" />
        </label>
        <div className="field-grid">
          <label className="field">
            <span>公开方式</span>
            <select value={visibility} disabled={createVersion} onChange={(event) => setVisibility(event.target.value)}>
              <option value="public">公开进商城</option>
              <option value="unlisted">不公开，仅链接访问</option>
            </select>
          </label>
          <label className="field">
            <span>访问密码</span>
            <input value={accessPassword} disabled={createVersion} onChange={(event) => setAccessPassword(event.target.value)} type="password" placeholder="可选" />
          </label>
        </div>
        <label className="check-line">
          <input
            type="checkbox"
            checked={enableCustom || createVersion}
            disabled={createVersion}
            onChange={(event) => setEnableCustom(event.target.checked)}
          />
          自定义 code
        </label>
        {(enableCustom || createVersion) && (
          <input
            className="standalone-input mono"
            value={customCode}
            onChange={(event) => setCustomCode(event.target.value)}
            placeholder={createVersion ? "输入要更新的已有 code，例如 my-landing" : "my-landing"}
          />
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
          更新现有发布，追加为新版本
        </label>
        {createVersion && (
          <div className="hint-box">
            更新必须填写已有 <code>code</code>。它不会创建新链接，只会给这个站点追加一个新版本；公开方式和访问密码沿用原站点设置。
          </div>
        )}

        {mode === "single" ? (
          <>
            <label className="field">
              <span>入口文件名</span>
              <input value={filename} onChange={(event) => setFilename(event.target.value)} placeholder="index.html" />
            </label>
            <label className="upload-line">
              <input type="file" accept=".html,.htm" onChange={(event) => void readUploaded(event.target.files)} />
              <Upload size={18} />上传 HTML
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
                    <div className="binary-file-note">二进制资源 · {formatSize(file.size)} · 将随整站一起上传</div>
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
  const rows = [
    ["code", result.code],
    ["访问地址", result.url],
    ["应用本体", result.detailUrl || result.url],
    ["版本预览", result.versionUrl || ""],
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
        <a className="button compact primary" href={result.url} target="_blank" rel="noreferrer">打开</a>
        {result.id && <a className="button compact" href={`/deploy/${encodeURIComponent(result.id)}`}>详情</a>}
      </div>
    </div>
  );
}

function AgentsPage({ config }: { config: RuntimeConfig | null }) {
  const baseURL = (config?.publicBaseURL || location.origin).replace(/\/+$/, "");
  const [tab, setTab] = useState<AgentDocTab>("skill");

  return (
    <section className="content-page">
      <div className="sub-hero">
        <div className="eyebrow"><Bot size={16} />Skill & MCP</div>
        <h1>让通用 Agent 接入 PagePilot</h1>
        <p>Skill 适合直接下载给 Agent 使用；MCP 适合支持 stdio JSON-RPC 的客户端。两者都使用同一套 Token、匿名会话和屏幕接口。</p>
        <div className="hero-actions">
          <a className="button primary" href="/skill/hostctl-deploy.zip"><Download size={18} />下载 Skill</a>
          <a className="button" href="/openapi.json"><Workflow size={18} />OpenAPI</a>
        </div>
      </div>
      <div className="doc-tabs" role="tablist" aria-label="Skill 与 MCP 文档">
        <button className={tab === "skill" ? "active" : ""} type="button" role="tab" aria-selected={tab === "skill"} onClick={() => setTab("skill")}>
          <Download size={16} />Skill
        </button>
        <button className={tab === "mcp" ? "active" : ""} type="button" role="tab" aria-selected={tab === "mcp"} onClick={() => setTab("mcp")}>
          <Workflow size={16} />MCP
        </button>
      </div>
      {tab === "skill" ? <SkillGuide config={config} baseURL={baseURL} /> : <MCPGuide config={config} baseURL={baseURL} />}
      <div className="info-grid">
        <FeatureTile icon={<KeyRound />} title="Token 分两种" desc="默认永久，也可以按 expiresAt/ttlSeconds 创建临时 Token。" />
        <FeatureTile icon={<ShieldCheck />} title="匿名身份按 session" desc="网页匿名用浏览器 cookie；Agent 匿名用本地 session 文件和请求头。" />
        <FeatureTile icon={<Monitor />} title="投屏仅注册用户" desc="屏幕绑定、投放、截图和远程指令必须使用注册用户 Token。" />
      </div>
    </section>
  );
}

function SkillGuide({ config, baseURL }: { config: RuntimeConfig | null; baseURL: string }) {
  const server = config?.publicBaseURL || "https://pagepilot.example.com";

  return (
    <>
      <SkillInstallCard baseURL={baseURL} />
      <div className="doc-grid">
        <DocBlock
          title="新建发布"
          lines={[
            `python skill/hostctl-deploy/scripts/hostctl_deploy.py --server ${server} deploy ./dist --title "中文项目名" --description "一句话说明" --visibility public`,
            "发布前请确认 title 是有意义的中文名字，不要使用 index.html、dist、未命名项目这类文件名。",
            "未公开发布使用 --visibility unlisted；访问密码使用 --access-password \"1234\"。"
          ]}
        />
        <DocBlock
          title="更新已有发布"
          lines={[
            "先从返回链接 /agent/{code}/、应用详情或后台站点列表确认 code。",
            `python skill/hostctl-deploy/scripts/hostctl_deploy.py --server ${server} deploy ./dist --code my-landing --update --title "中文项目名" --description "更新说明"`,
            "--update 只追加版本，不改变公开方式和访问密码；必须属于当前 Token 或当前匿名 session。"
          ]}
        />
        <DocBlock
          title="Token 与匿名绑定"
          lines={[
            "python skill/hostctl-deploy/scripts/hostctl_deploy.py token create --label agent --ttl-seconds 2592000",
            "不传 ttl/expires-at 时为永久 Token；传 ttl/expires-at 时为临时 Token。",
            "python skill/hostctl-deploy/scripts/hostctl_deploy.py claim-session --token YOUR_TOKEN",
            "claim-session 会把当前匿名 session 发布过的站点归属到这个用户。"
          ]}
        />
        <DocBlock
          title="屏幕投放"
          lines={[
            "python skill/hostctl-deploy/scripts/hostctl_deploy.py screens list --token YOUR_TOKEN",
            "python skill/hostctl-deploy/scripts/hostctl_deploy.py screens publish SCREEN_ID --code my-landing --token YOUR_TOKEN",
            "python skill/hostctl-deploy/scripts/hostctl_deploy.py screens command SCREEN_ID refresh --token YOUR_TOKEN",
            "截图、刷新、休眠、唤醒、软关机都需要注册用户 Token。"
          ]}
        />
      </div>
    </>
  );
}

function MCPGuide({ config, baseURL }: { config: RuntimeConfig | null; baseURL: string }) {
  const server = config?.publicBaseURL || baseURL || "https://pagepilot.example.com";

  return (
    <>
      <div className="mcp-card">
        <div>
          <div className="mini-label">PAGEPILOT MCP</div>
          <h2>面向支持 MCP 的 Agent 客户端</h2>
          <p>MCP 走 stdio JSON-RPC，适合把发布、版本、Token、屏幕投放等动作注册为工具。匿名发布可以使用 session；屏幕能力必须使用注册用户 Token。</p>
        </div>
        <a className="button" href="/openapi.json"><Workflow size={18} />查看 OpenAPI</a>
      </div>
      <div className="doc-grid">
        <DocBlock
          title="启动方式"
          lines={[
            `hostctl-mcp --server ${server} --token YOUR_TOKEN`,
            "私有服务器请把 --server 改为你的 PagePilot 地址。",
            "没有 Token 时只适合匿名发布；涉及屏幕、后台和用户资源时必须提供 Token。"
          ]}
        />
        <DocBlock
          title="发布工具"
          lines={[
            "deploy_site: 发布单 HTML 或多文件静态站点。",
            "list_sites: 查询当前用户或匿名 session 可管理的站点。",
            "set_access_password: 设置或清除访问密码。",
            "claim_anonymous_session: 把匿名 session 站点绑定到用户。"
          ]}
        />
        <DocBlock
          title="屏幕工具"
          lines={[
            "list_screens: 查询当前用户绑定的屏幕。",
            "publish_screen: 选择自己的站点或商城站点投放到屏幕。",
            "request_screen_screenshot: 后台指令触发一次截图。",
            "send_screen_command: refresh、sleep、wake、shutdown。"
          ]}
        />
        <DocBlock
          title="匿名身份"
          lines={[
            "Agent 匿名身份由本地 sessionId 决定，请持久化并发送 X-Hostctl-Session。",
            "X-Hostctl-Agent-Id / X-Hostctl-Agent-Label 只是设备标记，用于后台展示和排查。",
            "IP、User-Agent 只做辅助信息，不作为所有权依据。"
          ]}
        />
      </div>
    </>
  );
}

function SkillInstallCard({ baseURL }: { baseURL: string }) {
  const skillURL = `${baseURL}/skill/hostctl-deploy.zip`;
  const doctorCommand = `hostctl_deploy.py --server ${baseURL} doctor`;
  const agentPrompt = [
    "PAGEPILOT SKILL",
    `请从 ${skillURL} 下载并安装 hostctl-deploy Skill。`,
    `安装后使用 \`${doctorCommand}\` 检查连接，然后用它把网页发布到 PagePilot。`
  ].join("\n");

  return (
    <section className="skill-package-card">
      <div className="skill-package-head">
        <div>
          <div className="mini-label">hostctl-deploy.zip</div>
          <h2>实时打包</h2>
          <p>复制给 Agent 的安装说明会跟随当前服务器地址自动生成。</p>
        </div>
        <a className="button primary" href="/skill/hostctl-deploy.zip">
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
          <strong>复制给 AGENT</strong>
          <button type="button" className="button compact primary" onClick={() => navigator.clipboard.writeText(agentPrompt)}>
            <Copy size={14} />复制给 AGENT
          </button>
        </div>
        <pre>{agentPrompt}</pre>
      </div>
      <div className="package-url">
        <code>{doctorCommand}</code>
        <button type="button" className="button compact" onClick={() => navigator.clipboard.writeText(doctorCommand)}>
          <Copy size={14} />复制指令
        </button>
      </div>
    </section>
  );
}

function ScreensPage({ config }: { config: RuntimeConfig | null }) {
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
        <div className="eyebrow"><Monitor size={16} />Screen 投放</div>
        <h1>把 HTML 应用发布到广告屏</h1>
        <p>Android 屏幕 APP 通过一次配对绑定到用户，之后拉取播放清单。后台和 Skill/MCP 可投放应用、刷新 WebView、请求截图、休眠/唤醒和软关机。</p>
        <div className="hero-actions">
          <a className="button primary" href="/admin"><Monitor size={18} />管理屏幕</a>
          <a className="button" href="/agents/"><Bot size={18} />用 Agent 投屏</a>
        </div>
      </div>
      <div className="info-grid">
        <FeatureTile icon={<ShieldCheck />} title="安全绑定" desc="设备只保存 Device Token，不持有用户 Token。" />
        <FeatureTile icon={<ExternalLink />} title="投放 URL" desc="投放的是 PagePilot 应用 URL 和 manifest，不是裸 HTML 字符串。" />
        <FeatureTile icon={<Monitor />} title="横竖屏信息" desc="设备心跳会上报分辨率、方向、运行时和 WebView 信息。" />
      </div>
      <div className="panel-table">
        <div className="section-head">
          <div>
            <h2>我的屏幕</h2>
            <p>{error ? "登录注册用户后可查看自己的屏幕。" : `服务器：${config?.publicBaseURL || location.origin}`}</p>
          </div>
        </div>
        {error ? <div className="empty-wide">{error}</div> : (
          <div className="screen-list">
            {screens.map((screen) => (
              <div className="screen-row" key={screen.id}>
                <strong>{screen.name || screen.id}</strong>
                <span>{screen.status || "未知"} · {screen.currentSiteCode || "空闲"}</span>
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
    ["POST", "/api/deploy", "发布单 HTML 或多文件静态站点"],
    ["GET", "/api/deploys", "应用商城列表，支持点赞排序和管理员置顶"],
    ["POST", "/api/session", "创建/读取匿名部署会话"],
    ["POST", "/api/session/claim", "把匿名发布认领到当前用户"],
    ["POST", "/api/deploys/{code}/access", "匿名访客输入访问密码，获得 5 分钟访问票据"],
    ["GET", "/api/screens", "列出当前用户绑定屏幕"],
    ["POST", "/api/screens/{screenId}/publish", "投放应用到屏幕"],
    ["POST", "/api/screens/{screenId}/screenshot", "下发一次截图指令"],
    ["POST", "/api/screens/{screenId}/command", "刷新、休眠、唤醒、软关机"]
  ];
  return (
    <section className="content-page">
      <div className="sub-hero">
        <div className="eyebrow"><Code2 size={16} />API 文档</div>
        <h1>稳定 API 契约</h1>
        <p>React 前端、Skill、MCP 和屏幕 APP 都使用同一套 API。完整机器可读契约请查看 OpenAPI。</p>
        <div className="hero-actions">
          <a className="button primary" href="/openapi.json">查看 openapi.json</a>
          <a className="button" href="/agents/">Skill & MCP</a>
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
      <span>PagePilot · HTML 应用商城 · Skill & MCP · Screen 投放</span>
      <a href="/admin">后台</a>
    </footer>
  );
}

declare module "react" {
  interface InputHTMLAttributes<T> {
    webkitdirectory?: string;
  }
}
