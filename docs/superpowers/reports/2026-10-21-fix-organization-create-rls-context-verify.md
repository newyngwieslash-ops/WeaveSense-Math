# 创建组织RLS hotfix验证

2026-10-21，Asia/Shanghai。基线21bb303a，分支codex/fix-organization-create-rls-context。用户已确认新分支及proposal/design/tasks；禁止关闭RLS、真实模型、子代理、推送/合并/发布，全程遵守。

## 根因与实现

TenantMiddleware按请求开始时的已有租户设置app.tenant_ids，routes_p5.go取reqDB后调用CreateOrganization，新建组织不在原范围。Billing.Account初始化新租户账务被42501拒绝。生产调用方仅POST /orgs；其他调用为既有组织/角色测试和此前Git忽略的验收准备程序。

修复只在orgs.go共享入口：在现有事务/保存点内读取原范围，为服务端新生成tenant ID设置精确局部范围，初始化成功后恢复原值。任何SQL或恢复失败返回错误，事务/保存点负责完整回滚。不使用withSystemContext、不关闭RLS、不改TenantMiddleware、TenantTx或Billing.Account。SQLite路径不执行PG设置；无API、表结构或依赖变化。

## 本轮实际验证

| 检查 | 实际结果 |
|---|---|
| 新增TestCreateOrganizationRLSPostgres，旧实现RED | 退出1；真实TenantMiddleware+RegisterP5Routes POST /orgs=500，SQLSTATE42501；失败无组织记录。其他子场景揭示错误初始化范围 |
| 修复后同PG专项GREEN | 退出0，4子场景：HTTP、nested-context-and-isolation、failure-rolls-back-and-restores、independent-call通过 |
| 补充事务外检查后的最终PG回归 | 退出0，4子场景通过，失败创建的tenant/membership/workspace/workspace-membership/billing均无残留，避免只靠RLS隐藏残留的假通过 |
| 非超级用户PG既有RLS专项 | TestRuntimeRLSBlocksUnfilteredHandlerQueries、TestBackgroundWorkIsTenantScoped、TestRLSCapabilityAndProviderTables全部通过；连同本次为4个顶层测试 |
| 宿主go test ./internal/flow -run 'Test(Org\|P7Members\|Budget\|CreateOrganizationRLSPostgres)' -count=1 -json | 退出0，4顶层通过；PG项宿主无DSN跳过，已独立实际执行，不计为宿主通过 |
| go build ./internal/flow | 退出0 |

所有PG执行在flow-uat-20261020-postgres-1新建flow_rt_时间戳测试库。专用角色flow_uat_org_rls_20261021为NOSUPERUSER/NOBYPASSRLS/CREATEDB；测试实际检查session_user/current_user无super/bypass、flow_billing_accounts ENABLE和FORCE RLS。仅准备/清理角色使用既有容器管理员，不以超级用户跑RLS回归。最终库残留0、该角色已删除/残留0。

测试故意引发越租户写42501与注入账务P0001，这些日志是预期拒绝，测试用保存点恢复。测试触发器还检查初始化范围必须等于新tenant，拒绝空系统范围。成功恢复后的旧范围看不到新tenant/其他用户账务；旧事务仍可查询自己的账务。独立调用在默认系统连接上也只在初始化期设置新tenant，完成恢复原值。

## 轻量验证与边界

Comet build guard14/14、verify guard5/5实际通过，verify_result=pass、phase=archive、archive_confirmation=pending。自动scale因10文件（含工件）选full，核对只有1个生产文件/1个测试文件及配套文档后按技能显式override light，无规格或跨模块变化，不豁免实际安全专项。

任务3/3；实现聚焦1个生产文件+1个PG回归文件，工件/文档配套。无密钥、RLS策略或权限弱化。review_mode=off，不自动子代理审查；主会话检查了全部调用方和事务边界。若scale因文档/工件文件数选full，按技能允许覆盖为light，理由是无跨模块协调/规格变化且真实PG安全专项已执行。

未执行：产品全量Go/前端构建、18088镜像重建/替换、真实登录HTTP部署后重验、模型和外发。已有18088仍旧镜像；此前组织创建500保留历史证据，源码PG集成通过不等于18088部署通过。完整角色矩阵、邀请/同租户多workspace、缓存竞态、迁移/恢复/SSE/未知ACK与G01发布门槛仍待验。

旧pending-test/pending-acceptance增量、归档.comet.yaml、前端Agent面板及全部未跟踪文件保持。只精确提交本次pending-test新增节及hotfix文件，不git add .。本轮不归档，待用户最终确认。
