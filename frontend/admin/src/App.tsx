import {
  Bot,
  ClipboardList,
  Code2,
  Copy,
  Download,
  Eye,
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
  | "screens"
  | "tokens"
  | "users"
  | "anonymous"
  | "config"
  | "skill";

type DeployMode = "single" | "multi";
type SkillTab = "skill" | "mcp";
type ScreenPickTab = "mine" | "market";

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
  publicBaseURL?: string;
  configuredPublicBaseURL?: string;
  publicURLMode?: "configured" | "request_host";
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
  limits?: {
    maxSingleFileBytes?: number;
    maxSiteTotalBytes?: number;
    maxFilesPerSite?: number;
  };
}

interface SiteItem {
  code: string;
  publicId?: string;
  ownerTokenId?: string;
  ownerUsername?: string;
  currentVersion?: number;
  versionCount?: number;
  totalSize?: number;
  viewCount?: number;
  likeCount?: number;
  status?: string;
  visibility?: string;
  accessProtected?: boolean;
  isPinned?: boolean;
  createdAt?: string;
  lastVersionAt?: string;
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
}

class APIError extends Error {
  status?: number;
  body?: unknown;
}

const navItems: Array<{ tab: Tab; label: string; icon: React.ReactNode; adminOnly?: boolean }> = [
  { tab: "overview", label: "总览", icon: <LayoutDashboard size={18} /> },
  { tab: "deploy", label: "发布应用", icon: <Upload size={18} /> },
  { tab: "sites", label: "站点管理", icon: <ClipboardList size={18} /> },
  { tab: "screens", label: "硬件屏幕", icon: <Monitor size={18} /> },
  { tab: "tokens", label: "Skill & MCP Token", icon: <KeyRound size={18} /> },
  { tab: "account", label: "账号设置", icon: <ShieldCheck size={18} /> },
  { tab: "users", label: "用户管理", icon: <Users size={18} />, adminOnly: true },
  { tab: "anonymous", label: "匿名 Agent", icon: <Bot size={18} />, adminOnly: true },
  { tab: "config", label: "运行设置", icon: <Settings size={18} />, adminOnly: true },
  { tab: "skill", label: "Skill & MCP", icon: <Workflow size={18} />, adminOnly: true }
];

