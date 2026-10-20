# Handoff：已有画布情报交接修复与真实复验

## Session Metadata

- 时间：2026-10-20 23:57:05 +08:00，Asia/Shanghai。
- 项目：WeaveSense-Flow；分支 `codex/fix-intelligence-canvas-handoff-sync`。
- 实现HEAD：`684816b29f46641d1372b588c5b5f1fb24713e73`；之后只补本轮验证与工作流文档，以git核对最新HEAD。
- 用户已确认proposal/design/tasks、新分支及hotfix直接执行；没有启动子代理、推送或发布。

## Handoff Chain

- Continues from：[上一交接](2026-10-20-231114-handoff-sync-open-review-handoff.md)。
- 历史F01/F02与C01/C02已经完成并本地归档，不得重复开发。最初基线说明见 [原交接](2026-10-20-202424-handoff.md)。

## Current State Summary

共享加载入口已修复：旧dirty分支把remote当新基线却保留整份local，后续误发remove_node删除云端交接节点。先有实际RED，再复用三方合并转绿；38项专项与真实构建通过。18088只重建web，中文专题投送已有/新画布、刷新/重开与PG存在性交叉核对通过。旧U03历史脏缓存缺少可信基线，保持本地并报错；不能自动恢复未知编辑，也不宣称该缓存正常合并通过。00:04续记：文档已提交1f4d2ecc，build guard重新运行后14/14通过并进入verify；最终verify状态查本变更.comet.yaml，归档仍须本hotfix单独确认。不得将本轮通过扩大为全部U01–U05或发布通过。

## Important Context

- 保留全部已有修改，尤其归档 `openspec/changes/archive/2026-10-19-sidebar-intelligence-entry/.comet.yaml`、画布Agent面板、历史handoff、prd-audit-9.md、table.csv及本地中文参考文件；不reset/clean/restore，不git add .，不输出凭据。
- 画布Agent面板属于其他修改，当前SHA256 `8bf1a7a5c1cb4437148f31885cc6afd6b96dfbb28a0989b38e4814caf147bf89`。构建包含该工作区修改，不能称纯HEAD镜像。
- `.codegraph`不存在；不建立索引。文档中文，web使用Bun、业务API使用现有http。
- 8088旧部署与源码不一致，保持其服务/数据不动。用户授权隔离 `flow-uat-20261020` 的18088、新卷和新空库迁移；没有生产迁移授权。
- 真实模型/付费API、SMTP/Webhook外发及G01交付仍须独立授权；缺凭据仅记录变量名和本地配置步骤。

## Codebase Understanding

### Architecture Overview

情报后端共享HandoffToCanvas追加节点，前端跳转由画布生命周期调用loadCanvasProjectForEditing。已确认远端基线与本地编辑投影必须分开；本次复用mergeAgentCanvasEditor。全部调用方包括生命周期、卡片/文件夹、列表预载/导出、创作续聊、生成同步/消费。聚合syncRemoteUserData有相似静态线索但未找到外部调用，不据猜测扩大修复。

### Critical Files

| 文件 | 用途 |
|---|---|
| `web/src/services/user-data-sync.ts` | 共享加载、基线、操作保存与会话守卫 |
| `web/test/canvas-handoff-sync.test.ts` | 本地标题/删除/干净缓存/同字段冲突/缺基线/账号切换6项回归 |
| `web/src/lib/canvas/agent-canvas-patch.ts` | 复用的三方合并 |
| `openspec/changes/fix-intelligence-canvas-handoff-sync/tasks.md` | 3项任务及本次工作流 |
| `docs/superpowers/reports/2026-10-20-fix-intelligence-canvas-handoff-sync-verify.md` | RED、专项、构建及UI/PG证据 |
| `docs/content/docs/progress/pending-test.mdx` | 用户确认与未验边界 |

### Key Patterns Discovered

不能用remote与旧local的直接差分解释本地意图。缺可信旧基线时应停止覆盖；无法区分未知本地删除与云端新增时不猜测。视口裁切使屏外节点不在DOM中，刷新后适应画布再检查；等待真实正文，不能仅凭投送toast或瞬时AX节点数判定。

## Work Completed

