# Proposal：邀请接受 RLS 上下文修复

## Why

18088 的真实普通用户创建邀请返回200，尚未加入目标组织的账号接受有效邀请返回400。只读回滚检查确认邀请存在、未过期、未使用、未撤销，但在受邀者现有租户范围内可见数为0；`AcceptInvite` 在该范围内按令牌哈希查询 `flow_invites`，被 FORCE RLS 读过滤。

## What Changes

- 修复共享 `AcceptInvite` 的凭证兑换边界：以持有的随机邀请令定位唯一邀请，然后仅进入该邀请的目标租户完成原子兑换；保留并恢复调用者原范围。
- 复用现有事务及上下文能力，不修改通用 TenantMiddleware、TenantTx 或 RLS 策略；不关闭或禁用 FORCE RLS。
- 先在非超级用户、无 BYPASSRLS 的真实 PostgreSQL 新测试库补最小 RED，再修复；验证正常加入、无效/过期/撤销/已使用令牌、竞争单次兑换、失败回滚与上下文恢复。
- 更新功能/接口边界说明、待验清单和验证报告，明确真实执行与未执行项。

## Capabilities

### New Capabilities

无。不新增 API、角色、依赖或数据库对象。

### Modified Capabilities

无规格级要求变化。已完整核对 `flow-workspace-roles`、`intelligence-workspace` 和 `flow-schema-and-status-alignment`；本次恢复既有邀请加入行为及成员角色合同，不改变验收要求，设置 `skip_specs: true`。

## Impact

- 生产入口：`backend/internal/flow/routes_p5.go` 的 POST /intelligence/invites/accept → reqDB → `AcceptInvite`。前端 `intelligence.ts` 的 acceptInvite 经现有 http 调用，workspace-admin-panel 接受后刷新工作区目录。其余后端调用方为 p5/p7 测试；修复共享入口一次，不逐个改调用方。
- 预计代码范围：`backend/internal/flow/orgs.go` 和一个 PG 专项测试文件，配套必要文档和本变更工件。若需新迁移、策略或跨模块改造，停止并按 Comet 重新确认范围。
- 非目标：不重做组织创建、不改变邀请邮箱与令牌的现有产品合同、不改角色映射、不修复其他已登记权限问题、不新增自动外发；不修改 Engine、前端或既有验收数据。
- 基线：`6d7c00ec55548f3c4419b800d5139e113a545812`。用户已选新分支 `codex/fix-invite-accept-rls-context`；旧文档/归档/Agent面板与未跟踪文件独立保留。
- 尚待实测：PG 锁与条件更新在并发下的单次兑换，嵌套事务设置恢复和数据库失败后的原子性。前轮真实400和不可见证据不能替代本次自动 RED。
- 全程不调用真实模型、不启动子代理、不推送/合并/发布；本轮源码验证不自动等同18088已更新。
