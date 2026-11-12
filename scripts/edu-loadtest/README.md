# Education load-test harness (DEV-119)

Scaffold only. It does **not** replace a formal capacity and latency report on
the target environment (U16).

Scenarios:

* Task accept p95 — `k6-edu.js` `job_accept` (POST analyze), starting threshold 800 ms
* Authorized read p95 — `k6-edu.js` `auth_rw` and `vegeta-targets.txt`, starting threshold 400 ms

```bash
# Validate files without generating traffic
./scripts/edu-loadtest/run.sh --scaffold-only

# Local or target run (requires k6 or vegeta, a running API, and a test identity)
BASE_URL=http://127.0.0.1:18080 TOKEN=... WORKSPACE_ID=... REVISION_ID=... ./scripts/edu-loadtest/run.sh
```

Use fictional accounts only. Do not commit tokens, passwords, or internal hostnames.
