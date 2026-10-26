import { createHash } from "node:crypto";
import { HttpError } from "./errors.ts";

export function reportPayloadHash(value: any): string {
  const canonical = (v: any): any => Array.isArray(v) ? v.map(canonical) : v && typeof v === "object" ? Object.fromEntries(Object.keys(v).sort().map(k => [k, canonical(v[k])])) : v;
  return createHash("sha256").update(JSON.stringify(canonical(value))).digest("hex");
}

// Treat every structured row and nested source citation as evidence, not just
// prose claims. Coverage is disclosure metadata, not an evidence-bearing row.
export function reportEvidenceClaims(o: any): any[] {
  if (!Array.isArray(o.claims) || !o.claims.length || o.claims.length > 100) throw new HttpError(422, "claims: 1..100 required");
  const claims = [...o.claims];
  const visit = (v: any, path: string, row = false) => {
    if (Array.isArray(v)) { v.forEach((x, i) => visit(x, `${path}[${i}]`, true)); return; }
    if (!v || typeof v !== "object") return;
    if (path.includes(".claims[")) { claims.push(v); return; }
    const requires = row || /(?:_source|_config)$/.test(path);
    if (requires && (!Array.isArray(v.citations) || !v.citations.length)) throw new HttpError(422, `${path}: row/source citations required`);
    if ("citations" in v) {
      if (!Array.isArray(v.citations) || !v.citations.length) throw new HttpError(422, `${path}: citations required`);
      claims.push({ text: path, kind: v.kind ?? "source_statement", citations: v.citations });
    }
    for (const [k, x] of Object.entries(v)) if (!["citations", "coverage", "gaps", "material_scope"].includes(k)) visit(x, `${path}.${k}`);
  };
  visit(o.structured, "structured");
  return claims;
}

export function validateReportEvidence(claims: any[], scope: any, known: Map<string, any>): void {
  if (!scope || (!scope.document_ids?.length && !scope.source_ids?.length)) throw new HttpError(422, "material_scope required");
  for (const c of claims) {
    if (!["source_statement", "cross_source_verified", "model_inference"].includes(c.kind) || !String(c.text ?? "").trim()) throw new HttpError(422, "invalid claim");
    if (!Array.isArray(c.citations) || (c.kind !== "model_inference" && !c.citations.length)) throw new HttpError(422, "claim citations required");
    const sources = new Set<string>();
    for (const x of c.citations) {
      const ev = known.get(String(x.evidence_id));
      if (!ev || !Number.isInteger(x.revision) || x.revision < 1 || x.revision !== ev.revision) throw new HttpError(422, "citation evidence not found at cited revision");
      if (!scope.document_ids?.includes(ev.document_id) && !scope.source_ids?.includes(ev.source_id)) throw new HttpError(422, "citation outside material scope");
      if (x.chunk_index != null && x.chunk_index !== ev.chunk_index) throw new HttpError(422, "citation chunk mismatch");
      if (!ev.snippet_ok || !ev.url_ok || (x.snippet_hash != null && x.snippet_hash !== ev.snippet_hash) || (x.url != null && x.url !== ev.url)) throw new HttpError(422, "citation snippet hash or url mismatch");
      if (ev.source_id) sources.add(ev.source_id);
    }
    if (c.kind === "cross_source_verified" && sources.size < 2) throw new HttpError(422, "cross_source_verified needs independent sources");
  }
}
