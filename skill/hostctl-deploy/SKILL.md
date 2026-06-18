---
name: hostctl-deploy
description: Publish, update, inspect, and manage PagePilot static sites with the bundled hostctl_deploy.py script. Use when Codex needs to deploy generated HTML/CSS/JS, multi-file static demos, reports, dashboards, landing pages, append versions to existing PagePilot projects, claim anonymous sessions, manage access passwords, inspect versions, or perform token/admin/config operations.
---

# hostctl Deploy

Use the bundled script instead of hand-written API calls:

```bash
python skill/hostctl-deploy/scripts/hostctl_deploy.py doctor
```

Set the target with `--server` or `HOSTCTL_SERVER`. If unset, the script uses `http://localhost:8787`.

## Auth

- Anonymous mode creates/reuses a local Agent identity in `~/.hostctl/agent.json`, creates/reuses an anonymous session in `~/.hostctl/session.json`, and sends `X-Hostctl-Session`, `X-Hostctl-Agent-Id`, and `X-Hostctl-Agent-Label`.
- `agentId` is metadata only. Authorization comes from either `X-Hostctl-Session` for anonymous work or a Bearer Token for user-owned work.
- Anonymous deploys are quota-limited, but they can deploy, append owned versions, delete owned sites, and set or clear access passwords.
- When the user signs in or provides a Token, claim the current anonymous session so previous anonymous deploys move under that user:

```bash
python skill/hostctl-deploy/scripts/hostctl_deploy.py claim-session
```

- Tokens are user-owned. Use `token create` for permanent tokens, or pass `--expires-at` / `--ttl-seconds` for temporary tokens.

## Rules

- Use the script for deploy, append, inspect, access, token, claim-session, and admin work.
- Never deploy secrets: `.env`, API keys, bearer tokens, private credentials, or local config files.
- Do not deploy hidden, system, dependency, cache, log, database, or build-artifact folders/files such as `.git`, `.DS_Store`, `node_modules`, `__pycache__`, `*.log`, `*.db`, `dist.zip`, or local package archives unless the user explicitly asks and it is safe.
- Always provide `--description`; keep it concise and under 240 characters.
- For a new stable project, prefer a readable `--code`.
- Keep custom codes stable and route-safe: use lowercase letters, numbers, and hyphens; avoid reserved names such as `admin`, `api`, `skill`, `agent`, `agents`, `deploy`, `login`, and `register`.
- For an update, reuse the existing code and append a new version. If the original code or URL is unknown, ask before deploying.
- Do not append to a code unless it belongs to the current user, token, or anonymous session.
- For multi-file deploys, paths must be clean relative paths using `/`. Reject absolute paths, drive letters, backslashes, `..`, `.`, empty path segments, symlinks, or files outside the selected source directory.
- Keep the main entry as `index.html` whenever possible. If the site uses a different HTML entry, pass it explicitly with `--filename` and keep the same entry stable across appended versions.
- Before deploying a directory, make sure it contains exactly one intended HTML entry or a clear `index.html`. If several plausible HTML entries exist and the intended entry is unclear, ask the user before publishing.
- For multi-page HTML sites, preserve normal navigation: use relative links such as `href="settings.html"` or `href="./settings.html"` and do not call `preventDefault()` on those links unless the handler explicitly changes `window.location` to the same target.
- Do not generate root-relative asset or page links such as `/settings.html` for hosted apps, because apps run under `/agent/{code}/`.
- Inspect versions before switching, locking, unlocking, overwriting, or deleting versions.
- Do not overwrite or delete locked versions. Append a new version instead.
- Confirm before deleting a whole site.
- For private work, set or clear access with the `access` command. Do not expose protected content in summaries.
- Access passwords protect browser viewing only. Anonymous visitors can enter the password; a successful check grants a signed 5-minute browser cookie, and changing the site password invalidates old cookies.
- Marketplace like ranking is still available. Admin-pinned sites appear before all normal ranking results; only admins should pin or unpin sites from the admin console/API/script/MCP.
- After deploying or appending, verify the returned App URL, Short URL, and Version URL. If any URL returns 404, inspect `mainEntry`, current version, and the uploaded file list before reporting success.
- Built-in PagePilot pages such as `/deploy.html`, `/api-docs.html`, and `/agents/` should be served by the hostctl server. If these return 404, ask the operator to deploy the latest server build and check reverse proxy forwarding.

## Workflows

Check the server:

```bash
python skill/hostctl-deploy/scripts/hostctl_deploy.py --server http://127.0.0.1:8787 doctor
python skill/hostctl-deploy/scripts/hostctl_deploy.py session
```

Deploy a new site:

```bash
python skill/hostctl-deploy/scripts/hostctl_deploy.py deploy ./site \
  --code my-landing \
  --title "Project Landing" \
  --description "Landing page for the project launch."
```

Append a new version:

```bash
python skill/hostctl-deploy/scripts/hostctl_deploy.py append my-landing ./site-v2 \
  --description "Updated layout and copy."
```

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
