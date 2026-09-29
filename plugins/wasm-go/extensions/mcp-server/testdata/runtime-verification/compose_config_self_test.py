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

expected = {"gateway", "gateway-auto", "gateway-auto-baseline", "gateway-auto-explicit-baseline", "gateway-baseline", "gateway-oracle", "gateway-generation"}
expected.update("gateway-control-" + revision for revision in ("candidate", "affected", "oracle"))
expected.update("gateway-corpus-" + revision for revision in ("candidate", "affected", "oracle"))
check({name for name, _ in envoy_commands} == expected, "resolved Envoy service set is incomplete or unexpected")
for name, command in envoy_commands:
    positions = [index for index, token in enumerate(command) if token == "--concurrency"]
    check(len(positions) == 1, f"{name} has invalid concurrency flags: {command}")
    position = positions[0]
    check(position + 1 < len(command) and str(command[position + 1]) == "1",
          f"{name} concurrency is not exactly one worker: {command}")

print("resolved Compose Envoy concurrency self-test passed")
