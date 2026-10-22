# TSV导入拆列聚焦设计与任务

状态：用户已确认完整Shape，实际正式RED→GREEN、Runtime专项/构建/lint和18088保卷UI/PG复验完成；用户已明确接受本次无独立Verifier结果，随后另行确认本地归档。Native六项通过、status=done、archived=true、当前工作区kept；本次工件已归档到docs/superpowers/comet/archive/2026-10-22-fix-canvas-tsv-import，新主规格已创建。当前分支及其他修改保留，基线53ef897e。此文件是普通设计/任务计划，正式要求以归档brief/spec为准。

## 问题与入口

前轮真实TSV导入2条但字段为单键`id\ttitle\tscore`，本地真实解析器诊断一致。handleFileImport存fileName/rawText→detectDataSourceFormatFromFilename将.tsv归csv→parseDataSourceNode→parseCsvItems→splitCsvLine；末层只识别逗号。parseCsvItems的生产调用方只有共享数据源入口，extractNodeItems/evaluateCanvasDataflow覆盖预览/下游/重开。所有通用函数及相邻测试已检索，不为前轮连线现象改代码。

## 最小设计

复用当前CSV扫描及标量转换，给parseCsvItems/splitCsvLine传可选的明确分隔符（默认逗号）；仅共享入口的csv/.tsv选择tab。TSV使用原始sourceText，拆物理行前去文件起始BOM、忽略纯空白行，不先trim掉真实首尾空列；普通CSV的默认路径不变。参数与循环即能解决，不复制TSV解析器、不加自动探测或依赖。

引号内tab/逗号、双引号转义复用现有扫描；单行、空表头col_N、空值、trim/标量转换仍沿当前规则。不借本轮新增跨物理行引号字段或CSV严格校验。实现预计只需dataflow-evaluator.ts与一份小测试，文档另同步；实际以最小可验证diff为准。

## 实施任务

- [x] T1：正式最小RED为0通过/1失败，扩展RED为1通过/4失败；只新增一份无mock测试，覆盖中文、LF/CRLF/BOM、空列、文件名、单行/空文件、引号/逗号及显式格式隔离。
- [x] T2：共享扫描器传分隔符，仅csv/.tsv用tab和原正文；GREEN16项/73断言，全部调用方及默认CSV检查通过，无新依赖或独立解析器。
- [x] T3：Runtime专项、TypeScript/Vite构建、UI lint均exit0，dispatch复用检查；全Web2214通过/40失败/2错误，与上轮日志38个失败名称完全一致，无新增；功能/待测试/报告已同步。独立Verifier未启动，不称独立审查通过。
- [x] T4：仅18088 web保卷更新；新图pxVfkXyaXHR0s7Z8AndTK的真实选择器正常/空列TSV、下游title/flag、刷新及PG revision6/2节点/1连线通过。原七图、10文档/1报告、零模型预算及非web容器指纹保持；回退镜像/overlay留存，不称纯HEAD镜像。

## 确认与交付边界

用户当前目录和分支已明确，不重复询问。本轮完整Shape须明确确认才能Build；无独立Verifier结果若出现仍须本次独立接受，不沿用旧豁免。最终本地归档另待确认。本轮不启动代理/新聊天、清缓存、调用模型、外发、生产迁移或push/merge/release。TSV以外的U03/连线/重连不纳入。
