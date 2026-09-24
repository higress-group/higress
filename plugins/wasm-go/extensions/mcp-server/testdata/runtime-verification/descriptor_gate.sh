#!/usr/bin/env bash
set -u

HARNESS_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
EVIDENCE_DIR=${1:?usage: descriptor_gate.sh EVIDENCE_DIR [VERIFY_CHECKER] [FINALIZER_CHECKER]}
VERIFY_CHECKER=${2:-$HARNESS_DIR/verify.py}
FINALIZER_CHECKER=${3:-$HARNESS_DIR/finalize_evidence.py}
DESCRIPTOR_MISMATCH_EXIT=$(PYTHONPATH="$HARNESS_DIR" python3 -c \
  'from typed_canonical import DESCRIPTOR_MISMATCH_EXIT; print(DESCRIPTOR_MISMATCH_EXIT)') || exit 3

for checker in "$VERIFY_CHECKER" "$FINALIZER_CHECKER"; do
  for fixture_and_file in \
    "unsupported-semantics:.descriptor-selftest-structure-good.json" \
    "numeric-comparison-limit:.descriptor-selftest-number-good.json"; do
    fixture=${fixture_and_file%%:*}
    actual=${fixture_and_file#*:}
    if ! RUNTIME_EVIDENCE="$EVIDENCE_DIR" RUNTIME_DESCRIPTOR_SELF_TEST=1 \
      RUNTIME_DESCRIPTOR_FIXTURE="$fixture" RUNTIME_DESCRIPTOR_ACTUAL="$EVIDENCE_DIR/$actual" \
      python3 "$checker"; then
      echo "$checker rejected positive descriptor $actual" >&2
      exit 3
    fi
  done
  for fixture_and_file in \
    "unsupported-semantics:.descriptor-selftest-structure-deleted.json" \
    "unsupported-semantics:.descriptor-selftest-array-truncated.json" \
    "numeric-comparison-limit:.descriptor-selftest-number-as-string.json"; do
    fixture=${fixture_and_file%%:*}
    actual=${fixture_and_file#*:}
    if RUNTIME_EVIDENCE="$EVIDENCE_DIR" RUNTIME_DESCRIPTOR_SELF_TEST=1 \
      RUNTIME_DESCRIPTOR_FIXTURE="$fixture" RUNTIME_DESCRIPTOR_ACTUAL="$EVIDENCE_DIR/$actual" \
      python3 "$checker" >/dev/null 2>&1; then
      checker_status=0
    else
      checker_status=$?
    fi
    case "$checker_status" in
      "$DESCRIPTOR_MISMATCH_EXIT") ;;
      0)
        echo "$checker accepted tampered descriptor $actual" >&2
        exit 3
        ;;
      *)
        echo "$checker failed while checking $actual: expected exit $DESCRIPTOR_MISMATCH_EXIT, got $checker_status" >&2
        exit 3
        ;;
    esac
  done
done

echo "descriptor canonical positive and tamper-negative self-tests passed"
