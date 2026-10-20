# Platform ⇄ Loom Engine Service Contracts (P0 draft)

Status: contract baseline. Implementation starts in P1; changes require a version bump of the relevant section.

## 1. Execution context (signed, short-lived)
Every platform → Engine call carries `Authorization: Bearer <JWT>` signed by the platform (`FLOW_INTERNAL_JWT_*`). The request body never authorizes on its own.

| Claim | Meaning |
|---|---|
| `iss` / `aud` | `flow-platform` / `loom-engine` |
| `exp` / `iat` / `jti` | ≤ 300 s TTL; `jti` for replay detection |
| `tenant_id`, `workspace_id` | scope; every object lookup is checked against it |
| `actor` | `{type: user\|service_principal\|schedule, id}` |
| `operation_id`, `job_id?` | platform operation, optional Engine job |
| `scopes` | e.g. `intel.sources.read`, `intel.monitors.write`, `intel.collect`, `intel.research` |
| `budget_reservation_id` | required for any cost-incurring command |
| `policy_version` | authorization policy snapshot used |

Scheduled executions re-check the durable grant (membership, connection, budget) at each window; revoked grants produce `schedule.blocked` instead of running.

## 2. Idempotency
- External write APIs: header `X-Idempotency-Key`; existing Agent body `idempotencyKey` stays compatible.
- Same key + same params → replay original result. Same key + different params → **409**, original result untouched.
- Scheduled collection key: `tenant_id + schedule_id + scheduled_at` (UTC window start). Unique constraint in intel DB.
- Config updates carry `expected_revision`.

## 3. Internal command (platform → Engine)
```json
{
  "operation_id": "op_example",
  "idempotency_key": "tenant_schedule_window",
  "workspace_id": "workspace_example",
  "command": "collection.request",
  "source_ids": ["source_example"],
  "schedule_id": "schedule_example",
  "scheduled_at": "2026-10-13T01:00:00Z",
  "budget_reservation_id": "reservation_example",
  "policy_version": "policy_v1"
}
```
Command acceptance and Job creation happen in one intel-DB transaction; response `202 {status:"accepted", job_id}`.

## 4. Outbox / Inbox
- Manual: platform tx creates operation (`flow_operation_projections`) + reservation (`flow_budget_reservations`, optional) + outbox row (`flow_command_outbox`) → `CommandDispatcher` (`DispatchOne` / `DispatchDue` / `RunCommandDispatcher`, plus optional `X-Flow-Outbox-Mode: async` 202 queue mode) → Engine inbox (dedupe on `idempotency_key`) → Job.
- 手动命令仅由条件领取实际命中的派发者发送；`attempts` 是本次领取版本，所有回执、投影和预算变更在同一事务内先验证该版本。领取失败、活跃租约和未到退避时间均不发送，旧响应不能改写新回执或资金状态。
- 如果调用方已处于租户事务（REST `TenantMiddleware` 或 Agent `UserTx`），GORM 内层事务只建立 savepoint；派发入口因此只返回已入队状态，由后台在外层提交后发送。REST 返回 202 和实际 `status`；Agent 工具成功表示命令受理，输出中的 pending/dispatching/reconciliation_required 不代表业务完成。
- 用户没有提供幂等键时，平台仍向 Engine 转发持久化的自动键，所有重试复用同一键。Engine 幂等保证仍是网络/5xx 有界重试的必要条件。
- 成功回执、4xx 终态、预算释放/未知标记、投影删除/创建/更新和最终读取失败必须传播，REST/Agent 不按已拿到的上游状态码清除错误。事务失败回滚到 dispatching 并保留资金，活跃租约期间不重发；租约过期或结果不确定进入 reconciliation_required，只对账。数据库仍不可写时不能声称已成功进入对账状态。
- `DispatchDue` 聚合返回派发/数据库错误；后台循环记录不含上游正文或敏感 URL 的失败提示。强制数据库连接中断时的提交不确定、真实 PG17 多实例及非超级用户 RLS 仍需部署验收；SQLite 故障注入不能替代这些证据。
- Scheduled: Engine claims windows (lease + fencing token) and requests a platform grant; no platform-side scanner.
- Completion: business result and outbox event committed in the same tx. Platform consumes, dedupes on `consumer_id + event_id`, updates projections, settles, notifies.

