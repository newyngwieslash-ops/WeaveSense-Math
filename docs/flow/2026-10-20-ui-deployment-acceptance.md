# Flow 浏览器与隔离部署验收证据

时间：2026-10-20 22:36 +08:00（Asia/Shanghai）。仓库 HEAD `5ad423dbc20a17e2c48371e50c792e737ac97b7e`，分支 `codex/align-flow-status-and-baseline-evidence`。本次只做验收与本地隔离部署，没有修改产品逻辑、推送或发布。

## 任务账

| 项目 | 实际结果 | 剩余边界 |
|---|---|---|
| 本次文档变更归档 | 本地提交 `5ad423db` 已存在；16 个归档、0 活跃 OpenSpec 变更 | 归档元数据 branch_status 的已有未提交修改保留；未做远端交付 |
| U01 现有 8088 入口 | 用户登录后逐个点击九视图，均能打开；仅 Default 工作区和一名 owner，内容/信源/任务/报告/连接为空；MCP 页面显示无连接、未知结果 0 | 运行版本不同于当前源码，不能解除当前版本 U01 门槛；无第二账号/工作区，未验证隔离或角色权限 |
| U01 主题/键盘/窄屏 | 明暗切换可见；390×844 下左方向键使行业包获得可见焦点，Enter 可切换 | 键盘自动滚动导致整个内容祖先横移约 275px，右侧空白；需当前版本复核，未据此改代码。测试后恢复视口及浅色主题 |
| U02 | 报告、专题、内容库为空 | 中文导出、引用、三类交接与冲突/跨范围拒绝未执行 |
| U03 | 未执行 | 真实画布 DAG、保存恢复、撤销/重做仍待验收 |
| U04/U05 | 未执行 | 未调用模型、MCP 或采集；草稿/审批/SSE/重启/预算/对账仍待验收 |
| A01 core 本地隔离启动 | 4 镜像构建退出 0；8 个 core 服务创建，minio-init 退出 0，其余运行，平台/web/PG healthy | 只证明新空库 core 启动，不代表其余 profiles 或业务闭环 |
| A02–A10 准备 | 公共模板下全 profiles 的 `config --services` 退出 0，共 19 服务；读取现有监控、初始化和备份/恢复配置 | promtool 未安装；真实告警、对象/媒体、OCR/ASR、旧库/PG16 迁移、恢复、价目表/Embedding 未运行 |

## 版本与环境

8088 浏览器出现 Sources/Reports/内嵌聊天等旧布局，非专题页缺少当前 `web/src/pages/intelligence/index.tsx` 中的情报助手按钮；已有 Engine 文件哈希与当前源码不同。它不是当前版本验收环境。未将其空态或页面渲染计作当前 HEAD 全量通过。

用户明确批准新项目、新端口、新卷以及新空库初始化。独立项目 `flow-uat-20261020`，入口 `http://localhost:18088`，仅绑定 `127.0.0.1`；卷为该项目前缀的 pgdata/platformdata/objects/redisdata。未读现有 `.env.flow`，从公共模板生成独立随机密钥，保存在 Git 忽略的 `.local/flow-uat-20261020/uat.env`，不记录值。模型网关、SMTP、Telegram/WeChat 和 Embedding 禁用；注册仅为本地空库测试开启。Compose 覆盖仅作用于隔离项目，没有执行旧项目的重启或迁移。

配置文件 `.local/flow-uat-20261020/compose.acceptance.yml` 覆盖三个 env_file 及 web 端口。平台/web Dockerfile 是根配置的本地副本，配套 allowlist 构建上下文，防止根目录本地参考文件进入上下文。构建命令：

```powershell
docker compose -p flow-uat-20261020 --env-file .local/flow-uat-20261020/uat.env -f docker-compose.flow.yml -f .local/flow-uat-20261020/compose.acceptance.yml --profile core build platform web loom-engine loom-scheduler
docker compose -p flow-uat-20261020 --env-file .local/flow-uat-20261020/uat.env -f docker-compose.flow.yml -f .local/flow-uat-20261020/compose.acceptance.yml --profile core up -d --no-build --pull never
```

两条实际命令退出 0。构建存在既有大 chunk/动态导入警告；不是类型检查或产品测试结果。Engine Dockerfile 的构建期 db:migrate 因 DATABASE_URL 未设置而跳过；启动时才初始化隔离数据库。

平台 `/api/health/ready`：code=0、ready=true、build commit=`5ad423db…`、schema current/expected=`67/67`。Engine 内部 `/healthz`：HTTP 200、code=0。PG 应用角色 flow_platform/flow_intel 的 rolsuper/rolbypassrls 均 false；Intel 27 张表带 RLS/FORCE RLS。这里只核对标志，未运行跨租户行为测试。

新 Engine `/app/flow/engine.ts` SHA256=`f1a6a76551377d7a1db9f324f1d667c75c7dabbaeb519ee4dadd448a968d8301`，`gaps9.ts` SHA256=`6d1deaba91946da9625de6270320e8c6bfa786e536b24cd0d2ae0d9968eb2f03`，与本地文件相同。

