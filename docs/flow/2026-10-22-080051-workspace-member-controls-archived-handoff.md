# Handoff：成员撤销与 analyst 权限本地归档

## Session Metadata

- 时间：2026-10-22 08:00:51 +08:00，Asia/Shanghai。
- 项目：WeaveSense-Flow；分支 `codex/fix-workspace-member-controls-and-analyst-permissions`。
- 归档提交：`01d06cc192edafffdefbd911efc32118d04e2d02`；验证状态提交：`670efc56`；实现提交：`90ffb431`、`40dd9075`、`205a4ae3`。后续只补分支保留元数据和本交接，以git核对最终HEAD。
- 用户明确确认本次本地归档并保留当前分支；没有推送、Git合并、发布、模型调用、子代理或新聊天。

## Handoff Chain

Continues from：[修复与真实验收交接](2026-10-21-233702-workspace-member-controls-handoff.md)。本文件只更新归档收尾状态，不重复开发或运行旧归档。

## Current State Summary

`fix-workspace-member-controls-and-analyst-permissions` 已本地归档到 `openspec/changes/archive/2026-10-22-fix-workspace-member-controls-and-analyst-permissions`。两份delta按OpenSpec语义合入主规格，设计/计划已添加归档标注，七个工件移动及上述变更以11个精确文件提交。4/4任务完成、verify_result=pass、archived=true、branch_status=handled；最终archive guard实际6/6通过、退出0。OpenSpec活跃变更为0。

## Important Context

归档命令跨会话中断后原process id已不可恢复；实际目录、主规格合入、archived=true及归档指针均已存在，依据Comet恢复规则没有重复archive。不伪称重新取得该旧进程exit code；以归档提交与新跑最终guard核对完整性。

保留全部既有dirty和未跟踪文件：pending-test/pending-acceptance、旧侧栏归档配置、Agent面板、历史handoff、prd-audit-9.md、table.csv及本地中文参考。未reset/clean/restore、未git add .；pending-test此前仅暂存本轮新增节。此次不修改产品代码或现有数据，不重新执行模型或产品全量测试。

## Architecture Overview

成员面板通过既有http DELETE调用RemoveMember；角色PATCH调用SetMemberRole。两个原事务共同先锁tenant，再查询active目标、保护owner/最后管理员并写入，数据库错误完整回滚；有其他active owner/admin时允许管理员自撤销。analyst前端标签、邀请/编辑和类型保持一致；专题HTTP与本地工具共用intel.reports.write门禁，viewer/未知拒写及成员管理边界保持。

## Critical Files

| 文件 | 用途 |
|---|---|
| `openspec/changes/archive/2026-10-22-fix-workspace-member-controls-and-analyst-permissions/.comet.yaml` | 归档、验证、分支保留元数据 |
| `openspec/specs/intelligence-workspace/spec.md` | 成员撤销、角色投影及确认合同 |
| `openspec/specs/flow-workspace-roles/spec.md` | analyst专题写及隔离合同 |
| `backend/internal/flow/orgs.go`、`backend/internal/flow/p7.go` | 成员共享入口和锁顺序 |
| `web/src/pages/intelligence/workspace-admin-panel.tsx` | 真实撤销UI及analyst回显 |
| `docs/superpowers/reports/2026-10-21-workspace-member-controls-and-analyst-permissions-verify.md` | RED/GREEN、PG、HTTP、UI证据与限制 |

## Work Completed

本轮只核对归档及提交，不重复运行产品测试。引用前轮实际证据：Go专项9顶层通过、host PG1明确跳过；普通PG新库4顶层通过；Bun19通过/198断言、构建及lint通过；Comet实际build/verify均退出0，build14/14、verify5/5通过。

本轮主规格检查：`openspec validate intelligence-workspace --type spec --strict`有效；`flow-workspace-roles`提示原有Purpose仍为自动生成TBD占位。归档前版本已有该文本，本次没有引入，也不据此宣称两份主规格严格校验均无警告；保留历史内容，仅记录后续文档事项。

