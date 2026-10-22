# 预览编号覆盖修复验证记录

时间：2026-10-22 22:39 +08:00记录，22:47补归档，Asia/Shanghai。变更`fix-canvas-preview-overlay-hit-target`，分支`codex/fix-workspace-member-controls-and-analyst-permissions`，实现前基线`e111f1413a6a7062d33331b16a03b989192f4db8`。用户已分别确认本次结果限制和最终本地归档；无推送、合并、发布、清缓存、模型或子代理/新聊天。

## 当前结论

预览中心点击遮挡已在共享入口修复，正式RED→GREEN、Runtime专项/构建/lint及18088真实中心双向点击、键盘、缩放、媒体控件与PG保护检查通过。用户禁止子代理，未启动独立Verifier；本报告为主会话证据，不能称独立语义审查通过。用户先接受本次限制，后单独确认本地归档；预检ready=true/无阻塞/无需重验，实际archive exit0，Native stateVersion8、六项通过、status=done、archived=true、workspaceFinishResult=kept。新主规格canvas-node-overlay-hit-targets已创建，工件移至docs/superpowers/comet/archive/2026-10-22-fix-canvas-preview-overlay-hit-target。提交结果见最终带时间戳交接；归档不等于发布。

全Web本轮2218通过、40失败、2错误、17235断言，2258测试/317文件，37.00秒，exit1。与上一轮TSV日志按失败名称比较，38个去重名称完全相同，无新增/消失；这不是干净HEAD的全量对照，不能称全量通过。

## 根因、入口与最小修复

真实调用链为CanvasProjectWorldLayers→CanvasNode→showStatusTrack。编号span已有pointer-events-none，但绝对定位的外轨道仍接收鼠标，遮住数据源预览中心；键盘和未覆盖图标可切换，排除了解析器和viewMode回调故障。既有CodeGraph只读探索和rg追踪全部轨道成员及直接调用，没有建立索引。

仅两个产品组件变化：`canvas-node.tsx`使轨道默认穿透，图片组按钮和可见主图按钮显式auto，隐藏主图仍none；存储说明原auto沿用、编号/锁原none沿用。`canvas-image-layer-controls.tsx`仅图层触发按钮auto。数据源/逻辑在full且轨道可见时，共享内容容器加pt-8；轨道top-3、最高h-7，正文原pt-4配合留位，使控件低于装饰。无轨道/低LOD/其他节点不留位，无公共helper、依赖、全局CSS、API、表结构或RLS变化。

## 六项主会话核对

| ID | 实际证据 | 限制 |
|---|---|---|
| A1 | 65%新图首次中心命中BUTTON/inside=true；预览→源码及反向普通click成功，Enter/Space成功，aria-pressed随视图变化，四条中文记录保持 | 点击真实按钮中心，没有force、DOM样式注入或隐藏store操作 |
| A2 | 63%时source轨道bottom132.792、控件top142.619；logic top142.201；locked轨道bottom137.183、控件top142.619，均不重叠。编号/锁继续显示，轨道computed pointer=none | 无轨道、低LOD与其他类型留位边界由真实组件渲染检查覆盖；不是浏览器所有尺寸穷举 |
| A3 | 有效PNG图真实展开/收起图片组，隐藏主图opacity0/pointer none；选中子图后opacity1/auto、中心inside=true，选择后当前主图转到child2；图层Popover开/关成功；存储说明title保留、auto且中心命中 | 未调用生成、下载或图层另存；存储原生title的屏幕tooltip气泡未单独截图，仅核对真实DOM title/命中 |
| A4 | 同一共享入口，渲染覆盖默认/选中/只读/locked条件；实际点击不拖动source，PG位置0/0、480×360保持，四条输入/下游4条，零模型预算 | 不把此项当完整权限矩阵或所有媒体动作验收 |
| A5 | 用户确认Shape后正式RED0通过4失败8断言，再最小GREEN4通过43断言；Runtime相邻专项、tsc/Vite及lint均exit0 | 渲染检查是属性/类合同，不提供浏览器几何；真实几何另列 |
| A6 | 18088仅web保卷更新，两张新图真实导入、55%/63%/65%中心切换、刷新及列表重开；PG两图8节点/1边/四条配置，原十一图/实采/零预算/非web容器保持 | 镜像包含旧Agent面板dirty；首图媒体样本无效，媒体通过以第二图为准；无独立Verifier |

## 实际命令与Runtime

| 检查 | 结果 |
|---|---|
| `cd web; bun test test/canvas-node-overlay-hit-targets.test.tsx`，正式RED | exit1，0通过/4失败/8断言，全部预期合同缺失；日志red.log |
| 同命令，GREEN | exit0，4通过/0失败/43断言；green.log |
| Runtime：上述测试及canvas-batch-node-animation、canvas-node-lod、canvas-image-layers、canvas-dataflow五文件专项 | passed，exit0，4413ms |
| Runtime：`cd web; bun run build` | passed，exit0，17232ms，TypeScript/Vite通过 |
| Runtime：`cd web; bun run lint` | passed，exit0，30983ms；项目UI规则，非全面样式审计 |
| `cd web; bun run test` | exit1，2218通过/40失败/2错误，完整记录见下方 |
| `check-pg.py`、`preserve.py after` | exit0，两图配置及所有保护指纹匹配 |
| `git diff --check` | exit0，仅工作副本CRLF提示 |

Runtime候选`7b618d96-16ee-497c-a165-35a52b7e15cd`，Builder执行正式计划，dispatch明确reused，没有重跑完整最终计划。首次handoff使用了CLI不接受的fail/pass状态词，被校验拒绝；更正为passed/failed后成功，该拒绝没有运行检查或改变产品。

