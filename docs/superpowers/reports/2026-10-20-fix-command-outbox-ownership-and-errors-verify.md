# Flow Outbox 并发与错误传播验证

变更：fix-command-outbox-ownership-and-errors。时区：Asia/Shanghai。基线：f0231e841fd17d24c6b81125625f02f41e76be78；实施分支：codex/fix-command-outbox-ownership-and-errors。本报告仅覆盖本轮 hotfix，不表示当前项目全量验收或发布通过。

本地实现提交：0419fd7f。后续工作流元数据及本报告证据补充不改变该实现提交中的产品代码。

## 工作流与范围

用户明确确认方案、新分支和同一聚焦 hotfix；采用本次 hotfix 默认 direct 执行、direct TDD 编排、review_mode=off，仍先复现再修复。未使用子代理或新聊天，未推送、合并、发布、运行生产迁移或调用付费模型/真实外发。

Comet open guard 通过；隔离方式为 branch，bound_branch 已绑定用户指定分支。规模为 5 个任务、1 个 capability，按 full 核对任务、proposal/design/delta spec 和实现，不创建 hotfix 已免除的 full Design Doc 或实施计划。自动代码审查按 review_mode=off 跳过，主会话仍检查完整差异、共享调用方和资金边界。

## RED：修改产品代码前的复现

| 检查 | 实际失败证据 | 命令与退出码 |
|---|---|---|
| 活跃租约竞争 | 两个派发者调用 Engine 两次，第二个返回 dispatched | `go -C backend test ./internal/flow -run 'TestCommandOutbox(Competing\|Late\|Persistence\|Expired\|Final)' -count=1`，1 |
| 迟到响应 | 202 覆盖新回执；422 释放预算并写 failed；503 改预算与投影为对账 | 同上，1 |
| 写入失败 | 回执/投影/释放返回 nil，部分事务提交；MarkUnknown 失败被上游错误掩盖；再次派发可发送两次 | 同上，1 |
| 最终读取 | 注入读取错误仍返回 nil | 同上，1 |
| REST/Agent | 读取回调在扫描后返回错误，REST 仍 202，Agent 仍 succeeded | `go -C backend test ./internal/flow -run TestCommandOutboxCallersPreserveReadFailure -count=1`，1 |
| 外层事务 | 尚未提交、随后回滚的事务内已调用 Engine 一次 | `go -C backend test ./internal/flow -run TestCommandOutboxDefersUntilOuterTransactionCommits -count=1`，1 |
| 自动幂等键 | 两次调用都未转发平台已有自动键，key 为空 | `go -C backend test ./internal/flow -run TestCommandOutboxGeneratedKeySurvivesLostACK -count=1`，1 |

首次过期后台测试另有测试环境问题：SQLite 对带不同时区偏移的日期文本排序与 Go 时间比较不同。注入时间改为 UTC，与 CommandDispatcher 默认时间一致，后台正向/反向测试再运行；没有把该测试构造问题当作产品缺陷。初次调用方 before-query 注入没有扫描出响应字段，不能复现按状态码清错；改成 after-query 返回错误后明确复现。

## 修复与规格映射

| 规格场景 | 实现与验证依据 |
|---|---|
| Transactional enqueue and immediate dispatch | 保留 Enqueue 原事务；检测外层 gorm.TxCommitter，仅在实际提交后的后台发送。原 transactional 测试及 DefersUntilOuterTransactionCommits 通过 |
| Idempotent replay and parameter conflict detection | 原 replay/409 专项通过；GeneratedKeySurvivesLostACK 验证自动键稳定转发 |
| Transient Engine failure and background retry dispatcher | 保留有界退避；领取检查 next_attempt_at；原 TransientFailureRetryAndReconciliation 与 retry_wait 写入故障检查通过 |
| 两个派发者竞争和旧响应迟到 | 领取必须命中，attempts 版本保留在内存中；回写事务先以状态/版本 CAS 再操作资金和投影。CompetingDispatchers、ConcurrentClaimFromSameVersion、三种 LateReceipt 子项通过 |
| 租约过期或回执持久化失败 | 不接管过期租约发送，直接/后台均通过同一事务进入对账；回执事务失败保持 dispatching 和预留，持久化及最终读取错误传播 |
| 失败终态和资金变更原子性 | 9 种触发器故障覆盖回执、投影删除/创建/更新、Release、MarkUnknown、失败终态、重试/对账回执；断言回滚、重复调用不重发，余额仍 49000、预留仍 1000 |

