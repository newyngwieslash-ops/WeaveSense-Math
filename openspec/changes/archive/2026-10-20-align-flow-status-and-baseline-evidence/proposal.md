# Flow 当前状态与基线验证证据对齐

## Why

C01/C02 要求权威状态能区分本地实现、历史报告、本轮自动检查、真实环境和远端交付。当前路线图仍以 P0/P1 为现状，待验收清单仍称 Embedding 未实现，功能清单仍称画布未开放 MCP；这些与当前源码、归档和本轮执行结果不符。

## What Changes

- 以本地 9fbcf028e9661883d94c51ee538bc54d126c0357 为本轮证据基线，15 个历史归档；本变更单独管理，不沿用已归档 hotfix 的隔离选择。
- 为 implementation-status、roadmap、delivery-report 增加中文当前状态入口，明确 P0–P5 本地实现与真实发布门槛的区别；历史章节保留原日期和 SHA，不把旧验证改成新结果。
- 修正 pending-acceptance 的 Embedding 和媒体下载口径、features 的画布 MCP 权限边界、known-limitations 的媒体内存/上限说明。实际路由为 POST /api/intelligence/canvas-handoff；旧验证报告仅追加更正说明，不伪造当时运行结果。
- 不修改或提交用户原有未跟踪 prd-audit-9.md。逐行复算得到 177 Y / 8 D / 7 E / 4 O = 196，与原表头 176/9/7/4 不同；在权威状态文档记录此差异。185 Y / 1 D / 7 E / 3 O 只作历史实现映射，不作当前测试通过率。Flow PRD v2 完整原文尚缺，旧 Loom /prd 不是替代来源。
- 增加独立的当前基线证据账，记录实际命令、环境、退出码、跳过和限制；pending-test 只登记未完成验收，不冒充浏览器验收。A20 测试远端查询失败后回退本地，测试通过不能证明远端 SHA 一致。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

无。本轮仅文档和证据整理，不改变产品合同，设置 skip_specs: true，不为校验虚构 delta requirement。

## Impact

修改 8 份现有文档：docs/implementation-status.md、docs/flow/roadmap.md、docs/flow/delivery-report.md、docs/flow/pending-acceptance.md、docs/flow/known-limitations.md、docs/content/docs/overview/features.mdx、docs/content/docs/progress/pending-test.mdx，以及历史 intelligence-canvas-handoff-and-panels 验证报告的更正说明；新增一份基线证据账。超过 tweak 的 6 文件提示阈值，但都属同一文档口径收口，无产品源码、依赖、数据结构、迁移或公开 API 改动。用户已确认继续同一 tweak，并使用新分支 codex/align-flow-status-and-baseline-evidence。

## 已完成的独立预检

在未改产品代码的 9fbcf028 上执行检查，已整理为 docs/flow/2026-10-20-current-baseline-evidence.md：Flow 包通过；Canvas/Agent 专项通过（capability 包没有匹配用例，app 包运行匹配用例）；Bun 46 pass / 0 fail；web 构建通过；Engine 94 pass / 0 fail / 4 skip。真实 PG/RLS 跳过，A20 远端未确认，UI 与部署未执行。文档修改不改变该次检查的 SHA 或证据范围。
