# Handoff：组织创建RLS hotfix验证通过，待归档

## Session Metadata

2026-10-21 19:54:58 +08:00，Asia/Shanghai。WeaveSense-Flow，codex/fix-organization-create-rls-context。基线21bb303a，实现32c0b1ed；后续仅补验证状态/交接，最终HEAD用git核对。

## Handoff Chain

Continues from：[导出与权限验收](2026-10-21-191119-export-permission-acceptance-handoff.md)。前组织创建500由本hotfix源码/集成测试修复，18088仍旧镜像，历史失败不抹除。既有F/C与旧hotfix不重做。

## Current State Summary

fix-organization-create-rls-context完成3/3任务，先非超级用户PG RED再GREEN；build guard14/14、verify guard5/5通过。phase=archive、verify_result=pass、archive_confirmation=pending，等待本次最终归档确认。未归档、推送、合并、发布或部署。

## Important Context

用户明确确认本变更新分支与proposal/design/tasks；禁止关闭RLS、真实模型、子代理，全程遵守。原dirty及未跟踪文件保留。pending-test本次新增节独立patch暂存，旧验收增量未混入提交。模型额度0，不可擅自追加真实调用。

## Codebase Understanding

TenantMiddleware设置已有租户范围，POST /orgs取reqDB调用CreateOrganization；新组织不在旧范围，Billing.Account插入原来42501。现在只在CreateOrganization已有事务/保存点内读取原范围，设置为服务端新生成tenant，创建组织/成员/workspace/账务后恢复原值。失败依靠同事务/保存点完整回滚，不进入空系统范围。没有改通用Billing、TenantTx、TenantMiddleware、RLS策略或表结构。生产调用方仅routes_p5.go，其他为测试/忽略的验收准备。

### Critical Files

- `backend/internal/flow/orgs.go`：共享局部上下文修复。
- `backend/internal/flow/orgs_rls_pg_test.go`：普通PG角色真实HTTP及事务安全回归。
- `openspec/changes/fix-organization-create-rls-context/.comet.yaml`：验证通过、待归档。
- `docs/superpowers/reports/2026-10-21-fix-organization-create-rls-context-verify.md`：RED/GREEN命令结果与边界。

## Work Completed

非超级用户/NOBYPASSRLS PG17 RED实际复现真实中间件+POST /orgs=500/42501；修复后4子场景HTTP、嵌套上下文/跨租户隔离、注入账务失败回滚、独立调用通过。失败后从事务外确认tenant/membership/workspace/workspace-membership/billing均无隐藏残留；旧事务可继续访问自己的账务。触发器检查新账务初始化范围恰为新tenant，不能清空范围绕过。

同普通角色既有3项RLS专项全部通过：未过滤查询、后台租户范围、能力/供应商表；连同本次共4个PG顶层通过。宿主组织/角色/预算4顶层通过，宿主PG项无DSN跳过，已独立实跑不计宿主通过。go build ./internal/flow退出0。新测试库残留0，本轮flow_uat_org_rls_20261021角色已DROP/残留0。预期42501/P0001日志为拒绝/注入反证。

### Decisions Made

采用本次确认的新分支，hotfix direct、review_mode off，不启动子代理。scale因10文件含工件选full，核对局部生产修复+单测试文件后显式override light。无delta，skip_specs true。Comet旧运行时任务全勾选后需已安装CLI reconcile步骤，已正常记录，不绕过guard。

## Pending Work

本次最终本地归档尚未确认。18088没有重建/替换镜像，真实登录API部署后复验仍待；只完成当前源码真实PG路由集成，不称18088已修复。邀请接受链、完整角色矩阵/同租户多workspace、离线缓存竞态、真实MCP/外发、旧库迁移、SSE/未知结果恢复和监控/多实例及G01交付门槛仍未通过。O01–O06不擅自实现。

## Immediate Next Steps

1. 核对git/HEAD/branch与本变更元数据，不重复实施已完成测试。
2. 用户确认本次最终本地归档后执行comet-archive，保留其他修改；未确认则留archive pending，不重复询问旧分支/工件选择。
3. 若后续部署验收，需明确隔离环境重建范围并保留已有数据；不调用模型。新测试账号密码只在本地Git忽略私有文档，不能输出聊天。

## Environment State

flow-uat-20261020/18088保持运行，8088不动，未启动dev server。非超级用户角色只用于新测试库，已清理；原账号/测试组织/报告未操作。测试二进制和日志在.local/flow-uat-20261020，env与密码不输出。

## Potential Gotchas

保护旧归档.comet.yaml、canvas-cloud-agent-panel.tsx、历史handoff、prd-audit-9.md、table.csv和所有参考文件，不reset/clean/restore、不git add .。无.codegraph不建索引。当前镜像非纯HEAD且没有本修复；真实报告导出/33权限结果引用前轮证据，未本轮重跑，不称全量PRD/发布通过。

## Related Resources

[验证报告](../superpowers/reports/2026-10-21-fix-organization-create-rls-context-verify.md)、[任务](../../openspec/changes/fix-organization-create-rls-context/tasks.md)、[前轮证据](2026-10-21-report-export-account-isolation-evidence.md)。
