# C02 当前基线验证证据账

执行时间：2026-10-20（Asia/Shanghai）。SHA：9fbcf028e9661883d94c51ee538bc54d126c0357。执行时分支：codex/fix-command-outbox-ownership-and-errors；后续文档收口位于用户确认的新分支 codex/align-flow-status-and-baseline-evidence，产品代码未变。本文件只报告本轮实际自动检查，不表示真实环境或发布验收全部完成。

## 实际执行

环境为 Windows/amd64；Node v24.16.0、Bun 1.3.6。PATH 的 Go 启动器自报 1.22.12，backend 实际通过工具链选择使用 go1.25.0（go -C backend env GOVERSION）。

| 命令/目录 | 退出码与结果 | 边界 |
|---|---|---|
| git status/HEAD/branch、openspec list、.codegraph 存在检查 | 基线如上；开始 0 活跃、15 归档；无 .codegraph | 已有用户脏文件保留 |
| go -C backend test ./internal/flow/... -count=1 | 0；56.159s | SQLite/可控外部依赖；不等于 live/真实 PG |
| go -C backend test ./internal/canvas/capability/... ./internal/app -run 'TestCanvasAgentExecutesReadOnlyIntelAndMCPTools\|TestCanvasConversationContinuedFromIntelligenceUsesCanvasTools\|TestCloudAgentToolSchemaStaysCompact\|TestCloudAgentToolTableMatchesRuntimeDispatch' -count=1 | 0；app 5.306s；capability 1.445s 标注 no tests to run | capability 没有匹配测试，不能记为覆盖该包全部用例；app 为真实实现专项，未运行模型生成 |
| web：bun test test/canvas-dataflow.test.ts test/canvas-node-registry.test.ts test/intelligence-panels.test.tsx test/intelligence-gaps9.test.ts test/intelligence-empty-reason.test.ts | 0；46 pass / 0 fail，358 expect，5 文件，4.81s | 有 AntD List/Space 弃用提示；渲染/源码及算法检查，不是浏览器验收 |
| web：bun run build | 0；tsc 与 Vite 构建通过，Vite 10.19s | 大于 500 kB 分块提示保留，未为消除提示改代码 |
| services/loom-engine：node --experimental-strip-types --test flow/*.test.ts | 0；98 tests，94 pass / 0 fail / 4 skip，67.9235s | 主要 PGlite/fixture；真实 legacy/RLS/恢复依赖跳过；A20 见下节 |
| go -C backend test ./internal/flow -run 'TestRuntimeRLSBlocksUnfilteredHandlerQueries\|TestFlowRLSPostgres\|TestRLSCapabilityAndProviderTables' -count=1 -v | 0；实际匹配 RuntimeRLS 与 CapabilityAndProvider 两项，均 SKIP | FLOW_TEST_PG_RLS_DSN 未配置；TestFlowRLSPostgres 位于 database 包，未在该命令运行 |
| go -C backend test ./internal/database -run TestFlowRLSPostgres -count=1 -v | 0；TestFlowRLSPostgres SKIP | 同上，未配置实际 PG；不能记为 PG 通过 |
| prd-audit-9.md 逐行解析 | Y177 / D8 / E7 / O4，共196，P/N0 | 原表头 Y176/D9 已漂移；未修改或提交用户原文件 |
| git rev-list --left-right --count origin/main...HEAD | 0 / 32 | 本地保存的远端跟踪引用，未成功核实真实远端 |

## A20 的证据限制

Engine 测试内置 git ls-remote origin refs/heads/main。本轮命令启动后该网络查询失败（无法连接 GitHub），测试按既有实现捕获错误并回退 origin/main（必要时还会回退 HEAD），因此 A20 被计入通过仅证明本地引用/锁文件的弱检查，不证明远端 SHA、无本地超前、CI、merge 或部署版本。没有手工重试联网、推送或发布，也没有为使测试更严格而擅自改产品/测试代码。后续 G01 经明确授权再核实远端。

## 本轮未执行

未启动 dev server/Docker 或停止既有进程；未运行浏览器、真实模型/Embedding、SMTP/Webhook、生产迁移、实采、真实 MCP/OAuth、对象存储/备份恢复。Engine 的 4 个真实 PG 相关跳过项需本地配置 FLOW_TEST_PG_ADMIN_URL 与 FLOW_TEST_PG_APP_ROLE；Go 真实 PG 使用 FLOW_TEST_PG_RLS_DSN。只记录变量名称，未读取或输出值。

本轮不以 94/46 或历史 185 Y 作为全项目完成率。C01 的当前文档口径已在本次文档变更中对齐，C02 更广集成验证及 U/A/G 发布门槛继续保留。
