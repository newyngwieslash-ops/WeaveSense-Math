# Handoff：成员撤销与 analyst 权限修复

## Session Metadata

- 时间：2026-10-21 23:37:02 +08:00，Asia/Shanghai；收尾状态以git及.comet.yaml核对。
- 项目：WeaveSense-Flow；分支 `codex/fix-workspace-member-controls-and-analyst-permissions`。
- 起点 `2b47d1438f6574f8c6c717cb182c0b83e9460d17`；实现提交 `90ffb431`（共享成员安全边界）、`40dd9075`（专题门禁）、`205a4ae3`（前端）；证据 `a41c37ab`，任务映射 `38d38745`，后续仅本轮收尾。
- 用户已确认工件、设计、自撤销边界、新分支及主会话TDD/review off；尚未取得本次最终归档确认。无模型、子代理、新聊天、推送/合并/发布。

## Handoff Chain

Continues from：[浏览器角色验收](2026-10-21-213948-browser-role-matrix-handoff.md)。本次修该轮发现的撤销入口、analyst前端和专题写scope不一致；不重复旧归档hotfix。

## Current State Summary

产品代码、四任务、RED→GREEN及18088真实专项完成。Comet full的实际build/verify检查均退出0，build guard14/14通过，状态已进入archive、verify_result=pass；archived=false、branch_status=pending，尚未归档。功能/API/pending-test及验证报告已同步。实际报告见 `docs/superpowers/reports/2026-10-21-workspace-member-controls-and-analyst-permissions-verify.md`。

## Codebase Understanding

## Architecture Overview

WorkspaceAdminPanel→api/intelligence/http→P5 DELETE→RemoveMember；P7 PATCH→SetMemberRole。两个共享入口均先在原事务精确锁tenant，再查active目标/必要计数/写入；拒绝owner和最后管理员，错误原子回滚。已有其他active owner/admin时允许管理员自撤销。专题HTTP和本地intel_save_topic共同进入CreateTopic/UpdateTopic，canWriteTopics复用intel.reports.write，analyst可写但不能管理成员。

## Critical Files

| 文件 | 用途 |
|---|---|
| `backend/internal/flow/orgs.go`、`backend/internal/flow/p7.go` | 共享成员安全边界与租户行锁 |
| `backend/internal/flow/member_controls_test.go`、`backend/internal/flow/member_controls_pg_test.go` | owner/最后管理员/回滚/计数错误/并发最小检查 |
| `backend/internal/flow/topics.go`、`backend/internal/flow/topic_permissions_test.go` | analyst共享门禁及HTTP/工具/隔离 |
| `web/src/pages/intelligence/workspace-admin-panel.tsx`、`web/src/services/api/intelligence.ts` | 撤销确认、错误、防重复与analyst消费 |
| `openspec/changes/fix-workspace-member-controls-and-analyst-permissions/.comet.yaml` | 本次流程状态，不沿用旧hotfix |

### Key Patterns Discovered

旧RemoveMember排除owner的成员更新，却无条件删workspace关系；现在先拒绝整个事务。仅锁目标成员无法保护管理员总数，两个入口共同锁tenant。已有http会拒绝业务code失败，复用AppModal而非静态confirm。无.codegraph，未索引。

## Work Completed

### Tasks Finished

- 三组实际RED均已观察：owner/最后管理员/未命中假成功；analyst→editor及缺撤销命令；analyst专题服务拒绝和Gin403。
- Go专项9顶层通过，host PG1项SKIP明确记录；普通PG新库4顶层通过，含并发租户锁、异常全回滚和旧邀请/组织/RLS回归，临时角色已删除。
- Bun19通过/198断言，tsc/Vite、lint、Flow包构建通过；没有把SSR当真实点击。
- 18088新版本105项HTTP角色/同租户双workspace矩阵通过。真实UI analyst标签/专题新建编辑、角色viewer→analyst、analyst邀请（PG核对）、取消无请求、真实竞态失败保留错误、两次双击各一请求成功及成员刷新通过。
- 原成员HTTP会话撤销后立即404，个人scope200、目录移除、重放拒绝。最初Cookie测试工具错误已修正并用新组织重验，不把401当权限结果。

## Files Modified

