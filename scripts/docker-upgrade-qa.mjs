import { spawnSync } from "node:child_process";
import { mkdtemp, mkdir, rm, writeFile } from "node:fs/promises";
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

function usage() {
  return `Usage: node scripts/docker-upgrade-qa.mjs [--keep]

构造一份旧 SQLite + hosted 数据目录，执行 docker compose up -d --build，
再通过真实容器 HTTP 接口验证迁移后的站点、版本、Token、匿名 session、
屏幕、访问密码、FTS、Bundle、审计和 Skill ZIP。

Options:
  --keep   失败或成功后保留临时目录，便于在服务器上排查
  --help   显示帮助`;
}

function parseArgs(argv) {
  const out = { keep: false, help: false };
  for (const arg of argv) {
    if (arg === "--keep") {
      out.keep = true;
    } else if (arg === "--help" || arg === "-h") {
      out.help = true;
    } else {
      throw new Error(`unknown argument: ${arg}\n${usage()}`);
    }
  }
  return out;
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

function run(command, args, message, options = {}) {
  const result = spawnSync(command, args, {
    cwd: options.cwd || rootDir,
    env: { ...process.env, ...(options.env || {}) },
    stdio: options.stdio || "inherit",
    encoding: "utf8",
  });
  assert(result.status === 0, `${message}\n${result.stderr || result.stdout || ""}`.trim());
  return result;
}

function requireCommand(command, args, label) {
  const result = spawnSync(command, args, {
    cwd: rootDir,
    stdio: "pipe",
    encoding: "utf8",
  });
  const detail = result.error?.message || result.stderr || result.stdout || "";
  assert(
    result.status === 0,
    `${label} 不可用，请先在服务器安装并确认命令在 PATH 中。\n${detail}`.trim(),
  );
}

function yamlString(value) {
  return JSON.stringify(String(value));
}

async function writeComposeOverride(file, dirs, port, projectName, adminPassword) {
  const content = `services:
  hostctl:
    container_name: ${projectName}
    ports:
      - "127.0.0.1:${port}:8787"
    environment:
      HOSTCTL_HTTP_ADDR: "0.0.0.0:8787"
      HOSTCTL_HOSTED_DIR: "/var/www/hosted"
      HOSTCTL_DB_PATH: "/var/lib/hostctl/hostctl.db"
      HOSTCTL_COOLDOWN_SECONDS: "0"
      HOSTCTL_ANONYMOUS_DEPLOY_LIMIT: "10"
      REQUIRE_AUTH: "true"
      HOSTCTL_ADMIN_USERNAME: "admin"
      HOSTCTL_ADMIN_PASSWORD: ${yamlString(adminPassword)}
    volumes:
      - ${yamlString(`${dirs.hostctl}:/var/lib/hostctl`)}
      - ${yamlString(`${dirs.sql}:/var/lib/hostctl/sql`)}
      - ${yamlString(`${dirs.hosted}:/var/www/hosted`)}
      - ${yamlString(`${dirs.logs}:/var/log/hostctl`)}
`;
  await writeFile(file, content, "utf8");
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

async function waitForServer(baseURL, composeFiles, projectName) {
  let lastError = "";
  for (let i = 0; i < 180; i += 1) {
    try {
      const { body } = await request(baseURL, "/api/config", { expect: 200 });
      if (body?.success) return;
    } catch (error) {
      lastError = error.message;
    }
    await new Promise((resolve) => setTimeout(resolve, 1000));
  }
  printComposeLogs(composeFiles, projectName);
  throw new Error(`container did not become ready: ${lastError}`);
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
  return body;
}

function composeArgs(composeFiles, projectName, args) {
  return [
    "compose",
    ...composeFiles.flatMap((file) => ["-f", file]),
    "-p",
    projectName,
    ...args,
  ];
}

function runCompose(composeFiles, projectName, args, message) {
  return run("docker", composeArgs(composeFiles, projectName, args), message);
}

function tryComposeDown(composeFiles, projectName) {
  const result = spawnSync("docker", composeArgs(composeFiles, projectName, ["down", "--remove-orphans"]), {
    cwd: rootDir,
    stdio: "inherit",
    encoding: "utf8",
  });
  return result.status === 0;
}

function printComposeLogs(composeFiles, projectName) {
  spawnSync("docker", composeArgs(composeFiles, projectName, ["logs", "--no-color", "hostctl"]), {
    cwd: rootDir,
    stdio: "inherit",
    encoding: "utf8",
  });
}

async function verifyHTTP(baseURL, adminPassword) {
  const adminJar = new CookieJar();
  const publicJar = new CookieJar();
  await loginAdmin(baseURL, adminJar, "admin", adminPassword);

  const { body: sites } = await request(baseURL, "/api/admin/sites", { jar: adminJar });
  assert(sites.success, "admin site list did not succeed");
  const legacyDemo = sites.sites.find((site) => site.code === "legacy-demo");
  const legacySecret = sites.sites.find((site) => site.code === "legacy-secret");
  assert(legacyDemo, "legacy-demo missing after Docker upgrade");
  assert(legacySecret, "legacy-secret missing after Docker upgrade");
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
    body: { password: "legacy-secret" },
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

  const skill = await fetch(new URL("/skill/pagep.zip", baseURL));
  assert(skill.status === 200, "Skill ZIP is not downloadable from Docker container");
  assert((skill.headers.get("content-type") || "").includes("application/zip"), "Skill ZIP content type is not application/zip");
}

async function main() {
  const args = parseArgs(process.argv.slice(2));
  if (args.help) {
    console.log(usage());
    return;
  }

  requireCommand("docker", ["compose", "version"], "docker compose");
  requireCommand("go", ["version"], "go");

  const tmp = await mkdtemp(path.join(tmpdir(), "pagepilot-docker-upgrade-qa-"));
  const port = await freePort();
  const projectName = `pagepilot-docker-upgrade-qa-${Date.now().toString(36)}`.toLowerCase();
  const baseURL = `http://127.0.0.1:${port}`;
  const adminPassword = "legacy_admin_Pass123!";
  const dirs = {
    hostctl: path.join(tmp, "data", "docker", "hostctl"),
    sql: path.join(tmp, "data", "docker", "sql"),
    hosted: path.join(tmp, "data", "docker", "hosted"),
    logs: path.join(tmp, "data", "docker", "logs"),
  };
  const composeOverride = path.join(tmp, "docker-compose.upgrade-qa.yml");
  const composeFiles = [path.join(rootDir, "docker-compose.yml"), composeOverride];

  try {
    for (const dir of Object.values(dirs)) {
      await mkdir(dir, { recursive: true });
    }

    run("go", [
      "run",
      "./scripts/legacy_upgrade_dbcheck.go",
      "--mode", "seed",
      "--db", path.join(dirs.hostctl, "hostctl.db"),
      "--hosted", dirs.hosted,
      "--admin-password", adminPassword,
      "--secret-password", "legacy-secret",
    ], "seed legacy database failed");

    await writeComposeOverride(composeOverride, dirs, port, projectName, adminPassword);

    // Runs: docker compose -f docker-compose.yml -f <override> -p <project> up -d --build
    runCompose(composeFiles, projectName, ["up", "-d", "--build"], "docker compose up -d --build failed");
    await waitForServer(baseURL, composeFiles, projectName);
    await verifyHTTP(baseURL, adminPassword);

    run("go", [
      "run",
      "./scripts/legacy_upgrade_dbcheck.go",
      "--mode", "verify",
      "--db", path.join(dirs.hostctl, "hostctl.db"),
    ], "verify upgraded database failed");

    console.log("docker upgrade QA passed");
    console.log(`checked: ${tmp}`);
  } catch (error) {
    console.error(error.stack || error.message);
    printComposeLogs(composeFiles, projectName);
    process.exitCode = 1;
  } finally {
    // Runs: docker compose -f docker-compose.yml -f <override> -p <project> down --remove-orphans
    const downOK = tryComposeDown(composeFiles, projectName);
    if (!downOK) {
      console.error("docker compose down failed; please inspect and clean up the QA project manually.");
      process.exitCode = process.exitCode || 1;
    }
    if (args.keep || process.exitCode) {
      console.error(`临时目录已保留: ${tmp}`);
    } else {
      await rm(tmp, { recursive: true, force: true });
    }
  }
}

main().catch((error) => {
  console.error(error.stack || error.message);
  process.exit(1);
});