没有新增依赖、抽象层或表字段。使用现有 Billing、gorm.TxCommitter、幂等键、投影和后台派发；数据库结构无变化，无需 schema 文档或迁移。新增非平凡逻辑均有可运行检查，gofmt 已应用于实际 Go 文件。

## GREEN：本轮实际执行

| 命令 | 结果 |
|---|---|
| `go -C backend test ./internal/flow -run TestCommandOutbox -count=1` | 退出码 0，最近一次包耗时 6.652s；包含新增故障矩阵及旧 3 项 |
| `go -C backend test ./internal/flow/... -count=1` | 退出码 0，包耗时 22.075s；可配置的外部集成测试可能跳过，不等于真实 PG 通过 |
| `go -C backend build ./internal/flow` | 退出码 0；构建本次涉及的 Go 包，未构建无关前端 |
| `go -C backend test -race ./internal/flow -run TestCommandOutbox -count=1` | 退出码 0，包耗时 21.374s；没有 race 报告 |
| `go -C backend test ./internal/flow -run 'TestRuntimeRLS\|TestPostgres' -count=1 -v` | 退出码 0；TestRuntimeRLSBlocksUnfilteredHandlerQueries 明确 SKIP：FLOW_TEST_PG_RLS_DSN 未配置，不是 RLS 验收通过 |
| `openspec validate fix-command-outbox-ownership-and-errors --strict` | 退出码 0；MODIFIED block 保留旧三条 scenario 名称 |
| `git diff --check` | 退出码 0；只有已有 Git LF/CRLF 提示，无空白错误 |

race 检查后仅增加 retry_wait 故障子项并将测试 HTTP handler 的共享记录改为原子计数/通道；产品逻辑未变。新增/调整后 Outbox 专项再次通过。全 Flow/build 结果对应相同最终产品代码。

## 边界、跳过与发布门槛

- SQLite 与可控 Engine 不证明真实 PG17 非超级用户 RLS、多实例部署或网络分区/连接断开后的提交不确定。当前没有 FLOW_TEST_PG_RLS_DSN/FLOW_TEST_PG_ADMIN_URL 配置；后续本地配置这些变量，再运行现有真实集成路径，不能在报告中填写凭据值。
- 真实应用的 TenantMiddleware/UserTx 外层事务现在不会提前发送命令；本轮未在真实 UI 验证受理、后台派发、操作卡查询及重启衔接。这些保留在 pending-test/U05，不把工具受理 succeeded 当作业务任务完成。
- TenantMiddleware 外层 Commit 返回值与 HTTP 响应已发出的关系，是静态追踪中新登记的更广回执风险，当前未模拟连接断开复现；本次共享入口修复不宣称覆盖所有 HTTP 中间件提交失败。继续按 F02 集成边界核验，不能据此宣称全系统持久化错误均已消除。
- 没有执行 web/Engine 构建、真实 UI、Docker 部署、模型/实采、外发、备份恢复、生产迁移或远端 SHA/CI 核验。没有把旧报告当成本轮测试证据。
- 用户原有归档元数据、交接、审计表、table.csv 与参考文件均保留、不纳入本变更提交。

本地 hotfix 规格和代码检查满足本轮范围；全项目 C01/C02、U/A/G 发布门槛继续保留。Comet Runtime 实际运行 `go -C backend build ./internal/flow` 退出 0，并再次实际运行 `go -C backend test ./internal/flow/... -count=1` 退出 0（24.061s）。build guard 14/14、verify guard 5/5 通过，状态为 phase=archive、verify_result=pass，archive 入口 5/5 通过；尚未归档，必须等待最终用户确认。
