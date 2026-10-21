# 邀请接受 RLS hotfix 验证

2026-10-21 21:07 +08:00，Asia/Shanghai。基线 `6d7c00ec55548f3c4419b800d5139e113a545812`，分支 `codex/fix-invite-accept-rls-context`。用户已确认本次隔离和 proposal/design/tasks；本轮不关闭RLS、不调用模型、不启动子代理、不推送/合并/发布，保留既有修改与数据。

## 根因、调用方与修复

`TenantMiddleware` 在请求开始将已有租户范围写入局部事务。`routes_p5.go` 接受邀请时取 reqDB 进入 `AcceptInvite`；新组织不在受邀者原范围，原查询被 `flow_invites` FORCE RLS 读过滤。唯一生产后端调用方是该POST路由；其他为p5/p7测试。前端 `intelligence.ts` acceptInvite 经现有http调用，workspace-admin-panel成功后刷新工作区目录。

仅修改 `orgs.go` 共享入口：在已有事务/保存点内读取旧范围，复用 `withSystemContext` 仅按精确token_hash加FOR UPDATE锁查询一个邀请，立即恢复旧范围；有效凭证后设为该邀请精确tenant，完成原有领取和成员写入，恢复旧范围后返回。系统范围不用于成员写入；不改TenantMiddleware、TenantTx、RLS策略、邮箱规则、角色映射、API或表结构。

数据库查询、条件更新和范围恢复错误向上传递；只有ErrRecordNotFound被当成不存在，不以任意查询失败触发新建。失败依靠原事务/保存点回滚。并发在锁后重新检查终态，仅一人可消费邀请。空用户或令牌拒绝。SQLite沿用事务、无需PG上下文SQL。

## 本轮实际执行

| 检查 | 结果 |
| --- | --- |
| 新增 TestAcceptInviteRLSPostgres/HTTP，旧实现 RED | 退出1，有效邀请真实TenantMiddleware+RegisterP5Routes HTTP400，body为invite invalid or used；无新增成员 |
| 修复后邀请专项 GREEN | 退出0，HTTP、nested-context-and-isolation、invalid-expired-revoked、write-failure-rolls-back-and-restores、concurrent-single-use全部通过 |
| 最终增强PG回归 | 退出0；新增independent-call后邀请6子场景通过，CreateOrganizationRLSPostgres四子场景及既有RuntimeRLSBlocksUnfilteredHandlerQueries、BackgroundWorkIsTenantScoped、RLSCapabilityAndProviderTables通过；共5顶层 |
| 宿主 go test ./internal/flow -run 'Test(Org\|P7Members\|Budget)' -count=1 | 退出0，既有组织/邀请单次使用/撤销、成员角色、预算预留结算释放与配额共4顶层通过 |
| go build ./internal/flow | 退出0 |
| gofmt、git diff --check、openspec validate fix-invite-accept-rls-context --strict | 通过；skip_specs=true，无规格要求变化 |

PG测试复用pgTestDB创建并迁移新flow_rt_时间戳库。执行角色flow_uat_invite_rls_20261021是NOSUPERUSER/NOBYPASSRLS/CREATEDB，专项直接检查current_user/session_user无super/bypass、flow_invites启用且强制RLS。仅角色准备和清理用已有容器管理员；回归全部由普通角色执行，不以superuser通过冒充隔离有效。

RED/GREEN执行程序与日志在Git忽略的 `.local/flow-uat-20261020/run-invite-rls.py`、`invite-rls-red.log`、`invite-rls-green.log`、`invite-rls-final.log`。测试二进制为当前源码CGO_ENABLED=0的Linux测试包，仅运行PG专项，未用该二进制跑SQLite。日志可能有哈希及SQL，仅本地保留，不输出密码或令牌。

专项内触发器断言邀请accepted更新、member插入、非admin workspace-member插入的app.tenant_ids恰好等于目标tenant；这是检查系统范围没有被用于写入。注入workspace-member写失败后，邀请仍未用，原事务范围和个人账务可读，事务外tenant/workspace-member均只有owner，未因RLS隐藏残留而假通过。并发两连接不同用户竞争同令牌，只有一次成功、一次ErrInviteInvalid，成员各一条。成功/无效/失败路径均核对原范围，成功后原事务仍看不到目标邀请或账务；独立调用恢复空原范围。

HTTP成功场景校验真实响应role=viewer、ResolveScope为viewer且无信源写scope；通过既有RemoveMember后ResolveScope立即拒绝。此为真实PG中间件路由集成与域级权限检查，不宣称18088浏览器完成了邀请/撤销流程。

测试库由pgTestDB清理；本轮角色已DROP，最终flow_rt_库数0、该角色数0。18088原账号、未用邀请、组织、画布、实采文档和报告没有操作；未更新镜像或启动服务。

## 轻量验证判定

| 项目 | 结果 |
| --- | --- |
| 任务完成 | 3/3，RED→GREEN及相关检查已执行 |
| 根因消除 | 共享入口不再使用受邀者旧范围查找跨租户邀请；系统范围仅限令牌单行查询 |
| 构建与专项 | 上表真实执行均通过 |
| 范围和安全 | 单生产文件/单PG检查文件，角色/租户/错误/原子性保留，无策略或依赖变化 |
| 文档 | 功能/API边界、pending-test与交接同步，旧修改独立保留 |
| 自动审查 | hotfix默认review_mode=off，用户禁止子代理；跳过自动代码审查，主会话检查全部调用方与事务路径 |

Comet build guard实际14/14、verify guard实际5/5通过，verify_result=pass、phase=archive、archive_confirmation=pending。scale统计11文件选full，按技能允许显式override为light：本次仅局部生产入口+PG专项，其余是工件/文档，无delta、迁移或跨模块实现；不豁免真实安全回归。实现提交c94988d5，后续仅补验证状态和交接。

## 未执行与剩余门槛

未执行全量Go、前端构建、18088镜像更新或真实登录接受复验；前轮真实400保留为旧镜像失败，源码通过不能覆盖部署门槛。数据库设置/恢复SQL本身的失败未单独故障注入，本轮实际注入成员写失败覆盖保存点回滚；代码检查确认设置/恢复错误均返回。

完整角色矩阵、同租户多workspace、离线/跨标签缓存、真实MCP/外发、旧库迁移、SSE/未知ACK恢复、监控/多实例仍待验。模型额度0，不可自动追加。G01交付另待明确授权；本次本地归档仍须最终确认。
