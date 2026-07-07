import { spawn, spawnSync } from "node:child_process";
import { once } from "node:events";
import { createRequire } from "node:module";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import net from "node:net";
import { fileURLToPath } from "node:url";

const rootDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

function loadPlaywright() {
  const require = createRequire(import.meta.url);
  try {
    return require("playwright");
  } catch (error) {
    throw new Error(
      "visual QA 需要 Playwright。请先运行 `npm install --no-save playwright`，" +
        "再执行 `npx playwright install chromium`。原始错误：" + error.message,
    );
  }
}

async function launchChromium(chromium) {
  const attempts = [
    { label: "bundled chromium", options: { headless: true } },
    { label: "system msedge", options: { channel: "msedge", headless: true } },
    { label: "system chrome", options: { channel: "chrome", headless: true } },
  ];
  const errors = [];
  for (const attempt of attempts) {
    try {
      return await chromium.launch(attempt.options);
    } catch (error) {
      errors.push(`${attempt.label}: ${error.message.split("\n")[0]}`);
    }
  }
  throw new Error(`could not launch a browser:\n${errors.join("\n")}`);
}

async function freePort() {
  const server = net.createServer();
  await new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", resolve);
  });
  const address = server.address();
  await new Promise((resolve) => server.close(resolve));
  return address.port;
}

class CookieJar {
  constructor() {
    this.cookies = new Map();
  }

  store(response) {
    const values = typeof response.headers.getSetCookie === "function"
      ? response.headers.getSetCookie()
      : splitSetCookie(response.headers.get("set-cookie"));
    for (const raw of values) {
      const pair = raw.split(";")[0] || "";
      const idx = pair.indexOf("=");
      if (idx <= 0) continue;
      const name = pair.slice(0, idx).trim();
      const value = pair.slice(idx + 1).trim();
      if (value === "") {
        this.cookies.delete(name);
      } else {
        this.cookies.set(name, value);
      }
    }
  }

  header() {
    return Array.from(this.cookies.entries())
      .map(([name, value]) => `${name}=${value}`)
      .join("; ");
  }
}

function splitSetCookie(value) {
  if (!value) return [];
  return [value];
}

async function request(baseURL, pathOrURL, options = {}) {
  const {
    method = "GET",
    body,
    headers = {},
    jar,
    expect = 200,
    text = false,
  } = options;
  const url = pathOrURL.startsWith("http") ? pathOrURL : new URL(pathOrURL, baseURL).toString();
  const finalHeaders = { ...headers };
  let finalBody = body;
  if (body !== undefined && typeof body !== "string" && !(body instanceof Uint8Array)) {
    finalBody = JSON.stringify(body);
    finalHeaders["Content-Type"] = finalHeaders["Content-Type"] || "application/json";
  }
  if (jar) {
    const cookie = jar.header();
    if (cookie) finalHeaders.Cookie = cookie;
  }
  const response = await fetch(url, {
    method,
    headers: finalHeaders,
    body: finalBody,
    redirect: "manual",
  });
  if (jar) jar.store(response);
  const raw = await response.text();
  if (response.status !== expect) {
    throw new Error(`${method} ${url} returned ${response.status}, want ${expect}: ${raw.slice(0, 500)}`);
  }
  if (text) return { response, body: raw };
  const contentType = response.headers.get("content-type") || "";
  if (!contentType.includes("application/json")) {
    return { response, body: raw };
  }
  return { response, body: raw ? JSON.parse(raw) : null };
}

function captchaAnswer(captchaOrImage) {
  const image = String(captchaOrImage?.image || captchaOrImage || "");
  const match = image.match(/^data:image\/svg\+xml(;base64)?,(.+)$/);
  assert(match, "captcha image is not an SVG data URL");
  const svg = match[1]
    ? Buffer.from(match[2], "base64").toString("utf8")
    : decodeURIComponent(match[2]);
  const answer = svg.match(/>(\d{4})</)?.[1] || svg.match(/\b(\d{4})\b/)?.[1];
  assert(answer, "could not read captcha answer from SVG");
  return answer;
}

