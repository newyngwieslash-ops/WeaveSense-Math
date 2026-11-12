# -*- coding: utf-8 -*-
"""Versioned capability catalog (PRD §6.2, FR-M02).

Optional overlays (B4 / E94), off by default:
  MATH_CATALOG_ADMISSION_OVERLAY  JSON; only a signed official_frozen_set may
                                  raise a family from dev to admitted.
  MATH_FROZEN_PARAM_RANGES        JSON; only a signed official_frozen_set may
                                  tighten per-family parameter maxima.
Engineering admission sets and sample fixtures must not flip admission.

admission:
  planned      not implemented, the UI must not offer automatic verification;
  dev          implemented, development regression only, NOT admitted;
  admitted     passed the frozen admission evaluation (PRD §17.2) for this catalog version.
Only "admitted" families may be shown as "supported with automatic verification".
"""

CATALOG_VERSION = "edu-math-catalog/2026.10.0"

LIMITS = {
    "param_text_max": 48,
    "param_ast_nodes_max": 16,
    "param_ast_depth_max": 6,
    "integer_abs_max": 1000000,
    "radicand_max": 10000,
    "solve_timeout_s": 10,
    "verify_timeout_s": 10,
    "render_timeout_s": 30,
    "worker_memory_mb": 512,
    "upload_max_bytes": 10 * 1024 * 1024,
    "upload_max_pixels": 8192,
}

FAMILIES = [
    {"id": "SOLID-VOL-01", "name": "长方体体积与单位换算", "admission": "dev",
     "params": {"l": "(0,1000]", "w": "(0,1000]", "h": "(0,1000]"}, "units": ["", "mm", "cm", "dm", "m"],
     "methods": ["formula"], "target": "volume", "solver": "solid-vol-01/1.0.0",
     "notes": "三条棱两两垂直；可逐棱给单位（units），单位不同时必须给 target_unit；不含斜棱柱、缺边长。"},
    {"id": "SOLID-ANGLE-01", "name": "正方体 A1C 与底面 ABCD 所成角", "admission": "dev",
     "params": {"e": "(0,100]"}, "units": ["", "mm", "cm", "dm", "m"], "methods": ["vector_coordinate", "synthetic"],
     "target": "sin_angle_A1C_ABCD", "solver": "solid-angle-01/1.0.0",
     "notes": "只支持 A1C 与底面 ABCD；余弦、正切作为派生值；不泛化为任意线面角。"},
    {"id": "SOLID-ANGLE-02", "name": "正四棱锥 BE 与平面 PAC 所成角正弦", "admission": "dev",
     "params": {"a": "(0,100] 整数/有理数/受限根式", "h": "(0,100] 整数/有理数/受限根式"},
     "units": ["", "mm", "cm", "dm", "m"],
     "methods": ["vector_coordinate", "synthetic"],
     "target": "sin_angle_BE_PAC", "solver": "solid-angle-02/1.0.0",
     "notes": "E 为 PC 中点；点名必须为标准命名 P-ABCD；超出范围返回 PARAMETER_LIMIT。"},
    {"id": "CONIC-CHORD-01", "name": "椭圆右焦点弦长取值范围与端点构型", "admission": "dev",
     "params": {"a": "(0,100]", "b": "(0,a)"}, "methods": ["vieta"], "target": "focal_chord_length_range",
     "solver": "conic-chord-01/1.0.0",
     "notes": "x²/a²+y²/b²=1，a>b>0，只支持右焦点；竖直弦单独检查；水平弦（最大）与竖直弦（最小）均可取到。"},
    {"id": "CONIC-SLOPE-01", "name": "椭圆斜率积定值 k_PA·k_PB", "admission": "dev",
     "params": {"a": "(0,100]", "b": "(0,a)"}, "methods": ["point_substitution"], "target": "slope_product_PA_PB",
     "solver": "conic-slope-01/1.0.0",
     "notes": "P 为右顶点 (a,0)，A、B 关于原点对称；排除 A/B 为左右顶点，禁用点在图形中标出。"},
    {"id": "CONIC-PARABOLA-01", "name": "抛物线焦点弦 OA·OB 定值", "admission": "dev",
     "params": {"p": "(0,100]"}, "methods": ["vieta"], "target": "dot_OA_OB", "solver": "conic-parabola-01/1.0.0",
     "notes": "教材约定 y²=2px，焦点 (p/2,0)；排除轴向直线 y=0；两有限实交点由判别式保证。"},
]


def catalog():
    overlay = admission_overlay()
    families = [_apply_admission(f, overlay) for f in FAMILIES]
    out = {"catalog_version": CATALOG_VERSION, "limits": LIMITS, "families": families}
    if overlay:
        out["admission_overlay"] = {k: overlay[k] for k in ("error", "source_kind", "frozen_set_version") if k in overlay}
    return out


def family(fid):
    overlay = admission_overlay()
    for f in FAMILIES:
        if f["id"] == fid:
            return _apply_admission(f, overlay)
    return None


def _read_json_env(name):
    import json
    import os
    path = (os.environ.get(name) or "").strip()
    if not path:
        return None, None
    try:
        with open(path, encoding="utf-8") as fh:
            return json.load(fh), path
    except (OSError, ValueError) as e:
        return {"_error": "unreadable:%s" % e}, path


def admission_overlay():
    data, path = _read_json_env("MATH_CATALOG_ADMISSION_OVERLAY")
    if data is None:
        return None
    if data.get("_error"):
        return {"error": data["_error"], "path": path}
    if data.get("kind") != "catalog_admission_overlay":
        return {"error": "bad_kind", "path": path}
    if data.get("source_kind") != "official_frozen_set" or not data.get("signed"):
        return {"error": "not_official_or_unsigned", "path": path, "source_kind": data.get("source_kind")}
    if data.get("catalog_version") and data["catalog_version"] != CATALOG_VERSION:
        return {"error": "catalog_version_mismatch", "path": path}
    return {"error": None, "path": path, "source_kind": data.get("source_kind"),
            "frozen_set_version": data.get("frozen_set_version"), "families": data.get("families") or {}}


def _apply_admission(f, overlay):
    out = dict(f)
    if overlay and overlay.get("error") is None:
        st = (overlay.get("families") or {}).get(f["id"])
        if st in ("dev", "admitted", "planned"):
            out["admission"] = st
    return out


def frozen_param_ranges():
    data, path = _read_json_env("MATH_FROZEN_PARAM_RANGES")
    if data is None:
        return None
    if data.get("_error"):
        return {"error": data["_error"], "path": path, "families": {}}
    if data.get("kind") != "frozen_param_ranges":
        return {"error": "bad_kind", "path": path, "families": {}}
    if data.get("source_kind") != "official_frozen_set" or not data.get("signed"):
        return {"error": "not_official_or_unsigned", "path": path, "families": {}}
    return {"error": None, "path": path, "families": data.get("families") or {}}


def effective_param_max(family_id, name, default_max):
    """Return the tighter of the family default and a signed official frozen cap."""
    from .params import parse_exact
    cap = default_max
    ranges = frozen_param_ranges()
    if not ranges or ranges.get("error"):
        return cap
    spec = ((ranges.get("families") or {}).get(family_id) or {}).get(name) or {}
    raw = spec.get("max")
    if raw is None or raw == "":
        return cap
    try:
        frozen = parse_exact(str(raw))
    except Exception:
        return cap
    try:
        default = parse_exact(str(default_max))
        return frozen if (default - frozen).is_positive else default
    except Exception:
        return frozen
