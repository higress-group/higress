#!/usr/bin/env bash
set -u

HARNESS_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
. "$HARNESS_DIR/lifecycle.sh"

fail() {
  echo "lifecycle self-test failed: $1" >&2
  exit 1
}

runtime_sleep() { :; }
compose_status=0
service_status=1
compose_runtime() {
  test "$1" = stop || return 2
  return "$compose_status"
}
runtime_service_running() { return "$service_status"; }

compose_status=1
service_status=1
RUNTIME_STOP_ATTEMPTS=1 stop_runtime_service stopped-after-error || \
  fail "non-zero stop was rejected despite confirmed stopped state"

compose_status=1
service_status=0
if RUNTIME_STOP_ATTEMPTS=1 stop_runtime_service still-running; then
  fail "non-zero stop was accepted while the container remained running"
fi

compose_status=0
service_status=0
if RUNTIME_STOP_ATTEMPTS=1 stop_runtime_service success-but-running; then
  fail "successful stop command bypassed the running-state inspection"
fi

health_attempt=0
reset_attempt=0
backend_health_probe() {
  health_attempt=$((health_attempt + 1))
  test "$health_attempt" -ge 2
}
backend_reset_probe() {
  reset_attempt=$((reset_attempt + 1))
  test "$reset_attempt" -ge 2
}
RUNTIME_BACKEND_HEALTH_ATTEMPTS=3 RUNTIME_BACKEND_RESET_ATTEMPTS=3 reset_primary_backend || \
  fail "transient backend health/reset failures did not recover"
test "$health_attempt" -eq 2 || fail "health retry count changed"
test "$reset_attempt" -eq 2 || fail "reset retry count changed"

backend_health_probe() { return 1; }
if RUNTIME_BACKEND_HEALTH_ATTEMPTS=2 reset_primary_backend; then
  fail "permanently unavailable backend was accepted"
fi

backend_health_probe() { return 0; }
backend_reset_probe() { return 1; }
if RUNTIME_BACKEND_RESET_ATTEMPTS=2 reset_primary_backend; then
  fail "permanent reset failure was accepted"
fi

events=""
record_event() { events="${events}${events:+,}$1"; }
reset_primary_backend() { record_event reset; }
start_runtime_service() { record_event "start:$1"; }
wait_for_rejection_markers() { record_event "wait:$1:$2:$3"; }
stop_runtime_service() { record_event "stop:$1"; }
capture_gateway_log() { record_event "log:$1:$2"; }
capture_empty_primary_backend() { record_event "state:$1"; }

run_static_rejection_phase static-gateway static.log static-state.json marker-a marker-b || \
  fail "structured static rejection phase failed"
expected="reset,start:static-gateway,wait:static-gateway:marker-a:marker-b,stop:static-gateway,log:static-gateway:static.log,state:static-state.json"
test "$events" = "$expected" || fail "static phase order changed: $events"

events=""
run_corpus_verifier() { record_event "verify:$1"; }
run_corpus_revisions evidence || fail "structured corpus revisions failed"
expected="reset,start:gateway-corpus-candidate,verify:candidate,stop:gateway-corpus-candidate,log:gateway-corpus-candidate:evidence/gateway-corpus-candidate.log,reset,start:gateway-corpus-affected,verify:affected,stop:gateway-corpus-affected,log:gateway-corpus-affected:evidence/gateway-corpus-affected.log,reset,start:gateway-corpus-oracle,verify:oracle,stop:gateway-corpus-oracle,log:gateway-corpus-oracle:evidence/gateway-corpus-oracle.log"
test "$events" = "$expected" || fail "corpus revision order changed: $events"

events=""
stop_runtime_service() {
  record_event "stop:$1"
  test "$1" != gateway-corpus-candidate
}
corpus_status=0
run_corpus_revisions evidence || corpus_status=$?
test "$corpus_status" -eq "$CORPUS_LIFECYCLE_FAILED" || fail "stop failure was not fatal"
case "$events" in
  *"start:gateway-corpus-affected"*) fail "affected started after candidate stop failure" ;;
esac
expected="reset,start:gateway-corpus-candidate,verify:candidate,stop:gateway-corpus-candidate"
test "$events" = "$expected" || fail "events continued after candidate stop failure: $events"

echo "runtime lifecycle fault-injection self-tests passed"