Event envelope:
```json
{"event_id":"evt_…","operation_id":"op_…","job_id":"job_…","tenant_id":"…","workspace_id":"…",
 "sequence":42,"type":"collection.completed","occurred_at":"2026-10-13T01:02:03Z",
 "payload_version":1,"trace_id":"…","payload":{}}
```
Types: `collection.completed`, `document.revised`, `research.completed`, `operation.failed`, `usage.recorded`, `schedule.blocked`.
External actions are never assumed exactly-once: replayable commands, idempotent receipt, stable external IDs, call receipts; unknown state → query/reconcile, never blind resend.

## 5. Usage & billing
Engine records cost/tokens/receipts but holds no spendable balance. Usage kinds: `llm_input_tokens`, `llm_output_tokens`, `ocr_pages`, `asr_seconds`, `paid_source_calls`, `mcp_calls`, `stored_bytes`, `notifications` — integer minimum units with frozen `price_version`. Reserve before, settle by receipt, release if provably not sent, `pending_reconciliation` if unknown. Child-job budgets are carved atomically from the parent reservation. Eino/OpenCode call the platform Model Gateway with parent operation, child job, reservation, `price_version`, stable `call_id`.

## 6. Public API (platform, `{code,data,msg,reason}` envelope)
| Method & path | Behavior |
|---|---|
| GET /api/intelligence/workspaces | accessible intel workspaces |
| GET, POST /api/intelligence/sources | list / create |
| PATCH /api/intelligence/sources/{id} | update with revision |
| POST /api/intelligence/monitors/preview | monitor + cost bounds |
| POST /api/intelligence/monitors | save authorized monitor |
| POST /api/intelligence/collection-runs | collect, 202 |
| GET /api/intelligence/documents | filtered search, paged |
| GET /api/intelligence/documents/{id}/revisions/{rev} | revision + evidence |
| POST /api/intelligence/research-runs | research, 202 |
| GET /api/intelligence/operations/{id} | proxies Engine authoritative state |
| POST /api/intelligence/operations/{id}/cancel | request cancel |
| GET, POST /api/intelligence/schedules | durable schedules |
| GET, POST /api/agent/conversations | server-side conversation directory |
| GET /api/agent/conversations/{id}/messages | paged messages |
| GET, POST /api/mcp/connections | managed connections |

Task lists read projections with `source_version`, `as_of`, `sync_status`; details/control go to the Engine.

## 7. Agent tools (`intel_*`)
All tools receive the execution context implicitly; `workspace_id` is taken from context, not arguments.

| Tool | Purpose | Mode | Input schema (JSON Schema, abbreviated) |
|---|---|---|---|
| intel_list_sources | sources & health | read | `{status?: "active"\|"failing"\|"paused", cursor?: string, limit?: int≤100}` |
| intel_search_documents | material retrieval | read, paged | `{query: string, source_ids?: string[], from?: datetime, to?: datetime, cursor?: string, limit?: int≤50}` |
| intel_read_document | read by chunk/offset | read; returns coverage | `{document_id: string, revision?: int, offset?: int, max_chars?: int≤20000}` |
| intel_compare_evidence | structured evidence comparison | read / budgeted | `{claims: string[], document_ids: string[], max_cost_units?: int}` |
| intel_preview_monitor | preview monitor & cost bounds | no write | `{entities?: string[], keywords: string[], source_ids?: string[], interval: string, timezone: string}` |
| intel_apply_monitor | save approved monitor | write; approval + idempotent | `{preview_id: string, approval_id: string, idempotency_key: string}` |
| intel_collect_once | request one collection | write; returns operation & job | `{source_ids: string[], idempotency_key: string}` |
| intel_start_research | create research job | write; reservation, async | `{question: string, document_ids?: string[], budget_limit_units: int, idempotency_key: string}` |
| intel_get_operation | query operation | read; no model call | `{operation_id: string}` |
| intel_cancel_operation | cancel | write; ownership & state check | `{operation_id: string, reason?: string}` |
| intel_save_report | report + evidence map | idempotent write | `{title: string, body_md: string, citations: [{claim_id, document_id, revision, span}], idempotency_key: string}` |
| intel_preview_schedule | preview schedule | no write; tz, next run, budget | `{cron: string, timezone: string, template_id: string, monitor_ids: string[]}` |
| intel_apply_schedule | save schedule | write; version + durable grant | `{preview_id: string, approval_id: string, expected_revision?: int, idempotency_key: string}` |

Long-running tools return 202 + `operation_id`; P2 shows a task card. Auto-continuation requires a `waiting_external` run state (job_id, tool-call id, reservation, Pi checkpoint) resumed exactly once via Inbox (acceptance A21).
