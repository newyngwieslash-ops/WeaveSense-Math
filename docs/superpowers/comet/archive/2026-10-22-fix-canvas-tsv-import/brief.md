# Outcome

修复已确认的TSV拆列失败：现有文件入口接受.tsv，但共享CSV拆分器只识别逗号；两条三列中文TSV被错误转换为两个单键对象。修复后按制表符得到独立字段，预览、下游字段提取及保存重开一致，不依赖UI换格式或清缓存恢复。

# Scope

仅覆盖`fix-canvas-tsv-import`。用户明确保留当前目录、分支`codex/fix-workspace-member-controls-and-analyst-permissions`及所有已有修改，基线`53ef897e349317a978fe324773e268411a48e93f`。沿用format=csv/fileName/rawText持久合同，不新增tsv枚举、页签、API或表字段。

入口为handleFileImport→detectDataSourceFormatFromFilename→parseDataSourceNode→parseCsvItems→splitCsvLine，数据源经extractNodeItems/evaluateCanvasDataflow进入预览、下游和重开。已核对parseCsvItems生产调用方仅parseDataSourceNode；纯函数默认CSV调用保持。前轮U03验收账及本地诊断脚本只作故障证据/实现参考，不是本轮完整需求源；已归档JSONL不重开、不改主规格。

# Non-goals

不改普通CSV、JSON/JSONL、TXT/Markdown/intel合同，不按正文猜分隔符，不增加多行引号字段、严格坏行报错或自动纠错框架，不扩修重复表头/行宽等既有行为。不实现直接端点重连或其他U/A问题，不加依赖、流式大文件框架或迁移，不开展模型、外发、子代理、新聊天、生产迁移及G01交付。

# Acceptance examples

- 实际格式为csv且文件名trim后不区分大小写以.tsv结尾时，合法两条三列中文TSV得到两条独立id/title/score对象，顺序和中文正确，数字/布尔等沿用既有标量转换；LF与CRLF均通过，不再把制表符粘进列名。
- TSV保留首尾空列的位置，空表头沿现有规则命名col_N，缺失/空值沿现有规则为空字符串，UTF-8起始BOM和空白行不产生伪记录；空文件0条。只有一个非空物理行时沿用当前CSV单行行为，得到非空单元格列表，不另行引入表头模式。
- 复用既有引号扫描：引号内制表符不作为列分隔、双引号转义保持，逗号在TSV单元格内是普通数据；不新增跨物理行字段能力，也不改变当前trim/标量转换和宽松引号边界。
- 只有csv/.tsv组合走制表符；普通.csv、无文件标识CSV及parseCsvItems默认调用仍按逗号，不启发式识别tab。残留.tsv名称但用户明确切到JSON/JSONL、TXT、Markdown或intel时尊重实际格式，JSONL现有显式规则不变。
- 本轮Shape确认后先新增最小正式失败测试并实际RED，再修共享入口GREEN；TSV专项、现有dataflow与JSONL回归、正式TypeScript/Vite构建及UI lint通过。前轮诊断RED不代替本轮TDD；旧全Web40失败/2错误及未执行项如实保留，不宣称全量通过。
- 18088仅保卷更新必要web后，使用真实选择器导入正常/空列等TSV样本，核对结构化三列、下游提取及刷新/重开、PG正文/格式/连线；原七张画布、10文档/1报告、模型/预算和非web容器指纹保持，测试另建专项图不覆盖已有编辑。构建含其他dirty明确标注，不称纯HEAD；无模型或清缓存。

# Constraints and invariants

保留当前分支、所有旧dirty/未跟踪、历史handoff及未知缓存；不reset/clean/restore、不git add .。web使用Bun，业务调用走现有http，宿主后端必须显式指定调试数据目录。本轮若部署只更新18088隔离web、保留卷及可回退镜像，不操作8088或生产迁移，不启动dev server。

不消耗真实模型额度、不外发、不启动子代理或新聊天、不推送/合并/发布。新变更的完整Shape、无独立Verifier结果接受及最终本地归档分别遵守本轮确认点，不沿用旧变更授权。

# Decisions

用户已明确当前目录/分支继续，无需再问隔离方式。采用单一聚焦Native变更；两条三列TSV及下游title提取是可审阅目标。限定扩展名与实际格式而不做自动分隔符推断，复用既有CSV引号/转换和单行规则，避免影响已通过CSV及JSONL。实施选择为给现有拆分器传明确分隔符，TSV使用原正文防止trim吞掉前导空列，不复制解析器。详细设计及四项任务见`docs/superpowers/plans/2026-10-22-fix-canvas-tsv-import.md`。

# Open questions

无尚待调查的阻塞问题。以上具体结果、六项验收和非目标交Runtime准备本轮完整Shape确认；不在确认前修改产品或新增正式测试。用户禁止子代理，后续无独立Verifier限制必须如实记录并单独取得本变更结果接受。

# Verification expectations

开发先最小Bun失败检查，使用真实dataSource metadata/fileName/rawText；复用现有测试方式，不mock解析器。Builder handoff将最终专项/相邻回归、build及lint计划交Runtime执行；正式记录匹配当前候选时复用，避免无变化重复检查。18088文件旅程和PG保留是独立外部验收步骤，不塞入可重试的构建命令。

主会话核对所有调用方、格式边界及原数据指纹，持续维护tasks、功能/待测试和实际证据。独立Verifier未经授权不启动，不虚构通过；若环境阻塞则逐项列明未验，不能用源码替代真实UI。
