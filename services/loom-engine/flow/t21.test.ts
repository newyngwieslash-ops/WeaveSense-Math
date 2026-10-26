import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { PGlite } from "@electric-sql/pglite";
import { migrate, pgliteDb, type Db } from "./db.ts";
import * as engineModule from "./engine.ts";
import { Engine } from "./engine.ts";
import { handler } from "./server.ts";
import { sign } from "./auth.ts";
import { createServer } from "node:http";
import { createHmac } from "node:crypto";

const A = { tenant_id: "tn_a", workspace_id: "ws_a" };
const actor = "user:alice";
const config = () => ({ nodes: [
  { id: "input", type: "data-source", metadata: { dataSource: { format: "json", rawText: '[{"n":1}]' } } },
  { id: "logic", type: "logic", metadata: { logic: { operator: "compute", expression: "n*2" } } },
  { id: "output", type: "markdown", metadata: {} },
], connections: [{ fromNodeId: "input", toNodeId: "logic" }, { fromNodeId: "logic", toNodeId: "output" }] });
const hash = (c: any) => (engineModule as any).canvasConfigHash(c);
const reason = (r: string) => (e: any) => e.reason === r;
async function fixture(db: Db) {
  await migrate(db);
  const engine = new Engine(db, async () => { throw new Error("external collection forbidden"); });
  const source = await engine.createSource(A, { url: "https://example.invalid/feed" });
  const source2 = await engine.createSource(A, { url: "https://example.invalid/other" });
  const preview = await engine.previewMonitor(A, { keywords: ["battery"], source_ids: [source.id], interval_seconds: 3600, budget_micro_per_window: 100 });
  const { monitor } = await engine.applyMonitor(A, actor, "policy_v1", { preview_id: preview.preview_id, approval_id: "collection_approval", idempotency_key: "create" });
  const grant = (approval_id = "canvas_approval_1") => ({ approval_id, actor, policy_version: "policy_v1", config_version: 1,
    purpose: "monitor_canvas_recurring", execution_id: "agent_run_1", monitor_id: monitor.id, interval_seconds: 3600, timezone: "UTC",
    total_budget_micro: 1000, max_runs: 10, expires_at: "2099-01-01T00:00:00Z", revoked_at: null,
    config_hash: hash(config()), budget_micro_per_window: 100,
    scope: { ...A, canvas_id: "canvas_a", source_ids: [source.id], tool_ids: ["intel_run_canvas_workflow"] } });
  const patch = (input: any, s = A, who = actor) => engine.updateMonitor(s, who, monitor.id, input);
  return { engine, monitor, source, source2, grant, patch };
}
async function memory(t: any) { const db = await pgliteDb(); t.after(() => db.close()); return db; }

test("T21 binding, grant, pause/resume and unlink survive a real database close/reopen", async (t) => {
  const dir = await mkdtemp(join(tmpdir(), "loom-t21-"));
  t.after(() => rm(dir, { recursive: true, force: true }));
  let pg: PGlite;
  const open = async (): Promise<Db> => {
    pg = new PGlite(dir); await pg.waitReady;
    return { query: (sql, params) => pg.query(sql, params) as any,
      tx: (fn) => pg.transaction(tx => fn({ query: (sql, params) => tx.query(sql, params) as any })), close: () => pg.close() };
  };
  let db = await open();
  const f = await fixture(db);
  await f.patch({ canvas_id: "canvas_a" });
  assert.equal((await f.engine.listMonitors(A))[0].canvas_id, "canvas_a");
  await f.patch({ canvas_config: config(), canvas_auto_run_grant: f.grant() });
  const successfulRun: any = config();
  successfulRun.nodes.forEach((node: any) => { node.metadata.lastRunSnapshot = { status: "success" }; node.metadata.content = "derived"; });
  await f.patch({ canvas_config: successfulRun });
  assert.equal((await f.engine.listMonitors(A))[0].canvas_auto_run_grant.approval_id, "canvas_approval_1");
  await f.patch({ enabled: false });
  await db.close(); db = await open();
  try {
    const restarted = new Engine(db);
    assert.deepEqual(await migrate(db), [], "migration is restart-safe");
    const binding = await (restarted as any).getScheduleCanvasBinding(A, f.monitor.schedule_id);
    assert.equal(binding.canvas_id, "canvas_a");
    assert.equal(binding.state, "disabled");
    assert.equal(binding.canvas_auto_run_grant.approval_id, "canvas_approval_1");
    assert.equal(binding.canvas_auto_run_grant.actor, actor);
    await restarted.updateMonitor(A, actor, f.monitor.id, { enabled: true });
    assert.equal((await (restarted as any).getScheduleCanvasBinding(A, f.monitor.schedule_id)).state, "active");
    await restarted.updateMonitor(A, actor, f.monitor.id, { canvas_id: null } as any);
  } finally { await db.close(); }
  db = await open();
  try {
    const binding = await (new Engine(db) as any).getScheduleCanvasBinding(A, f.monitor.schedule_id);
    assert.equal(binding.canvas_id, null);
    assert.equal(binding.canvas_auto_run_grant, null);
    const cols = (await db.query("SELECT column_name FROM information_schema.columns WHERE table_name='flow_schedules'")).rows.map(r => r.column_name);
    assert.ok(!cols.includes("canvas_id") && !cols.includes("canvas_auto_run_grant"), "Monitor is the only binding authority");
  } finally { await db.close(); }
});

