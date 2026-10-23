# F04 测试会话隔离与 Windows 执行边界验证

时间：2026-10-23 17:22 +08:00，Asia/Shanghai。分支 `codex/fix-workspace-member-controls-and-analyst-permissions`，HEAD `1941f6a91f6f36af0256acdcfdf69a1d5ea9edd9`。用户已确认完整Shape；本报告记录本轮实际执行，不将历史报告或独立审查记为本轮通过。

## 结论与流程边界

正式最小RED后，测试准备/清理和Windows执行边界已修复；Runtime相关100项/367断言、正式构建和lint通过。实际全Web2300通过/8失败/0错误，仍未全量通过。保留原业务断言、安全拒绝和所有测试，无生产鉴权/API/RLS/发布脚本变化，无新增依赖。

Native候选 `8f1010d9-614f-41b9-b88f-74ff05a4696c`，iteration1/attempt1。用户禁止子代理与新聊天，因此没有获授权可运行的独立Verifier；dispatch仅登记，没有实际启动或独立结论。先如实提交verifier-unavailable，随后用户明确接受该有限结果，再另行确认本地归档。实际archive退出0，Runtime stateVersion8、done/archived=true，七项按该限制通过，工作区kept；归档预检ready=true、无阻塞、无需重验。已新增test-session-windows-execution主规格，工件位于docs/superpowers/comet/archive/2026-10-23-fix-test-session-isolation-and-windows-execution。精确提交与最终HEAD见收尾交接；本次本地归档没有部署、推送、合并或发布。

## 根因、共享边界与变更

会话调用链为cross-tab测试模拟window/storage→auth缓存与UserStore→user-scope固定scope/revision/AbortController→request和Agent SSE。跨标签测试beforeEach发布失效nonce，afterEach绑定guest时仍带测试nonce，随后恢复不含该nonce的window，后续受保护API正确抛AbortError。修测试自身的清理顺序及scope/UserStore恢复，保留生产守卫。appearance测试改用正式setActiveUserScope模拟账号确认，不再以直接写共享key冒充身份切换。

Windows路径使用标准fileURLToPath；HTTP允许名单比较仅统一目录分隔符，不扩大豁免。welcome执行真实main入口，兼容LF/CRLF并绑定当前真实依赖名。临时Agent模块图复制真实user-scope实现，保留会话拒绝、SSE重连/游标/看门狗、待确认持久化等原断言，补充“失效后0次transport”检查。

CI fixture默认Windows使用本地Git Bash，可用TEST_BASH指定已有安装；不安装依赖。源发布脚本只在测试自有临时目录规范换行，生产脚本不改。函数将docker/gh绑定到显式mock路径，shell-ready与tool-trace证明执行；缺shell是环境错误，不能冒充拒绝成功。保留原成功/失败业务期望，补mock stderr和调用证据。实际使用中文/空格临时目录；删除前校验绝对父目录和前缀，只删除本测试创建的临时数据，不清真实缓存。

| 文件 | 本轮作用 |
| --- | --- |
| `web/test/cross-tab-session-cache.test.ts` | 测试nonce、scope、UserStore和窗口恢复 |
| `web/test/canvas-appearance.test.ts` | 正式scope切换与测试清理 |
| `web/test/application-bootstrap.test.mjs` | 原生文件URL路径 |
| `web/test/agent-launcher-position.test.mjs` | 原生文件URL路径 |
| `web/test/http-ownership.test.ts` | 相同允许名单的跨平台路径比较 |
| `web/test/welcome-title.test.mjs` | 真实入口LF/CRLF及依赖绑定 |
| `web/test/agent-api-reliability.test.ts` | 临时依赖图真实session guard与SSE检查 |
| `web/test/ci-image-release.test.mjs` | 本地Bash/mock可验证执行与环境错误 |
| `web/test/test-execution-boundaries.test.ts` | 新增同进程组合及中文/空格文件URL回归 |

## 本轮实际检查

