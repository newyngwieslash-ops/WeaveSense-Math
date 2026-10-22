# Outcome

修复画布数据源预览/源码按钮被右上编号层遮挡：用户点击按钮中心应直接切换，无需寻找左侧图标或依赖键盘；编号仍可见，控件文字可读。以共享节点覆盖层为修复入口，同时保留覆盖层内真正交互控件的可用性。

# Scope

单一聚焦变更fix-canvas-preview-overlay-hit-target，当前目录与分支codex/fix-workspace-member-controls-and-analyst-permissions继续，基线e111f1413a6a7062d33331b16a03b989192f4db8。用户明确保留当前分支、旧修改及数据，不重问隔离方式。前轮U03验收账只作为故障证据，不是全量需求源；本次不重开已归档TSV/JSONL。

共享CanvasNode由CanvasProjectWorldLayers渲染，showStatusTrack右上容器包含ResourceLabelBadge、ResourceStorageBadge、BatchToggleBadge、CanvasImageLayerControls、BatchPrimaryBadge和NodeLockBadge；所有引用和直接调用已核对。编号和锁图标已有pointer-events-none，外容器仍拦截中心。预览状态切换本身正常，不能改解析器或各调用方逐个补救。

# Non-goals

不改数据源/逻辑求值、TSV/JSONL、Skill执行、连线/重连、模型、权限、API、表结构或RLS；不隐藏编号、删除锁状态或禁用媒体工具，不增加全局CSS覆盖、依赖、公共抽象或新设置。其他U/A及浏览器离线项不附加本变更，不清缓存、不调用模型、不启动子代理、新聊天或推送合并发布。

# Acceptance examples

- 数据源存在右上编号时，真实预览及源码按钮中心鼠标点击均可切换，首次点击不只选中节点；Enter/Space与aria-pressed状态保持正确，四条输入在两种视图内容一致，不依赖点击未遮挡的小图标。
- 编号及锁定标记继续可见，装饰及容器空隙不截获下层交互；数据源与逻辑节点顶栏和编号不相互遮住文字。仅有状态轨道的结构化节点预留最小顶栏空间，无轨道节点不增加空白，其他节点布局保持。
- 同一共享轨道的存储标记悬停说明、图片组展开/收起、可见主图选择和图层管理仍可命中；隐藏主图按钮仍不可命中。只对真实交互子控件恢复pointer-events，装饰编号/锁图标保持穿透，不把整个轨道或所有后代无差别设为可交互。
- 复用CanvasNode共享入口，默认、选中、只读及locked分支保持既有动作可见性和禁用边界，事件不误启节点拖拽、画布框选或模型任务；不改变节点元数据、尺寸合同、API/存储或数据流输出。
- 本次Shape明确确认后，先新增最小可运行回归检查并观察真实失败，再最小修复GREEN；实际渲染检查覆盖装饰穿透、条件顶栏空间和交互子控件，相邻画布专项、TypeScript/Vite及UI lint通过。前轮鼠标对照不替本轮正式RED，全量旧失败与未验项单列，不称全量通过。
- 18088只保卷更新web，以新合成专项图实测普通/缩放/键盘双向切换、鼠标中心命中、编号和控件可读及媒体工具不被封死，刷新/重开和PG配置保持；原十一图、实采10文档/1报告、零模型预算及非web容器指纹保持。镜像含其他dirty如实标注，不清缓存/模型/子代理；独立Verifier未执行时本次单独接受，最终归档另行确认。

# Constraints and invariants

保护所有旧dirty/untracked，尤其Agent面板、旧归档配置、混合pending文档、历史handoff、prd-audit-9.md、table.csv及本地参考文件，不reset/clean/restore、不git add .。web使用Bun；测试不得通过孤立HTML或注入DOM样式冒充产品修复。浏览器操作仅真实UI，静态渲染不证明命中，真实elementFromPoint和鼠标结果必须单列。

本次不启动dev server，若需实际复验只更新既有18088隔离web、保留卷和回退镜像，不动8088或生产迁移。无模型调用，不把点击工具可用扩大为真实生成或计费通过。用户禁止子代理，不沿用旧变更的结果接受或归档授权。

# Decisions

按项目配置使用Native，单变更、当前目录/分支继续。优先原生CSS能力：共享状态轨道默认穿透，存储标记复用现有pointer-events-auto，图片组/主图/图层按钮明确恢复自身命中；隐藏分支保留pointer-events-none。数据源/逻辑节点在有轨道时于共享内容容器预留一条最小顶部状态空间，避免只让点击穿过但编号仍盖住文字；其他节点不挪位置。无新helper或依赖，具体四任务见普通设计计划。

新主规格为canvas-node-overlay-hit-targets，CLI候选能力仅检索结果、associationDraft=null，没有关联或拷贝旧规格。前轮证据包含键盘/未遮挡图标成功及中心命中编号层，不把切换回调判坏。完整Shape确认后再Build，先RED后修复。

# Open questions

无尚待调查的用户行为歧义或外部依赖。当前完整结果/范围/六项验收及非目标需经本轮Shape确认，尚未新增正式测试或改产品。用户禁止子代理，后续独立Verifier限制需如实记录并单独接受，不启动或虚构独立执行。

# Verification expectations

本轮最小正式RED使用真实CanvasNode渲染及覆盖层/控件属性，辅以当前真实浏览器按钮中心命中的失败证据；不mock解析器或用测试辅助样式掩盖覆盖。修共享入口后验证同一检查GREEN；正式专项、build/lint计划交Runtime执行，候选匹配时复用。

媒体按钮用无模型合成图与实际回调核实，真实浏览器只操作新专项图；只读PG/容器指纹保护原十一图和实采数据。若任一交互缺少真实验证则单列，不用静态markup冒充UI。同步任务、功能/待测试、验证报告与带时间戳handoff；本轮不做G01或O01–O06。
