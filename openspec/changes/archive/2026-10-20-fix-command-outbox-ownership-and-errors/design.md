# 最小修复设计（待复现校准）

## Context

动机与证据等级见 proposal.md。三条入口为 REST proxyCommand → Enqueue/DispatchOne、Agent Tools.Execute → EnqueueAndDispatch、main.go → RunCommandDispatcher → DispatchDue → DispatchOne。预算复用 Billing.Release/MarkUnknown；前者内部已有事务，GORM 嵌套事务行为须由故障注入确认。

## Goals / Non-Goals

目标是恢复现有规格中的幂等、租约、对账及诚实回执合同。范围限定同一 Flow 命令链，不开发新的执行器、对账服务或数据库字段，不触发真实模型、外发或生产迁移。

## Decisions

1. 领取 UPDATE 必须实际命中才具备发送资格。全仓核对确认 attempts 仅由该入口递增，复用它作为领取版本，使用领取前版本和到期时间做条件更新并保留本次版本，避免领取后重读误认他人租约。回执事务先以 ID、dispatching 和本次版本条件更新，再执行投影及资金副作用；未命中者无权写入。测试同时验证同版本并发读取、活跃租约与三种迟到响应。
2. 过期 dispatching 不直接重发，统一进入 reconciliation_required。复用共享入口处理该状态，使 DispatchOne 和 DispatchDue 行为一致；预算保持预留，未知标记与投影在同一事务完成。无法持久化时返回错误并保留不可重发的 dispatching，不假定数据库失效时还能成功写入 reconciliation_required。
3. 成功/4xx/重试状态、投影和预算变更检查所有数据库结果并原子提交。Engine 已产生结果但本地事务失败时不得回复成功，也不得把记录重置为 pending；旧租约过期后只对账。既有明确网络/5xx重试政策保持现行规格约束，不能把数据库失败当成可重发条件。
4. REST、Agent 保留共享入口错误，未完成派发时返回实际排队/待对账状态，禁止以空状态码或已拿到的 Engine 响应绕过持久化结果。后台 DispatchDue 返回可观察错误，循环使用现有日志模式报告，避免静默吞错；日志只含安全标识和错误类别。
5. 新增测试落在现有 command_outbox_test.go，利用通道控制 Engine 返回顺序，利用 SQLite 触发器或 GORM 回调注入写失败；不增加测试框架。数据库提交不确定与 PG 多进程/RLS 另列集成边界，SQLite 测试不冒充生产证明。
6. 调用方处于外层租户事务时，GORM 内层事务仅是 savepoint。新增回滚测试确认先发送会越过持久化边界，因此共享入口检测 `gorm.TxCommitter` 后延后发送，外层提交后由现有后台派发；排队受理保留实际状态。用户未给键时也转发 row.IdempotencyKey，保证现行网络/5xx重试不丢失 Engine 幂等条件。两处均复用现有机制，不新增队列或接口。

## Risks / Trade-offs

- 领取版本方案以现有 attempts 单调增长为前提；实现前核对全仓写入者，若不成立则重新审阅方案，不能静默扩展 schema。
- 预算/投影写失败后需保留对账状态；测试必须检查余额和重复请求，而不只检查 error 非空。
- REST 和 Agent 也位于同一模块；预计产品文件超过 4 个时按 Comet 范围提示确认，不能借 hotfix 免除复现、文档与验收。
- 用户本轮已明确确认同一聚焦 hotfix 和 codex 分支；不重复询问文件数提示。外层中间件提交失败后的 HTTP 回执属于更广的响应提交合同，当前保持原实现并独立登记验收边界，本修复保证该外层事务提交前无 Engine 命令副作用。

## Migration Plan

预期不需要数据库迁移。部署、远端交付及真实环境验收独立保留，未经授权不执行。
