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
- Manual deploy at `/deploy.html`.
- Agent / Skill / MCP page at `/agents/`.
- Screen publishing page at `/screens/`.
- User/admin console at `/admin`.
- API documentation is inside the admin console at `/admin?tab=apiDocs`; machine-readable OpenAPI remains `/openapi.json`.

The public UI should feel like a polished PagePilot product, not a generic admin template. Current theme uses blue/cyan PagePilot branding, but future redesigns may change the visual system if they improve clarity and quality.

## Runtime And Storage

- Default app URL mode is path mode: `/agent/{code}/`.
- Domain and dual modes are still supported through env/config: `HOSTCTL_APP_URL_MODE`, `HOSTCTL_APP_DOMAIN_SUFFIX`, `HOSTCTL_APP_URL_SCHEME`, `HOSTCTL_APP_URL_PORT`.
- Docker default admin for an empty database is `admin / 123456`; users must change it after first login.
- Uploaded files currently persist on local filesystem:
  - Docker host: `./data/docker/hosted`
  - Container: `/var/www/hosted`
  - Dev mode: `./data/hosted`
- SQLite data persists under Docker host `./data/docker/hostctl`.
- OSS env placeholders exist, but full object-storage read/write adapter is not implemented yet.
- Email verification env placeholders exist, but full verification mail/token flow is not implemented yet.

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
go test ./cmd/... ./internal/...
npm run build --prefix frontend/admin
npm run build --prefix frontend/user
python -m py_compile skill/hostctl-deploy/scripts/hostctl_deploy.py
```

Do not use plain `go test ./...` while the local `竞品/jpage` checkout exists under this workspace; it includes nested Go files under `node_modules` and causes invalid import path failures.

## Skill Packaging

After editing `skill/hostctl-deploy`, rebuild:

```bash
python - <<'PY'
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
PY
```

## Known Follow-Ups

- Implement full SMTP email verification: user email fields, verification token table/hash, send mail, resend, login blocking policy, admin visibility.
- Implement full OSS storage adapter: write/read/delete/list/download/serve assets, migration path from local files, signed/private/public object access model.
- Improve Markdown rendering toward first-class docs: code highlight, KaTeX, Mermaid, theme switching, and safe CSP.
- Continue tightening preview isolation: independent preview endpoints, sandbox/CSP by content type, and no parent-context access from user HTML.
- Consider public share-key separation later, but not in the current product plan.
