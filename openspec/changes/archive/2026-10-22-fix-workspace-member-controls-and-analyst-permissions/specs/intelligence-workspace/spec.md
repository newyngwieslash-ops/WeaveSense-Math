## MODIFIED Requirements

### Requirement: 工作区成员角色准确投影与编辑一致
工作区成员查询接口（`GET /api/intelligence/members`）MUST 返回成员在当前工作区的有效角色（租户 `owner` 保持 `owner`，其余成员优先取当前工作区角色 `admin`、`editor`、`analyst`、`viewer`）；角色修改接口 MUST 接受以上可编辑角色（兼容 `member` 为 `editor`），前端邀请、角色选项及中文标签 MUST 一致且刷新后保持准确回显。可管理成员的用户 MUST 能通过真实成员页面对非owner成员发起经明确确认的撤销；确认界面 MUST 说明会撤销该组织所有工作区访问，取消不发送请求，失败不伪装成功。服务端 MUST 独立保护owner及最后有效管理员，不因删除工作区成员而使owner失去访问；撤销持久化失败 MUST 原子回滚。

#### Scenario: 将成员设为 viewer 后刷新仍显示 viewer
- **WHEN** 工作区管理员将某成员角色修改为 `viewer` 并重新查询成员
- **THEN** 该成员返回role为viewer，且丧失写权限

#### Scenario: 支持 editor 与 member 角色设置并回显为 editor
- **WHEN** 管理员将成员设为editor或兼容的member
- **THEN** 刷新后回显editor，与邀请角色一致

#### Scenario: analyst邀请与成员编辑保持回显
- **WHEN** 管理员在真实页面邀请analyst或将成员设为analyst，再刷新成员列表
- **THEN** 角色选项和中文标签保持analyst，不回退成editor

#### Scenario: 撤销确认取消与失败
- **WHEN** 管理员发起撤销非owner成员后取消，或确认后的持久化请求失败
- **THEN** 取消不发出撤销请求，失败明确反馈且不显示成功、不假装成员已移除

#### Scenario: 撤销成功后当前会话立即失权
- **WHEN** 管理员确认撤销，接口成功提交
- **THEN** 成员及目录刷新，目标成员对该组织各工作区的后续访问立即被拒绝，个人工作区不受影响

#### Scenario: owner及最后管理员保护
- **WHEN** 有人直接调用撤销接口指向owner，或并发撤销将移除最后有效管理员
- **THEN** 服务端拒绝破坏性操作，owner成员及工作区访问保持，不依赖前端隐藏命令

#### Scenario: 非管理角色不能撤销
- **WHEN** editor、analyst或viewer访问成员页面或直接调用撤销接口
- **THEN** 页面不提供撤销命令，服务端拒绝请求，成员关系保持

#### Scenario: 管理员自撤销边界
- **WHEN** 管理员确认撤销自己
- **THEN** 仅当该组织仍有其他active owner/admin时允许，随后其组织访问失效；owner及最后有效管理员的撤销请求始终被拒绝
