# 命令 Outbox 领取所有权与持久化错误传播

## Why

交接 F01/F02 指向现有手动采集和研究命令的安全边界。基线为 main / f0231e841fd17d24c6b81125625f02f41e76be78。

调查证据：新增最小测试已复现同命令 Engine 调用两次、三种迟到响应覆盖新回执/预算、投影和预算写失败被忽略或部分提交、REST/Agent 隐藏读取错误。外层租户事务回滚前已发生外部调用，未给用户幂等键时转发键为空，也均由测试先记录失败。原始输出与修复后证据见本轮 verification report。

已执行 `go -C backend test ./internal/flow -run TestCommandOutbox -count=1`，退出码 0，包耗时 3.341s；现有三个测试未覆盖上述竞争和写失败情形。

## What Changes

- 在共享派发入口验证领取结果，以本次领取版本约束所有终态、投影和预算副作用；过期未知结果保留对账语义。
- 回执、失败终态、预算释放/未知标记、投影写入保持事务一致，数据库失败向上传播，调用方不能根据响应状态码将其改写成成功。
- 使用现有 Go/GORM 测试工具复现竞争领取、迟到回执和持久化失败；先记录 RED，再修复并验证 GREEN。若某条线索不成立，记录反证并收缩改动。
- 同步服务合同和 pending-test，区分本轮执行证据、历史报告与真实环境待验收项。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `flow-command-outbox`：明确领取所有权、未知结果与持久化失败的验收边界。

## Impact

涉及 backend/internal/flow 的 command_outbox.go、现有测试、routes.go、tools.go，以及必要专题文档；不引入依赖、公开 API 或数据表字段。用户已确认同一聚焦 hotfix，在 codex/fix-command-outbox-ownership-and-errors 分支实施，保留原有修改，不使用子代理、不推送或发布。

C01/C02、U01–U05、A01–A15 按交接依赖继续；此变更不宣称完成发布验收。原有脏文件已归为用户保留文件，未经修改或纳入提交。
