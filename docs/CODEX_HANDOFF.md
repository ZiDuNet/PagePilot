# PagePilot Codex Handoff

This document is a handoff note for continuing PagePilot development on another machine or in another Codex session.

Last updated: 2026-07-07
Latest commit: use `git log -1 --oneline` on the active branch.

Additional local branch in progress: `codex/pagepilot-runtime-refactor`.

## Current Goal

Continue the remediation described in `docs/PAGEPILOT_REMEDIATION_PLAN.md` until PagePilot reaches a release-ready state.

The work is not complete yet. The repository has made large progress and now has reproducible runtime QA, browser visual QA, and a local legacy SQLite + hosted upgrade rehearsal for market detail, template reuse, Markdown runtime, ZIP runtime, encrypted access, `/admin?mode=register`, admin tabs, and legacy data preservation. Completion still requires a real Docker upgrade run against an old production-like database/hosted directory and a final requirement-by-requirement audit.

## Completed In The Current Round

- Promoted work onto `main` and pushed to `ZiDuNet/PagePilot`.
- Rebranded external CLI/MCP naming toward PagePilot / `pagep`.
- Kept compatibility aliases for old `hostctl` and `hostctl-deploy` paths.
- Added and rebuilt the PagePilot user SPA and admin SPA into embedded assets.
- Removed old standalone embedded HTML pages from `internal/web/user` and `internal/web/admin`.
- Added the new PagePilot logo assets to user/admin public and embedded app paths.
- Added remediation plan document: `docs/PAGEPILOT_REMEDIATION_PLAN.md`.
- Added this committed context/handoff layer for future Codex sessions.
- Implemented or wired email-related user fields:
  - `admin_users.email`
  - `admin_users.email_verified`
  - admin user create/edit/list support
  - OpenAPI schema updates
  - migration regression test for old SQLite databases without email columns
- Fixed an old-database startup failure:
  - removed `idx_admin_users_email` from initial `schema.sql`
  - create the email index after migration adds the email column
- Implemented configurable registration/email verification flow already reflected in config/docs/tests:
  - captcha
  - email verification code
  - registration creates verified email when enabled
- Implemented OSS storage adapter and storage abstraction for deployed app files.
- Added local/OSS storage tests and config tests.
- Changed default publish visibility to `unlisted`.
- Added tests that anonymous/public requests stay out of 创作市场 by default.
- Added and rebuilt Skill ZIP assets:
  - `/skill/pagep.zip` primary route
  - `/skill/hostctl-deploy.zip` compatibility alias
  - `skill/hostctl-deploy/assets/reveal.js`
  - `skill/hostctl-deploy/assets/reveal-base.css`
  - Reveal highlight/notes plugins and themes copied from the local jpage reference
- Updated `skill/hostctl-deploy/SKILL.md`:
  - PagePilot/pagep wording
  - default visibility guidance
  - category/tag/access-password questions
  - anonymous/session rules
  - Reveal.js Bundle as optional user-built static bundle
- Enhanced Markdown hosted rendering:
  - fenced code language class
  - tables
  - task lists
  - Mermaid semantic block
  - KaTeX/math semantic block
  - same-origin KaTeX/Mermaid runtime assets
  - bundled KaTeX fonts
  - relative images
  - safer inline URL handling
  - escaping for active HTML/script content
- Split Markdown hosted CSP from HTML hosted CSP:
  - Markdown uses nonce-only script CSP for platform runtime/init scripts and no `script-src 'self'`, `script-src 'unsafe-inline'` or `unsafe-eval`
  - Markdown allows only controlled runtime styles through `style-src-elem` / `style-src-attr` for KaTeX/Mermaid compatibility
  - HTML remains compatible with user-generated apps that need scripts