18088前轮已保卷更新平台/web，105项真实HTTP角色和同租户双workspace检查通过；真实UI覆盖analyst专题/角色/邀请、取消无请求、并发目标失效显示错误、双击一次请求和成功刷新。保留原成员HTTP会话撤销后立即404、目录移除、个人scope200、重放拒绝。普通PG另验证持久化失败全回滚及并发最后管理员保护，不把host跳过当PG通过。

## Files Modified

归档提交仅七个工件移动、两份主规格及设计/计划归档标注；本次后续只补本归档元数据和新交接。旧dirty归档文件、pending文档及Agent面板未混入提交。

## Decisions Made

用户已确认工件/设计/新分支/主会话TDD与review off、自撤销边界、最终归档并保留分支。本次明确不Git合并、不推送、不发布，不重复询问。自动审查未派发，主会话已做范围/权限/错误/并发核对。

## Immediate Next Steps

1. 先核对git status/HEAD/branch、openspec list --json及本归档元数据；归档已完成，不能再archive同一变更。
2. 按原U/A顺序继续未通过的无凭据验收；模型、真实MCP、外发及G01仍各待相应依赖和明确授权。
3. 若实际文件或运行环境有增量，先核对增量归属，不覆盖未知编辑或清旧画布脏缓存。

## Pending Work

未通过的发布门槛：完整U01权限与跨标签/离线缓存、U02其余引用链、U03算子/环/导入、U04/U05真实MCP/审批/SSE/预算及恢复；A02–A15恢复/迁移/监控/媒体、实采外发仍需对应真实证据。G01 push/merge/release未授权；O01–O06不擅自实现。完整PRD原文仍缺，以196项审计明细和现行规格可追溯，861项登记不是待开发功能数。

## Assumptions Made

未联网核对远端；本地origin/main不代表远端最新。前轮专项不等于完整PRD、全仓测试或生产交付。旧8088未作为本次源码环境，不把fixture或归档当live/发布。

## Potential Gotchas

新版本Comet要求实际check run，旧record-check声明不能通过guard；任务计划已映射四个稳定ID。通过已安装公共CLI运行，不改工具或绕过检查。归档日期为工具实际生成的2026-10-22，文档时间统一Asia/Shanghai。

18088前端保留原Agent面板dirty，镜像标记with-preserved-local-panel，不称纯HEAD；后端复用旧运行层并替换本地CGO静态二进制，glibc链接提示保留，不称标准生产镜像通过。暗色/窄屏、独立多浏览器同时在线与重启恢复未在此聚焦修复全验。原会话失权证据为保留HTTP Cookie，不称两浏览器同时在线。

## Environment State

隔离Compose `flow-uat-20261020` 的18088平台/web运行前轮修复；PG/Engine/调度器及8088保持，CANVAS_AUTO_MIGRATE=false，卷不变。旧非用户记录及实采文档/报告哈希保持，测试用户仅登录活动变化，模型任务/预算仍0。本轮归档不操作容器或数据库。

双库备份、回退镜像配置、测试账号/会话及截图在Git忽略的 `.local/flow-uat-20261020/member-controls-deploy`；只记录变量名CANVAS_BACKEND_DATA_DIR、PLATFORM_DB_PASSWORD、INTEL_DB_PASSWORD、POSTGRES_SUPERUSER_PASSWORD，不输出或提交值。宿主后端必须显式使用 `.local/project-workbench-debug`，不用backend/data；未新启动dev server。没有.codegraph，未建索引。

## Related Resources

- [前轮验证报告](../superpowers/reports/2026-10-21-workspace-member-controls-and-analyst-permissions-verify.md)
- [当前待测试](../content/docs/progress/pending-test.mdx)
- [原浏览器角色证据](2026-10-21-browser-role-workspace-acceptance.md)
