#!/usr/bin/env python3
"""Validate bounded Envoy concurrency from resolved Compose JSON."""

import json
import sys
from pathlib import Path


def check(condition, message):
    if not condition:
        raise RuntimeError(message)


config = json.loads(Path(sys.argv[1]).read_text())
envoy_commands = []
for name, service in config.get("services", {}).items():
    entrypoint = service.get("entrypoint") or []
    if "/usr/local/bin/envoy" not in entrypoint:
        continue
    command = service.get("command") or []
    envoy_commands.append((name, command))

check(len(envoy_commands) == 11, f"expected 11 resolved Envoy services, got {len(envoy_commands)}")
for name, command in envoy_commands:
    positions = [index for index, token in enumerate(command) if token == "--concurrency"]
    check(len(positions) == 1, f"{name} has invalid concurrency flags: {command}")
    position = positions[0]
    check(position + 1 < len(command) and str(command[position + 1]) == "1",
          f"{name} concurrency is not exactly one worker: {command}")

print("resolved Compose Envoy concurrency self-test passed")