- Hosted HTML and Markdown include `report-uri /api/security/csp-report`; browser CSP violations are normalized into `security.csp_report` audit logs.
- Added tests for Markdown rendering and Markdown CSP.
- Updated README and Docker/deploy docs for:
  - default admin account
  - storage persistence
  - OSS envs
  - email verification envs
  - Skill download paths

## Runtime Refactor Progress On `codex/pagepilot-runtime-refactor`

- Added `internal/bundle` and wired ZIP/Bundles into deploy:
  - strips a single wrapper/root directory
  - detects HTML/Markdown entry files
  - rejects path traversal and ambiguous batch ZIPs
  - stores Bundle metadata for later UI/API use
- Added SQLite tables and store interfaces for:
  - `site_search_fts`
  - `audit_logs`
  - `render_cache`
  - `version_bundles`
- 创作市场 search now uses FTS5 with Chinese `LIKE` fallback and startup backfill for existing sites.
- Moved Markdown rendering into `internal/render` and added render-cache integration for hosted Markdown.
- `POST /api/deploy` accepts multipart uploads in addition to JSON.
- Go CLI and MCP deploy local files/directories/ZIPs via multipart, with upload filename separated from entry filename.
- Python Skill deploy/append/overwrite/screen publish now use multipart for local files/directories/ZIPs. Directory sources are zipped locally and sent as a single upload; JSON/base64 is retained only for legacy compatibility.
- Python Skill deploy/append now prints a Chinese success summary before JSON, including server-returned app URL, detail URL, version URL, template source, reuse count and the reminder not to synthesize URLs client-side.
- Updated README, Docker docs, remediation plan, and Skill docs around multipart, ZIP entry detection, Markdown cache, FTS, and non-destructive Docker upgrade.

Verification run on this branch:

```powershell
go test -count=1 ./cmd/... ./internal/...
python -m py_compile skill/hostctl-deploy/scripts/hostctl_deploy.py skill/hostctl-deploy/scripts/pagep.py
python skill/hostctl-deploy/scripts/hostctl_deploy_test.py
```

Known remaining work on this refactor:

