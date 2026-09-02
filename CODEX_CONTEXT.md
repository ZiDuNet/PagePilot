# PagePilot Codex Context

This file is committed so another machine or Codex session can resume work without relying on local chat memory. It is a maintainer note, not the end-user manual.

## Project identity

- Product: PagePilot.
- Repository: `ZiDuNet/PagePilot`.
- Current release: `0.3.1`.
- Main branch: `main`.
- Primary CLI/MCP names: `pagep` and `pagep-mcp`; old `hostctl` names remain compatibility aliases.
- Do not commit the local competitor checkout or user demo data.

## Current product surface

- User homepage: `/`.
- Manual deploy: `/deploy`.
- Creation Market: `/market`.
- Agent/Skill/MCP guide: `/agents/`.
- Screen guide: `/screens/`.
- Admin console: `/admin`.
- Admin API docs: `/admin?tab=apiDocs`.
- Machine API contract: `/openapi.json`.
- Skill download: `/skill/pagep.zip`; old `/skill/hostctl-deploy.zip` remains compatible.

PagePilot accepts single HTML, Markdown, ZIP and multi-file static sites. It manages versions, access passwords, visibility, market reuse, source permissions, audit logs, tokens, and Android screen publishing.

## Runtime facts

- Default app URL mode is `path`: `/agent/{code}/`.
- `domain` and `dual` use `HOSTCTL_APP_DOMAIN_SUFFIX`, `HOSTCTL_APP_URL_SCHEME` and `HOSTCTL_APP_URL_PORT`.
- Domain mode requires wildcard DNS, a certificate covering the main host and wildcard host, and one reverse proxy that forwards the whole site. See [deploy/APP_URL_MODE.md](deploy/APP_URL_MODE.md).
- Browser sessions use HttpOnly SameSite=Lax cookies. CLI/Skill/MCP use Bearer tokens. Devices use Device Tokens.
- New sites consume one app quota. Appending versions does not; deleting the whole site restores one slot, deleting one version does not.
- Local storage uses `/var/www/hosted` in production, `data/hosted` in dev, and `./data/docker/hosted` on the Docker host.
- SQLite metadata uses `/var/lib/hostctl/hostctl.db` in production and `./data/docker/hostctl` on the Docker host.
- OSS and email verification are implemented behind configuration but require live provider validation before release claims.
- Production requires a stable `HOSTCTL_MASTER_KEY`. Empty databases also need bootstrap admin credentials.

## Build and test

~~~bash
make build
make test
go test -count=1 ./cmd/... ./internal/...
(cd frontend/user && npm run typecheck && npm run build)
(cd frontend/admin && npm run typecheck && npm run build)
node --test frontend/user/scripts/*.test.mjs
node --test frontend/admin/scripts/*.test.mjs
python -m py_compile skill/hostctl-deploy/scripts/hostctl_deploy.py skill/hostctl-deploy/scripts/pagep.py
python skill/hostctl-deploy/scripts/hostctl_deploy_test.py
python scripts/build_skill_zip.py
node scripts/runtime-qa.mjs
node scripts/visual-qa.mjs
node scripts/legacy-upgrade-qa.mjs
node scripts/docker-upgrade-qa.mjs
~~~

The runtime, visual and legacy scripts use temporary data. Docker upgrade QA requires Docker Compose and Go and must not point at production data. If a local competitor checkout makes a broad Go pattern noisy, use the targeted command above.

## Implementation constraints

- Keep URL fields and compatibility routes stable. API changes must update Go types, OpenAPI, frontend, CLI/Skill/MCP and [docs/API_INTEGRATION.md](docs/API_INTEGRATION.md).
- Use service-returned `url`/`pathUrl`/`domainUrl`/`versionUrl`. Clients must not reconstruct public URLs.
- Keep `filename` optional; let server-side Bundle detection choose normal HTML/Markdown/ZIP entries.
- Preserve ZIP path safety and structured `errorCode`/`stage`/`hint` responses.
- Keep access-password viewing separate from source download and template reuse.
- Keep CORS limited to API/OpenAPI; iframe embedding is controlled separately by CSP frame-ancestors.
- Keep `/api/device/ws` WebSocket support and proxy Upgrade headers.
- Keep old Skill and binary aliases, but use PagePilot/pagep in new UI and docs.
- Do not add public API-doc navigation or short-link sharing without an explicit product request.
- Do not remove `demo/` or other user-created untracked files.

## Current status and remaining external checks

Implemented: multipart and ZIP Bundle publishing, Markdown rendering/CSP, FTS market search, versions and quotas, source/reuse policies, access-password tickets, audit logs, screen control, embedded frontend/Skill assets, URL variants, and migration/QA scripts. See [docs/CURRENT_STATUS_AND_TODO.md](docs/CURRENT_STATUS_AND_TODO.md).

Remaining checks are external rather than missing menus:

1. Run real DNS/TLS/reverse-proxy tests for path/domain/dual, including a non-standard public port.
2. Run real Aliyun OSS publish/read/overwrite/delete/restore tests.
3. Run real SMTP registration and failure-path tests.
4. Review production-scale visual data and complex ZIP/Markdown/security cases.
5. Run Docker old-database upgrade and rollback rehearsals on the target host.

## Documentation workflow

- End-user changes: update [README.md](README.md) and the relevant file under [docs/](docs/README.md).
- Deployment/config changes: update [docs/CONFIGURATION.md](docs/CONFIGURATION.md), [docs/OPERATIONS.md](docs/OPERATIONS.md), and [deploy/](deploy/README.md).
- Skill changes: update `skill/hostctl-deploy/SKILL.md`, run its tests, rebuild `internal/web/skill/hostctl-deploy.zip`.
- Frontend changes: build both SPAs before compiling a release binary.
- Before handoff: run `git diff --check`, link checks, focused tests, and report anything not run.
