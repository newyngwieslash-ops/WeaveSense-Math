# TSV拆列修复验证记录

时间：2026-10-22 21:08 +08:00（归档收尾于21:19），Asia/Shanghai。变更`fix-canvas-tsv-import`，分支`codex/fix-workspace-member-controls-and-analyst-permissions`，基线`53ef897e349317a978fe324773e268411a48e93f`。用户已分别接受验证限制和确认本地归档；未调用模型、清缓存、启动子代理、新聊天、推送、合并或发布。

## 结论与当前门槛

TSV拆列共享修复已完成正式RED→GREEN、16项专项/73断言、Runtime正式构建/lint及18088真实文件/下游/刷新重开/PG保留验证。用户已明确接受本次没有独立Verifier的结果，并单独确认本地归档；Native Archive/stateVersion8、status=done、archived=true、六项通过、workspaceFinishResult=kept。归档预检及实际archive均exit0，新主规格canvas-tsv-data-source-import已创建，工件在docs/superpowers/comet/archive/2026-10-22-fix-canvas-tsv-import。不能把主会话核对写成独立审查通过，归档不等于发布。

全Web测试2214通过、40失败、2错误；与上轮同工作区JSONL日志逐名比较，没有新增失败，38个唯一失败名称完全一致。这是跨轮日志对照，不是本轮干净HEAD或仅撤回TSV补丁的全量基线对照，不称全量通过。

## 根因、调用方和最小修改

真实文件入口`handleFileImport`保存format/fileName/rawText；`.tsv`既有识别归入csv，但`splitCsvLine`只识别逗号，使两个三列数据行被转换为两个带tab的单键对象。全部生产调用为`parseDataSourceNode→parseCsvItems→splitCsvLine`；预览、字段列表与下游经`extractNodeItems/evaluateCanvasDataflow`共用入口。已用现有CodeGraph只读探索及rg追踪，没有建立索引。

仅修改`web/src/lib/canvas/dataflow-evaluator.ts`：原扫描器接收分隔符，`parseCsvItems`默认逗号，只有csv/.tsv选tab。TSV使用原正文、去文件起始BOM，拆分前保留整行首尾tab，只以trim判断空白行；引号、单元格trim、标量及单行列表复用原逻辑。不复制解析器、不加依赖、枚举、API、表结构或RLS修改。新增一份无mock的`web/test/canvas-tsv-import.test.ts`。

## 六项验收的主会话证据

| ID | 实际证据 | 限制 |
|---|---|---|
| A1 | 两条中文三列、数字类型、LF及CRLF专项通过；真实rows.tsv显示id/title/score，title下游为制表甲/乙 | 合成TSV走真实应用，不冒称实采 |
| A2 | 首尾/内部空列、col_N、缺值、BOM/空白行、单行/空文件专项；真实大写empty-columns.TSV显示col_1/title/flag/col_4 | 单行沿用现有非空单元格列表 |
| A3 | 引号内tab、逗号、双引号转义专项通过 | 没有新增多物理行引号或严格CSV语法校验 |
| A4 | 普通CSV/缺文件名/parseCsvItems默认、残留.tsv下显式JSON/TXT/Markdown/intel通过；既有JSONL5项通过 | 不猜分隔符，不扩展JSONL |
| A5 | Shape后正式最小RED0通过/1失败，扩展RED1通过/4失败，再共享修复GREEN16项/73断言；Runtime构建/lint退出0 | RED是断言差异，无导入/setup错误；完整Web旧失败单列 |
| A6 | 18088仅保卷替换web；新图真实正常/空列TSV、title/flag提取、reload、离开后从列表重开、PG核对与原七图指纹保持 | 构建含旧Agent面板dirty，不称纯HEAD；无独立Verifier |

以上为主会话证据对照，非Runtime独立语义验收结果。结构化字段以实际字段列表、下游与专项断言核对；本轮两次普通预览按钮点击未切换显示，未将其写成结构化卡片预览通过，也没有据此扩修UI。