- Admin audit-log API/UI has a productized baseline: `/api/admin/audit-logs`, OpenAPI, CLI/MCP/Skill query support, deploy failure logs, source-download success/failure logs, runtime config failure logs, and CSP violation security logs are present. The admin page now has user and site dropdown filters, action presets, actor-type filtering, result/role/object filters, time ranges, pagination, and free-text detail search. `scripts/runtime-qa.mjs` now verifies real SQLite pagination plus site/action/actor/role/time/detail keyword filters, source-download success and encrypted-denial logs, version-management audit logs, Token audit logs, anonymous-claim audit logs, traditional CSP report and Reporting API audit logs, runtime-config/category audit logs, Skill package upload audit logs, and user create/update/delete audit logs. Remaining work is broader business failure-path coverage and production-volume query QA.
- Creation-market detail and admin site detail now expose Bundle type, entry, root, searchable/copyable file tree, security mode, entry note, source-download policy and reuse parameters. File trees include path/name/SHA search, folder/file/size stats, copy path, and copy SHA actions. Admin site detail also shows a recent audit trail filtered by the current site code. New deploys and overwrites persist stable Bundle kinds: `single_html`, `markdown`, `zip_site`, `static_site`. User manual deploy and admin deploy now render structured deploy errors with stage, stable error code, server hint, local troubleshooting suggestions, and copyable diagnostics. The real `/api/deploy` path now preserves stable ZIP error codes such as `ZIP_AMBIGUOUS_ENTRY`, `ZIP_ENTRY_MISSING`, and `ZIP_UNSAFE_PATH`, with runtime QA coverage. Remaining work is production-scale visual QA and real-world ZIP/security-mode compatibility checks.
- Admin deploy upload is now aligned with manual deploy for HTML / Markdown / ZIP. A single ZIP selected in admin deploy switches to multi-file mode and is sent to server-side Bundle detection instead of being blocked by the local “must contain HTML” check.
- Markdown has a maintained rendering path with GFM, Chroma-style code highlighting, same-origin KaTeX/Mermaid runtime, bundled KaTeX fonts, nonce-protected built-in style/init script and cache. Markdown script CSP is nonce-only for platform runtime/init scripts and does not allow `script-src 'self'`, `script-src 'unsafe-inline'` or `unsafe-eval`; KaTeX/Mermaid runtime styles use controlled `style-src-elem` / `style-src-attr` allowances. Markdown HTML responses are no-store and bypass `ServeContent` conditional caching to avoid CSP nonce/body mismatches. Inline math extraction now skips code spans and normal code fences, and special fences accept info-string options such as `mermaid title=...` and `katex display`. The sanitizer now rejects entity-encoded active URLs such as `javascript&#58;...`, quoted/unquoted event handlers, and SVG `data:` image URLs while preserving relative and HTTPS image URLs. Remaining work is broader security QA and edge-case coverage.
- Admin `/admin` and `/admin/assets/*` now use a dedicated strict CSP with no `unsafe-inline` / `unsafe-eval`, `frame-ancestors 'none'`, `X-Frame-Options: DENY`, `nosniff`, and CSP report-uri.
- The front-end template reuse experience now covers the core loop: market detail, source download, source-structure summary in the reuse drawer, new-vs-update mode, Agent prompt, CLI command, and standalone MCP parameter copying. It still needs production-volume visual QA and any future batch-policy UX.
- A basic runtime smoke pass has been done with a clean temporary SQLite database, token-based public publish, `/api/deploys`, `/`, `/market`, `/deploy`, `/agents/`, `/screens/`, `/agent/{code}/`, `/admin`, `/openapi.json`, and `/skill/pagep.zip`. A focused deep QA pass also covered `/admin?mode=register`, all admin tabs after login, `/market/{code}` direct detail, template reuse modal modes, ZIP file tree display, ZIP relative assets, Markdown KaTeX/Mermaid/table/code rendering, anonymous encrypted password access, and encrypted source-download denial.
- `scripts/runtime-qa.mjs` now makes the non-browser runtime QA reproducible: it builds and starts a temporary server/database, then checks successful and failed registration, admin login, account password change, logout, Token creation/revocation, anonymous publish explicit claim, claimed-session deployment rejection, Markdown advanced rendering, real ZIP-site publish, ZIP Bundle detail, ZIP relative assets, stable ZIP deploy errors for ambiguous entry / missing entry / unsafe path, market detail, admin Bundle/file tree detail, public unencrypted source download with `source_download` success audit, encrypted-site source-download denial for publisher Token / admin Cookie / anonymous access-password Cookie with `source_download` failed audits, encrypted access, version-bound access tickets after current-version switches, version-management audit logs for lock/status/current/overwrite/delete, normal user deletion of their own site, admin site deletion, `site.delete` audit logs and post-delete invisibility, token-management audit logs without plaintext token leakage, anonymous-claim audit logs, traditional CSP report and Reporting API audit logs, runtime-config/category audit logs, Skill package upload audit logs, user create/update/delete audit logs without plaintext password leakage, template-source persistence, screen bind/publish/screenshot/command/unbind, auth/account/access/site-management/screen audit logs, audit pagination and filters, CORS separation, OpenAPI, and Skill ZIP download. Run it with `node scripts/runtime-qa.mjs` or `make runtime-qa`.
- Protected-site password verification now writes `site.access_login` audit logs for success/failure with site code, version number and failure reason, without storing plaintext passwords. Registration/login/logout now write `auth.register` / `auth.login` / `auth.logout`, and account password changes write `account.password`; these auth logs keep actor/target user metadata and never store plaintext passwords. Runtime QA checks successful and failed `auth.register`, `auth.login`, `account.password`, `auth.logout`, and successful anonymous `site.access_login`, including no plaintext-password leakage in audit details.
- `scripts/visual-qa.mjs` now makes browser visual QA reproducible: it builds and starts a temporary server/database, logs in through the browser captcha flow, seeds HTML/Markdown/encrypted/multi-file sites, more than one page of market feed items and audit activity, then checks public pages, app runtime pages, encrypted access, admin tabs, audit-log filtering/pagination UI, market load-more behavior, market Bundle detail, admin site Bundle detail, file trees, reuse parameters, the use-template modal, and encrypted-market source-download/reuse restriction hints across desktop/mobile for blank pages, horizontal overflow, request failures and console/page errors. Run it with `node scripts/visual-qa.mjs` or `make visual-qa`; it requires Playwright and falls back to system Edge/Chrome when bundled Chromium is not installed.
- `scripts/legacy-upgrade-qa.mjs` now makes local legacy upgrade QA reproducible without Docker: it seeds an old SQLite database plus hosted directory with public/encrypted sites, a legacy admin, owned/unowned tokens, anonymous session, screen binding, audit log, and files; then starts the current server, checks admin APIs, marketplace FTS, hosted app files, password access, encrypted source-download denial, screen/token/anonymous/audit APIs, and finally direct SQLite migration invariants. Run it with `node scripts/legacy-upgrade-qa.mjs` or `make legacy-upgrade-qa`.
- `scripts/docker-upgrade-qa.mjs` now provides the real Docker Compose rehearsal entrypoint: it seeds an old SQLite database plus hosted directory in a temporary bind-mounted data root, runs `docker compose up -d --build`, checks the upgraded container through HTTP, verifies Skill ZIP download, then reuses `legacy_upgrade_dbcheck.go --mode verify` against the upgraded database. Run it on a server with Docker Compose and Go via `node scripts/docker-upgrade-qa.mjs` or `make docker-upgrade-qa`; add `--keep` to preserve the temporary directory.
- Docker upgrade has not yet been executed on this Windows machine because Docker CLI is unavailable. SQLite old-schema regression, `legacy-upgrade-qa`, and the new `docker-upgrade-qa` script now cover `audit_logs` missing `actor_role/result`, old `sites.public_id`, old files, users, owned/unowned tokens, anonymous sessions, screens, access password state, FTS backfill, new tables, Skill ZIP download, and legacy data preservation once run on a Docker-capable host.
- MCP tool-list regression now covers the admin runtime tools (`set_site_reuse_policy`, `set_site_security_mode`, `get_admin_site_detail`, `query_audit_logs`), source read, and screen tools, plus local validation for invalid reuse/security mode values before any network call.
- The canonical status checklist is now `docs/CURRENT_STATUS_AND_TODO.md`.

