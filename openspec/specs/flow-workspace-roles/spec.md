# flow-workspace-roles Specification

## Purpose
TBD - created by archiving change workspace-analyst-role. Update Purpose after archive.

## Requirements

### Requirement: Workspace analyst role and scope mapping (PRD §10.1)
平台 SHALL 在组织邀请、成员角色修改、成员投影及scope校验中支持analyst，与owner/admin/editor/viewer并存。analyst SHALL 可依既有报告写scope新建及编辑本工作区专题，HTTP与工具共享同一权限合同；analyst MUST NOT 获得工作区成员管理权限，跨工作区对象访问 MUST 继续拒绝。

#### Scenario: Invite and assign analyst role
- **WHEN** 管理员邀请analyst或将现有成员设为analyst
- **THEN** 成员列表及有效scope返回analyst，并授予analysis/collection/monitor/schedule/report/MCP现有执行scope，拒绝邀请、改角色与撤销成员管理操作

#### Scenario: analyst专题新建编辑与隔离
- **WHEN** analyst通过HTTP或工具新建/编辑当前工作区专题
- **THEN** 写入成功且角色保持analyst；传入其他工作区对象ID时仍拒绝，不改写其他对象

#### Scenario: viewer和未知角色仍拒绝专题写入
- **WHEN** viewer或未知角色尝试新建/编辑专题
- **THEN** 系统拒绝写入，不因analyst修复获得额外权限
