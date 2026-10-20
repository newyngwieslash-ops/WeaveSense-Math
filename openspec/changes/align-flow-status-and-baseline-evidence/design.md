# 文档收口设计

## Context

基线与变更范围见 proposal.md。用户要求按交接继续 C01/C02，保护原有修改、不启动子代理、不推送或发布。Comet 路由为 tweak；用户确认新分支 codex/align-flow-status-and-baseline-evidence，isolation=branch，按 OpenSpec apply 实施。

## Decisions

1. 复用现有状态文档，不另造 PRD 或复制 196 项审计正文。implementation-status 为当前能力和证据入口；roadmap 保留 A01–A21 标识和退出标准，明确交接工作包 A01–A15 是另一套编号；delivery-report 新增本轮证据链接，旧阶段报告作为历史。
2. 文档只改当前事实和必要注释：Embedding 支持 none/hash/openai，但真实提供者仍待授权验收；媒体默认平台二进制代理与短时预签名已有实现，但 Engine 当前仍先读完整 Buffer 并做 hash，不能宣称全链路恒定内存。512 MiB 是流式分支硬上限，配置对象读取上限不等于无限提高它。原始 HTTP 字节直接保存，不写 gzip。
3. 画布仅开放三种只读 intel 工具和工作区已授权 MCP；read_only 不开放 MCP。Skill/DAG 不是任意代码沙箱。情报交接路由以实际注册为准。以上由源码与本轮专项确认，真实模型/浏览器验收仍保留。
4. 独立预检草稿已迁至 docs/flow/2026-10-20-current-baseline-evidence.md，保留原 SHA 和时间，避免维护两份证据账；不重复运行已通过且无相关代码变化的产品检查。Comet 需要运行证据时按工具实际要求选择文档校验，不以手工 record-check 冒充执行。
5. 用户原有 prd-audit-9.md、历史 handoff、table.csv、旧归档元数据不改动不暂存。对审计汇总的更正在当前文档记录来源和差值，不篡改原表快照。

## 验证与限制

查找修改过的现行段落，保证不再自称 Embedding 未实现、画布无 MCP、仅 25 MiB 或 P0/P1 待开发；历史报告中的旧描述保留日期并明确为历史。检查本轮证据和历史引用分离、链接可定位、无凭据值；执行 openspec strict validate 与 git diff --check。文档修改不会提升真实 UI、PG、生产恢复或远端交付状态。

产品边界问题只登记，不在文档变更内修代码：A20 测试存在离线回退；TenantMiddleware 外层 Commit/HTTP 回执仍待复现；真实 PG 与启用连接器需要凭据和授权。如确需修复，另走有最小复现的聚焦变更。
