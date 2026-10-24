# 云端 Agent 测试合同：节点注册表与画布写回冲突

## 完整边界

本能力约束 `backend/internal/app` 云端 Agent 测试与现行产品行为的一致性。默认不改变产品行为；只有在证明存在真实产品缺陷并经用户重新确认后，才允许改产品代码。测试不得被删除、跳过或放宽以变绿。Windows 目录 fsync、缺 Redis、文件权限位、Gmail 临时文件占用及未单独重跑的其他用例不在本能力范围。

## 节点注册表

`cloudAgentNodeTypes()` 返回的 `nodes` 须与现行产品的完整类型集合精确一致，包含情报与画布相关的 `data-source`、`logic`、`skill`、`batch-table` 等现行类型。断言以逐类型集合和数量共同校验，不再使用过时的固定 10。UI 专用 `panorama` 不得外露；`character` 作为 text 节点变体必须可被 Agent 发现，但不得出现在 add_node 的 nodeType 枚举中。注册表缺类型、多类型、重复类型时测试必须失败。

Scenario: 注册表断言与现行类型集合一致
- WHEN 运行 `TestCloudAgentNodeTypesExposeExecutableAllowList`
- THEN 对现行完整类型集合逐项通过，且 panorama 外露、character 进入 add_node 枚举、缺类型、多类型、重复类型这些负向控制实际失败

## 画布写回冲突定位

`ErrCreationConflict`（数据已被更新，请刷新后重试）在 `TestCloudAgentCanvasPatchesPersistDraftSubmissionAndAllTerminalStates` 与 `TestCloudAgentMediaCompletionKeepsGenerationAndWritebackFailures` 中出现。须先最小复现并定位触发点，分类为测试造数/时序、Windows 环境差异、真实产品缺陷三者之一，并记录文件、行号、命令与输出。`TestCloudAgentMissingCanvasStillCheckpointsMediaFailure` 整包失败而单独通过的差异必须有解释，或明确记为未解释。

Scenario: 根因分类有证据
- WHEN 复现并检查冲突触发点
- THEN 证据文档给出三类之一的结论、触发文件与行号、复现命令和实际输出；无法归类时记为未解释，不得推测为通过

## 按根因处理

若根因是测试造数、并发或时序问题，只修改测试并保持对真实业务条件的检测力。若是 Windows 环境差异，只修改测试辅助或记录文档。若是真实产品竞态或乐观锁缺陷，停止并向用户汇报证据，用户重新确认范围前不得修改产品代码，也不得把该测试记为通过。

Scenario: 测试或环境根因修复后稳定
- WHEN 根因为测试或环境并完成修复
- THEN 三个相关测试在 `-count=3` 下稳定通过，且原有断言、负向控制与全部测试均保留

Scenario: 产品缺陷不擅自修改
- WHEN 根因被证实为真实产品缺陷
- THEN 保留失败证据并停在重新确认范围，没有产品代码改动，也没有宣称通过

## 组测试与记录

在已安装 `backend/agent-runtime/pi` 依赖的环境中运行 `TestCloudAgent` 全组，除已登记的环境类失败外，本能力范围内无失败，且没有删除或跳过测试。新证据文档记录实际命令、结果、未执行项和剩余环境类失败，C02 基线文档对应项更新状态。未在 Linux 运行必须如实写明。

Scenario: 全组结果与记录一致
- WHEN 运行 `go test ./internal/app -run TestCloudAgent -count=1`
- THEN 范围内失败为零，失败项仅限已登记的环境类，测试总数不减少，证据文档与 C02 文档同步更新
