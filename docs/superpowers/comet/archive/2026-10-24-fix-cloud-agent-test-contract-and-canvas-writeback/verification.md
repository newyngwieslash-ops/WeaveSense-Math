---
generated_from_state_version: 11
---

# 验证

## 当前结果

- 结果: **已归档**
- 验证情况: **已完成检查，验证结果已确认**
- 目标周期: 1
- 迭代: 1
- 验证器尝试次数: 2
- 完成时间: 2026-10-24T12:05:53.766Z
- 摘要: 只改了测试：A1 改为精确集合校验并加负向控制；A2/A3 通过夹具关闭已批准媒体等待器消除了 ErrCreationConflict 竞争。没有改动产品代码，没有删除或跳过测试，文档如实。

## 验收

| 编号 | 结果 | 来源 | 验收项 | 原因 |
| --- | --- | --- | --- | --- |
| A1 | passed | brief.md | A1：节点注册表断言列出现行完整类型集合并逐项检查，类型缺失、多出或重复时测试失败；原有 panorama/character 断言保持。 | 新断言用硬编码的13类型期望集合对 cloudAgentNodeTypes() 做精确集合校验；该注册表来自 canvasCapabilityRegistry，期望集合独立而非动态抄写。负向控制对缺失、多出、重复三类漂移都有检测。原 panorama 外露、character 不可创建/可发现/不进 add_node 枚举的断言在 diff 中未删除。 |
| A2 | passed | brief.md | A2：已用文件/行号与命令输出记录 `ErrCreationConflict` 在 CanvasPatches、MediaCompletion 两个测试中的触发点和根因分类（测试造数/时序、Windows 环境、真实产品缺陷之一），MissingCanvas 的整包与单独结果差异也有解释或明确记为未解释。 | 根因归类为测试夹具时序竞争，有代码支持：startApprovedCloudAgentMediaWaiter 只检查 approvedMediaClosed，不检查 disablePiRuntime；creationTestService 设了 disablePiRuntime:true，所以后台等待器仍会运行，与测试直接调用 advanceCloudAgentByID 或改写画布产生竞争，触发 saveCreationCanvasWithHistory 的 ErrCreationConflict。MissingCanvas 单独通过而整包失败，与随机时序竞争一致。 |
| A3 | passed | brief.md | A3：若根因是测试或环境，修复后这三个测试在 `go test ./internal/app -run <三个测试> -count=3` 下稳定通过；若是产品缺陷，保留失败证据并停在向用户重新确认，不得改产品代码后宣称通过。 | agentMediaFixture 中设 approvedMediaClosed=true 使等待器直接返回，只关闭了竞争源。产品代码确实未改：git status 显示 backend 下只有两个测试文件被修改。Runtime 回执显示五个目标测试 -count=3 通过；Builder 报告称 10 轮全过。风险：若有测试依赖该夹具并需要等待器运行，会失去这部分覆盖，但 TestCloudAgent 全组仍全部通过。 |
| A4 | passed | brief.md | A4：`go test ./internal/app -run TestCloudAgent -count=1` 在已安装 agent-runtime 依赖的当前环境下，除登记在 C02 文档的环境类失败外，不再有本变更范围内的失败，且未删除或跳过任何测试。 | 日志 .local/fix-agent-group.log 显示 ok 且无 FAIL，约 536 行 PASS 以及 1 个 SKIP。diff 只有 58 行新增、1 行删除（旧的 len==10 断言），没有删除或新增 skip。被跳过的 TestCloudAgentVideoFramesPostgres 是预先存在的跳过，原因是 CANVAS_TEST_POSTGRES_DSN 未配置。Runtime 回执 TestCloudAgent 全组 passed。 |
| A5 | passed | brief.md | A5：`docs/flow/pending-acceptance.md` 之外的新证据文档记录本次实际命令、结果、未执行项和剩余环境类失败，C02 基线文档对应项得到状态更新。 | 验证报告和 C02 文档末尾更新段如实记录了命令和结果，包括基线 8/10 失败、修复后 10/10 通过。它们列出了未执行项：未在 Linux/Docker 运行、PG 用例跳过、非 CloudAgent 用例未重跑、fsync/Redis/权限位/Gmail 等环境类失败仍待 F09，并说明已撤回此前对 runtime 两项为真实回归的初判。未发现夸大。 |

