# JSONL文件导入聚焦设计与任务

状态：实施和主会话验证完成；用户先明确接受本次无独立Verifier结果，再单独确认本地归档，Native已done/archived=true、六项通过、当前分支kept。变更`fix-canvas-jsonl-import`归档在`docs/superpowers/comet/archive/2026-10-22-fix-canvas-jsonl-import`，实施基线6b93ce3c。全量旧失败及未执行独立审查不因归档而消除；此文件是普通实施计划。

## 已核对入口与调用方

| 入口 | 现状及修复责任 |
|---|---|
| DataSourceNodeContent.handleFileImport | file.text后按扩展识别，.jsonl仍存format=json、fileName和rawText；保留现有入口 |
| detectDataSourceFormatFromFilename | .json/.jsonl同归json；不新增枚举或UI页签 |
| parseDataSourceNode | 根据真实格式和.jsonl标识选择逐行解析，原有普通格式分支保持 |
| parseJsonItems | 普通JSON/情报、TXT试探及extractNodeItems普通文本调用；保持整份严格语义，不塞全局JSONL回退 |
| extractNodeItems→evaluateCanvasDataflow | 数据源全部经共享边界；有效记录进入字段和逻辑，坏行0条，持久元数据重开同样生效 |

## 最小设计

在共享数据源入口限定json/.jsonl后逐物理行JSON.parse，复用现有toCanvasDataItem。标准库循环即可，不加依赖或通用格式框架。跳纯空白行，保留原行号；遇错立即整份0条及中文行号错误。一行一条，行内数组/对象沿用标准化，不用普通JSON的顶层数组/包装展开。不得改变原始rawText/fileName或依据当前内容改写其他用户缓存。

普通JSON仍整份解析；CSV即使残留.jsonl名称也尊重format。旧已保存JSONL节点无需迁移，重新求值即修复。实现优先只改dataflow-evaluator.ts及一个小测试文件，必要文档另更新；不为计划行数强行添加helper。

## 实施任务

- [x] T1：先最小RED。两中文对象预期2条/实际0条，随后4项新边界失败、1项原行为通过；测试无mock，覆盖空白/CRLF/BOM、坏行原行号、值标准化与严格JSON/格式切换。
- [x] T2：共享入口最小修复。仅parseDataSourceNode增加显式JSONL分支，保留parseJsonItems和所有其他调用方。专项及相邻数据流11项/58断言通过；全量同命令对照与原失败另记证据，不改TSV或拖接。
- [x] T3：Runtime专项11项/58断言、正式构建及lint退出0，dispatch复用相同记录；功能、pending-test和报告更新。全量旧40失败/2错误及无独立Verifier明确保留；六项在用户明确接受限制后才由Runtime通过，归档另获单独确认。
- [x] T4：18088仅保卷替换web，使用新增专项画布通过真实选择器导入合法和坏行JSONL，得到2条/原第5行错误及0下游；恢复合法正文后reload与PG revision7/2节点/1连线一致。原五张画布、实采数据与非web容器不变，模型/预算0。文件名仍bad-lines.jsonl，未伪称再次上传原文件。详见本轮验证报告。

## 不变量与确认点

不启动子代理或新聊天；旧变更Verifier豁免、归档/交付选择不沿用。本轮Shape须确认后才开始T1；本轮结果如缺独立Verifier需后续明确接受，最终本地归档仍另确认。全程不推送/Git合并/发布、不清缓存、不操作生产迁移。外部凭据不输出。
