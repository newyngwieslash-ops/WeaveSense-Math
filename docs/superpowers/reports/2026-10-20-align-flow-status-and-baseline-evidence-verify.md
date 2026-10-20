# Flow 状态文档与当前基线证据验证

变更：align-flow-status-and-baseline-evidence；基线 `9fbcf028e9661883d94c51ee538bc54d126c0357`，分支 `codex/align-flow-status-and-baseline-evidence`。时区 Asia/Shanghai。用户已确认同一文档 tweak、新分支与超过 6 文件的范围；不启动子代理，不推送/发布。

## 本轮变更

只修改 8 份既有文档，新增[基线证据账](../../flow/2026-10-20-current-baseline-evidence.md)和本报告，并维护本变更工件。没有修改 backend/web/Engine 产品代码、依赖、数据结构或部署配置，无 delta spec，skip_specs=true。旧审计、旧交接、table.csv、参考文件及历史归档元数据不纳入本变更。

| 任务 | 结果与定位 |
|---|---|
| 1.1 当前口径 | implementation-status 建立当前/历史证据入口；roadmap 澄清 P0/P1 历史及两套 A 编号；delivery-report 保留原阶段日期/SHA并增加当前说明；pending-acceptance/known-limitations 对齐 Embedding、媒体、真实依赖；features 对齐受管 MCP/read_only；旧交接接口验证报告追加实际路由更正 |
| 2.1 基线证据 | 草稿移为 docs/flow/2026-10-20-current-baseline-evidence.md 的唯一证据账；各状态入口与 pending-test 链接，历史通过数不改成新验收 |
| 3.1 文档验收 | openspec strict validate、git diff --check、Python 标准库内容/路径断言通过；9 份文档、7 个证据链接、历史测试数字和当前措辞边界已检查 |

## 实际验证与证据来源

以下产品检查是在本轮前置预检中对 `9fbcf028` 实际执行，完整命令、版本、退出码及耗时见唯一证据账；本次只有文档变化，没有重复运行后再虚报新结果。

- Go Flow 包通过；Canvas Agent app 匹配专项通过，capability 包没有匹配用例，不能声称覆盖全部 capability。
- Bun 5 文件 46 pass/0 fail、358 expect；web tsc/Vite build 通过，有 AntD 弃用和分块大小提示保留。
- Engine 98 tests：94 pass/0 fail/4 skip；真实 PG 的 legacy/RLS/恢复路径跳过。A20 git ls-remote 无法连接后回退本地，套件通过不证明远端 SHA/CI/部署版本。
- Go 真实 PG/RLS 指定检查均因 FLOW_TEST_PG_RLS_DSN 未配置而 SKIP，明确没有计为通过。

本轮文档实际检查：

| 检查 | 结果 |
|---|---|
| openspec validate align-flow-status-and-baseline-evidence --strict | 退出 0；skip_specs 合法，无虚构产品需求 |
| git diff --check | 退出 0；LF/CRLF 提示不是语法失败 |
| Python 3.13 标准库一次性断言（Runtime 记录原始参数） | 退出 0；9 文档、7 链接存在，当前 Embedding/MCP/gzip/媒体边界正确，保留历史 21/0/233 数字 |
| git diff --name-only 9fbcf028 -- backend web services | 无产品源路径变更；生成物保持 Git 忽略 |

没有 `docs/package.json` 或 docs/node_modules，本轮未执行文档站 types:check/build，不把文本检查当作站点构建。新增 MDX 仅含正文、列表、代码片段与 Markdown 链接，未引入组件或 JSX。

## 完整核对与审阅范围

3/3 tasks 完成；proposal/design 与实际文档差异一致。无 delta spec，无 full 工作流技术 Design Doc/plan 要求。按文件规模执行完整工件/设计核对；review_mode=off，只跳过自动子代理代码审查，本会话仍检查差异和来源，不启动子代理。

历史文档仍可含原日期下的 P0 only、旧上限或旧路由，入口明确为历史，旧路由报告顶部追加实际注册的更正。没有删除历史证据或把当前测试数字写回旧验证。原审计逐行 Y177/D8/E7/O4=196 与表头漂移的差异，在当前入口记录而未改用户文件。

## 剩余门槛

C01 当前文档口径已对齐，C02 的可独立自动检查已记录，C02 更广集成和 U/A/G 门槛仍未完成：真实 PG/RLS/多实例/提交不确定、实际 UI/Agent/SSE/重启链路、Compose/恢复/迁移副本、真实模型/实采/MCP/OAuth/外发及最终远端交付。TenantMiddleware 外层 Commit 与 HTTP 回执仍待独立复现。未配置凭据时只记录名称，不降低验收要求。

本地文档提交为 `55210c10`。Comet Runtime 已实际运行上述 Python 文档检查作为本变更的 build/verify 文档校验（不是产品或文档站构建），两次均退出 0；build guard 14/14、verify guard 5/5、archive 入口 5/5 通过。状态为 phase=archive、verify_result=pass、archived=false，归档仍需本变更的最终确认；没有网络重试、付费调用、服务启动、推送、合并或发布。
