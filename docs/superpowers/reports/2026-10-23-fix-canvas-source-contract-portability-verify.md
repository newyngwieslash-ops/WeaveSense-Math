# F05源码合同可移植性验证

时间：2026-10-23 19:08 +08:00，Asia/Shanghai；19:16归档增补。验证基线HEAD b7d6f5d486baa7c7eda50d5c58f154179cecd1a5，分支codex/fix-workspace-member-controls-and-analyst-permissions。用户确认完整Shape后实施，随后分别确认有限结果和本地归档；未部署。精确提交及最终HEAD见收尾交接。

## 当前结论

全Web实际退出0：2320通过/0失败/0错误、17993断言、320文件、48.21秒；F04剩余8失败全部消除，新增0。Runtime正式369项/2286断言及TypeScript/Vite构建、lint均退出0。改动只有测试及相关文档，未发现需修改生产代码的真实缺陷，不改生产菜单、媒体、鉴权或样式来迎合测试。

Native候选6ec9dc80-a708-436a-ba1a-f6f40713f6d1，iteration1/attempt1。用户禁止子代理/新聊天，dispatch仅登记，未启动独立Verifier；已如实提交verifier-unavailable。用户先明确接受没有独立审查的有限结果，再另行确认本地归档；预检ready=true/无阻塞/无需重验，实际archive退出0，Runtime stateVersion8、done/archived=true、六项按限制通过，workspaceFinishResult=keep/kept、merged=false、pushed=false。新增canvas-source-contract-portability主规格，工件位于docs/superpowers/comet/archive/2026-10-23-fix-canvas-source-contract-portability。本地归档不授权部署或其他交付。

## 原因与最小修改

7项原断言对源码换行敏感：键盘/上传顺序固定LF，图片与视频/引用函数结束符固定LF，collection和scoped菜单CSS固定LF。现行语义原本存在，CRLF造成空切片或字符串不匹配。复用现有module-group-source，新增readSourceText只将CRLF转LF，分组和六份测试共用，不删除空白、中文、引号、属性或顺序，不吞读取错误。

第8项旧菜单9项计数过时；bfcf5b1b已新增DataSource/Logic。保留原描述/header/箭头/BatchTable断言，计数严格11项，并强化为完整标题、描述、创建类型和禁用类型的精确映射，不能靠遗漏/重复/错配通过。真实CanvasConnectionCreateMenu SSR有/无绘图分别11/10项，数据源禁用说明和aria正确；原创建guard断言保持。

| 文件 | 作用 |
| --- | --- |
| web/test/helpers/module-group-source.ts | 共享源码读取仅CRLF→LF，组合成员及顺序保持 |
| web/test/canvas-keyboard.test.ts | 原复制断言接共享读取 |
| web/test/canvas-image-upload.test.tsx | 原上传/弹窗断言接共享读取 |
| web/test/canvas-media-performance.test.ts | 媒体及相邻源码读取复用入口 |
| web/test/canvas-resource-mention-editor.test.ts | 引用函数/相邻源码读取复用入口 |
| web/test/workspace-menu-density.test.tsx | 精确样式断言接共享读取 |
| web/test/canvas-connection-create-menu.test.ts | 精确11映射、真实SSR及原创建守卫 |
| web/test/source-contract-portability.test.ts | 新共享读取、原断言LF/CRLF和负向回归 |

17个rg命中文件含helper自身，实际16个测试消费者，全部纳入35文件并集。调用链及八项逐项位置见[调查报告](../../flow/2026-10-23-web-eight-failures-investigation.md)，本次未将历史整个PRD纳入。

## 检测力与过程记录

新增回归真实读取含中文/空格的原生路径及文件URL，手写期望保持BOM、制表符、空格、中文和孤立CR；缺失文件实际抛错。临时混合换行的主/拆分模块验证成员及顺序，finally只移除本测试注册项和已校验父目录/前缀的自有临时目录，不清真实站点缓存。

