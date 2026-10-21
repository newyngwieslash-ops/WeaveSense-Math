# 成员撤销与 analyst 权限验证报告

时间：2026-10-21 23:37 +08:00，Asia/Shanghai。变更：`fix-workspace-member-controls-and-analyst-permissions`；分支同名codex前缀；基线 `2b47d1438f6574f8c6c717cb182c0b83e9460d17`。实现提交：`90ffb431`、`40dd9075`、`205a4ae3`。本报告只覆盖本聚焦变更，不代表完整PRD或U/A/G全部通过。

## 结论与范围

产品实现、最小RED→GREEN、相关构建、普通PG并发/回滚和18088真实HTTP/浏览器专项通过。四项任务完成；Comet阶段guard另以实际运行输出为准，不手写verify_result=pass。没有新API、数据库字段、RLS策略或依赖变化。未调用真实模型、外发、生产迁移、子代理、推送、合并或发布。

收尾：Comet full实际build/verify检查均退出0，build guard14/14通过，verify guard已将状态推进archive、verify_result=pass。归档仍未执行，branch_status=pending。首次guard发现计划步骤缺任务ID，已为四项任务补稳定映射；旧技能record-check声明不被新版guard认可，改用已安装公共CLI的实际check run，未修改工具或绕过检查。

## 实际RED与根因

| 边界 | 旧实现失败 | 共享修复 |
|---|---|---|
| RemoveMember | owner撤销返回nil却删除workspace关系；最后管理员可撤销；重复/跨租户未命中返回nil | 同租户行锁后查询active目标、拒绝owner、计数保护最后管理员；状态与workspace删除同事务 |
| 前端成员 | analyst规范化返回editor；管理者真实组件没有撤销命令 | 补齐analyst类型/标签/邀请与编辑选项；既有http DELETE、AppModal确认、取消、错误及防重复 |
| 专题 | analyst Create返回ErrForbidden，Gin POST返回403 | canWriteTopics一行复用既有intel.reports.write；HTTP及本地工具共同受益 |

调用方已追踪：RemoveMember由P5 DELETE调用；SetMemberRole由P7 PATCH调用，两者共同锁tenant，避免不同目标锁无法保护管理员总数。CreateTopic/UpdateTopic由专题POST/PATCH与intel_save_topic调用，共享门禁修复。前端保持既有request.ts/http信封与错误处理，不绕开租户header。

## 本轮运行命令与结果

- `cd backend; go test ./internal/flow -run 'TestMemberControls|TestOrgInvite|TestP7Members|TestTopicAnalyst|TestSavedTopics' -count=1 -v`：9项顶层测试通过；host未配PG DSN的1项明确SKIP，不计PG通过。SQLite检查含owner、最后管理员/允许自撤销、重复/跨租户拒绝、计数错误传播、双workspace撤销、个人scope及触发器失败回滚。
- Linux测试二进制在18088的PG17容器用临时NOSUPERUSER/NOBYPASSRLS/CREATEDB角色、新数据库执行 `TestMemberControlsPostgres`：通过。两个独立事务均实际等待FOR UPDATE租户锁；释放后撤销/降级仅一次成功，另一次ErrForbidden，仍有一位有效管理员。删除触发器异常原子回滚；owner关系保持。触发器仅新测试库，测试库清理、临时角色已删除。
- 同一普通PG环境执行 `TestCreateOrganizationRLSPostgres|TestAcceptInviteRLSPostgres|TestRuntimeRLSBlocksUnfilteredHandlerQueries`：3项顶层测试通过，含真实HTTP、范围恢复、失败回滚与邀请并发单次兑换。
- `cd web; bun test test/intelligence-member-api.test.ts test/intelligence-panels.test.tsx test/intelligence-gaps9.test.ts`：19通过、0失败、198断言。验证DELETE编码/header及业务错误抛出；SSR仅证明渲染，交互另见下方真实UI。
- `cd web; bun run build`：tsc与Vite通过；`bun run lint`退出0；`cd backend; go build ./internal/flow`退出0。Bun测试保留既有AntD List/Space弃用提示，Vite保留大chunk提示，无新增依赖。
- `openspec validate fix-workspace-member-controls-and-analyst-permissions --strict`、`git diff --check`通过；CRLF提示不当作产品错误。

## 18088部署与数据保护

仅替换隔离Compose `flow-uat-20261020` 的platform/web，`CANVAS_AUTO_MIGRATE=false`，复用原卷。双库dump、回退镜像与配置仅保存在Git忽略的 `.local/flow-uat-20261020/member-controls-deploy`。8088、PG、Engine和调度器容器ID不变。部署后、添加验收数据前，10类表与备份完全一致。

