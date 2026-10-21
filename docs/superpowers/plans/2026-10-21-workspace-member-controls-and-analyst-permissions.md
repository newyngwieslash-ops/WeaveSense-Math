---
change: fix-workspace-member-controls-and-analyst-permissions
design-doc: docs/superpowers/specs/2026-10-21-workspace-member-controls-and-analyst-permissions-design.md
base-ref: 2b47d1438f6574f8c6c717cb182c0b83e9460d17
---

# 成员撤销与 analyst 权限实施计划

> 主会话按 executing-plans 逐任务执行，用户禁止子代理。每项先最小RED，再实现GREEN，不跳过失败原因检查。

**Goal:** 补齐成员撤销UI、analyst回显及专题权限，确保owner/最后管理员和组织隔离。

**Architecture:** 复用现有http DELETE、AppModal、成员事务和报告写scope。两个成员管理入口先锁tenant，专题只修共享canWriteTopics；无新框架或依赖。

**Tech Stack:** Go/GORM/Gin/PostgreSQL、React/TypeScript/AntD、Bun。

## Global Constraints

保留原dirty/未跟踪文件与18088数据；不reset/clean/restore、不git add .。不关闭RLS、不调用真实模型、不启动子代理、不推送/合并/发布。不新增API/表结构/依赖。PG故障注入只在新测试库，真实UI只用专用测试成员。管理员自撤销仅在仍有其他active owner/admin时允许，owner永不撤销。

## Task 1：撤销共享安全入口（OpenSpec 1.1）

**Files:** 修改backend/internal/flow/orgs.go、p7.go；新增backend/internal/flow/member_controls_pg_test.go，复用p5_test.go/p7_test.go与pgTestDB；同步docs/content/docs/backend/http-api.mdx、pending-test.mdx。

**Interfaces:** RemoveMember(db,Scope,userID) error与SetMemberRole(db,Scope,userID,role) error签名不变；现有routes_p5/routes_p7保持响应合同。

- [ ] 先写最小owner保护RED，在testDB创建组织与owner scope，调用RemoveMember后断言返回ErrForbidden且ResolveScope(owner,workspace)仍成功。最小断言形式：

```go
err := RemoveMember(db, admin, ownerID)
if !errors.Is(err, ErrForbidden) { t.Fatalf("owner revoke: %v", err) }
if _, err := ResolveScope(db, ownerID, workspaceID); err != nil { t.Fatal(err) }
```

- [ ] 运行 `cd backend; go test ./internal/flow -run TestMemberControls -count=1`，记录旧实现是否返回nil/丢owner工作区。PG专项通过现有pgTestDB和普通NOSUPERUSER/NOBYPASSRLS角色运行，未配置DSN跳过不能算PG通过。
- [ ] 在两个原事务的目标查询前以相同顺序锁tenant，再校验target与计数；复用GORM现有clause.Locking，不新增锁服务。核心锁形式：

```go
var tenant model.Tenant
if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&tenant, "id = ?", scope.TenantID).Error; err != nil { return err }
```

仅目标active成员可变；owner拒绝。目标admin撤销或降级时检查同tenant剩余active owner/admin，count错误直接返回。成员状态及workspace关系变更仍在同事务，不提前commit。
- [ ] GREEN覆盖：非管理角色/跨tenant拒绝，允许有其他管理员的自撤销，最后管理员拒绝，普通PG两连接撤销/降级竞争、workspace-member删除触发器注入错误后全回滚。外层scope与旧记录继续可读，个人scope保持。运行已有Org/P7Members及RLS专项，更新接口边界说明。
- [ ] 精确提交该任务代码/测试/文档和tasks勾选；提交格式 `fix(flow): 成员撤销 - 保护所有者与并发管理边界`，不暂存旧清单增量。

## Task 2：前端撤销与analyst（OpenSpec 2.1）

**Files:** 修改web/src/services/api/intelligence.ts、web/src/pages/intelligence/workspace-admin-panel.tsx、web/test/intelligence-panels.test.tsx；必要协议断言复用web/test/intelligence-gaps9.test.ts；同步features.mdx与pending-test。

**Interfaces:** 新增既有DELETE前端调用removeMember(workspaceId:string,userId:string):Promise<null>（以http实际返回类型为准，不引入信封嵌套）；OrgMember/setMemberRole联合类型增加analyst。组件仍接收workspaceId/onWorkspacesChanged/initialData，不新增生产测试专用参数。

