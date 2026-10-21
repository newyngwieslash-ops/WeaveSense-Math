# Tasks（zh-CN）

## 1. 最小复现与共享入口修复

- [x] 1.1 补一个最小回归测试文件，复用现有SQLite/PG设施，先运行旧实现确认长提问预算RefID失败（PG为22001），记录RED命令和实际失败；覆盖根/子预算、中文英文emoji边界及完整request/context，不调用真实模型。
- [x] 1.2 仅在StartSynthesis复用truncRunes限制预算摘要为80字符，随后验证上述测试GREEN、短问题不变、相同前缀不同key不混淆、同键不增task/reservation/账单、预算失败不遗留预留；同步功能说明和pending-test，gofmt仅处理本次Go文件。

## 2. 相关回归与验收交接

- [x] 2.1 运行合成/模型任务/预算相关专项与实际隔离PG检查（新测试库、无模型调用），核对所有StartSynthesis调用方与root/child路径，完成Comet检查及验证报告；报告区分本轮执行/引用旧报告/跳过/阻塞，记录保留修改与未通过发布门槛，更新tasks和时间戳handoff。仅精确提交本hotfix内容；归档须另获最终确认，不推送/合并/发布。

## 执行边界

用户已确认新分支codex/fix-synthesis-long-question-enqueue；hotfix直接执行，不启动子代理、不调用真实模型。采用先最小RED再修复和验证；等待本次工件最终确认后才能推进open guard与实施。历史工件、其他dirty及未跟踪文件保持原样。
