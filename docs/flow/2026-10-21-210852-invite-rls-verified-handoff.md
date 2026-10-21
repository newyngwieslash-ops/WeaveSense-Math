# Handoff：邀请接受RLS修复验证通过，待本地归档

## Session Metadata

2026-10-21 21:10 +08:00，Asia/Shanghai。WeaveSense-Flow，分支codex/fix-invite-accept-rls-context，基线6d7c00ec，实现c94988d5。后续只提交验证状态和本交接，最终HEAD用git核对。

## Handoff Chain

Continues from：[18088部署验收交接](2026-10-21-202755-org-rls-deployed-handoff.md)。组织创建修复已在旧18088镜像中验证，本次只修新发现的邀请接受400；旧归档不重复开发。

## Current State Summary

fix-invite-accept-rls-context任务3/3，真实普通PG角色先RED复现有效邀请HTTP400，再GREEN。build guard14/14、verify guard5/5实际通过，phase=archive、verify_result=pass、archive_confirmation=pending，等待本次最终归档确认。没有归档、部署、推送、合并或发布。

## Important Context

用户确认本次新分支及proposal/design/tasks，要求先失败测试再修复；不关闭RLS、不调用真实模型、不启动子代理。所有原dirty和未跟踪文件保留。pending-test仅本次新增节精确patch暂存，旧验收增量仍在工作区。模型额度0，不能擅自追加。

## Codebase Understanding

TenantMiddleware设置已有租户范围，routes_p5.go取reqDB进入AcceptInvite，受邀者尚未加入目标组织，原token_hash查询被RLS过滤。修复共享AcceptInvite：复用withSystemContext仅一次精确令牌哈希查询+FOR UPDATE锁，然后立即恢复旧范围；有效凭证后切入记录指向的精确tenant兑换及写成员，结束恢复旧范围。任何DB/设置/恢复错误均返回并由原事务/保存点回滚。不改策略、通用上下文helper或中间件、API、表结构、邮箱规则与角色合同。

### Critical Files

- `backend/internal/flow/orgs.go`：唯一生产修复文件。
- `backend/internal/flow/invites_rls_pg_test.go`：普通PG角色六子场景回归。
- `openspec/changes/fix-invite-accept-rls-context/.comet.yaml`：已验证，待归档。
- `docs/superpowers/reports/2026-10-21-fix-invite-accept-rls-context-verify.md`：实际命令与范围限制。

## Work Completed

旧实现在新增TestAcceptInviteRLSPostgres/HTTP下退出1，实际400/未新增成员。修复后六子场景通过：HTTP成功与viewer scope/撤销、嵌套范围隔离、无效/过期/撤销令牌、注入workspace成员写失败全回滚、两个连接同令牌仅一次成功、独立调用。触发器断言邀请更新及两类成员插入恰在目标tenant范围；成功/拒绝/失败均验证旧范围恢复，事务外确认无隐藏成员/令牌消费残留。

最终PG连同组织创建四子场景及既有三项RLS专项共5顶层全部通过。宿主组织/角色/预算相关4顶层通过，go build ./internal/flow退出0。新测试库与flow_uat_invite_rls_20261021临时NOSUPERUSER/NOBYPASSRLS角色已清理，残留数均0；只准备/清理角色用容器管理员，测试不用超级用户。原18088账号、组织和未用邀请未动。

### Decisions Made

本次用户确认的新分支与hotfix直接执行设置已绑定，实际遵循RED→GREEN。review_mode off且用户禁止子代理，自动代码审查跳过，主会话检查调用方与事务边界。scale因工件/文档计11文件选full，按技能override light，原因是单生产入口/单PG专项，无delta、迁移或跨模块实现。

## Pending Work

本次最终归档尚未确认；Comet保留archive pending、branch_status pending。18088镜像尚未包含本邀请修复，前轮真实接受400仍有效，不能把PG路由集成通过说成部署后成功。真实邀请单次使用、viewer权限与撤销完整HTTP/浏览器链路要在部署后重验。完整角色矩阵/同租户多workspace、离线/跨标签缓存、真实MCP/外发、旧库迁移、SSE/未知结果恢复、监控/多实例与G01交付门槛继续未通过；O01–O06不擅自实施。

## Immediate Next Steps

1. 核对git/branch/HEAD、openspec list与本变更元数据，已完成测试不重复开发。
2. 本次最终归档确认后按comet-archive执行并精确提交，保留当前分支与其他修改，未经确认不归档；不重复询问旧工件和隔离选择。
3. 后续若更新18088，保留原卷并带compose.acceptance.yml与compose.org-rls.yml，核对备份和源码版本。不要重跑旧check-org-deployed.py制造更多组织；复用前轮未用邀请做非破坏性真实接受验证前先核实状态。无模型调用。

## Environment State

flow-uat-20261020/18088沿用前轮平台commit d5f17bfa的局部镜像；本轮没有重启或重新探测其健康，8088保持不动。CGO=0 Linux测试二进制仅用于PG测试，不用于SQLite或部署；宿主相关测试和build正常。日志和工具在.local/flow-uat-20261020，私有账号/Key不输出。

## Potential Gotchas

只有令牌单行查询使用既有系统上下文；此明确凭证兑换例外不能扩成整个接受流程或全局请求放行。未对设置/恢复SQL本身注入故障，本轮真实注入成员写失败验证保存点回滚，设置/恢复错误传播由代码检查支撑。原中间件提交错误、成员移除后重新入会等非本轮问题不擅自扩展。

保护旧归档.comet.yaml、Agent面板、历史handoff、prd-audit-9.md、table.csv与所有本地文件，不reset/clean/restore、不git add .。无.codegraph不建索引。全量Go/前端构建未跑，PG真实专项不等于全PRD或发布通过。

## Related Resources

[验证报告](../superpowers/reports/2026-10-21-fix-invite-accept-rls-context-verify.md)、[任务](../../openspec/changes/fix-invite-accept-rls-context/tasks.md)、[部署与原失败](2026-10-21-org-rls-deployment-acceptance.md)。