构建期间另有非本轮写入的 `web/src/components/canvas/canvas-cloud-agent-panel.tsx` 单行变更：缺少受管模型时提前报错。它保持未提交，未覆盖；新前端可能包含此工作区变更，所以平台 build SHA 不能证明前端是纯 HEAD。该文件当前 SHA256=`0af1566b4ae411a82d748e72d10c61d310d0aad6a6a7f1f5f86169957d0560d3`。现有 8088 平台/web 运行时间亦出现外部变化，不将其他操作归为本轮成果。

## 18088 登录后续验收（23:00 +08:00）

用户已在 Codex 内置浏览器 tab1 登录，新库登录前置已解除。下列是当前工作区构建的真实 UI 结果：

| 项目 | 结果 | 边界 |
|---|---|---|
| U01 九视图 | 逐项打开成功，当前中文界面、所有者角色和无数据原因可见；溢出标签通过菜单可导航 | 仅一个账号/Default，未验证账号切换、其他角色或多工作区权限 |
| U01 助手/主题/窄屏 | 助手抽屉打开、Esc关闭；实际390×844时dialog边界0–390，正文与操作可见；明暗切换可见 | 旧部署横移步骤在新版未复现，祖先scrollLeft列表为空，不据旧现象改代码；全部结束恢复视口与浅色全局主题 |
| U03 基础数据流 | 新建并命名验收画布；非法JSON报错/0条，中文JSON恢复2条；输出连接创建If/Else，score>80时T/F各1，>100时T0/F2；预览可切换 | 未触发真实生成，不把T0当下游付费生成门控全链路通过 |
| U03 CSV/算子/恢复 | 带引号逗号的中文CSV解析2条；过滤2→1，提取score得90/65，改title得到中文值；撤销回score，重做回title；刷新恢复两节点、连接及内容 | A/B交并差、Skill、环检测、本地文件导入、全部格式、重连拖动仍未完整验收 |
| U02 专题→已有画布 | **失败**：UI提示投送成功、URL含新节点，但适应/刷新后仍只有原两节点；隔离PG确认revision15/nodes2/新节点不存在 | 根因未确定，禁止宣称交接全部通过；不归因于模型或未登录 |
| U02 专题→新画布对照 | 同一中文专题创建新画布成功，正文及类型/工作区/来源标识脚注可见；关闭自动唤起Agent，未生成 | 0证据的人工测试专题，不能替代报告/文档引用与导出验收；其他冲突/跨租户/续聊仍待验收 |

隔离测试画布 ID `cFOSdg0PrpHG-bgkuYVC9`，名称 `U03 隔离验收 2026-10-20`。失败交接节点 ID `intel_node_ad4cf9037265ed8792064918`。新画布对照 ID `canvas_5158db1cc61dae10566679a8`。只保存人工合成中文数据，不包含真实采集或模型结果，不删除测试产物。

截图保存在 Git 忽略的 `.local/flow-uat-20261020/screenshots/`：`u03-json-ifelse.jpg`、`u02-existing-handoff-missing.jpg`、`u02-new-topic-handoff.jpg`。均来自真实应用，不是独立HTML或静态仿制。

## 新发现的聚焦修复边界

建议变更 `fix-intelligence-canvas-handoff-sync`，仅调查并修复“已有画布外部交接节点被丢弃/覆盖”的现有行为。证据包括POST 200成功提示、节点ID、刷新缺失、PG查询和新画布对照。预期验收：已同步画布追加情报后原节点/连线与新节点同时保留；存在本地未确认操作时只能重放真实本地差异，不能把云端新增视为本地删除；刷新/再次打开仍保留，权限和版本冲突不退化。

当前线索：`loadCanvasProjectForEditing` 的dirty分支把remote记为新基线却保留整个local；后续 `buildCanvasOperations(base, next)` 会为仅基线存在的节点生成remove_node。登录聚合恢复也有 `buildCanvasOperations(remote, local)`。这只是待最小测试证实的因果假设，尚不能断定首次缺失发生在哪一分支。后端HandoffToCanvas和REST/Agent共享入口、条件GET、缓存/操作日志均纳入追踪。未修改产品代码，未编写新修复测试。

拟定3项任务：①最小自动检查复现真实本地变更+云端新增，以及新画布对照；②在已证实的共享入口最小修复并保留权限/幂等/版本边界；③专项/构建及18088真实投送重验，更新pending-test。没有新增公开API、表结构或依赖；若根因需要跨模块协调/结构调整，重新评估范围。Comet已路由hotfix，入口隔离选择未确认前不进入实施，不继承旧变更设置。

## PG 最小检查与剩余门槛

补充真实 PG17 最小检查：`.local/flow-uat-20261020/rls-check.sql` 在隔离 Intel 库内 `SET LOCAL ROLE flow_intel`，事务插入两个租户探针，设置租户 A 后断言只能读一行、跨租户 UPDATE 为零、跨租户 INSERT 抛 insufficient_privilege。命令退出 0，ROLLBACK 后探针残留 0。仅证明信源表实际 RLS 读/更新/插入边界，不解除平台 RLS、HTTP 租户链路、全表策略、多实例或提交不确定门槛。Engine 迁移最新记录为 `0018_composite_keys_and_fk.sql`。

新库登录已由用户完成，未采集密码。U02已有画布失败优先定位；U01/U03仅部分场景通过，U04/U05仍未执行，A02–A15与G01继续保留真实依赖与授权门槛。真实模型执行、付费调用、外发、生产迁移及G01仍需各自授权。
