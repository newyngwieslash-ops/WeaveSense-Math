# 长提问合成入队修复

## Why

真实隔离验收发现，合成提问超过80字符时，入队在预算事务内触发PostgreSQL SQLSTATE22001（varchar(80)），任务未创建，模型未调用。根因是StartSynthesis把完整Question直接作为FlowBudgetReservation.RefID；模型提示与SynthesisContext本来可保留完整问题。研究、定时简报和同步合成都共享此入口。

## What Changes

- 仅在StartSynthesis为预算参考摘要复用truncRunes，限制至80个Unicode字符，保留短问题原值。
- 模型提示、任务上下文保留完整Question；租户、幂等键、预算金额与结算合同不变。
- 先增加最小RED回归，再修共享入口，覆盖根预留/子分配、中文与英文边界、同键重放及预算失败。
- 使用隔离PostgreSQL新测试库验证真实varchar约束，全程使用现有测试网关或只入队，不连接真实模型。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

无规格要求变更；修复现有合成入队实现的字段映射错误，.openspec.yaml设置skip_specs: true，不新增delta规格。已核对现有规格目录及intelligence-workspace全文，其报告/权限合同不变。

## Impact

预计生产改动只有backend/internal/flow/model_gateway.go一处映射；增加一个最小回归测试文件，复用既有SQLite/PG测试设施。同步功能与pending-test中本问题状态及验证报告。无API、表结构、迁移或依赖变化，不改通用Billing对业务ID的处理。

## 范围与约束

用户已选择codex/fix-synthesis-long-question-enqueue；基线7f61c190。全部已有修改/未跟踪文件保留，不启动子代理、不调用真实模型、不推送或发布。现有归档及历史验收报告不重新归档或冒称发布。旧报告/真实权限、外发、迁移验收不在本变更内。

## 待确认与验证边界

方案不截短完整提问，也不截断真实业务引用ID。需要最小测试确认根预留与子分配都从共享映射获益；Unicode以rune计数匹配PG字符上限。待用户确认本次具体proposal/design/tasks后进入RED测试与实施。
