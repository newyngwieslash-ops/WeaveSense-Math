# Handoff：组织创建 RLS hotfix 本地归档完成

## Session Metadata

2026-10-21 20:10:58 +08:00，Asia/Shanghai。项目 WeaveSense-Flow；分支 codex/fix-organization-create-rls-context。实现提交32c0b1ed、验证状态提交fb2d736e、归档提交065a08c1；随后仅提交归档收尾元数据与本交接，最终HEAD以git核对。

## Handoff Chain

Continues from：[验证交接](2026-10-21-195458-organization-rls-verified-handoff.md)。本文件更新其“待归档”状态，不重复开发或归档同一问题。前轮导出和权限验收沿该交接链追溯。

## Current State Summary

用户明确确认本地归档并保留当前分支。fix-organization-create-rls-context已移动至openspec/changes/archive/2026-10-21-fix-organization-create-rls-context，五个文件移动已独立提交。archive脚本5/5通过；branch_status=handled、archived=true、verify_result=pass，最终archive guard6/6通过，当前变更选择已清除，openspec list --json返回0个活跃变更。未推送、合并、发布或部署。

## Important Context

本轮只做归档和文档收尾，不修改产品逻辑，不重跑产品测试。引用前轮真实非超级用户PG RED→GREEN及构建证据，不把归档等同于发布或完整验收。所有原有修改与未跟踪文件保留，模型额度0，不得再调用真实模型；不启动子代理或新聊天，不关闭RLS。

## Codebase Understanding

TenantMiddleware给请求设置已有租户范围，POST /orgs通过reqDB进入CreateOrganization；新组织账务原来因不在旧范围而触发42501。修复仅在共享CreateOrganization现有事务/保存点内读取旧范围，临时设置为服务端新生成tenant，完成组织/成员/workspace/账务初始化后恢复原值。失败依靠事务/保存点回滚，禁止清空到系统范围；未修改通用中间件、Billing、RLS策略、API或表结构。

### Critical Files

- `backend/internal/flow/orgs.go`：共享入口修复。
- `backend/internal/flow/orgs_rls_pg_test.go`：非超级用户PG回归。
- `openspec/changes/archive/2026-10-21-fix-organization-create-rls-context/.comet.yaml`：最终归档状态。
- `docs/superpowers/reports/2026-10-21-fix-organization-create-rls-context-verify.md`：实际验证及未执行边界。

## Work Completed

本轮实际执行archive-confirm、archive及最终archive guard，均退出0；脚本5/5和guard6/6通过，精确暂存五个工件移动并提交065a08c1。没有delta specs，主规格未变。保留当前分支的决定已落实，无需重复询问。

引用前轮验证：旧实现真实Gin中间件+POST /orgs返回500/42501；修复后HTTP、嵌套范围与隔离、注入失败回滚、独立调用四个PG子场景通过。同普通角色三项既有RLS专项通过；宿主组织/角色/预算四个顶层通过，宿主PG项因无DSN跳过但已独立实跑。go build ./internal/flow退出0，build guard14/14、verify guard5/5通过，任务3/3。本轮未重复运行这些产品检查。

### Decisions Made

用户已确认工件、新分支、实施与最终本地归档。保留分支和工作区；没有合并、推送、发布或删除。前轮review_mode off，无子代理；light验证覆盖真实PG安全专项。所有旧选择继续有效。

## Pending Work

发布门槛仍未通过：18088未重建/替换包含本hotfix的镜像，部署后真实登录组织创建API尚待复验。完整权限矩阵、邀请接受链、同租户多workspace与离线/跨标签缓存竞态仍待核实。实际MCP、SSE断线/未知结果恢复、旧库迁移、监控与多实例，以及依赖具备后的实采外发收口尚未完整验收。G01 push/merge/release另待明确授权，O01–O06不擅自实现。

## Immediate Next Steps

1. 核对git status、HEAD、branch、归档元数据和openspec list --json；本次归档已完成，不再archive。
2. 继续验收前先核对隔离Compose实际构建配置和源码归属，再在保留18088数据的范围内重建并验证组织创建HTTP与权限；不得把当前旧镜像说成已修复。
3. 沿前轮证据账推进未完成权限与恢复门槛。账号密码仅保存在Git忽略的本地私有文档，不输出到聊天；无额外模型调用、真实邮件/Webhook或生产迁移授权。

## Environment State

本轮未启动或重启任何服务；沿用flow-uat-20261020/18088与原8088，运行状态来自前轮，归档期间未做健康复验。临时PG测试库和专用测试角色已在前轮清理。没有启动dev server。宿主后端若需启动，必须显式CANVAS_BACKEND_DATA_DIR=../.local/project-workbench-debug。

## Potential Gotchas

保护pending-test.mdx与pending-acceptance.md旧验收增量、旧归档.comet.yaml、canvas-cloud-agent-panel.tsx、全部历史handoff、prd-audit-9.md、table.csv及本地参考文件。不reset/clean/restore，不git add .。无.codegraph不建索引。18088镜像含既有前端修改，不能称纯HEAD。五格式导出与33项权限结果为前轮证据，不算本轮重验；完整PRD原文缺失，196项明细和现行规格仍是追溯基线。

## Related Resources

[验证报告](../superpowers/reports/2026-10-21-fix-organization-create-rls-context-verify.md)、[已归档任务](../../openspec/changes/archive/2026-10-21-fix-organization-create-rls-context/tasks.md)、[导出与账号隔离证据](2026-10-21-report-export-account-isolation-evidence.md)。
