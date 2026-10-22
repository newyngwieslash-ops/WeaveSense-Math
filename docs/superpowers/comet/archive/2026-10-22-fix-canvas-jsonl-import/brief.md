# Outcome

修复画布数据源选择器接受合法JSONL文件却解析失败的问题：每个非空物理行解析为一个标准化记录，保留顺序和中文内容，预览/下游求值及保存重开一致；坏行明确失败，不输出部分成功数据。

# Scope

仅覆盖 `fix-canvas-jsonl-import`。用户已选择保留当前目录和分支`codex/fix-workspace-member-controls-and-analyst-permissions`，基线`6b93ce3c304eaa1f713622a4c2e5090f2362f3ab`；现有dirty和未跟踪文件均非本变更自动所有。

入口为DataSourceNodeContent.handleFileImport→detectDataSourceFormatFromFilename→持久配置format=json/fileName=*.jsonl→parseDataSourceNode→extractNodeItems→evaluateCanvasDataflow。修复共享数据源解析边界，同时覆盖已保存JSONL节点的重新求值，不新增存储字段、format枚举或表结构。

前轮U03验收账与fixture仅作故障证据/实现参考，非本轮全量需求源；根因是.jsonl被归为json后整份JSON.parse。parseJsonItems另被普通JSON、情报、TXT试探及普通文本节点调用，其严格JSON合同不扩展。Native发现的会话缓存能力是关键词误关联，已通过Runtime撤销，仅移除本轮自动复制的草稿，原主规格和历史归档不变。

# Non-goals

不修TSV、手工拖接/重连、复杂环、Skill或其他U/A问题；不增加JSONL专用UI页签、流式大文件框架、宽松JSON/评论/自动纠错或依赖。不改普通JSON、API、后端、数据库、RLS、账号scope及模型/计费合同。浏览器离线/冷启动/独立Profile继续最后处理。

# Acceptance examples

- `.jsonl`（含大小写扩展）通过现有文件入口进入共享求值器时，两个中文对象按行和原顺序得到2条，字段原样；LF、CRLF、首尾空白/空白行及UTF-8 BOM不产生伪记录，空文件为0条。
- 每个非空JSONL行独立使用标准JSON解析，单行对象/标量/数组/null复用既有CanvasDataItem转换且一行一条，不将对象的items/rows/data等包装字段或数组展开成跨行记录。
- 任一非空行非法时整份返回0条和包含原物理行号的中文JSONL错误；前面已解析行不得作为部分成功结果流入下游，不静默跳坏行、回退TXT或执行内容。
- 只有format=json且文件名明确为.jsonl的持久数据源使用JSONL规则；普通.json与无JSONL标识的JSON保持整份严格解析，原TXT/intel/CSV/Markdown及手工切换CSV但仍保留.jsonl文件名的节点不被强制改回JSONL。
- 先补最小失败测试实际RED，后修共享入口GREEN；JSONL专项、现有数据流6项相邻回归、TypeScript/Vite构建和UI lint通过，保留真实失败和未执行边界，不调用模型或子代理。
- 18088只保卷更新必要web后，通过真实文件选择器导入合法/坏行JSONL，核对2条/行号错误、下游与刷新重开、PG持久结果及原画布/文档/报告指纹；保留所有现有数据/缓存。若镜像包含其他dirty明确记录，不称纯HEAD；部署/浏览器受阻则如实列未通过，不能用源码单测代替。

# Constraints and invariants

不清localStorage/IndexedDB/Cookie/HTTP缓存，不覆盖未知本地编辑；原五张画布及实采数据保持，本轮验收只使用专项测试图或新节点。web使用Bun，业务请求走既有http，不启动dev server。禁止真实模型/付费调用、外发、生产迁移、子代理、新聊天、推送/Git合并/发布。归档、提交和交付遵循各自后续确认，不沿用旧变更授权。

# Decisions

用户本轮已明确选择当前分支/数据保留及先工件、最小失败检查后修复。采用单一Native变更，不拆子任务代理，不复用旧Verifier豁免为本轮通过。

拟定行为边界：显式JSONL文件名而非对所有坏JSON启发式回退；忽略纯空白行以符合既有导入交互，坏非空行原子失败；行号按原物理行计算，包括被忽略空行。对象/标量/数组/null复用现有标准化，一行一条。详细设计与四项任务见`docs/superpowers/plans/2026-10-22-fix-canvas-jsonl-import.md`，待完整Shape确认后实施。

# Open questions

无尚待调查的阻塞问题；上述具体结果/范围/验收和非目标待Runtime准备后的用户完整确认。此句不代替确认，不在确认前编辑产品代码或新增失败测试。

# Verification expectations

开发阶段先在现有Bun数据流测试或单一小专项中使用真实metadata/fileName/rawText复现前轮两个中文对象失败；只运行最小必要检查。提交Builder时由Runtime运行最终JSONL及相邻专项、正式构建/lint，避免完整检查无变化重复执行。18088真实文件旅程、刷新和PG指纹为独立外部验收步骤，不能放进可重试构建命令触发部署或数据改写。

主会话核对所有共享调用方和最小diff。用户禁止子代理，本轮独立Verifier未授权，不启动、不虚构通过；如Runtime要求，记录此限制并在本轮结果接受时另获明确接受，不能沿用旧变更接受。归档另行确认。
