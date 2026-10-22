# Proposal：成员撤销入口与 analyst 权限一致性

## Why

18088验收发现成员页面没有撤销入口，analyst缺少中文标签、邀请和角色编辑选项，并被当成editor回显；后端虽授予analyst报告写scope，专题共享入口仍拒绝该角色。现有RemoveMember还存在owner租户成员不变、工作区成员却可能被删除的静态线索，需要先最小RED核实后修复共享安全入口。

## What Changes

- 在现有成员页面为可管理的非owner成员提供“撤销成员”命令与明确确认弹窗，说明会撤销该组织所有工作区权限；成功后刷新成员及工作区目录，失败保留真实错误。复用既有DELETE接口和http，不新增API。
- 补齐analyst中文标签、邀请/角色下拉、类型与回显，保留member→editor已有兼容；owner不可编辑/撤销。
- 将专题共享写权限与现有intel.reports.write scope合同一致，使analyst可以新建/编辑专题，仍禁止viewer及未知角色写入和analyst管理成员。
- 测试确认并修复RemoveMember的owner保护；复用既有最后管理员约束的判定模式，避免接入UI后通过直接调用或竞态绕过安全边界。任何持久化失败需完整回滚，不放宽RLS。
- 各模块先最小失败检查再实现，随后相关专项/构建，文档与待验同步；不把18088旧镜像验收当作本次已部署。

## Capabilities

### New Capabilities

无。复用既有组织成员、邀请、专题能力，不新增工作区创建、模型或迁移能力。

### Modified Capabilities

- `intelligence-workspace`：完善“工作区成员角色准确投影与编辑一致”，补analyst前端回显及经确认的撤销入口、owner保护、撤销后权限与目录刷新场景。
- `flow-workspace-roles`：补充analyst专题新建/编辑的具体写scope场景，明确不得进入成员管理操作。

## Impact

基线2b47d1438f6574f8c6c717cb182c0b83e9460d17，当前仍在上轮分支，尚未选择本轮隔离。Comet识别为前端/后端协调和撤销安全修复，采用full流程，不继承旧hotfix配置。

前端涉及workspace-admin-panel.tsx、api/intelligence.ts及既有intelligence-panels测试；后端涉及topics.go共享canWriteTopics与orgs.go共享RemoveMember及其真实路由/PG检查。生产后端撤销调用方为routes_p5.go，其他为既有p5/invites回归；专题HTTP与本地工具共同复用CreateTopic/UpdateTopic。不得只修单条HTTP路径。

非目标：不改变租户级撤销和跨工作区同步角色的现有合同、不新增每工作区独立角色编辑，不重做邀请兑换、不改Engine/数据库结构/RLS策略、不新增依赖或通用角色框架；不操作原用户或原验收组织成员。旧dirty文档、归档配置、Agent面板和全部未跟踪文件独立保留。

关键待证：owner/最后管理员撤销边界与事务锁；页面确认/取消/失败分支；analyst专题HTTP及工具共同入口。采用一个聚焦变更，因为三处缺口共同阻塞成员管理/角色验收，测试与规格需保持同一合同。若发现需要新API、迁移或独立角色模型，停止并重新确认范围。全程无模型、子代理、推送/合并/发布。