## Verification Already Run

Fresh local verification on 2026-07-07:

```powershell
python scripts/build_skill_zip.py
npm run build --prefix frontend/user
npm run build --prefix frontend/admin
go test -count=1 ./cmd/... ./internal/...
python -m py_compile skill\hostctl-deploy\scripts\hostctl_deploy.py skill\hostctl-deploy\scripts\pagep.py
python skill\hostctl-deploy\scripts\hostctl_deploy_test.py
node --test scripts\docker-upgrade-qa.test.mjs
npm run test:preview-sandbox --prefix frontend/user
npm run test:template-reuse --prefix frontend/user
node frontend\admin\scripts\auditFilters.test.mjs
node frontend\admin\scripts\deployUpload.test.mjs
node frontend\admin\scripts\previewSandbox.test.mjs
node frontend\admin\scripts\deviceInfo.test.mjs
node scripts/runtime-qa.mjs
node scripts/visual-qa.mjs
node scripts/legacy-upgrade-qa.mjs
```

Notes:

- Do not use plain `go test ./...` while the local competitor checkout is inside the workspace.
- On this Windows machine, Playwright was installed outside the repo under `%TEMP%\pagepilot-qa-node-tools` and loaded with `NODE_PATH` for the visual QA run, so no project `package.json` was changed for that temporary test dependency.
- `node scripts/docker-upgrade-qa.mjs` is not included in the passed list above because this Windows machine does not have Docker CLI installed. Run it on a Docker-capable server before release.

