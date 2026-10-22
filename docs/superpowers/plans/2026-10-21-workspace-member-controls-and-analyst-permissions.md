---
change: fix-workspace-member-controls-and-analyst-permissions
design-doc: docs/superpowers/specs/2026-10-21-workspace-member-controls-and-analyst-permissions-design.md
base-ref: 2b47d1438f6574f8c6c717cb182c0b83e9460d17
archived-with: 2026-10-22-fix-workspace-member-controls-and-analyst-permissions
---

# 成员撤销与 analyst 权限实施计划

<!-- comet-task-authority: openspec/changes/fix-workspace-member-controls-and-analyst-permissions/tasks.md -->

任务状态唯一来源为OpenSpec tasks.md。2026-10-21补齐稳定ID映射，合并未映射的第二份checkbox清单，不改变已确认范围和验收要求。主会话executing-plans逐任务先RED再GREEN；保护原dirty、未跟踪文件和18088数据，不调用模型、启动子代理、外发、生产迁移或推送/合并/发布，不关闭RLS、不新增API/表结构/依赖。review off仅不派发自动审查，主会话仍检查安全边界。

## 1. 撤销共享安全入口

<!-- comet-task-ref:member-boundary -->

文件：backend/internal/flow/orgs.go、p7.go、member_controls_test.go、member_controls_pg_test.go，复用testDB/pgTestDB及P5/P7回归；同步http-api/pending-test。

1. 最小RED：CreateOrganization建立owner/admin；RemoveMember(owner)应ErrForbidden且ResolveScope保持成功；覆盖最后管理员、跨租户和重复撤销。`go test ./internal/flow -run TestMemberControls -count=1`记录旧实现失败，PG未配DSN的跳过不算通过。
2. 两个原事务均先用GORM clause.Locking UPDATE精确锁tenant，再查active目标；拒绝owner，删除/降级admin前计数active owner/admin。查询/计数/更新/删除错误返回并原子回滚，不新增锁服务。
3. GREEN：非管理/跨租户拒绝、有其他管理者允许自撤销、最后管理员拒绝、双workspace撤销和个人scope保持；普通PG新库验证两连接撤销/降级竞争、触发器删除失败全回滚。禁止运行库注入故障。
4. 精确提交：`fix(flow): 成员撤销 - 保护所有者与并发管理边界`。

## 2. 前端撤销与analyst

<!-- comet-task-ref:member-ui -->

文件：web/src/services/api/intelligence.ts、workspace-admin-panel.tsx、intelligence-panels.test.tsx、intelligence-member-api.test.ts；同步features/pending-test。

1. RED：normalizeEditableRole(analyst)保持analyst及中文标签；真实组件管理者非owner可见撤销，owner/不可管理者不可见。`bun test test/intelligence-panels.test.tsx`记录失败；复用SSR不新增DOM依赖。
2. OrgMember/setMemberRole补analyst，邀请/编辑选项及标签一致；复用http.delete、encodeURIComponent和X-Workspace-Id，不新建信封或fetch。
3. AppModal明确组织全部workspace影响，点击仅打开、取消不请求；同步ref防重复，提交中禁止取消；失败保留目标和错误，成功才关闭、提示和刷新成员/目录。
4. GREEN：组件/API专项及18088真实取消请求数、并发失效错误、双击一次请求、成功刷新/原成员会话失权、analyst邀请/编辑及PG角色核对。未部署时明确待验，不把SSR称交互通过。
5. 精确提交：`fix(flow): 成员管理 - 补齐撤销确认与分析员选项`。

## 3. analyst专题共享门禁

<!-- comet-task-ref:topic-scope -->

文件：backend/internal/flow/topics.go、topic_permissions_test.go，复用gaps9_test.go及本地工具；同步角色文档。

1. RED：analyst Create/Update及Gin POST/PATCH应成功，旧门禁应导致Create/POST拒绝。`go test ./internal/flow -run TestTopicAnalyst -count=1`记录原因。
2. 仅共享canWriteTopics改为hasScope(s.Role,"intel.reports.write")，不改变scope映射/签名/对象过滤。
3. GREEN：服务Create/Update、工具保存、HTTP、viewer/未知拒写、analyst成员管理拒绝、跨workspace GET/PATCH404、版本409，保留tenant过滤和乐观锁。
4. 精确提交：`fix(flow): 专题权限 - 对齐分析员报告写scope`。

## 4. 集成与证据

<!-- comet-task-ref:integration-evidence -->

文件：本变更tasks/.comet.yaml、验证报告和带时间戳docs/flow交接；必要时精确同步delta文档。

1. 相关Go/普通PG、Bun组件/API、Flow包构建、bun run build及lint；仅因新失败扩大检查，不改旧dirty。
2. 对照两份delta逐场景记录实际/引用/未执行。18088更新前备份、核对卷和镜像归属，仅换platform/web、关闭自动迁移，8088不动；构建含旧dirty如实标注。
3. 主会话检查权限/错误/并发/调用方，不派发审查；故障注入只新PG测试库，UI只使用专用测试组织及成员。
4. 报告/交接记截图、HTTP/PG、旧数据保护、模型零新增及剩余U/A/G门槛；Comet full检查后等待本次最终归档确认。精确提交，不推送/合并/发布。
