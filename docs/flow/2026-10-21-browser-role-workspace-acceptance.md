# 18088 浏览器邀请与角色/双工作区验收

2026-10-21 21:39 +08:00，Asia/Shanghai。分支codex/fix-invite-accept-rls-context，源码HEAD7885052c，部署后端7a009195。本轮不改产品代码、不调用模型、不关闭RLS、不启动子代理、不推送/合并/发布。既有修改和数据保留。

## 浏览器真实路径

使用内置浏览器tab3、127.0.0.1:18088独立站点；原localhost账号/画布/报告tab未操作。读取Git忽略的既有测试账号凭据，仅填真实登录表单，不输出密码。所有者通过页面创建专用组织UAT Browser Roles 20261021，选择该工作区并生成viewer邀请；退出后登录受邀者，在页面输入令牌并点击接受，出现“已加入工作区”，目录新增组织，成员列表显示owner和只读viewer，无成员角色编辑控件。

viewer通过页面生成邀请返回workspace not accessible；重用已用令牌显示invite invalid or used；添加example.invalid测试信源显示missing scope intel.sources.write且列表仍空。没有采集请求或模型执行。

页面不存在撤销成员按钮，源码WorkspaceAdminPanel也只有角色下拉。因此撤销动作通过owner独立真实HTTP DELETE完成，仅针对本轮新组织的测试viewer。**这是API撤销后浏览器失权验收，不是纯浏览器撤销点击闭环。** 原viewer浏览器会话不退出、不刷新直接再点添加，立即显示workspace not found；刷新后回到Default，组织不再出现在目录。缺少撤销入口登记待修，未擅自实现。

截图位于Git忽略的.local/flow-uat-20261020/browser-role-uat：viewer-members.png、viewer-write-denied.png、revoked-viewer-denied.png、revoked-directory.png。邀请令仅存invitation.private.json且未显示/提交。模型配置引导曾遮挡邀请角色下拉，关闭引导后操作成功，未配置或调用模型。

## 五角色核心权限矩阵

复用两普通账号，在新专用组织UAT Role Matrix 20261021经真实HTTP创建组织、邀请接受、依次设置admin/editor/analyst/viewer。owner通过其原账号验证，其他角色在同一测试成员会话中逐次变更，核对有效成员角色。check-role-matrix.py实际退出0，**106项HTTP状态断言及业务字段断言通过**，结果在browser-role-uat/matrix-checks.json与matrix-result.json。

| 操作 | owner | admin | editor | analyst | viewer |
| --- | --- | --- | --- | --- | --- |
| 信源/文档/报告/专题/成员/账务读取 | 200 | 200 | 200 | 200 | 200 |
| 成员canManage | true | true | false | false | false |
| MCP调用审计管理入口（不执行MCP） | 200 | 200 | 403 | 403 | 403 |
| 新增信源（不采集） | 201 | 201 | 201 | 201 | 403 |
| 新增专题 | 201 | 201 | 201 | **403** | 403 |
| 生成邀请/修改普通成员角色 | 200 | 200 | 403 | 403 | 403 |
| 修改owner为viewer | 403 | 403 | 403 | 403 | 403 |

矩阵记录当前可观测行为，不将所有当前行为自动视为正确：`ScopesForRole("analyst")`及flow-workspace-roles规格授予报告/分析等写scope，但canWriteTopics只允许admin/editor，analyst专题写403需决策/聚焦修复。前端ROLE_LABEL、normalizeEditableRole、邀请与成员下拉缺少analyst，可能将分析员投影成编辑者；本轮源码和真实选项确认缺口，未将后端矩阵通过当作前端完整角色通过。未执行模型/MCP调用、采集、付费预算、审批和所有业务端点组合，所以不能称“完整全功能角色矩阵通过”。

## 同租户双工作区隔离

没有工作区创建API，仅GET /workspaces。本轮在上述新测试tenant下直接插入第二workspace B和owner workspace-membership，随后所有权限断言走真实HTTP；不把SQL准备称产品创建链路通过。先不授予测试成员B成员身份，确认其目录只有A、对B报告/信源/专题/成员/账务全部404。然后仅插入该新B的viewer membership，使其同时可访问A/B。

owner分别在A/B通过API创建标记专题和相同URL的独立信源，确认source ID不同；owner与双工作区viewer的列表不混入另一工作区对象，猜另一工作区topic ID详情404；owner即使有A/B权限，跨workspace更新topic和撤销source仍404，正确scope下对象保持可读。最终owner撤销测试成员，成员对A/B均立即404。HTTP同时覆盖平台专题查询和Engine信源查询，不调用采集。

注意SetMemberRole当前对租户全部workspace成员角色一起更新，RemoveMember也是租户级撤销；本轮测试采用现行合同，不把其说成独立每workspace角色编辑。成员列表还按tenant枚举，workspace无membership时角色兜底语义未在本轮扩展验证。

## 数据与运行边界

与前轮invite-rls-deploy/after-tests.json比较，本轮后快照browser-matrix-after.json确认原非users平台行全部哈希不变、原Engine报告/文档/修订哈希不变，用户ID集合不变；登录时间允许更新，非登录字段未逐字段比对。所有记录的容器ID保持，平台角色无super/bypass、邀请/账务/模型/预算表ENABLE/FORCE RLS保持。模型任务0、预算预留0。

新增专用组织/工作区/邀请、专题和信源保留为验收数据；没有修改原用户资料、原组织成员或已有实采对象。两个专用组织的测试成员最终均已撤销。测试信源无调度、不触发采集。未重建服务、未跑产品全量测试，Comet0活跃。

剩余门槛：浏览器撤销入口缺失、analyst前端支持与专题权限口径未闭合，完整角色/业务组合仍待；离线/跨标签缓存、真实MCP/SSE/未知结果恢复、旧库迁移/恢复、监控/多实例及外发仍待。G01未授权，O01–O06不擅自实施。