## Critical Remaining Work

The consolidated checklist is maintained in `docs/CURRENT_STATUS_AND_TODO.md`. Keep that file in sync whenever a remaining item is completed.

### 1. Runtime Smoke Test

Run the server from a clean checkout and from an upgraded old database:

```powershell
go run ./cmd/hostctl-server --dev
```

Check:

- old SQLite DB migrates without `no such column: email`, `no such column: result`, or `no such column: public_id`
- `/`
- `/market`
- `/deploy`
- `/agents/`
- `/screens/`
- `/admin`
- `/openapi.json`
- `/skill/pagep.zip`
- `/skill/hostctl-deploy.zip`

Clean temporary runtime smoke, focused browser QA, and local legacy upgrade QA have passed on 2026-07-07. The focused pass covered admin register mode, admin tabs after login, market detail, template reuse, ZIP runtime, Markdown KaTeX/Mermaid, anonymous encrypted access, and encrypted source-download denial. The reproducible `scripts/runtime-qa.mjs` covers the API/runtime chain for Markdown, real ZIP-site publish, ZIP Bundle detail, ZIP relative assets, encrypted access, source-download isolation for publisher Token / admin Cookie / anonymous access-password Cookie, anonymous publish explicit claim, claimed-session deployment rejection, version-bound access tickets after current-version switches, version-management audit logs for lock/status/current/overwrite/delete, token-management audit logs without plaintext token leakage, traditional CSP report and Reporting API audit logs, runtime-config/category audit logs, Skill package upload audit logs, user create/update/delete audit logs without plaintext password leakage, template-source persistence, successful/failed registration, login, account-password, logout, access-password, screen bind/publish/screenshot/command/unbind, site-management audit logs, screen audit logs, audit pagination/filtering, CORS separation, OpenAPI and Skill ZIP. The reproducible `scripts/visual-qa.mjs` covers public pages, app runtime pages, encrypted access, admin tabs, audit-log filtering/pagination UI, market/admin Bundle detail, file trees, reuse parameters, the use-template modal, and encrypted-market source-download/reuse restriction hints across desktop/mobile, and caught the mobile overflow plus Markdown nonce/cache regressions fixed in this round. The reproducible `scripts/legacy-upgrade-qa.mjs` covers a local old SQLite + hosted upgrade rehearsal. The reproducible `scripts/docker-upgrade-qa.mjs` is ready for a Docker-capable server, but has not been executed on this Windows machine. Keep the real old-database Docker upgrade check as the blocking deployment proof until it passes on the target server.

### 2. UI/UX Review

The UI has changed a lot, but it still needs a product-quality review across all pages. The user explicitly disliked earlier card-heavy and cramped layouts.

Review with `ui-ux-pro-max` style expectations:

- homepage
- 创作市场列表
- 创作市场详情
- use-template modal, especially long CLI/Agent command wrapping and update-existing mode
- manual deploy
- agents page
- screens page
- login/register, including `/admin?mode=register`
- encrypted-page password UI
- admin overview
- admin deploy
- admin app management
- admin category management
- admin token management
- admin user management
- admin anonymous management
- admin runtime settings
- admin Skill/MCP/CLI page
- admin API docs

Things to watch:

- no horizontal scroll
- no clipped Chinese text
- no clipped `g`, descenders, or large headings
- no repeated unnecessary cards
- no old PagePilot/hostctl wording mismatch
- desktop wide layout should not feel artificially narrow
- 创作市场卡片操作应保持 hover/compact，不要视觉噪音过重

### 3. 创作市场 Product Logic

Still needs deeper review:

