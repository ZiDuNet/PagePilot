#!/usr/bin/env python3
"""重新生成内置 pagep Skill 下载包。"""

from __future__ import annotations

from pathlib import Path
import zipfile


ROOT = Path(__file__).resolve().parents[1]
SKILL_ROOT = ROOT / "skill" / "hostctl-deploy"
OUTPUT = ROOT / "internal" / "web" / "skill" / "hostctl-deploy.zip"
ZIP_PREFIX = Path("pagep")
EXCLUDED_SUFFIXES = {".pyc", ".pyo"}


def should_include(path: Path) -> bool:
    """判断文件是否应该进入 Skill ZIP。"""
    rel = path.relative_to(SKILL_ROOT)
    if "__pycache__" in rel.parts:
        return False
    return path.suffix not in EXCLUDED_SUFFIXES


def main() -> None:
    """按稳定目录结构打包 Skill。"""
    OUTPUT.parent.mkdir(parents=True, exist_ok=True)
    with zipfile.ZipFile(OUTPUT, "w", zipfile.ZIP_DEFLATED) as archive:
        for path in sorted(SKILL_ROOT.rglob("*")):
            if not path.is_file() or not should_include(path):
                continue
            rel = path.relative_to(SKILL_ROOT)
            archive.write(path, ZIP_PREFIX / rel)

    with zipfile.ZipFile(OUTPUT, "r") as archive:
        names = set(archive.namelist())
        if "pagep/SKILL.md" not in names:
            raise SystemExit("generated Skill ZIP missing pagep/SKILL.md")
        bad = [
            name
            for name in names
            if "__pycache__" in Path(name).parts or Path(name).suffix in EXCLUDED_SUFFIXES
        ]
        if bad:
            raise SystemExit(f"generated Skill ZIP contains cache files: {bad}")
        print(f"wrote {OUTPUT} ({len(names)} entries)")


if __name__ == "__main__":
    main()
