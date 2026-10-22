# JSONL导入修复验证报告

时间：2026-10-22，Asia/Shanghai。分支`codex/fix-workspace-member-controls-and-analyst-permissions`，实施验证基线`6b93ce3c304eaa1f713622a4c2e5090f2362f3ab`。用户确认完整Shape后执行；先接受无独立Verifier结果，再单独确认本地归档。归档已完成，当前分支保留，无推送、合并或发布；提交与终检见后续时间戳交接。

## 结论与限制

合法JSONL的实际失败已用最小RED复现并修复；主会话核对六项要求的证据如下。专项、构建/lint和18088真实旅程通过，但全Web仍有旧失败；用户禁止子代理/新聊天，本轮没有独立Verifier，不能将主会话核对称为独立验收。用户分别明确接受结果和授权本地归档后，Native归档退出0、stateVersion9、六项通过、status=done、archived=true、workspaceFinishResult=kept。归档预检ready=true、无阻塞、无需重验；主规格已新增canvas-jsonl-data-source-import。

## 根因、调用方及最小修改

文件入口`DataSourceNodeContent.handleFileImport`接受.jsonl，file.text后经`detectDataSourceFormatFromFilename`保存format=json、fileName及rawText。共享`parseDataSourceNode`原先把正文交给整份`JSON.parse`，第二条记录因此报错。`extractNodeItems`和`evaluateCanvasDataflow`复用此入口，覆盖导入、编辑和重开；通用`parseJsonItems`另用于普通JSON、情报、TXT试探及普通文本，不扩展它的严格合同。

只在`web/src/lib/canvas/dataflow-evaluator.ts`增加显式json/.jsonl逐物理行分支，复用`toCanvasDataItem`；保留原始正文以计算物理行号，坏行整份0条。生产diff为15行新增/1行删除，无helper、依赖、API、数据库或RLS变化。单一无mock测试文件`web/test/canvas-jsonl-import.test.ts`留作回归。

## 六项证据对照

| ID | 主会话实际证据 | 结果边界 |
|---|---|---|
| A1 | 两中文对象顺序、LF/CRLF/BOM/空白/空文件/大小写文件名专项；真实rows.jsonl导入2条 | 本轮实际运行 |
| A2 | 对象包装、行内数组、转义字符串、数字/布尔/null专项 | 一行一条，不展开items包装；未调用模型 |
| A3 | 前两空行后第5行非法，专项及真实bad-lines.jsonl导入均报第5行，源/下游0条 | 不返回前面合法行；页面正文保留 |
| A4 | 普通JSON及无标识严格拒绝多份JSON，普通数组/包装保持，残留.jsonl的CSV/TXT/intel/Markdown尊重实际格式 | 专项实际通过，相邻6项回归通过 |
| A5 | 最小RED 0通过/1失败，扩展RED 1通过/4失败；GREEN 11通过/58断言；Runtime构建/lint退出0 | 全量旧失败单列，独立Verifier未执行 |
| A6 | 仅更新18088 web；真实选择器合法/坏文件、下游、reload和PG；原五张画布及实采数据指纹保持 | 合成验收内容走真实应用，不称实采或全部U03通过 |

## 实际命令与全量对照

| 命令 | 实际结果 |
|---|---|
| `bun test web/test/canvas-jsonl-import.test.ts web/test/canvas-dataflow.test.ts` | 11通过、0失败、58断言，Runtime exit0/2302ms |
| `cd web; bun run build` | TypeScript/Vite正式构建exit0/16888ms |
| `cd web; bun run lint` | UI退场规则exit0/25366ms；不是通用风格或全量静态审计 |
| `cd web; bun run test`，修复版 | 2209通过、40失败、2错误、17177断言，2249测试/315文件，exit1 |
| 同命令，仅暂时撤回本轮解析器补丁，保留其他dirty和新增测试 | 2205通过、44失败、2错误、17170断言，exit1；随后apply_patch恢复完全相同补丁 |
| `git diff --check` | exit0，仅CRLF工作副本提示 |

