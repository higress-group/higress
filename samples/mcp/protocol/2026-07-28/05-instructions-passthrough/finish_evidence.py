#!/usr/bin/env python3
"""Publish this demo's final status and checksums after restoration/cleanup."""
import argparse
import hashlib
import json
import os
import sys
import tempfile
from pathlib import Path


def atomic_write(path, value):
    temporary = None
    try:
        with tempfile.NamedTemporaryFile(mode="w", encoding="utf-8", dir=path.parent, prefix="." + path.name + ".", delete=False) as stream:
            temporary = Path(stream.name)
            stream.write(value)
        os.replace(temporary, path)
    finally:
        if temporary is not None:
            temporary.unlink(missing_ok=True)


def finish(root, initial_status):
    status = initial_status
    errors, phases = [], {}

    def failed(label, error):
        nonlocal status
        status = status or 1
        message = f"{label}: {error}"
        if message not in errors:
            errors.append(message)
            print(message, file=sys.stderr)

    for phase in ("direct", "on", "off", "changed"):
        try:
            report = json.loads((root / (phase + ".json")).read_text())
            if not isinstance(report, dict) or report.get("phase") != phase:
                raise ValueError("phase report is invalid")
            phases[phase] = report
        except (OSError, ValueError) as exc:
            failed(phase + ".json", exc)

    def publish_status():
        # A failure writing either status carrier must also reach the other.
        for _ in range(2):
            before = status
            summary = json.dumps({"exitCode": status, "phases": phases, "errors": errors}, ensure_ascii=False, indent=2) + "\n"
            completion = f"exit={status}; case port-forward stopped; see restore.log; case resources retained for manual repeat\n"
            for name, value in (("summary.json", summary), ("completion.txt", completion)):
                try:
                    # Recompute completion if summary publication just failed.
                    if name == "completion.txt":
                        value = f"exit={status}; case port-forward stopped; see restore.log; case resources retained for manual repeat\n"
                    atomic_write(root / name, value)
                except OSError as exc:
                    failed(name, exc)
            if status == before:
                break

    def checksums():
        return "".join(hashlib.sha256(path.read_bytes()).hexdigest() + "  " + path.name + "\n"
                       for path in sorted(root.iterdir()) if path.is_file() and path.name not in ("SHA256SUMS", "verify.log"))

    publish_status()
    for attempt in range(2):
        try:
            atomic_write(root / "SHA256SUMS", checksums())
            break
        except OSError as exc:
            failed("SHA256SUMS", exc)
            # Never leave a previous success manifest after status changed.
            try:
                if (root / "SHA256SUMS").is_file():
                    (root / "SHA256SUMS").unlink()
            except OSError as remove_error:
                failed("remove stale SHA256SUMS", remove_error)
            publish_status()
    return status


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("root", type=Path)
    parser.add_argument("status", type=int)
    args = parser.parse_args()
    raise SystemExit(finish(args.root, args.status))
