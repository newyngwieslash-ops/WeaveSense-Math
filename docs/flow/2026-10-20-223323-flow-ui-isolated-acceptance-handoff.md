# Handoff：隔离验收环境就绪，等待登录

## Session Metadata

时间：2026-10-20 22:40 +08:00（Asia/Shanghai）。分支 `codex/align-flow-status-and-baseline-evidence`，HEAD `5ad423dbc20a17e2c48371e50c792e737ac97b7e`。接续 [状态文档交接](2026-10-20-221539-status-docs-verified-handoff.md)，完整范围见 [原交接](2026-10-20-202424-handoff.md)。

## Current State Summary

文档变更已按用户确认本地归档，提交 5ad423db，16 归档/0 活跃变更。用户完成 8088 登录，实际浏览九视图后发现部署与当前源码不一致；用户进一步明确允许新项目/端口/卷/空库初始化。现已启动 `flow-uat-20261020`，入口 http://localhost:18088/login，等待用户自行注册/登录。没有推送、发布、付费模型、真实外发、生产迁移或子代理。

## Architecture Overview

复用 Flow Compose core，公共模板生成全新随机本地凭据；未读现有 .env.flow。隔离覆盖配置和 Dockerfile 副本在 `.local/flow-uat-20261020`；平台/web 构建上下文 allowlist 防止本地参考文件进入镜像。新卷只属于新项目，web 仅绑定回环18088；模型、SMTP、采集和 Embedding 未启用。

## Important Context

现有8088起初前端缺少当前情报助手按钮、Engine文件不同；其九视图、空态、明暗及键盘观察不能算当前HEAD验收。之后原8088平台/web运行时间变化，非本轮操作，不归为本轮成果。

构建期间出现另一方未提交 `web/src/components/canvas/canvas-cloud-agent-panel.tsx` 单行修改：没有受管模型时提前抛错。已保留，未修改或暂存。新镜像可能包含此变更，因此平台内嵌SHA为5ad423db不等于前端纯HEAD。文件哈希已登记在验收记录。

## Work Completed

- 新4镜像构建退出0、core启动退出0；minio-init退出0，平台/web/PG healthy，Engine内部healthz 200。
- 平台schema67/67；Engine两个关键文件哈希和工作区一致，迁移到0018。
- PG17应用角色非超级用户/无BYPASSRLS，Intel27张表RLS/FORCE标志。信源表实际租户读隔离、跨租户更新/插入拒绝检查通过，事务回滚、探针残留0。
- 公共模板全profiles配置列举19服务；没有把配置解析算启动。

## Critical Files

- [本次证据账](2026-10-20-ui-deployment-acceptance.md)：命令、边界、哈希、U/A任务状态。
- `docs/content/docs/progress/pending-test.mdx`：新增本轮证据入口，未批量勾选历史项。
- `.local/flow-uat-20261020/compose.acceptance.yml`：本地隔离覆盖；`uat.env`为忽略文件且有秘密，不输出值。
- `.local/flow-uat-20261020/rls-check.sql`：可运行最小RLS事务检查，只用于新隔离库。

## Files Modified

本轮仅新增证据账/本交接和pending-test证据段；本地隔离文件均Git忽略。原有两个归档.comet.yaml、prd-audit-9.md、table.csv、旧handoff和参考文件均保护；画布Agent外来修改保留。未提交任何本轮文件，不擅自归档新的业务变更。

## Decisions Made

用户明确批准隔离环境，不再重问启动/新空库初始化许可。仍不授权付费模型、外发、生产迁移、push/merge/release。仅验收/记录，发现产品问题先当前版本复现，再走Comet聚焦变更及必要选择。

## Immediate Next Steps

1. 等用户回复“18088 已登录”；新库不继承8088账号。浏览器iab tab2停在新登录页，tab1保留旧MCP页。使用cua恢复文档后续操作。
2. 登录后先U01当前中文九视图、助手抽屉、窄屏/焦点/滚动。旧版390×844键盘左移焦点导致内容祖先横滚约275px；在新版复核，不依据旧部署改代码。
3. U02/U03使用可追溯测试内容，U04/U05模型部分单独审批。继续A01–A10无凭据准备；其余真实依赖和G01仍保留。

## Verification

本轮只执行Compose构建/启动/状态、HTTP健康与版本、SQL标志及信源RLS最小事务检查、真实浏览器旧部署导航。不重跑旧Go/Bun测试、不把旧94/46结果写为本轮。构建警告为大chunk/动态导入，构建退出0；docs无站点构建。首个handoff脚本因默认GBK失败，改用Python UTF8成功；临时脚手架只作模板，正式交接在此。

## Assumptions Made

新镜像来自当前工作区；已知只有画布面板外来源码增量，不声称纯HEAD。RLS最小检查不等于全业务租户验收。

## Potential Gotchas

Docker管道在普通沙箱拒绝访问，需经自动审批的require_escalated执行Docker只读/启动命令。不要输出docker inspect环境或完整展开Compose配置，包含新密钥。PowerShell rg文件通配使用-g，避免路径星号报错。Compose2.33不支持config --no-env-resolution，已改将JSON捕获到变量仅输出非敏感断言。

## Environment State

新core保持运行；原8088未由本轮启停。仅隔离环境注册开启，方便本地测试；用户自行处理密码。变量只记录CANVAS_REGISTRATION_ENABLED、CANVAS_CORS_ORIGINS、FLOW_INTERNAL_JWT_SIGNING_KEY、数据库/Redis/MinIO变量名称，秘密均不进文档。未启动宿主dev server。

## Release Gates

U01–U05当前真实业务闭环、平台RLS/全表/多实例/提交不确定/HTTP回执、A02–A15真实部署依赖/恢复/迁移/模型/MCP/OAuth/外发、G01真实远端SHA/CI/交付均未全部通过。A01只完成本地core空库启动，PG只补信源RLS最小行为；不能称发布通过。
