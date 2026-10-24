# C02 当前 SHA 回归基线（2026-10-24 13:13–13:24 +08:00）

环境：Windows 本地，HEAD `2ea771fc`，分支 `codex/fix-workspace-member-controls-and-analyst-permissions`。没有改产品代码或测试，没有部署、模型、外发。日志在 Git 忽略的 `.local/c02-go-test.log`、`.local/c02-engine-flow.log`。

## 结果

| 范围 | 命令 | 结果 |
| --- | --- | --- |
| Engine | `cd services/loom-engine && npm run test:flow` | 98 项：94 通过 / 0 失败 / 4 跳过（3 项真实 PG 的 RLS、备份恢复、HTTP 端到端未设 `FLOW_TEST_PG_ADMIN_URL`、`FLOW_TEST_PG_APP_ROLE`；另 1 项未核对原因）。跳过不是通过 |
| 后端 | `cd backend && go test ./...` | 27 个包 ok；5 个包 FAIL：`internal/app`、`internal/handler`、`internal/hostupdate`、`internal/platform`、`internal/repository`；约 49 个失败用例 |

## 失败分类（已证实 / 推断）

| 类别 | 现象 | 状态 |
| --- | --- | --- |
| Windows 目录 fsync | `sync <临时目录>: Access is denied`，来自 `settings.go` 的 `dir.Sync()`，波及密钥发布、资源 CDN、媒体恢复等约 30 项 | 推断为环境问题；未在 Linux/Docker 验证 |
| 缺 Redis | `temporary Redis did not start`（app、platform 各 1） | 环境问题，未验证 |
| 文件权限 | `TestSetEnvValuePreservesOtherSettings`：`mode=666, want 640` | Windows 无 Unix 权限位，推断 |
| 临时文件占用 | `TestGmailMailboxRegistrationConcurrent` TempDir 清理失败 | 环境问题，推断 |
| 单独重跑仍失败 | `TestCloudAgentNodeTypesExposeExecutableAllowList`、`TestCloudAgentRuntimeCompletesToolRoundTrip`、`TestCloudAgentLiveDoesNotRetryRejectedRequest` | **不是 fsync 连带；疑似真实回归或测试合同漂移** |
| 单独重跑通过 | `TestCloudAgentMissingCanvasStillCheckpointsMediaFailure` | 整包并发/环境相关 |

### 单独重跑仍失败的 3 项（只读定位）

1. `TestCloudAgentNodeTypesExposeExecutableAllowList`（`cloud_agent_test.go:624`）断言节点数 `== 10`。实际注册表已含 `data-source`、`logic`、`skill`、`batch-table` 等，应超过 10。9 个 tracked dirty 中没有后端文件，所以这是 HEAD 上的失败。初判：情报/技能节点加入后数量断言过时（测试合同漂移）。**未改，未确认。**
2. `TestCloudAgentRuntimeCompletesToolRoundTrip`：run 失败，日志含 `invalid Agent checkpoint (runtime validation): Agent runtime has multiple active tasks`（`cloud_agent_runtime_state.go:213`）。已有 `cloud_agent_single_active_task_test.go` 专门针对这个线上问题。是否真实回归待查。
3. `TestCloudAgentLiveDoesNotRetryRejectedRequest`：`got 0 upstream calls`，与 2 同属运行时中断，疑似同根因。

## 未证实 / 未执行

- 没有 Linux/Docker 运行，因此 fsync/Redis/权限类只是推断。
- 其余约 10 个"原因未明"用例未逐个单独重跑。
- 没有运行带 PG/Redis 的集成路径。

## 建议后续

- 新建聚焦 Native 变更处理上面 3 项（需用户确认 Shape）。
- 在 Linux 容器里跑 `go test ./...`（F09），再把 fsync/Redis/权限类确认为环境问题或修复。

## 更新（2026-10-24 19:33 +08:00）：聚焦变更 fix-cloud-agent-test-contract-and-canvas-writeback

详见 [验证报告](../superpowers/reports/2026-10-24-fix-cloud-agent-test-contract-and-canvas-writeback-verify.md)。本变更尚未验证/归档，以下是实施结果：

- 节点数量断言：已确认为测试合同漂移（现行 13 个类型），改为精确集合校验并加负向控制。
- Agent runtime exited: exit status 1 两项：根因是缺 `backend/agent-runtime/pi/node_modules`，已 `npm ci`，不是产品回归；上文把它们列为'疑似真实回归'的初判作废。
- `ErrCreationConflict` 三项：根因是测试夹具里已批准媒体的后台等待器不遵守 `disablePiRuntime`，与测试直接推进竞争；夹具关闭后 10 轮全过（修改前 8/10 失败）。产品侧等待器不遵守该标志仅记录，未改。
- `go test ./internal/app -run TestCloudAgent`：347 通过 / 0 失败 / 1 跳过（需要真实 PG）。
- fsync、缺 Redis、权限位、Gmail 临时文件占用及其他约 10 个用例仍未处理，归 F09。
