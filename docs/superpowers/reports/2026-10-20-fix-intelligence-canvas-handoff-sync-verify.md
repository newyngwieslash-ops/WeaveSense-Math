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

## 待完成

18088仅前端重建、旧失败画布与新画布UI/PG复验、包含底层合并的扩大专项、Comet build/verify实际执行证据和阶段guard仍待本轮收口。不会把旧94/46等历史结果当本轮结果。

未执行Go/Engine/PG迁移测试：没有后端、API、表或依赖变化。本检查使用内存存储与可控HTTP返回，是自动回归，不是live采集/MCP/模型；真实UI另记。U01–U05和A/G门槛不会批量勾选。
