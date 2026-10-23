# Handoff：F05八失败分类完成，测试合同工件待确认

## Session Metadata

时间：2026-10-23 18:51:15 +08:00，Asia/Shanghai。项目WeaveSense-Flow，HEAD `b7d6f5d486baa7c7eda50d5c58f154179cecd1a5`，分支 `codex/fix-workspace-member-controls-and-analyst-permissions`。用户保留当前分支、全部旧修改和数据；本轮只读调查及工件，未修改正式测试/产品源码、提交、归档、部署、清缓存、模型、子代理、新聊天或推送合并发布。

## Handoff Chain

Continues from：[F04归档收尾](2026-10-23-183735-f04-local-archive-handoff.md)。用户本轮要求先调查剩余8项，区分过时测试合同与真实缺陷，再按/comet准备聚焦工件，确认后先最小RED再修复。F04已归档，不重复执行或继承其结果接受。

## Current State Summary

新建Native `fix-canvas-source-contract-portability`，isolation=current，未新建分支/目录。brief、完整规格、四任务及调查报告已落盘，Runtime prepare-shape-confirmation退出0，phase=Shape、status=await-user、stateVersion2、六项验收pending。当前等待完整Shape明确确认，尚未运行confirmed、修改实现或补正式新RED。

原6失败文件组合实际60通过/8失败/419断言、68测试/3.37秒，退出1。只读内存对照7项失败断言raw=false、仅CRLF→LF后true；第8项固定9条而产品11条，新增DataSource/Logic可追溯到bfcf5b1b和现行规格。当前证据支持7个读取换行问题和1个过时计数，未证实真实生产缺陷。不把只读诊断当正式RED或完整GREEN。

## Important Context

七换行问题分别是键盘复制顺序、上传关闭顺序、RetainedCanvasImage函数切片、InactiveVideoPreview切片、syncInlineMentionPreviews切片、collection焦点及scoped菜单CSS固定LF。真实语义仍存在，但后续全部原断言及负向控制须实际执行，不能只修首断言就称全部通过。

连接菜单当前11项均有描述及创建类型；应加强为精确集合/映射，真实SSR有/无绘图11/10项并检查禁用说明/守卫，不能退回产品9项或改成“至少9项”。若确认后的检查暴露真实产品缺陷，先记录并重确认范围，不能偷偷扩成生产修复。

## Decisions Made

当前分支/目录继续，单一聚焦测试合同变更，无子代理。优先复用现有module-group-source读取；仅规范CRLF→LF，不用source-contract.code()全空白折叠。17条rg文件命中包含helper本身，实际16个测试消费者，均纳入专项/组合；四任务及六验收已具体化，用户当前请求不代替完整Shape最终确认。

## Assumptions Made

完整PRD原文仍缺失，本次只按交接限定8项及必要保护纪律，不把全PRD/40工作包承接为本变更。没有联网核对远端；原全Web2300通过/8失败/0错误属于F04引用，本轮未重跑全Web。未操作18088、浏览器、容器、PG或数据指纹。

## Codebase Understanding

## Architecture Overview

moduleGroupSource读取真实拆分组源码，当前保留原CRLF；直接readFileSync/Bun.file同样保留。源码测试固定LF或LF函数终点，造成假失败。产品链路：画布project→useCanvasKeyboard/CanvasConnectionCreateMenu，ImageContent→RetainedCanvasImage，VideoNodeContent→InactiveVideoPreview，多个编辑器→CanvasResourceMentionTextarea同值分支→syncInlineMentionPreviews。共享测试读取修一次，所有消费者回归；不改生产文件。

## Critical Files

| 文件 | 用途 |
| --- | --- |
| `docs/superpowers/comet/changes/fix-canvas-source-contract-portability/brief.md` | 八项来源映射、六验收、范围和保护 |
| `docs/superpowers/comet/changes/fix-canvas-source-contract-portability/specs/canvas-source-contract-portability/spec.md` | 完整目标合同 |
| `docs/superpowers/plans/2026-10-23-fix-canvas-source-contract-portability.md` | 四项任务，均未执行 |
| `docs/flow/2026-10-23-web-eight-failures-investigation.md` | 八失败逐项位置、证据及分类 |
| `web/test/helpers/module-group-source.ts` | 可复用共享读取及16测试消费者 |