Runtime候选`2591353c-209b-4087-8f06-f83b0a4b201e`，三项正式检查由Builder handoff计划执行；后续dispatch返回`reused`，没有重复运行相同计划。全Web失败不放进“已通过”列，也不为本聚焦修改扩修无关失败。

同命令对照差异恰为4个新增JSONL失败名称，修复版无独有失败；40个失败实例合并为38个名称。两项错误包括临时目录中`@/lib/user-scope`解析失败及Windows `/E:/...`路径BadPathName。完整日志与名称对照留在Git忽略的`.local/flow-uat-20261020/jsonl-fix/full-web-tests.log`、`baseline-web-tests.log`和`failure-comparison.json`。这是本工作区局部撤回对照，不称干净HEAD全量基线。

## 18088真实UI与PG

1. 用产品原生画布ZIP选择器新增`U03 JSONL修复复验 2026-10-22`，ID`de04-db1Q77AlWTObKDdF`，2节点/1连线；未改原五张图。ZIP仅有合成数据源与title提取节点，不是静态HTML，不经隐藏store或SQL写入。
2. 数据源真实文件选择器导入前轮`rows.jsonl`，自动JSON，显示2条及id/title字段，下游显示逐行甲、逐行乙。上传工具曾等待很久，最终成功；等待不当作产品失败。
3. 先在页面编辑补充坏行检查，再由真实选择器导入本轮`bad-lines.jsonl`（CRLF、前两空行、物理第5行非法）：报“JSONL 解析失败：第 5 行不是有效 JSON”，预览0、下游输入0/输出0。
4. 在页面恢复合法两行正文，不再次上传原文件，故名称仍为`bad-lines.jsonl`；正常同步后reload，仍2条/两条下游，显示已同步。PG只读核对revision7、2节点/1连线，format=json、文件名及完整两行正文、json_extract/title一致。
5. `.local/flow-uat-20261020/jsonl-fix/preserve.py after`退出0：原五张图逐行md5完全相同；文档10条指纹`2c298b93a5c0490a95b9e3014d27d7b2`、报告1条`7da5d5768498ec7615ad42c6908a1a26`保持；模型任务/预算均0；platform/PG/Engine/scheduler容器ID、镜像及启动时间保持。只新增一张验收图，保留全部缓存及数据。

真实浏览器截图已由工具返回；工具没有已记录的本地截图保存接口，因此未声称生成可追溯图片文件。结论以实际AX/DOM、Runtime记录与只读PG交叉核对为准。

## 镜像与回退

仅将正式web/dist复制到既有运行镜像，tag为`flow-uat-web:jsonl-import-20261022`，镜像sha256为`fe49170bca5f05298acbae94bc7b6417e274f5dcaf3e95818770e2f568323d9e`。构建含保留的Agent面板等当前dirty，标签明确working-tree，不能称纯HEAD。宿主dev server未启动，8088未操作，没有后端或迁移部署。

Compose按根flow→acceptance→member-controls→cross-tab-fix→jsonl-fix overlay，使用`up -d --no-build --pull never --no-deps web`；没有down或卷操作。原`flow-uat-web:cross-tab-session-cache-20261022`镜像sha256为`fa7333e8c7c2fe498c0b6d86c02783ae95ebc0205e6f0d8050ad79704e02f7b0`，回退只需去掉最后jsonl-fix overlay后同样仅重建web。回退配置留存，未执行回退。

## 仍未通过的门槛

- 全Web旧40失败/2错误，完整回归未通过；无独立Verifier结果虽获接受并已本地归档，但不代表独立审查或发布通过。
- TSV、手工拖接/重连/交互建环、完整Skill及U03/U04/U05不在本轮修复范围。
- 真实Offline/进程冷启动/独立Profile按用户决定最后处理，待办文档继续有效；监控告警、真实旧库/PG16迁移、未知结果恢复、对象联合恢复、实采外发及G01仍有各自门槛。
- 本轮未执行Go、后端构建或全站文档构建：未改后端/API/表结构；没有模型调用、子代理、新聊天、清缓存、外发、生产迁移、push/merge/release，不实施O01–O06。
