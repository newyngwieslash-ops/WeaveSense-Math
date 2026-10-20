# Handoff：F01/F02 本地修复通过，待 Comet 归档确认

创建时间：2026-10-20 21:16:00 +08:00（Asia/Shanghai）。接续 [调查交接](2026-10-20-204127-outbox-preflight-handoff.md)，需求和其余工作包仍引用 [完整交接](2026-10-20-202424-handoff.md)。本文件是本轮最新有效交接，旧文档不覆盖。

## Current State Summary

分支：codex/fix-command-outbox-ownership-and-errors。本地产品实现提交：0419fd7f，基线 f0231e841fd17d24c6b81125625f02f41e76be78。后续仅保存工作流元数据、报告和本交接的本地提交，以 git log 核对最终 HEAD，不把实现提交与最终交付 SHA 混为一谈。

同一活跃变更 fix-command-outbox-ownership-and-errors：5/5 tasks 完成，phase=archive、verify_result=pass、verify_mode=full、archived=false、archive_confirmation=pending、branch_status=pending。build guard 14/14、verify guard 5/5、archive 入口 5/5 通过。14 个历史归档未改，本变更尚未归档，需等待用户最终确认。没有推送、合并或发布。

## Important Context

用户明确选择方案 1：保留原有修改，在指定 codex 分支实施同一聚焦 hotfix，先补最小复现再修复验证；不启动子代理，不推送或发布。已执行，无子代理或新聊天。hotfix 使用本次默认 direct/direct/off，规模评估后在 verify 阶段设为 full，手工核对全部规格与设计，自动审查按 review_mode=off 跳过。

F01/F02 本轮已在本地 SQLite/可控 Engine 复现并修复，但不表示真实 PG、UI 或系统所有持久化边界已通过。新发现的外层 TenantMiddleware Commit/HTTP 回执关系仅静态核对、独立登记，未修改中间件、未声明其已修复。C01/C02、U/A/G 发布门槛仍按完整交接保留。

## Architecture Overview

REST proxyCommand、Agent EnqueueAndDispatch、后台 DispatchDue 共用 DispatchOne。领取条件包含 attempts 版本、可派发状态和退避到期；领取后保留本次版本，不重新读取并认领他人状态。所有回执先 CAS 再在同一事务修改预算和投影。活跃租约不发送，过期租约直接对账，迟到响应不能覆盖新回执。

已处于外层租户事务时，内层 GORM 事务只是 savepoint，DispatchOne 因此保留排队状态，由现有后台在外层提交后发送。REST 返回 202 和实际状态；Agent 工具成功表示命令受理，pending/dispatching/对账状态均不是业务完成。自动幂等键在用户未提供键时也稳定转发给 Engine。

## Critical Files

- `backend/internal/flow/command_outbox.go`：领取版本、原子回执/投影/预算、错误传播、过期对账、事务内延后派发、自动键转发与安全日志。
- `backend/internal/flow/command_outbox_test.go`：13 个顶层 TestCommandOutbox（原 3 项 + 新增 10 项），含竞争、9 种写失败、迟到响应、读取错误和资金断言。
- `backend/internal/flow/routes.go`：不再根据 ResponseStatus 掩盖派发错误；未完成派发返回实际排队状态。
- `backend/internal/flow/tools.go`：保留派发错误，排队输出保留 operation/outbox/status。
- `docs/superpowers/reports/2026-10-20-fix-command-outbox-ownership-and-errors-verify.md`：本轮 RED/GREEN、完整规格映射、执行结果及未覆盖边界。
- `docs/flow/service-contracts.md` 与 `docs/content/docs/progress/pending-test.mdx`：服务合同和真实环境验收。

## Files Modified

本轮实现提交仅包含上述 4 个 Go 文件、功能清单、待测试清单、服务合同、验证报告及本变更 OpenSpec 工件。没有引入依赖或数据库字段，无需 schema 迁移。

用户原有变更全部保留：已修改的历史 sidebar-intelligence-entry/.comet.yaml（branch_status/bound_branch），.zcodeignore、原有两组旧交接、202424 交接/提示词、prd-audit-9.md、table.csv、本地中文参考文件。没有暂存它们；不使用 reset/restore/clean 或 git add .。

session-handoff 生成的两份本轮临时脚手架已整理为 docs/flow 有效交接。第一次 apply_patch 删除自身脚手架失败，最终仅以两个明确 LiteralPath 删除本轮生成的模板，没有清理用户文件或目录。本轮最终 handoff 已运行校验器，100/100，无凭据告警。

## Decisions Made

