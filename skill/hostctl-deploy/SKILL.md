---
name: hostctl-deploy
description: Use when an agent needs to deploy, update, inspect, manage, or screen-cast PagePilot static sites with the bundled hostctl_deploy.py script.
---

# hostctl Deploy

Use the bundled script instead of hand-written API calls:

```bash
python skill/hostctl-deploy/scripts/hostctl_deploy.py doctor
```

Set the target with `--server` or `HOSTCTL_SERVER`. If unset, the script uses `http://localhost:8787`. Server addresses are not fixed; save or pass the target server explicitly when working with a deployed PagePilot instance.

## Auth

- Anonymous mode creates/reuses a local agent identity in `~/.hostctl/agent.json`, creates/reuses an anonymous session in `~/.hostctl/session.json`, and sends `X-Hostctl-Session`, `X-Hostctl-Agent-Id`, and `X-Hostctl-Agent-Label`.
- `agentId` is metadata only. Authorization comes from either `X-Hostctl-Session` for anonymous work or a Bearer Token for user-owned work.
- Web anonymous users use the browser `hostctl_anon_session` cookie. Agent anonymous users use the local `sessionId`. Both become `anon:{sessionId}` owners on the server. IP and User-Agent are display/debug metadata only.
- Every unauthenticated deploy is recorded as an anonymous session. Empty sessions that never deploy are not shown in the admin anonymous list.
- Anonymous deploys are quota-limited, but they can deploy, append owned versions, delete owned sites, and set or clear access passwords.
- When the user signs in or provides a token, claim the current anonymous session so previous anonymous deploys move under that user:

```bash
python skill/hostctl-deploy/scripts/hostctl_deploy.py claim-session
```

- Tokens are user-owned. Use `token create` for permanent tokens, or pass `--expires-at` / `--ttl-seconds` for temporary tokens.
- Screen commands require a registered user token. Anonymous sessions cannot bind screens, publish to hardware, or request screenshots.

## Rules

- Use the script for deploy, append, inspect, access, token, claim-session, and admin work.
- Never deploy secrets: `.env`, API keys, bearer tokens, private credentials, or local config files.
- Do not deploy hidden, system, dependency, cache, log, database, or build-artifact folders/files such as `.git`, `.DS_Store`, `node_modules`, `__pycache__`, `*.log`, `*.db`, `dist.zip`, or local package archives unless the user explicitly asks and it is safe.
- Always provide `--description`; keep it concise and under 240 characters.
- Always provide a human-readable Chinese `--title` when deploying or appending. Do not use filenames such as `index.html`, `demo`, or `test` as the title.
- For a new stable project, prefer a readable `--code`.
- Keep custom codes stable and route-safe: use lowercase letters, numbers, and hyphens; avoid reserved names such as `admin`, `api`, `skill`, `agent`, `agents`, `deploy`, `login`, and `register`.
- Before publishing, confirm whether the user wants a new publish or an update to an existing one. If the original code or URL is unknown, ask before deploying.
- Updating an existing publish requires the existing `code`. The user can get it from the returned `/agent/{code}/` URL, the detail page, the admin site list, or `list_sites`.
- `--update` / `append` must append a new version to that code. It must not silently create a new code when the user intended to update.
- Use `visibility=public` for marketplace entries and `visibility=unlisted` for link-only entries. For protected sites, set an access password after deploy.
- Do not append to a code unless it belongs to the current user, token, or anonymous session.
- For multi-file deploys, paths must be clean relative paths using `/`. Reject absolute paths, drive letters, backslashes, `..`, `.`, empty path segments, symlinks, or files outside the selected source directory.
- Keep the main entry as `index.html` whenever possible. If the site uses a different HTML entry, pass it explicitly with `--filename` and keep the same entry stable across appended versions.
- Before deploying a directory, make sure it contains exactly one intended HTML entry or a clear `index.html`. If several plausible HTML entries exist and the intended entry is unclear, ask the user before publishing.
- For multi-page HTML sites, preserve normal navigation: use relative links such as `href="settings.html"` or `href="./settings.html"` and do not call `preventDefault()` on those links unless the handler explicitly changes `window.location` to the same target.
- Hosted apps always keep `/agent/{code}/` as the compatible path-mode URL. Servers may also enable wildcard-domain URLs such as `https://{code}.apps.example.com/`; check `/api/config` before assuming the preferred public URL.
- In path mode, do not generate root-relative asset or page links such as `/settings.html`; use relative links like `settings.html` or `./assets/app.js`. Wildcard-domain mode supports root-relative links better, but path mode remains the default compatibility entry.
- Inspect versions before switching, locking, unlocking, overwriting, or deleting versions.
- Do not overwrite or delete locked versions. Append a new version instead.
- Confirm before deleting a whole site.
- For private work, set or clear access with the `access` command. Do not expose protected content in summaries.
- Access passwords protect browser viewing only. Anonymous visitors can enter the password; a successful check grants a signed 5-minute browser cookie, and changing the site password invalidates old cookies.
- Marketplace like ranking is still available. Admin-pinned sites appear before all normal ranking results; only admins should pin or unpin sites from the admin console/API/script/MCP.
- A registered user can bind multiple hardware screens. Use `screen list` to inspect the current user's screens, and publish only to screens owned by that user. If multiple screens are available, ask the user to choose one before publishing.
- Screen publishing sends a playback manifest to the device. The screen loads the PagePilot App URL and can later cache assets from the manifest; do not send raw HTML strings directly to hardware.
- Use `screen screenshot` to request a screenshot, `screen refresh` to refresh WebView, `screen sleep` to enter standby, `screen wake` to resume playback, and `screen shutdown` to request soft shutdown or black-screen standby.
- `screen shutdown` depends on device/OEM capabilities for real power-off. Treat it as soft standby unless the hardware explicitly supports power control.
- Time-based power scheduling is hardware-specific and not guaranteed on every device. Do not promise universal support without OEM or device-owner integration.
- After deploying or appending, verify the returned App URL and Version URL. If any URL returns 404, inspect `mainEntry`, current version, and the uploaded file list before reporting success.
- Built-in PagePilot pages such as `/deploy.html`, `/api-docs.html`, `/agents/`, and `/screens/` should be served by the hostctl server. If these return 404, ask the operator to deploy the latest server build and check reverse proxy forwarding.

