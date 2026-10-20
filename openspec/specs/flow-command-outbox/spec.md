# flow-command-outbox Specification

## Purpose
规定平台手动采集与研究命令的持久入队、幂等派发、租约所有权、回执及预算原子提交，以及未知执行结果的对账边界。

## Requirements

### Requirement: Platform transactional command outbox and async dispatcher (PRD §11.3)
平台 SHALL 将手动采集和研究命令、初始操作投影及可选预算预留在同一平台事务中持久化，以租户及幂等键去重并拒绝参数冲突。只有成功领取本次派发租约的执行者 SHALL 调用 Engine；回执及关联副作用 SHALL 校验本次领取版本，持久化失败 SHALL 向调用方报告，未知外部结果 SHALL 保留对账语义。

#### Scenario: Transactional enqueue and immediate dispatch
- **WHEN** REST 或 Agent 以幂等键提交手动采集或研究命令
- **THEN** 平台 SHALL 原子创建命令、投影及所需预算预留，成功领取后调用 Engine 并持久化回执
- **AND** 调用方处于外层事务时 SHALL 延后到该事务提交后由后台发送，受理回执 SHALL 保留真实排队状态；用户未给幂等键时 SHALL 转发平台已持久化的自动键

#### Scenario: Idempotent replay and parameter conflict detection
- **WHEN** 同一租户和幂等键再次提交
- **THEN** 平台 SHALL 对相同参数返回已存状态，对不同参数返回 409，不重复执行已完成命令

#### Scenario: Transient Engine failure and background retry dispatcher
- **WHEN** Engine 调用发生网络错误或 HTTP 5xx
- **THEN** 平台 SHALL 按现行有界退避策略写入 retry_wait，达到上限后进入 reconciliation_required；所有相关数据库错误 SHALL 传播

#### Scenario: 两个派发者竞争和旧响应迟到
- **WHEN** 两个派发者尝试领取同一命令，或旧派发者在所有权已失效后返回
- **THEN** 只有本次领取成功者 SHALL 发送，旧响应 SHALL 不覆盖新的回执、投影或预算状态

#### Scenario: 租约过期或回执持久化失败
- **WHEN** dispatching 租约过期，或 Engine 已返回但本地回执/投影/预算事务无法提交
- **THEN** 平台 SHALL 报告持久化失败或未知状态，保留资金和对账线索，禁止因该失败直接盲重发或向用户宣称成功

#### Scenario: 失败终态和资金变更原子性
- **WHEN** Engine 明确 4xx 拒绝或命令达到对账条件
- **THEN** 终态、对应投影和预算释放/未知标记 SHALL 原子提交，任一步数据库失败 SHALL 回滚并传播到 REST、Agent 或后台错误处理
