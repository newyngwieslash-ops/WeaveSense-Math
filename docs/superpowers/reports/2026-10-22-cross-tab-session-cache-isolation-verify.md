# 跨标签会话与目录缓存隔离修复验证

时间采用Asia/Shanghai，2026-10-22。当前分支 `codex/fix-workspace-member-controls-and-analyst-permissions`，起点 `d9b809eea20aeb61408d97b8c7424da2a017d9a7`。归档提交与最终HEAD以git及最新交接核对。

## 状态和边界

用户已确认Native工件、current隔离及先RED后GREEN。2026-10-22 10:37用户明确接受本次没有独立Verifier的验证结果，随后另行明确确认本地归档并保留当前分支。实际archive退出0，Runtime为Archive/done、state version8、archived=true、verification_result=pass、6/6验收通过，workspaceFinishResult=keep/kept。验收依据是用户接受不完整独立验证；**独立Verifier仍未启动，不代表独立语义审查通过**。

归档前dry-run实际ready=true、blockers=[]、requiresReverification=false。获单独授权后执行confirmed归档，新增主规格 `docs/superpowers/comet/specs/user-session-cache-isolation/spec.md`，四个工件移动到 `docs/superpowers/comet/archive/2026-10-22-fix-cross-tab-session-cache-isolation`。没有Git合并、推送、PR或工作区清理。

平台存在子代理工具，但用户明确禁止启动子代理和新聊天，故本任务没有获授权的独立执行方式；没有虚构工具故障或启动记录。Runtime已通过verifier-unavailable保存此具体原因；此前等待的明确接受现已取得，不能反过来把独立审查记为完成。

失败基线见 [调查证据](../../flow/2026-10-22-cross-tab-cache-acceptance.md)。本变更只修账号确认、目录响应与存储归属，不改API/表结构/RLS或业务权限，不新增依赖、不清用户持久缓存，不调用真实模型或外发。

## 原因与共享修复

| 入口 | 原因/最终行为 |
|---|---|
| `auth-session-hydrator.tsx` | 原先只启动恢复。现统一监听身份失效及共享scope变化，以服务端session确认；已知变化立即阻断，失败保留重试。普通聚焦先核对，身份未变不卸载页面或取消消费者 |
| `user-scope.ts` | 原先每次从共享localStorage读取scope，旧标签写入会被另一页重定向。现固定本页已确认scope，失效代次与取消signal保护后续业务请求；持久内容不清除 |
| `api/auth.ts` | 原先缓存失效不脱离旧在途请求。现按失效代次及跨标签revision拒绝旧响应；旧finally不能清掉新请求，payload进入账号切换前再次核对revision |
| `api/request.ts` | 复用原http，受保护请求发起和返回均核对归属，失效取消原传输，认证恢复端点仍可用；错误/取消语义保持 |
| `api/intelligence.ts` | 目录在发起时捕获用户和代次，返回前再次核对；不在响应返回时才猜缓存账号，拒绝显式错误账号 |
| `intelligence/index.tsx` | 三个目录调用链及其消息刷新追踪完成；初始effect卸载取消应用，异步UI入口处理正常取消，不把旧请求错误写到新目录 |
| `user-session.ts` | 复用原持久化flush及串行切换；等待期间revision改变不继续应用旧scope/payload，保留原账号编辑 |
| `generation-consumer-lifecycle.ts`、`api/agent.ts` | 消费者与Agent SSE绑定会话signal，失效立即取消；SSE普通传输超时/瞬时故障重试保持原逻辑 |

## 实际RED与GREEN

最小文件 `web/test/cross-tab-session-cache.test.ts` 最初3项实际RED：

1. 共享scope被另一标签改成乙后，旧页读回乙，而非原账号甲。
2. invalidate后第二个session请求仍复用旧请求，期望2次实际1次。
3. 账号切换后的目录迟到响应返回applied，期望AbortError，存在错写缓存边界。

随后共享scope未投递事件前的HTTP门禁、消费者取消分别先失败；Agent SSE取消另先失败。补齐共享保护后通过。最终10项检查还覆盖同账号退出再登录、旧session先完成时不清新请求、同账号不可达保留缓存、显式错误账号拒绝及未投递事件前的revision变化。全部mock仅为无模型单元/异步边界检查，不称live调用。

开发阶段实际45 pass / 0 fail / 221断言。正式Native Runtime再次执行相同6文件专项，另执行构建及lint：