### Tasks Finished

- 最小测试确认RED：加载遗漏节点，后续PATCH确实包含删除交接节点。
- 共享入口复用既有合并，保留冲突、session epoch及本地更新守卫，无API/表/依赖变化。
- 5文件专项38通过、0失败、114断言；Comet实际日志5b003e22-b459-436b-9a6b-e08a2e87540f。
- `bun run --cwd web build`实际执行tsc与Vite、退出0；日志da561536-1a31-4198-abfe-127d259b39c3。错误命令只打印Bun帮助的记录无效，报告已说明。
- 仅18088隔离web重建退出0；已有画布6节点/1原连线刷新和重开保留，新画布1节点刷新保留，PG只读查询交叉确认。

### Decisions Made

用户本次单独选择分支隔离与确认工件；hotfix direct/direct/off设置来自本变更，不继承旧档。旧脏缓存只保护，不清除、不静默覆盖。自动审查按off跳过，人工审阅聚焦共享入口及既有守卫。

## Pending Work

- Comet guard完成后仅在用户确认本hotfix归档时执行本地归档；此前文档变更的归档授权不自动覆盖本hotfix。保留分支，不推送/合并/发布。
- U02报告/文档与真实引用、跨范围拒绝、旧脏缓存人工恢复仍待验；U01角色/多工作区、U03其余算子/环/导入、U04/U05模型/MCP/审批/SSE/预算/重启仍不算通过。
- A02–A10真实恢复/迁移/监控/媒体与模型配置仍待验证；A11–A15实采外发依赖配置后最后处理。G01仍待交付授权；O01–O06不擅自实现。

## Immediate Next Steps

1. 先核对git status、HEAD、branch及openspec list --json；只对本聚焦变更继续，不覆盖其他文件。
2. 阅读本报告最新Comet结论；若phase archive则先取得本变更归档确认再执行脚本，不能重新修同一实现。
3. 归档后按handoff原验收顺序继续无凭据验证，实采/外发/真实模型和交付等待明确授权。

## Environment State

### Running Processes

隔离Compose core仍运行：web18088、platform、Engine、scheduler、PG17、Redis、MinIO。用户已自行登录内置浏览器；不采集账号密码。旧tab1保留原U03错误现场，新tab3保留成功复验画布；结束前markHandoff。不启动宿主dev server。

### Environment Variables

只记录名称：`CANVAS_BACKEND_DATA_DIR`、`PLATFORM_DB_PASSWORD`、`INTEL_DB_PASSWORD`、`POSTGRES_SUPERUSER_PASSWORD`；值仅在Git忽略的隔离env内。若运行宿主后端必须显式 `CANVAS_BACKEND_DATA_DIR=../.local/project-workbench-debug`，不使用backend/data。

### Test Artifacts

已有画布 `canvas_5158db1cc61dae10566679a8`：最新只读PG revision14/6节点/1连线；新增 `intel_node_d54ec6430dd080cc2fe87c5c`、原连线 `Oel0tjAi3pF-NmhR4RiZf`。新画布 `canvas_b58dddabd32d632a1bbbbd3e`：revision3/1节点/0连线，交接节点存在。只有人工专题0外部证据，无模型或实采。

截图在 `.local/flow-uat-20261020/screenshots/`，包含u02-hotfix-existing-six-with-link.jpg、u02-hotfix-refreshed-six-with-link.jpg、u02-hotfix-new-canvas-refreshed.jpg，均是真实应用截图，未提交本地数据。

## Potential Gotchas

原tab1曾显示旧内容而PG已保留节点，新完整标签正常；未证明独立额外代码缺陷。旧U03无可信基线明确拒绝合并，仍需人工恢复决策。不能以归档代替发布，也不能以38项fixture专项代替live采集、模型、生产迁移或全PRD验收。

## Related Resources

[UI部署证据账](2026-10-20-ui-deployment-acceptance.md)、[本次验证报告](../superpowers/reports/2026-10-20-fix-intelligence-canvas-handoff-sync-verify.md)。完整PRD原文缺失，继续使用仓库196项审计明细与现行规格；历史861未勾选项需要去重，不当作待开发功能数。