- [ ] RED先补analyst规范化与中文标签、管理者非owner撤销可见、owner及不可管理者不可见的真实组件静态渲染断言；API专项断言DELETE编码ID及X-Workspace-Id。最小现有函数检查：

```ts
expect(normalizeEditableRole("analyst")).toBe("analyst");
expect(ROLE_LABEL.analyst).toBe("分析员 (analyst)");
```

- [ ] `cd web; bun test test/intelligence-panels.test.tsx`记录RED；不以缺DOM环境误判产品失败，不安装jsdom/testing-library。
- [ ] API使用 `http.delete(`/intelligence/members/${encodeURIComponent(userId)}`, ws(workspaceId))`。页面增加analyst下拉/标签，成员行用canManage且m.role!==owner展示命令。复用AppModal和目标成员/loading状态，点击只打开弹窗，确认才请求；描述组织所有工作区影响，失败不移除成员、不提示成功，loading阻止重复提交。
- [ ] GREEN运行组件/协议专项；真实页面检查取消不请求、失败仍显示错误、确认成功目录刷新及撤销后原会话拒绝。页面未部署就标未执行，不把静态渲染称点击验证；不创建独立HTML。
- [ ] 同步功能/待验说明并精确提交，提交格式 `fix(flow): 成员管理 - 补齐撤销确认与分析员选项`。

## Task 3：analyst专题共享门禁（OpenSpec 3.1）

**Files:** 修改backend/internal/flow/topics.go；扩展backend/internal/flow/topics_test.go或同域已有测试，必要时新增最小角色专项；同步角色接口文档和pending-test。

**Interfaces:** canWriteTopics/ CreateTopic/UpdateTopic与intel_save_topic工具签名不变；既有版本锁与tenant/workspace过滤不变。

- [ ] RED在现有测试DB建立analyst scope，调用CreateTopic/UpdateTopic和真实Gin POST/PATCH，预期201/200；旧实现应因canWriteTopics返回false失败。断言示例：

```go
title := "analyst topic"
topic, err := CreateTopic(db, analyst, userID, TopicInput{Title: &title}, time.Now())
if err != nil { t.Fatal(err) }
if topic.WorkspaceID != analyst.WorkspaceID { t.Fatal("workspace mismatch") }
```

- [ ] 运行 `cd backend; go test ./internal/flow -run 'Test.*Topic' -count=1` 并保存RED原因；工具用既有本地执行入口，不调用模型或真实MCP。
- [ ] 仅替换共享门禁为 `return hasScope(s.Role, "intel.reports.write")`，不改scope映射本身。
- [ ] GREEN覆盖analyst HTTP与工具保存/编辑、viewer/未知拒写、analyst成员管理拒绝、跨workspace404及版本冲突；同步文档并精确提交 `fix(flow): 专题权限 - 对齐分析员报告写scope`。

## Task 4：聚焦集成与证据（OpenSpec 4.1）

**Files:** 本变更tasks/.comet.yaml、docs/superpowers/reports/2026-10-21-workspace-member-controls-and-analyst-permissions-verify.md及新时间戳docs/flow交接；需要时精确更新上游delta。

- [ ] 运行受影响Go/PG专项、Flow包构建、Bun组件/API专项；执行 `cd web; bun run build` 和现行lint中与弹窗/Empty有关的检查。仅因新失败扩大检查，不为无关旧dirty运行改写格式。
- [ ] 对照两份delta逐场景记录覆盖。真实18088当前镜像仍旧，需先确认构建归属/保留卷再验证新UI；未更新时明确部署阻塞，不伪造截图或发布状态。
- [ ] 主会话检查权限/错误/并发/调用方与文档，无自动审查派发；标准/彻底审查需要子代理，与用户约束冲突，本轮只能review off并如实记录。
- [ ] Comet full验证、生成报告/交接后等待本次最终归档确认。只精确提交本变更文件，保留用户所有其他修改；不推送/合并/发布。

## 计划自检

四任务对应上游四项，owner/最后管理员/自撤销、analyst回显、scope与RLS、UI取消/失败/成功、跨workspace和错误原子性均有检查。API示例复用现有http，不新增依赖或抽象。执行设置尚未选择，不写入isolation/build_mode/tdd_mode/review_mode；下一联合选择需确认隔离/分支名并审阅本设计和计划。
