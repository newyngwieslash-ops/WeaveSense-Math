# Handoff：18088组织修复部署，邀请接受仍阻塞

## Session Metadata

2026-10-21 20:28 +08:00，Asia/Shanghai。WeaveSense-Flow，分支codex/fix-organization-create-rls-context，部署源码HEAD d5f17bfa；后续仅补验收文档，最终HEAD以git核对。

## Handoff Chain

Continues from：[本地归档交接](2026-10-21-201057-organization-rls-archived-handoff.md)。本次按用户明确授权更新18088隔离后端、保留数据并复验，不重复归档或开发同一问题。

## Current State Summary

18088已部署包含CreateOrganization修复的后端，healthy、健康接口commit=d5f17bfa；真实两账号各自POST /orgs=200。组织/共享账务和多账号拒绝共51项通过，创建邀请另1项通过；接受邀请第53项返回400，完整检查脚本退出1。不能称完整权限或发布通过。Comet仍0活跃，本hotfix早已归档。

## Important Context

用户授权仅隔离部署与验收；未关闭RLS、调用模型、启动子代理、推送、合并或发布，无产品源码改动。现有dirty与未跟踪文件保留。模型额度0。仅platform被替换，web/Engine/scheduler/数据库及8088容器不变，所有原卷保留。

## Codebase Understanding

已修CreateOrganization在局部事务中精确初始化新tenant并恢复旧范围，真实HTTP已通过。新发现AcceptInvite依旧在请求已有租户上下文直接按token_hash查询flow_invites，受邀者尚未属于目标tenant而被FORCE RLS读过滤；与组织初始化的插入42501不同，本轮接受失败无42501日志。

### Critical Files

- `backend/internal/flow/orgs.go`：CreateOrganization与AcceptInvite共享实现。
- `backend/internal/flow/routes_p5.go`：真实组织/邀请HTTP入口。
- `backend/internal/database/flow_rls.go`：invites等表FORCE RLS规则。
- `docs/flow/2026-10-21-org-rls-deployment-acceptance.md`：本轮证据及部署限制。

## Work Completed

两库pg_dump备份和旧镜像已保存到Git忽略的本地目录。常规Docker构建因远端元数据损坏失败，改用已有Go镜像禁网、只读源码/模块、CGO静态编译，再沿用旧运行镜像仅覆盖binary；非完整标准生产镜像，不称全栈纯HEAD。CANVAS_AUTO_MIGRATE=false；健康接口commit及宿主/容器binary SHA-256一致。详见证据报告。

替换后、测试前9张关键表指纹一致；测试后原租户/成员/工作区/账务/画布无删除/改变，两个测试用户登录信息更新，新增2个专用空组织及默认workspace/owner/共享账务。实采reports/documents/revisions指纹不变。平台角色无super/bypass、三张账务执行表ENABLE/FORCE RLS一致；模型任务和预算预留仍0。

真实HTTP检查两账号组织创建、成员与账务读取、相互组织读写404、系统管理员403、原报告五格式在自身/伪造workspace下404、匿名401，共51项通过；邀请创建200后接受400。只读BEGIN/ROLLBACK反证确认邀请存在、有效、未用、未撤销，但受邀者已有tenant范围可见数0。没有SQL或域服务补建成员掩盖失败。

### Decisions Made

保留当前分支、所有既有数据和修改；镜像仅本地验收，不发布。保留新组织和未使用邀请供调查。邀请单次使用、viewer加入、权限切换和撤销即时拒读因为接受失败未执行；不擅自扩展修复新的安全入口。

## Pending Work

邀请接受400需要下一聚焦hotfix：沿真实调用方和RLS边界设计凭证兑换入口，先最小RED，不能清空上下文绕过整个请求。当前未新建Comet工件或取得该新修复的工件确认。完整角色矩阵/同租户多workspace、离线/跨标签缓存、旧库迁移/恢复、真实MCP/SSE/未知结果恢复、监控/多实例、实采外发和G01交付仍未通过。O01–O06不擅自实施。

## Immediate Next Steps

1. 核对git/HEAD/branch与18088健康commit；现有归档不重跑。检查本轮证据，不把52通过覆盖第53失败。
2. 若用户继续修复邀请接受，按/comet新聚焦hotfix形成具体工件、最小PG失败检查与安全方案，再实施；不能把本轮验收授权扩展成任意权限重构。
3. 修复后复验真实邀请接受、单次使用、viewer权限和撤销；不消耗模型，不操作8088、生产或外发。

## Environment State

Compose flow-uat-20261020/18088运行，附加配置.local/flow-uat-20261020/compose.org-rls.yml；platform使用新的局部运行镜像，其余服务旧镜像不变。所有后续操作须带原compose.acceptance.yml及本轮override，避免丢失构建与自动迁移设置。旧镜像标签flow-uat-platform:before-org-rls-20261021保留。

本地org-rls-deploy目录有before/after-deploy/after-tests指纹、两库备份、HTTP逐项结果、新组织ID与邀请范围反证。账号密码只在原Git忽略私有文档；没有输出凭据。未启动dev server，宿主后端数据目录约束仍适用。

## Potential Gotchas

不要重跑check-org-deployed.py以免重复创建空组织；其失败退出为真实邀请阻塞。静态链接器glibc警告和混合运行层意味着未测SQLite扩展/用户查找等兼容性，正式交付需要标准构建恢复后复验。--version不受支持，临时无网络无挂载探测容器已停止移除。原dirty画布及缓存不能清理。保护旧归档、Agent面板、prd-audit-9.md、table.csv与历史handoff，无.codegraph不建索引。

## Related Resources

[本轮证据](2026-10-21-org-rls-deployment-acceptance.md)、[原代码验证](../superpowers/reports/2026-10-21-fix-organization-create-rls-context-verify.md)、[前轮导出权限](2026-10-21-report-export-account-isolation-evidence.md)。
