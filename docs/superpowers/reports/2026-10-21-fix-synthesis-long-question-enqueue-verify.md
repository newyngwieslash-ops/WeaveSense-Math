# 长提问入队 hotfix 验证

时间：2026-10-21，Asia/Shanghai。基线7f61c190，分支codex/fix-synthesis-long-question-enqueue。用户已确认新分支与proposal/design/tasks；禁止真实模型、子代理、推送发布，全程遵守。

## 根因与实现

StartSynthesis将完整Question传入预算RefID，FlowBudgetReservation.RefID为varchar(80)。共享调用方为Synthesize（同步）、tools_p3.go的researchSynthesis（父预算子分配）、routes_p5.go内部summarize（简报根预留）。RefID用于预算参考/账单备注，幂等按租户+key，非按RefID。唯一生产代码修改为RefID: truncRunes(req.Question, 80)，复用现有Unicode截取；提示和Context.Question全文不变。无API、表结构、依赖变更。

## 本轮实际执行

| 检查 | 结果 |
|---|---|
| SQLite RED：go test ./internal/flow -run '^TestSynthesisQuestionEnqueue$' -count=1 | 退出1，root/child英文、中文、emoji的81字符摘要实际保留81，应为80 |
| PG17 RED：交叉编译测试并在flow-uat-20261020-postgres-1执行TestSynthesisQuestionEnqueuePostgres | 退出1，新测试库实际SQLSTATE22001；不是环境失败 |
| 修复后同SQLite回归 | 退出0，12个root/child边界子场景通过 |
| 最终专项：go test ./internal/flow -run 'Test(SynthesisQuestionEnqueue\|Synthesize\|ModelTask\|Budget\|Brief)' -count=1 -json | 退出0，7个顶层测试通过；宿主PG项因无DSN跳过，未计通过 |
| 最终PG17专项 | 退出0，1个顶层/12个子场景通过，真实varchar约束；任务/预留计数、父分配、余额和账单幂等断言通过 |
| go build ./internal/flow | 退出0 |
| gofmt / git diff --check | 本次Go文件已格式化，差异无空白错误；已有文件CRLF提示不作功能失败 |

PG使用既有pgTestDB设施新建flow_rt_时间戳测试库，自动清理；初次GREEN后查询残留0，最终检查继续用相同自动清理。测试只入队，scriptedGW调用计数必须0，不执行RunTask。PG连接使用隔离容器已有超级用户，不将本次作为非超级用户RLS验收。未触碰原业务库和恢复库。

边界覆盖：短值、英文80/81、中文80/81、emoji81；根预留/子预算；完整request.user与Context.Question；同key复用原task/reservation、不增数量/账单/余额/父分配；相同摘要前缀不同key独立；预算不足无任务/预留/余额/父分配残留。

## 验证范围与保留项

Comet build guard14/14、verify guard5/5实际通过，verify_result=pass、phase=archive，等待最终归档确认。自动scale因10个提交文件计入工件而选full；经核对仅1行生产修改、1个测试文件及文档/工件、无delta或跨模块协调，按verify技能显式override为light。该选择只调整验证重量，不豁免专项或PG检查。最终PG测试库残留0。

轻量检查：任务3/3、改动与工件一致、Flow包构建通过、相关测试通过、无密钥或权限变更。review_mode=off，跳过自动代码审查/子代理，主会话已检查一行映射及所有调用方。没有delta spec，跳过规格深度漂移检查；不增加UI/API/DB测试范围。

未执行全量Go/前端构建、运行镜像替换、真实Agent/简报和模型验证，本次不需要且真实模型授权已耗尽。此前实采模型结果仅引用历史证据，不算本修复部署验证。完整权限、外发、PG16/旧库迁移、恢复/SSE/未知ACK与发布门槛继续待验。

已有pending-test与pending-acceptance历史增量、旧归档.comet.yaml、前端Agent面板与全部未跟踪文件保留。pending-test本次新增节单独暂存，其余旧内容不混入本提交。禁止git add .，仅精确暂存本hotfix文件。归档等待本次最终确认，不推送/合并/发布。
