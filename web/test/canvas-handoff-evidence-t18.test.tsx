import { expect, test } from "bun:test";
import { buildReportHandoffDraft } from "@/pages/intelligence/canvas-handoff-modal";
import type { ReportDetail } from "@/services/api/intelligence";

for (const kind of ["report", "brief"] as const) test(`${kind} real handoff preserves evidence identity, revision and supplied char bounds`, () => {
    const citation = { evidence_id: "actual-evidence-from-response", document_id: "doc", revision: 3, source_id: "source", chunk_index: 1, quote: "same quotation", truncated: false, char_start: 20, char_end: 34 };
    const detail = { id: "report-id", title: "title", kind, body: "body", skill: {}, coverage: {}, created_at: "now", claims: [{ idx: 1, text: "claim", kind: "source_statement", citations: [citation, citation, { ...citation, evidence_id: "second-evidence", revision: 4 }, { document_id: "display-only", quote: "display quote", title: "display" }] }] } as unknown as ReportDetail;
    const draft = buildReportHandoffDraft(detail);
    expect(draft.kind).toBe(kind);
    expect(draft.citations).toHaveLength(3);
    expect(draft.citations![0]).toMatchObject({ document_id: "doc", evidence_id: citation.evidence_id, revision: 3, char_start: 20, char_end: 34 });
    expect(draft.citations![1]).toMatchObject({ evidence_id: "second-evidence", revision: 4 });
    expect(draft.citations![2]).toEqual({ document_id: "display-only", title: "display", snippet: "display quote" });
});
