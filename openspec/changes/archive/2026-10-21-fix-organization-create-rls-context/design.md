# 创建组织RLS上下文设计（zh-CN）

## Context

见proposal.md。调用链：TenantMiddleware设置app.tenant_ids为用户已有租户 → routes_p5.go取reqDB → CreateOrganization嵌套事务 → 新tenant、owner成员、workspace和workspace成员 → Billing.Account插入新租户账务。最后一步的RLS仍检查旧范围，实际42501。现有独立域调用不带租户上下文，因此旧SQLite及域测试不能覆盖。

## Goals / Non-Goals

保持组织初始化全有或全无；RLS始终开启，局部范围只包含服务端新生成的组织ID；返回调用方时原范围逐字恢复，不能污染后续查询或连接池。支持HTTP外层事务及独立域调用，无表结构或API改变。

不改全局TenantMiddleware、TenantTx、Billing.Account或RLS策略，不增加通用抽象，不给任意传入tenant提升权限，不将空范围作为修复。邀请接受、成员撤销、其他HTTP提交错误不扩入本变更。

## Decisions

共享修复位于CreateOrganization，不在handler单独绕过。沿用其现有GORM事务/保存点边界，PostgreSQL读取并保留旧app.tenant_ids，为新组织初始化设置精确的新tenant范围；成功时在同连接上恢复旧值，任何读取、设置、初始化或恢复错误都返回并回滚。嵌套事务失败由回滚保存点撤销数据和局部设置，不能在已失败SQL事务里强行继续或伪装成功。SQLite无需设置PG变量。

不采用withSystemContext：其清空租户值会进入全租户系统范围，超过本任务最小授权。也不直接套用TenantTx后立即返回：嵌套SET LOCAL在成功时会留到外层事务结束，必须显式恢复；本次不为解决一个入口重构通用事务设施。

新组织ID由NewID服务端生成、owner来自已认证用户，不信任请求体传入租户/角色。设置范围只为新建组织相关初始化，不合并其他任意租户。

## 验证设计

一个最小PG回归文件，复用pgTestDB、真实TenantMiddleware、RegisterP5Routes和现有model/测试帮助函数。运行前确认session_user/current_user均非超级用户、无BYPASSRLS，账务表ENABLE/FORCE RLS为true。仅新建专属空测试库，结束清理；不修改18088原业务记录。

先RED：真实POST /api/intelligence/orgs在普通角色下应成功却返回500/42501，并核对失败无初始化残留。再GREEN：完整tenant/owner/workspace/member/billing成功提交，payer是创建者、kind/owner_type正确。

在已有多租户外层事务内直接调用共享函数，检查返回后app.tenant_ids原值一致；未属于调用方的tenant账务查询看不到、写入受42501拒绝（测试保存点回滚预期拒绝，不让故意失败污染后续断言）。注入仅作用于新账务初始化的失败，确认组织/成员/工作区/账务完全回滚，旧上下文恢复且原租户查询仍可执行。补独立域调用/SQLite既有组织、角色与账务专项回归。

## Risks / Trade-offs

范围恢复本身失败必须使本次创建失败，不能报告成功。SQL失败后的恢复依赖现有事务/保存点回滚，需用真实PG反证测试，不靠mock设置变量。测试角色带CREATEDB仅为专属新库设施，最终清理测试库及本轮角色，不触碰原用户/库。

现有dirty文档、旧归档配置和前端改动保留；只精确提交本变更。已有18088仍旧镜像，本轮不擅自重建或部署；源码+真实PG路由集成通过不等于已部署18088复验。

## Migration Plan

无迁移或数据库定义变更。后续部署后新请求走修复；本轮只本地开发验证，不推送/合并/发布。失败创建在原事务中回滚，不需要清理用户历史数据。
