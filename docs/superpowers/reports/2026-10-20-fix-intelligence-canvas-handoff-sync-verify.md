# 已有画布情报交接同步修复验证

时间：2026-10-20，Asia/Shanghai。分支 `codex/fix-intelligence-canvas-handoff-sync`，代码起点69accb65。用户已确认本变更工件及新分支；不启动子代理、不推送发布。所有本地原有修改保留。

## 根因与覆盖

`loadCanvasProjectForEditing` 在dirty且本地不同于remote时，把remote写入acknowledgedProjects却返回并保留旧local。下一次 `buildCanvasOperations(remote, local)` 因而把云端独有节点生成remove_node。最小检查通过真实共享函数+可控http边界确认：第一次RED为加载返回只有original；第二次RED实际PATCH包含删除intel-handoff与本地标题set_field，退出1，排除仅渲染问题。

修复在共享加载入口复用mergeAgentCanvasEditor，可信旧基线用于三方合并，remote与本地投影分开保存。没有可信基线或同字段冲突时不推进远端基线、不覆盖local。原requireUnchangedLocal/session epoch守卫保留，pending投影才安排保存。

调用方已追踪：画布生命周期、画布列表导出/预载、canvas-project-card、canvas-folder-card、creation-canvas-conversation、canvas-generation-task-sync及canvas-generation-consumer。refreshCanvasAfterAgent已有对应合并机制，本次复用；syncRemoteUserData聚合恢复静态有相似线索，但当前全仓未找到外部调用，只有自身失败重试，不为未复现线索扩大修复。

## 实际检查

- `web`工作目录 `bun test test/canvas-handoff-sync.test.ts`：RED 0通过/1失败（分别确认加载缺失和误发删除）；修复后1通过/0失败。
- 扩展6个边界后，`bun test test/canvas-handoff-sync.test.ts test/canvas-operations-build.test.ts test/canvas-persistence-loss.test.ts test/agent-canvas-sync.test.ts`：27通过/0失败，81断言。真实发送的操作列表断言只有本地标题更改；显式本地删除原节点仍可发送，云端新增保留。
- `bun run --cwd web build`：实际执行tsc --noEmit与Vite，退出0，Vite10.93s，保留大chunk警告。
- 一次错误的Comet命令 `bun --cwd web run build`只输出Bun帮助却退出0；该条记录**无效，不作为构建通过证据**，后续必须记录实际构建命令。

## 18088真实复验（23:53–23:56 +08:00）

仅隔离web重建成功（日志 `.local/flow-uat-20261020/web-hotfix-build.log`）；浏览器及容器入口同为 `index-CWWXyWVj.js`。前端包含他人未提交的canvas-cloud-agent-panel修改，不称为纯HEAD构建。

已有画布 `canvas_5158db1cc61dae10566679a8`：完整新标签打开后3节点可见；同一会话投送后4节点，创建数据源连线后5节点/1连线，再次投送唯一标题“U02 hotfix 连线保留复验”后6节点/原连线保留。刷新后先适应画布，DOM与UI显示6节点；PG只读查询revision13/6节点/1连线，指定新节点及原连线均true；后续查询revision14/6/1仍存在。原连线ID `Oel0tjAi3pF-NmhR4RiZf`，交接ID `intel_node_d54ec6430dd080cc2fe87c5c`。

同一专题新画布对照 `canvas_b58dddabd32d632a1bbbbd3e`：1节点、正文和来源脚注可见，刷新后保留；PG revision3/1节点/0连线、交接节点存在。自动唤起Agent关闭，全程无模型或外发。

旧U03画布缺少可信基线，UI保留原两节点且同步错误；PG仍保留交接节点与原连线。按设计保护本地，不清缓存、不猜测恢复，该历史脏缓存不算正常合并验收通过。原tab1还曾在已同步状态显示旧内容，数据库节点存在；新完整标签可正常打开并后续连续投送通过，无法据此证实额外代码缺陷，保留观察。视口裁切会使DOM暂不呈现屏外节点，刷新后必须适应画布再比较。

真实截图位于Git忽略目录 `.local/flow-uat-20261020/screenshots/`：`u02-hotfix-reopen-three.jpg`、`u02-hotfix-existing-six-with-link.jpg`、`u02-hotfix-refreshed-six-with-link.jpg`、`u02-hotfix-new-canvas.jpg`、`u02-hotfix-new-canvas-refreshed.jpg`。

扩大专项实际通过Comet运行器执行5文件：`bun test --cwd web test/canvas-handoff-sync.test.ts test/canvas-operations-build.test.ts test/canvas-persistence-loss.test.ts test/agent-canvas-sync.test.ts test/agent-canvas-patch.test.ts`，38通过/0失败/114断言，退出0，日志5b003e22-b459-436b-9a6b-e08a2e87540f。此前误填不存在的agent-canvas-merge文件仅运行4文件27测试，不计为扩大专项。

不把旧94/46等历史结果当本轮结果。API、表和权限合同未改；自动审查按本次hotfix review_mode=off跳过，人工核对共享入口、全部调用方、session epoch和冲突守卫。U01–U05和A/G门槛仍部分未完成。

未执行Go/Engine/PG迁移测试：没有后端、API、表或依赖变化。本检查使用内存存储与可控HTTP返回，是自动回归，不是live采集/MCP/模型；真实UI另记。U01–U05和A/G门槛不会批量勾选。

## Comet正式验证

2026-10-21 00:04 +08:00继续收口。build guard首次因文档更新使旧输入证据失效而拒绝推进；重新通过运行器实际执行 `bun run --cwd web build`，退出0（7515ab91-4f37-40e3-bec5-e9d8eaeeb270），随后build guard 14/14通过，phase进入verify、check_epoch=2。这不是产品构建失败，不隐瞒旧证据失效。

| 检查 | 结果与依据 |
|---|---|
| 完整性 | 3/3任务已完成，openspec instructions apply为all_done；源码改动仅1共享入口+1测试文件 |
| 正确性 | 实际RED→GREEN；6边界与38专项通过；真实已有/新画布UI及PG存在性见上文 |
| 一致性 | 与design四项决定一致，复用既有合并、保存两份不同语义快照、保留冲突和会话守卫，无新同步协议 |
| 构建 | 实际tsc与Vite退出0；大chunk提示为已有构建提示，不在本次改动范围 |
| 安全和归属 | 无新依赖/密钥/权限放宽/迁移；不提交.env、截图或本地数据；他人面板和旧归档修改明确归属并按用户授权保留 |
| 审查策略 | 本变更review_mode=off，跳过自动审查和子代理；人工复核本变更diff、调用方、冲突及session epoch |

按提交范围补充完整性/正确性/一致性核对，而非仅按工作区剩余脏文件估计范围。无delta specs（明确skip_specs，恢复现行行为），不做虚构的delta场景覆盖；hotfix无独立Superpowers Design Doc/Plan，按已确认的proposal/design/tasks核对。本次范围未发现需修复的CRITICAL/IMPORTANT或待接受的行为偏差。旧缓存恢复、原tab观察及完整U/A/G是已明确未验边界，不能升级为通过结论。

verify阶段需独立运行器证据与guard，最终状态以本变更.comet.yaml为准；归档还需本hotfix的单独最终确认，未推送发布。
