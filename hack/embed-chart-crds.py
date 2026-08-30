#!/usr/bin/env python3
"""Install generated CRDs via Helm templates so helm upgrade updates the schema."""

from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SRC = ROOT / "config" / "crd" / "bases"
TEMPLATES = ROOT / "charts" / "karkive" / "templates"
LEGACY = ROOT / "charts" / "karkive" / "crds"


def main() -> None:
    TEMPLATES.mkdir(parents=True, exist_ok=True)
    for old in TEMPLATES.glob("crd-*.yaml"):
        old.unlink()
    for src in sorted(SRC.glob("*.yaml")):
        body = src.read_text()
        if not body.endswith("\n"):
            body += "\n"
        (TEMPLATES / f"crd-{src.name}").write_text(
            "{{- if .Values.crds.install }}\n" + body + "{{- end }}\n"
        )
    if LEGACY.is_dir():
        for p in LEGACY.glob("*.yaml"):
            p.unlink()
        LEGACY.rmdir()


if __name__ == "__main__":
    main()
