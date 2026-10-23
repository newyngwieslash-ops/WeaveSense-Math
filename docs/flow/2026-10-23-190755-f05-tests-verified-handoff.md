# Handoff：F05原八失败消除，全Web通过，有限结果待接受

## Session Metadata

时间：2026-10-23 19:07:55 +08:00，Asia/Shanghai；整理至19:11。项目WeaveSense-Flow，HEAD `b7d6f5d486baa7c7eda50d5c58f154179cecd1a5`，分支 `codex/fix-workspace-member-controls-and-analyst-permissions`。用户确认本次完整Shape后进入Build。保留当前分支、全部旧修改和数据，未提交、归档、部署、清缓存、调用模型、启动子代理/新聊天或推送/合并/发布。

## Handoff Chain

Continues from：[F05工件待确认交接](2026-10-23-185115-f05-shape-ready-handoff.md)。用户本轮已明确确认完整工件并要求先最小RED再修复，不重复询问Shape/隔离。旧F04已归档，其结果接受不适用于本次。

## Current State Summary

Native `fix-canvas-source-contract-portability` 已实际RED→GREEN，仅8份测试/helper文件变化。Runtime正式35文件369通过/2286断言、构建/lint退出0。完整Web实际2320通过/0失败/0错误、17993断言、320文件/48.21秒，原8失败全部消除、新增0，所有既有测试保持，不把过时合同改成生产行为。

当前Verify/await-user、stateVersion6、verification_result=blocked，六项验收pending。候选 `6ec9dc80-a708-436a-ba1a-f6f40713f6d1`，iteration1/attempt1。用户禁止子代理/新聊天，没有获授权可用独立Verifier；dispatch仅登记且复用有效检查，未启动独立审查，已如实回报verifier-unavailable。需要本次明确接受这一有限结果，普通“继续”不是接受。结果接受不含提交、归档或部署。

## Important Context

7项原失败是源码读取CRLF导致固定LF字符串/函数切片不命中，现行产品语义原本存在；第8项旧菜单固定9项，而已归档低代码实现增加DataSource/Logic至11项。本次只规范测试读取CRLF→LF，保留其他字节及原业务断言；菜单严格11项完整标题/描述/创建/禁用类型映射，实际组件SSR有/无绘图11/10项、禁用说明通过，不改成宽松计数。

新增回归复用原8个具名回调，读取真实源码临时副本，LF/CRLF基线先通过，再验证12种语义变异被原断言拒绝。中文空格路径/file URL、BOM/孤立CR/异常路径及分组成员顺序均实测。所有临时清理限测试自有且校验绝对父目录/前缀；没有真实缓存或用户数据清理。

## Decisions Made

当前分支/目录继续，主会话TDD，不启动子代理。现有helper共享读取修一次，无新依赖，不用全空白折叠code()。不修改web/src/backend/services/.github/鉴权/API/表/RLS或依赖锁文件。六项流程通过需本次明确接受无独立Verifier结果，之后提交/归档仍单独授权，不能沿用F04接受。

## Assumptions Made

没有联网核对远端。没有新模型/外发/生产迁移授权。18088未部署或操作，浏览器/PG/容器指纹本轮未重验。完整PRD原文仍缺，以196项审计明细和现行规格追溯；全Web通过不等于完整PRD/真实环境/发布通过。

## Codebase Understanding

## Architecture Overview

原源码合同通过直接读取和moduleGroupSource读真实模块组；readSourceText只规范CRLF，moduleGroupSource和六测试复用。17个文件rg命中含helper自身，实际16个测试消费者，全部进入35文件并集。生产链路的键盘、媒体、引用、样式和菜单不变；新回归仅提取已知具名测试块并用Bun.Transpiler执行，边界不匹配即失败，不是通用JS解析器。

## Critical Files

| 文件 | 用途 |
| --- | --- |
| `docs/superpowers/comet/changes/fix-canvas-source-contract-portability/brief.md` | 六验收及保护/范围 |
| `docs/superpowers/comet/changes/fix-canvas-source-contract-portability/specs/canvas-source-contract-portability/spec.md` | 完整目标合同 |
| `docs/superpowers/plans/2026-10-23-fix-canvas-source-contract-portability.md` | 实施完成，结果接受待办 |
| `docs/superpowers/reports/2026-10-23-fix-canvas-source-contract-portability-verify.md` | 实际检查/过程错误与限制 |
| `web/test/helpers/module-group-source.ts` | 共享只规范EOL读取 |
| `web/test/source-contract-portability.test.ts` | 原断言LF/CRLF与12种变异控制 |
| `web/test/canvas-connection-create-menu.test.ts` | 精确菜单映射、真实SSR、原守卫 |