| 检查 | 结果 | 原始记录 |
| --- | --- | --- |
| 新回归与CI最小RED，修复前 | exit1，7通过/9失败/19断言，3.79秒；会话AbortError、路径/CRLF、Agent别名与mock未命中均实际失败 | `.local/f04-test-boundaries/red.log` |
| 首轮会话与入口GREEN | exit0，15通过/39断言 | `.local/f04-test-boundaries/core-green.log` |
| 首轮CI GREEN，补缺shell用例前 | exit0，13通过/96断言 | `.local/f04-test-boundaries/ci-green.log` |
| 最终14文件同进程相关检查 | exit0，100通过/367断言，11.99秒 | `.local/f04-test-boundaries/combined-dev.log` |
| Runtime绑定相关专项 | exit0，100通过/367断言，约13.34秒 | Runtime checks日志下f04-related |
| Runtime `bun run build` | exit0，约12.50秒 | Runtime checks日志下f04-build |
| Runtime `bun run lint` | exit0，约23.12秒 | Runtime checks日志下f04-lint |
| 最终 `bun test` | exit1，2300通过/8失败/0错误、17575断言、319文件/2308项，40.13秒 | `.local/f04-test-boundaries/full-web.log` |

Runtime日志目录：`.comet/runtime/native/changes/fix-test-session-isolation-and-windows-execution/logs/checks/`，本轮三份前缀为 `b0750d4d-5ea6-4709-816a-65f584902b08-`。Runtime提交候选实际执行检查，dispatch复用同候选有效回执，没有重复重跑或伪造记录。

最终相关命令在web目录执行：

```powershell
bun test test/test-execution-boundaries.test.ts test/cross-tab-session-cache.test.ts test/intelligence-member-api.test.ts test/model-quote.test.ts test/resource-upload-boundary.test.ts test/resource-upload-progress.test.ts test/skills-api.test.ts test/canvas-appearance.test.ts test/application-bootstrap.test.mjs test/agent-launcher-position.test.mjs test/agent-api-reliability.test.ts test/http-ownership.test.ts test/welcome-title.test.mjs test/ci-image-release.test.mjs
```

新增回归中会话两文件在同一子进程组合执行，不是把每个文件隔离后冒称全套通过；此外14文件组合和完整bun test也实际执行。中文/空格目录执行真实生产纯函数的Bun构建与VM断言，Agent检查使用真实守卫，均不调用真实外部服务。

## 全量剩余债务

历史当前HEAD基线来自 `.local/flow-uat-20261020/f03-forensics-20261023/full-web-current.log`：2234通过/40失败/2错误、318文件，去重失败名称38个。本轮逐名比较保存在 `.local/f04-test-boundaries/failure-comparison.json`：消除30个去重名称，新增0个，剩余8个。数量增长也包含原先加载失败而未执行的Agent用例和新增回归，不能将通过数差值等同新增功能数。

以下名称与全量日志一致，属于确认范围外的静态UI合同旧债，本轮不删、不跳过、不放宽：

1. Canvas keyboard keeps node copy as the fallback when no text is selected
2. canvas resource mention editor > refreshes async reference previews even when prompt text has not changed
3. collection select focus uses a thin keyboard ring, not a persistent pointer outline
4. connection menu omits the header and provides descriptions for every option
5. large canvas media rendering > decodes canvas images synchronously so zoom re-rasterization never paints blank frames
6. large canvas media rendering > keeps inactive video nodes on a viewport-gated static first frame
7. menu surfaces are explicitly scoped and old account inner overrides are removed
8. 图片上传占位 > 上传先占位再传输，完成替换同一节点，弹窗不等待传输完成

## 未执行、保护和发布门槛

- 未运行Linux真实CI、真实docker/gh发布、浏览器UI或PG验收；本轮本地Bash/mock不能代替远端CI或发布成功。
- 没有部署、重启、恢复或迁移18088，没有读取凭据值、模型调用、子代理、新聊天或推送/合并/发布。先完成结果接受，后按单独确认执行本地归档和精确提交；不据此扩大其他交付权限。
- HEAD和分支保持；旧canvas-cloud-agent-panel.tsx、两份runbook、历史文档、归档.comet.yaml和全部既有dirty/untracked继续保留。此次普通文档只增补本轮事实。
- 原F03历史图缺可信基线，恢复继续阻塞；本轮未重读PG或浏览器，不把此前17图/实采/零预算指纹当新保护检查通过。只操作测试自有临时数据。
- 8项全Web旧债、Linux真实CI、完整U/A验收、离线/冷启动/独立上下文后置项及G01交付仍未通过；原F03恢复仍阻塞。本次无独立Verifier的有限结果接受已经完成，不能扩大为独立审查、完整PRD验收或发布通过。