后端用本地缓存的Go1.25 CGO静态编译并覆盖既有运行镜像中的二进制；glibc静态dlopen/用户/DNS链接提示保留，不据此宣称标准生产镜像已验证。曾生成的CGO关闭编译候选未用于部署。前端使用已构建dist覆盖既有运行镜像，包含用户原有未提交Agent面板改动，标记为 `205a4ae3-with-preserved-local-panel`，不称纯HEAD镜像。未改Engine代码。

最终快照逐旧行比较：除两个测试用户的登录活动字段外，所有原记录哈希保持；原实采documents/revisions/reports哈希保持；模型任务及预算预留仍0；抽查RLS enabled/forced与应用角色super/bypass标志保持。新增专用组织、成员、邀请、离线example.invalid信源及专题为本轮验收记录，无采集/外发。

## 真实HTTP与浏览器证据

105项部署后HTTP检查通过：owner/admin/editor/analyst/viewer核心读写矩阵、analyst专题201、viewer写403、非管理成员管理403、owner撤销403；同租户双workspace未加入拒绝与对象跨scope GET/PATCH 404。来源只创建无效域名测试记录，不采集、不执行MCP。

真实内置浏览器127.0.0.1测试标签页执行：

1. analyst显示中文标签且无撤销命令；专题新建、编辑备注成功，HTTP复读确认持久化。
2. owner成员无撤销入口；成员编辑viewer→analyst准确回显；邀请下拉有analyst，实际生成邀请，PG按非敏感邮箱核对role=analyst/1条，未输出令牌或发送邮件。
3. 撤销弹窗说明该组织全部工作区影响、owner/最后管理员边界；取消关闭且成员保持。后端DELETE计数仍仅前期owner拒绝探针1次，取消无请求。
4. 弹窗打开后，另一owner HTTP请求撤销同一新测试目标，再确认：真实403/flow:not found，弹窗保留错误，不显示成功。未在运行数据库注入故障；数据库故障另由普通PG新库覆盖。
5. 新专用组织中两次真实双击成功：成员立即由2变1、弹窗关闭，每次仅一个DELETE。后端总6次DELETE与owner拒绝/并发撤销/过期目标失败/两次成功/重放拒绝逐项对上。
6. 成功撤销后保留的原成员HTTP Cookie会话：topics/sources/members立即404，工作区目录移除，个人topics仍200，重复DELETE403。不是重新登录后推断。

截图：`analyst-no-manage.png`、`revoke-confirm.png`、`revoke-failure.png`、`revoke-success.png`，只在上述Git忽略目录。首张截图视口未完整包含角色行，角色依据实际AX及HTTP；其余弹窗和成功成员列表为真实页面。

测试脚本首版空CookieJar被真值判断替换，导致首次后验401，明确作为测试工具错误而非权限结果；改为None判断并断言Cookie非空，再建新测试组织完整重验通过。首次UI邀请toast观察错过，最终以PG落库角色核对，不把false观察记为产品失败。

浏览器退出测试账号曾被自动审批拒绝；用户明确授权两测试账号切换后才执行。并发撤销脚本曾被误判目标为历史组织；只读证明目标不在部署前备份且与旧matrix不同后，同一操作重试获准，未绕过拒绝。

## 规格与设计核对

| 维度 | 结果 |
|---|---|
| 完整性 | 4/4任务、2项修改需求、11个场景均有实现和可追溯证据 |
| 正确性 | 共享入口、权限、并发、回滚及真实UI行为符合本次规格，无CRITICAL/IMPORTANT发现 |
| 一致性 | 符合高层及技术设计，无规格漂移；无新增抽象、依赖、API或表结构 |

两份delta共11个场景均能映射：旧viewer/editor投影由P7回归；analyst邀请/编辑由真实UI+PG+HTTP；撤销取消/失败由真实UI+请求计数；撤销成功/即时失权由UI+保留原HTTP会话；owner/最后管理员/自撤销由SQLite+普通PG；非管理角色由UI/HTTP；analyst专题及隔离由服务/工具/HTTP/浏览器；viewer/未知由专项。设计中租户行锁顺序、原事务回滚、共用scope、http和AppModal均落地，无delta与技术设计分歧。

review_mode=off：用户禁止子代理，未派发自动审查；主会话核对全部调用方、租户过滤、并发锁、错误传播及变更范围。数据库结构文档不改，因为无结构变化。

## 未执行、引用及剩余门槛

本轮未运行全仓go test ./...、全前端测试、暗色/窄屏专项、独立多浏览器并发缓存或重启恢复；原成员“同一会话立即失权”为保留HTTP会话，未称两个浏览器同时在线。18088的成功/失败/取消交互为真实UI。此前浏览器viewer及历史106项矩阵仅作背景，本报告105项为本轮重跑。

完整U01–U05、A02–A15、真实模型/MCP/外发、跨标签恢复和G01远端交付仍按交接推进，不能以本聚焦修复关闭。没有新增模型授权或真实外发授权；本地归档仍需本次最终确认。