function authHeaders(headers: Record<string, string> = {}) {
  const token = localStorage.getItem("hostctl-admin-token") || localStorage.getItem("hostctl-token") || "";
  return token && !headers.Authorization ? { ...headers, Authorization: `Bearer ${token}` } : headers;
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
    const err = new APIError(body?.detail || body?.errorCode || `HTTP ${res.status}`);
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

function formatDate(value?: string) {
  if (!value) return "-";
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? value : d.toLocaleString("zh-CN", { hour12: false });
}

function textSize(text: string) {
  return new Blob([text]).size;
}

function isTextFile(name: string) {
  return /\.(html?|css|js|mjs|json|txt|md|svg|xml|csv|webmanifest|map)$/i.test(name);
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

function siteURL(code: string) {
  return `/agent/${encodeURIComponent(code)}/`;
}

export default function App() {
  const [authChecking, setAuthChecking] = useState(true);
  const [session, setSession] = useState<SessionInfo | null>(null);
  const [config, setConfig] = useState<RuntimeConfig | null>(null);
  const [activeTab, setActiveTab] = useState<Tab>("overview");
  const [toast, setToast] = useState("");
  const [error, setError] = useState("");

  const isAdmin = !!session?.isAdmin;
  const visibleNav = navItems.filter((item) => !item.adminOnly || isAdmin);

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
              onClick={() => setActiveTab(item.tab)}
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

        {error && <div className="alert error">{error}</div>}
        {activeTab === "overview" && <Overview config={config} session={session} setError={setError} setTab={setActiveTab} />}
        {activeTab === "account" && <AccountPanel session={session} showToast={showToast} />}
        {activeTab === "deploy" && <DeployPanel config={config} showToast={showToast} setError={setError} />}
        {activeTab === "sites" && <SitesPanel isAdmin={isAdmin} showToast={showToast} setError={setError} />}
        {activeTab === "screens" && <ScreensPanel isAdmin={isAdmin} showToast={showToast} setError={setError} />}
        {activeTab === "tokens" && <TokensPanel isAdmin={isAdmin} showToast={showToast} setError={setError} />}
        {activeTab === "users" && isAdmin && <UsersPanel showToast={showToast} setError={setError} />}
        {activeTab === "anonymous" && isAdmin && <AnonymousPanel setError={setError} />}
        {activeTab === "config" && isAdmin && <ConfigPanel config={config} onConfig={setConfig} showToast={showToast} setError={setError} />}
        {activeTab === "skill" && isAdmin && <SkillMCPPanel config={config} showToast={showToast} setError={setError} />}
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
    sites: isAdmin ? "站点管理" : "我的站点",
    screens: "硬件屏幕",
    tokens: "Skill & MCP Token",
    users: "用户管理",
    anonymous: "匿名 Agent",
    config: "运行设置",
    skill: "Skill & MCP"
  };
  return titles[tab];
}

function tabSubtitle(tab: Tab, isAdmin: boolean) {
  const subtitles: Record<Tab, string> = {
    overview: isAdmin ? "查看全站发布、用户、Agent 和屏幕状态。" : "查看你的发布、Token 和屏幕操作入口。",
    account: "修改密码，确认当前登录身份。",
    deploy: "上传单 HTML 或多文件静态站点，新建发布或追加版本。",
    sites: isAdmin ? "管理全站应用、归属、加密、下架、置顶和版本。" : "管理你发布的应用、访问密码和版本。",
    screens: "绑定屏幕、选择站点投放、刷新 WebView 和下发截图指令。",
    tokens: "创建永久或临时 Token，供 Skill/MCP/Agent 调用。",
    users: "创建账号、调整额度、停用或删除用户。",
    anonymous: "查看未登录发布产生的网页匿名和 Agent 匿名 session。",
    config: "调整公网 URL、泛域名访问、上传额度、CORS 和匿名额度。",
    skill: "维护 Skill 下载包，并查看 MCP 接入说明。"
  };
  return subtitles[tab];
}

function LoginScreen({
  config,
  needsSetup,
  onAuthed,
  onConfig
}: {
  config: RuntimeConfig | null;
  needsSetup: boolean;
  onAuthed: (session: SessionInfo) => void;
  onConfig: (cfg: RuntimeConfig) => void;
}) {
  const [mode, setMode] = useState<"password" | "token">("password");
  const [authMode, setAuthMode] = useState<"login" | "register">("login");
  const [captcha, setCaptcha] = useState<Captcha>({});
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [captchaAnswer, setCaptchaAnswer] = useState("");
  const [token, setToken] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const loadCaptcha = useCallback(async () => {
    const data = await api<Captcha>("/api/auth/captcha");
    setCaptcha(data);
    setCaptchaAnswer("");
  }, []);

  useEffect(() => {
    void loadCaptcha();
    api<RuntimeConfig>("/api/config").then(onConfig).catch(() => undefined);
  }, [loadCaptcha, onConfig]);

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    setError("");
    setBusy(true);
    try {
      if (mode === "token") {
        if (!token.trim()) throw new Error("请输入 Token");
        localStorage.setItem("hostctl-admin-token", token.trim());
        const session = await api<SessionInfo>("/api/admin/session");
        onAuthed(session);
        return;
      }
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
        setError("管理员已创建，请登录。");
        return;
      }
      if (authMode === "register") {
        await api("/api/auth/register", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ username: username.trim(), password, captchaId: captcha.id, captcha: captchaAnswer.trim() })
        });
        setAuthMode("login");
        setPassword("");
        await loadCaptcha();
        setError("注册成功，请登录。");
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
      setError(err instanceof Error ? err.message : String(err));
      if (mode === "password") void loadCaptcha();
    } finally {
      setBusy(false);
    }
  }

  return (
    <main className="login-page">
      <section className="login-card">
        <div className="brand login-brand"><Logo /><span>PagePilot</span></div>
        <h1>{mode === "token" ? "Token 登录" : needsSetup ? "创建管理员" : authMode === "register" ? "注册账号" : "登录后台"}</h1>
        <p>{needsSetup ? "当前还没有管理员账号，请先创建第一个管理员。" : "登录后进入管理工作台。普通用户可以管理自己的发布、Token 和屏幕；管理员可以管理全站。"}</p>
        <div className="segmented">
          <button className={mode === "password" ? "active" : ""} type="button" onClick={() => setMode("password")}>账号密码</button>
          <button className={mode === "token" ? "active" : ""} type="button" onClick={() => setMode("token")}>Token</button>
        </div>
        <form onSubmit={submit}>
          {mode === "token" ? (
            <label className="field">
              <span>Token</span>
              <input value={token} onChange={(event) => setToken(event.target.value)} autoComplete="off" placeholder="hostctl_..." />
            </label>
          ) : (
            <>
              <label className="field">
                <span>用户名</span>
                <input value={username} onChange={(event) => setUsername(event.target.value)} autoComplete="username" />
              </label>
              <label className="field">
                <span>密码</span>
                <input value={password} onChange={(event) => setPassword(event.target.value)} type="password" autoComplete="current-password" />
              </label>
              <label className="field">
                <span>{captcha.prompt || "验证码"}</span>
                <div className="inline-input">
                  <input value={captchaAnswer} onChange={(event) => setCaptchaAnswer(event.target.value)} />
                  <button className="button" type="button" onClick={() => void loadCaptcha()}>换一个</button>
                </div>
              </label>
            </>
          )}
          {error && <div className={error.includes("成功") ? "alert success" : "alert error"}>{error}</div>}
          <button className="button primary full" type="submit" disabled={busy}>{busy ? "处理中..." : mode === "token" ? "Token 登录" : needsSetup ? "创建管理员" : authMode === "register" ? "注册账号" : "登录"}</button>
        </form>
        {mode === "password" && !needsSetup && (
          <button className="button ghost full" type="button" onClick={() => setAuthMode(authMode === "register" ? "login" : "register")}>
            {authMode === "register" ? "已有账号，去登录" : "注册普通账号"}
          </button>
        )}
        <div className="hint-line">当前服务：{config?.publicBaseURL || location.origin}</div>
      </section>
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

function DeployPanel({ config, showToast, setError }: { config: RuntimeConfig | null; showToast: (msg: string) => void; setError: (msg: string) => void }) {
  const [mode, setMode] = useState<DeployMode>("single");
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [code, setCode] = useState("");
  const [append, setAppend] = useState(false);
  const [visibility, setVisibility] = useState("public");
  const [password, setPassword] = useState("");
  const [content, setContent] = useState("");
  const [entry, setEntry] = useState("index.html");
  const [files, setFiles] = useState<DeployFile[]>([]);
  const [result, setResult] = useState<any>(null);
  const [busy, setBusy] = useState(false);
  const fileInput = useRef<HTMLInputElement | null>(null);
  const dirInput = useRef<HTMLInputElement | null>(null);

  const totalSize = mode === "single" ? textSize(content) : files.reduce((sum, file) => sum + file.size, 0);

  async function readFiles(list: FileList | null) {
    if (!list?.length) return;
    if (mode === "single") {
      const file = list[0];
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
    const main = valid.find((file) => file.path.toLowerCase().endsWith("index.html")) || valid.find((file) => /\.html?$/i.test(file.path));
    const body: any = {
      title: title.trim() || undefined,
      description: description.trim(),
      enableCustomCode: append || !!code.trim(),
      customCode: append || code.trim() ? code.trim() : undefined,
      createVersion: append,
      visibility: append ? undefined : visibility,
      accessPassword: !append && password.trim() ? password.trim() : undefined
    };
    if (mode === "single") {
      body.filename = entry.trim() || "index.html";
      body.content = content;
    } else {
      body.filename = main?.path || (entry.trim() && /\.html?$/i.test(entry.trim()) ? entry.trim() : "index.html");
      body.files = valid.map((file) => file.contentBase64
        ? { path: file.path.trim(), contentBase64: file.contentBase64 }
        : { path: file.path.trim(), content: file.content });
    }
    return body;
  }

  async function submit() {
    setError("");
    if (!description.trim()) return setError("请填写一句话描述");
    if (append && !code.trim()) return setError("更新现有发布必须填写已有 code");
    if (mode === "single" && !content.trim()) return setError("请粘贴或上传 HTML 内容");
    if (mode === "multi" && !files.length) return setError("请上传多文件项目");
    if (mode === "multi" && !files.some((file) => /\.html?$/i.test(file.path))) return setError("多文件项目需要至少包含一个 HTML 入口文件");
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
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="two-col deploy-layout">
      <div className="panel">
        <div className="panel-head"><div><h2>{append ? "更新现有发布" : "发布新应用"}</h2><p>支持单 HTML、多文件目录、公开/未公开、访问密码和追加版本。</p></div></div>
        <div className="segmented">
          <button className={mode === "single" ? "active" : ""} type="button" onClick={() => setMode("single")}>单 HTML</button>
          <button className={mode === "multi" ? "active" : ""} type="button" onClick={() => setMode("multi")}>多文件项目</button>
        </div>
        <label className="field"><span>标题</span><input value={title} onChange={(event) => setTitle(event.target.value)} placeholder="有意义的中文名字" /></label>
        <label className="field"><span>一句话描述 *</span><input value={description} onChange={(event) => setDescription(event.target.value)} maxLength={240} placeholder="这个应用是做什么的" /></label>
        <label className="field"><span>自定义 / 更新 code</span><input className="mono" value={code} onChange={(event) => setCode(event.target.value)} placeholder={append ? "填写已有 code，例如 my-landing" : "可选，例如 my-landing"} /></label>
        <label className="check-line"><input checked={append} type="checkbox" onChange={(event) => setAppend(event.target.checked)} />更新现有发布，追加为新版本</label>
        {append && <div className="hint-box">更新必须填写已有 <code>code</code>。它不会创建新链接，只给原站点追加版本；公开方式和访问密码沿用原设置。</div>}
        <div className="form-grid">
          <label className="field"><span>可见性</span><select value={visibility} disabled={append} onChange={(event) => setVisibility(event.target.value)}><option value="public">公开进应用商城</option><option value="unlisted">未公开，仅链接访问</option></select></label>
          <label className="field"><span>访问密码</span><input type="password" value={password} disabled={append} onChange={(event) => setPassword(event.target.value)} placeholder="可选，至少 4 位" /></label>
        </div>

        {mode === "single" ? (
          <>
            <label className="field"><span>入口文件名</span><input value={entry} onChange={(event) => setEntry(event.target.value)} placeholder="index.html" /></label>
            <label className="upload-zone">
              <input type="file" accept=".html,.htm" onChange={(event) => void readFiles(event.target.files)} />
              <FileUp size={18} />上传 HTML
            </label>
            <textarea className="code-input" value={content} onChange={(event) => setContent(event.target.value)} placeholder="<!doctype html>..." />
          </>
        ) : (
          <>
            <input ref={fileInput} className="hidden" type="file" multiple onChange={(event) => void readFiles(event.target.files)} />
            <input ref={dirInput} className="hidden" type="file" multiple webkitdirectory="" onChange={(event) => void readFiles(event.target.files)} />
            <div className="upload-box" onDragOver={(event) => event.preventDefault()} onDrop={(event) => { event.preventDefault(); void readFiles(event.dataTransfer.files); }}>
              <strong>上传多文件静态站点</strong>
              <span>保留 CSS、JS、图片、字体等相对路径，优先使用 index.html 作为入口。</span>
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
      </div>
      <div className="panel">
        <div className="panel-head"><div><h2>结果与预览</h2><p>部署成功后可复制链接和进入版本管理。</p></div></div>
        {result ? (
          <div className="result-box">
            <InfoRow label="Code" value={result.code} />
            <InfoRow label="访问地址" value={result.url} copy />
            <InfoRow label="版本" value={`v${result.versionNumber || "-"}`} />
            <InfoRow label="大小" value={formatSize(result.size)} />
            <div className="actions">
              <a className="button primary" href={result.url} target="_blank" rel="noreferrer"><Eye size={16} />打开</a>
              <button className="button" type="button" onClick={() => navigator.clipboard.writeText(result.url)}><Copy size={16} />复制</button>
              <a className="button" href={`/deploy/${encodeURIComponent(result.id || result.code)}`} target="_blank" rel="noreferrer">详情</a>
            </div>
            <iframe title="部署预览" src={result.url} sandbox="allow-scripts allow-forms allow-popups allow-downloads" />
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
  const [query, setQuery] = useState("");
  const [status, setStatus] = useState("");
  const [versions, setVersions] = useState<any[] | null>(null);
  const [versionCode, setVersionCode] = useState("");

  const load = useCallback(async () => {
    try {
      const data = await api<{ sites?: SiteItem[] }>("/api/admin/sites");
      setSites(data.sites || []);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }, [setError]);

  useEffect(() => { void load(); }, [load]);

  const filtered = useMemo(() => sites.filter((site) => {
    const hit = !query.trim() || site.code.includes(query.trim()) || (site.ownerUsername || "").includes(query.trim());
    const statusHit = !status || site.status === status;
    return hit && statusHit;
  }), [query, sites, status]);

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
    showToast(visibility === "public" ? "已公开进商城" : "已设为未公开");
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
      <div className="toolbar">
        <label className="search-box"><Search size={16} /><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索 code 或 owner" /></label>
        <select value={status} onChange={(event) => setStatus(event.target.value)}><option value="">全部状态</option><option value="active">运行中</option><option value="inactive">已下架</option></select>
      </div>
      <div className="table-wrap">
        <table>
          <thead><tr><th>Code</th><th>归属</th><th>状态</th><th>版本</th><th>访问/点赞</th><th>大小</th><th>修改</th><th>操作</th></tr></thead>
          <tbody>
            {filtered.map((site) => (
              <tr key={site.code}>
                <td><code>{site.code}</code></td>
                <td>{site.ownerUsername || site.ownerTokenId || "-"}</td>
                <td><div className="badge-stack">{site.isPinned && <span className="badge amber">置顶</span>}{statusBadge(site.status || "active", site.accessProtected)}<span className="badge slate">{site.visibility === "unlisted" ? "未公开" : "公开"}</span></div></td>
                <td>v{site.currentVersion || "-"} · {site.versionCount || 0}</td>
                <td>{site.viewCount || 0} / {site.likeCount || 0}</td>
                <td>{formatSize(site.totalSize)}</td>
                <td>{formatDate(site.lastVersionAt || site.createdAt)}</td>
                <td>
                  <div className="actions tight">
                    <a className="button compact" href={siteURL(site.code)} target="_blank" rel="noreferrer">打开</a>
                    <button className="button compact" type="button" onClick={() => void setPassword(site)}><Lock size={14} />加密</button>
                    <button className="button compact" type="button" onClick={() => void toggleVisibility(site)}>{site.visibility === "unlisted" ? "公开" : "未公开"}</button>
                    <button className="button compact" type="button" onClick={() => void openVersions(site)}>版本</button>
                    {isAdmin && <button className="icon-button" type="button" title="置顶" onClick={() => void togglePin(site)}><Pin size={14} /></button>}
                    <button className="icon-button danger" type="button" title="删除" onClick={() => void deleteSite(site)}><Trash2 size={14} /></button>
                  </div>
                </td>
              </tr>
            ))}
            {!filtered.length && <tr><td colSpan={8}>暂无站点。</td></tr>}
          </tbody>
        </table>
      </div>
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
          <div className="panel-head"><div><h2>投放规则</h2><p>后台不会让用户手写 code 投屏，统一从自己的站点或应用商城选择。</p></div></div>
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
            <button className={pickTab === "market" ? "active" : ""} type="button" onClick={() => setPickTab("market")}>应用商城</button>
          </div>
          <div className="choice-list">
            {(pickTab === "mine" ? sites : market).map((site) => (
              <button className="choice-card" type="button" key={site.code} onClick={() => void publish(pickScreen, site.code)}>
                <strong>{site.code}</strong>
                <span>{"title" in site ? site.title || site.description || "应用商城站点" : `${site.versionCount || 0} 个版本`}</span>
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
    await api("/api/admin/users", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ username, password, deployLimit: limit, isAdmin })
    });
    setUsername("");
    setPassword("");
    setIsAdmin(false);
    showToast("用户已创建");
    await load();
  }

  async function update(user: UserItem) {
    await api(`/api/admin/users/${encodeURIComponent(user.id)}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ username: user.username, deployLimit: user.deployLimit, isAdmin: user.isAdmin, isActive: user.isActive })
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
        <div className="panel-head"><div><h2>添加用户</h2><p>创建普通用户或管理员账号。</p></div></div>
        <label className="field"><span>用户名</span><input value={username} onChange={(event) => setUsername(event.target.value)} /></label>
        <label className="field"><span>初始密码</span><input type="password" value={password} onChange={(event) => setPassword(event.target.value)} placeholder="至少 8 位" /></label>
        <label className="field"><span>部署额度</span><input type="number" value={limit} onChange={(event) => setLimit(Number(event.target.value))} /></label>
        <label className="check-line"><input type="checkbox" checked={isAdmin} onChange={(event) => setIsAdmin(event.target.checked)} />管理员</label>
        <button className="button primary" type="button" onClick={() => void create()}><UserPlus size={16} />创建用户</button>
      </div>
      <div className="panel">
        <div className="panel-head"><div><h2>用户列表</h2><p>调整额度、停用或删除用户。</p></div><button className="button" type="button" onClick={() => void load()}><RefreshCw size={16} />刷新</button></div>
        <div className="table-wrap compact">
          <table>
            <thead><tr><th>用户</th><th>额度</th><th>角色</th><th>状态</th><th>操作</th></tr></thead>
            <tbody>
              {users.map((user, index) => (
                <tr key={user.id}>
                  <td><input value={user.username} onChange={(event) => setUsers((prev) => prev.map((item, i) => i === index ? { ...item, username: event.target.value } : item))} /><br /><small>{user.id}</small></td>
                  <td><input type="number" value={user.deployLimit} onChange={(event) => setUsers((prev) => prev.map((item, i) => i === index ? { ...item, deployLimit: Number(event.target.value) } : item))} /><small>已用 {user.deployCount}</small></td>
                  <td><label className="check-line compact"><input type="checkbox" checked={user.isAdmin} onChange={(event) => setUsers((prev) => prev.map((item, i) => i === index ? { ...item, isAdmin: event.target.checked } : item))} />管理员</label></td>
                  <td><label className="check-line compact"><input type="checkbox" checked={user.isActive} onChange={(event) => setUsers((prev) => prev.map((item, i) => i === index ? { ...item, isActive: event.target.checked } : item))} />启用</label></td>
                  <td><div className="actions tight"><button className="button compact" type="button" onClick={() => void update(user)}>保存</button><button className="button compact danger" type="button" onClick={() => void remove(user)}>删除</button></div></td>
                </tr>
              ))}
              {!users.length && <tr><td colSpan={5}>暂无用户。</td></tr>}
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

function ConfigPanel({ config, onConfig, showToast, setError }: { config: RuntimeConfig | null; onConfig: (cfg: RuntimeConfig) => void; showToast: (msg: string) => void; setError: (msg: string) => void }) {
  const [draft, setDraft] = useState({
    publicBaseURL: "",
    publicURLMode: "configured" as "configured" | "request_host",
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
      publicBaseURL: config.configuredPublicBaseURL || config.publicBaseURL || "",
      publicURLMode: config.publicURLMode || "configured",
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
          publicBaseURL: draft.publicBaseURL,
          publicURLMode: draft.publicURLMode,
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

  const configuredBaseURLPreview = (draft.publicBaseURL || "https://pagepilot.example.com").replace(/\/+$/, "");
  const requestBaseURLPreview = (typeof location !== "undefined" ? location.origin : "https://current.example.com").replace(/\/+$/, "");
  const baseURLPreview = draft.publicURLMode === "request_host" ? requestBaseURLPreview : configuredBaseURLPreview;
  const portText = String(draft.appURLPort || "").trim();
  const portSuffix = portText ? `:${portText}` : "";
  const domainSuffix = String(draft.appDomainSuffix || "apps.example.com").replace(/^\.+/, "");
  const sampleCode = "demo-app";
  const pathURL = `${baseURLPreview}/agent/${sampleCode}/`;
  const domainURL = `${draft.appURLScheme}://${sampleCode}.${domainSuffix}${portSuffix}/`;
  const modeText = draft.appURLMode === "domain" ? "只生成泛域名链接" : draft.appURLMode === "dual" ? "同时保留路径和泛域名链接" : "默认生成 /agent/{code} 路径链接";
  const publicURLModeText = draft.publicURLMode === "request_host" ? "按当前访问域名生成主站链接" : "固定使用 Public Base URL";
  const embedModeText = draft.embedPolicy === "deny" ? "禁止任何网站 iframe 嵌入应用" : draft.embedPolicy === "self" ? "只允许本站嵌入应用" : draft.embedPolicy === "allowlist" ? "本站和白名单来源可嵌入应用" : "允许任意网站 iframe 嵌入应用";

  return (
    <section className="panel config-panel">
      <div className="panel-head">
        <div>
          <h2>运行设置</h2>
          <p>这里控制对外链接、Agent/Skill 下载说明、二维码、投屏链接和上传限额。改完后新生成的链接会按这里生效。</p>
        </div>
      </div>

      <div className="config-layout">
        <div className="config-main">
          <section className="config-section">
            <div className="config-section-head">
              <strong>公网访问地址</strong>
              <span>用户和 Agent 看到的服务器地址</span>
            </div>
            <label className="field rich-field">
              <span>Public Base URL</span>
              <input className="mono" value={draft.publicBaseURL} onChange={(event) => setDraft({ ...draft, publicBaseURL: event.target.value })} placeholder="https://pagepilot.example.com" />
              <em>填浏览器实际访问后台/首页的地址，必须包含协议和端口；不要带路径。会影响首页按钮、二维码、Skill 下载文案。</em>
            </label>
            <label className="field rich-field">
              <span>主站链接来源</span>
              <select value={draft.publicURLMode} onChange={(event) => setDraft({ ...draft, publicURLMode: event.target.value as "configured" | "request_host" })}>
                <option value="configured">固定使用 Public Base URL</option>
                <option value="request_host">按当前访问域名生成</option>
              </select>
              <em>选择“按当前访问域名”后，首页、Skill/MCP、OpenAPI、二维码和 /agent 路径模式会跟随浏览器实际访问域名；泛域名应用访问仍由下方独立配置控制。请确认前面是可信反向代理，并正确传递 Host / X-Forwarded-Host。</em>
            </label>
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
              <code>{publicURLModeText}</code>
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
              <li>首页应用打开链接</li>
              <li>部署成功后的返回 URL</li>
              <li>二维码和屏幕投放地址</li>
              <li>Skill 下载说明和 Agent 文案</li>
            </ul>
          </div>
          <button className="button primary full" type="button" onClick={() => void save()}><Save size={16} />保存运行设置</button>
        </aside>
      </div>
    </section>
  );
}

function SkillMCPPanel({ config, showToast, setError }: { config: RuntimeConfig | null; showToast: (msg: string) => void; setError: (msg: string) => void }) {
  const [tab, setTab] = useState<SkillTab>("skill");
  const [files, setFiles] = useState<Array<{ path: string; label?: string }>>([]);
  const [path, setPath] = useState("SKILL.md");
  const [content, setContent] = useState("");
  const [meta, setMeta] = useState<any>(null);
  const base = (config?.publicBaseURL || location.origin).replace(/\/+$/, "");

  const load = useCallback(async (nextPath = path) => {
    try {
      const data = await api<any>(`/api/admin/skill?path=${encodeURIComponent(nextPath)}`);
      setFiles(data.files || []);
      setPath(data.path || nextPath);
      setContent(data.content || "");
      setMeta(data);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }, [path, setError]);

  useEffect(() => { void load("SKILL.md"); }, []);

  async function save() {
    await api("/api/admin/skill", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ path, content })
    });
    showToast("Skill 已保存");
    await load(path);
  }

  const skillURL = `${base}/skill/hostctl-deploy.zip`;
  const prompt = [
    "PAGEPILOT SKILL",
    `请从 ${skillURL} 下载并安装 hostctl-deploy Skill。`,
    `安装后使用 \`hostctl_deploy.py --server ${base} doctor\` 检查连接，然后用它把网页发布到 PagePilot。`
  ].join("\n");

  return (
    <section className="panel">
      <div className="panel-head">
        <div><h2>Skill & MCP</h2><p>Skill 是可下载的 Agent 能力包；MCP 是 stdio 工具服务，两者共用 Token 与权限模型。</p></div>
        <div className="actions">
          <a className="button" href="/skill/hostctl-deploy.zip" target="_blank" rel="noreferrer"><Download size={16} />下载 Skill</a>
          {tab === "skill" && <button className="button primary" type="button" onClick={() => void save()}><Save size={16} />保存 Skill</button>}
        </div>
      </div>
      <div className="segmented slim">
        <button className={tab === "skill" ? "active" : ""} type="button" onClick={() => setTab("skill")}>Skill</button>
        <button className={tab === "mcp" ? "active" : ""} type="button" onClick={() => setTab("mcp")}>MCP</button>
      </div>
      {tab === "skill" ? (
        <div className="skill-layout">
          <aside>
            <label className="field"><span>文件</span><select value={path} onChange={(event) => void load(event.target.value)}>{files.map((file) => <option value={file.path} key={file.path}>{file.label || file.path}</option>)}</select></label>
            <div className="meta-box"><InfoRow label="路径" value={meta?.path || path} /><InfoRow label="大小" value={formatSize(meta?.size)} /><InfoRow label="下载包" value="实时打包" /></div>
            <div className="copy-panel">
              <strong>复制给 AGENT</strong>
              <pre>{prompt}</pre>
              <button className="button compact" type="button" onClick={() => navigator.clipboard.writeText(prompt)}><Copy size={14} />复制</button>
            </div>
          </aside>
          <textarea className="skill-editor mono" value={content} onChange={(event) => setContent(event.target.value)} spellCheck={false} />
        </div>
      ) : (
        <div className="doc-grid">
          <DocBlock title="启动方式" lines={[`hostctl-mcp --server ${base} --token YOUR_TOKEN`, "私有服务器请替换 --server；屏幕能力必须使用注册用户 Token。"]} />
          <DocBlock title="发布工具" lines={["deploy_site: 新建发布或追加版本，支持多文件和访问密码。", "list_sites: 查询当前用户或匿名 session 的站点。", "set_access_password: 设置或清除访问密码。", "claim_anonymous_session: 将匿名发布归属到注册用户。"]} />
          <DocBlock title="屏幕工具" lines={["list_screens: 查询用户屏幕。", "publish_screen: 从自己的站点或商城选择投放。", "request_screen_screenshot: 下发一次截图指令。", "send_screen_command: refresh、sleep、wake、shutdown。"]} />
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
    <svg className="logo" viewBox="0 0 96 96" aria-hidden="true">
      <defs>
        <linearGradient id="admin-react-logo-bg" x1="6" x2="88" y1="6" y2="90"><stop stopColor="#101348" /><stop offset="0.55" stopColor="#172D67" /><stop offset="1" stopColor="#155E75" /></linearGradient>
        <linearGradient id="admin-react-logo-window" x1="32" x2="66" y1="35" y2="74"><stop stopColor="#FFFFFF" /><stop offset="1" stopColor="#EAF7FF" /></linearGradient>
        <linearGradient id="admin-react-logo-bar" x1="31" x2="67" y1="35" y2="49"><stop stopColor="#60A5FA" /><stop offset="1" stopColor="#8B5CF6" /></linearGradient>
      </defs>
      <rect x="5" y="5" width="86" height="86" rx="26" fill="url(#admin-react-logo-bg)" />
      <path d="M13 67C32 45 60 36 84 29" fill="none" stroke="#22D3EE" strokeLinecap="round" strokeWidth="4" opacity="0.82" />
      <path d="M17 73C35 59 61 49 86 43" fill="none" stroke="#A78BFA" strokeLinecap="round" strokeWidth="3" opacity="0.55" />
      <rect x="31" y="35" width="36" height="39" rx="11" fill="url(#admin-react-logo-window)" />
      <path d="M31 45a10 10 0 0 1 10-10h16a10 10 0 0 1 10 10v5H31z" fill="url(#admin-react-logo-bar)" />
      <circle cx="39" cy="44" r="2.7" fill="#F472B6" /><circle cx="48" cy="44" r="2.7" fill="#FBBF24" /><circle cx="57" cy="44" r="2.7" fill="#86EFAC" />
      <path d="M38 58q3-4 7 0M53 58q3-4 7 0" fill="none" stroke="#111827" strokeLinecap="round" strokeWidth="3" />
      <path d="M45 66q4 4 9 0" fill="#FB7185" stroke="#831843" strokeLinecap="round" strokeWidth="1.6" />
    </svg>
  );
}
