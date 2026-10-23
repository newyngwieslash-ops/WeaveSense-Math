# 画布源码合同的换行可移植性与现行菜单覆盖

## 完整边界

本能力仅用于web测试验证，不改变产品行为。源码合同在Windows CRLF和LF文件中必须检查相同业务语义。复用现有测试源码读取入口，读取真实源文件；仅将CRLF转为LF，保留其他字符、空白和先后顺序，不能吞读取错误或给缺失文件默认空正文。带中文/空格的合法路径和文件URL使用标准库处理，无新增依赖；组合模块仍读取完整既有组成员，不遗漏、不改顺序。

## 键盘与上传

有真实文本选区时复制快捷键退出节点复制路径；无选区时仍preventDefault后copySelectedNodes。文件上传先创建占位再传输，完成替换同节点，删除中途的节点不能重建，原媒体引用保护保持；上传弹窗先onClose再await pendingUpload。LF/CRLF只能影响文本读取，不改变这些原断言。

## 资源与媒体

RetainedCanvasImage可见图片必须decoding=sync且非async。InactiveVideoPreview保持视口门控/已有首帧短路、静态首帧而非video标签、hydrate调用、取消与临时URL释放、异步持久化与播放入口；原其他相邻断言不删。引用文本未变时仍同步预览，src/类型变化替换预览、标签更新，不替换整个编辑内容或触发onChange。函数源码切片必须在LF/CRLF下一致，不把空切片当正确实现。

## 样式合同

collection控件pointer路径无持久描边，keyboard路径保留1px control-focus-ring，不主动blur；菜单表面显式scoped、无边框/阴影，旧account-inner覆盖保持移除，原选择器和声明值逐项校验。不得以全空白删除或宽泛正则代替原具体期望。

## 连接菜单

当前完整唯一集合为Text、Script、BatchTable、Image、Config、Drawing、Video、Audio、MediaConversion、DataSource、Logic，共11项；每项保持现行中文标题、非空描述及对应创建类型和getDisabledReason调用，不能少项、重复或类型错配。绘图仅canCreateDrawing时出现，实际组件SSR应为11/10项；禁用aria及说明存在且创建守卫仍拒绝禁用操作。旧无多余header、箭头、装饰线以及BatchTable要求保留；不使用至少9项或删除原测试来抵消失败。

## 检测力与执行

确认完整Shape后先以新可运行回归观测RED，再修复共享入口。测试负向控制必须对复制/上传顺序、图片解码、视频门控、预览更新、CSS焦点/边框和菜单项/描述/类型缺失或错配实际失败；仅修改自有内存/临时副本，原源文件保持。所有原测试保留，不删/跳过，不修改产品以迎合测试。若暴露真实产品缺陷先记录并重新确认范围。

## 保护与验证

当前分支、既有修改和数据保持，全部helper直接调用方及原六测试纳入专项/组合；Runtime构建/lint与一次全Web执行结果逐项留证，未执行不能记通过。无浏览器操作、部署、生产接口或RLS变更，无缓存清理、真实模型、外发、子代理/新聊天、提交/归档或推送合并发布。独立Verifier限制须本次如实处理，旧接受不可继承。
