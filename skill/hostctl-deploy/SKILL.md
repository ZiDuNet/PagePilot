---
name: hostctl-deploy
description: Publish, update, inspect, and manage PagePilot static sites with the bundled hostctl_deploy.py script. Use when Codex needs to deploy generated HTML/CSS/JS, multi-file static demos, reports, dashboards, landing pages, append versions to existing PagePilot projects, bind an Agent with a user binding code, manage access passwords, inspect versions, or perform token/admin/config operations.
---

# hostctl Deploy

Use the bundled script instead of hand-written API calls:

```bash
python skill/hostctl-deploy/scripts/hostctl_deploy.py doctor
```

Set the target with `--server` or `HOSTCTL_SERVER`. If unset, the script uses `http://localhost:8787`.

## Auth

- Anonymous mode creates/reuses a local Agent identity in `~/.hostctl/agent.json`, creates/reuses an anonymous session in `~/.hostctl/session.json`, and sends `X-Hostctl-Session`, `X-Hostctl-Agent-Id`, and `X-Hostctl-Agent-Label`.
- `agentId` is a local random UUID, not a hardware serial number. Keep using it across anonymous deploys and later binding so the server can recognize the same Agent without collecting hardware identifiers.
- Anonymous deploys are quota-limited and cannot create password-protected sites.
- When quota is exhausted, ask the user to sign in to PagePilot, create an Agent binding code, then run:

```bash
python skill/hostctl-deploy/scripts/hostctl_deploy.py bind <binding-code> --agent-label <agent-name>
```

Binding sends the same `agentId` and stores a user-owned token plus Agent identity in `~/.hostctl/config.json`. User-bound Agents can deploy under that user's quota and can only update sites owned by that user.

## Rules

- Use the script for deploy, append, inspect, access, token, and admin work.
- Never deploy secrets: `.env`, API keys, bearer tokens, private credentials, or local config files.
- Do not deploy hidden, system, dependency, cache, log, database, or build-artifact folders/files such as `.git`, `.DS_Store`, `node_modules`, `__pycache__`, `*.log`, `*.db`, `dist.zip`, or local package archives unless the user explicitly asks and it is safe.
- Always provide `--description`; keep it concise and under 240 characters.
- For a new stable project, prefer a readable `--code`.
- Keep custom codes stable and route-safe: use lowercase letters, numbers, and hyphens; avoid reserved names such as `admin`, `api`, `skill`, `agent`, `agents`, `deploy`, `login`, and `register`.
- For an update, reuse the existing code and append a new version. If the original code or URL is unknown, ask before deploying.
- Do not append to a code unless it belongs to the current user/token/session.
- For multi-file deploys, paths must be clean relative paths using `/`. Reject absolute paths, drive letters, backslashes, `..`, `.`, empty path segments, symlinks, or files outside the selected source directory.
- Keep the main entry as `index.html` whenever possible. If the site uses a different HTML entry, pass it explicitly with `--filename` and keep the same entry stable across appended versions.
- Before deploying a directory, make sure it contains exactly one intended HTML entry or a clear `index.html`. If several plausible HTML entries exist and the intended entry is unclear, ask the user before publishing.
- For multi-page HTML sites, preserve normal navigation: use relative links such as `href="settings.html"` or `href="./settings.html"` and do not call `preventDefault()` on those links unless the handler explicitly changes `window.location` to the same target.
- Do not generate root-relative asset or page links such as `/settings.html` for hosted apps, because apps run under `/agent/{code}/`.
- Inspect versions before switching, locking, unlocking, overwriting, or deleting versions.
- Do not overwrite or delete locked versions. Append a new version instead.
- Confirm before deleting a whole site.
- For private work, set or clear access with the `access` command. Do not expose protected content in summaries.
- After deploying or appending, verify the returned App URL, Short URL, and Version URL. If any URL returns 404, inspect `mainEntry`, current version, and the uploaded file list before reporting success.

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

Inspect and manage versions:

```bash
python skill/hostctl-deploy/scripts/hostctl_deploy.py versions my-landing
python skill/hostctl-deploy/scripts/hostctl_deploy.py current my-landing 2
python skill/hostctl-deploy/scripts/hostctl_deploy.py lock my-landing 2
python skill/hostctl-deploy/scripts/hostctl_deploy.py lock my-landing 2 --unlock
```

Set or clear a visit password:

```bash
python skill/hostctl-deploy/scripts/hostctl_deploy.py access my-landing --password "shared-password"
python skill/hostctl-deploy/scripts/hostctl_deploy.py access my-landing --clear
```

Download or inspect content:

```bash
python skill/hostctl-deploy/scripts/hostctl_deploy.py get my-landing
python skill/hostctl-deploy/scripts/hostctl_deploy.py get my-landing --download --output site.zip
```

Privileged operations require an admin session or admin token in production:

```bash
python skill/hostctl-deploy/scripts/hostctl_deploy.py token create --label ci-bot
python skill/hostctl-deploy/scripts/hostctl_deploy.py token list
python skill/hostctl-deploy/scripts/hostctl_deploy.py admin sites
python skill/hostctl-deploy/scripts/hostctl_deploy.py config get
```

## Report Back

After a successful deploy, keep the answer short and include:

- App URL: `detailUrl` or `/agent/{code}`
- Short URL: `url`
- Version URL: `versionUrl`
- Code and version number

If a command fails, report `errorCode`, `detail`, `hint`, `requestId`, and `retryAfterSeconds` when present.
