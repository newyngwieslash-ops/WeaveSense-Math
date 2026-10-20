# Handoff：登录后验收发现已有画布投送丢失

## Session Metadata

2026-10-20 23:03:39 +08:00（Asia/Shanghai）。分支 `codex/align-flow-status-and-baseline-evidence`；验收构建基线 `5ad423dbc20a17e2c48371e50c792e737ac97b7e`，后续文档提交不改变该构建证据。接续 [隔离部署交接](2026-10-20-223323-flow-ui-isolated-acceptance-handoff.md)；完整任务范围见 [原始交接](2026-10-20-202424-handoff.md)。

## Current State Summary

用户已在Codex内置浏览器登录18088。完成U01部分与U03部分真实UI验收；U02发现“投送已有画布成功提示后节点缺失”，同一专题投送新画布对照成功。无产品修复。Comet意图已路由hotfix，初始化并选择 `fix-intelligence-canvas-handoff-sync`，停在open入口隔离选择，待用户答复异步单选。原文档变更已归档，16历史归档；本新变更不代表旧功能重新开发。

## Important Context

本轮没有模型调用、采集、真实外发或生产迁移；没有子代理、push/merge/release。隔离项目/新卷/18088/新空库初始化已有明确授权，不重问。新前端可能包含另一方 `canvas-cloud-agent-panel.tsx` 单行模型校验修改，始终保护、不暂存，不称纯HEAD。

## Architecture Overview

复用当前Flow Compose，隔离项目 `flow-uat-20261020`。所有配置和新密钥保存在Git忽略的 `.local/flow-uat-20261020`，不输出值；仅回环18088公开。浏览器所有交互使用cua；当前有效绑定为browser2/tab1，变量activeUat。旧登录tab2已关闭。结束前重新markHandoff，后续恢复需重读cua文档。

## Work Completed

- U01九视图打开、当前中文空态、owner回显、助手打开/Esc关闭；实际390×844抽屉边界0–390、明暗显示。旧8088内容横移在新版相同步骤未复现，未据猜测改代码。
- U03人工中文JSON非法→合法恢复、2条数据；If/Else score>80 T1/F1，>100 T0/F2；带引号逗号CSV2条、过滤2→1、提取score/title、撤销/重做、刷新后节点和连线恢复。
- U02创建人工中文专题（无真实证据、未调用模型）。既有画布投送提示成功但节点不存在；新画布投送正文与来源脚注可见，作为对照。
- 本地core健康/PG信源RLS结果继续引用上一交接，不把它写成全产品通过。

## Critical Files

- [本次证据账](2026-10-20-ui-deployment-acceptance.md)：含登录后结果、失败输入/ID/PG结果、修复范围草案与3项任务。
- `docs/content/docs/progress/pending-test.mdx`：本轮实际UI补证和U02失败登记。
- `backend/internal/flow/canvas_handoff.go`：REST/Agent共享投送入口，CAS更新画布payload/revision、再镜像会话。
- `web/src/services/user-data-sync.ts`：loadCanvasProjectForEditing、syncRemoteUserData、buildCanvasOperations、实际重放/保存。
- `web/src/pages/canvas/use-canvas-project-lifecycle.ts` 与 `project.tsx`：加载、intelNodeId选择及预填处理。
- `web/src/pages/intelligence/canvas-handoff-modal.tsx`：投送后直接onNavigate，未主动接入画布同步基线。

## Files Modified

本轮只更新验收记录、pending-test并新增本交接；新Comet目录只有骨架/初始化状态，尚未写proposal/design/tasks或实施测试。上个交接和验收记录也属于本会话产物，可精确纳入文档提交；旧用户handoff、旧sidebar归档.comet.yaml、prd-audit-9.md、table.csv、.zcodeignore、中文参考及画布Agent外来修改均不纳入。

## Decisions Made

新发现按独立聚焦hotfix处理，不复用前两个归档的隔离/执行/测试/审查设置。建议新分支 `codex/fix-intelligence-canvas-handoff-sync`；当前异步问题也提供当前分支或暂缓选择，用户未答前不切分支、不写isolation、不进入实施。选择后依技能完成具体工件确认，再先最小失败检查定位根因。

## Immediate Next Steps

1. 收取用户对新变更隔离的选择，遵循comet-hotfix入口规则；当前命令会话70053正在完成select/check/status，先收尾读取，避免重复init。OpenSpec版本1.14.0。
2. 围绕真实复现补最小自动失败证据。当前线索是loadCanvasProjectForEditing的dirty分支：新remote被当作已确认基线而旧local保留，后续buildCanvasOperations可把云端新增节点生成为remove_node。登录聚合恢复也有remote→local差分。**尚未证实具体触发分支，不能直接修猜测。**
3. 追踪全部调用方，修共享入口，重验既有画布、新画布、本地未确认编辑、刷新/重开、版本冲突；再继续U02/U03剩余场景与U04/U05/A。

## Verification

失败画布 `cFOSdg0PrpHG-bgkuYVC9`（U03隔离验收），失败节点 `intel_node_ad4cf9037265ed8792064918`；POST /api/intelligence/canvas-handoff日志status200。UI适应/刷新仍只显示原两节点；PG只读查询 `revision|nodes|exists` 为 `15|2|f`；mutation receipt15文档也是2节点（13、12等旧回执亦2），尚未取得请求operations证明具体删除来源。未把它武断称为事务提交失败或渲染问题。

新画布对照 `canvas_5158db1cc61dae10566679a8`，新节点 `intel_node_8ff744fa1d01f2e737b755dd`，中文内容/来源脚注可见、URL参数被正常消费；自动唤起Agent已关闭，未发模型请求。截图在忽略目录screenshots，文件名详见证据账。

本轮未运行新的Go/Bun产品测试；之前自动测试见独立基线证据账，不能移用为本轮通过。手工/UI和只读PG结果均如上。全局主题已恢复浅色，viewport已reset；画布自身黑色外观为独立配置。

## Assumptions Made

既有画布路径有实际数据丢失现象，根因仍需小测试；新画布对照只覆盖人工topic，未证明report/document、证据引用或格式导出。测试数据只在新隔离库保留，不删除。

## Potential Gotchas

浏览器viewport能力之前作用于重复登录tab2，第一次设置并未改变tab1，不能算桌面/窄屏证据；关闭tab2后已实测innerWidth390/1280。AX树会短暂保留关闭动画节点，截图/后续状态已核对。Docker访问需require_escalated；SQL通过stdin传入避免PowerShell转义损坏。Comet稳定CLI会较慢，优先等待原会话。

## Environment State

18088新core仍运行，用户已登录；tab1停在新画布对照页。未由本轮重启旧8088。工具会话70053仅状态命令；其他构建已退出0，无未知产品测试进程。密钥仅保留变量名称和本地文件位置，不复制到报告。使用D:/python3132/python.exe -X utf8运行handoff脚本。

## Release Gates

U02已有画布失败为新增未通过门槛；U01多账号/工作区/角色、U02导出与引用/冲突、U03全部集合/Skill/环/导入、U04/U05模型/MCP/审批/SSE/恢复、平台全表PG/多实例/提交不确定、A02–A15及G01真实交付均未收口。不称全产品验收或发布通过。
