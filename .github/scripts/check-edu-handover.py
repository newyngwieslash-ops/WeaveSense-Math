#!/usr/bin/env python3
"""Consistency gate for the education-math handover docs (DEV-127).

Checks, using only the files in docs/math-workbench:
* every DEV in the checklist has exactly one row in 04 with a known status;
* the checklist box is [x] exactly when 04 says 完成;
* the 04 summary table equals the counts computed from the per-item rows;
* 11 lists every DEV that is not 完成, with the same status, and nothing else;
* 09 section 3.1 (DEV-007 entity table) names every edu_ table in models_edu.go.
Exit 1 with a list of problems otherwise.
"""
import re
import sys
from collections import Counter
from pathlib import Path

STATUSES = ("完成", "进行中", "待开始", "阻塞")
PACKAGES = [("W0", 1, 12), ("W1", 13, 28), ("W2", 29, 43), ("W3", 44, 66), ("W4", 67, 83),
            ("W5", 84, 97), ("W6", 98, 112), ("W7", 113, 127), ("试点", 128, 133)]


def package(dev):
    n = int(dev.split("-")[1])
    for name, lo, hi in PACKAGES:
        if lo <= n <= hi:
            return name
    return None


def table_rows(text, prefix):
    for line in text.splitlines():
        if line.startswith("| " + prefix):
            yield [c.strip() for c in line.strip().strip("|").split("|")]


def check(root):
    d = Path(root) / "docs" / "math-workbench"
    matrix = (d / "04-追踪矩阵.md").read_text(encoding="utf-8")
    checklist = (d / "WeaveSense-Flow_Development_Checklist.md").read_text(encoding="utf-8")
    handover = (d / "11-运维交接与已知缺口.md").read_text(encoding="utf-8")
    problems = []

    status = {}
    for cells in table_rows(matrix, "DEV-"):
        dev, st = cells[0], cells[2]
        if dev in status:
            problems.append(f"04: {dev} appears twice")
        if st not in STATUSES:
            problems.append(f"04: {dev} has unknown status {st!r}")
        status[dev] = st

    boxes = {m[2]: m[1] == "x" for m in re.finditer(r"^- \[( |x)\] \*\*(DEV-\d+)", checklist, re.M)}
    for dev in sorted(set(boxes) | set(status)):
        if dev not in status:
            problems.append(f"04: {dev} from the checklist has no row")
        elif dev not in boxes:
            problems.append(f"checklist: {dev} from 04 has no box")
        elif boxes[dev] != (status[dev] == "完成"):
            problems.append(f"checklist: {dev} box is {'[x]' if boxes[dev] else '[ ]'} but 04 says {status[dev]}")

    computed = {name: Counter() for name, _, _ in PACKAGES}
    for dev, st in status.items():
        computed[package(dev)][st] += 1
    total = Counter()
    for c in computed.values():
        total.update(c)
    summary = {}
    for cells in table_rows(matrix, "W"):
        summary[cells[0]] = cells[1:6]
    for cells in table_rows(matrix, "试点"):
        summary[cells[0]] = cells[1:6]
    for cells in table_rows(matrix, "合计"):
        summary[cells[0]] = cells[1:6]
    for name, c in list(computed.items()) + [("合计", total)]:
        want = [str(c[s]) for s in STATUSES] + [str(sum(c.values()))]
        if summary.get(name) != want:
            problems.append(f"04 summary: {name} is {summary.get(name)} but rows give {want}")

    listed = {cells[0]: cells[1] for cells in table_rows(handover, "DEV-")}
    for dev, st in status.items():
        if st != "完成" and listed.get(dev) != st:
            problems.append(f"11: {dev} should be listed as {st}, found {listed.get(dev)}")
    for dev in listed:
        if status.get(dev, "完成") == "完成":
            problems.append(f"11: {dev} is listed as a gap but 04 says 完成")
    problems += entity_problems(Path(root), d)
    problems += image_problems(Path(root), d)
    return problems, total


def entity_problems(root, d):
    models = root / "backend" / "internal" / "model" / "models_edu.go"
    if not models.exists():
        return []
    tables = sorted(set(re.findall(r'TableName\(\) string \{ return "(edu_[a-z_]+)"', models.read_text(encoding="utf-8"))))
    found = list(d.glob("09-*.md"))
    text = found[0].read_text(encoding="utf-8") if found else ""
    start, end = text.find("### 3.1"), text.find("### 3.2")
    section = text[start:end] if 0 <= start < end else ""
    named = set(re.findall(r"edu_[a-z_]+", section))
    return [f"09: entity table {t} from models_edu.go is not listed in section 3.1" for t in tables if t not in named]


DOCKERFILES = ["Dockerfile", "backend/Dockerfile"]


def image_layers(text):
    """FROM images and apk packages named in one Dockerfile."""
    images, packages = [], []
    joined = re.sub(r"\\\n", " ", text)
    for line in joined.splitlines():
        words = line.split()
        if words[:1] == ["FROM"]:
            refs = [w for w in words[1:] if not w.startswith("--")]
            if refs:
                images.append(refs[0])
        for m in re.finditer(r"apk add((?:\s+[^&;|]+)?)", line):
            packages += [w for w in m.group(1).split() if not w.startswith("-")]
    return images, packages


def image_problems(root, d):
    found = list(d.glob("06-*.md"))
    text = found[0].read_text(encoding="utf-8") if found else ""
    start = text.find("### 部署镜像基础层")
    end = text.find("\n### ", start + 1)
    section = text[start:end] if start >= 0 else ""
    named = set(re.findall(r"`([^`]+)`", section))
    problems = []
    for rel in DOCKERFILES:
        f = root / rel
        if not f.exists():
            continue
        images, packages = image_layers(f.read_text(encoding="utf-8"))
        for item in images + packages:
            if item not in named:
                problems.append(f"06: {rel} uses {item} but the base layer section does not list it")
    return problems


def main(argv):
    root = argv[1] if len(argv) > 1 else "."
    problems, total = check(root)
    for p in problems:
        print(p)
    print("edu handover: " + " ".join(f"{s}={total[s]}" for s in STATUSES) + f" problems={len(problems)}")
    return 1 if problems else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
