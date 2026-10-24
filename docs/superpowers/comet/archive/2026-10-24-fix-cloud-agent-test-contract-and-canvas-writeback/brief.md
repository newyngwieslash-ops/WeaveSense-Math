# 目标

让后端 `internal/app` 包里的云端 Agent 测试在当前 HEAD 上反映真实产品行为：节点注册表数量断言与现行产品一致，画布写回冲突（`ErrCreationConflict`，"数据已被更新，请刷新后重试"）弄清根因并按根因处理。C02 基线见 `docs/flow/2026-10-24-c02-go-engine-baseline.md`。

# 范围

1. `TestCloudAgentNodeTypesExposeExecutableAllowList`（`cloud_agent_test.go`）断言 `len(nodes) == 10`，现行注册表已含 `data-source`、`logic`、`skill`、`batch-table` 等，数量过时。改为与现行注册表一致的精确断言：逐类型核对集合、UI 专用 `panorama` 不外露、`character` 不进入 add_node 枚举等原有断言全部保留。
2. 画布写回冲突：`TestCloudAgentCanvasPatchesPersistDraftSubmissionAndAllTerminalStates`（`cloud_agent_canvas_events_test.go`）、`TestCloudAgentMediaCompletionKeepsGenerationAndWritebackFailures`（`cloud_agent_media_writeback_test.go`）单独运行稳定失败，`advanceCloudAgentByID` 返回 `ErrCreationConflict`；`TestCloudAgentMissingCanvasStillCheckpointsMediaFailure` 整包失败、单独通过。先最小复现并定位根因，再分流：
   - 测试自身造数/并发/时序问题：只改测试。
   - Windows 环境差异：只改测试辅助或记入文档，不改产品。
   - 真实产品竞态或乐观锁缺陷：停止，向用户汇报证据并重新确认范围后才改产品代码。

# 非目标

- 不处理 `sync <目录>: Access is denied`（Windows 目录 fsync）、缺 Redis、文件权限位 666/640、Gmail 临时文件占用这些环境类失败；它们归 F09（Linux CI）。
- 不处理其他约 10 个原因未明、未单独重跑的 `internal/app` 用例。
- 不重做 `backend/agent-runtime/pi` 依赖安装（本次已用 `npm ci` 装好，属于环境，不改 lock 文件）。
- 不删除、不跳过任何测试；不为让测试变绿而放宽断言；不改与上述三项无关的产品代码；不部署、不调用模型、不改数据库结构。

# 验收示例

- A1：节点注册表断言列出现行完整类型集合并逐项检查，类型缺失、多出或重复时测试失败；原有 panorama/character 断言保持。
- A2：已用文件/行号与命令输出记录 `ErrCreationConflict` 在 CanvasPatches、MediaCompletion 两个测试中的触发点和根因分类（测试造数/时序、Windows 环境、真实产品缺陷之一），MissingCanvas 的整包与单独结果差异也有解释或明确记为未解释。
- A3：若根因是测试或环境，修复后这三个测试在 `go test ./internal/app -run <三个测试> -count=3` 下稳定通过；若是产品缺陷，保留失败证据并停在向用户重新确认，不得改产品代码后宣称通过。
- A4：`go test ./internal/app -run TestCloudAgent -count=1` 在已安装 agent-runtime 依赖的当前环境下，除登记在 C02 文档的环境类失败外，不再有本变更范围内的失败，且未删除或跳过任何测试。
- A5：`docs/flow/pending-acceptance.md` 之外的新证据文档记录本次实际命令、结果、未执行项和剩余环境类失败，C02 基线文档对应项得到状态更新。

# 约束与不变量

- 保持当前分支和目录；9 个 tracked dirty 与全部 untracked 原样保留，不 reset/clean/restore，不 `git add .`，只精确暂存本变更文件。
- 遵守 AGENTS：Go 层次不变，handler 不放业务判断；未经重新确认不改产品行为。
- 测试辅助改动要有明确原因，并保持对真实业务条件的检测力（负向控制仍能失败）。
- 不调用真实模型/付费 API，不外发，不启动子代理或新聊天，不推送、合并或发布。

# 决策

- 范围选 A1：只修节点数量断言并调查写回冲突；缺依赖和 fsync 不纳入。
- 隔离方式选当前目录/当前分支（current），与前面 7 个已归档变更一致。
- 缺 `agent-runtime/pi/node_modules` 判定为环境问题，已 `npm ci` 处理，不属于代码缺陷。
- 写回冲突是否改产品代码，取决于根因；真实产品缺陷必须先回到用户重新确认。

# 待解决问题

- 无阻塞问题。写回冲突根因是实施中要查明的事实，不是待用户决策项；若查明为产品缺陷会按约束重新确认。

# 验证预期

- RED：先观测节点数量断言在新版回归下对多出/缺失类型失败，冲突复现命令及输出入证据文档。
- GREEN：上述三个测试 `-count=3`，`TestCloudAgent` 全组，`go vet ./internal/app`（如可行）；Windows 环境类失败单独列出并说明未验证 Linux。
- 如实标注：未在 Linux/Docker 执行；未执行真实浏览器；无独立 Verifier 时由用户明确接受有限结果。
