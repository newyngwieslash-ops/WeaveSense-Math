# 创建组织的RLS上下文修复

## Why

18088真实POST /api/intelligence/orgs返回500，日志为flow_billing_accounts插入触发RLS42501。TenantMiddleware在请求开始设置用户已有租户范围，CreateOrganization随后创建的新租户不在其中，Billing.Account初始化被拒绝。此前不经过HTTP租户事务的域服务准备成功，不能作为API通过证据。

## What Changes

- 在共享CreateOrganization内部，只为本次新生成的组织初始化建立准确的租户范围，并恢复调用方原上下文，保持组织、成员、工作区和账务同事务。
- 先在非超级用户PostgreSQL、ENABLE/FORCE RLS下重现失败，再修复与验证；覆盖成功、完整回滚、上下文恢复和跨租户拒绝。
- 保留直接域调用与SQLite行为，复用现有事务设施，不改通用Billing或全局RLS策略。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

无规格要求变更。已核对现有flow-workspace-roles要求；本次修复已有创建组织实现的事务作用域错误，原权限合同保持。skip_specs: true，不制造新delta。

## Impact

主要入口backend/internal/flow/orgs.go；生产调用方routes_p5.go的POST /orgs，另有既有域测试。预计新增一个PG最小回归文件并同步功能/待测试文档，无API、表结构、迁移或依赖变化。

## 范围与授权

基线21bb303a，用户已选择codex/fix-organization-create-rls-context。所有旧dirty/未跟踪文件保留，不关闭RLS、不使用空tenant系统范围绕过、不调用真实模型、不启动子代理、不推送/合并/发布。不修改邀请流程、其他已归档hotfix、全量权限矩阵或生产数据。

## 验收目标

真实TenantMiddleware+POST /orgs在普通PG角色下返回成功，完整组织与账务持久化；原上下文逐字恢复，其他租户读写仍拒绝；注入账务初始化失败后无组织/成员/工作区/账务残留，外层事务可继续安全使用。没有18088镜像部署授权时只记录源码/PG集成证据，不称线上重验通过。
