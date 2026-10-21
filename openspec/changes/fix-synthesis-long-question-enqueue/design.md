# 长提问入队设计（zh-CN）

## Context

见proposal.md的Why。数据链为研究工具researchSynthesis、内部简报summarize或同步Synthesize → StartSynthesis → ModelTasks.Enqueue → Billing.Reserve/Allocate → FlowBudgetReservation.RefID（varchar(80)）。Question还独立进入任务request.user和SynthesisContext.Question；这两个位置必须保留全文。RefID在预算中仅用于参考和账单备注，幂等仍以tenant+key定位，不依赖提问摘要。

## Goals / Non-Goals

目标是在现有80字符数据库约束下正确创建长问题合成任务，并保证完整提示、上下文、租户预算、父子分配和同键幂等不退化。只改共享StartSynthesis中Question到预算摘要的映射。

不扩大数据库字段、不迁移数据、不截断通用Billing的真实业务ID、不改默认模型token配置、不修改研究/简报各调用方、不新增抽象依赖。不处理未知结果/Outbox等已归档或独立问题。

## Decisions

复用现有truncRunes(req.Question, 80)生成RefID。字符计数采用rune而非字节，避免中文/emoji被截成无效UTF-8，符合PG varchar的字符计数。原问题继续完整写入request.user和Context；已有短提问不变。

不把RefID改成Key：Key上限160字符，比RefID更长，而且会改变短提问账单参考语义。不在Billing.Reserve/Allocate统一截断：会静默更改其他调用方的真实资源ID。不扩表：没有必要，预算摘要不需要承担完整提问存储职责。

## 最小验证方案

一个回归测试文件复用现有testDB、fund、fakeGW与pgTestDB，不新增框架。SQLite检查根预留和子分配、中文/英文/emoji的80及81字符边界；检查RefID等于80字符前缀、全文在request/context保留、同键task/reservation/账单不增、预算不足不留任务或预留。RED首先针对旧实现的真实持久映射断言，不能用手工截断测试输入绕过问题。

PG专项复用pgTestDB，创建隔离新测试库并按现有清理回调删除，仅测试该新库。旧实现长问题须出现22001，修复后入队成功；不调用RunTask或真实模型。实际执行时记录PG连接方式和真实运行/跳过/阻塞，不能将SQLite代替PG通过。

## Risks / Trade-offs

超过80字符的提问可能具有相同参考摘要；任务仍由租户+幂等键唯一定位，全文保留在上下文，不影响幂等。测试将验证共享前缀不同key仍各自入队，不把摘要当唯一标识。

原有dirty文档和其他前端改动保留。仅本hotfix片段可纳入后续精确提交，禁止git add .。PG测试与源码构建不触发真实模型调用；无浏览器和部署变更。

## Migration Plan

无表结构或API变更，无数据迁移。仅发布后新入队使用安全预算摘要；本轮不推送、部署或发布，也不重跑旧模型任务。若回退代码，长提问会再次受到原字段限制。