## 实际检查

| 检查 | 结果 |
|---|---|
| `bun test web/test/canvas-tsv-import.test.ts`，最小正式RED | exit1，0通过/1失败 |
| 同命令，扩展RED | exit1，1通过/4失败、13断言 |
| `bun test web/test/canvas-tsv-import.test.ts web/test/canvas-dataflow.test.ts web/test/canvas-jsonl-import.test.ts` | 16通过/0失败/73断言；开发和Runtime各一次，Runtime exit0/2727ms |
| `cd web; bun run build` | Runtime exit0/20647ms，TypeScript与Vite均通过 |
| `cd web; bun run lint` | Runtime exit0/27739ms，项目UI退场规则；非全面风格审计 |
| `cd web; bun run test` | exit1，2214通过/40失败/2错误、17192断言，2254测试/316文件，49.29s |
| `git diff --check` | exit0，只有工作副本CRLF提示 |

Runtime候选`f71ecb3b-273c-4df0-8603-a06774396781`，Builder handoff执行三项正式计划，后续dispatch返回`reused`，没有重跑同一最终计划。首次提交check plan字段不符被CLI拒绝（要求id/executable/argv）；更正结构后成功，第一次未运行任何检查，不是产品失败。

## 18088真实旅程与PG

1. 用产品画布ZIP文件选择器新增`U03 TSV修复复验 2026-10-22`，ID`pxVfkXyaXHR0s7Z8AndTK`，2节点/1连线；原七张画布不修改。ZIP仅作合成验收图，文件内容仍从真实数据源选择器导入，没有隐藏store或SQL写入。
2. `rows.tsv`真实选择器导入：CSV实际格式、2条记录、独立id/title/score，下游title输入2/输出2，为制表甲、制表乙。
3. `empty-columns.TSV`真实选择器导入：起始BOM/CRLF/空白行及首尾tab，页面字段col_1/title/flag/col_4，2条；title输出甲1条（另一为空），改提取路径flag后输出true和false。
4. 再次选择器导入rows.tsv，恢复title提取，等待已同步后reload；两条/三字段与下游仍保持。离开回主页/画布列表后重新打开同一图，内容与结果一致且最终已同步。
5. PG只读核对revision6时payload_json：format=csv、fileName=rows.tsv、完整带tab/换行正文；2节点/1边及json_extract/title一致。首次只读SQL误用data列失败，查information_schema后以实际payload_json重查成功，没有写入或迁移。
6. `preserve.py after`退出0，原七图逐行md5保持；flow_documents为10条/`2c298b93a5c0490a95b9e3014d27d7b2`，flow_reports为1条/`7da5d5768498ec7615ad42c6908a1a26`，均与before相同；flow_model_tasks与flow_budget_reservations均0。platform/PG/Engine/scheduler的ID、镜像与启动时间保持，只新增一图。

截图已由真实浏览器工具返回；当前文档不声称生成独立本地图文件。结论以实际AX/DOM、Runtime和只读PG交叉证据为准。工具AX/DOM有渲染等待，首次返回加载中不当作失败，也没有重复导入制造第二张图。

## 镜像与回退

本轮从既有JSONL镜像复制Runtime构建产物web/dist，tag`flow-uat-web:tsv-import-20261022`，镜像`sha256:7e0654b95afb356d5bca32d7caac94da978ca2723c925097755742e8052db6fc`。revision标签为`53ef897e-tsv-working-tree`，包含原有Agent面板等dirty，不能称纯HEAD。

Compose叠加根flow→acceptance→member-controls→cross-tab-fix→jsonl-fix→tsv-fix，仅`up -d --no-build --pull never --no-deps web`，没有down/卷删除/后端或迁移变更。回退只需去掉最后tsv-fix overlay后同样仅重建web，原JSONL镜像`sha256:fe49170bca5f05298acbae94bc7b6417e274f5dcaf3e95818770e2f568323d9e`保留。回退方案未实际执行；8088与宿主dev server未动。