test("T21 grant scope/actor/hash/version/budget are strict; rejected changes are atomic", async (t) => {
  const db = await memory(t); const f = await fixture(db);
  await f.patch({ canvas_id: "canvas_a" });
  for (const change of [
    { scope: { ...f.grant().scope, tenant_id: "other" } },
    { scope: { ...f.grant().scope, workspace_id: "other" } },
    { scope: { ...f.grant().scope, canvas_id: "other" } },
    { scope: { ...f.grant().scope, source_ids: [f.source2.id] } },
    { scope: { ...f.grant().scope, source_ids: [f.source.id, "foreign_source"] } },
    { actor: "user:mallory" }, { approval_id: "" }, { approval_id: "user-direct:alice:key" },
    { config_hash: "wrong" }, { config_version: 0 }, { budget_micro_per_window: -1 },
  ]) await assert.rejects(f.patch({ canvas_config: config(), canvas_auto_run_grant: { ...f.grant(), ...change } }), reason("canvas_grant_invalid"));
  assert.equal((await f.engine.listMonitors(A))[0].canvas_auto_run_grant, null);
  await assert.rejects(f.patch({ canvas_id: "other" }, { ...A, workspace_id: "other" }), (e: any) => e.status === 404);
  await assert.rejects((f.engine as any).getScheduleCanvasBinding({ ...A, tenant_id: "other" }, f.monitor.schedule_id), (e: any) => e.status === 404);
  await assert.rejects(f.patch({ canvas_id: "other", expected_version: 1 }), reason("monitor_version_conflict"));
});

test("T21 expanded sources/tools/cost/config require a new specific canvas approval, including preview apply", async (t) => {
  const db = await memory(t); const f = await fixture(db);
  await f.patch({ canvas_id: "canvas_a", canvas_config: config(), canvas_auto_run_grant: f.grant() });
  for (const patch of [{ source_ids: [f.source.id, f.source2.id] }, { budget_micro_per_window: 200 }, { interval_seconds: 300 }]) {
    await assert.rejects(f.patch({ ...patch, approval_id: "collection_only" }), reason("canvas_reapproval_required"));
  }
  await assert.rejects(f.patch({ canvas_config: config(), canvas_auto_run_grant: { ...f.grant(), scope: { ...f.grant().scope, tool_ids: ["intel_run_canvas_workflow", "intel_search"] } } }), reason("canvas_reapproval_required"));
  await assert.rejects(f.patch({ canvas_config: config(), canvas_auto_run_grant: { ...f.grant(), config_version: 2 } }), reason("canvas_reapproval_required"));
  const changed = config(); changed.nodes[1].metadata.logic!.expression = "n*3";
  await assert.rejects(f.patch({ canvas_config: changed }), reason("canvas_reapproval_required"));
  await assert.rejects(f.patch({ canvas_config: changed, canvas_auto_run_grant: { ...f.grant("new_approval"), config_hash: hash(changed) } }), reason("canvas_reapproval_required"));
  const pv = await f.engine.previewMonitor(A, { monitor_id: f.monitor.id, keywords: ["battery"], source_ids: [f.source.id, f.source2.id], interval_seconds: 3600, budget_micro_per_window: 100 });
  await assert.rejects(f.engine.applyMonitor(A, actor, "policy_v1", { preview_id: pv.preview_id, approval_id: "other_collection_approval", idempotency_key: "expand" }), reason("canvas_reapproval_required"));
  await f.patch({ canvas_config: changed, canvas_auto_run_grant: { ...f.grant("new_approval"), config_version: 2, config_hash: hash(changed), scope: { ...f.grant().scope, tool_ids: ["intel_run_canvas_workflow", "intel_search"] } } });
  assert.equal((await f.engine.listMonitors(A))[0].canvas_auto_run_grant.config_version, 2);
  await f.patch({ canvas_auto_run_grant: null, enabled: false });
  await f.patch({ enabled: true });
  assert.equal((await f.engine.listMonitors(A))[0].canvas_auto_run_grant, null, "resume cannot resurrect revoked authorization");
  await f.patch({ canvas_config: config(), canvas_auto_run_grant: f.grant("approval_after_revocation") });
  await f.patch({ canvas_id: "canvas_b" });
  assert.equal((await f.engine.listMonitors(A))[0].canvas_auto_run_grant, null, "rebind cannot inherit old canvas authority");
});