## Work Completed

- 完整读取AGENTS及新交接，核对HEAD/分支/dirty；openspec list --json为0活跃，Comet路由Native，F04不重复归档。
- CodeGraph只读追踪关键入口/调用方；未建或刷新索引。原六文件诊断及只读EOL探针对照已记录，没有修改原测试或生产文件。
- 当前菜单11项标题/描述/类型逐项列证，历史提交和现行低代码规格支持新增两项。
- Native完整Shape与普通设计/任务准备好，已请求Runtime进入最终确认，六项pending。

## Files Modified

仅新Native brief/spec/Runtime工件，普通计划/调查/交接及两份pending增补；另有Git忽略.local/f05-ui-contracts诊断文件。原.gitignore/AGENTS、旧归档.comet.yaml、Agent面板、runbook、历史handoff/审计/CSV等全部保留。暂存区为空，无产品或正式测试变更。

## Pending Work

完整Shape用户确认后才能T1新正式RED→T2共享读取→T3精确菜单合同→T4相关/所有消费者/构建lint/全Web及文档。原8项尚未修复，不称全量通过。原F03缺基线恢复、Linux真实CI、U/A及离线最后处理和G01仍待；旧Verifier接受不适用本变更。

## Immediate Next Steps

1. 用户明确确认完整Shape后，按最新Runtime命令执行confirm-shape，当前guard为expected-state-version2、expected-action confirm-shape；普通“继续”不是完整确认。
2. 进入Build后先读brief/完整spec/六验收，新增最小RED并留日志；不能把本轮60/8诊断或内存探针当正式新RED。
3. 共享读取只规范CRLF，保留原断言/测试，并补语义负向控制；菜单严格11集合及真实SSR11/10，不改生产菜单或样式。
4. 相关/所有helper消费者及相邻F04组合后运行正式build/lint和一次全Web，记录退出码/实际/引用/未执行；真实产品缺陷若出现，登记并重确认范围。无独立Verifier时按本变更规则单独接受，提交/归档仍另授权。

## Environment State

### Active Processes

只运行原六文件Bun诊断和只读探针，均结束。Comet task初次检索长期无输出已中断退出1，不反复盲试，不称已应用未返回记忆。无浏览器、数据库、容器、dev server或子代理操作，18088保持前轮部署数据。

### Environment Variables

未读取凭据或env值，无模型调用。本轮无需后端；以后宿主后端仍必须显式CANVAS_BACKEND_DATA_DIR指向.local/project-workbench-debug。

## Verification Evidence

Git忽略.local/f05-ui-contracts/existing-six-files.log：实际exit1，60pass/8fail/419断言。eol-probe.mjs/json：只读原源，7个原失败断言在内存仅CRLF转LF成立，11菜单映射完整；不是整体原测试GREEN。正式新增RED、修改、build/lint、全Web、浏览器/实机媒体和LinuxCI本轮均未执行。

## Potential Gotchas

Runtime正式文件只编辑brief/spec，状态和verification.md不手改。生成state语言en不改变用户中文文档约定，brief/spec/说明已中文。17文件匹配含helper自身，不能报17个测试消费者。code()会删全空白，不适合本轮严格边界。无语义证据不改生产；源码对照成立不代替浏览器或真实产品验收。

## Related Resources

- [调查报告](2026-10-23-web-eight-failures-investigation.md)
- [具体任务](../superpowers/plans/2026-10-23-fix-canvas-source-contract-portability.md)
- [完整brief](../superpowers/comet/changes/fix-canvas-source-contract-portability/brief.md)
- [待验收账](pending-acceptance.md)
- [F04收尾](2026-10-23-183735-f04-local-archive-handoff.md)
