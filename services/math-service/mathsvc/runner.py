# -*- coding: utf-8 -*-
"""Subprocess entry: python -m mathsvc.runner <stage> <cpu_s> <mem_bytes>  (JSON in, JSON out)."""

import json
import resource
import sys

from .errors import MathError


def apply_limits(cpu_s, mem_bytes):
    """Limit this process before it reads the request or imports the math code.

    The limits are set here, after exec, instead of in a preexec_fn: running
    Python between fork and exec in the multi-threaded service can deadlock the
    child before exec, and the parent then waits in Popen outside any timeout
    (seen as a cheap stage taking 116 s against a 30 s wall ceiling)."""
    resource.setrlimit(resource.RLIMIT_AS, (mem_bytes, mem_bytes))
    # SIGXCPU at the soft limit, SIGKILL one second later.
    resource.setrlimit(resource.RLIMIT_CPU, (cpu_s, cpu_s + 1))
    resource.setrlimit(resource.RLIMIT_NOFILE, (64, 64))


def main():
    stage = sys.argv[1]
    apply_limits(int(sys.argv[2]), int(sys.argv[3]))
    req = json.load(sys.stdin)
    try:
        from .pipeline import run_stage
        out = {"ok": True, "result": run_stage(stage, req["payload"], req.get("prior") or {})}
    except MathError as e:
        out = {"ok": False, "error": e.to_dict()}
    except Exception as e:  # noqa: BLE001 - reported as internal, never as "no solution"
        out = {"ok": False, "error": MathError("INTERNAL", "stage crashed: %s" % type(e).__name__).to_dict()}
    sys.stdout.write(json.dumps(out, ensure_ascii=False))


if __name__ == "__main__":
    main()
