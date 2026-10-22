# Flow 待验收：真实环境、依赖与发布门槛

2026-10-22 19:44补记：用户先明确接受JSONL变更没有独立Verifier的结果，随后另行确认本地归档；Native已归档到docs/superpowers/comet/archive/2026-10-22-fix-canvas-jsonl-import，6/6通过、status=done、archived=true、当前分支kept，无推送/合并/发布。全Web旧失败、完整U03及发布门槛保持。下方“等待限制接受”是此前阶段记录。

2026-10-22 JSONL修复增量：用户确认Shape后实际RED→GREEN；共享数据源入口最小修复，专项11项/58断言、Runtime构建/lint及18088真实合法/坏行文件、下游、刷新与PG核对通过。原五张画布/10文档/1报告指纹与非web容器保持，模型/预算0，新增1张专项图保留。全Web40失败/2错误与撤回本补丁对照的旧失败一致；本变更无独立Verifier，Native六项仍pending，等待本轮限制接受，尚未归档或提交。详见[修复证据](../superpowers/reports/2026-10-22-fix-canvas-jsonl-import-verify.md)。下方首次失败保留历史；TSV、手工连线及完整U03不据此通过。

2026-10-22 修复增量：Native `fix-cross-tab-session-cache-isolation`已实现共享身份失效、scope固定和迟到响应保护；正式专项45项/221断言、构建与lint通过，18088双标签/503阻断及恢复/同账号缓存回退已实测。真实503发现的portal逃逸已修并重验。用户先接受没有独立Verifier的结果、后另行明确确认本地归档；archive退出0、archived=true、status=done、当前分支保留。没有推送/合并/发布，也不代表独立语义审查完成；真正离线、冷启动及其余U/A/G门槛保持。见[修复证据](../superpowers/reports/2026-10-22-cross-tab-session-cache-isolation-verify.md)。下方首次失败保留历史。

本地实现状态见[实现状态](../implementation-status.md)，本轮当前基线的实际检查见[证据账](2026-10-20-current-baseline-evidence.md)。本文件区分“已有实现待验收”和“未开发”，不因 fixture、归档或测试进程退出 0 自动解除真实环境门槛。下方按 Round 分组保留历史登记时间，其“CI 已跑”是历史引用。

当前仍需：真实 PG17 非超级用户 RLS、多实例/租约/提交不确定、实际 REST/Agent 受理→后台→操作卡/重启链路、真实 UI、整套 Compose profiles（含 subtitles/asr/backup）、私有存储/媒体/raw-response、旧库与 PG16→17 隔离副本迁移、备份恢复、监控告警、实际价目表/预算。外层租户中间件 Commit 与 HTTP 回执的关系仍待独立复现，不能因 Outbox 共享入口修复而声称全部解决。

真实模型/Embedding、SMTP/Webhook、生产迁移、付费来源及最终交付需要各自授权。缺少凭据时只记录变量名称和本地配置步骤，继续无凭据工作，不在文档填写值。A20 离线回退不证明远端 SHA/CI/部署一致，G01 继续待核实。

Live, real-account acceptance for the connectors below is **deferred to the very last step** of delivery. Until the user provides these credentials, A19 (and the live part of A14) stays **pending-credentials**. All current connector tests run on fixtures (`mode=fixture`), and no RSS test is counted as Telegram/WeChat acceptance.

## Telegram
- `TELEGRAM_API_ID` and `TELEGRAM_API_HASH` from https://my.telegram.org
- A logged-in Telethon session: either a phone number + login code (run `services/loom-engine/workers/telethon/login.py`), or an existing session string (`TELEGRAM_SESSION`) or session file path (`TELEGRAM_SESSION_PATH`)
- A deployed Telethon worker, with its URL set as `TELEGRAM_WORKER_URL` for Loom Engine
- One public channel handle to sample

## WeChat Official Accounts
- A running WeaveSense auth gateway with a logged-in WeChat session: `WECHAT_GATEWAY_URL` and `WECHAT_GATEWAY_TOKEN`
- One official-account feed ID (the gateway `source_id`)
- Optional backup provider: `DAJIALA_KEY` (plus `DAJIALA_VERIFYCODE` if that account requires it)