## Work Completed

- 确认Shape后先新增正式RED2通过/12失败/30断言，退出1；不是引用此前60/8调查。
- 共享helper增加readSourceText，原六测试接入；保留全部原业务断言，菜单9改为已实现11并强化映射。新增11回归及真实SSR1项，无删跳过测试。
- 开发中修正负向控制先验证基线，避免环境错误冒充拒绝；另修新增测试对TypeScript7 Compiler API的错误假设，移除依赖引用。均已在最终通过前修复，不记作产品缺陷或忽略失败。
- 最终35文件组合369通过/2286断言；Runtime同专项约16.57秒、build约11.40秒、lint约21.99秒均退出0。dispatch reuse同候选回执，未无故重跑有效检查。
- 完整bun test一次退出0，2320通过/0失败/0错误、17993断言；8去重失败消除、新增0。报告/任务/pending更新。7份其他旧tracked文件字节哈希复核一致，暂存区空。

## Files Modified

现有7文件：module-group-source、canvas-keyboard、canvas-image-upload、canvas-media-performance、canvas-resource-mention-editor、workspace-menu-density、canvas-connection-create-menu；新增source-contract-portability.test.ts。另有本次工件、报告/任务/交接与pending增补。旧Agent面板、AGENTS/.gitignore、runbook、旧归档.comet.yaml、历史handoff/审计/CSV/中文参考等保持，不git add .。

## Pending Work

本次无独立Verifier结果接受仍待；六项不能提前写通过。原F03恢复缺可信基线仍阻塞，不覆盖恢复/清缓存。Linux真实CI、真实浏览器/媒体实机、U/A剩余及最后处理的离线/冷启动/独立Profile仍待；G01推送合并发布另授权。原全Web8失败已本轮消除，不继续登记为当前8失败。

## Immediate Next Steps

1. 用 `comet native select fix-canvas-source-contract-portability --json` 读取最新状态。当前v6等待本次无独立Verifier结果接受，不重建变更、不重复Shape或重跑有效测试。
2. 用户明确接受后才执行Runtime完整命令：`comet native next fix-canvas-source-contract-portability --summary <本次有限结果接受> --confirmed --expected-state-version 6 --expected-action confirm-verifier-unavailable`。版本变化先读状态，不用普通“继续”替代接受。
3. 按新的continuation执行归档预检；提交/归档需另行明确授权，不能以全Web通过或结果接受替代。当前未授权部署。
4. 后续按原交接继续U/A无凭据验收；原F03只读取证/可信差异恢复方案，离线等最后处理。若修产品先另建聚焦范围，不把本次测试合同变更扩大。

## Environment State

### Active Processes

本轮仅Bun测试/构建/lint及Comet流程命令，已结束；没有dev server、后端、浏览器、容器、数据库或子代理操作。18088保留前轮部署数据，本轮未实机验收或重读保护指纹。Shape记忆检索长期无输出已中断，不冒称应用过未返回的记忆；当前任务尚未流程完成，不调用task --complete。

### Environment Variables

未读取env值或凭据，无模型调用。宿主后端仍须显式CANVAS_BACKEND_DATA_DIR指向.local/project-workbench-debug，本轮未启动。

## Verification Evidence

Git忽略.local/f05-ui-contracts下red.log、green-dev.log、related-dev.log、full-web.log、failure-comparison.json、protected-hashes.json和related-files.json保存实际证据。Runtime日志目录.comet/runtime/native/changes/fix-canvas-source-contract-portability/logs/checks，三文件前缀a7f32314-ad4e-4dbd-8003-2a57066f0336-。正式检查全部通过但无独立语义审查，未伪造Verifier启动或结论。

## Potential Gotchas

当前六项pending属于独立审查限制接受门槛，不能把Runtime自动检查或全量测试替代该确认。新测试提取依赖8个已知具名块边界，维护其格式时需同步边界；不匹配会失败，不会放过错误。生产代码没有本轮变化，build不等于部署；旧Agent面板dirty仍在，不称纯HEAD镜像。原F03不恢复、不清缓存。

## Related Resources

- [验证报告](../superpowers/reports/2026-10-23-fix-canvas-source-contract-portability-verify.md)
- [任务](../superpowers/plans/2026-10-23-fix-canvas-source-contract-portability.md)
- [调查分类](2026-10-23-web-eight-failures-investigation.md)
- [前轮工件交接](2026-10-23-185115-f05-shape-ready-handoff.md)
- [待验收账](pending-acceptance.md)
- [浏览器后置待办](todo-browser-offline-cold-context.md)