## 检查

| 检查 | 命令 | 工作目录 | 状态 | 退出码 | 耗时 |
| --- | --- | --- | --- | ---: | ---: |
| go vet ./internal/app | vet ./internal/app | backend | passed | 0 | 5817 ms |
| 五个目标测试 -count=3 | test ./internal/app -run ^(TestCloudAgentCanvasPatchesPersistDraftSubmissionAndAllTerminalStates\|TestCloudAgentMediaCompletionKeepsGenerationAndWritebackFailures\|TestCloudAgentMissingCanvasStillCheckpointsMediaFailure\|TestCloudAgentNodeTypesExposeExecutableAllowList\|TestCloudAgentNodeTypeSetCheckDetectsDrift)$ -count=3 | backend | passed | 0 | 23432 ms |
| TestCloudAgent 全组 | test ./internal/app -run TestCloudAgent -count=1 | backend | passed | 0 | 144982 ms |

### Builder 报告的证据

以下为 Builder 报告，不等同于 Runtime 检查凭据或独立验收结果。

- 基线：三测试组合x10(修改前): failed — 8失败/2通过，证明随机冲突
- 三测试组合x10(夹具修改后): passed — 10/10 ok
- go vet ./internal/app: passed — 无输出
- TestCloudAgent全组 -count=1 -v: passed — 347通过/0失败/1跳过(TestCloudAgentVideoFramesPostgres需要真实PG)；.local/fix-agent-group.log
- Linux/Docker运行: not-run — 未执行，fsync/Redis/权限位类环境失败仍待F09
- 已知限制: 仅Windows本地执行，未在Linux/Docker验证；fsync、缺Redis、权限位、Gmail临时文件占用及其他约10个internal/app用例不在本变更范围，仍待F09
- 已知限制: TestCloudAgentVideoFramesPostgres跳过，需要真实PG，不算通过
- 已知限制: 产品代码startApprovedCloudAgentMediaWaiter不遵守disablePiRuntime，生产不设该标志故无用户可见影响，未修改，如需改须另行确认范围
- 已知限制: 无独立Verifier时结果需用户明确接受

## 阻塞项

_无。_

## 风险与跳过的工作

- 仅在 Windows 验证，未在 Linux/Docker 验证
- 真实 Postgres 用例跳过，不算通过
- 产品侧等待器不遵守 disablePiRuntime，按约束未修改，只在夹具中规避
- 夹具关闭等待器，可能削弱依赖该夹具的测试对等待器行为的覆盖
- 347 通过/1 跳过 等具体数字我只通过日志部分佐证，没有重跑全组
- fsync/Redis 等环境类失败不在本次范围内，仍然存在

## 之前的迭代

| 目标周期 | 迭代 | 尝试 | 结果 | 未解决项 | 摘要 | 完成时间 |
| ---: | ---: | ---: | --- | --- | --- | --- |
| 1 | 1 | 1 | blocked | A1, A2, A3, A4, A5 | 用户本次明确禁止启动子代理和新聊天，因此没有可用的独立 Verifier 执行；Runtime 的 3 项命令检查已通过（复用），未做独立语义复核。 | 2026-10-24T11:41:18.834Z |
| 1 | 1 | 2 | pass | — | 只改了测试：A1 改为精确集合校验并加负向控制；A2/A3 通过夹具关闭已批准媒体等待器消除了 ErrCreationConflict 竞争。没有改动产品代码，没有删除或跳过测试，文档如实。 | 2026-10-24T12:05:53.766Z |



## 结论

只改了测试：A1 改为精确集合校验并加负向控制；A2/A3 通过夹具关闭已批准媒体等待器消除了 ErrCreationConflict 竞争。没有改动产品代码，没有删除或跳过测试，文档如实。
