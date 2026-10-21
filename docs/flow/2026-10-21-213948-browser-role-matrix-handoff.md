# Handoff：浏览器邀请、角色矩阵与双工作区隔离验收

## Session Metadata

2026-10-21 21:39:48 +08:00，Asia/Shanghai。WeaveSense-Flow，codex/fix-invite-accept-rls-context，基线7885052c。后端部署7a009195不变，本轮仅验收和文档，最终HEAD用git核对。

## Handoff Chain

Continues from：[HTTP部署交接](2026-10-21-212446-invite-rls-deployed-handoff.md)。邀请hotfix已归档/部署，不重复开发。

## Current State Summary

浏览器真实登录、创建专用组织、viewer邀请生成/接受、成员投影、重用拒绝及viewer拒写通过。页面无撤销按钮，用owner真实HTTP撤销后原浏览器会话立即拒写、刷新组织目录移除。五角色核心矩阵和同租户双workspace共106项HTTP匹配当前预期；analyst前端支持和专题写权限存在缺口，完整角色合同仍未通过。

## Important Context

用户授权验收，禁止模型、关闭RLS、子代理、推送/合并/发布，全程遵守。无产品代码修改、无服务重建、无新Comet工件。原dirty与未跟踪文件保留。原localhost用户tabs1/2未操作；127.0.0.1的tab3为测试viewer，已撤销两个本轮组织成员身份，保留个人scope。

## Codebase Understanding

WorkspaceAdminPanel提供创建/接受邀请和三角色编辑，缺撤销按钮、analyst选项/标签。ScopesForRole支持analyst写scope，但topics.go的canWriteTopics只允许admin/editor。ListMembers按租户枚举再投影workspace角色；SetMemberRole同步租户全部workspace角色，RemoveMember租户级撤销。无POST workspace API，第二workspace需测试准备。

### Critical Files

- `web/src/pages/intelligence/workspace-admin-panel.tsx`：撤销/analyst前端缺口。
- `backend/internal/flow/topics.go`：analyst专题权限分歧。
- `backend/internal/flow/p7.go`：角色更新及成员投影。
- `docs/flow/2026-10-21-browser-role-workspace-acceptance.md`：实际证据、矩阵及限制。

## Work Completed

真实浏览器页面操作与截图通过，邀请令和密码仅本地私有读取/保存未输出。viewer生成邀请及新增信源失败，原令牌重用失败；owner API撤销后同浏览器原会话立即显示workspace not found，刷新目录不再有组织。

HTTP新专用tenant验证owner/admin/editor/analyst/viewer读写和管理门禁，106项通过。第二workspace只准备新行，实际列表/详情/更新/信源撤销隔离断言经HTTP；先无成员时B404，后A/B都有成员时对象仍隔离。最终测试成员对A/B撤销立即404。测试信源仅新增，不采集、不设调度。

只读快照确认原非users平台行和实采report/document/revision哈希不变；登录元数据更新、用户ID不变，容器ID和RLS不变，模型任务及预算预留0。新增专用测试数据保留不清理，测试成员均removed。

### Decisions Made

不将API撤销+浏览器失权称纯页面撤销闭环；不把当前analyst403写成规格通过。未擅自新增按钮或更改权限，后续修复按/comet聚焦工件确认。无模型的矩阵仅覆盖已列端点，不冒充完整所有业务角色组合。

## Pending Work

优先处理新缺口：撤销成员UI入口；analyst邀请/标签/编辑与专题写权限合同。后续需追踪真实调用方、先最小失败检查，按Comet新聚焦变更评估，不直接沿用旧归档设置。完整权限矩阵仍未收口；同租户双workspace核心对象隔离已通过，不重复建设工作区创建能力。离线/跨标签、真实MCP/SSE/未知结果恢复、旧库迁移/恢复、监控/多实例/外发及G01门槛继续待。

## Immediate Next Steps

1. 核对git/HEAD/branch、openspec0活跃与本证据，原归档不重跑。
2. 用户如要求修上述缺口，先/comet解析范围及工件确认，再RED→修复，不把验收授权扩大为功能开发。
3. 继续其余无模型验收；不无故重跑check-role-matrix.py（会创建组织/对象）或更改原成员。

## Environment State

18088运行前轮镜像7a009195，无重建；8088不动。tab3保持127.0.0.1 viewer个人scope，截图在.local/flow-uat-20261020/browser-role-uat。matrix-setup.json记录新A/B及tenant，matrix-result.json/检查脚本保存106项；browser-org.json记录浏览器专用组织。私有邀请令文件不提交。

## Potential Gotchas

保留原dirty文档、归档配置、Agent面板和所有未跟踪文件；不reset/clean/restore、不git add .。角色矩阵使用同一成员依次赋权，不是假称五个同时登录账号。第二workspace及其初始成员经SQL准备，仅新test tenant，不称API创建。前端analyst缺口与专题403需分辨规格/实现，不随手改。无.codegraph不建索引。

## Related Resources

[本轮证据](2026-10-21-browser-role-workspace-acceptance.md)、[前轮31项](2026-10-21-invite-rls-deployment-acceptance.md)、[原修复报告](../superpowers/reports/2026-10-21-fix-invite-accept-rls-context-verify.md)。
