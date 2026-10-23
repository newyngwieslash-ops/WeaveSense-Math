# 目标

修复F04已复现的测试进程内会话状态污染、Windows路径/换行/临时模块加载及CI测试shell执行边界，使既有业务与拒绝断言真正执行、结果可复现。本变更只维护测试基础设施与测试准备/清理，不改变生产鉴权或业务行为。

# 范围

当前目录、当前分支 `codex/fix-workspace-member-controls-and-analyst-permissions` 继续，基线HEAD `1941f6a91f6f36af0256acdcfdf69a1d5ea9edd9`，保留全部现有修改和数据。单一聚焦Native变更，主会话顺序执行，不启动子代理、新聊天或Supervisor子变更。

1. 追踪 `cross-tab-session-cache.test.ts` 的全局window/storage、UserStore、auth缓存和user-scope准备/清理，修复其与后续成员、报价、上传、Skills测试的相互影响；appearance测试以现有授权scope入口模拟真实账号切换，不直接改共享存储冒充已确认身份。
2. Windows执行入口：`application-bootstrap.test.mjs`、`agent-launcher-position.test.mjs`使用正确文件URL到本机路径转换；`http-ownership.test.ts`按平台路径分隔符规范化允许名单；`welcome-title.test.mjs`处理LF/CRLF且执行当前真实入口的依赖。
3. `agent-api-reliability.test.ts` 的临时副本完整解析实际运行依赖，保留真实会话守卫和SSE协议检查，不用总返回成功的鉴权替身掩盖问题。
4. `ci-image-release.test.mjs` 使用可核实的本地Bash执行测试fixture，正确传递本机/Posix路径及换行；确认mock docker/gh实际收到预期调用，避免WSL启动失败被误计为业务拒绝通过。不得调用真实远端Docker/gh操作。
5. 必要改动限于 `web/test/` 的现有文件及确有复用价值的最小测试helper、相关普通文档。新回归检查覆盖顺序运行、路径/换行及fixture隔离，不替换全量测试为按文件隔离运行。

## 证据覆盖与边界

上轮 `docs/flow/2026-10-23-f03-forensics-f04-baseline.md` 是调试证据而非全项目需求来源。本轮只覆盖F04“会话状态污染与Windows执行边界”及其安全约束；不承接全部40工作包、完整PRD或其他静态UI合同。已完整阅读其F04分组与对应实际测试代码；以下事实用于定位，正式RED仍在完整Shape确认后补充并留日志。

| 已知证据 | 使用方式 | 验收 |
| --- | --- | --- |
| 会话专项10pass→成员1fail/AbortError，成员单跑pass；另外四组单跑通过 | 最小顺序回归及跨文件影响核对 | A1、A2 |
| Windows URL.pathname、relative分隔符、临时别名缺失、welcome VM错误 | 本机路径/模块/换行执行边界，不推定产品故障 | A1、A3、A4 |
| CI五个成功路径返回1；默认bash为WSL且--version退出1，Git Bash存在 | 先验证测试依赖和mock命中，再执行原成功/拒绝断言 | A1、A5 |
| 2234pass/40fail/2errors、38去重名字 | 广域基线；非当前范围的失败逐项保留 | A6 |
| 原F03缺基线，数据/旧dirty保护 | 不恢复、不操作18088，本轮无产品数据写入 | A7 |

# 非目标

不改 `web/src/` 生产代码、后端、Engine、RLS、API、数据库、生产CI发布脚本或依赖版本；不清缓存、不恢复原F03、不部署18088，不实际调用模型、MCP、上传或外发，不启动dev server、子代理或新聊天，不提交、归档、推送、合并或发布。若事实要求修改生产行为，先回Shape，不静默扩大范围。

选项数量9/11、媒体/复制/引用/CSS等静态UI合同漂移不是本轮目标；不能为消除它们删功能、改期望或放宽比较。Linux真实CI执行无可用环境时记录未执行，不能用Windows/Git Bash或path.posix合成检查替代真实Linux通过。

# 验收示例