## Workflows

Check the server:

```bash
python skill/hostctl-deploy/scripts/hostctl_deploy.py --server http://127.0.0.1:8787 doctor
python skill/hostctl-deploy/scripts/hostctl_deploy.py session
```

Configure app URL mode as an admin:

```bash
python skill/hostctl-deploy/scripts/hostctl_deploy.py config set-app-url --mode path
python skill/hostctl-deploy/scripts/hostctl_deploy.py config set-app-url --mode domain --domain-suffix apps.pagepilot.example.com --scheme https
python skill/hostctl-deploy/scripts/hostctl_deploy.py config set-app-url --mode domain --domain-suffix pagepilot.dell.4dbim.cc --scheme https --port 1143
```

Deploy a new site:

```bash
python skill/hostctl-deploy/scripts/hostctl_deploy.py deploy ./site \
  --code my-landing \
  --title "项目首页" \
  --visibility public \
  --description "Landing page for the project launch."
```

Append a new version:

```bash
python skill/hostctl-deploy/scripts/hostctl_deploy.py deploy ./site-v2 \
  --code my-landing \
  --update \
  --title "项目首页升级版" \
  --description "Updated layout and copy."

python skill/hostctl-deploy/scripts/hostctl_deploy.py append my-landing ./site-v2 \
  --title "项目首页升级版" \
  --description "Updated layout and copy."
```

When appending, the original visibility and access password remain unchanged. Change access separately with the `access` command.

Manage access:

```bash
python skill/hostctl-deploy/scripts/hostctl_deploy.py access my-landing --password "change-me"
python skill/hostctl-deploy/scripts/hostctl_deploy.py access my-landing --clear
```

Pin marketplace entries as an admin:

```bash
python skill/hostctl-deploy/scripts/hostctl_deploy.py admin pin-site my-landing
python skill/hostctl-deploy/scripts/hostctl_deploy.py admin pin-site my-landing --unpin
```

Create tokens:

```bash
python skill/hostctl-deploy/scripts/hostctl_deploy.py token create --label ci-bot
python skill/hostctl-deploy/scripts/hostctl_deploy.py token create --label temp-runner --ttl-seconds 86400
python skill/hostctl-deploy/scripts/hostctl_deploy.py token list
```

Claim anonymous work after a user Token is available:

```bash
python skill/hostctl-deploy/scripts/hostctl_deploy.py claim-session
```

Configure server and publish to a screen:

```bash
python skill/hostctl-deploy/scripts/hostctl_deploy.py --server https://pagepilot.example.com screen list
python skill/hostctl-deploy/scripts/hostctl_deploy.py screen bind 123456 --name "Lobby Screen"
python skill/hostctl-deploy/scripts/hostctl_deploy.py screen publish --screen screen_xxx --app my-landing
python skill/hostctl-deploy/scripts/hostctl_deploy.py screen publish --screen screen_xxx --source ./site \
  --title "大厅展示页" \
  --visibility unlisted \
  --access-password "change-me" \
  --description "Fullscreen demo for the lobby screen."
python skill/hostctl-deploy/scripts/hostctl_deploy.py screen screenshot screen_xxx --output ./screen-shot.jpg
python skill/hostctl-deploy/scripts/hostctl_deploy.py screen refresh screen_xxx
python skill/hostctl-deploy/scripts/hostctl_deploy.py screen sleep screen_xxx
python skill/hostctl-deploy/scripts/hostctl_deploy.py screen wake screen_xxx
python skill/hostctl-deploy/scripts/hostctl_deploy.py screen shutdown screen_xxx
python skill/hostctl-deploy/scripts/hostctl_deploy.py screen status screen_xxx
python skill/hostctl-deploy/scripts/hostctl_deploy.py screen unbind screen_xxx
```