忽略目录`.local/flow-uat-20261020/tsv-fix`保留before/after、full-web-tests.log、failure-comparison.json、合成文件/ZIP、preserve.py、Dockerfile/overlay及CLI响应；uat.env值不输出或提交。

## 全Web失败名称与错误

下列38个名称对应40次失败实例，与上轮JSONL修复日志完全相同。两错误仍是临时模块`@/lib/user-scope`解析失败和Windows `/E:/...`路径BadPathName，原始日志保留，不由本轮TSV修复解决。

- Canvas keyboard keeps node copy as the fallback when no text is selected
- DEV previs lab loads without calling the appearance backend
- a pending skills response cannot replace the cache after installation
- added Skills retries a transient backend proxy failure
- appearance loads in parallel with normal startup: dev=false path=/dev/previs-repro
- appearance loads in parallel with normal startup: dev=false path=/login
- appearance loads in parallel with normal startup: dev=true path=/dev/previs-repro-other
- appearance loads in parallel with normal startup: dev=true path=/dev/previs-repro/
- appearance loads in parallel with normal startup: dev=true path=/login
- axios.create stays in the shared request client
- business API modules call http instead of axios or request(apiClient)
- canvas custom appearance > stores defaults locally with the active account scope
- canvas resource mention editor > refreshes async reference previews even when prompt text has not changed
- collection select focus uses a thin keyboard ring, not a persistent pointer outline
- commit image reuse > only a missing manifest permits a rebuild
- commit image reuse > reuses the exact existing digest
- connection menu omits the header and provides descriptions for every option
- digest promotion > current main promotes latest and SHA tags using digests
- digest promotion > stale main cannot roll latest back
- digest promotion > tag release promotes the exact version without touching latest
- large canvas media rendering > decodes canvas images synchronously so zoom re-rasterization never paints blank frames
- large canvas media rendering > keeps inactive video nodes on a viewport-gated static first frame
- member revoke uses workspace header, encoded ID and rejects business failure
- menu surfaces are explicitly scoped and old account inner overrides are removed
- model quote API targets > direct system videos send the actual options and cancellation signal to the catalog quote
- model quote API targets > logical models retain their dedicated quote route
- public film entry remains independent: /welcome
- public film entry remains independent: /welcome/
- welcome entry loads configured appearance
- 传输结束不吞掉服务端保存错误
- 分片上传累加片内真实进度，传输100%后仍须等待合并响应（日志原名含空格）
- 图片上传占位 > 上传先占位再传输，完成替换同一节点，弹窗不等待传输完成
- 普通 multipart 将传输进度换算为文件字节，总量未知不伪造百分比
- 资源直传失败分类 > 保留后端可读文案，并把 multipart 超限翻译成中文
- 资源直传失败分类 > 成功响应仍返回后端资源本身
- 资源直传失败分类 > 断网等非 HTTP 失败按瞬时处理
- 资源直传失败分类 > 鉴权、越权与体积超限属于重试不会自愈的永久失败
- 资源直传失败分类 > 限流与服务端故障属于瞬时失败，允许退回本机后重传

## 未执行、阻塞和跳过

独立Verifier依用户限制未启动。平台有子代理工具，但本轮无获授权的独立执行方式；Runtime将该限制归类为verifier-unavailable，不能把其通用“平台不可用”文案解释成工具不存在。用户已分别明确接受本次限制和确认本地归档，Runtime确认、归档dry-run与实际archive均exit0；当前分支保留。接受后未改产品，也未重复运行既有产品检查或部署，收尾只核对归档/提交与原工作文件保护。

未执行Go/后端构建、完整文档站构建、真实模型/外发/生产迁移；这些不在本次纯前端解析器范围。完整U03/Skill/复杂拓扑、直接端点重连、真实离线/冷启动/独立Profile及其余U/A、恢复迁移/监控/联合恢复/未知结果对账、实采外发与G01仍待。浏览器后置TODO继续有效；不实施O01–O06。
