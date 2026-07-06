# PagePilot Codex Context

This file is committed on purpose so another machine or Codex session can resume development without relying on local chat memory.

## Project Identity

- Product name: PagePilot.
- Repository: `ZiDuNet/PagePilot`.
- Current release version: `0.2.0`.
- Main working branch after this cleanup: `main`.
- Historical implementation branch: `codex/public-url-embed-settings`; it has been promoted into `main`.
- Do not commit `竞品/`; it is a local read-only competitor reference checkout.

## Product Direction

PagePilot is an Agent-first publishing platform for HTML, Markdown, ZIP, and multi-file static sites. Core product surfaces:

- Public homepage.
- Creation Market at `/market`.
- Manual deploy at `/deploy`.
- Agent / Skill / MCP page at `/agents/`.
- Screen publishing page at `/screens/`.
- User/admin console at `/admin`.
- API documentation is inside the admin console at `/admin?tab=apiDocs`; machine-readable OpenAPI remains `/openapi.json`.

The public UI should feel like a polished PagePilot product, not a generic admin template. Current theme uses blue/cyan PagePilot branding, but future redesigns may change the visual system if they improve clarity and quality.

## Runtime And Storage

- Default app URL mode is path mode: `/agent/{code}/`.
- Domain and dual modes are still supported through env/config: `HOSTCTL_APP_URL_MODE`, `HOSTCTL_APP_DOMAIN_SUFFIX`, `HOSTCTL_APP_URL_SCHEME`, `HOSTCTL_APP_URL_PORT`.
- Docker default admin for an empty database is `admin / 123456`; users must change it after first login.
- Uploaded files persist through the configured storage backend:
  - `HOSTCTL_STORAGE_BACKEND=local` uses local filesystem paths.
  - `HOSTCTL_STORAGE_BACKEND=oss` uses Aliyun OSS for publish, preview/read, source download, version overwrite cleanup, version delete, and whole-site delete.
- Local filesystem paths:
  - Docker host: `./data/docker/hosted`
  - Container: `/var/www/hosted`
  - Dev mode: `./data/hosted`
- SQLite data persists under Docker host `./data/docker/hostctl`.
- OSS envs: `HOSTCTL_OSS_ENDPOINT`, `HOSTCTL_OSS_BUCKET`, `HOSTCTL_OSS_ACCESS_KEY_ID`, `HOSTCTL_OSS_ACCESS_KEY_SECRET`, `HOSTCTL_OSS_PREFIX`, `HOSTCTL_OSS_PUBLIC_BASE_URL`.
- Email registration verification is implemented behind env/config: captcha -> email code -> register -> `email_verified=true`; admin user management exposes email and verification state.

## Important Compatibility Rules

- Keep `/skill/pagep.zip` as the primary Skill download URL.
- Keep `/skill/hostctl-deploy.zip` as a compatibility alias.
- Keep compatibility for old deploy APIs used by existing Skill/CLI/MCP flows.
- New UI/docs should use PagePilot and `pagep` naming.
- Avoid reintroducing public `/api-docs.html` navigation; docs belong in admin.
- Do not implement short-link sharing until explicitly requested. Current primary app URL is `/agent/{code}/`; wildcard app domains are reserved/supported separately.

## Build And Test Commands

Use these before committing:

```bash
go test -count=1 ./cmd/... ./internal/... ./apps/...
npm run build --prefix frontend/admin
npm run build --prefix frontend/user
python -m py_compile skill/hostctl-deploy/scripts/hostctl_deploy.py
```

Do not use plain `go test ./...` while the local `竞品/jpage` checkout exists under this workspace; it includes nested Go files under `node_modules` and causes invalid import path failures.

## Skill Packaging

After editing `skill/hostctl-deploy`, rebuild:

PowerShell-safe rebuild command:

```powershell
@'
from pathlib import Path
import zipfile
root = Path('skill/hostctl-deploy')
out = Path('internal/web/skill/hostctl-deploy.zip')
with zipfile.ZipFile(out, 'w', zipfile.ZIP_DEFLATED) as z:
    for path in sorted(root.rglob('*')):
        if not path.is_file():
            continue
        rel = path.relative_to(root)
        if '__pycache__' in rel.parts or path.suffix in {'.pyc', '.pyo'}:
            continue
        z.write(path, Path('hostctl-deploy') / rel)
'@ | python -
```

## Current Completion State

The latest pushed remediation commit before this context update was `ad49ce6`.

Implemented and verified in the current round:

- PagePilot/pagep naming alignment across CLI/MCP/docs while keeping compatibility aliases.
- Embedded user/admin SPA assets, updated logo assets, and removal of old standalone HTML pages.
- Email fields and admin user management support for `email` and `email_verified`.
- Old SQLite migration fix for `admin_users.email`: the email index is created after migrations add the column.
- Registration email-verification flow behind config/env.
- OSS storage adapter and storage abstraction for deployed files.
- Default publish visibility is `unlisted`.
- Anonymous/public visibility tests.
- Markdown hosted rendering improvements: code blocks, tables, task lists, Mermaid/math semantic blocks, relative images, safer URL handling.
- Stricter no-script CSP for hosted Markdown while keeping HTML app compatibility.
- Skill ZIP includes jpage-derived Reveal.js assets as optional user-bundled static presentation support.
- README, Docker/deploy docs, remediation plan, and Skill docs were updated.

Verification commands that passed:

```bash
go test -count=1 ./cmd/... ./internal/... ./apps/...
npm run build --prefix frontend/user
npm run build --prefix frontend/admin
python -m py_compile skill/hostctl-deploy/scripts/hostctl_deploy.py
```

## Known Follow-Ups

See `docs/CODEX_HANDOFF.md` for the detailed unfinished checklist.

High-priority remaining work:

- Runtime smoke-test a clean checkout and an upgraded old SQLite database.
- Full UI/UX review of homepage, market, deploy, screens, login/register, encrypted page, and all admin pages.
- Marketplace product-logic review for categories, tags, owned updates, anonymous claim, favorites, deletes, and permissions.
- Live-test SMTP email verification with a real provider, and decide whether login should require verified email for non-admin accounts.
- Live-test OSS with real Aliyun credentials; still consider local-to-OSS migration tooling and signed/private object access policy.
- Decide whether to keep the minimal safe Markdown renderer or add a real local Markdown pipeline for syntax highlighting, KaTeX, Mermaid, and theme switching.
- Continue tightening preview isolation: independent preview endpoints, sandbox/CSP by content type, and no parent-context access from user HTML.
- Consider public share-key separation later, but not in the current product plan.
