# PagePilot Codex Handoff

This document is a handoff note for continuing PagePilot development on another machine or in another Codex session.

Last updated: 2026-07-06
Last pushed commit at the time of writing: `ad49ce6`

## Current Goal

Continue the remediation described in `docs/PAGEPILOT_REMEDIATION_PLAN.md` until PagePilot reaches a release-ready state.

The work is not complete yet. The repository has made large progress, but completion still requires visual QA, runtime QA, and a final requirement-by-requirement audit.

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
- Added tests that anonymous/public requests stay out of the marketplace by default.
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
  - relative images
  - safer inline URL handling
  - escaping for active HTML/script content
- Split Markdown hosted CSP from HTML hosted CSP:
  - Markdown uses stricter no-script CSP
  - HTML remains compatible with user-generated apps that need scripts
- Added tests for Markdown rendering and Markdown CSP.
- Updated README and Docker/deploy docs for:
  - default admin account
  - storage persistence
  - OSS envs
  - email verification envs
  - Skill download paths
  - pagep/pagep-mcp usage

## Verification Already Run

These passed before commit `ad49ce6`:

```powershell
go test -count=1 ./cmd/... ./internal/... ./apps/...
npm run build --prefix frontend/user
npm run build --prefix frontend/admin
python -m py_compile skill\hostctl-deploy\scripts\hostctl_deploy.py
```

Notes:

- `go test -count=1 ./cmd/... ./internal/... ./apps/...` prints `go: warning: "./apps/..." matched no packages`; this is expected and the command still exits successfully.
- Do not use plain `go test ./...` while the local competitor checkout is inside the workspace.

## Critical Remaining Work

### 1. Runtime Smoke Test

Run the server from a clean checkout and from an upgraded old database:

```powershell
go run ./cmd/hostctl-server --dev
```

Check:

- old SQLite DB migrates without `no such column: email`
- `/`
- `/market`
- `/deploy`
- `/agents/`
- `/screens/`
- `/admin`
- `/openapi.json`
- `/skill/pagep.zip`
- `/skill/hostctl-deploy.zip`

### 2. UI/UX Review

The UI has changed a lot, but it still needs a product-quality review across all pages. The user explicitly disliked earlier card-heavy and cramped layouts.

Review with `ui-ux-pro-max` style expectations:

- homepage
- creation market list
- creation market detail
- use-template modal
- manual deploy
- agents page
- screens page
- login/register
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
- market card actions should be hover/compact and not visually noisy

### 3. Marketplace Product Logic

Still needs deeper review:

- category and tag behavior across web UI, admin, Skill, CLI, MCP, API
- whether anonymous deploys can be edited/updated only by their anonymous session
- whether claimed anonymous deploys become normal user-owned deploys correctly in UI
- whether "mine" and "favorites" counts match filters
- unauthorized favorite should show friendly UI message, not raw API text
- own-card delete and admin delete flows need runtime QA
- update existing publish should prefer selecting owned sites, not hand-entering other people's code

### 4. Markdown Advanced Rendering

Current Markdown rendering is safer and better than before, but it is not a full advanced renderer yet.

Done:

- basic headings, paragraphs, lists, tables, task lists, blockquote, code, Mermaid/math semantic containers
- strict Markdown CSP

Still incomplete:

- real syntax highlighting
- real KaTeX rendering
- real Mermaid diagram rendering
- theme switch UI
- robust Markdown parser behavior for all CommonMark edge cases

Decision needed:

- keep minimal server renderer and document it as safe preview, or
- introduce a maintained Markdown renderer pipeline with local bundled assets and stronger CSP handling.

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
go test -count=1 ./cmd/... ./internal/... ./apps/...
npm run build --prefix frontend/user
npm run build --prefix frontend/admin
python -m py_compile skill\hostctl-deploy\scripts\hostctl_deploy.py
go run ./cmd/hostctl-server --dev
```

If Skill source changes, rebuild the embedded ZIP:

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

