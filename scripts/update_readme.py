#!/usr/bin/env python3
"""发布时自动更新 README.md：
  1. 「当前版本」行：版本号 + 从 docs/releases/vX.Y.Z.md 提取的第一段摘要。
  2. 「安装包架构选择」表格里的 UFI-Gate-<version>-arm{32,64}.js 文件名。
  3. 「版本记录」表格顶部插入一行：版本号 + 摘要 + 详情链接。

幂等：重复跑同一版本不会产生重复行；会先清掉旧行再插新行。
用法：python3 scripts/update_readme.py <X.Y.Z>
"""
from __future__ import annotations

import re
import sys
from pathlib import Path


def extract_summary(release_md: Path) -> str:
    """取 release notes 中标题之后的第一段非空正文，到空行或下一个 # 标题为止。"""
    lines = release_md.read_text(encoding="utf-8").splitlines()
    i = 0
    n = len(lines)
    # 跳过开头空行
    while i < n and not lines[i].strip():
        i += 1
    # 跳过一级标题（# UFI-Gate vX.Y.Z）
    if i < n and lines[i].lstrip().startswith("#"):
        i += 1
    # 跳过标题后的空行
    while i < n and not lines[i].strip():
        i += 1
    buf: list[str] = []
    while i < n:
        line = lines[i].strip()
        if not line or line.startswith("#"):
            break
        buf.append(line)
        i += 1
    summary = "".join(buf).strip()
    return summary.rstrip("。")


def main() -> int:
    if len(sys.argv) != 2:
        print("usage: update_readme.py <X.Y.Z>", file=sys.stderr)
        return 1
    version = sys.argv[1]
    if not re.fullmatch(r"\d+\.\d+\.\d+", version):
        print(f"bad version: {version}", file=sys.stderr)
        return 1
    tag = f"v{version}"

    root = Path(__file__).resolve().parent.parent
    readme = root / "README.md"
    release_md = root / "docs" / "releases" / f"{tag}.md"
    if not release_md.exists():
        print(f"missing release notes: {release_md}", file=sys.stderr)
        return 1

    summary = extract_summary(release_md)
    if not summary:
        print(f"empty summary in {release_md}", file=sys.stderr)
        return 1
    summary_cell = summary.replace("|", "\\|")
    notes_link = f"docs/releases/{tag}.md"

    text = readme.read_text(encoding="utf-8")

    # 1. 当前版本整行替换
    new_current = f"**当前版本：`{version}`。** {summary}。"
    text, n = re.subn(
        r"\*\*当前版本：`[^`]+`。\*\*[^\n]*",
        new_current,
        text,
        count=1,
    )
    if n != 1:
        print("failed to locate '当前版本' line", file=sys.stderr)
        return 1

    # 2. 安装包文件名（两处：arm32 / arm64）
    text, n32 = re.subn(
        r"UFI-Gate-\d+\.\d+\.\d+-arm32\.js",
        f"UFI-Gate-{version}-arm32.js",
        text,
    )
    text, n64 = re.subn(
        r"UFI-Gate-\d+\.\d+\.\d+-arm64\.js",
        f"UFI-Gate-{version}-arm64.js",
        text,
    )
    if n32 != 1 or n64 != 1:
        print(f"expected 1 arm32/arm64 filename, got {n32}/{n64}", file=sys.stderr)
        return 1

    # 3. 版本记录表格：先清掉同版本旧行（幂等），再在表头下插新行
    text = re.sub(
        rf"\n\| `{re.escape(version)}` \| [^\n]*\|",
        "",
        text,
    )
    new_row = f"| `{version}` | {summary_cell}。[详情]({notes_link}) |"
    pattern = re.compile(r"(\| 版本 \| 说明 \|\n\| --- \| --- \|\n)")
    text, n = pattern.subn(
        lambda m: m.group(1) + new_row + "\n",
        text,
        count=1,
    )
    if n != 1:
        print("failed to locate changelog table header", file=sys.stderr)
        return 1

    readme.write_text(text, encoding="utf-8")
    print(f"README.md updated for {tag}: {summary}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