- category and tag behavior across web UI, admin, Skill, CLI, MCP, API
- whether anonymous deploys can be edited/updated only by their anonymous session
- whether claimed anonymous deploys become normal user-owned deploys correctly in UI
- whether "mine" and "favorites" counts match filters
- unauthorized favorite should show friendly UI message, not raw API text
- own-card delete and admin delete flows are now covered by `scripts/runtime-qa.mjs`; remaining review is UI copy/confirmation polish under real user data.
- update existing publish should prefer selecting owned sites, not hand-entering other people's code

### 4. Markdown Advanced Rendering

Markdown is now an advanced hosted entry type, but it still needs more production QA.

Done:

- GFM blocks: headings, paragraphs, lists, tables, task lists, blockquote, links, images and code fences
- Chroma-style code highlighting generated server-side
- inline `$...$`, single-line `$$E=mc^2$$` and multi-line `$$ ... $$` math blocks rendered by same-origin KaTeX runtime
- `mermaid` fences rendered by same-origin Mermaid runtime
- bundled KaTeX fonts served from `/markdown-assets/`
- `?theme=auto|light|dark`
- render cache keyed by code, version, entry, content hash, theme and renderer version
- nonce-only Markdown script CSP for platform runtime/init scripts and CSP report-uri

Still incomplete:

- broader visual QA for real Markdown reports
- more formula and Mermaid syntax regression cases
- theme switch UI
- Markdown relative attachment/download policy review
- CSP/XSS专项复查，尤其是运行时样式放行和 sanitizer 边界

### 5. OSS Real Provider Test

Code and config support exist, but live Aliyun OSS validation is still required:

- publish single HTML
- publish Markdown with relative image
- publish ZIP
- download source
- delete version
- delete site
- confirm object keys are normalized and no traversal is possible

### 6. Email Real SMTP Test

Code/tests cover the flow, but real SMTP must be checked:

- SMTP config reads correctly
- captcha -> email code -> register works
- expired/invalid/reused codes show clear Chinese errors
- admin-created users can set email and verified status
- decide whether login should require verified email in the future

### 7. Security Review

Still required before claiming release-ready:

- HTML app sandbox/CSP compatibility and risk review
- preview iframe sandbox now uses a shared no-`allow-same-origin` policy, but still needs browser visual QA
- Markdown no-script CSP review
- access password cookie behavior
- private/unlisted/public download permissions
- admin/user isolation
- anonymous session claim behavior
- ZIP traversal protection
- OSS key traversal protection
- token one-time display and revoke behavior
- whether rate limiting or anti-abuse needs more work

### 8. Docs Finalization

Docs have been updated but still need final pass:

- README screenshots could be refreshed from the latest UI.
- Docker docs should be tested on a clean server.
- API docs in admin should be compared against `/openapi.json`.
- Skill instructions should be checked with a real Agent run.
- All docs should avoid old public branding such as `htmlcode.fun`.

## Important Local Directory Rule

Do not commit the local competitor checkout:

```text
竞品/
```

It is intentionally left untracked and used only as a local reference for jpage design and mechanisms.

## Useful Next Commands

```powershell
git pull origin main
go test -count=1 ./cmd/... ./internal/...
npm run build --prefix frontend/user
npm run build --prefix frontend/admin
node scripts/runtime-qa.mjs
node scripts/visual-qa.mjs
node scripts/legacy-upgrade-qa.mjs
node scripts/docker-upgrade-qa.mjs
python -m py_compile skill\hostctl-deploy\scripts\hostctl_deploy.py skill\hostctl-deploy\scripts\pagep.py
python skill\hostctl-deploy\scripts\hostctl_deploy_test.py
go run ./cmd/hostctl-server --dev
```

If Skill source changes, rebuild the embedded ZIP:

```powershell
python scripts/build_skill_zip.py
```

`make build` / `make docker` and the Dockerfile builder stage run the same script before compiling Go binaries, so `docker compose up -d --build` should embed the current Skill ZIP even when the operator does not run Make locally.

