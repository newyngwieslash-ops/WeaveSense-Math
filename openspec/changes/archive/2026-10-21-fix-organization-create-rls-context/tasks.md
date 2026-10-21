# Tasks（zh-CN）

## 1. 最小复现与共享入口修复

- [x] 1.1 复用pgTestDB增加一个最小回归文件，确认测试角色NOSUPERUSER/NOBYPASSRLS和ENABLE/FORCE RLS，在真实TenantMiddleware+POST /orgs下先记录旧实现500/42501的RED与失败回滚结果；不调用模型，不用超级用户掩盖RLS。
- [x] 1.2 在CreateOrganization原事务内限定新组织租户初始化并恢复旧范围，检查所有调用方；重跑GREEN及原范围恢复/其他租户读写拒绝/注入失败全回滚/外层事务可继续检查，同步功能及pending-test文档，gofmt仅本次Go文件。

## 2. 相关回归与验收交接

- [x] 2.1 运行组织/角色/预算相关专项、非超级用户PG路由集成与Flow构建，清理本轮测试库/角色；核对租户边界无退化，完成验证报告、Comet检查和新时间戳handoff，明确执行/引用/未执行/阻塞项。仅精确本地提交本hotfix内容，归档待最终确认；不推送/合并/发布，不将源码验证称18088已部署通过。

## 执行约束

用户已明确选择codex/fix-organization-create-rls-context并保留已有修改。采用hotfix direct默认执行，先最小RED再修复，不关闭RLS/清空范围、不调用真实模型、不启动子代理。待本次具体工件最终确认后推进open guard与实施；不沿用旧变更已归档设置。
