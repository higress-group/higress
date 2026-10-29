#!/usr/bin/env bash
set -euo pipefail
unset GH_TOKEN GITHUB_TOKEN
export PYTHONDONTWRITEBYTECODE=1
compat_harness_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
exec "${COMPAT_PYTHON:-python3}" "$compat_harness_dir/run_compat.py" "$@"
