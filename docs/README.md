# 文档索引

这套文档按读者和任务组织。项目行为以当前代码、运行中的 `/openapi.json` 和本文档中的“当前”章节为准；带有“历史”标记的文件用于了解决策背景，不作为部署步骤。

## 按任务阅读

| 任务 | 入口 |
| --- | --- |
| 第一次启动并发布站点 | [GETTING_STARTED.md](GETTING_STARTED.md) |
| 查看所有环境变量和默认值 | [CONFIGURATION.md](CONFIGURATION.md) |
| 写 API 客户端、CLI、Skill 或 MCP 集成 | [API_INTEGRATION.md](API_INTEGRATION.md) |
| 上线、反向代理、备份、升级和排障 | [OPERATIONS.md](OPERATIONS.md) |
| 开启路径、泛域名或双模式 | [../deploy/APP_URL_MODE.md](../deploy/APP_URL_MODE.md) |
| Docker Compose 部署 | [../deploy/DOCKER.md](../deploy/DOCKER.md) |
| systemd + Caddy 部署 | [../deploy/README.md](../deploy/README.md) |
| Android 屏幕端 | [../apps/screen-app/README.md](../apps/screen-app/README.md) |
| Agent Skill 规则和命令 | [../skill/hostctl-deploy/SKILL.md](../skill/hostctl-deploy/SKILL.md) |

## 项目参考

- [CURRENT_STATUS_AND_TODO.md](CURRENT_STATUS_AND_TODO.md)：当前已落地能力、验证范围和剩余风险。
- [CODEX_HANDOFF.md](CODEX_HANDOFF.md)：历史交接记录；新任务请先看上面的当前文档。
- [PAGEPILOT_REMEDIATION_PLAN.md](PAGEPILOT_REMEDIATION_PLAN.md)：历史整改计划，保留用于追溯，不代表仍有全部待办。

## 文档维护约定

- 新增或修改 API 时，同时更新类型、OpenAPI、[API_INTEGRATION.md](API_INTEGRATION.md) 和必要的 README 示例。
- 新增环境变量时，更新 [CONFIGURATION.md](CONFIGURATION.md)、[.env.example](../.env.example) 和 Docker/systemd 示例。
- 修改 URL 生成或反向代理行为时，更新 [../deploy/APP_URL_MODE.md](../deploy/APP_URL_MODE.md)，并验证当前版本与历史版本链接。
- 命令名以 `pagep` 为主；`hostctl`、`hostctl-mcp` 仅作为兼容别名说明。
- 文档中的域名、Token、密码和服务器 IP 都是占位符，不能直接复制到生产环境。
