# F04测试隔离与Windows执行边界：设计及任务

时间：2026-10-23，Asia/Shanghai。Native `fix-test-session-isolation-and-windows-execution`，用户先明确接受没有独立Verifier的有限结果，再另行确认本地归档并保留当前分支；实际archive退出0、stateVersion8、done/archived=true、七项按限制通过、工作区kept。归档工件位于docs/superpowers/comet/archive/2026-10-23-fix-test-session-isolation-and-windows-execution。当前分支 `codex/fix-workspace-member-controls-and-analyst-permissions`，归档前HEAD `1941f6a9`，精确提交与最终HEAD见收尾交接。没有部署、推送、合并或发布。

## 已知链路与最小设计

会话测试修改window/storage→UserStore/认证缓存→user-scope的固定身份/revision/checking与AbortController→request和Agent SSE守卫。只恢复window不能证明模块状态恢复；先新回归检查观察真实失败点，再在测试生命周期或相关测试准备入口修一次。身份校验拒绝本身是应保留的安全行为，不能生产bypass。appearance使用正式scope切换入口，不能将直接改共享key作为成功登录。

Bun.build调用方只有已定位两个测试入口使用URL.pathname；优先fileURLToPath。http-ownership复用node:path后规范化比较用分隔符，不扩大豁免。临时Agent副本读取当前真实依赖，保持会话守卫；welcome VM按LF/CRLF执行同一入口断言。只在重复成立时复用/新增小helper，不引入测试框架。

CI测试当前spawn命中Windows System32的WSL Bash，--version退出1（E_ACCESSDENIED）；本机Git Bash已存在，正式Build再验证fixture路径与mock命中。测试必须先证明自身依赖正常，拒绝断言才有意义；真实docker/gh不能回退执行，禁止实际远端操作或发布。生产.github脚本不在修改范围。

## Tasks

- [x] T1：确认完整Shape后新增正式RED：7通过/9失败/19断言，退出1。新组合检查复现会话AbortError、Windows入口/路径/CRLF、Agent依赖和CI未命中mock，原始日志已保存（A1）。
- [x] T2：修测试自身nonce清理和窗口/scope/UserStore恢复；appearance按正式入口切账号。同进程组合五组API与安全拒绝通过，生产鉴权未改（A2）。
- [x] T3：两个入口用fileURLToPath，HTTP比较统一分隔符；临时Agent副本使用真实session guard，welcome兼容LF/CRLF；CI使用本地Bash及显式fixture工具函数、调用轨迹和环境错误断言，原业务断言保留（A3–A5）。
- [x] T4实施与记录：Runtime专项100通过/367断言、build/lint退出0；全Web2300通过/8失败/0错误，逐名对照无新增失败；报告/pending/新handoff已更新（A6–A7）。
- [x] T4结果接受与归档：用户分别确认有限结果和本地归档，Runtime七项按限制通过、预检ready=true无阻塞，实际archive退出0、done/archived=true、工作区kept。Linux真实CI与8项旧失败仍未通过，原F03恢复继续阻塞。

## 验证与保护

前轮当前HEAD全Web2234pass/40fail/2errors、38去重名字；此为引用，非本轮新RED。CI、路径与会话修复须实际使目标用例执行，不能将依赖错误转为跳过。最后广域全量可能仍有选项/媒体/键盘/引用/CSS静态旧债，逐项列明而不假造GREEN。

本轮仅测试和文档，无部署或18088操作，原F03恢复继续阻塞。旧修改与未跟踪文件不动，不清缓存、不调用模型、不启动子代理/新聊天，不推送合并发布；独立Verifier接受、归档和提交不由本次Shape自动授权。

实际结果与逐名剩余债务见 [验证报告](../reports/2026-10-23-fix-test-session-isolation-and-windows-execution-verify.md)。此前40失败/2错误保留为历史基线，不代表当前结果；本次未运行Linux真实CI或浏览器/PG验收。
