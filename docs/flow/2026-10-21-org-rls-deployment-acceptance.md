# 18088 组织创建修复部署与权限复验

时间：2026-10-21 20:28 +08:00，Asia/Shanghai。分支 `codex/fix-organization-create-rls-context`，源码 HEAD `d5f17bfa`。用户授权仅更新隔离环境、保留数据并复验；本轮未改产品源码、关闭 RLS、调用模型、启动子代理、推送、合并或发布。

## 部署与数据保留

仅重建并替换 `flow-uat-20261020-platform-1`，使用 `up -d --no-deps --no-build platform`，保留 `flow-uat-20261020_platformdata`。web、postgres、Engine、scheduler 以及 8088 的 platform/web 容器 ID 均未变化，原挂载一致。未执行 down、卷删除或恢复覆盖。

常规 Dockerfile 构建因 Docker Hub 镜像元数据 size validation/镜像站 EOF 失败。替代路径使用本机 `golang:1.25.0-bookworm`，源码及现有 Go 模块只读挂载、`--network none`、GOPROXY/GOSUMDB off，CGO_ENABLED=1 外部静态链接编译；再以保存的旧 platform 运行镜像为基础仅覆盖后端二进制。依赖、Pi、插件和迁移工具沿用旧运行层，因此这是验收专用局部镜像，不称全栈纯 HEAD 或正式发布构建。链接器提示 glibc 的 dlopen/用户查询/DNS 静态链接限制；本轮 PostgreSQL/Engine HTTP 路径实际通过，未据此宣称 SQLite 扩展等未测路径兼容。

本地 Compose 附加文件 `.local/flow-uat-20261020/compose.org-rls.yml` 设置 `CANVAS_AUTO_MIGRATE=false`，启动只核对既有 schema 版本。健康接口 build.commit 实际返回完整 `d5f17bfa8b28c4f8f2e7a2f26aac0ccaa8e45db0`，容器 healthy。镜像 `sha256:60cca3b8946870ebe823cb83d3b2e8cbb1f653d9d6c73e1cc82dd839b9d250b3`；宿主与容器二进制 SHA-256 同为 `fea00f5c1e93468bfc076508148b4baeac7dfd6457c15c2160d71b068a0a053f`。

首次用 --version 探测时程序实际启动；该临时容器无网络、无挂载，仅写自身临时层，已停止并自动移除，未触及原数据。后续版本确认使用真实健康接口。

部署前两库 pg_dump -Fc 已保存在 Git 忽略的 `.local/flow-uat-20261020/org-rls-deploy`：platform 841700 字节、intel 104887 字节；旧镜像保留为 `flow-uat-platform:before-org-rls-20261021`。备份仅完成生成与大小检查，本轮未做恢复演练。

替换后、HTTP 验收前，users、tenant/member/workspace、workspace-member、billing、model-task、reservation、canvas 共9表数据指纹完全一致。验收后原租户/成员/工作区/账务/画布行均无删除或内容改变；两个测试用户登录时间更新，用户数仍3。新增两个专用组织及各自默认工作区、owner成员、共享账务，无覆盖旧数据。Engine实采 reports/documents/revisions 指纹前后一致。模型任务及预算预留均为0。

平台角色仍 NOSUPERUSER/NOBYPASSRLS；billing、model-task、reservation 表 ENABLE/FORCE RLS 前后一致，没有执行策略变更。

## 本轮实际 HTTP 验证

使用本地已有两个普通测试账号、独立 CookieJar，通过 localhost:18088 真实登录；密码和 Cookie 不输出。原用户会话未读取或修改。

| 范围 | 结果 |
| --- | --- |
| 两账号真实登录 | 200，普通用户管理端访问403 |
| 匿名创建组织、空白名称 | 分别401、400 |
| 两账号分别 POST /api/intelligence/orgs | 均200、业务code=0；原500已在部署后消除 |
| 创建后独立请求读取workspace/members/billing | 均200；owner及canManage正确，共享账务tenant/payer与创建账号一致，reservations为空 |
| 双向读取另一账号组织成员/账务/报告 | 均404 |
| 双向修改或删除另一组织owner、写入信源 | 均404 |
| 两账号使用自己或伪造原用户workspace导出原实采报告五格式 | 均404，无报告正文泄漏 |
| 匿名五格式导出 | 全部401 |

上述51项 HTTP 状态断言及附加业务字段断言通过。随后 owner 创建 viewer 邀请200，为第52项通过；第53项真实接受邀请预期200、实际400，脚本退出1。不能把完整脚本称通过。没有通过域服务或SQL补建受邀成员来掩盖失败。

## 剩余失败：邀请接受被原租户范围隐藏

真实 POST /api/intelligence/invites/accept 返回400。只读回滚事务核对：新邀请存在、未过期、未使用、未撤销；SET LOCAL ROLE flow_platform 并设置受邀者现有 active tenant 范围后，新邀请可见数为0。源码 `AcceptInvite` 直接通过 token_hash 在当前请求事务中查询 `flow_invites`；该表的 FORCE RLS 仅允许当前租户范围，受邀者尚未加入目标组织，因此查询不可见。日志未提供42501，此为读过滤证据，不能写成插入报错。

本轮未改 AcceptInvite、通用中间件或策略。邀请单次使用、viewer真实加入后拒写、角色切换、撤销后即时拒读因依赖接受成功而未执行。两专用空组织和未使用邀请保留供后续调查，不删除未知编辑或清缓存。

## 证据和门槛

本地最小可运行检查：`.local/flow-uat-20261020/check-org-deployed.py`（会新增验收组织，不应无故重跑）、`org-deploy-snapshot.py`、`check-deploy-preservation.py`、`check-invite-scope.py`。结果文件在同目录 `org-rls-deploy/`，含 HTTP 逐项状态、组织ID、数据指纹、邀请范围反证及备份；不提交凭据、本地数据库或镜像构建产物。

本轮没有重新做浏览器账号切换，前轮浏览器缓存与五格式文件内容证据仍只引用[前轮报告](2026-10-21-report-export-account-isolation-evidence.md)。无全量Go/前端构建、恢复演练或生产迁移。完整权限矩阵、同租户多workspace、离线/跨标签竞态、真实MCP/SSE/未知结果恢复、旧库迁移、监控/多实例和外发仍未收口；G01交付未授权。
