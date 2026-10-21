# Design：复用成员管理与scope合同

## Context

见proposal。后端已经支持analyst邀请/赋权/投影和DELETE成员，前端尚未消费完整能力。专题HTTP和本地工具最终复用CreateTopic/UpdateTopic，权限集中在canWriteTopics。现有RemoveMember排除owner的成员更新与无排除的工作区成员删除不一致，UI接入前需补安全回归。

## Goals / Non-Goals

**Goals:** 三处用户可见行为一致；复用既有API/http、AppModal和权限映射；撤销具有明确影响范围、取消与失败反馈，后端独立保护owner/管理权限及原子性。

**Non-Goals:** 无新API/数据库迁移/依赖/全局角色框架；不改租户级撤销和多workspace同步角色，不改邀请兑换、重新入会或Engine，不操作既有验收成员。

## Decisions

1. 成员页面保持现有状态组织，直接补analyst标签和下拉选项、API类型；新增既有DELETE的http调用。复用AppModal展示目标成员和“该组织所有工作区”影响范围；不使用静态Modal.confirm。确认成功后刷新成员及目录；取消不发请求，失败不显示成功或移除成员，防止重复提交。
2. 非管理者看不到撤销命令，owner条目始终不提供编辑/撤销；后端仍须校验，不能以隐藏按钮替代权限。共享RemoveMember在原事务中先查目标组织成员、拒绝owner，再原子更新成员状态及删除该租户workspace-membership。沿用SetMemberRole的owner/最后管理员约束模式，对并发判定采用同一租户内可序列化的行锁顺序，避免检查后被并发改写；SQLite保持可运行，无新锁表。若实测发现需要跨模块事务重构，停下评估，不顺带重构。
3. 专题共享写权限复用已有hasScope(s.Role,"intel.reports.write")，而非新增analyst分支散落handler/工具。HTTPCreate/Patch、CreateTopic/UpdateTopic及本地工具均按同一门禁；viewer/未知角色拒写，analyst不获得CreateInvite/SetMemberRole/RemoveMember权限。
4. 前端复用既有Bun测试入口与真实组件，优先为回显/命令呈现/确认流程补最小检查；后端为analyst专题共享入口/真实HTTP和RemoveMember安全原子性补专项，涉及RLS时使用普通PG新测试库。每条修复先观察旧实现失败，不把静态线索当已复现。
5. 保留既有dirty文件，新增本变更工件和精确代码/文档增量；full流程后续设计/执行设置需独立确认，不沿用旧hotfix默认值，不自动子代理审查。UI与安全协同同一变更，避免只加按钮却遗留共享入口风险。

## Risks / Trade-offs

- [撤销实为租户级操作] → 弹窗明确所有工作区受影响，不更名为单工作区删除；成功后刷新目录，原会话下次请求立即拒绝。
- [owner/最后管理员被伪造请求或并发绕过] → 服务层保护和普通PG并发/失败回滚检查，不只测按钮隐藏；若静态owner线索不成立记录反证。
- [analyst专题允许写被误扩展成管理权限] → 明确测试专题允许、成员管理拒绝、跨workspace对象404；不扩大其他角色scope。
- [组件测试仅静态渲染不能验证点击] → 报告区分静态、交互与真实页面；能复用现有工具则做真实路由重验，部署未做则明确待验，不创建仿制HTML。

## Migration Plan

无数据库迁移。先完成本轮工件确认，后按full设计与执行设置确认再RED→GREEN。仅运行最小专项和相关前端Bun构建/后端构建；发布门槛仍含18088当前源码UI和完整角色复验。隔离环境更新需保持现有卷与Compose配置，原数据不清理；G01推送/合并/发布另待明确授权。
