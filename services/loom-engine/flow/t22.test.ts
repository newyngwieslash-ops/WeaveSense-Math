import { test } from "node:test";
import assert from "node:assert/strict";
import { createServer } from "node:http";
import { migrate, pgliteDb } from "./db.ts";
import { Engine } from "./engine.ts";
import { handler } from "./server.ts";
import { sign } from "./auth.ts";

const A = { tenant_id: "tn_t22", workspace_id: "ws_t22" };
const at = new Date("2026-10-25T12:00:00Z");
async function setup(t: any, partial = false) {
  const db = await pgliteDb(); t.after(() => db.close()); await migrate(db);
  const engine = new Engine(db, async (_url, source) => {
    const items = [1, 2].map(i => ({ external_id: `${source}-${i}`, title: `battery ${i}`, body: "real collected material", published_at: at.toISOString() }));
    return partial ? { items, collector: "rss", mode: "live", rejected_entries: 1 } as any : items;
  }, () => at);
  const sources = await Promise.all(["a", "b"].map(id => engine.createSource(A, { url: `https://example.invalid/${id}` })));
  const pv = await engine.previewMonitor(A, { keywords: ["battery"], source_ids: sources.map(s => s.id), interval_seconds: 3600 });
  const { monitor } = await engine.applyMonitor(A, "user:alice", "policy_v1", { preview_id: pv.preview_id, approval_id: "approved", idempotency_key: "monitor" });
  await db.query("UPDATE flow_schedules SET next_run_at=$2 WHERE id=$1", [monitor.schedule_id, at.toISOString()]);
  await engine.schedulerTick();
  return { db, engine, monitor, sources };
}

test("T22 completed events distinguish source completion from entire frozen multi-source window", async t => {
  const f = await setup(t);
  const first = await f.engine.claim("worker-a"); await f.engine.execute(first);
  let events = (await f.db.query("SELECT payload FROM flow_outbox WHERE type='collection.completed' ORDER BY sequence")).rows;
  assert.equal(events.length, 1);
  const p = events[0].payload;
  assert.equal(p.schedule_id, f.monitor.schedule_id);
  assert.equal(p.window.scheduled_at, at.toISOString());
  assert.equal(p.operation_id, first.operation_id);
  assert.equal(p.cycle_complete, false);
  assert.equal(p.source_completions.filter((x: any) => x.status === "succeeded").length, 1);
  assert.deepEqual([...p.expected_source_ids].sort(), f.sources.map(s => s.id).sort());
  const second = await f.engine.claim("worker-b"); await f.engine.execute(second);
  events = (await f.db.query("SELECT payload FROM flow_outbox WHERE type='collection.completed' ORDER BY sequence")).rows;
  assert.equal(events[1].payload.cycle_complete, true);
  assert.equal(events[1].payload.cycle_status, "succeeded");
  assert.equal(events[1].payload.source_completions.length, 2);
  await assert.rejects(f.engine.execute(first), (e: any) => e.status === 409);
  assert.equal((await f.db.query("SELECT * FROM flow_outbox WHERE type='collection.completed'")).rows.length, 2);
});

