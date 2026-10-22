# 画布预览遮挡聚焦设计与任务

状态：用户已分别确认完整Shape、本次无独立Verifier结果及最终本地归档；当前目录/分支保留，实现前基线e111f141。T1–T4已执行；预检和实际archive退出0，Native六项通过、status=done/archived=true、工作区kept，新主规格已创建。正式要求见本变更归档brief/spec；提交结果以最终交接为准。

## 入口和证据

CanvasProjectWorldLayers→CanvasNode→showStatusTrack覆盖层。唯一轨道包含ResourceLabelBadge、ResourceStorageBadge、BatchToggleBadge、CanvasImageLayerControls、BatchPrimaryBadge、NodeLockBadge，已用CodeGraph及rg核对所有直接引用。前轮真实预览中心elementFromPoint命中外div而非button；Enter/Space和未覆盖图标正常，根因不是解析器或viewMode回调。

## 最小设计

复用原生pointer-events：轨道默认none；已有存储说明auto沿用；图片组/可见主图/图层触发按钮显式auto，隐藏主图仍none；装饰和间隙穿透。无新组件/helper/依赖，不加全局选择器。

仅当数据源/逻辑节点有showStatusTrack时，于共享内容容器预留一条最小顶部状态空间，避免只修点击而仍盖住预览文字；无轨道和其他节点保持。顶轨道原top-3且按钮最高h-7，所需空间由现有间距/尺寸核实；正文从余下区域按原规则布局，节点持久尺寸不变。若实际渲染反证该方案不适用，回到已确认目标做最小实现调整，不能擅自改行为。

## 四项任务

- [x] T1：正式渲染RED为0通过/4失败/8断言，全部为预期合同缺失，日志保留；不以旧UI观察替代。
- [x] T2：两个共享组件最小修复；GREEN为4通过/43断言，覆盖默认/选中/只读/locked、条件留位、显式媒体命中及隐藏主图分支。
- [x] T3：Runtime专项、TypeScript/Vite及UI lint通过，候选复用检查。全Web2218通过/40失败/2错误，38个去重失败名称与TSV轮相同，无新增；无独立Verifier限制待本次接受。
- [x] T4：18088仅web保卷更新。新增两图，第一图验证中心/键盘/63%与65%缩放，媒体初样本SVG/缺batchChildIds不能作为媒体通过；第二张有效PNG/完整关系补验展开/收起、可见主图、隐藏分支、图层面板、存储命中及55%预览，刷新/列表重开与PG通过。原十一图、实采10文档/1报告、零模型预算及非web容器保持，回退镜像保留。

## 确认和交付

当前目录/分支已明确，本轮Shape已确认。无子代理/新聊天/模型/清缓存/push/merge/release；最终结果限制接受与本地归档分别确认，不沿用旧变更。真实媒体生成和完整U03/Skill/重连均非本轮目标。完整证据见`docs/superpowers/reports/2026-10-22-fix-canvas-preview-overlay-hit-target-verify.md`。
