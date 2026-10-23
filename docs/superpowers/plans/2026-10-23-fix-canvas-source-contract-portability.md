# F05画布源码合同可移植性：设计与任务

时间：2026-10-23，Asia/Shanghai。归档前HEAD b7d6f5d4，分支codex/fix-workspace-member-controls-and-analyst-permissions。Native fix-canvas-source-contract-portability：用户先明确接受无独立Verifier有限结果，再单独确认本地归档并保留分支；实际archive退出0，stateVersion8、done/archived=true、六验收按限制通过、工作区kept。工件已移入docs/superpowers/comet/archive/2026-10-23-fix-canvas-source-contract-portability并新增主规格。精确提交及最终HEAD见收尾交接；没有部署或推送合并发布。

## 设计依据

原6文件实际60通过/8失败/419断言。7项失败断言仅在内存把CRLF转LF即成立，保留原文件与原测试不改；第8项固定9条，而bfcf5b1b已增加DataSource/Logic，目前11个唯一条目均有描述和正确类型。当前证据支持测试合同边界问题，未证实生产缺陷；正式新RED须在完整Shape确认后补充。

现有moduleGroupSource读取分组源码，17文件引用，消费者检查源码片段/结构，不消费换行字节协议；在现有helper内复用最小只规范CRLF的读取能力，不使用code()的全空白折叠。六失败文件改走该边界，保持原语义断言。连接菜单保持精确计数并补完整标题/描述/类型映射、条件绘图真实SSR与禁用守卫；不改生产菜单以恢复旧9项。

## Tasks

- [x] T1（A1/A4）：确认后新增正式RED，实际2通过/12失败/30断言，退出1；读取能力、混合分组和旧计数失败留证，不用Shape旧诊断替代。
- [x] T2（A2/A4）：现有helper新增只规范CRLF的readSourceText，六测试接入，原7项及全部原语义断言保留；真实读取LF/CRLF、BOM/孤立CR/中文空格和错误路径、12个语义变异控制通过，无生产缺陷修复。
- [x] T3（A3/A4）：菜单精确11项标题/描述/创建类型/禁用类型映射，实际组件SSR11/10及禁用说明通过；原header/箭头/BatchTable/创建guard断言均保持。
- [x] T4实施（A5/A6）：原六文件、所有helper消费者和相邻F04等35文件369通过/2286断言；Runtime同专项/build/lint通过；完整Web2320通过/0失败/0错误、17993断言，原8失败消除、新增0；报告/pending/handoff更新。
- [x] T4结果接受与归档：用户分别明确接受有限结果和确认本地归档，Runtime六项按限制通过；预检ready=true无阻塞，实际archive退出0、done/archived=true、工作区kept。仅本次文件精确提交，其他修改保留；部署/推送合并发布仍未授权。

## 核对范围

测试入口：canvas-keyboard、canvas-resource-mention-editor、canvas-connection-create-menu、canvas-media-performance、canvas-image-upload、workspace-menu-density。共享helper调用方包括admin-ui-regressions、appearance-bootstrap、assets-collection-layout、canvas-image-source、canvas-node-drag-overlays、canvas-node-stack-order、canvas-selection、create-canvas-handoff、create-library-button、director-keyframe-delete-wiring、director-onboarding、director-template-mode-wiring、scrollbar-style-source及上述两个失败消费者；具体列表以实施前rg的完整集合为准，17引用含helper自身，不将计数当独立测试数。

实际16个测试消费者还包含site-appearance-and-skins；与原六文件/新增回归/F04相关并集为35文件，精确命令清单在.local/f05-ui-contracts/related-files.json及Runtime回执。新回归复用原8个具名测试断言，仅对真实源码临时副本控制换行或注入错误，不修改src；提取依赖这8个已知具名块的明确边界，边界变化会报错，不冒充通用JS解析器。

实际检查/过程失败及限制见[验证报告](../reports/2026-10-23-fix-canvas-source-contract-portability-verify.md)。

## 保护

仅测试和文档；不改web/src/backend/services/.github或依赖。当前分支及全部旧dirty/untracked、18088和F03现场保持，不清缓存、不调用模型、不启动子代理、新聊天或dev server，无部署、提交/归档或推送合并发布。独立Verifier及本地归档各自另按本次规则处理。
