# Handoff：18088邀请接受与撤销HTTP闭环通过

## Session Metadata

2026-10-21 21:24:46 +08:00，Asia/Shanghai。WeaveSense-Flow，分支codex/fix-invite-accept-rls-context，部署源码7a0091954def1b183344c5b30104b3e212fed01a；后续仅补验收文档，最终HEAD用git核对。

## Handoff Chain

Continues from：[归档交接](2026-10-21-211307-invite-rls-archived-handoff.md)。邀请修复已归档，本轮仅按用户明确授权部署18088并真实HTTP复验，不重复开发或归档。

## Current State Summary

18088仅更新platform，健康接口commit7a009195、容器healthy、宿主/容器二进制哈希一致。31项真实登录HTTP检查全部通过：邀请接受200、同/异账号重用400、viewer读200/写与管理403、owner撤销200后原viewer会话立即拒绝读写404，个人scope仍可用。PG确认邀请消费一次、成员removed、workspace-member不存在。0活跃Comet。

## Important Context

用户授权保留既有数据和修改，不关闭RLS、不调用真实模型、不启动子代理、不推送/合并/发布，全程遵守。原浏览器会话不读取或切换，本轮完整链路是HTTP API+PG持久验证，不冒充浏览器点击旅程。没有产品源码修改，未重新跑源码专项。

## Codebase Understanding

已修AcceptInvite用精确token_hash单行查询及锁完成凭证发现，验证后进入精确目标tenant兑换，再恢复原范围。ENABLE/FORCE RLS和策略不变。部署使用旧运行层覆盖当前源码后端，Pi/插件/前端/Engine保持原版本，因此不是全栈纯HEAD或正式标准发布构建。

### Critical Files

- `backend/internal/flow/orgs.go`：已部署的邀请兑换修复。
- `docs/flow/2026-10-21-invite-rls-deployment-acceptance.md`：31项和保留数据证据。
- `docs/content/docs/progress/pending-test.mdx`：本轮通过与剩余边界。
- `docs/superpowers/reports/2026-10-21-fix-invite-accept-rls-context-verify.md`：前轮源码RED/GREEN引用。

## Work Completed

部署前两库pg_dump及旧镜像保存；10张关键表在替换后、HTTP测试前指纹完全一致。仅platform重建，原命名卷保持，web/postgres/Engine/scheduler及8088容器ID不变。平台角色NOSUPERUSER/NOBYPASSRLS，邀请/账务/模型/预算表ENABLE/FORCE RLS保持。没有迁移、卷删除或数据恢复覆盖。

真实两账号独立CookieJar复用已有空组织，新增1条邀请；接受后目录/角色/canManage准确，同/异账号重用拒绝，viewer读/写管理权限正确。撤销后同一会话立即读报告/信源/成员/账务及写信源全部404，目录移除，已用令牌不能重新加入，个人scope可读、owner权限保持。31项状态断言和附加业务字段断言退出0；PG持久交叉核对通过。

原tenant/member/workspace/invite/billing/canvas行保持，用户ID集合不变、登录时间更新；原用户非登录字段未逐字段核对。实采report/document/revision指纹不变。新组织0、新邀请1、新成员1且removed，工作区成员最终无新增；模型任务0、预算预留0。保留检查的首次挂载数组顺序误报经只读检查确认只是Docker返回顺序，按Destination排序后通过，无实际卷变动。

### Decisions Made

采用已有Go镜像禁网CGO静态编译+旧运行层覆盖路径，避免镜像源历史故障；保留glibc静态链接限制，不扩大到未测SQLite扩展兼容。仅验收局部部署，正式交付另行。原未使用邀请保留，本轮新邀请通过API生成并消费，不删除历史证据。

## Pending Work

浏览器邀请点击旅程、完整角色矩阵、同租户多workspace、离线/跨标签缓存、真实MCP/SSE/未知结果恢复、旧库迁移与恢复演练、监控/多实例和依赖具备后的外发仍待；不能将31项称完整U/A或全PRD。正式标准镜像构建未验证，G01 push/merge/release另待明确授权，O01–O06不擅自实现。模型额度0。

## Immediate Next Steps

1. 核对git/HEAD/branch和openspec0活跃；邀请hotfix已归档且已部署，不重做。
2. 沿待验清单继续完整权限/恢复等无模型工作；若重复真实邀请测试，先核对本轮成员removed状态，不能盲跑脚本或覆盖历史数据。
3. 如需重建18088，带docker-compose.flow.yml、compose.acceptance.yml、compose.org-rls.yml和最新compose.invite-rls.yml；保留卷与备份，勿只用旧override降回旧binary。

## Environment State

flow-uat-20261020/18088平台运行commit7a009195，镜像sha256:5e346c0f80fc5ab31700ce9c2669ea96d648fac8ee83e756bc6a73139e5acd98。旧镜像flow-uat-platform:before-invite-rls-20261021保留。CANVAS_AUTO_MIGRATE=false，没有启动dev server或改8088。

Git忽略的.local/flow-uat-20261020/invite-rls-deploy有本轮备份、指纹、health、HTTP逐项状态、持久核对及私有邀请恢复状态；不提交凭据或打印令牌。check-invite-deployed.py会新增邀请/撤销成员，无故不重跑。

## Potential Gotchas

原dirty文档、归档.comet.yaml、Agent面板、历史handoff、prd-audit-9.md、table.csv和本地参考文件保护，不reset/clean/restore、不git add .。pending-test与pending-acceptance含旧增量，本轮提交若需包含清单应精确patch，不整体暂存。无.codegraph不建索引。备份生成不等于恢复演练，HTTP状态断言不等于浏览器验收。

## Related Resources

[本轮证据](2026-10-21-invite-rls-deployment-acceptance.md)、[前轮失败和组织部署](2026-10-21-org-rls-deployment-acceptance.md)、[源码验证](../superpowers/reports/2026-10-21-fix-invite-accept-rls-context-verify.md)。