test("T21 configuration hash excludes derived writes, not authored inputs or configuration", () => {
  const before = config(); const after: any = structuredClone(before);
  after.nodes.forEach((n: any) => { n.metadata.lastRunSnapshot = { status: "success", sampleItems: [42] }; n.metadata.content = "derived output"; });
  assert.equal(hash(before), hash(after));
  after.nodes[1].metadata.logic.expression = "n*99";
  assert.notEqual(hash(before), hash(after));
  const authored: any = { nodes: [{ id: "text", type: "text", metadata: { content: "input A" } }], connections: [] };
  const edited = structuredClone(authored); edited.nodes[0].metadata.content = "input B";
  assert.notEqual(hash(authored), hash(edited));
  const bound: any = { nodes: [{ id: "src", type: "data-source", metadata: { dataSource: { sourceBinding: { kind: "topic", refId: "topic_a" }, rawText: "old" }, content: "old" } }], connections: [] };
  const refreshed = structuredClone(bound); refreshed.nodes[0].metadata.dataSource.rawText = "new";
  refreshed.nodes[0].metadata.dataSource.sourceBinding.lastSyncedAt = "now"; refreshed.nodes[0].metadata.content = "new";
  assert.equal(hash(bound), hash(refreshed));
  refreshed.nodes[0].metadata.dataSource.sourceBinding.refId = "topic_b";
  assert.notEqual(hash(bound), hash(refreshed));
});

test("T21 HTTP never trusts injected canvas grants even with a valid platform token", async (t) => {
  const db = await memory(t); const f = await fixture(db);
  const key = "k".repeat(32); const server = createServer(handler(f.engine, { signingKey: key, audience: "loom-engine" }));
  await new Promise<void>(resolve => server.listen(0, "127.0.0.1", resolve));
  t.after(() => new Promise<void>(resolve => server.close(() => resolve())));
  const token = sign(key, { ...A, iss: "platform", aud: "loom-engine", actor: { type: "user", id: "alice" }, scopes: ["intel.monitors.write"], policy_version: "policy_v1" });
  const res = await fetch(`http://127.0.0.1:${(server.address() as any).port}/v1/monitors/${f.monitor.id}`, { method: "PATCH", headers: { authorization: `Bearer ${token}` }, body: JSON.stringify({ canvas_id: "canvas_a", canvas_auto_run_grant: { approval_id: "fake" } }) });
  assert.equal(res.status, 409);
  assert.equal((await res.json()).reason, "canvas_approval_unavailable");
  assert.equal((await f.engine.listMonitors(A))[0].canvas_id, null);
});

test("T21 recurring grant requires bounded frequency, budget, runs, expiry and no revocation", async (t) => {
  const db = await memory(t); const f = await fixture(db);
  for (const change of [{ purpose: "canvas_workflow_once" }, { interval_seconds: 300 }, { max_runs: 0 }, { total_budget_micro: 50 },
    { expires_at: "2000-01-01T00:00:00Z" }, { revoked_at: "2026-01-01T00:00:00Z" }, { monitor_id: "other" }]) {
    await assert.rejects(f.patch({ canvas_id: "canvas_a", canvas_config: config(), canvas_auto_run_grant: { ...f.grant(), ...change } }), reason("canvas_grant_invalid"));
  }
});

test("T21 signed server approval binds exact mutation, actor, monitor, expiry and optimistic version", async (t) => {
  const db = await memory(t); const f = await fixture(db); const key = "k".repeat(32);
  const server = createServer(handler(f.engine, { signingKey: key, audience: "loom-engine" }));
  await new Promise<void>(resolve => server.listen(0, "127.0.0.1", resolve));
  t.after(() => new Promise<void>(resolve => server.close(() => resolve())));
  const token = sign(key, { ...A, iss: "platform", aud: "loom-engine", actor: { type: "user", id: "alice" }, scopes: ["intel.monitors.write"], policy_version: "policy_v1" });
  const mutation = { canvas_id: "canvas_a", canvas_config: config(), canvas_auto_run_grant: f.grant(), expected_version: 1 };
  const seal = (overrides: any = {}) => {
    const payload = Buffer.from(JSON.stringify({ ...A, actor, monitor_id: f.monitor.id, expires_at: Math.floor(Date.now()/1000)+60, mutation, ...overrides })).toString("base64url");
    return { payload, signature: createHmac("sha256", key).update("monitor-canvas-v1\n"+payload).digest("hex") };
  };
  const call = (body: any) => fetch(`http://127.0.0.1:${(server.address() as any).port}/v1/monitors/${f.monitor.id}`, { method: "PATCH", headers: { authorization: `Bearer ${token}` }, body: JSON.stringify(body) });
  for (const proof of [seal({ actor: "user:mallory" }), seal({ monitor_id: "other" }), seal({ workspace_id: "other" }), seal({ expires_at: 1 }), { ...seal(), signature: "0".repeat(64) }]) {
    const res = await call({ canvas_authorization: proof }); assert.equal(res.status, 403); assert.equal((await res.json()).reason, "canvas_authorization_invalid");
  }
  const proof = seal();
  assert.equal((await call({ canvas_authorization: proof, canvas_id: "injected" })).status, 403);
  const ok = await call({ canvas_authorization: proof }); assert.equal(ok.status, 200, await ok.text());
  assert.equal((await f.engine.listMonitors(A))[0].canvas_auto_run_grant.approval_id, "canvas_approval_1");
  await f.patch({ canvas_id: null });
  assert.equal((await call({ canvas_authorization: proof })).status, 409, "old signed authorization cannot resurrect an unlinked grant");
});
