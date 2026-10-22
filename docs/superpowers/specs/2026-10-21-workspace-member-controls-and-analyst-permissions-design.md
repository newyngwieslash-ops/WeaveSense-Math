---
comet_change: fix-workspace-member-controls-and-analyst-permissions
role: technical-design
canonical_spec: openspec
archived-with: 2026-10-22-fix-workspace-member-controls-and-analyst-permissions
status: final
---

# 成员撤销与 analyst 合同技术设计

2026-10-21，Asia/Shanghai。用户已确认工件及租户行锁方案；管理员自撤销仅在仍有其他active owner/admin时允许。规范来源为本变更两份OpenSpec delta，不在本文件复制另一套要求。

## 边界与调用链

前端WorkspaceAdminPanel→api/intelligence.ts→既有http DELETE /intelligence/members/:userId→routes_p5.go→reqDB→RemoveMember。角色修改沿同API层→routes_p7.go→SetMemberRole。成员列表及canManage继续由后端返回，owner工作区有效scope为admin，但展示角色保持owner。

专题HTTP POST/PATCH及本地intel_save_topic工具最终进入CreateTopic/UpdateTopic；统一修canWriteTopics使其复用既有intel.reports.write scope，避免只修路由遗留工具失败。现有租户和workspace过滤、版本乐观锁及错误合同保留。

## 撤销与角色变更的事务顺序

1. 入口继续先拒绝非admin/owner scope；在原事务/保存点中以scope.TenantID精确查询并锁该tenant行。PG使用FOR UPDATE，SQLite依GORM方言和原事务行为，不引入新全局锁。
2. 两个入口以相同顺序“tenant→目标membership→必要计数→写入”执行，避免不同目标行锁无法保护最后管理员总数。只串行同tenant成员管理，不阻塞其他tenant或普通读取。
3. 查当前tenant的目标active成员，owner立即拒绝，不触及其workspace-membership。删除/降级admin前检查仍有效owner/admin数量；只有存在其他管理者才允许，包括用户已确认的管理员自撤销。
4. 任何查询、锁、计数、成员状态更新或workspace-membership删除/角色更新错误均返回；状态与workspace变更由同事务原子回滚。不得以RowsAffected=0或count错误掩盖成功；重复撤销遵循明确未找到/拒绝错误，不返回假成功。
5. tenant级撤销保持现行语义：仅删除目标成员在该tenant下所有workspace的关系，个人scope和其他tenant不变。现有中间件和RLS策略不改；不在此轮修通用Commit错误。

仅给RemoveMember加锁而让SetMemberRole不参与，会保留并发降级路径，因此两个共享入口必须同时参与。备选advisory lock虽可行，但增加PG专有键与SQLite分支，未采用。无需抽象新的锁服务或角色框架。

## 前端状态与接口

OrgMember.role和setMemberRole类型补analyst；ROLE_LABEL新增“分析员 (analyst)”，normalizeEditableRole保留该值。邀请和成员下拉加入同一现有枚举，不把未知/owner可编辑化，保留member兼容editor。

在成员行仅canManage且目标非owner时提供“撤销成员”按钮。组件保留待撤销成员与提交中状态，AppModal明确姓名及该组织所有workspace影响；取消清空目标且不调用API。确认必须检查当前目标、workspace和loading；调用现有http DELETE，不使用原生fetch或静态Modal.confirm。成功才关闭/提示并刷新成员、目录，失败保留目标和真实错误供重试；loading禁用重复确认及取消，避免关闭后回执作用到其他成员。不得显示已移除后再吞掉失败。

当前仓库无jsdom/testing-library，不增加依赖。Bun复用静态渲染与协议检查验证可发现入口、角色回显；确认/取消/失败/重复点击的运行行为由真实应用浏览器检查承担。若运行镜像未更新，必须标未执行，不把静态源码断言当交互通过。

## 最小失败检查与验证

- RemoveMember：旧实现对owner目标不得造成workspace成员丢失；非管理、跨tenant、最后管理员/管理员自撤销及两workspace撤销。普通PG新库注入workspace删除错误核对原子回滚；并发撤销/降级后仍有有效管理者。
- UI/API：analyst规范化/中文标签、管理可见/owner和viewer不可见撤销命令、http DELETE正确workspace header；真实页面覆盖确认取消/失败/成功刷新。
- 专题：先旧代码analyst Create/Update失败，再同scope HTTP/共享工具入口成功；viewer/未知拒写，analyst成员管理仍拒绝，跨workspace404、版本冲突不退化。
- 每个任务先看RED原因，再最小实现GREEN；相关Go/PG与Bun专项后执行Flow包及web构建，不默认全量测试。静态、fixture、PG、真实UI证据分别记录。

## 数据、部署与交付

无API/表结构/RLS或依赖变更。失败注入只在PG新测试库，18088既有账号/邀请/组织/画布不做破坏性操作。原dirty文档、归档配置、Agent面板及未跟踪文件保留；精确暂存本变更路径。用户禁止子代理，计划和实现由主会话执行，自动审查不派发；执行隔离另经本轮联合选择。

本轮开发不等于18088已部署。若真实UI需更新运行环境，使用现有隔离Compose/备份并保留卷；未执行则登记部署待验。不得启动真实模型、外发、生产迁移或推送/合并/发布。后续仍需当前HEAD的浏览器撤销/analyst端到端复验和剩余U/A门槛。