- A1：完整Shape确认后先补最小可运行回归检查，实际观测RED后再修复；至少覆盖会话专项后正常受保护请求/顺序运行、Windows本机文件入口和CI fixture启动/调用命中。已有加载失败和旧日志只作参考，不冒充本轮新增RED；反证成立时记录并不为猜测改代码。
- A2：同一Bun进程中，会话专项及后续成员、报价、资源上传边界/进度、Skills的既有断言通过；appearance经正式scope切换仍按账号隔离。旧身份失效、scope漂移、在途请求取消、迟到响应拒绝和跨账号持久化保护的负向断言保留并通过，测试退出恢复全局环境且不留下会话状态影响。不得只靠全量改为逐文件独立进程规避污染。
- A3：bootstrap和launcher真实入口能在Windows驱动器、含空格/中文路径的必要检查中正常构建执行；HTTP所有权允许名单在Windows/POSIX分隔符下语义相同，仍能发现非允许调用者。用标准库路径转换，不删生产守卫或扩大允许名单；实际平台和合成路径样本明确区分。
- A4：临时Agent模块能解析当前真实依赖，原SSE/审批/重连/游标检查实际执行，真实会话保护不得改为恒成功替身；welcome入口的LF/CRLF检查保持同一业务断言，不出现残留import或未绑定依赖导致的VM异常。测试仅替换既有边界副作用，未知模块加载失败仍可见。
- A5：CI镜像测试选择可执行本地Bash并正确传递fixture路径/环境，原成功、缺manifest、错误拒绝、stale main、tag及缺架构等断言全部实际执行；有可审阅调用证据证明mock docker/gh被使用，真实程序不得回退执行。shell或fixture依赖坏掉应报测试环境失败，不能当业务拒绝通过，不跳过或删除用例、不下载安装新软件。
- A6：上述相关测试单独和组合运行均GREEN；正式必要TypeScript/Vite构建与lint通过。最终全Web实际运行一次，原始退出码/通过失败错误数与逐名差异留证；本范围失败必须消除，其他旧债如仍在则逐项保留，不冒称全量GREEN或发布通过。新增或暴露的失败先分类，范围外产品问题另记，不用删改断言抵消。
- A7：只改证实的测试边界与相关文档，所有触及入口及其测试调用方追踪记录可审阅，优先复用既有helper；无新增依赖/生产测试后门、缓存清理或真实业务请求。更新任务、pending-test、报告和时间戳交接，保留当前分支、所有旧修改与数据；无子代理独立Verifier时如实记录，结果接受和归档分别另按本次流程确认。

# 约束与不变量

用户明确要求不放宽生产鉴权，不删除、跳过或降低测试断言，不清缓存、不调用真实模型、不启动子代理、不推送合并发布。不得用NODE_ENV或生产bypass满足测试。测试中的storage/window/fetch/timer修改只能作用于自建替身并严格恢复，不能操作现有浏览器站点存储。

CI只运行自建本地fixture。shell解析或mock路径不可用必须失败，不能落到真实docker、gh或网络程序；路径写入/清理限定本次测试创建的临时目录，删除前确认解析路径，不删除现有数据。运行日志脱敏，不输出环境凭据。普通已有编辑和F03阻塞现场保持。

# 决策

用户已选择保留当前分支与目录，无需再询问隔离方式。采用一个聚焦Native变更，顺序TDD；本轮主会话执行，不沿用旧归档变更的审查/交付授权。标准库fileURLToPath/path优先，复用现有source-contract/module-group-source仅在合同不被放宽时使用；只有实际消除重复才新增最小helper，不加通用测试框架。

工件准备阶段不修改测试或产品源码；初始请求不替代本次完整Shape最终确认。正式确认后的计划先观察新RED，再最小修复、专项与广域验证。

# 待解决问题

没有尚需用户选择的行为分歧或隔离决策；当前等待Runtime准备完整Shape最终确认。事实待验证：会话清理的最小根因、Git Bash fixture完整路径语义及LF/CRLF下真实入口依赖，均由Build内最小检查确认；如需改变范围则回Shape。

# 验证预期

主会话先执行最小RED并留原始日志，再修对应测试入口。相关专项至少含cross-tab-session-cache、intelligence-member-api、model-quote、resource-upload-boundary、resource-upload-progress、skills-api、canvas-appearance、application-bootstrap、agent-launcher-position、agent-api-reliability、http-ownership、welcome-title、ci-image-release及新增边界检查。检查组合顺序影响，不将独立进程作为唯一GREEN证据。

正式候选交Runtime运行必要专项、build/lint；全Web广域另保留真实退出码及失败名称对比，范围外旧失败不能自动计为通过或无故移除。明确记录Windows实际、Git Bash实际、合成路径及Linux未执行的区别；fixture不是live发布、模型或业务权限验收。没有独立Verifier的限制需用户本次明确接受，不预填通过；本次Shape不授权归档或提交。