1. 复用 attempts，未新增租约表、令牌字段或派发抽象；全仓检查确认仅本共享入口递增。
2. 回执事务失败保留 dispatching 和资金，活跃期间不再发；可写后过期转对账。最终读取失败也向上传播，不能承诺数据库不可用时已完成对账标记。
3. 外层事务内不发送，以现有后台完成提交后派发；无需更改 RLS 或引入第二队列。
4. 保持现行网络/5xx 有界退避，但必须转发稳定幂等键；本地测试证明键稳定，不冒充真实 Engine live 幂等验收。

## Verification

本轮先运行失败测试再修复：竞争调用 2 次；202/422/503 迟到回执覆盖新状态/资金；8 种初始写失败隐藏或部分提交；读取错误被吞；REST 202/Agent succeeded 掩盖错误；外层回滚前已发送；自动键为空。后补第 9 种 retry_wait 写失败检查。详细命令和首次失败输出见验证报告。

实际通过：Outbox 专项（最近一次 6.652s）、Flow 包回归（22.075s）、Go Flow 包构建、race 专项（21.374s，无 race）、openspec strict validate、git diff --check。Comet Runtime 另实际运行包构建和全 Flow 包测试，退出均为 0，后者 24.061s；不是手工 record-check 声明替代执行。

PG/RLS 选择性检查退出 0，但 TestRuntimeRLSBlocksUnfilteredHandlerQueries 明确因 FLOW_TEST_PG_RLS_DSN 未配置而 SKIP；未验证真实 PG17/RLS、跨实例、真实数据库连接中断、UI、Engine/web 构建、Docker、模型、实采、外发、备份恢复、迁移或远端。Go 测试环境为本地 SQLite 与可控 Engine。

## Immediate Next Steps

1. 等用户按 comet-archive 最终确认点选择：确认归档、调整/重新验证、暂不归档。不得因 verify_result=pass 自动归档。确认后执行 archive-confirm，再由脚本合并主规格并移动变更；只提交实际归档相关路径。
2. 用户已禁止推送/发布；归档后按技能处理分支，保留当前本地分支，不擅自 merge、push 或创建 PR。如用户后续改变分支意图，遵循其明确选择。
3. 随后按完整交接推进 C01/C02，再 U01–U05、A01–A10；优先补实际 REST/Agent 事务受理→后台→操作卡/刷新/重启，以及中间件提交失败回执的集成证据。没有凭据继续无凭据准备，不做 O 项。

## Assumptions Made

本地 hotfix 通过只针对本变更规格和可运行检查；外层中间件整体提交/回执合同、真实 PG 行为和部署恢复仍待验收。历史 185 Y 不是运行通过数，196 项矩阵不是未提供 PRD 原文的替代证明；861 项仍是跨产品历史验收登记，不能当待开发数量。

## Potential Gotchas

本机技能目录 Comet 脚本与已安装 CLI 版本不同，旧脚本不识别 check_epoch。恢复时用已安装 comet CLI；本轮兼容恢复使用其同版本 assets/skills/comet/scripts，勿用旧脚本改状态。手工 record-check 不满足新版 guard，必须 comet check run 实际执行。提交新跟踪的原忽略文档会使整树快照失效，应让 Runtime 重新执行，不跳过 guard。build 阶段设置 verify_mode=full 被拒，进入 verify 后设置成功。

Python 默认 3.9 不适合交接脚本；本轮使用 Python 3.13 -X utf8。SQLite 时间注入统一 UTC，否则带偏移的文本比较会误导测试。没有 .codegraph，不创建索引。改 Engine 前先读其 AGENTS.md。只记录变量名称，不读写真实凭据值。

## Environment State

没有启动/停止任何 dev server、Docker 或产品进程，未占用新服务端口。启动前仍需核对既有进程；Flow 默认 8088，宿主 Vite 3000。宿主后端必须显式 CANVAS_BACKEND_DATA_DIR=../.local/project-workbench-debug，禁止 backend/data。

真实 PG 后续本地配置 FLOW_TEST_PG_RLS_DSN、FLOW_TEST_PG_ADMIN_URL；其余凭据变量沿用完整交接，仅记录名称。本轮未读取真实 .env，不使用付费 API、邮件、Webhook 或生产迁移。

## Release Gates

本地 F01/F02 专项及包回归通过；真实 PG/RLS/多实例/提交不确定及 U05 链路仍未通过。C01 权威文档口径、C02 当前全产品证据、U01–U05 真实产品交互、A01–A10 部署与恢复/模型/预算/迁移副本、A11–A15 实采/MCP/外发及两租户闭环、G01 远端 SHA/CI/交付均尚未收口。归档不是发布，当前无发布通过结论。