平台确实存在子代理工具，但用户禁止子代理及新聊天，所以本任务没有获授权的独立执行方式。记录verifier-unavailable时明确说明这一事实，没有声称平台物理缺少工具，也没有提交伪造的Verifier结果。Runtime待限制接受期间继续完成已授权外部UI/PG和普通文档，未越过确认门槛。正式handoff中UI的not-run是提交时状态，后续实测证据由本报告补足，不手改Runtime状态/报告。

## 18088旅程、样本纠正与持久化

1. 保留原十一图，使用真实ZIP选择器新增`7CXWq6qHNQ-TTsVZXQUJK`（U03预览覆盖命中验收），8节点/1边，PG revision2。数据源四条甲90/乙65/丙88/丁50，filter下游4条。65%时预览中心rect x317.278/y111.633/w46.489/h13.867，elementFromPoint为BUTTON且inside=true；双向click和键盘成功。
2. 第一图媒体fixture使用SVG和假local标识，且根缺batchChildIds：普通图可显示，但图层合成解码失败、普通图片组无有效计数。核对真实metadata合同后，判断该样本不足以证明媒体交互，没有为样本改产品、覆盖原图或清缓存。
3. 新增`Ia3U3_-fRwrhmfgqxdflH`（U03媒体控件命中补验），有效PNG由本地标准库生成，根有batchChildIds与primaryImageId，8节点/1边；没有模型请求。图片组展开后两个子图出现，隐藏主图none，选中第二图后auto，点击将primaryImageId改为media-child-2；再收起，图层面板正常打开/关闭，层按钮及说明可见。合成本地PNG时现有客户端合成/本地资源上传自行执行，未操作真实外发或付费API。
4. 第二图65%→55%，中心预览/源码切换均成功。调用reload后读取实际页面，并通过菜单回画布列表再重开同图：四条输入、下游4条、锁定标记、图片组和图层入口保持，重开后再次中心点击成功。首图一次直接goto因同步期间被abort，改用正常菜单路由；未清缓存、强制关闭或重复盲试。
5. PG只读快照确认第二图revision6、8节点/1边、原四条输入、locked=true，source位置与480×360尺寸不变；primaryImageId=media-child-2及两条child关系保持。截图时顶栏仍显示正在同步，故不以该UI状态声称全部媒体云同步完成；本轮持久配置通过依据为PG与重开交叉核对。
6. before/after：old_canvas_equal、tables_equal、nonweb_containers_equal均true，new_canvas_count=2。原十一图逐行md5、10文档指纹2c298b93a5c0490a95b9e3014d27d7b2、1报告指纹7da5d5768498ec7615ad42c6908a1a26保持，模型任务/预算0。两张本轮图均保留，不删除失败样本。

真实截图保存在Git忽略目录`.local/flow-uat-20261020/preview-overlay-fix/ui-proof.jpg`；浏览器工具也已实际返回截图。样本/快照/RED/GREEN/全量日志/CLI响应/失败对比均在同目录，无凭据。静态渲染或截图本身不替代真实交互结果。

## 镜像与回退

仅从既有TSV前端镜像复制本轮Runtime构建web/dist，tag`flow-uat-web:preview-overlay-20261022`，digest`sha256:48c1c3e060e0b508e0d9255aebcccdb58e514cf725145adab557d5508bd06b77`，标签`e111f141-preview-overlay-working-tree`。含既有Agent面板dirty，不称纯HEAD。

Compose叠加flow→acceptance→member-controls→cross-tab-fix→jsonl-fix→tsv-fix→preview-overlay-fix，仅up web、--no-build/--pull never/--no-deps。非web容器ID/镜像/启动时间保持。回退去掉最后overlay并按相同约束仅重建web，原TSV镜像`sha256:7e0654b95afb356d5bca32d7caac94da978ca2723c925097755742e8052db6fc`保留；回退未实际执行。8088、卷、后端、迁移与宿主dev server未动。

## 全Web旧失败逐名登记

以下38个唯一名称对应40条失败（跨测试运行重复名称已去重）；与TSV轮old_only/current_only均空。另有两个测试间错误：临时agent.ts不能解析`@/lib/user-scope`，以及Windows `/E:/...`路径BadPathName。名称对照不是根因已全部证明相同，未扩修无关失败。

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
- 分片上传累加片内真实进度，传输 100% 后仍须等待合并响应
- 图片上传占位 > 上传先占位再传输，完成替换同一节点，弹窗不等待传输完成
- 普通 multipart 将传输进度换算为文件字节，总量未知不伪造百分比
- 资源直传失败分类 > 保留后端可读文案，并把 multipart 超限翻译成中文
- 资源直传失败分类 > 成功响应仍返回后端资源本身
- 资源直传失败分类 > 断网等非 HTTP 失败按瞬时处理
- 资源直传失败分类 > 鉴权、越权与体积超限属于重试不会自愈的永久失败
- 资源直传失败分类 > 限流与服务端故障属于瞬时失败，允许退回本机后重传

## 未执行、阻塞与发布门槛

独立Verifier未执行，用户已接受本次限制并另行确认本地归档；没有独立审查结论。浅色主题、所有缩放/屏幕尺寸、真实浏览器原生title气泡、全部媒体生成/下载/另存、完整权限矩阵不由本次解除。Go/PG产品测试及文档站构建未运行：本次不改后端/数据库，最小只读PG检查已执行。

完整U03/Skill库→节点/Agent执行、直接端点重连、U04/U05模型/MCP/审批/SSE/未知恢复、恢复迁移/监控/联合对象恢复及实采外发仍有门槛；真正离线/冷启动/独立Profile按用户决定最后处理。G01须另行授权，O01–O06不实现；禁止模型/清缓存/子代理/推送合并发布继续有效。
