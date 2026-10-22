# Handoff：预览覆盖命中修复已本地归档

## Session Metadata

时间：2026-10-22 22:51:22 +08:00创建，22:54补齐，Asia/Shanghai。项目WeaveSense-Flow，分支`codex/fix-workspace-member-controls-and-analyst-permissions`。实现与归档提交`1ccf8fa87ebb3dd41cdaee9b2b19e9888a99e743`，此前基线`e111f1413a6a7062d33331b16a03b989192f4db8`。本交接将另作收尾文档提交，最终HEAD以git核对。无推送、合并、发布、清缓存、模型、子代理或新聊天。

## Handoff Chain

Continues from：[修复与真实复验交接](2026-10-22-223649-preview-overlay-verified-handoff.md)，更新其中待归档状态；前承[Shape交接](2026-10-22-220404-preview-overlay-shape-ready-handoff.md)。TSV/JSONL、旧Outbox等归档不重复开发。

## Current State Summary

用户先明确确认Shape，再接受本次没有独立Verifier的结果，最后单独确认本地归档并保留分支。实际archive退出0，Native stateVersion8、status=done、archived=true、6/6通过、workspaceFinishResult=kept；主规格canvas-node-overlay-hit-targets已创建，工件移至docs/superpowers/comet/archive/2026-10-22-fix-canvas-preview-overlay-hit-target。14个本轮文件精确提交为1ccf8fa8，混合文档仅新增本轮段，全部旧dirty/untracked及数据保留。OpenSpec Classic list为0活跃，Native列表四个变更均done，无活跃Native。

## Important Context

归档已完成，不再询问旧选择、重跑archive、重新Shape或修同一问题。独立语义审查未执行：平台有子代理能力，但用户禁止子代理/新聊天，已明确接受本次限制；Native通过理由是user-confirmed-degraded，不代表独立Verifier通过。构建含旧Agent面板dirty，非纯HEAD镜像。

## Decisions Made

当前目录与当前分支继续，原生CSS最小共享修复：轨道默认穿透，仅真实媒体子控件恢复auto，隐藏主图保持none；有可见轨道且full的数据源/逻辑容器pt-8，无持久尺寸、API、表结构、RLS、解析器或依赖变化。归档仅本地提交，不触发G01交付。

## Assumptions Made

没有联网确认远端。全Web旧失败名称完全相同只说明名称对照，非干净HEAD或全部根因对照；完整PRD原文仍缺，以196项审计明细/现行规格作可追溯基线。fixture与本地图层合成不代表实采或模型调用。

## Codebase Understanding

## Architecture Overview

CanvasProjectWorldLayers→CanvasNode→showStatusTrack。原编号span虽pointer-events-none，外轨道仍截获下层预览中心；数据源状态切换正常。共享修复一次涵盖编号/锁、存储、图片组、主图和图层入口，未复制到各节点解析器。

## Critical Files

| 文件 | 用途 |
|---|---|
| `web/src/components/canvas/canvas-node.tsx` | 轨道穿透、条件留位及交互边界 |
| `web/src/components/canvas/canvas-image-layer-controls.tsx` | 图层触发按钮auto |
| `web/test/canvas-node-overlay-hit-targets.test.tsx` | 4项渲染合同检查 |
| `docs/superpowers/comet/archive/2026-10-22-fix-canvas-preview-overlay-hit-target/comet-state.yaml` | done/archived/6项通过及用户限制接受 |
| `docs/superpowers/reports/2026-10-22-fix-canvas-preview-overlay-hit-target-verify.md` | 完整真实UI/PG、命令、失败名称和限制 |

## Work Completed

### Tasks Finished

- 正式RED 0通过4失败8断言→最小GREEN 4通过43断言；两个产品组件变化5行删除/5行替换级别，无新生产helper。
- Runtime五文件专项exit0/4413ms、TypeScript/Vite构建exit0/17232ms、lint exit0/30983ms；候选7b618d96-16ee-497c-a165-35a52b7e15cd，dispatch复用检查。
- 全Web2218通过/40失败/2错误、17235断言、2258测试/317文件；38个去重失败名称与TSV轮无增减，报告逐名登记，未称全量通过。
- 18088真实首次中心双向点击、Enter/Space、55%/63%/65%缩放；编号/顶栏不重叠，图片组展开/收起、可见主图选择、隐藏主图none、图层面板开关、存储说明title/命中核对。
- 刷新及经列表重开、PG配置核对通过；原十一图、10实采文档/1报告、零模型任务/预算与非web容器指纹保持。
- 用户分别接受限制和确认归档，实际archive、精确暂存/提交与最终只读status已完成。git diff --cached为空；diff --check通过，仅CRLF提示。

## Files Modified

1ccf8fa8仅本次两个组件/测试、功能/代码地图、本次pending区段、计划/报告、四个归档文件和新主规格，共14文件。Git忽略的既有todo.mdx也仅在本地更新本轮验收口径，未将其旧全文强制提交。旧Agent面板、AGENTS、.gitignore、旧归档配置及历史本地参考均未纳入。

