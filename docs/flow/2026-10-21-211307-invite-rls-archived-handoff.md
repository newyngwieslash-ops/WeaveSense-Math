# Handoff：邀请接受RLS hotfix本地归档完成

## Session Metadata

2026-10-21 21:13:07 +08:00，Asia/Shanghai。WeaveSense-Flow，分支codex/fix-invite-accept-rls-context。实现c94988d5、验证状态2cf6f513、归档3de3abb1；随后仅补分支收尾和本交接，最终HEAD以git核对。

## Handoff Chain

Continues from：[验证交接](2026-10-21-210852-invite-rls-verified-handoff.md)。本文件更新其待归档状态，已完成修复不重复实施，归档不重复执行。

## Current State Summary

用户明确确认本地归档并保留当前分支。fix-invite-accept-rls-context已归档至openspec/changes/archive/2026-10-21-fix-invite-accept-rls-context；脚本实际5/5通过，五个工件移动精确提交3de3abb1。最终archive guard6/6通过，archived=true、verify_result=pass、branch_status=handled，当前选择已清除，openspec list --json为0活跃变更。无delta specs，无主规格变化。未部署、推送、合并或发布。

## Important Context

本轮仅归档收尾，不修改产品代码，不重复产品测试。用户选择保留当前分支及原dirty/未跟踪文件，无需重复询问旧隔离、工件或分支决定。没有关闭RLS、调用模型、启动子代理或新聊天。18088仍旧镜像，源码通过不等于实际部署通过。

## Codebase Understanding

AcceptInvite原来沿请求已有租户范围查token_hash，受邀者尚不属于目标tenant，FORCE RLS将有效邀请隐藏。已修共享入口，在已有事务内复用withSystemContext仅做令牌单行查询和FOR UPDATE锁；立即恢复原范围，验证凭证后以精确目标tenant写兑换/成员，结束恢复原范围。数据库错误向上传递并回滚。不改变RLS策略、通用中间件、邮箱合同、API、角色、表结构或依赖。

### Critical Files

- `backend/internal/flow/orgs.go`：共享凭证兑换修复。
- `backend/internal/flow/invites_rls_pg_test.go`：普通PG角色六子场景检查。
- `openspec/changes/archive/2026-10-21-fix-invite-accept-rls-context/.comet.yaml`：最终归档状态。
- `docs/superpowers/reports/2026-10-21-fix-invite-accept-rls-context-verify.md`：RED/GREEN及限制。

## Work Completed

本轮实际archive-confirm与archive退出0，脚本5/5，五文件移动精确提交；按用户已明确选择保留分支后设置branch_status handled，guard6/6通过，清除选择，OpenSpec0活跃。没有重跑产品测试。

引用前轮实际验证：普通PG角色/NOBYPASSRLS下真实中间件路由RED=400；修复后HTTP、范围恢复/隔离、无效/过期/撤销/重用、注入成员写失败全回滚、并发单次兑换及独立调用六场景通过。触发器确认兑换更新和成员写入限定目标tenant，事务外无隐藏残留。相关组织四子场景及既有三项RLS专项合计5顶层通过，宿主组织/角色/预算4顶层与Flow包构建通过；build14/14、verify5/5。测试库/临时角色残留均0。详见前交接与报告。

### Decisions Made

本次工件、新分支、hotfix直接执行和最终归档均已确认；review_mode off，无子代理。归档仅移动工件，不发布。保留分支codex/fix-invite-accept-rls-context和现有工作区。

## Pending Work

18088尚未部署邀请修复，真实接受邀请400仍是旧镜像失败，部署后邀请接受/单次使用、viewer读写和撤销即时拒读仍待真实复验。完整角色矩阵、同租户多workspace、离线/跨标签、真实MCP/SSE/未知结果恢复、旧库迁移/恢复、监控/多实例与实采外发尚未全部通过。G01 push/merge/release待明确授权，O01–O06不擅自实施；模型额度0。

## Immediate Next Steps

1. 核对git/HEAD/branch、归档元数据和openspec list；归档已完成，不重复archive。
2. 后续如获准更新18088，核对实际Compose和备份，仅更新platform并保留卷，带原compose.acceptance.yml与compose.org-rls.yml，确认源码commit和二进制哈希。原有镜像是验收局部构建，不能称全栈纯HEAD或正式发布。
3. 复用前轮验收组织前先检查未用邀请状态，避免盲跑check-org-deployed.py新增组织；重验真实登录接受、重用拒绝和viewer撤销。不得在原验收数据上注入失败，不调用模型或真实外发。

## Environment State

本轮未重启服务或重新健康检查。沿用flow-uat-20261020/18088平台旧commit d5f17bfa，8088不动。原两库备份和旧镜像在本地保留；本次邀请修复未构建运行镜像。账号密码和env仅Git忽略本地私有文件，不输出。没有启动dev server；宿主后端需显式CANVAS_BACKEND_DATA_DIR=../.local/project-workbench-debug。

## Potential Gotchas

系统上下文例外只用于精确邀请令单行读取，不扩成全请求放行；FORCE RLS不关闭。未单独注入范围设置/恢复SQL自身错误，代码检查其传播，已实际注入成员写失败检查回滚。保护pending-test/pending-acceptance旧增量、旧归档.comet.yaml、Agent面板、所有历史handoff、prd-audit-9.md、table.csv和本地参考文件。不reset/clean/restore，不git add .，无.codegraph不建索引。

## Related Resources

[验证报告](../superpowers/reports/2026-10-21-fix-invite-accept-rls-context-verify.md)、[归档任务](../../openspec/changes/archive/2026-10-21-fix-invite-accept-rls-context/tasks.md)、[18088原失败](2026-10-21-org-rls-deployment-acceptance.md)。
