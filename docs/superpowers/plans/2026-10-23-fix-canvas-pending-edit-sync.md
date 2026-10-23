# F03合法编辑未同步：设计与任务

时间：2026-10-23，Asia/Shanghai。Native变更 `fix-canvas-pending-edit-sync`，用户先明确接受第二候选无独立Verifier的有限结果，再独立确认本地归档并保留分支。实际archive退出0，stateVersion12、done/archived=true、工作区kept；原F03恢复继续登记阻塞。基线 `8de18f8a`，继续当前目录/分支。

## 事实和待查边界

旧材料：`docs/flow/2026-10-23-u03-ports-fields-acceptance.md`，保留其全部原材料。当前新增只读PG核对仍revision5/`*`；重新打开127.0.0.1字段图，DOM合法值`^深层甲$`、True1/False3、云端暂未同步，warn/error为空。原localhost情报画布未操作。

访问日志显示坏值保存最后一次PATCH在08:25:37；08:25:58、08:30:35及本轮10:53:03加载有GET304，没有后续operations PATCH。GET304只证明版本缓存命中，不能证明合法编辑落库；尚不能据此唯一确认错误根因或说请求必被后端拒绝。

共享链路：`LogicNodeContent.patchLogic`→节点更新→`useCanvasProjectLifecycle`观察并更新CanvasStore→`installRemoteUserDataAutoSync`→`scheduleRemoteUserDataSync`→`saveRemoteUserDataNow`→等待加载/串行锁→`saveRemoteUserDataBatch`→journal准备或重放→`applyRemoteCanvasOperations`→服务端同事务画布+receipt→GET确认→更新基线/投影→剩余编辑继续提交。另检查用户scope初始化、加载错误及本地持久化队列是否在发出PATCH前阻断。

已检索全部直接调用：画布生命周期/列表/卡片/文件夹、项目素材、素材库/上传/选择器、previs、creation入口/对话、Agent保存/审批、project-asset-sync、用户会话初始化及同步服务内部加载/刷新/删除。实施前按实际受影响共享函数继续读取调用上下文，不逐调用方重复补救。

## 最小设计

先用真实共享函数、内存HTTP替身和独立测试scope重建“坏值已确认→合法编辑→离页/加载/保存”的状态顺序，记录是否发出请求及抛错来源。对未确认原请求与新编辑分开建模验证，但沿用现有journal、三方合并、串行锁及API，不新建框架。

只有RED证实的共享边界进入修复。回执只确认其对应请求，后续编辑继续保存；冲突/缺基线保持拒绝，不把当前远端猜成旧基线。若根因不在预期入口，记录反证后在同一F03范围调整位置；需要改变API/表结构/权限/恢复语义时回Shape。

## Tasks

- [x] T1：16原图/关键表/五容器保护快照；两例加载RED、Agent刷新RED及显式直存RED均在产品相应修复前观测。明确已证实共享边界，未宣称所有历史现场根因唯一（A1）。
- [x] T2：精确哈希匹配恢复基线；初始化不将脏缓存当已确认，直存复用锁内共享加载；原pending仍先重放。16新增检查79断言GREEN，包含另一坏图不阻塞好图（A2–A4）。
- [x] T3：Runtime64项221断言、构建/lint通过；全Web2234pass/原40fail/2errors，38失败名称相同，明细见报告。未独立Verifier，不伪造通过（A5）。
- [ ] T4：18088最终r2部署、新图坏值→合法值、重开/普通标题/PG及原数据指纹通过；原F03仍报缺可信基线、PG5/*、本地合法值保持，因此原现场收口仍阻塞。报告与新交接已保存（A6部分通过）。

## 检查计划及约束

现有参考测试为 `web/test/canvas-handoff-sync.test.ts`、`canvas-operations-build.test.ts`、`canvas-persistence-loss.test.ts` 和会话隔离专项。新失败测试按定位选最少文件，不在Shape提前实施。测试替身不写真实数据，真实UI和PG另记；最终正式检查交Runtime运行，避免同候选重复完整构建。

原16画布、10实采文档/1报告、模型任务/预算和容器记录重新只读快照，原F03与新图单独列允许的普通保存增量；不能用“允许F03变化”豁免其他字段或节点丢失。部署前读对应Compose/覆盖文件，保卷、不启dev server，不碰8088；发现不匹配先核对。

保留全部dirty/untracked和数据，不覆盖恢复、清缓存、模型、子代理、新聊天或推送/合并/发布；仅本变更获本地归档/相关提交授权。已执行结果以任务和验证报告为准；用户接受有限结果不将T4原现场恢复改为通过。