async function waitForServer(baseURL, proc) {
  let lastError = "";
  for (let i = 0; i < 80; i += 1) {
    if (proc.exitCode !== null) {
      throw new Error(`server exited early with ${proc.exitCode}`);
    }
    try {
      const { body } = await request(baseURL, "/api/config", { expect: 200 });
      if (body?.success) return;
    } catch (error) {
      lastError = error.message;
    }
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  throw new Error(`server did not become ready: ${lastError}`);
}

async function loginAdminAPI(baseURL, jar, username, password) {
  const { body: captcha } = await request(baseURL, "/api/auth/captcha", { jar });
  const { body } = await request(baseURL, "/api/admin/login", {
    method: "POST",
    jar,
    body: {
      username,
      password,
      captchaId: captcha.id,
      captcha: captchaAnswer(captcha),
    },
  });
  assert(body.success, "admin API login did not succeed");
  return body;
}

async function seedSites(baseURL, authHeader, suffix) {
  const htmlCode = `qa-visual-html-${suffix}`;
  const mdCode = `qa-visual-md-${suffix}`;
  const multiCode = `qa-visual-multi-${suffix}`;
  const secretCode = `qa-visual-secret-${suffix}`;

  await request(baseURL, "/api/deploy", {
    method: "POST",
    headers: authHeader,
    body: {
      title: "视觉 QA HTML 应用",
      description: "用于浏览器视觉 smoke 的公开 HTML 应用",
      filename: "index.html",
      visibility: "public",
      category: "tool",
      tags: ["qa", "visual"],
      enableCustomCode: true,
      customCode: htmlCode,
      files: [{
        path: "index.html",
        content: `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>Visual QA HTML</title>
  <style>
    *,*::before,*::after{box-sizing:border-box}
    body{margin:0;font-family:system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;background:#f8fafc;color:#0f172a}
    main{min-height:100vh;display:grid;place-items:center;padding:32px}
    section{width:min(860px,100%);border:1px solid #dbeafe;border-radius:24px;background:linear-gradient(135deg,#fff,#ecfeff);padding:36px;box-shadow:0 24px 80px rgba(15,23,42,.12)}
    h1{margin:0 0 12px;font-size:clamp(30px,6vw,64px);line-height:1}
    p{font-size:18px;line-height:1.8}
  </style>
</head>
<body>
  <main><section><h1>Visual QA HTML</h1><p>PagePilot 临时浏览器视觉检查页面。</p></section></main>
</body>
</html>`,
      }],
    },
  });

  await request(baseURL, "/api/deploy", {
    method: "POST",
    headers: authHeader,
    body: {
      title: "视觉 QA 多文件站点",
      description: "用于验证 Bundle 信息、完整文件树和复用参数的公开多文件站点",
      filename: "index.html",
      visibility: "public",
      category: "tool",
      tags: ["qa", "bundle"],
      enableCustomCode: true,
      customCode: multiCode,
      files: [
        {
          path: "index.html",
          content: `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>Visual QA Multi</title>
  <link rel="stylesheet" href="assets/app.css">
</head>
<body>
  <main><h1>Visual QA Multi</h1><p>多文件静态站点。</p></main>
  <script src="scripts/app.js"></script>
</body>
</html>`,
        },
        {
          path: "assets/app.css",
          content: "body{margin:0;font-family:system-ui;background:#f8fafc;color:#0f172a}main{min-height:100vh;display:grid;place-items:center}",
        },
        {
          path: "scripts/app.js",
          content: "document.documentElement.dataset.visualMulti='ok';",
        },
      ],
    },
  });

  await request(baseURL, "/api/deploy", {
    method: "POST",
    headers: authHeader,
    body: {
      title: "视觉 QA Markdown 文档",
      description: "用于验证 Markdown 高级渲染的浏览器页面",
      filename: "README.md",
      visibility: "public",
      category: "docs",
      tags: ["qa", "markdown"],
      enableCustomCode: true,
      customCode: mdCode,
      files: [{
        path: "README.md",
        content: `# Markdown Visual QA

这是一份临时视觉检查文档，包含表格、代码、公式和 Mermaid。

| 能力 | 状态 |
| --- | --- |
| 表格 | OK |
| 代码高亮 | OK |

\`\`\`js
console.log("visual qa");
\`\`\`

\`\`\`mermaid
flowchart LR
  A[发布] --> B[预览]
\`\`\`

$$
E = mc^2
$$
`,
      }],
    },
  });

  await request(baseURL, "/api/deploy", {
    method: "POST",
    headers: authHeader,
    body: {
      title: "视觉 QA 加密应用",
      description: "用于浏览器验证匿名密码访问",
      filename: "index.html",
      visibility: "public",
      accessPassword: "visual-secret",
      enableCustomCode: true,
      customCode: secretCode,
      files: [{
        path: "index.html",
        content: "<!doctype html><html lang=\"zh-CN\"><head><meta charset=\"utf-8\"><title>Secret Visual QA</title></head><body><h1>Secret Visual QA</h1></body></html>",
      }],
    },
  });

  const bulkCodes = [];
  for (let i = 0; i < 28; i += 1) {
    const code = `qa-visual-feed-${suffix}-${String(i).padStart(2, "0")}`;
    bulkCodes.push(code);
    await request(baseURL, "/api/deploy", {
      method: "POST",
      headers: authHeader,
      body: {
        title: `Market pagination QA ${String(i + 1).padStart(2, "0")}`,
        description: "Used by visual QA to verify market pagination and load-more behavior.",
        filename: "index.html",
        visibility: "public",
        category: i % 2 === 0 ? "tool" : "docs",
        tags: ["qa", "pagination"],
        enableCustomCode: true,
        customCode: code,
        files: [{
          path: "index.html",
          content: `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>${code}</title></head><body><main><h1>${code}</h1><p>Market pagination QA item ${i + 1}</p></main></body></html>`,
        }],
      },
    });
  }

  return { htmlCode, mdCode, multiCode, secretCode, bulkCodes };
}

async function seedAuditActivity(baseURL, jar, sites) {
  for (let i = 0; i < 24; i += 1) {
    await request(baseURL, `/api/admin/sites/${encodeURIComponent(sites.htmlCode)}/pin`, {
      method: "PATCH",
      jar,
      body: { pinned: i % 2 === 0 },
    });
  }
  await request(baseURL, `/api/admin/sites/${encodeURIComponent(sites.htmlCode)}/security-mode`, {
    method: "PATCH",
    jar,
    body: { securityMode: "compatible" },
  });
  await request(baseURL, `/api/admin/sites/${encodeURIComponent(sites.htmlCode)}/reuse-policy`, {
    method: "PATCH",
    jar,
    body: { reusePolicy: "allow", sourceDownloadPolicy: "allow" },
  });
}

async function checkedPage(context, baseURL, item, viewport) {
  const issues = [];
  const page = await context.newPage();
  await page.setViewportSize(viewport);
  page.on("pageerror", (error) => issues.push(`pageerror: ${error.message}`));
  page.on("console", (msg) => {
    if (msg.type() === "error") {
      const text = msg.text();
      if (!text.includes("favicon")) issues.push(`console: ${text}`);
    }
  });
  page.on("requestfailed", (request) => {
    const url = request.url();
    if (!url.includes("favicon")) {
      issues.push(`requestfailed: ${request.method()} ${url} ${request.failure()?.errorText || ""}`);
    }
  });

  await page.goto(new URL(item.path, baseURL).toString(), { waitUntil: "domcontentloaded" });
  await page.waitForLoadState("networkidle", { timeout: 15000 }).catch(() => undefined);
  if (item.selector) {
    await page.locator(item.selector).first().waitFor({ state: "visible", timeout: 10000 });
  }
  if (item.text) {
    await page.getByText(item.text, { exact: false }).first().waitFor({ state: "visible", timeout: 10000 });
  }
  await page.waitForTimeout(250);

  const metrics = await page.evaluate(() => {
    const doc = document.documentElement;
    const body = document.body;
    const text = (body?.innerText || "").replace(/\s+/g, " ").trim();
    const viewportWidth = doc.clientWidth;
    const wideElements = Array.from(document.querySelectorAll("body *"))
      .map((element) => {
        const rect = element.getBoundingClientRect();
        const style = getComputedStyle(element);
        const parent = element.parentElement;
        return {
          tag: element.tagName.toLowerCase(),
          className: typeof element.className === "string" ? element.className : "",
          id: element.id || "",
          parent: parent && typeof parent.className === "string" ? parent.className : "",
          boxSizing: style.boxSizing,
          computedWidth: style.width,
          width: Math.round(rect.width),
          left: Math.round(rect.left),
          right: Math.round(rect.right),
        };
      })
      .filter((item) => item.width > viewportWidth + 3 || item.right > viewportWidth + 3 || item.left < -3)
      .sort((a, b) => Math.max(b.width, b.right) - Math.max(a.width, a.right))
      .slice(0, 8);
    const layoutElements = [".page-main", ".market-main", ".market-page", ".market-workspace", ".market-content"]
      .map((selector) => {
        const element = document.querySelector(selector);
        if (!element) return null;
        const rect = element.getBoundingClientRect();
        const style = getComputedStyle(element);
        return {
          selector,
          className: typeof element.className === "string" ? element.className : "",
          boxSizing: style.boxSizing,
          display: style.display,
          gridTemplateColumns: style.gridTemplateColumns,
          widthStyle: style.width,
          maxWidth: style.maxWidth,
          width: Math.round(rect.width),
          left: Math.round(rect.left),
          right: Math.round(rect.right),
        };
      })
      .filter(Boolean);
    return {
      clientWidth: doc.clientWidth,
      scrollWidth: Math.max(doc.scrollWidth, body?.scrollWidth || 0),
      clientHeight: doc.clientHeight,
      scrollHeight: Math.max(doc.scrollHeight, body?.scrollHeight || 0),
      textLength: text.length,
      title: document.title,
      wideElements,
      layoutElements,
    };
  });
  assert(metrics.textLength >= (item.minTextLength || 12), `${item.label} appears blank`);
  assert(metrics.scrollHeight >= metrics.clientHeight, `${item.label} has invalid page height`);
  assert(
    metrics.scrollWidth <= metrics.clientWidth + 3,
    `${item.label} horizontal overflow: scrollWidth=${metrics.scrollWidth}, clientWidth=${metrics.clientWidth}, layout=${JSON.stringify(metrics.layoutElements)}, wide=${JSON.stringify(metrics.wideElements)}`,
  );
  const screenshot = await page.screenshot({ fullPage: false });
  assert(screenshot.length > 1500, `${item.label} screenshot looks empty`);
  assert(!issues.length, `${item.label} browser issues:\n${issues.join("\n")}`);
  await page.close();
}

async function auditField(page, label) {
  const fields = page.locator(".audit-panel label.field");
  const count = await fields.count();
  for (let i = 0; i < count; i += 1) {
    const field = fields.nth(i);
    const title = (await field.locator("span").first().textContent())?.trim();
    if (title === label) return field;
  }
  throw new Error(`audit filter field not found: ${label}`);
}

async function selectAuditFilter(page, label, value) {
  const field = await auditField(page, label);
  await field.locator("select").selectOption(value);
  await page.waitForLoadState("networkidle", { timeout: 10000 }).catch(() => undefined);
  await page.waitForTimeout(350);
}

async function fillAuditFilter(page, label, value) {
  const field = await auditField(page, label);
  await field.locator("input").fill(value);
  await page.waitForLoadState("networkidle", { timeout: 10000 }).catch(() => undefined);
  await page.waitForTimeout(350);
}

async function waitAuditLoaded(page) {
  await page.locator(".audit-panel").waitFor({ state: "visible", timeout: 10000 });
  await page.getByText("已加载", { exact: false }).first().waitFor({ state: "visible", timeout: 10000 });
  await page.locator(".audit-table tbody tr").first().waitFor({ state: "visible", timeout: 10000 });
}

async function verifyAuditPanelUI(context, baseURL, sites, adminUserId) {
  const page = await context.newPage();
  await page.goto(`${baseURL}/admin?tab=audit`, { waitUntil: "domcontentloaded" });
  await waitAuditLoaded(page);
  await page.getByText("审计日志", { exact: false }).first().waitFor({ state: "visible", timeout: 10000 });
  await page.getByText("时间", { exact: true }).first().waitFor({ state: "visible", timeout: 10000 });
  await page.getByText("操作者", { exact: true }).first().waitFor({ state: "visible", timeout: 10000 });
  await page.getByText("详情", { exact: true }).first().waitFor({ state: "visible", timeout: 10000 });

  await selectAuditFilter(page, "每页", "20");
  await page.locator(".audit-pager button").filter({ hasText: "下一页" }).click();
  await page.getByText("2 /", { exact: false }).first().waitFor({ state: "visible", timeout: 10000 });
  await page.locator(".audit-pager button").filter({ hasText: "上一页" }).click();
  await page.getByText("1 /", { exact: false }).first().waitFor({ state: "visible", timeout: 10000 });

  await fillAuditFilter(page, "关键词", sites.htmlCode);
  await page.locator(".audit-table").getByText(sites.htmlCode, { exact: false }).first().waitFor({ state: "visible", timeout: 10000 });

  await selectAuditFilter(page, "动作", "site.pin");
  await page.locator(".audit-table").getByText("置顶状态", { exact: false }).first().waitFor({ state: "visible", timeout: 10000 });
  const actionRows = await page.locator(".audit-table tbody tr").evaluateAll((rows) =>
    rows.map((row) => row.innerText),
  );
  assert(actionRows.length > 0 && actionRows.every((text) => text.includes("site.pin")), "audit action UI filter did not isolate site.pin rows");

  await selectAuditFilter(page, "站点", sites.htmlCode);
  await page.locator(".audit-table").getByText(sites.htmlCode, { exact: false }).first().waitFor({ state: "visible", timeout: 10000 });

  await fillAuditFilter(page, "操作者 ID", adminUserId);
  await page.locator(".audit-table").getByText(adminUserId, { exact: false }).first().waitFor({ state: "visible", timeout: 10000 });

  await selectAuditFilter(page, "角色", "admin");
  const roleRows = await page.locator(".audit-table tbody tr").evaluateAll((rows) =>
    rows.map((row) => row.innerText),
  );
  assert(roleRows.length > 0 && roleRows.every((text) => text.includes("管理员")), "audit role UI filter did not isolate admin rows");
  await page.close();
}

async function verifyMarketBundleDetailUI(context, baseURL, sites) {
  const page = await context.newPage();
  await page.goto(`${baseURL}/market/${encodeURIComponent(sites.multiCode)}`, { waitUntil: "domcontentloaded" });
  await page.locator(".market-detail-layout").waitFor({ state: "visible", timeout: 10000 });
  for (const text of [
    "视觉 QA 多文件站点",
    "Bundle 信息",
    "入口",
    "index.html",
    "完整文件树",
    "assets/app.css",
    "scripts/app.js",
    "复用参数",
    "复制 CLI 命令",
    "复制 MCP 参数",
  ]) {
    await page.getByText(text, { exact: false }).first().waitFor({ state: "visible", timeout: 10000 });
  }
  await page.getByRole("button", { name: /使用此模板/ }).click();
  await page.locator(".market-use-modal").waitFor({ state: "visible", timeout: 10000 });
  for (const text of ["新建二创", "更新已有", "源文件结构", "ZIP 源码包", "Agent / MCP", "MCP 参数"]) {
    await page.getByText(text, { exact: false }).first().waitFor({ state: "visible", timeout: 10000 });
  }
  await page.locator(".market-use-modal").getByText("assets/app.css", { exact: false }).first().waitFor({ state: "visible", timeout: 10000 });
  await page.getByRole("tab", { name: "更新已有" }).click();
  await page.locator(".market-use-modal").getByText(sites.multiCode, { exact: false }).first().waitFor({ state: "visible", timeout: 10000 });
  await page.getByText(`pagep append ${sites.multiCode}`, { exact: false }).first().waitFor({ state: "visible", timeout: 10000 });
  await page.close();
}

async function verifyProtectedMarketReuseUI(context, baseURL, sites) {
  const page = await context.newPage();
  await page.goto(`${baseURL}/market/${encodeURIComponent(sites.secretCode)}`, { waitUntil: "domcontentloaded" });
  await page.locator(".market-detail-layout").waitFor({ state: "visible", timeout: 10000 });
  for (const text of [
    "视觉 QA 加密应用",
    "网页已加密",
    "访问密码",
    "加密作品不提供源码下载和模板复用",
    "当前作品不开放模板复用",
    "模板复用受限",
    "源码下载受限",
  ]) {
    await page.getByText(text, { exact: false }).first().waitFor({ state: "visible", timeout: 10000 });
  }
  await assertDisabledButton(page, "模板复用受限");
  await assertDisabledButton(page, "源码下载受限");
  await page.close();
}

async function assertDisabledButton(page, name) {
  const button = page.getByRole("button", { name }).first();
  await button.waitFor({ state: "visible", timeout: 10000 });
  assert(await button.isDisabled(), `${name} button should be disabled`);
}

async function verifyMarketPaginationUI(context, baseURL, sites) {
  assert(sites.bulkCodes?.length > 24, "visual QA market pagination seed must exceed one page");
  const page = await context.newPage();
  await page.goto(`${baseURL}/market`, { waitUntil: "domcontentloaded" });
  await page.locator(".market-page").waitFor({ state: "visible", timeout: 10000 });
  await page.locator(".app-card").first().waitFor({ state: "visible", timeout: 10000 });

  const initialCount = await page.locator(".app-card").count();
  assert(initialCount >= 20 && initialCount <= 24, `market first page count = ${initialCount}, want one page`);

  const loadMore = page.locator(".market-load-more button").first();
  await loadMore.waitFor({ state: "visible", timeout: 10000 });
  await loadMore.click();
  await page.waitForFunction(
    (count) => document.querySelectorAll(".app-card").length > count,
    initialCount,
    { timeout: 10000 },
  );

  const expandedCount = await page.locator(".app-card").count();
  assert(expandedCount > initialCount, `market load-more did not append cards: ${initialCount} -> ${expandedCount}`);
  await page.getByText("Market pagination QA 01", { exact: false }).first().waitFor({ state: "visible", timeout: 10000 });
  await page.close();
}

async function verifyAdminSiteBundleDetailUI(context, baseURL, sites) {
  const page = await context.newPage();
  await page.goto(`${baseURL}/admin?tab=sites`, { waitUntil: "domcontentloaded" });
  await page.locator(".admin-shell").waitFor({ state: "visible", timeout: 10000 });
  await page.locator(".site-toolbar .search-box input").fill(sites.multiCode);
  await page.locator("tbody tr").filter({ hasText: sites.multiCode }).first().waitFor({ state: "visible", timeout: 10000 });
  await page.locator("tbody tr").filter({ hasText: sites.multiCode }).first().getByRole("button", { name: "详情" }).click();
  await page.locator(".site-detail-modal").waitFor({ state: "visible", timeout: 10000 });
  for (const text of [
    sites.multiCode,
    "源码下载与模板复用",
    "运行安全模式",
    "Bundle / 安全",
    "入口",
    "index.html",
    "完整文件树",
    "assets/app.css",
    "scripts/app.js",
    "复用参数",
    "复制 Agent 提示词",
    "复制 CLI 命令",
    "复制 MCP 参数",
    "最近审计",
  ]) {
    await page.getByText(text, { exact: false }).first().waitFor({ state: "visible", timeout: 10000 });
  }
  await page.locator(".site-detail-modal .file-tree-search input").fill("scripts");
  await page.locator(".site-detail-modal").getByText("scripts/app.js", { exact: false }).first().waitFor({ state: "visible", timeout: 10000 });
  await page.close();
}

async function verifyLoginFlow(browser, baseURL, username, password) {
  const context = await browser.newContext({ viewport: { width: 1366, height: 900 } });
  const page = await context.newPage();
  await page.goto(`${baseURL}/admin`, { waitUntil: "domcontentloaded" });
  await page.locator(".login-page").waitFor({ state: "visible", timeout: 10000 });
  assert(!(await page.locator(".admin-shell").count()), "unauthenticated admin page should not show admin shell");

  await page.goto(`${baseURL}/admin?mode=register`, { waitUntil: "domcontentloaded" });
  await page.locator(".login-page").waitFor({ state: "visible", timeout: 10000 });
  await page.getByText("注册账号", { exact: false }).first().waitFor({ state: "visible", timeout: 10000 });

  await page.goto(`${baseURL}/admin`, { waitUntil: "domcontentloaded" });
  await page.locator("img.captcha-image").waitFor({ state: "visible", timeout: 10000 });
  const src = await page.locator("img.captcha-image").getAttribute("src");
  await page.locator('input[autocomplete="username"]').fill(username);
  await page.locator('input[type="password"]').fill(password);
  await page.locator('input[inputmode="numeric"]').first().fill(captchaAnswer(src));
  await page.locator('button[type="submit"]').click();
  await page.locator(".admin-shell").waitFor({ state: "visible", timeout: 10000 });
  await page.close();
  return context;
}

async function verifyProtectedAccess(browser, baseURL, code) {
  const context = await browser.newContext({ viewport: { width: 390, height: 844 } });
  const page = await context.newPage();
  await page.goto(`${baseURL}/agent/${encodeURIComponent(code)}/`, { waitUntil: "domcontentloaded" });
  await page.getByText("这个网页已加密", { exact: false }).first().waitFor({ state: "visible", timeout: 10000 });
  await page.locator("#p").fill("visual-secret");
  await page.locator('button[type="submit"]').click();
  await page.getByText("Secret Visual QA", { exact: false }).first().waitFor({ state: "visible", timeout: 10000 });
  const metrics = await page.evaluate(() => ({
    clientWidth: document.documentElement.clientWidth,
    scrollWidth: Math.max(document.documentElement.scrollWidth, document.body.scrollWidth),
  }));
  assert(metrics.scrollWidth <= metrics.clientWidth + 3, "protected access page has horizontal overflow");
  await context.close();
}

async function main() {
  const { chromium } = loadPlaywright();
  const tmp = await mkdtemp(path.join(tmpdir(), "pagepilot-visual-qa-"));
  const port = await freePort();
  const baseURL = `http://127.0.0.1:${port}`;
  const exe = path.join(tmp, process.platform === "win32" ? "hostctl-server-visual-qa.exe" : "hostctl-server-visual-qa");
  const adminUser = "visual_admin";
  const adminPassword = "visual_Admin_Pass123!";
  const suffix = Date.now().toString(36);

  const build = spawnSync("go", ["build", "-o", exe, "./cmd/hostctl-server"], {
    cwd: rootDir,
    stdio: "inherit",
  });
  assert(build.status === 0, "go build ./cmd/hostctl-server failed");

  const server = spawn(exe, [], {
    cwd: rootDir,
    env: {
      ...process.env,
      HOSTCTL_HTTP_ADDR: `127.0.0.1:${port}`,
      HOSTCTL_HOSTED_DIR: path.join(tmp, "hosted"),
      HOSTCTL_DB_PATH: path.join(tmp, "hostctl.db"),
      HOSTCTL_DEV: "1",
      HOSTCTL_ALLOW_REGISTRATION: "1",
      HOSTCTL_EMAIL_VERIFICATION_ENABLED: "0",
      HOSTCTL_COOLDOWN_SECONDS: "0",
      HOSTCTL_ANONYMOUS_DEPLOY_LIMIT: "10",
      HOSTCTL_ADMIN_USERNAME: adminUser,
      HOSTCTL_ADMIN_PASSWORD: adminPassword,
    },
    stdio: ["ignore", "pipe", "pipe"],
    windowsHide: true,
  });
  const logs = [];
  server.stdout.on("data", (data) => logs.push(String(data)));
  server.stderr.on("data", (data) => logs.push(String(data)));

  let browser;
  try {
    await waitForServer(baseURL, server);
    const adminJar = new CookieJar();
    const admin = await loginAdminAPI(baseURL, adminJar, adminUser, adminPassword);
    const { body: tokenResp } = await request(baseURL, "/api/token", {
      method: "POST",
      jar: adminJar,
      body: { label: "visual-qa", isAdmin: false },
    });
    assert(tokenResp.ownerUserId === admin.userId, "visual QA token owner mismatch");
    const authHeader = { Authorization: `Bearer ${tokenResp.token}` };
    const sites = await seedSites(baseURL, authHeader, suffix);
    await seedAuditActivity(baseURL, adminJar, sites);

    browser = await launchChromium(chromium);
    const adminContext = await verifyLoginFlow(browser, baseURL, adminUser, adminPassword);
    await verifyAuditPanelUI(adminContext, baseURL, sites, admin.userId);
    await verifyMarketPaginationUI(adminContext, baseURL, sites);
    await verifyMarketBundleDetailUI(adminContext, baseURL, sites);
    await verifyProtectedMarketReuseUI(adminContext, baseURL, sites);
    await verifyAdminSiteBundleDetailUI(adminContext, baseURL, sites);

    const desktop = { width: 1440, height: 960 };
    const mobile = { width: 390, height: 844 };
    const publicPages = [
      { label: "首页", path: "/", selector: ".page-main" },
      { label: "创作市场", path: "/market", selector: ".market-page", text: "视觉 QA HTML 应用" },
      { label: "市场详情", path: `/market/${sites.multiCode}`, selector: ".market-detail-layout", text: "视觉 QA 多文件站点" },
      { label: "手动部署", path: "/deploy", selector: ".deploy-format-strip" },
      { label: "Skill & MCP", path: "/agents/", selector: ".content-page", text: "Skill" },
      { label: "屏幕介绍", path: "/screens/", selector: ".screen-page-v2" },
      { label: "HTML 应用运行页", path: `/agent/${sites.htmlCode}/`, text: "Visual QA HTML" },
      { label: "Markdown 运行页", path: `/agent/${sites.mdCode}/?theme=dark`, text: "Markdown Visual QA" },
    ];
    const adminTabs = [
      "overview",
      "deploy",
      "sites",
      "categories",
      "screens",
      "tokens",
      "account",
      "users",
      "anonymous",
      "audit",
      "config",
      "skill",
      "apiDocs",
    ];

    for (const viewport of [desktop, mobile]) {
      for (const item of publicPages) {
        await checkedPage(adminContext, baseURL, item, viewport);
      }
    }

    for (const tab of adminTabs) {
      await checkedPage(adminContext, baseURL, {
        label: `后台 ${tab}`,
        path: `/admin?tab=${tab}`,
        selector: ".admin-shell",
      }, desktop);
    }
    for (const tab of ["overview", "sites", "skill", "apiDocs"]) {
      await checkedPage(adminContext, baseURL, {
        label: `后台移动端 ${tab}`,
        path: `/admin?tab=${tab}`,
        selector: ".admin-shell",
      }, mobile);
    }

    await verifyProtectedAccess(browser, baseURL, sites.secretCode);
    await adminContext.close();
    console.log("visual QA passed");
    console.log(`checked: ${sites.htmlCode}, ${sites.mdCode}, ${sites.multiCode}, ${sites.secretCode}`);
  } finally {
    if (browser) await browser.close();
    if (!server.killed && server.exitCode === null) {
      server.kill("SIGKILL");
      await Promise.race([
        once(server, "exit"),
        new Promise((resolve) => setTimeout(resolve, 3000)),
      ]);
    }
    if (server.exitCode && server.exitCode !== 0 && logs.length) {
      console.error(logs.join(""));
    }
    await rm(tmp, { recursive: true, force: true });
  }
}

main().catch((error) => {
  console.error(error.stack || error.message);
  process.exit(1);
});
