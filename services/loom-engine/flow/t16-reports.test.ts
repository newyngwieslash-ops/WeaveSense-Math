import { test } from "node:test";
import assert from "node:assert/strict";
import { pgliteDb, migrate } from "./db.ts";
import { Engine } from "./engine.ts";
import { Reports } from "./reports.ts";
import { handler } from "./server.ts";
import { sign } from "./auth.ts";
import { createServer } from "node:http";
const scope = { tenant_id: "t16", workspace_id: "w16" };

test("T16 report persists full payload, checks fixed key and nested citations", async () => {
  const db = await pgliteDb(); await migrate(db);
  try {
    const e = new Engine(db, async () => [{ external_id: "1", title: "Monitoring", body: "Monitoring measured value 2", url: "https://example.test/doc", published_at: "2026-10-12T00:00:00Z" }]);
    const src = await e.createSource(scope, { url: "https://example.test/feed" });
    await e.requestCollection(scope, "collect", { source_ids: [src.id] }); await e.workOnce("test");
    const rp = new Reports(e);
    const research = await rp.startResearch(scope, "research", { parent_run_id: "run16", question: "Monitoring" });
    const citation = research.output.claims[0].citations[0];
    const payload = { title: "Report", body: "# Full body", claims: research.output.claims, material_scope: { source_ids: [src.id] }, skill: { id: "skill16", version: 1, hash: "pinned" }, coverage: { regions: ["A"], truncated: true }, structured: { regional_metrics: [{ region: "A", citations: [citation], factor_source: { version: "v1", citations: [citation] } }] } };
    const saved = await rp.saveReport(scope, "user:alice", "save16", payload);
    const server = createServer(handler(e, { signingKey: "k".repeat(32), audience: "loom-engine" }));
    await new Promise<void>(resolve => server.listen(0, "127.0.0.1", resolve));
    try {
      const lookup = async (workspace_id: string, hash: string) => {
        const token = sign("k".repeat(32), { ...scope, workspace_id, iss: "platform", aud: "loom-engine", actor: { type: "user", id: "alice" }, scopes: ["intel.reports.write"], policy_version: "policy_v1" });
        return fetch(`http://127.0.0.1:${(server.address() as any).port}/v1/reports/by-key?key=save16&request_hash=${hash}`, { headers: { authorization: `Bearer ${token}` } });
      };
      const hash = (await db.query("SELECT request_hash FROM flow_reports WHERE id=$1", [saved.report_id])).rows[0].request_hash;
      const found = await lookup(scope.workspace_id, hash); assert.equal(found.status, 200); assert.equal((await found.json() as any).data.report_id, saved.report_id);
      assert.equal((await lookup(scope.workspace_id, "wrong")).status, 409);
      assert.equal((await lookup("foreign", hash)).status, 404);
    } finally { await new Promise<void>(resolve => server.close(() => resolve())); }
    const loaded = await new Reports(e).getReport(scope, saved.report_id!);
    assert.deepEqual(loaded.structured, payload.structured);
    assert.equal(loaded.body, payload.body); assert.deepEqual(loaded.coverage.regions, ["A"]);
    assert.equal((await new Reports(e).saveReport(scope, "user:alice", "save16", payload)).report_id, saved.report_id);
    await assert.rejects(rp.saveReport(scope, "user:alice", "save16", { ...payload, body: "different" }), (err: any) => err.status === 409);
    await assert.rejects(rp.saveReport({ ...scope, workspace_id: "other" }, "user:alice", "save16", payload), (err: any) => err.status === 409);
    const bad = structuredClone(payload); bad.structured.regional_metrics[0].factor_source.citations[0].revision = 900;
    await assert.rejects(rp.saveReport(scope, "user:alice", "bad16", bad), (err: any) => err.status === 422);
    const missing = structuredClone(payload); missing.structured.regional_metrics[0].citations = [];
    await assert.rejects(rp.saveReport(scope, "user:alice", "missing16", missing), (err: any) => err.status === 422);
    const sameSource = { ...payload, claims: [{ text: "Two sources", kind: "cross_source_verified", citations: [citation, citation] }] };
    await assert.rejects(rp.saveReport(scope, "user:alice", "single-source", sameSource), (err: any) => err.status === 422);
    const rows = await db.query("SELECT id FROM flow_reports"); assert.equal(rows.rows.length, 1);
    const original = (await db.query("SELECT * FROM flow_evidence WHERE id=$1", [citation.evidence_id])).rows[0];
    await db.query("UPDATE flow_evidence SET revision=999 WHERE id=$1", [citation.evidence_id]);
    assert.equal((await rp.resolveEvidence(scope, [citation.evidence_id]))[0].snippet_ok, false, "nonexistent document revision is not evidence");
    await db.query("UPDATE flow_evidence SET revision=$2, char_start=char_start+1, char_end=char_end+1 WHERE id=$1", [citation.evidence_id, original.revision]);
    assert.equal((await rp.resolveEvidence(scope, [citation.evidence_id]))[0].snippet_ok, false, "quote occurrence elsewhere cannot validate incorrect UTF16 range");
    await assert.rejects(rp.saveReport(scope, "user:alice", "bad-range", payload), (err: any) => err.status === 422);
  } finally { await db.close(); }
});