| 正式检查ID | 命令 | 实际结果 |
|---|---|---|
| cross-tab-and-adjacent | `bun test web/test/cross-tab-session-cache.test.ts web/test/api-request.test.ts web/test/agent-error-events.test.ts web/test/intelligence-gaps9.test.ts web/test/canvas-handoff-sync.test.ts web/test/generation-storage-consistency.test.ts` | exit0，45通过/221断言 |
| frontend-build | `web` 中 `bun run build` | exit0，tsc/Vite通过；保留原有大chunk提示 |
| frontend-ui-contract | `web` 中 `bun run lint` | exit0 |

Runtime operation `f52ab5c2-b456-4a72-b2f0-bd48aadc4852`，candidate `39cf711c-bd77-4cd7-9616-ea7448889157`。dispatch步骤复用3项已绑定记录，没有重新全跑。记录/原日志由Runtime保存在 `.comet/runtime/native/changes/fix-cross-tab-session-cache-isolation`，没有手改状态或报告。

## 18088真实浏览器证据

仅使用 `127.0.0.1:18088` 两个既有测试标签及原本地测试账号。未清缓存、未触碰localhost原用户画布、未发送模型问题。仅更新web，保留回退镜像 `flow-uat-web:before-cross-tab-20261022`；最终web使用正式Runtime构建产物，标签 `d9b809ee-cross-tab-session-cache-working-tree`，含原有Agent面板dirty，不称纯HEAD镜像。

- A owner目录预热，B退出：A自动到登录页，旧标记数量0，无手动刷新。
- B登录member：A账户菜单投影member，只显示member标记，owner数量0；反向owner同样正确。最终构建产物又完整执行owner退出→member→退出→owner，均无上一账号标记残留。
- session确认503故障注入：最初隐藏父容器方案失败，Drawer portal仍可见，作为本轮第二次实际UI RED保留；改为卸载受保护子树后再注入，旧目录和Drawer均0，仅有失败说明及“重新确认登录”。故障配置已恢复，点击重试后两页均回匿名登录表单。
- 同一已确认owner账号离开情报页，暂停platform期间站内重返：owner标记可见，member数量0。没有在该窗口更改身份；因此证明同账号warm-start，不声称可离线确认新账号。随后立即unpause，最终健康恢复。
- 快速/迟到顺序采用可控deferred单元检查，浏览器为真实重复退出/登录旅程；不把串行浏览器点击称网络竞态压力测试。

临时503只修改隔离web内nginx精确session location，原配置已保存并恢复；没有改变后端鉴权、Cookie、RLS或生产配置。一次接口暂停导致health短时unhealthy，恢复后须以最终检查为准，不能遗漏该过渡状态。

## 六项验收证据归属

| ID | 主会话实际核对依据 | 独立语义评估 |
|---|---|---|
| A1 | 双向实际账号切换/退出；同账号中间失效及迟到顺序单元检查 | 未执行；用户接受该限制后通过 |
| A2 | 精确session503真实UI，portal RED→修复后0旧目录/0旧Drawer，重试恢复 | 未执行；用户接受该限制后通过 |
| A3 | 认证/目录deferred、原请求取消和三调用链检查；HTTP错误合同回归 | 未执行；用户接受该限制后通过 |
| A4 | scope错位最小检查、原生成存储消费链路相邻回归；不触碰原未知脏编辑 | 未执行；用户接受该限制后通过 |
| A5 | 同账号接口不可达真实目录、跨账号标记隔离及缓存单元检查 | 未执行；用户接受该限制后通过 |
| A6 | Runtime三项检查和18088实际专项；数据保留快照及最终环境核对 | 未执行；用户接受该限制后通过 |

截图、nginx回退配置、构建日志及仅含哈希的业务快照在Git忽略目录 `.local/flow-uat-20261020/cross-tab-fix`。`session-503-blocked.png`是失败证据，不可当通过；`session-503-fixed.png`才是修复后无旧portal证据。密码/密钥/Cookie不写本文或提交。

## 未执行、限制与发布门槛

没有运行独立Verifier、全仓测试、真实模型、真实MCP/外发、生产迁移、push/merge/release。没有清浏览器持久缓存或索引。真浏览器offline/冷启动、所有IndexedDB路径、独立浏览器上下文和完整PRD未验；原196项审计为追溯基线，861项不是待开发功能数。

最终数据保留核对：画布3条、实采文档10条、报告1条的整表哈希与before一致，模型任务0、预算预留0；platform/PG/Engine/调度器的容器ID和镜像均一致，仅web更换。nginx最终配置与故障前字节一致；web/platform均healthy、paused=false。`preservation-final.json`四项比较均true。登录活动/测试会话本来会变化，不宣称全库逐字不变。

未知旧脏画布仍不清除或猜测恢复。用户接受验证结果及单独归档授权均已记录，本地归档完成，交付未执行。归档收尾引用原45项/构建/UI证据，没有重复运行产品测试或操作18088。
