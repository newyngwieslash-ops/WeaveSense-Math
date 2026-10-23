# Outcome

修复全Web剩余8项测试失败的已证实原因：7项源码断言被CRLF读取边界误伤，1项连接菜单仍固定9项而现行产品已有11项。保留业务语义和负向断言，通过共享测试源码读取边界与精确菜单合同更新恢复可信检查。当前没有证据支持修改生产代码；完整Shape确认后先新增最小RED，再修复验证。

# Scope

仅web/test中的6份失败测试、现有helpers/module-group-source.ts共享读取及一份最小回归，必要时复用已有测试工具；相关普通任务、调查/验证报告、pending与时间戳交接。现行11项菜单及描述、创建类型、绘图条件、禁用行为是完整目标，不改产品菜单。moduleGroupSource有17个直接引用文件，均纳入回归检查，不以单文件隔离代替全量。

## Source coverage

用户指定新交接的必要范围为剩余8失败及既有保护/验收纪律；其余U/A/F03/交付为非目标。交接已完整读取，代码、历史提交和现行规格只作调试/实现依据，不将整份PRD纳入变更。

| Source item and location | Read status | Content to preserve | Spec location | Acceptance ID | Coverage status | Reason or replacement |
| --- | --- | --- | --- | --- | --- | --- |
| S1: 收尾交接Pending Work键盘复制回退 | complete | 文本选区优先，无选区才阻止默认并复制节点 | specs/canvas-source-contract-portability/spec.md 键盘与上传 | A2 | covered | 原业务断言保持，CRLF诊断为假失败 |
| S2: 同段资源引用异步预览 | complete | 文本不变时刷新src/标签，不替换编辑选择或触发onChange | 同规格 资源与媒体 | A2 | covered | 函数切片结束符只接受LF |
| S3: 同段collection焦点圈 | complete | pointer不留描边，keyboard保留1px ring | 同规格 样式合同 | A2 | covered | CRLF使原精确字符串不命中 |
| S4: 同段连接菜单说明 | complete | 当前11项完整集合、描述/创建类型、绘图条件和禁用语义 | 同规格 连接菜单 | A3 | covered | 旧9项计数被已归档bfcf5b1b新增两项取代，精确校验不能放宽为至少9项 |
| S5: 同段同步图片解码 | complete | RetainedCanvasImage仍decoding=sync且不能async | 同规格 资源与媒体 | A2 | covered | 源码函数切片被CRLF截断 |
| S6: 同段离屏视频首帧 | complete | 视口门控、静态首帧、取消/URL释放/持久化语义 | 同规格 资源与媒体 | A2 | covered | 相邻函数边界匹配被CRLF截断 |
| S7: 同段菜单CSS边界 | complete | scoped菜单无边框及旧override移除，原选择器和值保持 | 同规格 样式合同 | A2 | covered | CRLF使原精确字符串不命中 |
| S8: 同段上传占位时序 | complete | 先占位、替换同节点、onClose早于await pendingUpload | 同规格 键盘与上传 | A2 | covered | CRLF不等于原LF字符串 |
| S9: 用户本轮约束/交接保护与流程 | complete | 当前分支、旧修改和数据保留；先确认再RED；不放宽/删跳过，不清缓存/模型/子代理/推送合并发布 | 同规格 保护与执行 | A1,A4,A5,A6 | covered | 约束持续有效 |
| S10: 交接其余F03、离线、U/A、G01 | complete | 历史阻塞与授权边界保持 | — | — | non-goal | 不恢复、不部署、不开展生产或真实模型验收 |

# Non-goals

不改web/src、backend、services、API、表结构、RLS、生产鉴权、业务菜单或样式，不把CRLF问题改成产品格式重写；不全局format，不变更换行配置或锁文件；不增加依赖。无部署、提交或归档授权，不清缓存、不调用真实模型、不启动子代理/新聊天、不推送合并发布。原F03恢复/离线后置及其他验收不收口。

# Acceptance examples

- A1：确认完整Shape后先补最小可运行RED，覆盖真实读取的LF/CRLF及带中文/空格路径、现行菜单精确集合；在修复前实际失败并保留日志。Shape阶段原6文件60通过/8失败/419断言和只读内存对照是诊断，不代替确认后的新RED。若新证据不成立，记录反证不改生产代码。
- A2：键盘、上传、图片解码、视频首帧、异步引用、collection焦点和scoped菜单这7项原业务断言在LF/CRLF输入下语义相同并全部通过；共享读取只规范CRLF为LF，不删空白、中文、引号、标识符、选择器、属性或操作顺序，不用宽泛匹配掩盖缺失。
- A3：连接菜单严格覆盖当前11个唯一条目及其非空描述和对应CanvasNodeType，保留原无多余header/箭头和BatchTable要求；实际组件SSR在canCreateDrawing=true/false分别11/10项，并保留禁用说明/aria及创建守卫。新增DataSource/Logic是已实现范围，不退回9项、不放宽为大于等于计数。
- A4：负向控制证明检测力：改错复制/上传顺序、sync解码、视频视口门控、引用刷新、CSS焦点/边框或删改菜单项/描述/类型时，对应合同必须实际拒绝；只用自有内存或临时源副本，不改生产文件。异常路径不能被归一化吞掉，真实缺陷若暴露先记录并重新确认产品范围。
- A5：原6文件、共享helper所有直接测试调用方及新增回归单独/组合GREEN；Runtime构建/lint通过，全Web实际运行一次留退出码/通过失败错误数和逐名差异。原8失败必须按本范围处理，新增/仍在失败不能删跳过或冒称全量通过；无实际运行的平台/浏览器不算通过。
- A6：仅测试及相关文档，优先复用已有读取helper，无新依赖；精确记录修改文件/调用方、实际/引用/未执行结果，旧tracked/untracked和数据保持。当前分支不变，未提交/归档/部署；不清缓存、模型、子代理或推送合并发布。无独立Verifier如实登记，结果接受、归档分别另行授权。

# Constraints and invariants

基线b7d6f5d486baa7c7eda50d5c58f154179cecd1a5，当前分支codex/fix-workspace-member-controls-and-analyst-permissions；全部既有dirty/untracked保留。文档中文，Asia/Shanghai，web用Bun，手工编辑apply_patch。当前有.codegraph，只读使用，不重建索引。禁止为测试成功改生产守卫、菜单或样式，禁止删跳过测试或修改原业务期望为较弱条件；9→11有历史实现/规格依据并加强完整映射。真实浏览器、Linux CI或部署未执行不能算通过。

# Decisions

沿用用户指定current目录和当前分支，不再询问隔离。单一聚焦测试合同变更，所有失败汇聚于源码读取/精确合同，拆成Supervisor无收益，且不使用子代理。只规范EOL，不使用现有code()全空白折叠，以免误改字符串检测边界。复用现有module-group-source所在测试helper容纳共享纯读取，所有调用方回归。没有证实真实产品缺陷；若后续语义负向控制暴露真实缺陷，先登记再确认范围。

# Open questions

调查范围内没有待决产品行为；完整Shape必须经本轮最终确认后进入Build。用户初始调查请求不代替最终确认；不新增重复确认阻塞。

# Verification expectations

先新增最小RED→共享读取GREEN与严格菜单合同→原六文件/直接调用方/相邻F04组合→Runtime build/lint→全Web真实结果。负向控制和真实组件SSR补足静态源码检查的局限，但不冒称浏览器视觉验收。调查日志.local/f05-ui-contracts/existing-six-files.log及eol-probe.json可追溯；无独立Verifier按本次单独接受规则处理，不继承F04接受。保留原F03与发布门槛。
