# fix-cloud-agent-test-contract-and-canvas-writeback 验证报告（2026-10-24 19:33 +08:00）

环境：Windows 本地，HEAD `2ea771fc`，分支 `codex/fix-workspace-member-controls-and-analyst-permissions`，已 `npm ci` 安装 `backend/agent-runtime/pi` 依赖（环境修复，没改 lock）。没有部署、模型、外发、子代理、Linux/Docker 执行。

## 根因结论

| 项 | 分类 | 证据 |
| --- | --- | --- |
| `TestCloudAgentNodeTypesExposeExecutableAllowList` 断言 `len(nodes)==10` | 测试合同漂移 | 现行注册表 13 个类型：audio、batch-table、character、data-source、file、frame、image、logic、markdown、script、skill、text、video；其中 `character` 不可创建，`cloudAgentNodeTypeNames()` 为另外 12 个 |
| `TestCloudAgentRuntimeCompletesToolRoundTrip`、`TestCloudAgentLiveDoesNotRetryRejectedRequest` 等 | 环境缺依赖 | 日志 `Agent runtime exited: exit status 1`；`backend/agent-runtime/pi/node_modules` 不存在；`npm ci` 后通过 |
| `CanvasPatches…`、`MediaCompletion…`、`MissingCanvas…` 的 `ErrCreationConflict` | 测试夹具时序竞争 | 见下 |

### 写回冲突

1. 失败不稳定：同一测试单独运行 4 次也有通过有失败，所以不是用例顺序或 fsync 造成。
2. 触发点：`saveCreationCanvasWithHistory`（`canvas_bridge.go`）在读取与保存之间发现 `PayloadJSON` 或 `Revision` 变化即返回 `ErrCreationConflict`；测试在直接调用 `advanceCloudAgentByID`、直接改写 `payload_json` 时发生。
3. 并发写入者：`startApprovedCloudAgentMediaWaiter`（`cloud_agent_pi_coordinator.go:136`）会在后台 goroutine 里调用 `finishApprovedCloudAgentMedia` 推进同一运行；它**不检查** `disablePiRuntime`，而测试服务（`creationTestService`，`creation_test.go:33`）设置了 `disablePiRuntime: true`，所以等待器仍在后台运行并与测试的直接推进竞争。
4. 验证：基线（修改前）10 轮三测试组合 **8 轮失败 / 2 轮通过**；夹具里设 `s.approvedMediaClosed = true`（已有字段，现有 `startApprovedCloudAgentMediaWaiter` 在其为真时直接返回）后 **10 轮全部通过**。

### 产品侧观察（未修改）

`startApprovedCloudAgentMediaWaiter` 不遵守 `disablePiRuntime`。生产环境从不设置该标志（只在测试里为 true），所以对用户没有可见影响；是否让等待器也遵守该标志属于产品代码改动，按 Shape 约束需用户重新确认，**本变更没有改**。

## 改动

- `backend/internal/app/cloud_agent_test.go`：节点注册表断言改为对 13 个现行类型的精确集合校验（`cloudAgentExpectedNodeTypes`、`checkCloudAgentNodeTypeSet`），原 panorama 外露、character 不可创建/可发现/不进 add_node 枚举断言保留；新增 `TestCloudAgentNodeTypeSetCheckDetectsDrift` 对缺失、多出、重复三类漂移做负向控制；补 `fmt` 导入。
- `backend/internal/app/cloud_agent_media_test.go`：`agentMediaFixture` 设 `s.approvedMediaClosed = true` 并注明原因。
- 没有改任何产品代码，没有删除或跳过测试。

## 检查结果（本轮实际执行）

| 命令 | 结果 |
| --- | --- |
| 基线：三测试组合 ×10（修改前） | 8 失败 / 2 通过 |
| 三测试组合 ×10（夹具修改后） | 10 通过 |
| `go vet ./internal/app` | 无输出 |
| `go test ./internal/app -run 'TestCloudAgentNodeTypesExposeExecutableAllowList\|TestCloudAgentNodeTypeSetCheckDetectsDrift' -count=1` | 通过 |
| `go test ./internal/app -run TestCloudAgent -count=1 -v` | ok；347 通过 / 0 失败 / 1 跳过（`TestCloudAgentVideoFramesPostgres`，需要真实 PG） |
| 五个目标测试 `-count=3` | ok |

## 未执行 / 限制

- 没有 Linux/Docker 运行；Windows fsync、缺 Redis、权限位、Gmail 临时文件占用等环境类失败不在本变更范围，仍待 F09。
- 没有执行 `internal/app` 其他非 CloudAgent 用例、`handler`/`platform` 等包的重跑。
- 真实 PG 用例跳过，不算通过。
- 产品侧"等待器不遵守 `disablePiRuntime`"未处理，见上。