仅本聚焦产品文件、专项测试、两份delta、工件/设计/计划、API/功能/pending-test新节和本轮证据。pending-test仅暂存本轮新增节，原dirty增量继续保留；旧归档.comet.yaml、Agent面板和全部其他未跟踪文件没有暂存或回滚。

## Decisions Made

用户选择新分支、主会话executing-plans、TDD、review off，明确禁止子代理。owner始终不能撤销；有其他有效管理者才允许管理员自撤销。用户已明确授权两个本地测试账号切换；自动审批拒绝后才询问，获准后执行。新测试目标被误判为旧matrix时，先只读证明不存在于部署前备份，再重试获准，没有绕过拒绝。

## Pending Work

## Immediate Next Steps

1. 核对git status/HEAD/branch和本变更元数据，读取最终报告；产品专项已通过，不重复开发。
2. 检查本次最终归档确认是否已回复；没有回复时保持archive待确认。新运行时不接受旧record-check声明作为guard依据，本轮已用实际check run补齐，勿改工具或绕过。计划已补四个稳定任务ID映射。
3. 用户确认后才本地归档：两份delta按OpenSpec语义合入主规格，移动工件及提交元数据；保持开发分支，不做Git合并/推送/发布。归档后按原U/A顺序继续；G01另待授权。

### Blockers/Open Questions

完整U01跨标签/离线缓存、U02引用链其余场景、U03算子/环/导入、U04/U05真实MCP/审批/SSE/预算恢复仍未全过；A02–A15恢复/迁移/监控/媒体、实采外发及G01未全过。此修复没有追加模型额度，没有真实邮件/Webhook或生产迁移授权。

### Deferred Items

暗色/窄屏及独立多浏览器同时在线未于本次重验；原会话失权使用保留HTTP Cookie，不称两浏览器同时在线。既有18088刷新后需重选workspace的观察未立新缺陷，本次不扩范围；旧无可信画布基线脏缓存不清除。

## Context for Resuming Agent

## Important Context

保护全部旧dirty/未跟踪文件，尤其pending-test、pending-acceptance、归档配置、Agent面板、历史handoff、prd-audit-9.md、table.csv及本地中文参考；不reset/clean/restore、不git add .。不启动子代理、不调用真实模型、不推送发布。无API/表结构/RLS变化，不需生产迁移。

## Assumptions Made

未联网核对远端，origin/main仅本地保存状态。完整PRD原文仍缺，以196项审计明细/现行规格可追溯；861项登记非待开发功能数；旧Loom /prd非Flow PRD v2。

## Potential Gotchas

18088前端包含原Agent面板dirty，运行镜像标记with-preserved-local-panel，不称纯HEAD。后端采用本地Go缓存CGO静态编译覆盖旧运行层；glibc链接提示保留，不能据此宣称生产标准镜像通过。原手写record-check不满足新版Comet guard，需实际check run；PowerShell wrapper可能吞掉--，可直接 `node F:/nodejs/node_modules/@rpamis/comet/bin/comet.js` 使用同一已安装公共CLI。

## Environment State

### Running Processes

18088隔离Compose仅platform/web更新，PG/Engine/调度器与8088容器保持。自动迁移false，卷保持。双库dump、回退镜像及截图仅Git忽略的 `.local/flow-uat-20261020/member-controls-deploy`。最终快照旧非用户行及实采文档/报告哈希保持，用户仅登录活动变化，模型任务/预算仍0。

内置浏览器tab3在127.0.0.1的connections、管理测试账号已登录，成功验收组织仅剩owner；tab1/2 localhost原用户会话未改。未新启动dev server。宿主后端若需启动，必须显式CANVAS_BACKEND_DATA_DIR=../.local/project-workbench-debug。

### Environment Variables

仅记录名称：CANVAS_BACKEND_DATA_DIR、PLATFORM_DB_PASSWORD、INTEL_DB_PASSWORD、POSTGRES_SUPERUSER_PASSWORD、FLOW_TEST_PG_RLS_DSN。凭据/测试账号及独立会话Cookie仅本地忽略目录，不输出或提交值。真实模型授权剩余0。

## Related Resources

- [验证报告](../superpowers/reports/2026-10-21-workspace-member-controls-and-analyst-permissions-verify.md)
- [前轮真实验收证据](2026-10-21-browser-role-workspace-acceptance.md)
- [待测试](../content/docs/progress/pending-test.mdx)
