# Tasks

## 1. 成员撤销共享安全边界

- [x] 1.1 先补最小RED确认RemoveMember对owner/非管理者/其他租户和最后管理员的真实行为，再修共享事务入口；普通PG新测试库验证并发保护、故障注入全回滚、成功后多workspace立即拒绝及个人scope保持，记录反证/结果并同步接口安全说明，不在18088原成员上注入失败。 <!-- comet-task:member-boundary -->

  实际：SQLite RED复现owner失权、最后管理员被撤销及重复/跨租户假成功；GREEN含计数错误传播、双工作区及个人scope。普通PG专项通过删除触发器回滚与两连接租户锁竞争，相关组织/邀请/RLS三项通过，临时角色已删除。多工作区撤销检查为SQLite，PG并发/回滚检查独立记录。

## 2. 前端成员入口与analyst回显

- [x] 2.1 先在既有Bun组件/API检查中复现撤销入口与analyst选项/标签/回显缺失，再复用http DELETE和AppModal接入；覆盖管理者与owner保护、取消不发请求、确认防重复/成功刷新/失败不假成功及analyst邀请/角色编辑，更新功能与pending-test，不新建仿制页面或依赖。 <!-- comet-task:member-ui -->

  实际：组件两项RED后GREEN，Bun 19通过/198断言，API编码/header/业务错误、tsc/Vite及lint通过。18088真实UI验证analyst标签/无撤销入口、专题新建/编辑、成员viewer→analyst回显、analyst邀请生成（PG角色核对）、取消无DELETE、并发目标已撤销时保留错误、双击仅一次DELETE及成功成员刷新。原成员HTTP会话立即404、目录移除、个人scope200、重放拒绝。账号切换经用户明确授权；审批误判测试目标后提供新租户证明再重试，未绕过。

## 3. 专题角色合同

- [x] 3.1 先补analyst专题Create/Update及真实HTTP最小RED，再复用既有报告写scope修共享门禁；验证HTTP与本地工具共同入口、viewer/未知角色拒绝、analyst仍不能管理成员、跨workspace对象404，并同步角色文档。 <!-- comet-task:topic-scope -->

  实际：服务Create与Gin POST先RED（403）；共享门禁一行修复后Create/Update、工具Create、HTTP POST/PATCH、409、同租户跨workspace GET/PATCH 404、viewer/未知角色拒写与analyst成员管理拒绝通过。

## 4. 聚焦集成与交付证据

- [x] 4.1 运行相关Go/PG与Bun专项及前后端最小充分构建，核对两份delta规格；在真实页面验证入口/弹窗/analyst回显/撤销失权（若未部署明确待验），记录本轮实际/引用/未执行/阻塞，生成验证报告及带时间戳交接，保留既有修改和数据，无模型、子代理、推送/合并/发布。 <!-- comet-task:integration-evidence -->

  自动/真实证据已落盘；review_mode=off源于用户禁止子代理，不派发自动审查，主会话检查权限/事务/调用方。18088保卷更新、105项真实HTTP与浏览器专项通过；原非用户行、实采文档报告、RLS保持，用户行仅登录活动变化，模型任务/预算零新增。全局U/A/G门槛未据此关闭。
