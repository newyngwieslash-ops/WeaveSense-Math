import importlib.util
import pathlib
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("ho", pathlib.Path(__file__).with_name("check-edu-handover.py"))
ho = importlib.util.module_from_spec(spec)
spec.loader.exec_module(ho)

PACKS = ["W0", "W1", "W2", "W3", "W4", "W5", "W6", "W7", "试点"]


def needs(devs):
    return "\n## 9 须提供\n\n| U1 | 事项 | 谁 | 需要 | " + " ".join(devs) + " | 动作 |\n"


def write(root, statuses, boxes=None, gaps=None, summary=None, need=None):
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
        "\n".join(f"| {k} | {v} | 名称 | 缺口 |" for k, v in gaps.items()) + "\n"
        + needs(need if need is not None else [k for k, v in statuses.items() if v != "完成"]), encoding="utf-8")


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

    def test_entity_table_must_list_every_model_table(self):
        with tempfile.TemporaryDirectory() as root:
            write(root, BASE)
            m = pathlib.Path(root) / "backend" / "internal" / "model"
            m.mkdir(parents=True)
            (m / "models_edu.go").write_text('func (A) TableName() string { return "edu_a" }\nfunc (B) TableName() string { return "edu_b" }\n', encoding="utf-8")
            d = pathlib.Path(root) / "docs" / "math-workbench"
            (d / "09-x.md").write_text("### 3.1\n| a edu_a |\n### 3.2\nedu_b\n", encoding="utf-8")
            p = ho.check(root)[0]
            self.assertEqual(p, ["09: entity table edu_b from models_edu.go is not listed in section 3.1"])
            (d / "09-x.md").write_text("### 3.1\n| a edu_a | edu_b |\n### 3.2\n", encoding="utf-8")
            self.assertEqual(ho.check(root)[0], [])


    def test_image_layers_must_be_listed(self):
        with tempfile.TemporaryDirectory() as root:
            write(root, BASE)
            r = pathlib.Path(root)
            (r / "Dockerfile").write_text("FROM --platform=$X a/b:1 AS build\nFROM c:2\nRUN apk add --no-cache x y \\\n    && adduser -S app\n", encoding="utf-8")
            d = r / "docs" / "math-workbench"
            (d / "06-x.md").write_text("### 部署镜像基础层\n| `a/b:1` | `x` |\n### 正式复核\n`c:2` `y`\n", encoding="utf-8")
            self.assertEqual(ho.check(root)[0], [
                "06: Dockerfile uses c:2 but the base layer section does not list it",
                "06: Dockerfile uses y but the base layer section does not list it",
            ])
            (d / "06-x.md").write_text("### 部署镜像基础层\n| `a/b:1` | `x` | `c:2` | `y` |\n", encoding="utf-8")
            self.assertEqual(ho.check(root)[0], [])


    def test_section_headings_must_match(self):
        with tempfile.TemporaryDirectory() as root:
            write(root, BASE)
            d = pathlib.Path(root) / "docs" / "math-workbench"
            f = d / "11-运维交接与已知缺口.md"
            good = "### 进行中（1）\n\n| DEV-013 | 进行中 | 名称 | 缺口 |\n\n### 待开始（1）\n\n| DEV-068 | 待开始 | 名称 | 缺口 |\n\n### 阻塞（1）\n\n| DEV-133 | 阻塞 | 名称 | 缺口 |\n"
            good += needs(["DEV-013", "DEV-068", "DEV-133"])
            f.write_text(good, encoding="utf-8")
            self.assertEqual(ho.check(root)[0], [])
            f.write_text(good.replace("进行中（1）", "进行中（2）"), encoding="utf-8")
            self.assertEqual(ho.check(root)[0], ["11: section 进行中 says 2 and lists 1 rows but 04 has 1"])
            moved = "### 进行中（1）\n\n### 待开始（1）\n\n| DEV-013 | 进行中 | 名称 | 缺口 |\n| DEV-068 | 待开始 | 名称 | 缺口 |\n\n### 阻塞（1）\n\n| DEV-133 | 阻塞 | 名称 | 缺口 |\n"
            moved += needs(["DEV-013", "DEV-068", "DEV-133"])
            f.write_text(moved, encoding="utf-8")
            self.assertEqual(ho.check(root)[0], [
                "11: section 进行中 says 1 and lists 0 rows but 04 has 1",
                "11: section 待开始 says 1 and lists 2 rows but 04 has 1",
                "11: DEV-013 is 进行中 but sits in the 待开始 section",
            ])

    def test_open_items_must_say_what_the_user_provides(self):
        self.assertEqual(self.run_check(need=["DEV-013", "DEV-001", "DEV-133"]), [
            "11: DEV-068 is 待开始 but section 9 does not say what the user must provide",
            "11: section 9 still names DEV-001, which 04 says is 完成",
        ])
        self.assertEqual(self.run_check(need=["DEV-013", "DEV-068", "DEV-133", "DEV-999"]),
                         ["11: section 9 names unknown DEV-999"])
        with tempfile.TemporaryDirectory() as root:
            write(root, BASE)
            f = pathlib.Path(root) / "docs" / "math-workbench" / "11-运维交接与已知缺口.md"
            f.write_text(f.read_text(encoding="utf-8").split("\n## 9")[0], encoding="utf-8")
            self.assertEqual(ho.check(root)[0], ["11: section 9 (what the user must provide or decide) is missing"])


if __name__ == "__main__":
    unittest.main()
