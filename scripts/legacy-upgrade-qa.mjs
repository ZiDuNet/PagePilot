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

async function waitForServer(baseURL, proc) {
  let lastError = "";
  for (let i = 0; i < 100; i += 1) {
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
  assert(body.userId === "user-1", "legacy admin user id changed unexpectedly");
  return body;
}

async function stopServer(server) {
  if (!server.killed && server.exitCode === null) {
    server.kill("SIGKILL");
    await Promise.race([
      once(server, "exit"),
      new Promise((resolve) => setTimeout(resolve, 3000)),
    ]);
  }
}

function runGo(args, message) {
  const result = spawnSync("go", args, {
    cwd: rootDir,
    stdio: "inherit",
  });
  assert(result.status === 0, message);
}

async function main() {
  const tmp = await mkdtemp(path.join(tmpdir(), "pagepilot-legacy-upgrade-qa-"));
  const port = await freePort();
  const baseURL = `http://127.0.0.1:${port}`;
  const exe = path.join(tmp, process.platform === "win32" ? "hostctl-server-legacy-qa.exe" : "hostctl-server-legacy-qa");
  const dbPath = path.join(tmp, "hostctl.db");
  const hostedDir = path.join(tmp, "hosted");
  const adminPassword = "legacy_admin_Pass123!";
  const secretPassword = "legacy-secret";
  let server;
  const logs = [];

  try {
    runGo([
      "run",
      "./scripts/legacy_upgrade_dbcheck.go",
      "--mode", "seed",
      "--db", dbPath,
      "--hosted", hostedDir,
      "--admin-password", adminPassword,
      "--secret-password", secretPassword,
    ], "seed legacy database failed");

    runGo(["build", "-o", exe, "./cmd/hostctl-server"], "go build ./cmd/hostctl-server failed");

    server = spawn(exe, [], {
      cwd: rootDir,
      env: {
        ...process.env,
        HOSTCTL_HTTP_ADDR: `127.0.0.1:${port}`,
        HOSTCTL_HOSTED_DIR: hostedDir,
        HOSTCTL_DB_PATH: dbPath,
        HOSTCTL_DEV: "1",
        HOSTCTL_COOLDOWN_SECONDS: "0",
        HOSTCTL_ANONYMOUS_DEPLOY_LIMIT: "10",
      },
      stdio: ["ignore", "pipe", "pipe"],
      windowsHide: true,
    });
    server.stdout.on("data", (data) => logs.push(String(data)));
    server.stderr.on("data", (data) => logs.push(String(data)));

    await waitForServer(baseURL, server);
    const adminJar = new CookieJar();
    const publicJar = new CookieJar();
    await loginAdmin(baseURL, adminJar, "admin", adminPassword);

    const { body: sites } = await request(baseURL, "/api/admin/sites", { jar: adminJar });
    assert(sites.success, "admin site list did not succeed");
    const legacyDemo = sites.sites.find((site) => site.code === "legacy-demo");
    const legacySecret = sites.sites.find((site) => site.code === "legacy-secret");
    assert(legacyDemo, "legacy-demo missing after upgrade");
    assert(legacySecret, "legacy-secret missing after upgrade");
    assert(legacyDemo.currentVersion === 1 && legacyDemo.visibility === "public", "legacy-demo metadata changed");
    assert(legacySecret.accessProtected === true, "legacy-secret lost access password state");

    const { body: detail } = await request(baseURL, "/api/admin/sites/legacy-demo", { jar: adminJar });
    assert(detail.success, "admin site detail did not succeed");
    assert(detail.bundle?.mainEntry === "index.html", "legacy-demo bundle entry was not inferred");
    assert(detail.bundle?.fileCount === 2, "legacy-demo file count was not preserved");
    assert(detail.files?.some((file) => file.path === "assets/app.js"), "legacy-demo file tree lost nested asset");
    assert(detail.reuse?.allowDownload === true, "public unencrypted legacy site should remain downloadable");

    const { body: market } = await request(baseURL, "/api/deploys?q=Legacy&pageSize=20");
    assert((market.deploys || []).some((site) => site.code === "legacy-demo"), "FTS-backed marketplace search missed legacy-demo");

    const { body: demoPage } = await request(baseURL, "/agent/legacy-demo/", { text: true });
    assert(demoPage.includes("legacy demo ok"), "legacy-demo hosted HTML did not load");
    const { body: demoAsset } = await request(baseURL, "/agent/legacy-demo/assets/app.js", { text: true });
    assert(demoAsset.includes("legacyDemo"), "legacy-demo nested hosted asset did not load");

    const { body: lockedPage } = await request(baseURL, "/agent/legacy-secret/", {
      jar: publicJar,
      expect: 401,
      text: true,
    });
    assert(lockedPage.includes("已加密") || lockedPage.includes("访问密码"), "legacy-secret did not show password gate");
    await request(baseURL, "/api/deploys/legacy-secret/access", {
      method: "POST",
      jar: publicJar,
      body: { password: secretPassword },
    });
    const { body: unlockedPage } = await request(baseURL, "/agent/legacy-secret/", {
      jar: publicJar,
      text: true,
    });
    assert(unlockedPage.includes("legacy secret ok"), "legacy-secret password access did not unlock page");
    await request(baseURL, "/api/deploy/content?code=legacy-secret&download=1", {
      jar: publicJar,
      expect: 403,
    });

    const { body: screens } = await request(baseURL, "/api/screens", { jar: adminJar });
    assert(screens.screens?.some((screen) => screen.id === "screen-legacy" && screen.currentSiteCode === "legacy-demo"), "legacy screen binding missing");

    const { body: audit } = await request(baseURL, "/api/admin/audit-logs?siteCode=legacy-demo&pageSize=50", { jar: adminJar });
    assert((audit.logs || []).some((log) => log.action === "site.update" && log.result === "success"), "legacy audit log missing or result not backfilled");

    const { body: tokens } = await request(baseURL, "/api/tokens", { jar: adminJar });
    assert(tokens.tokens?.some((token) => token.id === "owned-token" && token.ownerUserId === "user-1"), "owned legacy token missing");
    assert(!tokens.tokens?.some((token) => token.id === "legacy-system-token"), "unowned legacy token should be removed");

    const { body: anonymous } = await request(baseURL, "/api/admin/anonymous-sessions", { jar: adminJar });
    assert(anonymous.sessions?.some((session) => session.id === "anon-legacy" && session.deployCount === 5), "legacy anonymous session missing");

    await stopServer(server);
    runGo([
      "run",
      "./scripts/legacy_upgrade_dbcheck.go",
      "--mode", "verify",
      "--db", dbPath,
    ], "verify upgraded database failed");

    console.log("legacy upgrade QA passed");
    console.log(`checked: ${dbPath}`);
  } finally {
    if (server) {
      await stopServer(server);
      if (server.exitCode && server.exitCode !== 0 && logs.length) {
        console.error(logs.join(""));
      }
    }
    await rm(tmp, { recursive: true, force: true });
  }
}

main().catch((error) => {
  console.error(error.stack || error.message);
  process.exit(1);
});