两个混合文档通过HEAD正文加本轮块构造暂存blob，工作区旧内容不改；首次Windows text管道导致CRLF暂存扩大，提交前改用字节管道修正，最终pending-test仅6行新增，pending-acceptance仅4行新增。没有reset/clean/restore或git add .。

## Pending Work

### Blockers/Open Questions

发布门槛未通过：全Web旧40失败/2错误；完整U03 Skill库→节点/Agent执行、直接端点重连、规模边界；U04/U05真实模型/MCP/审批/SSE/未知恢复；旧库/PG升级、联合对象恢复、监控完整双target告警、实采及外发。按依赖继续，不把本次UI修复或归档当整体发布通过。

### Deferred Items

真正Offline/冷启动/独立Profile按用户决定最后处理；浅色/所有尺寸和缩放、原生title气泡屏幕证据、全部媒体生成/下载/另存均未由本轮解除。G01 push/merge/release另待明确授权，O01–O06不实现。缺凭据只列变量名及本地配置步骤，禁止模型继续有效。

## Immediate Next Steps

1. 核对git status、HEAD、branch、openspec list与本归档状态；归档已done，不重复执行或重新开工。归档后用`comet native status fix-canvas-preview-overlay-hit-target --json`读取，不能用select恢复已移走的活跃目录。
2. 按原交接继续无模型U/A验收，完整Skill路径/其余无凭据边界先核实；浏览器离线后置。新真实缺陷先证据再新聚焦变更。
3. 保护旧dirty/untracked、原十三图及实采数据，不清缓存、不覆盖未知编辑。交付仍须单独授权。

## Environment State

### Tools/Services Used

PowerShell、Bun1.3.6、Comet Native、PG17/Docker、既有CodeGraph只读，无新索引；CUA真实UI/read-only DOM，不使用隐藏store或SQL写入。

### Running Processes

flow-uat-20261020在18088继续运行，8088未动。仅web镜像flow-uat-web:preview-overlay-20261022，digest48c1c3e060e0b508e0d9255aebcccdb58e514cf725145adab557d5508bd06b77，标签e111f141-preview-overlay-working-tree，含旧dirty。保留原TSV回退镜像7e0654b95afb356d5bca32d7caac94da978ca2723c925097755742e8052db6fc及全部overlay；platform/PG/Engine/scheduler未重启，无迁移/dev server。

CUA tab2保留在127.0.0.1:18088/canvas/Ia3U3_-fRwrhmfgqxdflH，测试owner登录，源码按钮表示当前结构化预览；已markHandoff，下轮重标。tab1原会话未操作。

### Environment Variables

仅名称：CANVAS_BACKEND_DATA_DIR、PLATFORM_DB_PASSWORD、INTEL_DB_PASSWORD、POSTGRES_SUPERUSER_PASSWORD、FLOW_MODEL_GATEWAY_KEY。值保留本地忽略文件，不输出/提交。宿主后端若需要必须显式.local/project-workbench-debug，不用backend/data。

## Potential Gotchas

新增两图均保留：7CXWq6qHNQ-TTsVZXQUJK为预览图（媒体初样本SVG/缺batchChildIds无效，不计媒体通过）；Ia3U3_-fRwrhmfgqxdflH为有效PNG/完整batch补验图。两图8节点1边、PG revision2/6；第二图primaryImageId=media-child-2，source四条/480×360/位置0,0保持。页面截图同步指示仍在，持久配置结论依据PG/重开，不声称媒体全同步结束。

Native归档后select返回ENOENT（活跃目录已移走），doctor误报active change需要migration；未执行repair、不重建目录。支持归档读取的native status实际exit0、done/archived/6通过/无阻塞，归档元数据及主规格已提交。Classic comet guard不适用Native，不伪称执行Classic guard。

Comet项目知识已保存可复用轨道命中经验（memo-f57559b6）；task --complete接口等待无输出，停止本次调用，未伪称记账成功。学习检查未发现新个人偏好，不写个人任务记忆；该辅助接口失败不阻塞归档。

## Related Resources

- [完整验证报告](../superpowers/reports/2026-10-22-fix-canvas-preview-overlay-hit-target-verify.md)
- [设计与四任务](../superpowers/plans/2026-10-22-fix-canvas-preview-overlay-hit-target.md)
- [归档状态](../superpowers/comet/archive/2026-10-22-fix-canvas-preview-overlay-hit-target/comet-state.yaml)
- [待验收账](pending-acceptance.md)、[浏览器后置待办](todo-browser-offline-cold-context.md)

全部临时日志/样本/PG快照/部署与回退文件在.local/flow-uat-20261020/preview-overlay-fix，真实截图ui-proof.jpg已保存并可展示，不提交。历史861项是验收登记，不是待开发功能数。
