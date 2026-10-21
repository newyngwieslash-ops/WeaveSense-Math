# 18088 邀请接受修复部署与完整HTTP链路验收

2026-10-21 21:24 +08:00，Asia/Shanghai。分支codex/fix-invite-accept-rls-context，部署源码7a0091954def1b183344c5b30104b3e212fed01a。用户明确授权本轮隔离部署与真实邀请接受、单次使用、viewer及撤销复验；保留原修改和数据。不关闭RLS、不调用模型、不启动子代理、不推送/合并/发布。

## 部署与保留数据

仅替换flow-uat-20261020-platform-1；带docker-compose.flow.yml、原compose.acceptance.yml和compose.org-rls.yml，追加Git忽略的compose.invite-rls.yml，执行build --pull=false与up -d --no-deps --no-build platform。CANVAS_AUTO_MIGRATE=false，无数据库迁移、down或卷删除。原platformdata命名卷保持，web、postgres、Engine、scheduler与8088平台/前端容器ID均不变。

沿用前轮局部镜像路径：已有golang:1.25.0-bookworm，源码/模块只读、网络禁用、CGO_ENABLED=1静态链接编译，再复用原平台运行层仅覆盖backend binary。没有重建前端、Pi、插件或迁移工具；这是验收局部构建，不称正式全栈纯HEAD镜像。链接器仍提示glibc静态dlopen/用户查询/DNS限制，本轮真实PG/Engine HTTP路径通过，不扩大到未测SQLite扩展兼容性。

健康接口实际返回commit=7a0091954def1b183344c5b30104b3e212fed01a，容器healthy。镜像sha256:5e346c0f80fc5ab31700ce9c2669ea96d648fac8ee83e756bc6a73139e5acd98；宿主与容器backend SHA-256同为c1edc57aa0a5e60879c439d70a25ed5e920953b6890534e1cb7f204f8a806706。

部署前两库pg_dump -Fc保存在.local/flow-uat-20261020/invite-rls-deploy：platform839092字节、intel104887字节；旧镜像flow-uat-platform:before-invite-rls-20261021保留。只验证备份生成及大小，没有将此称为恢复演练。

部署后、验收写入前10张关键平台表指纹全部一致。验收后原tenant/member/workspace/workspace-member/invite/billing/canvas行哈希不变；原用户数量/ID集合不变，两个账号真实登录允许更新时间字段，本轮未逐字段比较用户非登录字段。实采report/document/revision哈希不变。新组织0、新邀请1、新tenant-member1（最终status=removed），workspace-member最终无新增。模型任务0、预算预留0。

平台角色无SUPERUSER/BYPASSRLS，invites/billing/model-task/reservation表ENABLE/FORCE RLS前后一致，无策略修改。原挂载一致；保留检查首次按数组顺序比较时误报，检查发现postgres挂载两项仅返回顺序变化，按Destination排序后通过，数据库容器本身未替换。

## 本轮实际31项真实HTTP检查

使用已有两个普通测试账号，独立CookieJar，通过127.0.0.1:18088真实登录；脚本显式禁用宿主HTTP代理。原浏览器会话未读取或切换。复用前轮API创建的空组织ws_08e1f44b7deb7cb9f8c8e4ad，旧未用邀请保持不动，仅通过真实API生成一条新viewer邀请并兑换。

| 范围 | 实际结果 |
| --- | --- |
| 两账号登录、owner查询成员 | 200 |
| 接受前另一账号读目标报告列表、workspace目录 | 目标报告404，目录无目标workspace |
| 匿名、无效邀请令 | 分别401、400 |
| owner生成viewer邀请、受邀者接受 | 均200，code=0，返回tenant/workspace/role=viewer准确 |
| 接受后workspace目录与成员投影 | 目录出现目标；成员role=viewer，canManage=false |
| viewer读取报告/信源列表 | 均200 |
| 同账号与另一账号重用该令牌 | 均400 |
| viewer新增信源、生成邀请、修改/删除owner | 均403，无成功写入 |
| viewer访问平台admin、原用户报告 | 分别403、404 |
| owner真实DELETE撤销本次成员 | 200 |
| 原viewer会话立即再读报告/信源/成员/账务 | 全部404，无重新登录或等待刷新权限 |
| 撤销后写信源、目录、再次兑换已用令牌 | 写入404，目录不含目标，兑换400 |
| 撤销后个人workspace、owner权限 | viewer个人报告200；owner仍可管理，列表无已撤销viewer且owner角色保留 |

check-invite-deployed.py退出0，共31个HTTP状态断言通过，另有业务code/role/目录字段断言。只读PG查询交叉确认：新invite accepted_by匹配受邀者、accepted_at非空；成员status=removed，workspace-member不存在。前轮接受400保留历史证据，本轮当前镜像真实200闭合该路径。

## 证据与限制

最小本地检查和结果位于Git忽略的.local/flow-uat-20261020：check-invite-deployed.py、invite-deploy-snapshot.py、check-invite-preservation.py及invite-rls-deploy/http-checks.json、http-result.json、preservation-result.json、health.json。新邀请令的恢复状态只保存在invitation.private.json，不提交或输出。验收脚本会新增邀请并撤销成员，不应无故重跑。

本轮未修改产品代码、重跑Go专项、做浏览器点击验收或重新核对导出文件内容；源码普通PG RED/GREEN见[修复报告](../superpowers/reports/2026-10-21-fix-invite-accept-rls-context-verify.md)，前轮五格式导出和浏览器缓存结果仅引用旧证据。当前完整邀请链路指真实登录HTTP API及PG持久结果，不称完整浏览器用户旅程。

完整角色矩阵、同租户多workspace、离线/跨标签缓存、真实MCP/SSE/未知结果恢复、旧库迁移和恢复演练、监控/多实例及依赖具备后的外发仍未全部通过。正式标准镜像构建与G01交付另待完成/授权；O01–O06不擅自实现。没有模型或外发额度被消耗。