回归提取原8个具名测试回调，运行其原断言，对真实源码临时副本分别用LF/CRLF读取；不复制一份较弱业务判定。基线先通过，再注入12种语义错误：复制/上传顺序、sync→async、视频门控、引用刷新、焦点ring、菜单边框、菜单标题、空描述、创建类型错配、重复标题、整项删除。每种错误有命中计数且原断言实际失败；加载/准备错误会向上传播，不能当成功拒绝。

开发中第一轮负向用例准备存在将环境错误当reject的风险，在实施修复前改为基线先通过、仅捕获原回调失败，再记录正式RED。随后开发GREEN首轮暴露本机TypeScript7包没有Compiler API；这是新增回归脚本错误，未当产品缺陷或通过。已移除该依赖引用，使用已知具名块的明确边界并用Bun.Transpiler运行；边界不匹配即失败，未加依赖。两次过程问题已修复后，最终相关/全量及Runtime全部通过。

## 实际检查

| 检查 | 本轮实际结果 | 记录 |
| --- | --- | --- |
| Shape原6文件诊断 | exit1，60通过/8失败/419断言；只读分类，不是正式新RED | .local/f05-ui-contracts/existing-six-files.log |
| 确认后最小RED | exit1，2通过/12失败/30断言，修复前 | .local/f05-ui-contracts/red.log |
| 开发7文件GREEN | 80通过/0失败/826断言；后续又补整项删除控制 | .local/f05-ui-contracts/green-dev.log |
| 最终35文件组合 | exit0，369通过/2286断言，15.33秒 | .local/f05-ui-contracts/related-dev.log |
| Runtime正式35文件 | exit0，369通过/2286断言，约16.57秒 | f05-related |
| Runtime bun run build | exit0，约11.40秒；含既有大chunk提示 | f05-build |
| Runtime bun run lint | exit0，约21.99秒 | f05-lint |
| 完整bun test | exit0，2320通过/0失败/0错误，17993断言，48.21秒 | .local/f05-ui-contracts/full-web.log |

Runtime日志目录：.comet/runtime/native/changes/fix-canvas-source-contract-portability/logs/checks/，本轮三日志前缀a7f32314-ad4e-4dbd-8003-2a57066f0336-。Builder提交实际执行，dispatch确认绑定未变后复用回执，未重跑有效检查。正式Runtime检查不等于独立语义审查。

最小直接重跑（web目录）：

```powershell
bun test test/source-contract-portability.test.ts test/canvas-keyboard.test.ts test/canvas-resource-mention-editor.test.ts test/canvas-connection-create-menu.test.ts test/canvas-media-performance.test.ts test/canvas-image-upload.test.tsx test/workspace-menu-density.test.tsx
```

35文件精确命令参数见related-files.json及Runtime回执；全量实际使用bun test一次。F04旧基线2300通过/8失败/0错误为比较引用，逐名对照failure-comparison.json记录old_unique8/new_unique0/added0。测试数增加12来自新回归11项和真实SSR1项，不等于新增产品功能数。

## 保护、未执行与门槛

- HEAD/分支不变、暂存区空；7份其他旧tracked文件字节哈希复核一致。既有混合文档、runbook、Agent面板、归档.comet.yaml、历史handoff/审计/CSV及所有未跟踪内容保留；没有reset/clean/restore或git add .。
- 未改web/src/backend/services/.github、依赖/锁文件/API/表/RLS，无新生产后门，无删/跳过测试。没有真实模型、浏览器缓存或用户数据操作。
- 未部署18088、未检查本轮浏览器/PG/容器指纹，真实SSR不等于浏览器视觉/媒体实机；Linux真实CI未运行。原F03恢复缺可信基线继续阻塞，离线/冷启动/独立上下文仍最后处理，其他U/A/G门槛未据此通过。
- 独立Verifier未执行，本次有限结果与本地归档分别明确授权，没有继承F04接受；仅本次文件精确提交，不推送合并发布。全Web通过只说明当前工作区这次测试结果，不能扩大为完整PRD或发布通过。
