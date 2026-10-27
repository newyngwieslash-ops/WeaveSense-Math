import importlib.util
import pathlib
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("ho", pathlib.Path(__file__).with_name("check-edu-handover.py"))
ho = importlib.util.module_from_spec(spec)
spec.loader.exec_module(ho)

PACKS = ["W0", "W1", "W2", "W3", "W4", "W5", "W6", "W7", "试点"]


def write(root, statuses, boxes=None, gaps=None, summary=None):
    d = pathlib.Path(root) / "docs" / "math-workbench"
    d.mkdir(parents=True, exist_ok=True)
    boxes = boxes if boxes is not None else {k: v == "完成" for k, v in statuses.items()}
    gaps = gaps if gaps is not None else {k: v for k, v in statuses.items() if v != "完成"}
    counts = {p: {s: 0 for s in ho.STATUSES} for p in PACKS + ["合计"]}
    for k, v in statuses.items():
        for p in (ho.package(k), "合计"):
            if v in counts[p]:
                counts[p][v] += 1
    lines = ["| 工作包 | 完成 | 进行中 | 待开始 | 阻塞 | 合计 |", "| --- | --- | --- | --- | --- | --- |"]
    for p in PACKS + ["合计"]:
        c = (summary or {}).get(p) or [counts[p][s] for s in ho.STATUSES] + [sum(counts[p].values())]
        lines.append("| " + " | ".join([p] + [str(x) for x in c]) + " |")
    lines += ["", "| DEV | 名称 | 状态 | 证据 | 说明 |", "| --- | --- | --- | --- | --- |"]
    lines += [f"| {k} | 名称 | {v} | E1 | 说明 |" for k, v in statuses.items()]
    (d / "04-追踪矩阵.md").write_text("\n".join(lines) + "\n", encoding="utf-8")
    (d / "WeaveSense-Flow_Development_Checklist.md").write_text(
        "\n".join(f"- [{'x' if b else ' '}] **{k} 名称**" for k, b in boxes.items()) + "\n", encoding="utf-8")
    (d / "11-运维交接与已知缺口.md").write_text(
        "\n".join(f"| {k} | {v} | 名称 | 缺口 |" for k, v in gaps.items()) + "\n", encoding="utf-8")


BASE = {"DEV-001": "完成", "DEV-013": "进行中", "DEV-068": "待开始", "DEV-133": "阻塞"}


class Handover(unittest.TestCase):
    def run_check(self, **kw):
        with tempfile.TemporaryDirectory() as root:
            write(root, kw.pop("statuses", BASE), **kw)
            return ho.check(root)[0]

    def test_consistent(self):
        self.assertEqual(self.run_check(), [])

    def test_unticked_done_item_fails(self):
        p = self.run_check(boxes={**{k: v == "完成" for k, v in BASE.items()}, "DEV-001": False})
        self.assertTrue(any("DEV-001" in x for x in p), p)

    def test_ticked_open_item_fails(self):
        p = self.run_check(boxes={**{k: v == "完成" for k, v in BASE.items()}, "DEV-013": True})
        self.assertTrue(any("DEV-013" in x for x in p), p)

    def test_missing_gap_fails(self):
        p = self.run_check(gaps={"DEV-013": "进行中", "DEV-133": "阻塞"})
        self.assertTrue(any("DEV-068" in x for x in p), p)

    def test_wrong_gap_status_fails(self):
        p = self.run_check(gaps={"DEV-013": "完成", "DEV-068": "待开始", "DEV-133": "阻塞"})
        self.assertTrue(any("DEV-013" in x for x in p), p)

    def test_done_item_listed_as_gap_fails(self):
        p = self.run_check(gaps={"DEV-001": "进行中", "DEV-013": "进行中", "DEV-068": "待开始", "DEV-133": "阻塞"})
        self.assertTrue(any("DEV-001" in x for x in p), p)

    def test_stale_summary_fails(self):
        p = self.run_check(summary={"W1": [1, 0, 0, 0, 1]})
        self.assertTrue(any("W1" in x for x in p), p)

    def test_unknown_status_fails(self):
        p = self.run_check(statuses={**BASE, "DEV-002": "差不多"})
        self.assertTrue(any("差不多" in x for x in p), p)


if __name__ == "__main__":
    unittest.main()
