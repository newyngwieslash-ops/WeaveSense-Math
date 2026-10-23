# Handoff：F04测试会话与Windows边界工件待确认

## Session Metadata

时间：2026-10-23 16:59:40 +08:00，Asia/Shanghai。HEAD `1941f6a91f6f36af0256acdcfdf69a1d5ea9edd9`，当前分支 `codex/fix-workspace-member-controls-and-analyst-permissions`。本轮仅Shape调查与工件；无产品/测试源码修改、正式RED、构建、部署、提交、归档、模型、子代理、新聊天或推送合并发布。旧修改与原数据保持。

## Handoff Chain

Continues from：[F03只读与F04分组交接](2026-10-23-164611-f03-forensics-f04-baseline-handoff.md)。用户随后明确请求通过/comet新建聚焦变更，先修F04测试间会话状态污染和Windows执行边界；保留当前分支，不放宽生产鉴权、不删除跳过测试。

## Current State Summary

`comet workflow resolve . --activate --json`选择Native；新建 `fix-test-session-isolation-and-windows-execution`，isolation=current、未新建分支/目录，language=zh-CN。brief、完整测试合同规格与四项设计任务已完成；Runtime prepare-shape-confirmation实际退出0，stateVersion2、phase=shape、status=await-user、七项验收pending。尚未获本次完整Shape确认，不运行confirmed或提前修改实现。

## Important Context

范围是测试基础设施，不是放宽生产身份校验。已有会话专项后成员API最小顺序失败，五组API单跑通过；这些是前轮调查证据，本轮正式RED须在确认后补充留日志。当前默认bash指向System32的WSL启动器，--version实际退出1/E_ACCESSDENIED；本地Git Bash存在，但fixture完整执行仍待Build核实，不能把shell失败当正确拒绝。

原F03恢复继续阻塞，5历史回执不足以确认本地基线；不刷新、清缓存或强制回写原图。旧全Web2234pass/40fail/2errors是上轮实际基线，本轮不重复运行或称已修。其他静态UI合同漂移不纳入本轮。

## Codebase Understanding

## Architecture Overview

测试改window/storage→UserStore/auth缓存→user-scope固定scope/revision/checking/AbortController→http与Agent SSE守卫；清理必须覆盖测试自身的状态，不改生产bypass。文件入口从URL转本机路径，允许名单规范化分隔符；临时模块依赖、welcome LF/CRLF与CI Bash/mock解析是独立执行边界，保留真实业务和负向断言。

## Critical Files

| 文件 | 用途 |
| --- | --- |
| `docs/superpowers/comet/changes/fix-test-session-isolation-and-windows-execution/brief.md` | 七项验收、范围/非目标及决策 |
| `docs/superpowers/comet/changes/fix-test-session-isolation-and-windows-execution/specs/test-session-windows-execution/spec.md` | 完整测试执行合同 |
| `docs/superpowers/plans/2026-10-23-fix-test-session-isolation-and-windows-execution.md` | 四项任务，均未执行 |
| `web/test/cross-tab-session-cache.test.ts` | 会话污染最小前序，追踪其准备/清理 |
| `web/src/lib/user-scope.ts` | 真实安全合同，禁止本轮修改 |

## Work Completed

### Tasks Finished

- 按用户指定current隔离建立单一Native变更，不重复询问已选择的分支。
- 完整读取相关入口/调用及相邻helper，复用fileURLToPath/node:path和已有source-contract/module-group-source的可行边界明确；不增加框架或依赖。
- 工件具体化：同进程会话组合、Windows路径/换行、临时Agent依赖、CI本地fixture及可验证mock命中；正式RED后修复，专项/build/lint与全Web真实债务记录。
- Runtime已保存完整Shape并等待确认；旧归档的Verifier限制接受和交付授权不继承。

## Files Modified

仅本变更brief/规格、Runtime状态、普通设计任务、本交接及pending入口。前轮两份runbook、F04报告、旧pending内容及所有其他dirty/untracked保留；没有暂存/提交，不git add .。本轮未操作浏览器、容器或数据库。

## Pending Work

## Immediate Next Steps

1. 用户明确确认本次完整Shape后，按最新Runtime confirm-shape命令执行，保留expected-state-version2/expected-action confirm-shape；不能用普通“继续”推定全部工件已确认。
2. 进入Build后先读formal brief/spec/七验收，执行T1正式最小RED；引用旧调查不替代新增失败证据。实现只触及web/test及必要普通文档，生产鉴权/API/RLS/发布脚本不改。
3. T2修测试会话边界，T3修Windows执行边界及CI mock；负向用例保留，禁止用每文件进程隔离代替组合GREEN。需要生产行为变更时回Shape。
4. T4候选交Runtime相关专项/build/lint，全Web保存原始退出码与逐名差异；范围外旧债列明。无独立Verifier如实记录并等待本轮结果接受，不能沿用旧接受；归档/提交另待明确选择。

### Blockers/Open Questions

唯一当前用户确认点为完整Shape，不是新的隔离或执行方式选择。会话最小根因、Git Bash完整fixture语义及换行依赖由Build验证；Linux真实CI未执行不得冒称通过。原F03、其他U/A/G门槛保持。

## Environment State

### Active Processes

18088沿用前轮隔离环境，本轮未探测或操作容器/浏览器，不重启或部署。宿主只检查bash可执行位置/退出状态，无dev server。任务记忆检索一次长期无输出已中断，退出1，未冒称成功；不据此阻断已建立Shape或反复重试。

### Environment Variables

未读取或输出凭据。只读环境模板与本地命令位置，不查看env值；CANVAS_BACKEND_DATA_DIR使用约束继续有效。CI后续只准自建mock，不触发真实docker/gh远端操作。

## Decisions Made

用户明确当前分支继续、不放宽生产鉴权、不删除跳过测试、不清缓存/模型/子代理/交付。单一聚焦变更、主会话TDD，完整工件须经本轮最终确认；本次Shape不授权提交或归档。

## Assumptions Made

没有新的PRD来源或远端核对。文档和旧日志仅作调试证据，不承接整个PRD/40工作包。正式检查与Linux可用性尚未证明，七项验收均pending。

## Potential Gotchas

CI错误拒绝测试的nonzero可能只是WSL启动失败；必须证明mock实际命中，不能删case或只断言nonzero。临时Agent副本不得以恒成功session替身掩盖真实鉴权；scope漂移拒绝仍应保留。静态源码格式helper只能消除格式差异，不能降低业务期望。Runtime状态不手工编辑。

## Related Resources

- [本轮具体brief](../superpowers/comet/changes/fix-test-session-isolation-and-windows-execution/brief.md)
- [本轮任务](../superpowers/plans/2026-10-23-fix-test-session-isolation-and-windows-execution.md)
- [原分组证据](2026-10-23-f03-forensics-f04-baseline.md)
- [待验收账](pending-acceptance.md)
