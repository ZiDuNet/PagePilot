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
- Always provide `--description`; keep it concise and under 240 characters.
- For a new stable project, prefer a readable `--code`.
- For an update, reuse the existing code and append a new version. If the original code or URL is unknown, ask before deploying.
- Do not append to a code unless it belongs to the current user/token/session.
- Inspect versions before switching, locking, unlocking, overwriting, or deleting versions.
- Do not overwrite or delete locked versions. Append a new version instead.
- Confirm before deleting a whole site.
- For private work, set or clear access with the `access` command. Do not expose protected content in summaries.

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