test("T22 evidence validates exact revision and UTF16 range, returns stable receipt without recent-list lookup", async t => {
  const f = await setup(t); await f.engine.execute(await f.engine.claim("a"));
  const doc = (await f.db.query("SELECT id FROM flow_documents ORDER BY id LIMIT 1")).rows[0];
  const read = await f.engine.readDocument(A, { document_id: doc.id });
  for (const item of [
    { document_id: doc.id, revision: 99, char_start: 0, char_end: 7, quote: "battery" },
    { document_id: doc.id, revision: 1, char_start: 1, char_end: 8, quote: "battery" },
  ]) await assert.rejects(f.engine.createEvidence(A, "bad:" + item.revision, { items: [item] }), (e: any) => e.status === 422);
  const request = { items: [{ document_id: doc.id, revision: 1, char_start: 0, char_end: 7, quote: read.text.slice(0, 7) }] };
  const first: any = await f.engine.createEvidence(A, "receipt", request);
  assert.equal(first.items.length, 1); assert.ok(first.items[0].evidence_id);
  assert.equal(first.items[0].revision, 1);
  assert.equal((await f.engine.createEvidence(A, "receipt", request) as any).items[0].evidence_id, first.items[0].evidence_id);
  await assert.rejects(f.engine.createEvidence(A, "receipt", { items: [{ ...request.items[0], char_end: 6, quote: read.text.slice(0, 6) }] }), (e: any) => e.status === 409);
  const body = "a".repeat(19997) + "😀" + "z";
  await f.db.query("UPDATE flow_document_revisions SET title='', body=$2 WHERE document_id=$1 AND revision=1", [doc.id, body]);
  const page = await f.engine.readDocument(A, { document_id: doc.id, revision: 1, max_chars: 20000 });
  assert.equal(page.coverage.returned_chars, 19999);
  const next = await f.engine.readDocument(A, { document_id: doc.id, revision: 1, offset: page.coverage.next_offset!, max_chars: 20000 });
  assert.equal(page.text + next.text, "\n\n" + body);
  assert.ok(!JSON.stringify(page).includes("\\ud83d"));
  await assert.rejects(f.engine.readDocument(A, { document_id: doc.id, revision: 1, offset: 20000 }), (e: any) => e.status === 400);
});

test("T22 partial job never announces a successful cycle", async t => {
  const f = await setup(t, true);
  await f.engine.execute(await f.engine.claim("a")); await f.engine.execute(await f.engine.claim("b"));
  const events = (await f.db.query("SELECT payload FROM flow_outbox WHERE type='collection.completed' ORDER BY sequence")).rows;
  assert.equal(events.length, 2);
  assert.ok(events.every(e => e.payload.cycle_complete === false));
  assert.equal(events[1].payload.cycle_status, "partial");
});

test("T22 real monitor hits HTTP is scoped, revision-pinned, paged and half-open", async t => {
  const f = await setup(t);
  await f.engine.execute(await f.engine.claim("a")); await f.engine.execute(await f.engine.claim("b"));
  const key = "t".repeat(32);
  const server = createServer(handler(f.engine, { signingKey: key, audience: "loom-engine" }));
  await new Promise<void>(resolve => server.listen(0, "127.0.0.1", resolve));
  t.after(() => new Promise<void>(resolve => server.close(() => resolve())));
  async function get(scope: any, suffix = "") {
    const token = sign(key, { ...scope, iss: "platform", aud: "loom-engine", actor: { type: "user", id: "alice" }, scopes: ["intel.documents.read"], policy_version: "v1" });
    return fetch(`http://127.0.0.1:${(server.address() as any).port}/v1/monitors/${f.monitor.id}/hits${suffix}`, { headers: { authorization: `Bearer ${token}` } });
  }
  const response = await get(A, "?limit=1"); assert.equal(response.status, 200);
  const page = (await response.json()).data; assert.equal(page.items.length, 1); assert.equal(page.next_offset, 1);
  const hit = page.items[0]; assert.ok(hit.document_id); assert.equal(hit.revision, 1); assert.ok(hit.operation_id); assert.equal(hit.monitor_id, f.monitor.id);
  const next = (await (await get(A, "?limit=1&offset=1")).json()).data;
  assert.notEqual(next.items[0].hit_id, hit.hit_id);
  assert.equal((await get({ ...A, workspace_id: "foreign" })).status, 404);
  assert.equal((await get({ ...A, tenant_id: "foreign" })).status, 404);
  const empty = (await (await get(A, "?from=2026-10-24T00:00:00Z&to=2026-10-25T12:00:00Z")).json()).data;
  assert.deepEqual(empty.items, []);
  assert.equal((await get(A, "?offset=-1")).status, 400);
  const restarted = new Engine(f.db, async () => { throw new Error("no external call"); });
  assert.equal((await (restarted as any).listMonitorHits(A, f.monitor.id, {})).items.length, 4);
});
