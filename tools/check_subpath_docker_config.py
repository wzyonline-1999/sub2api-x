#!/usr/bin/env python3
"""Validate Docker config details required by sub-path deployments."""

from __future__ import annotations

import subprocess
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
DOCKERFILES = [ROOT / "Dockerfile", ROOT / "deploy" / "Dockerfile"]
SUBPATH_COMPOSE = ROOT / "deploy" / "docker-compose.sub2api.yml"


def tracked_paths() -> set[str]:
    result = subprocess.run(
        ["git", "ls-files"],
        cwd=ROOT,
        check=True,
        text=True,
        stdout=subprocess.PIPE,
    )
    return {line.strip() for line in result.stdout.splitlines() if line.strip()}


def fail(message: str) -> None:
    print(message, file=sys.stderr)
    raise SystemExit(1)


def main() -> None:
    tracked = tracked_paths()
    tracked_docs_legal = any(path.startswith("docs/legal/") for path in tracked)

    for dockerfile in DOCKERFILES:
        text = dockerfile.read_text(encoding="utf-8")
        rel = dockerfile.relative_to(ROOT)

        if "COPY docs/legal/" in text and not tracked_docs_legal:
            fail(f"{rel}: references docs/legal, but no docs/legal files are tracked")

        if rel.name == "Dockerfile" and "HEALTHCHECK" in text:
            healthcheck_line = "\n".join(
                line for line in text.splitlines() if "HEALTHCHECK" in line or "wget" in line
            )
            if "SERVER_BASE_PATH" not in healthcheck_line:
                fail(f"{rel}: healthcheck must include SERVER_BASE_PATH")
            if "${base_path%/}" not in healthcheck_line:
                fail(f"{rel}: healthcheck must trim trailing slash from SERVER_BASE_PATH")
            if "${base_path#/}" not in healthcheck_line:
                fail(f"{rel}: healthcheck must normalize missing leading slash in SERVER_BASE_PATH")

    compose_text = SUBPATH_COMPOSE.read_text(encoding="utf-8")
    if "$${base_path%/}" not in compose_text:
        fail(f"{SUBPATH_COMPOSE.relative_to(ROOT)}: healthcheck must trim trailing slash from SERVER_BASE_PATH")
    if "$${base_path#/}" not in compose_text:
        fail(f"{SUBPATH_COMPOSE.relative_to(ROOT)}: healthcheck must normalize missing leading slash in SERVER_BASE_PATH")


if __name__ == "__main__":
    main()
