# 全Web剩余8失败：只读分类与F05范围

时间：2026-10-23 18:48 +08:00，Asia/Shanghai。当前HEAD b7d6f5d486baa7c7eda50d5c58f154179cecd1a5，分支codex/fix-workspace-member-controls-and-analyst-permissions。保留全部旧修改、数据与F03现场，无模型、子代理、部署或提交。

## 实际诊断

原6文件组合实际退出1：60通过/8失败/419断言、68项、3.37秒，8名称与F04最终全量一致。本轮没有重跑全Web；2300通过/8失败/0错误仍为F04引用。日志在.local/f05-ui-contracts/existing-six-files.log。

只读探针eol-probe.mjs读取真实文件及分组源码，逐项重复原失败断言；原始输入7项均false，仅在内存把CRLF替换LF后7项均true。没有修改源文件、原测试或断言，此诊断不是完整测试GREEN或确认后的正式RED。结果eol-probe.json保留换行计数及11菜单映射。

| 失败 | 实际失败位置 | 分类与依据 | 继续验证 |
| --- | --- | --- | --- |
| 键盘无选区节点复制 | canvas-keyboard.test.ts:16 | CRLF不匹配固定LF；真实入口仍先文本选区保护、再preventDefault/copySelectedNodes | 原断言保留，语义顺序负向控制 |
| 上传先占位/弹窗关闭 | canvas-image-upload.test.tsx:107 | CRLF不匹配onClose与await间LF；createFileNode/replaceNodeMedia断言此前均通过 | 完整原断言及错误顺序控制 |
| 图片同步解码 | canvas-media-performance.test.ts:63 | CRLF使函数结束符切片为空；真实RetainedCanvasImage使用decoding=sync | 切片非空，async变异必须被拒绝 |
| 离屏视频首帧 | canvas-media-performance.test.ts:75 | CRLF使两个函数边界切片为空；真实视口门控、静态预览、取消/URL释放存在 | 全原视频断言与门控负向控制 |
| 引用异步预览 | canvas-resource-mention-editor.test.ts:19 | CRLF使sync函数切片终点-1；真实同值分支调用sync，内部比较src/替换预览/更新label | 保留全原断言，漏刷新变异须拒绝 |
| collection焦点 | workspace-menu-density.test.tsx:54 | CSS声明间CRLF不等于固定LF | 精确选择器/值保留，不以宽泛正则放过缺失 |
| scoped菜单边框 | workspace-menu-density.test.tsx:93 | 固定LF导致scoped规则字符串不命中；原其他断言在诊断继续执行情况以正式Build为准 | 精确规则与边框负向控制 |
| 连接菜单描述 | canvas-connection-create-menu.test.ts:13 | 旧固定9项过时；现为11，DataSource/Logic由bfcf5b1b新增，两项与openspec/specs/canvas-lowcode-dataflow/spec.md现行能力一致 | 精确11集合/描述/类型、真实SSR11/10、禁用守卫 |

## 入口与调用方

CodeGraph追踪：useCanvasKeyboard由画布project调用；ImageContent→RetainedCanvasImage，VideoNodeContent→InactiveVideoPreview→静态图片及播放按钮；CanvasResourceMentionTextarea同值分支→syncInlineMentionPreviews，消费者包含画布提示、批量表、脚本、Agent及创建页；CanvasConnectionCreateMenu由project调用，ConnectionCreateOption含禁用guard。upload源断言的创建/替换检查已经越过，仅弹窗LF字面量失败。collection与菜单样式由工作台真实组件使用。

现有源码读取helper可复用：moduleGroupSource有17文件引用，均为源码合同。source-contract.ts的code()会删除所有空白，本轮不选用该宽规范化方式；只规范CRLF，保留其他语义字节。连接菜单必须以精确11项和身份映射强化，不能将9改成“至少9”或删原测试。没有证实真实产品缺陷，不为猜测改web/src；后续若变异/完整断言暴露真实缺陷，登记并重新确认范围。

## 当前流程与未执行

Native fix-canvas-source-contract-portability已按current建立，完整brief/规格/任务准备确认。确认前不修改正式测试或产品代码；确认后先新的最小RED再修复，本次诊断不替代TDD。任务记忆检索一次长期无输出已中断，未冒称成功或应用了未返回的记忆。

未执行新增正式RED、修复、build/lint、全Web、浏览器UI、LinuxCI、18088/PG指纹核对、部署/归档/提交；不把源码成立当完整UI/媒体实机验收。独立Verifier未授权，不继承旧接受。原F03恢复、离线后置和U/A/G门槛保持。