## MCP
- One real remote MCP endpoint (HTTPS, streamable HTTP) and its token. Either save the token as a tenant secret (`POST /api/mcp/secrets`, needs `FLOW_SECRETS_KEK`) and reference it as `secret:<id>`, or provide it as `FLOW_MCP_CRED_<NAME>` and reference it as `env:FLOW_MCP_CRED_<NAME>`

## Procedure (when credentials arrive)
1. Configure the env vars above and create one source/connection per connector.
2. Run one collection (or one MCP tool call) with a small budget.
3. Record the `flow_connector_samples` row (`mode=live`) or the `flow_mcp_usage` row, with time (Asia/Shanghai), environment, and receipts, in `delivery-report.md`.

## Added by the gap-closing round (2026-10-14)
These need a real environment. None of them has been run yet.
- `docker compose -f docker-compose.flow.yml --profile core --profile wechat --profile extract --profile research --profile monitoring config`, then boot the stack. Check that Prometheus shows both targets up.
- Upgrade an existing PG16 volume to PG17 with backup.sh and restore.sh (see release-deploy.md).
- Email notifications against a real SMTP server. Webhook notifications against a real receiver; verify the HMAC signature and `X-Flow-Delivery-Id`.
- Legacy Loom import: dry run against a copy of the real legacy database, then `--apply`; spot-check the old_id→new_id map.
- Telegram media key migration (`media-keys.ts plan` / `apply`) on real stored objects, before deleting the old keys.
- OCR on real Telegram images (Chinese text) through `--profile extract`.

## Added by round 5 (2026-10-14)
- Generate a production `FLOW_SECRETS_KEK` (`openssl rand -base64 32`), store it outside the database, and test secret rotation against a real MCP server.
- `docker compose config` now passes for every profile locally; booting the full stack has still not been run.

## Added in round 7 (2026-10-15)
- ASR with a real model: build `workers/asr` with `requirements-model.txt` (faster-whisper) and transcribe a real audio or video file under compose profile `asr`. CI only covers the stub transcriber.
- A18 object restore runs in CI on MinIO, but has not been run against the production object store.

## Added in round 8 (2026-10-15)
- Boot `--profile backup` on the real stack, confirm `/backups/last-success` updates daily and retention prunes, and do one restore drill from a daily dump. CI runs `daily-test.sh` (PG17) only.
- Wire Alertmanager routing for the new §14 alerts (deployment-specific) and confirm they fire on the real stack.

## Added in round 9 (2026-10-15)
- Media download against a real MinIO bucket (`MINIO_ACCESS_KEY`/`MINIO_SECRET_KEY` on `loom-engine`): download a stored Telegram attachment from the content library and confirm another workspace gets 404. Tests use a fake object store and check the SigV4 URL shape only.
- Monitor cost estimate with a real price book: set `FLOW_PRICE_BOOK` (`paid_source_calls`) and `FLOW_PAID_SOURCE_KINDS`, then check the approval card numbers.
- Embedding 与混合检索已有实现：`none` / `hash` / `openai` 提供者及可选 FTS + 余弦检索；真实提供者仍待配置 `EMBED_PROVIDER`、`EMBED_BASE_URL`、`EMBED_API_KEY`、`EMBED_MODEL`、`EMBED_DIMS`、`VECTOR_SEARCH_ENABLED` 并经独立授权验收。不把本地 hash 或 fake HTTP 当作真实模型通过。

## 2026-10-21 邀请RLS部署闭环

- [x] 18088已部署commit7a009195，31项真实登录HTTP检查通过：接受、单次使用、viewer只读和owner撤销后原会话立即拒绝访问。PG持久结果一致；原组织/邀请/画布和实采报告文档保持，RLS开启，无模型/预算新增。详见[本轮证据](2026-10-21-invite-rls-deployment-acceptance.md)。
- [ ] 浏览器点击旅程、完整角色矩阵/同租户多workspace、缓存竞态、恢复迁移及其余U/A门槛仍待；不称全栈正式发布构建。

## Known simplifications (local development)
历史四项简化（导出格式、原始响应存储、Embedding、25 MiB 媒体代理）已有本地增量，当前实现与保留限制见 [known-limitations](known-limitations.md)。媒体默认二进制代理支持流式分支上限 512 MiB，另有短时预签名；Engine 当前仍完整读取 Buffer 并校验 hash，不是全链路恒定内存，真实 MinIO 与大文件完整性仍待验收。
