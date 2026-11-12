// Education load-test scaffold (DEV-119, E95).
// Measures two declared scenarios against a local or target environment:
//   job_accept — POST /api/edu/problem-revisions/:id/analyze (task accept)
//   auth_rw    — GET /api/edu/me plus GET /api/edu/observability (authorized read)
// Thresholds are starting points, not a formal capacity report. A formal p95
// report needs the target environment and vendor quotas (U16).
// Usage: BASE_URL=http://127.0.0.1:18080 TOKEN=... REVISION_ID=... k6 run k6-edu.js
import http from "k6/http";
import { check, sleep } from "k6";

const BASE = __ENV.BASE_URL || "http://127.0.0.1:18080";
const TOKEN = __ENV.TOKEN || "";
const REVISION = __ENV.REVISION_ID || "";
const WORKSPACE = __ENV.WORKSPACE_ID || "";

export const options = {
  scenarios: {
    job_accept: {
      executor: "constant-arrival-rate",
      rate: Number(__ENV.JOB_RATE || 2),
      timeUnit: "1s",
      duration: __ENV.DURATION || "10s",
      preAllocatedVUs: 5,
      exec: "jobAccept",
    },
    auth_rw: {
      executor: "constant-arrival-rate",
      rate: Number(__ENV.AUTH_RATE || 5),
      timeUnit: "1s",
      duration: __ENV.DURATION || "10s",
      preAllocatedVUs: 5,
      exec: "authRW",
    },
  },
  thresholds: {
    "http_req_duration{scenario:job_accept}": ["p(95)<800"],
    "http_req_duration{scenario:auth_rw}": ["p(95)<400"],
    checks: ["rate>0.99"],
  },
};

function headers() {
  const h = { "Content-Type": "application/json" };
  if (TOKEN) h.Authorization = "Bearer " + TOKEN;
  if (WORKSPACE) h["X-Workspace-Id"] = WORKSPACE;
  return h;
}

export function jobAccept() {
  if (!REVISION) {
    // Scaffold mode: hit a documented path so the script is executable;
    // a missing revision is not a formal measurement.
    const res = http.get(BASE + "/api/edu/me", { headers: headers() });
    check(res, { "job_accept scaffold reached the server": (r) => r.status !== 0 });
    return;
  }
  const res = http.post(
    BASE + "/api/edu/problem-revisions/" + REVISION + "/analyze",
    JSON.stringify({}),
    { headers: headers() },
  );
  check(res, { "job_accept got a response": (r) => r.status !== 0 });
  sleep(0.1);
}

export function authRW() {
  const r1 = http.get(BASE + "/api/edu/me", { headers: headers() });
  const r2 = http.get(BASE + "/api/edu/observability", { headers: headers() });
  check(r1, { "auth read /me reached the server": (r) => r.status !== 0 });
  check(r2, { "auth read /observability reached the server": (r) => r.status !== 0 });
}
