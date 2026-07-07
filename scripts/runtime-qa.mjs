import { spawn, spawnSync } from "node:child_process";
import { once } from "node:events";
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
  const isFormData = typeof FormData !== "undefined" && body instanceof FormData;
  if (body !== undefined && typeof body !== "string" && !(body instanceof Uint8Array) && !isFormData) {
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

function captchaAnswer(captcha) {
  const image = String(captcha.image || "");
  const match = image.match(/^data:image\/svg\+xml(;base64)?,(.+)$/);
  assert(match, "captcha image is not an SVG data URL");
  const svg = match[1]
    ? Buffer.from(match[2], "base64").toString("utf8")
    : decodeURIComponent(match[2]);
  const answer = svg.match(/>(\d{4})</)?.[1] || svg.match(/\b(\d{4})\b/)?.[1];
  assert(answer, "could not read captcha answer from SVG");
  return answer;
}

function makeZipBase64(files) {
  const chunks = [];
  const central = [];
  let offset = 0;
  for (const [name, content] of Object.entries(files)) {
    const nameBuf = Buffer.from(name, "utf8");
    const data = Buffer.from(content, "utf8");
    const crc = crc32(data);
    const local = Buffer.alloc(30);
    local.writeUInt32LE(0x04034b50, 0);
    local.writeUInt16LE(20, 4);
    local.writeUInt16LE(0, 6);
    local.writeUInt16LE(0, 8);
    local.writeUInt16LE(0, 10);
    local.writeUInt16LE(0, 12);
    local.writeUInt32LE(crc, 14);
    local.writeUInt32LE(data.length, 18);
    local.writeUInt32LE(data.length, 22);
    local.writeUInt16LE(nameBuf.length, 26);
    local.writeUInt16LE(0, 28);
    chunks.push(local, nameBuf, data);

    const header = Buffer.alloc(46);
    header.writeUInt32LE(0x02014b50, 0);
    header.writeUInt16LE(20, 4);
    header.writeUInt16LE(20, 6);
    header.writeUInt16LE(0, 8);
    header.writeUInt16LE(0, 10);
    header.writeUInt16LE(0, 12);
    header.writeUInt16LE(0, 14);
    header.writeUInt32LE(crc, 16);
    header.writeUInt32LE(data.length, 20);
    header.writeUInt32LE(data.length, 24);
    header.writeUInt16LE(nameBuf.length, 28);
    header.writeUInt16LE(0, 30);
    header.writeUInt16LE(0, 32);
    header.writeUInt16LE(0, 34);
    header.writeUInt16LE(0, 36);
    header.writeUInt32LE(0, 38);
    header.writeUInt32LE(offset, 42);
    central.push(header, nameBuf);
    offset += local.length + nameBuf.length + data.length;
  }
  const centralStart = offset;
  const centralDir = Buffer.concat(central);
  const end = Buffer.alloc(22);
  end.writeUInt32LE(0x06054b50, 0);
  end.writeUInt16LE(0, 4);
  end.writeUInt16LE(0, 6);
  end.writeUInt16LE(Object.keys(files).length, 8);
  end.writeUInt16LE(Object.keys(files).length, 10);
  end.writeUInt32LE(centralDir.length, 12);
  end.writeUInt32LE(centralStart, 16);
  end.writeUInt16LE(0, 20);
  return Buffer.concat([...chunks, centralDir, end]).toString("base64");
}

const crcTable = (() => {
  const table = new Uint32Array(256);
  for (let i = 0; i < table.length; i += 1) {
    let c = i;
    for (let k = 0; k < 8; k += 1) {
      c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
    }
    table[i] = c >>> 0;
  }
  return table;
})();

function crc32(data) {
  let c = 0xffffffff;
  for (const byte of data) {
    c = crcTable[(c ^ byte) & 0xff] ^ (c >>> 8);
  }
  return (c ^ 0xffffffff) >>> 0;
}

async function assertZipDeployError(baseURL, authHeader, name, zipFiles, expectedCode) {
  const { body } = await request(baseURL, "/api/deploy", {
    method: "POST",
    headers: authHeader,
    expect: 400,
    body: {
      title: `运行时 QA ZIP 错误 ${name}`,
      description: "用于验证 ZIP/Bundle 错误提示产品化",
      filename: "site.zip",
      visibility: "unlisted",
      files: [{
        path: "site.zip",
        contentBase64: makeZipBase64(zipFiles),
      }],
    },
  });
  assert(body?.success === false, `${name} ZIP error response must be unsuccessful`);
  assert(body?.stage === "zip_bundle", `${name} ZIP error stage = ${body?.stage}`);
  assert(body?.errorCode === expectedCode, `${name} ZIP errorCode = ${body?.errorCode}, want ${expectedCode}`);
  assert(String(body?.detail || "").trim(), `${name} ZIP error missing detail`);
  assert(String(body?.hint || "").trim(), `${name} ZIP error missing user hint`);
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

async function loginAdmin(baseURL, jar, username, password) {
  const { body: captcha } = await request(baseURL, "/api/auth/captcha", { jar });
  const answer = captchaAnswer(captcha);
  const { body } = await request(baseURL, "/api/admin/login", {
    method: "POST",
    jar,
    body: {
      username,
      password,
      captchaId: captcha.id,
      captcha: answer,
    },
  });
  assert(body.success, "admin login did not succeed");
  assert(body.userId, "admin login response missing userId");
  return body;
}

async function registerUser(baseURL, jar, username, password, email = "", expect = 200) {
  const { body: captcha } = await request(baseURL, "/api/auth/captcha", { jar });
  const answer = captchaAnswer(captcha);
  const { body } = await request(baseURL, "/api/auth/register", {
    method: "POST",
    jar,
    expect,
    body: {
      username,
      email,
      password,
      captchaId: captcha.id,
      captcha: answer,
    },
  });
  return body;
}

async function main() {
  const tmp = await mkdtemp(path.join(tmpdir(), "pagepilot-runtime-qa-"));
  const port = await freePort();
  const baseURL = `http://127.0.0.1:${port}`;
  const exe = path.join(tmp, process.platform === "win32" ? "hostctl-server-qa.exe" : "hostctl-server-qa");
  const adminUser = "qa_admin";
  const adminPassword = "qa_admin_Pass123!";
  const suffix = Date.now().toString(36);
  const mdCode = `qa-md-${suffix}`;
  const zipCode = `qa-zip-${suffix}`;
  const protectedCode = `qa-secret-${suffix}`;
  const reusedCode = `qa-reuse-${suffix}`;
  const versionAuditCode = `qa-version-${suffix}`;
  const ownerDeleteCode = `qa-owner-delete-${suffix}`;
  const adminDeleteCode = `qa-admin-delete-${suffix}`;
  const anonymousClaimCode = `qa-anon-${suffix}`;
  const runtimeUser = `qa_user_${suffix}`;
  const runtimeUserPassword = `qa_user_Pass_${suffix}!`;
  const managedUser = `qa_managed_${suffix}`;
  const managedUserPassword = `qa_managed_Pass_${suffix}!`;

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

  try {
    await waitForServer(baseURL, server);
    const qaAuditSince = new Date(Date.now() - 1000).toISOString();
    const adminJar = new CookieJar();
    const publicJar = new CookieJar();
    const admin = await loginAdmin(baseURL, adminJar, adminUser, adminPassword);

    const { body: tokenResp } = await request(baseURL, "/api/token", {
      method: "POST",
      jar: adminJar,
      body: { label: "runtime-qa", isAdmin: false },
    });
    assert(tokenResp.token, "token creation response missing plaintext token");
    assert(tokenResp.ownerUserId === admin.userId, "token owner is not the logged-in admin user");
    const authHeader = { Authorization: `Bearer ${tokenResp.token}` };

    await assertZipDeployError(baseURL, authHeader, "批量多入口", {
      "one/index.html": "<!doctype html><html><body><h1>one</h1></body></html>",
      "two/index.html": "<!doctype html><html><body><h1>two</h1></body></html>",
    }, "ZIP_AMBIGUOUS_ENTRY");
    await assertZipDeployError(baseURL, authHeader, "缺少入口", {
      "assets/app.css": "body{color:#0f172a}",
      "assets/app.js": "console.log('no entry')",
    }, "ZIP_ENTRY_MISSING");
    await assertZipDeployError(baseURL, authHeader, "路径穿越", {
      "../index.html": "<!doctype html><html><body><h1>bad</h1></body></html>",
    }, "ZIP_UNSAFE_PATH");

    const changedAdminPassword = `${adminPassword}_Next456!`;
    await request(baseURL, "/api/account/password", {
      method: "PATCH",
      jar: adminJar,
      body: {
        oldPassword: adminPassword,
        newPassword: changedAdminPassword,
      },
    });
    const auditJar = new CookieJar();
    const auditAdmin = await loginAdmin(baseURL, auditJar, adminUser, changedAdminPassword);
    assert(auditAdmin.userId === admin.userId, "password change did not preserve admin user identity");
    const { body: authAudit } = await request(baseURL, `/api/admin/audit-logs?actorId=${encodeURIComponent(admin.userId)}&pageSize=50`, {
      jar: auditJar,
    });
    const authLogs = authAudit.logs || [];
    assert(authLogs.some((item) => item.action === "auth.login" && item.result === "success"), "runtime QA missing auth.login audit log");
    const accountPasswordLog = authLogs.find((item) => item.action === "account.password" && item.result === "success");
    assert(accountPasswordLog, "runtime QA missing account.password audit log");
    const authAuditJSON = JSON.stringify(authLogs.map((item) => item.detail || {}));
    assert(!authAuditJSON.includes(adminPassword), "auth/account audit leaked old password");
    assert(!authAuditJSON.includes(changedAdminPassword), "auth/account audit leaked new password");

    const registerJar = new CookieJar();
    const registeredUser = await registerUser(
      baseURL,
      registerJar,
      runtimeUser,
      runtimeUserPassword,
      `${runtimeUser}@example.test`,
    );
    assert(registeredUser.userId, "runtime QA register response missing userId");
    await registerUser(baseURL, new CookieJar(), runtimeUser, `${runtimeUserPassword}_dup`, "", 400);
    const { body: registerAudit } = await request(baseURL, `/api/admin/audit-logs?action=auth.register&q=${encodeURIComponent(runtimeUser)}&pageSize=20`, {
      jar: auditJar,
    });
    const registerLogs = registerAudit.logs || [];
    assert(registerLogs.some((item) => item.action === "auth.register" && item.result === "success" && item.targetId === registeredUser.userId), "runtime QA missing successful auth.register audit log");
    assert(registerLogs.some((item) => item.action === "auth.register" && item.result === "failed" && item.targetId === runtimeUser), "runtime QA missing failed auth.register audit log");
    const registerAuditJSON = JSON.stringify(registerLogs.map((item) => item.detail || {}));
    assert(!registerAuditJSON.includes(runtimeUserPassword), "register audit leaked plaintext password");
    assert(!registerAuditJSON.includes(`${runtimeUserPassword}_dup`), "failed register audit leaked plaintext password");

    const anonymousJar = new CookieJar();
    const { body: anonymousSession } = await request(baseURL, "/api/session", {
      jar: anonymousJar,
      headers: {
        "X-Hostctl-Agent-Id": "runtime-qa-agent",
        "X-Hostctl-Agent-Label": "Runtime QA Agent",
      },
    });
    assert(anonymousSession.sessionId, "anonymous session response missing sessionId");
    const { body: anonymousDeploy } = await request(baseURL, "/api/deploy", {
      method: "POST",
      jar: anonymousJar,
      body: {
        title: "运行时 QA 匿名发布",
        description: "用于验证匿名发布认领和归属迁移",
        filename: "index.html",
        visibility: "unlisted",
        enableCustomCode: true,
        customCode: anonymousClaimCode,
        files: [{
          path: "index.html",
          content: "<!doctype html><html><head><meta charset=\"utf-8\"><title>Anonymous Claim QA</title></head><body><main><h1>anonymous claim qa</h1></main></body></html>",
        }],
      },
    });
    assert(anonymousDeploy.success && anonymousDeploy.code === anonymousClaimCode, "anonymous deploy failed before claim");
    const { body: anonymousBeforeClaim } = await request(baseURL, `/api/admin/sites/${encodeURIComponent(anonymousClaimCode)}`, {
      jar: adminJar,
    });
    assert(anonymousBeforeClaim.site?.ownerTokenId === `anon:${anonymousSession.sessionId}`, "anonymous site owner did not use anonymous session before claim");

    const runtimeUserJar = new CookieJar();
    const runtimeLogin = await loginAdmin(baseURL, runtimeUserJar, runtimeUser, runtimeUserPassword);
    assert(runtimeLogin.userId === registeredUser.userId && runtimeLogin.isAdmin === false, "runtime user login failed before anonymous claim");
    const { body: claimResult } = await request(baseURL, "/api/session/claim", {
      method: "POST",
      jar: runtimeUserJar,
      body: { sessionId: anonymousSession.sessionId },
    });
    assert(
      claimResult.success &&
      claimResult.userId === registeredUser.userId &&
      claimResult.sessionId === anonymousSession.sessionId &&
      claimResult.siteCount === 1 &&
      claimResult.deployCount === 1 &&
      claimResult.alreadyClaimed === false,
      `anonymous claim result unexpected: ${JSON.stringify(claimResult)}`,
    );
    const { body: anonymousAfterClaim } = await request(baseURL, `/api/admin/sites/${encodeURIComponent(anonymousClaimCode)}`, {
      jar: adminJar,
    });
    assert(anonymousAfterClaim.site?.ownerTokenId === `user:${registeredUser.userId}`, "anonymous site owner did not migrate to user after claim");
    await request(baseURL, "/api/deploy", {
      method: "POST",
      jar: anonymousJar,
      expect: 401,
      body: {
        title: "运行时 QA 已认领匿名继续发布",
        description: "已认领匿名 session 不应继续发布",
        filename: "index.html",
        enableCustomCode: true,
        customCode: `${anonymousClaimCode}-after`,
        files: [{
          path: "index.html",
          content: "<!doctype html><html><body>should be rejected</body></html>",
        }],
      },
    });
    const { body: anonymousSessions } = await request(baseURL, "/api/admin/anonymous-sessions", {
      jar: adminJar,
    });
    const claimedSession = (anonymousSessions.sessions || []).find((item) => item.id === anonymousSession.sessionId);
    assert(claimedSession?.claimedByUserId === registeredUser.userId, "admin anonymous session list did not show claimed user");
    const { body: claimAudit } = await request(baseURL, `/api/admin/audit-logs?action=anonymous.claim&targetType=anonymous_session&targetId=${encodeURIComponent(anonymousSession.sessionId)}&pageSize=10`, {
      jar: adminJar,
    });
    const claimLog = (claimAudit.logs || []).find((item) => item.action === "anonymous.claim" && item.result === "success");
    assert(claimLog?.actorId === registeredUser.userId && claimLog?.actorRole === "user", "runtime QA missing anonymous.claim user audit log");
    assert(claimLog?.detail?.siteCount === 1 && claimLog?.detail?.deployCount === 1 && claimLog?.detail?.auto === false, "anonymous.claim audit detail is incomplete");

    const markdown = `# PagePilot QA 文档

Inline math $a+b$ and autolink https://example.com.

Inline code \`$HOME$\` should stay code, not math.

[bad link](javascript&#58;alert(1))

<a href="javascript&#58;alert(1)" onclick=alert(1)>bad raw link</a>
<img src="data:image/svg+xml;base64,PHN2ZyBvbmxvYWQ9YWxlcnQoMSk+" onerror=alert(1)>
<img src="images/logo.svg" srcset="images/logo.svg 1x, data:image/svg+xml;base64,PHN2ZyBvbmxvYWQ9YWxlcnQoMSk+ 2x">
<svg><a xlink:href="javascript:alert(1)">bad namespace url</a></svg>

![logo](images/logo.svg)

| 能力 | 状态 |
| --- | --- |
| 表格 | OK |
| ~~旧逻辑~~ | 已替换 |

- [x] 发布
- [ ] 复查

\`\`\`go
fmt.Println("qa")
\`\`\`

\`\`\`sh
echo "$HOME$"
\`\`\`

\`\`\`mermaid title=qa-flow
flowchart LR
  A[开始] --> B[结束]
\`\`\`

\`\`\`katex display
x = {-b \\pm \\sqrt{b^2-4ac} \\over 2a}
\`\`\`

$$
E = mc^2
$$
`;

    const { body: mdDeploy } = await request(baseURL, "/api/deploy", {
      method: "POST",
      headers: authHeader,
      body: {
        title: "运行时 QA Markdown 文档",
        description: "用于验证 Markdown 高级渲染、Bundle 详情、复用和审计日志",
        filename: "README.md",
        visibility: "public",
        category: "docs",
        tags: ["qa", "markdown"],
        enableCustomCode: true,
        customCode: mdCode,
        files: [
          { path: "README.md", content: markdown },
          {
            path: "images/logo.svg",
            content: `<svg xmlns="http://www.w3.org/2000/svg" width="80" height="32"><text x="4" y="22">QA</text></svg>`,
          },
        ],
      },
    });
    assert(mdDeploy.success && mdDeploy.code === mdCode, "markdown deploy failed");

    const { body: mdAppend } = await request(baseURL, "/api/deploy", {
      method: "POST",
      headers: authHeader,
      body: {
        title: "运行时 QA Markdown 文档 v2",
        description: "追加版本用于验证版本和审计",
        filename: "README.md",
        visibility: "public",
        enableCustomCode: true,
        customCode: mdCode,
        createVersion: true,
        files: [
          { path: "README.md", content: `${markdown}\n\n追加版本。` },
          {
            path: "images/logo.svg",
            content: `<svg xmlns="http://www.w3.org/2000/svg" width="80" height="32"><text x="4" y="22">QA2</text></svg>`,
          },
        ],
      },
    });
    assert(mdAppend.versionNumber === 2, "append deploy did not create v2");

    await request(baseURL, `/api/admin/sites/${encodeURIComponent(mdCode)}/pin`, {
      method: "PATCH",
      jar: adminJar,
      body: { pinned: true },
    });
    await request(baseURL, `/api/admin/sites/${encodeURIComponent(mdCode)}/security-mode`, {
      method: "PATCH",
      jar: adminJar,
      body: { securityMode: "compatible" },
    });
    await request(baseURL, `/api/admin/sites/${encodeURIComponent(mdCode)}/reuse-policy`, {
      method: "PATCH",
      jar: adminJar,
      body: { reusePolicy: "allow", sourceDownloadPolicy: "allow" },
    });

    const { body: adminDetail } = await request(baseURL, `/api/admin/sites/${encodeURIComponent(mdCode)}`, {
      jar: adminJar,
    });
    assert(adminDetail.bundle?.kind === "markdown", "admin detail did not report markdown bundle");
    assert(adminDetail.bundle?.mainEntry === "README.md", "admin detail missing markdown entry");
    assert(adminDetail.bundle?.fileCount === 2, "admin detail missing file count");
    assert(Array.isArray(adminDetail.files) && adminDetail.files.some((file) => file.path === "images/logo.svg"), "admin detail missing file tree");
    assert(adminDetail.reuse?.allowDownload === true && adminDetail.reuse?.allowReuse === true, "admin detail reuse policy not available");

    const { body: marketList } = await request(baseURL, `/api/deploys?q=${encodeURIComponent(mdCode)}&sort=newest&pageSize=10`);
    assert(marketList.deploys?.some((item) => item.code === mdCode), "market list does not include public markdown deploy");
    const { body: marketDetail } = await request(baseURL, `/api/deploys/${encodeURIComponent(mdCode)}`);
    assert(marketDetail.bundle?.kind === "markdown", "market detail missing markdown bundle");
    assert(marketDetail.reuse?.allowDownload === true, "market detail should allow source download for public unprotected site");
    assert(String(marketDetail.reuse?.cli || "").includes("pagep get"), "market detail missing CLI reuse command");
    assert(marketDetail.reuse?.mcp?.tool === "deploy_site", "market detail missing MCP reuse parameters");

    const { body: markdownHTML, response: markdownResp } = await request(baseURL, `/agent/${encodeURIComponent(mdCode)}/?theme=dark`, {
      text: true,
    });
    const markdownCSP = markdownResp.headers.get("content-security-policy") || "";
    assert(markdownHTML.includes('data-theme="dark"'), "markdown runtime did not apply explicit theme");
    assert(markdownHTML.includes('class="chroma"'), "markdown runtime missing code highlight HTML");
    assert(markdownHTML.includes('class="mermaid"'), "markdown runtime missing Mermaid container");
    assert(markdownHTML.includes("data-pagepilot-math-inline"), "markdown runtime missing inline math marker");
    assert(markdownHTML.includes("data-pagepilot-math-block"), "markdown runtime missing block math marker");
    assert(markdownHTML.includes("$HOME$"), "markdown runtime did not preserve dollar content inside code");
    assert(!markdownHTML.includes('<code><span class="markdown-math-inline"'), "markdown runtime converted code span dollars into math");
    assert(!markdownHTML.toLowerCase().includes("javascript&#58;"), "markdown runtime kept encoded javascript URL");
    assert(!markdownHTML.toLowerCase().includes("onclick="), "markdown runtime kept onclick handler");
    assert(!markdownHTML.toLowerCase().includes("onerror="), "markdown runtime kept onerror handler");
    assert(!markdownHTML.toLowerCase().includes("data:image/svg+xml"), "markdown runtime kept SVG data URL");
    assert(!markdownHTML.toLowerCase().includes('srcset="data:'), "markdown runtime kept unsafe srcset");
    assert(!markdownHTML.toLowerCase().includes("xlink:href"), "markdown runtime kept namespaced active URL attribute");
    assert(markdownHTML.includes('/markdown-assets/katex/katex.min.js" nonce="'), "markdown runtime missing nonce on KaTeX runtime");
    assert(markdownHTML.includes('/markdown-assets/katex/contrib/auto-render.min.js" nonce="'), "markdown runtime missing nonce on KaTeX auto-render runtime");
    assert(markdownHTML.includes('/markdown-assets/mermaid/mermaid.min.js" nonce="'), "markdown runtime missing nonce on Mermaid runtime");
    assert(markdownCSP.includes("script-src 'nonce-"), "markdown CSP missing nonce-only script policy");
    assert(!markdownCSP.includes("script-src 'self'"), "markdown CSP must not allow broad same-origin scripts");
    assert(!markdownCSP.includes("script-src 'self' 'unsafe-inline'"), "markdown CSP must not allow unsafe-inline scripts");

    const { response: downloadResp } = await request(baseURL, `/api/deploy/content?code=${encodeURIComponent(mdCode)}&download=1`, {
      headers: authHeader,
      text: true,
    });
    assert(downloadResp.headers.get("content-type")?.includes("application/zip"), "multi-file source download should be a zip");
    const { body: sourceDownloadAudit } = await request(baseURL, `/api/admin/audit-logs?siteCode=${encodeURIComponent(mdCode)}&action=source_download&pageSize=10`, {
      jar: adminJar,
    });
    const sourceDownloadLog = (sourceDownloadAudit.logs || []).find((item) => item.action === "source_download" && item.result === "success");
    assert(sourceDownloadLog?.targetType === "site" && sourceDownloadLog?.targetId === mdCode, "runtime QA missing successful source_download audit log");
    assert(sourceDownloadLog?.detail?.download === true, "successful source_download audit detail missing download flag");

    const { body: versionV1 } = await request(baseURL, "/api/deploy", {
      method: "POST",
      headers: authHeader,
      body: {
        title: "运行时 QA 版本审计",
        description: "用于验证版本管理审计日志",
        filename: "index.html",
        visibility: "unlisted",
        enableCustomCode: true,
        customCode: versionAuditCode,
        files: [{
          path: "index.html",
          content: "<!doctype html><html><head><meta charset=\"utf-8\"><title>Version Audit v1</title></head><body><main><h1>version audit v1</h1></main></body></html>",
        }],
      },
    });
    assert(versionV1.versionNumber === 1, "version audit v1 deploy failed");
    const { body: versionV2 } = await request(baseURL, "/api/deploy", {
      method: "POST",
      headers: authHeader,
      body: {
        title: "运行时 QA 版本审计 v2",
        description: "用于验证版本切换和覆盖审计",
        filename: "index.html",
        visibility: "unlisted",
        enableCustomCode: true,
        customCode: versionAuditCode,
        createVersion: true,
        files: [{
          path: "index.html",
          content: "<!doctype html><html><head><meta charset=\"utf-8\"><title>Version Audit v2</title></head><body><main><h1>version audit v2</h1></main></body></html>",
        }],
      },
    });
    assert(versionV2.versionNumber === 2, "version audit v2 append failed");
    const { body: versionV3 } = await request(baseURL, "/api/deploy", {
      method: "POST",
      headers: authHeader,
      body: {
        title: "运行时 QA 版本审计 v3",
        description: "用于验证版本删除审计",
        filename: "index.html",
        visibility: "unlisted",
        enableCustomCode: true,
        customCode: versionAuditCode,
        createVersion: true,
        files: [{
          path: "index.html",
          content: "<!doctype html><html><head><meta charset=\"utf-8\"><title>Version Audit v3</title></head><body><main><h1>version audit v3</h1></main></body></html>",
        }],
      },
    });
    assert(versionV3.versionNumber === 3, "version audit v3 append failed");
    await request(baseURL, `/api/deploys/${encodeURIComponent(versionAuditCode)}/versions/1`, {
      method: "PATCH",
      headers: authHeader,
      body: { status: "inactive" },
    });
    await request(baseURL, `/api/deploys/${encodeURIComponent(versionAuditCode)}/versions/1/lock`, {
      method: "POST",
      headers: authHeader,
      body: { locked: true },
    });
    await request(baseURL, `/api/deploys/${encodeURIComponent(versionAuditCode)}/current`, {
      method: "PATCH",
      headers: authHeader,
      body: { versionNumber: 2 },
    });
    await request(baseURL, `/api/deploys/${encodeURIComponent(versionAuditCode)}/versions/2`, {
      method: "PATCH",
      headers: authHeader,
      body: {
        title: "运行时 QA 版本审计 v2 覆盖",
        description: "覆盖 v2 以验证 version.overwrite 审计",
        filename: "index.html",
        files: [{
          path: "index.html",
          content: "<!doctype html><html><head><meta charset=\"utf-8\"><title>Version Audit v2 overwrite</title></head><body><main><h1>version audit v2 overwrite</h1></main></body></html>",
        }],
      },
    });
    await request(baseURL, `/api/deploys/${encodeURIComponent(versionAuditCode)}/versions/3`, {
      method: "DELETE",
      headers: authHeader,
    });
    const { body: versionAudit } = await request(baseURL, `/api/admin/audit-logs?siteCode=${encodeURIComponent(versionAuditCode)}&pageSize=50`, {
      jar: adminJar,
    });
    const versionActions = new Set((versionAudit.logs || []).map((item) => item.action));
    for (const action of ["deploy.create", "deploy.version.create", "version.status", "version.lock", "version.current", "version.overwrite", "version.delete"]) {
      assert(versionActions.has(action), `version audit log missing action ${action}`);
    }
    const versionRows = (versionAudit.logs || []).filter((item) => String(item.action || "").startsWith("version."));
    assert(versionRows.length >= 5, "version audit log missing version management rows");
    assert(versionRows.every((item) =>
      item.createdAt &&
      item.actorType &&
      item.actorRole &&
      item.action &&
      item.siteCode === versionAuditCode &&
      item.targetType === "version" &&
      item.targetId &&
      item.result === "success" &&
      item.detail !== undefined
    ), "version audit rows missing required audit fields");

    await request(baseURL, "/api/deploy", {
      method: "POST",
      jar: runtimeUserJar,
      body: {
        title: "运行时 QA 普通用户删除",
        description: "用于验证普通用户只能删除自己的发布",
        filename: "index.html",
        visibility: "public",
        enableCustomCode: true,
        customCode: ownerDeleteCode,
        files: [{
          path: "index.html",
          content: "<!doctype html><html><head><meta charset=\"utf-8\"><title>Owner Delete QA</title></head><body><main><h1>owner delete qa</h1></main></body></html>",
        }],
      },
    });
    await request(baseURL, `/api/admin/sites/${encodeURIComponent(ownerDeleteCode)}`, {
      method: "DELETE",
      jar: runtimeUserJar,
    });
    await request(baseURL, `/api/admin/sites/${encodeURIComponent(ownerDeleteCode)}`, {
      jar: adminJar,
      expect: 404,
    });
    const { body: ownerDeletedMarket } = await request(baseURL, `/api/deploys?q=${encodeURIComponent(ownerDeleteCode)}&pageSize=10`);
    assert(!(ownerDeletedMarket.deploys || []).some((item) => item.code === ownerDeleteCode), "owner-deleted site still appears in market list");
    const { body: ownerDeleteAudit } = await request(baseURL, `/api/admin/audit-logs?siteCode=${encodeURIComponent(ownerDeleteCode)}&action=site.delete&pageSize=10`, {
      jar: adminJar,
    });
    const ownerDeleteLog = (ownerDeleteAudit.logs || []).find((item) => item.action === "site.delete" && item.result === "success");
    assert(ownerDeleteLog?.actorId === registeredUser.userId && ownerDeleteLog?.actorRole === "user", "runtime QA missing owner site.delete audit log");
    assert(ownerDeleteLog?.targetType === "site" && ownerDeleteLog?.targetId === ownerDeleteCode && ownerDeleteLog?.siteCode === ownerDeleteCode, "owner site.delete audit log missing target fields");

    await request(baseURL, "/api/deploy", {
      method: "POST",
      jar: adminJar,
      body: {
        title: "运行时 QA 管理员删除",
        description: "用于验证管理员删除站点和审计链路",
        filename: "index.html",
        visibility: "public",
        enableCustomCode: true,
        customCode: adminDeleteCode,
        files: [{
          path: "index.html",
          content: "<!doctype html><html><head><meta charset=\"utf-8\"><title>Admin Delete QA</title></head><body><main><h1>admin delete qa</h1></main></body></html>",
        }],
      },
    });
    await request(baseURL, `/api/admin/sites/${encodeURIComponent(adminDeleteCode)}`, {
      method: "DELETE",
      jar: adminJar,
    });
    await request(baseURL, `/api/deploys/${encodeURIComponent(adminDeleteCode)}`, {
      expect: 404,
    });
    const { body: adminDeleteAudit } = await request(baseURL, `/api/admin/audit-logs?siteCode=${encodeURIComponent(adminDeleteCode)}&action=site.delete&pageSize=10`, {
      jar: adminJar,
    });
    const adminDeleteLog = (adminDeleteAudit.logs || []).find((item) => item.action === "site.delete" && item.result === "success");
    assert(adminDeleteLog?.actorId === admin.userId && adminDeleteLog?.actorRole === "admin", "runtime QA missing admin site.delete audit log");
    assert(adminDeleteLog?.targetType === "site" && adminDeleteLog?.targetId === adminDeleteCode && adminDeleteLog?.siteCode === adminDeleteCode, "admin site.delete audit log missing target fields");

    const { body: zipDeploy } = await request(baseURL, "/api/deploy", {
      method: "POST",
      headers: authHeader,
      body: {
        title: "运行时 QA ZIP 站点",
        description: "用于验证单 ZIP 发布、Bundle 详情和相对资源",
        visibility: "public",
        category: "tool",
        tags: ["qa", "zip"],
        enableCustomCode: true,
        customCode: zipCode,
        files: [{
          path: "site.zip",
          contentBase64: makeZipBase64({
            "project/dist/index.html": "<!doctype html><html><head><meta charset=\"utf-8\"><title>ZIP QA</title><link rel=\"stylesheet\" href=\"assets/app.css\"></head><body><main><h1>zip qa</h1><script src=\"scripts/app.js\"></script></main></body></html>",
            "project/dist/assets/app.css": "body{color:#075985}",
            "project/dist/scripts/app.js": "document.documentElement.dataset.zipQa='ok';",
          }),
        }],
      },
    });
    assert(zipDeploy.success && zipDeploy.code === zipCode, "zip deploy failed");
    const { body: zipAdminDetail } = await request(baseURL, `/api/admin/sites/${encodeURIComponent(zipCode)}`, {
      jar: adminJar,
    });
    assert(zipAdminDetail.bundle?.kind === "zip_site", "admin detail did not report zip_site bundle");
    assert(zipAdminDetail.bundle?.mainEntry === "index.html", "admin detail missing ZIP entry");
    assert(zipAdminDetail.bundle?.fileCount === 3, "admin detail missing ZIP file count");
    assert(Array.isArray(zipAdminDetail.files) && zipAdminDetail.files.some((file) => file.path === "assets/app.css"), "admin detail missing ZIP file tree");
    const { body: zipHTML } = await request(baseURL, `/agent/${encodeURIComponent(zipCode)}/`, { text: true });
    assert(zipHTML.includes("zip qa"), "ZIP runtime HTML did not render");
    const { body: zipCSS } = await request(baseURL, `/agent/${encodeURIComponent(zipCode)}/assets/app.css`, { text: true });
    assert(zipCSS.includes("#075985"), "ZIP relative CSS asset was not served");

    const { body: pairing } = await request(baseURL, "/api/device/pairing/start", {
      method: "POST",
      body: {
        deviceName: "运行时 QA 屏幕",
        appVersion: "runtime-qa",
        runtime: "node",
        deviceInfo: {
          platform: "runtime-qa",
          os: process.platform,
          screen: {
            width: 1920,
            height: 1080,
            orientation: "landscape",
          },
        },
      },
    });
    assert(pairing.pairingCode && pairing.pairingId && pairing.pairingSecret, "screen pairing start response missing fields");
    const { body: bindScreen } = await request(baseURL, "/api/screens/bind", {
      method: "POST",
      jar: adminJar,
      body: {
        pairingCode: pairing.pairingCode,
        name: "运行时 QA 大屏",
      },
    });
    const screenId = bindScreen.screen?.id;
    assert(bindScreen.success && screenId, "screen bind did not return screen id");
    const { body: completePairing } = await request(baseURL, "/api/device/pairing/complete", {
      method: "POST",
      body: {
        pairingId: pairing.pairingId,
        pairingSecret: pairing.pairingSecret,
      },
    });
    assert(completePairing.paired && completePairing.deviceToken, "screen device did not complete pairing");
    const deviceHeader = { Authorization: `Device ${completePairing.deviceToken}` };
    const { body: screenList } = await request(baseURL, "/api/screens", {
      jar: adminJar,
    });
    assert(screenList.screens?.some((item) => item.id === screenId && item.deviceName === "运行时 QA 屏幕"), "bound screen missing from screen list");
    await request(baseURL, `/api/screens/${encodeURIComponent(screenId)}/publish`, {
      method: "POST",
      jar: adminJar,
      body: { code: mdCode },
    });
    const { body: manifest } = await request(baseURL, "/api/device/manifest", {
      headers: deviceHeader,
    });
    assert(manifest.mode === "webapp" && manifest.siteCode === mdCode, "screen manifest did not point to published markdown site");
    assert(String(manifest.entryUrl || "").includes(`/agent/${mdCode}/`), "screen manifest entry URL missing published site path");

    const { body: shotRequest } = await request(baseURL, `/api/screens/${encodeURIComponent(screenId)}/screenshot`, {
      method: "POST",
      jar: adminJar,
    });
    const screenshotRequestId = shotRequest.screenshot?.requestId;
    assert(screenshotRequestId, "screen screenshot request did not return request id");
    const tinyPng = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII=";
    await request(baseURL, "/api/device/screenshot", {
      method: "POST",
      headers: deviceHeader,
      body: {
        requestId: screenshotRequestId,
        contentBase64: tinyPng,
        mimeType: "image/png",
        width: 1,
        height: 1,
      },
    });
    const { response: shotImage } = await request(baseURL, `/api/screens/${encodeURIComponent(screenId)}/screenshot`, {
      jar: adminJar,
      text: true,
    });
    assert(shotImage.headers.get("content-type") === "image/png", "screen screenshot image was not returned");

    const { body: commandRequest } = await request(baseURL, `/api/screens/${encodeURIComponent(screenId)}/command`, {
      method: "POST",
      jar: adminJar,
      body: { type: "refresh" },
    });
    const commandRequestId = commandRequest.command?.requestId;
    assert(commandRequest.command?.type === "refresh" && commandRequestId, "screen refresh command missing request id");
    const { body: manifestWithCommand } = await request(baseURL, "/api/device/manifest", {
      headers: deviceHeader,
    });
    assert(manifestWithCommand.command?.requestId === commandRequestId, "screen manifest missing pending refresh command");
    await request(baseURL, "/api/device/command/ack", {
      method: "POST",
      headers: deviceHeader,
      body: {
        requestId: commandRequestId,
        type: "refresh",
      },
    });
    const { body: manifestAfterCommand } = await request(baseURL, "/api/device/manifest", {
      headers: deviceHeader,
    });
    assert(!manifestAfterCommand.command, "screen command remained pending after device ack");
    await request(baseURL, `/api/screens/${encodeURIComponent(screenId)}`, {
      method: "DELETE",
      jar: adminJar,
    });
    const { body: screenAudit } = await request(baseURL, `/api/admin/audit-logs?targetType=screen&targetId=${encodeURIComponent(screenId)}&pageSize=50`, {
      jar: adminJar,
    });
    const screenActions = new Set((screenAudit.logs || []).map((item) => item.action));
    for (const action of ["screen.bind", "screen.publish", "screen.screenshot.request", "screen.command.request", "screen.unbind"]) {
      assert(screenActions.has(action), `screen audit log missing action ${action}`);
    }
    const screenAuditJSON = JSON.stringify(screenAudit.logs || []);
    assert(screenAuditJSON.includes(screenshotRequestId), "screen screenshot audit missing request id");
    assert(screenAuditJSON.includes(commandRequestId), "screen command audit missing request id");
    assert((screenAudit.logs || []).every((item) => item.createdAt && item.actorType && item.result), "screen audit rows missing required fields");
    const { body: screenAuditByDetail } = await request(baseURL, `/api/admin/audit-logs?targetType=screen&q=${encodeURIComponent(commandRequestId)}&pageSize=10`, {
      jar: adminJar,
    });
    assert((screenAuditByDetail.logs || []).some((item) => item.action === "screen.command.request" && item.targetId === screenId), "audit keyword search did not match screen command detail JSON");

    const { body: protectedDeploy } = await request(baseURL, "/api/deploy", {
      method: "POST",
      headers: authHeader,
      body: {
        title: "运行时 QA 加密页面",
        description: "用于验证访问密码与源码下载隔离",
        filename: "index.html",
        visibility: "public",
        enableCustomCode: true,
        customCode: protectedCode,
        accessPassword: "secret123",
        files: [{
          path: "index.html",
          content: "<!doctype html><html><head><meta charset=\"utf-8\"><title>Protected QA</title></head><body><main><h1>protected qa</h1></main></body></html>",
        }],
      },
    });
    assert(protectedDeploy.success && protectedDeploy.code === protectedCode, "protected deploy failed");

    const { body: protectedDetail } = await request(baseURL, `/api/deploys/${encodeURIComponent(protectedCode)}`);
    assert(protectedDetail.accessProtected === true, "protected marketplace detail is not marked encrypted");
    assert(protectedDetail.reuse?.allowDownload === false, "encrypted site must not allow source download");
    assert(protectedDetail.reuse?.allowReuse === false, "encrypted site must not allow template reuse");
    await request(baseURL, `/api/deploy/content?code=${encodeURIComponent(protectedCode)}&download=1`, {
      headers: authHeader,
      expect: 403,
    });
    await request(baseURL, `/api/deploy/content?code=${encodeURIComponent(protectedCode)}&download=1`, {
      jar: adminJar,
      expect: 403,
    });

    const { body: passwordPage } = await request(baseURL, `/agent/${encodeURIComponent(protectedCode)}/`, {
      jar: publicJar,
      expect: 401,
      text: true,
    });
    assert(passwordPage.includes("这个网页已加密"), "anonymous visitor should see password gate");
    await request(baseURL, `/api/deploys/${encodeURIComponent(protectedCode)}/access`, {
      method: "POST",
      jar: publicJar,
      body: { password: "secret123" },
    });
    const { body: unlockedPage } = await request(baseURL, `/agent/${encodeURIComponent(protectedCode)}/`, {
      jar: publicJar,
      text: true,
    });
    assert(unlockedPage.includes("protected qa"), "anonymous password access did not unlock protected page");
    await request(baseURL, `/api/deploy/content?code=${encodeURIComponent(protectedCode)}&download=1`, {
      jar: publicJar,
      expect: 403,
    });
    const { body: protectedDownloadAudit } = await request(baseURL, `/api/admin/audit-logs?siteCode=${encodeURIComponent(protectedCode)}&action=source_download&pageSize=20`, {
      jar: adminJar,
    });
    const protectedDownloadFailures = (protectedDownloadAudit.logs || []).filter((item) => item.action === "source_download" && item.result === "failed");
    assert(protectedDownloadFailures.length >= 3, "runtime QA missing encrypted source_download failure audit logs");
    assert(protectedDownloadFailures.every((item) =>
      item.targetType === "site" &&
      item.targetId === protectedCode &&
      item.detail?.download === true &&
      item.detail?.errorCode === "FORBIDDEN" &&
      item.detail?.stage === "source_download"
    ), "encrypted source_download failure audit logs are incomplete");
    const { body: protectedAppend } = await request(baseURL, "/api/deploy", {
      method: "POST",
      headers: authHeader,
      body: {
        title: "运行时 QA 加密页面 v2",
        description: "用于验证访问密码票据绑定版本",
        filename: "index.html",
        visibility: "public",
        enableCustomCode: true,
        customCode: protectedCode,
        createVersion: true,
        files: [{
          path: "index.html",
          content: "<!doctype html><html><head><meta charset=\"utf-8\"><title>Protected QA v2</title></head><body><main><h1>protected qa v2</h1></main></body></html>",
        }],
      },
    });
    assert(protectedAppend.versionNumber === 2, "protected append did not create v2");
    const { body: relockedCurrentPage } = await request(baseURL, `/agent/${encodeURIComponent(protectedCode)}/`, {
      jar: publicJar,
      expect: 401,
      text: true,
    });
    assert(relockedCurrentPage.includes("这个网页已加密"), "old access ticket should not unlock new current version");
    const { body: explicitV1Page } = await request(baseURL, `/agent/${encodeURIComponent(protectedCode)}/versions/1/`, {
      jar: publicJar,
      text: true,
    });
    assert(explicitV1Page.includes("protected qa"), "old access ticket should still unlock explicit v1 URL");
    await request(baseURL, `/api/deploys/${encodeURIComponent(protectedCode)}/access?version=2`, {
      method: "POST",
      jar: publicJar,
      body: { password: "secret123" },
    });
    const { body: unlockedV2Page } = await request(baseURL, `/agent/${encodeURIComponent(protectedCode)}/`, {
      jar: publicJar,
      text: true,
    });
    assert(unlockedV2Page.includes("protected qa v2"), "version-specific password access did not unlock current v2 page");
    const { body: protectedAccessAudit } = await request(baseURL, `/api/admin/audit-logs?siteCode=${encodeURIComponent(protectedCode)}&action=site.access_login&pageSize=20`, {
      jar: adminJar,
    });
    const accessLog = (protectedAccessAudit.logs || []).find((item) => item.action === "site.access_login");
    assert(accessLog?.result === "success", "audit log missing successful protected access login");
    assert(accessLog?.actorType === "browser" && accessLog?.actorRole === "public", "protected access audit actor should be public browser");
    assert(accessLog?.detail?.versionNumber, "protected access audit missing version number");
    assert(!JSON.stringify(accessLog.detail || {}).includes("secret123"), "protected access audit leaked password");

    const { body: reusedDeploy } = await request(baseURL, "/api/deploy", {
      method: "POST",
      headers: authHeader,
      body: {
        title: "运行时 QA 复用作品",
        description: "基于 Markdown QA 作品复用发布",
        filename: "index.html",
        visibility: "unlisted",
        enableCustomCode: true,
        customCode: reusedCode,
        templateSourceCode: mdCode,
        templateSourceVersion: 2,
        files: [{
          path: "index.html",
          content: "<!doctype html><html><head><meta charset=\"utf-8\"><title>Reuse QA</title></head><body><main><h1>reuse qa</h1></main></body></html>",
        }],
      },
    });
    assert(reusedDeploy.templateSourceCode === mdCode, "reuse deploy did not echo template source code");
    const { body: reusedAdminDetail } = await request(baseURL, `/api/admin/sites/${encodeURIComponent(reusedCode)}`, {
      jar: adminJar,
    });
    assert(reusedAdminDetail.site?.templateSourceCode === mdCode, "admin detail did not persist template source code");
    assert(reusedAdminDetail.site?.visibility === "unlisted", "reuse QA site should remain unlisted");

    const { body: audit } = await request(baseURL, `/api/admin/audit-logs?q=${encodeURIComponent(mdCode)}&pageSize=100`, {
      jar: adminJar,
    });
    const actions = new Set((audit.logs || []).map((item) => item.action));
    for (const action of ["deploy.create", "deploy.version.create", "site.pin", "site.security_mode", "site.reuse_policy"]) {
      assert(actions.has(action), `audit log missing action ${action}`);
    }
    assert((audit.logs || []).every((item) => item.createdAt && item.actorType && item.result), "audit log rows missing required fields");
    const auditUntil = new Date(Date.now() + 60_000).toISOString();
    const { body: siteAuditPage1 } = await request(baseURL, `/api/admin/audit-logs?siteCode=${encodeURIComponent(mdCode)}&pageSize=1&page=1`, {
      jar: adminJar,
    });
    const { body: siteAuditPage2 } = await request(baseURL, `/api/admin/audit-logs?siteCode=${encodeURIComponent(mdCode)}&pageSize=1&page=2`, {
      jar: adminJar,
    });
    assert(siteAuditPage1.total >= 5 && siteAuditPage1.logs?.length === 1 && siteAuditPage1.page === 1 && siteAuditPage1.pageSize === 1, "audit pagination page 1 did not return expected metadata");
    assert(siteAuditPage2.total === siteAuditPage1.total && siteAuditPage2.logs?.length === 1 && siteAuditPage2.page === 2, "audit pagination page 2 did not preserve total metadata");
    assert(siteAuditPage1.logs[0].id !== siteAuditPage2.logs[0].id, "audit pagination returned duplicate first rows");

    const { body: actionAudit } = await request(baseURL, `/api/admin/audit-logs?siteCode=${encodeURIComponent(mdCode)}&action=site.security_mode&pageSize=10`, {
      jar: adminJar,
    });
    assert((actionAudit.logs || []).length === 1 && actionAudit.logs[0].action === "site.security_mode", "audit action filter did not isolate site.security_mode");

    const { body: actorAudit } = await request(baseURL, `/api/admin/audit-logs?siteCode=${encodeURIComponent(mdCode)}&actorId=${encodeURIComponent(admin.userId)}&pageSize=50`, {
      jar: adminJar,
    });
    assert((actorAudit.logs || []).length >= 5 && (actorAudit.logs || []).every((item) => item.actorId === admin.userId), "audit actor filter did not isolate the admin user chain");

    const { body: roleAudit } = await request(baseURL, `/api/admin/audit-logs?siteCode=${encodeURIComponent(mdCode)}&actorRole=admin&pageSize=50`, {
      jar: adminJar,
    });
    assert((roleAudit.logs || []).some((item) => item.action === "site.pin") && (roleAudit.logs || []).every((item) => item.actorRole === "admin"), "audit role filter did not isolate admin actions");

    const { body: timeAudit } = await request(baseURL, `/api/admin/audit-logs?siteCode=${encodeURIComponent(mdCode)}&since=${encodeURIComponent(qaAuditSince)}&until=${encodeURIComponent(auditUntil)}&pageSize=50`, {
      jar: adminJar,
    });
    const timeActions = new Set((timeAudit.logs || []).map((item) => item.action));
    for (const action of ["deploy.create", "deploy.version.create", "site.pin", "site.security_mode", "site.reuse_policy"]) {
      assert(timeActions.has(action), `audit time filter missing action ${action}`);
    }

    const cspSample = `runtime qa csp ${suffix}`;
    await request(baseURL, "/api/security/csp-report", {
      method: "POST",
      headers: {
        "Content-Type": "application/csp-report",
        "User-Agent": "runtime-csp-report",
      },
      body: {
        "csp-report": {
          "document-uri": `${baseURL}/agent/${mdCode}/`,
          "blocked-uri": "https://evil.example.test/runtime-csp.js",
          "violated-directive": "script-src-elem",
          "effective-directive": "script-src-elem",
          "original-policy": "default-src 'self'; report-uri /api/security/csp-report",
          "source-file": `${baseURL}/agent/${mdCode}/index.html`,
          "line-number": 12,
          "column-number": 4,
          disposition: "enforce",
          "script-sample": cspSample,
        },
      },
      expect: 204,
    });
    const { body: cspAudit } = await request(baseURL, `/api/admin/audit-logs?siteCode=${encodeURIComponent(mdCode)}&action=security.csp_report&q=${encodeURIComponent(cspSample)}&pageSize=10`, {
      jar: adminJar,
    });
    const cspLog = (cspAudit.logs || []).find((item) => item.action === "security.csp_report");
    assert(cspLog?.result === "reported", "runtime QA missing CSP report audit log");
    assert(cspLog?.actorType === "browser" && cspLog?.actorRole === "public", "CSP report audit actor should be public browser");
    assert(cspLog?.siteCode === mdCode && cspLog?.targetType === "csp" && cspLog?.targetId === "script-src-elem", "CSP report audit target is incomplete");
    assert(cspLog?.userAgent === "runtime-csp-report", "CSP report audit missing browser user-agent");
    assert(cspLog?.detail?.blockedUri === "https://evil.example.test/runtime-csp.js", "CSP report audit missing blocked URI");
    assert(cspLog?.detail?.documentUri === `${baseURL}/agent/${mdCode}/`, "CSP report audit missing document URI");
    assert(cspLog?.detail?.lineNumber === 12 && cspLog?.detail?.columnNumber === 4, "CSP report audit missing source position");
    assert(cspLog?.detail?.sample === cspSample, "CSP report audit missing sample text");

    const reportingSample = `runtime reporting api csp ${suffix}`;
    await request(baseURL, "/api/security/csp-report", {
      method: "POST",
      headers: {
        "Content-Type": "application/reports+json",
        "User-Agent": "runtime-reporting-api",
      },
      body: [{
        type: "csp-violation",
        url: `${baseURL}/agent/${mdCode}/`,
        body: {
          documentURL: `${baseURL}/agent/${mdCode}/`,
          blockedURL: "https://cdn.example.test/runtime-reporting.js",
          effectiveDirective: "script-src-elem",
          violatedDirective: "script-src-elem",
          originalPolicy: "default-src 'self'; report-uri /api/security/csp-report",
          sourceFile: `${baseURL}/agent/${mdCode}/index.html`,
          lineNumber: 21,
          columnNumber: 8,
          statusCode: 200,
          referrer: `${baseURL}/market/${mdCode}`,
          disposition: "enforce",
          sample: reportingSample,
        },
      }],
      expect: 204,
    });
    const { body: reportingAudit } = await request(baseURL, `/api/admin/audit-logs?siteCode=${encodeURIComponent(mdCode)}&action=security.csp_report&q=${encodeURIComponent(reportingSample)}&pageSize=10`, {
      jar: adminJar,
    });
    const reportingLog = (reportingAudit.logs || []).find((item) => item.action === "security.csp_report");
    assert(reportingLog?.result === "reported", "runtime QA missing Reporting API CSP audit log");
    assert(reportingLog?.actorType === "browser" && reportingLog?.actorRole === "public", "Reporting API CSP audit actor should be public browser");
    assert(reportingLog?.siteCode === mdCode && reportingLog?.targetType === "csp" && reportingLog?.targetId === "script-src-elem", "Reporting API CSP audit target is incomplete");
    assert(reportingLog?.userAgent === "runtime-reporting-api", "Reporting API CSP audit missing browser user-agent");
    assert(reportingLog?.detail?.blockedUri === "https://cdn.example.test/runtime-reporting.js", "Reporting API CSP audit missing blocked URL");
    assert(reportingLog?.detail?.violatedDirective === "script-src-elem", "Reporting API CSP audit missing violated directive");
    assert(reportingLog?.detail?.effectiveDirective === "script-src-elem", "Reporting API CSP audit missing effective directive");
    assert(reportingLog?.detail?.statusCode === 200, "Reporting API CSP audit missing status code");
    assert(reportingLog?.detail?.sample === reportingSample, "Reporting API CSP audit missing sample text");

    await request(baseURL, "/api/config", {
      method: "PUT",
      jar: adminJar,
      body: { corsAllowOrigins: "https://studio.example.test" },
    });
    const apiCors = await fetch(`${baseURL}/api/config`, {
      headers: { Origin: "https://studio.example.test" },
    });
    assert(apiCors.headers.get("access-control-allow-origin") === "https://studio.example.test", "API CORS allow-list did not apply");
    const hostedCors = await fetch(`${baseURL}/agent/${encodeURIComponent(mdCode)}/`, {
      headers: { Origin: "https://studio.example.test" },
    });
    assert(!hostedCors.headers.get("access-control-allow-origin"), "hosted app content inherited API CORS allow-list");

    await request(baseURL, "/api/admin/market/categories", {
      method: "PUT",
      jar: adminJar,
      body: {
        categories: [
          { slug: "qa-tools", label: "QA 工具", note: "runtime QA category" },
          { slug: "docs", label: "文档", note: "runtime QA docs category" },
        ],
      },
    });

    const { body: openAPI } = await request(baseURL, "/openapi.json");
    assert(openAPI.paths?.["/api/admin/audit-logs"], "OpenAPI missing audit log path");
    assert(openAPI.paths?.["/api/admin/sites/{code}"], "OpenAPI missing admin site detail path");
    assert(openAPI.paths?.["/api/security/csp-report"], "OpenAPI missing CSP report path");

    const skillForm = new FormData();
    skillForm.append(
      "file",
      new Blob([Buffer.from(makeZipBase64({ "pagep/SKILL.md": "# PagePilot runtime QA skill\n" }), "base64")], {
        type: "application/zip",
      }),
      "runtime-qa-skill.zip",
    );
    await request(baseURL, "/api/admin/skill/package", {
      method: "POST",
      jar: adminJar,
      body: skillForm,
    });

    const skill = await fetch(`${baseURL}/skill/pagep.zip`);
    assert(skill.status === 200, "skill zip is not downloadable");

    const { body: createdManagedUser } = await request(baseURL, "/api/admin/users", {
      method: "POST",
      jar: adminJar,
      body: {
        username: managedUser,
        email: `${managedUser}@example.test`,
        emailVerified: true,
        password: managedUserPassword,
        isAdmin: false,
        deployLimit: 3,
      },
    });
    const managedUserId = createdManagedUser.user?.id;
    assert(managedUserId, "admin user create response missing user id");
    await request(baseURL, `/api/admin/users/${encodeURIComponent(managedUserId)}`, {
      method: "PATCH",
      jar: adminJar,
      body: {
        username: `${managedUser}_renamed`,
        deployLimit: 7,
        isActive: true,
      },
    });
    await request(baseURL, `/api/admin/users/${encodeURIComponent(managedUserId)}`, {
      method: "DELETE",
      jar: adminJar,
    });

    const { body: managementAudit } = await request(baseURL, `/api/admin/audit-logs?actorId=${encodeURIComponent(admin.userId)}&targetType=config&pageSize=50`, {
      jar: adminJar,
    });
    const managementActions = new Set((managementAudit.logs || []).map((item) => item.action));
    for (const action of ["config.update", "config.market_categories"]) {
      assert(managementActions.has(action), `management audit log missing action ${action}`);
    }
    assert((managementAudit.logs || []).every((item) =>
      item.createdAt &&
      item.actorType === "user" &&
      item.actorId === admin.userId &&
      item.actorRole === "admin" &&
      item.targetType === "config" &&
      item.result === "success" &&
      item.detail !== undefined
    ), "management config audit rows missing required fields");

    const { body: skillAudit } = await request(baseURL, "/api/admin/audit-logs?action=skill.package_upload&targetType=skill_package&targetId=hostctl-deploy.zip&pageSize=10", {
      jar: adminJar,
    });
    const skillUploadLog = (skillAudit.logs || []).find((item) => item.action === "skill.package_upload" && item.result === "success");
    assert(skillUploadLog?.targetType === "skill_package", "runtime QA missing skill.package_upload audit log");
    assert(skillUploadLog?.detail?.sha256, "skill.package_upload audit log missing package sha256");

    const { body: userAudit } = await request(baseURL, `/api/admin/audit-logs?targetType=user&targetId=${encodeURIComponent(managedUserId)}&pageSize=20`, {
      jar: adminJar,
    });
    const userActions = new Set((userAudit.logs || []).map((item) => item.action));
    for (const action of ["user.create", "user.update", "user.delete"]) {
      assert(userActions.has(action), `user management audit log missing action ${action}`);
    }
    assert((userAudit.logs || []).every((item) =>
      item.createdAt &&
      item.actorType === "user" &&
      item.actorId === admin.userId &&
      item.actorRole === "admin" &&
      item.targetType === "user" &&
      item.targetId === managedUserId &&
      item.result === "success" &&
      item.detail !== undefined
    ), "user management audit rows missing required fields");
    assert(!JSON.stringify(userAudit.logs || []).includes(managedUserPassword), "user management audit leaked plaintext password");

    await request(baseURL, `/api/tokens/${encodeURIComponent(tokenResp.id)}`, {
      method: "DELETE",
      jar: adminJar,
    });
    const { body: tokenAudit } = await request(baseURL, `/api/admin/audit-logs?targetType=token&targetId=${encodeURIComponent(tokenResp.id)}&pageSize=10`, {
      jar: adminJar,
    });
    const tokenActions = new Set((tokenAudit.logs || []).map((item) => item.action));
    for (const action of ["token.create", "token.revoke"]) {
      assert(tokenActions.has(action), `token audit log missing action ${action}`);
    }
    assert((tokenAudit.logs || []).every((item) =>
      item.createdAt &&
      item.actorType &&
      item.actorRole &&
      item.targetType === "token" &&
      item.targetId === tokenResp.id &&
      item.result === "success" &&
      item.detail !== undefined
    ), "token audit rows missing required fields");
    assert(!JSON.stringify(tokenAudit.logs || []).includes(tokenResp.token), "token audit leaked plaintext token");

    await request(baseURL, "/api/admin/logout", {
      method: "POST",
      jar: adminJar,
    });
    const { body: logoutAudit } = await request(baseURL, `/api/admin/audit-logs?actorId=${encodeURIComponent(admin.userId)}&action=auth.logout&pageSize=20`, {
      jar: auditJar,
    });
    const logoutLog = (logoutAudit.logs || []).find((item) => item.action === "auth.logout" && item.result === "success");
    assert(logoutLog?.targetId === admin.userId, "runtime QA missing auth.logout audit log");

    console.log("runtime QA passed");
    console.log(`checked: ${mdCode}, ${zipCode}, ${protectedCode}, ${reusedCode}, ${anonymousClaimCode}`);
  } finally {
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
